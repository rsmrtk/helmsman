// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of K9s

package dao

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/rsmrtk/helmsman/internal"
	"github.com/rsmrtk/helmsman/internal/client"
	"github.com/rsmrtk/helmsman/internal/config"
	"github.com/rsmrtk/helmsman/internal/render"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
)

var _ Accessor = (*Alias)(nil)

// Alias tracks standard and custom command aliases.
type Alias struct {
	NonResource

	*config.Aliases
}

// NewAlias returns a new set of aliases.
func NewAlias(f Factory) *Alias {
	a := Alias{
		Aliases: config.NewAliases(),
	}
	a.Init(f, client.AliGVR)

	return &a
}

// AliasesFor returns a set of aliases for a given gvr.
func (a *Alias) AliasesFor(gvr *client.GVR) sets.Set[string] {
	return a.Aliases.AliasesFor(gvr)
}

// List returns a collection of aliases.
func (*Alias) List(ctx context.Context, _ string) ([]runtime.Object, error) {
	aa, ok := ctx.Value(internal.KeyAliases).(*Alias)
	if !ok {
		return nil, fmt.Errorf("expecting *Alias but got %T", ctx.Value(internal.KeyAliases))
	}
	m := aa.ShortNames()
	oo := make([]runtime.Object, 0, len(m))
	for gvr, aliases := range m {
		sort.StringSlice(aliases).Sort()
		oo = append(oo, render.AliasRes{
			GVR:     gvr,
			Aliases: aliases,
		})
	}

	return oo, nil
}

// Get fetch a resource.
func (*Alias) Get(_ context.Context, _ string) (runtime.Object, error) {
	return nil, errors.New("nyi")
}

// Ensure makes sure alias are loaded.
// crdShortcuts are short commands for custom resources helmsman knows about,
// keyed by "group/resource". They only apply when the cluster has the CRD.
var crdShortcuts = map[string][]string{
	"argoproj.io/applications":            {"argo", "app"},
	"argoproj.io/applicationsets":         {"appset"},
	"serving.kserve.io/inferenceservices": {"isvc"},
	"serving.kserve.io/servingruntimes":   {"sr"},
}

func (a *Alias) Ensure(path string) (config.Alias, error) {
	if err := MetaAccess.LoadResources(a.Factory); err != nil {
		return config.Alias{}, err
	}
	return a.Alias, a.load(path)
}

func (a *Alias) load(path string) error {
	if err := a.Load(path); err != nil {
		return err
	}

	crdGVRS := make(client.GVRs, 0, 50)
	for _, gvr := range MetaAccess.AllGVRs() {
		meta, err := MetaAccess.MetaFor(gvr)
		if err != nil {
			return err
		}
		if IsK9sMeta(meta) {
			continue
		}
		if IsCRD(meta) {
			crdGVRS = append(crdGVRS, gvr)
			continue
		}
		a.Define(gvr, gvr.AsResourceName())

		// Allow single shot commands for k8s resources only expect for metrics resource which override pods and nodes ;(!
		if isStandardGroup(gvr.GVSub()) && gvr.G() != "metrics.k8s.io" {
			a.Define(gvr, meta.Name, meta.SingularName)
		}
		if len(meta.ShortNames) > 0 {
			a.Define(gvr, meta.ShortNames...)
		}
		a.Define(gvr, gvr.String())
	}

	for _, gvr := range crdGVRS {
		meta, err := MetaAccess.MetaFor(gvr)
		if err != nil {
			return err
		}
		a.Define(gvr, strings.ToLower(meta.Kind), meta.Name)
		a.Define(gvr, meta.SingularName)

		if len(meta.ShortNames) > 0 {
			a.Define(gvr, meta.ShortNames...)
		}
		a.Define(gvr, gvr.String())
		a.Define(gvr, meta.Name+"."+meta.Group)
		for _, short := range crdShortcuts[meta.Group+"/"+meta.Name] {
			a.Define(gvr, short)
		}
	}

	return nil
}
