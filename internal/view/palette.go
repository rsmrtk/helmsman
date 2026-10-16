// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

package view

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"

	"github.com/derailed/tcell/v2"
	"github.com/derailed/tview"
	"github.com/rsmrtk/helmsman/internal/client"
	"github.com/rsmrtk/helmsman/internal/config"
	"github.com/rsmrtk/helmsman/internal/slogs"
	"github.com/sahilm/fuzzy"
	"gopkg.in/yaml.v3"
)

const (
	palettePage    = "palette"
	bookmarksFile  = "bookmarks.yaml"
	paletteWidth   = 84
	paletteHeight  = 22
	paletteMaxRows = 200
)

// paletteResources are the resource views offered in the palette, as typed after ":".
var paletteResources = []string{
	"pods", "deployments", "statefulsets", "daemonsets", "replicasets", "jobs", "cronjobs",
	"services", "ingresses", "configmaps", "secrets", "serviceaccounts",
	"nodes", "namespaces", "events", "persistentvolumes", "persistentvolumeclaims",
	"horizontalpodautoscalers", "networkpolicies", "storageclasses",
}

// Bookmark is a saved cluster, namespace and view.
type Bookmark struct {
	Context   string `yaml:"context"`
	Namespace string `yaml:"namespace"`
	View      string `yaml:"view"`
}

func (b Bookmark) label() string {
	return fmt.Sprintf("★ %s › %s › %s", b.Context, b.Namespace, b.View)
}

// command is what the prompt would take: "pods demo @kind-practice".
func (b Bookmark) command() string {
	return fmt.Sprintf("%s %s @%s", b.View, b.Namespace, b.Context)
}

type paletteItem struct {
	label, cmd string
}

func bookmarksPath() string {
	return filepath.Join(config.AppConfigDir, bookmarksFile)
}

func loadBookmarks() []Bookmark {
	bb, err := os.ReadFile(bookmarksPath())
	if err != nil {
		return nil
	}
	var out []Bookmark
	if err := yaml.Unmarshal(bb, &out); err != nil {
		slog.Warn("Unable to read bookmarks", slogs.Error, err)
		return nil
	}

	return out
}

func saveBookmarks(bb []Bookmark) error {
	if err := os.MkdirAll(filepath.Dir(bookmarksPath()), 0o750); err != nil {
		return err
	}
	data, err := yaml.Marshal(bb)
	if err != nil {
		return err
	}

	return os.WriteFile(bookmarksPath(), data, 0o600)
}

// bookmarkCmd saves, or removes when already saved, the current cluster, namespace and view.
func (a *App) bookmarkCmd(*tcell.EventKey) *tcell.EventKey {
	top, ok := a.Content.Top().(interface{ GVR() *client.GVR })
	if !ok {
		a.Flash().Warn("Nothing to bookmark here")
		return nil
	}
	ns := client.CleanseNamespace(a.Config.ActiveNamespace())
	if ns == "" {
		ns = client.NamespaceAll
	}
	cur := Bookmark{Context: a.Config.ActiveContextName(), Namespace: ns, View: top.GVR().String()}

	bb := loadBookmarks()
	for i, b := range bb {
		if b == cur {
			bb = append(bb[:i], bb[i+1:]...)
			if err := saveBookmarks(bb); err != nil {
				a.Flash().Err(err)
				return nil
			}
			a.Flash().Infof("Bookmark removed: %s", cur.label())
			return nil
		}
	}
	if err := saveBookmarks(append(bb, cur)); err != nil {
		a.Flash().Err(err)
		return nil
	}
	a.Flash().Infof("Bookmarked: %s", cur.label())

	return nil
}

func (a *App) paletteItems() []paletteItem {
	var items []paletteItem
	for _, b := range loadBookmarks() {
		items = append(items, paletteItem{b.label(), b.command()})
	}

	if cc, err := a.Conn().Config().Contexts(); err == nil {
		names := make([]string, 0, len(cc))
		for n := range cc {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			items = append(items, paletteItem{"⎈ cluster  " + n, "ctx " + n})
		}
	}

	if nn, err := a.factory.Client().ValidNamespaceNames(); err == nil {
		names := make([]string, 0, len(nn))
		for n := range nn {
			names = append(names, n)
		}
		sort.Strings(names)
		items = append(items, paletteItem{"▣ namespace  all", "pods " + client.NamespaceAll})
		for _, n := range names {
			items = append(items, paletteItem{"▣ namespace  " + n, "pods " + n})
		}
	}

	for _, r := range paletteResources {
		items = append(items, paletteItem{"≡ " + r, r})
	}

	return items
}

// paletteCmd opens a fuzzy jump palette over bookmarks, clusters, namespaces and resources.
func (a *App) paletteCmd(*tcell.EventKey) *tcell.EventKey {
	if a.Content.Pages.HasPage(palettePage) {
		return nil
	}
	var (
		items  = a.paletteItems()
		shown  = items
		styles = a.Styles
		accent = styles.Frame().Border.FocusColor.Color()
		input  = tview.NewInputField()
		list   = tview.NewList()
	)
	labels := make([]string, len(items))
	for i, it := range items {
		labels[i] = it.label
	}

	input.SetLabel(" ⌕ ").SetLabelColor(accent)
	input.SetFieldBackgroundColor(styles.BgColor()).SetFieldTextColor(styles.FgColor())
	input.SetBackgroundColor(styles.BgColor())

	list.ShowSecondaryText(false).SetHighlightFullLine(true)
	list.SetMainTextColor(styles.FgColor()).SetBackgroundColor(styles.BgColor())
	list.SetSelectedBackgroundColor(accent).SetSelectedTextColor(styles.BgColor())

	refill := func(q string) {
		list.Clear()
		shown = shown[:0:0]
		if q == "" {
			shown = items
		} else {
			for _, m := range fuzzy.Find(q, labels) {
				shown = append(shown, items[m.Index])
			}
		}
		if len(shown) > paletteMaxRows {
			shown = shown[:paletteMaxRows]
		}
		for _, it := range shown {
			list.AddItem(it.label, "", 0, nil)
		}
	}
	refill("")

	close := func() {
		a.Content.Pages.RemovePage(palettePage)
	}
	choose := func() {
		i := list.GetCurrentItem()
		if i < 0 || i >= len(shown) {
			return
		}
		cmd := shown[i].cmd
		close()
		a.gotoResource(cmd, "", false, true)
	}
	input.SetChangedFunc(refill)
	input.SetInputCapture(func(evt *tcell.EventKey) *tcell.EventKey {
		switch evt.Key() {
		case tcell.KeyEscape:
			close()
			return nil
		case tcell.KeyEnter:
			choose()
			return nil
		case tcell.KeyDown, tcell.KeyCtrlN, tcell.KeyTab:
			list.SetCurrentItem(min(list.GetCurrentItem()+1, max(list.GetItemCount()-1, 0)))
			return nil
		case tcell.KeyUp, tcell.KeyCtrlK, tcell.KeyBacktab:
			list.SetCurrentItem(max(list.GetCurrentItem()-1, 0))
			return nil
		default:
			return evt
		}
	})

	body := tview.NewFlex().SetDirection(tview.FlexRow)
	body.AddItem(input, 1, 0, true)
	body.AddItem(list, 0, 1, false)
	body.SetBorder(true).SetBorderColor(accent).SetTitleColor(accent)
	body.SetTitle(" Jump  (↑↓ choose · Enter go · Esc close) ")
	body.SetBackgroundColor(styles.BgColor())

	// centred modal: spacers around a fixed-size box
	row := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(body, paletteWidth, 0, true).
		AddItem(nil, 0, 1, false)
	col := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(row, paletteHeight, 0, true).
		AddItem(nil, 0, 2, false)

	a.Content.Pages.AddPage(palettePage, col, true, true)
	a.SetFocus(col)

	return nil
}
