// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

package view

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// A read-only look at cloud accounts through the provider CLIs the user is
// already signed in with. helmsman stores no credentials: it runs
// aws / gcloud / az, reads their JSON and draws it in the status palette.

type cloudService struct {
	Provider string // aws | gcp | azure
	Name     string
	Args     []string
	List     string   // path to the list in the JSON; arrays met on the way are flattened
	Cols     []string // header
	Paths    []string // one JSON path per column; "|last" keeps what follows the final /
	State    int      // index of the column that carries the state, or -1
}

func cliOf(provider string) string {
	switch provider {
	case "aws":
		return "aws"
	case "gcp":
		return "gcloud"
	default:
		return "az"
	}
}

var cloudServices = []cloudService{
	{"aws", "ec2", []string{"ec2", "describe-instances", "--output", "json"}, "Reservations.Instances",
		[]string{"NAME", "ID", "STATE", "TYPE", "PRIVATE IP", "PUBLIC IP"},
		[]string{"Tags.Name", "InstanceId", "State.Name", "InstanceType", "PrivateIpAddress", "PublicIpAddress"}, 2},
	{"aws", "s3", []string{"s3api", "list-buckets", "--output", "json"}, "Buckets",
		[]string{"NAME", "CREATED"}, []string{"Name", "CreationDate"}, -1},
	{"aws", "eks", []string{"eks", "list-clusters", "--output", "json"}, "clusters",
		[]string{"NAME"}, []string{"."}, -1},
	{"aws", "rds", []string{"rds", "describe-db-instances", "--output", "json"}, "DBInstances",
		[]string{"NAME", "STATE", "ENGINE", "CLASS"}, []string{"DBInstanceIdentifier", "DBInstanceStatus", "Engine", "DBInstanceClass"}, 1},
	{"gcp", "vm", []string{"compute", "instances", "list", "--format=json"}, "",
		[]string{"NAME", "STATE", "TYPE", "ZONE", "INTERNAL IP"}, []string{"name", "status", "machineType|last", "zone|last", "networkInterfaces.networkIP"}, 1},
	{"gcp", "gke", []string{"container", "clusters", "list", "--format=json"}, "",
		[]string{"NAME", "STATE", "LOCATION", "VERSION"}, []string{"name", "status", "location", "currentMasterVersion"}, 1},
	{"gcp", "buckets", []string{"storage", "buckets", "list", "--format=json"}, "",
		[]string{"NAME", "LOCATION", "CLASS"}, []string{"name", "location", "default_storage_class"}, -1},
	{"azure", "vm", []string{"vm", "list", "-d", "-o", "json"}, "",
		[]string{"NAME", "STATE", "SIZE", "GROUP", "LOCATION"}, []string{"name", "powerState", "hardwareProfile.vmSize", "resourceGroup", "location"}, 1},
	{"azure", "aks", []string{"aks", "list", "-o", "json"}, "",
		[]string{"NAME", "STATE", "VERSION", "GROUP", "LOCATION"}, []string{"name", "provisioningState", "kubernetesVersion", "resourceGroup", "location"}, 1},
	{"azure", "storage", []string{"storage", "account", "list", "-o", "json"}, "",
		[]string{"NAME", "STATE", "GROUP", "LOCATION"}, []string{"name", "provisioningState", "resourceGroup", "location"}, 1},
	{"azure", "group", []string{"group", "list", "-o", "json"}, "",
		[]string{"NAME", "STATE", "LOCATION"}, []string{"name", "properties.provisioningState", "location"}, 1},
}

func findService(provider, name string) (cloudService, bool) {
	for _, s := range cloudServices {
		if s.Provider == provider && s.Name == name {
			return s, true
		}
	}

	return cloudService{}, false
}

// walk follows a dotted path through decoded JSON. A list met on the way is
// iterated, so "Reservations.Instances" flattens two levels; Tags (a list of
// {Key, Value}) is read through the "Tags.Name" convention.
func walk(v any, path string) []any {
	if path == "" {
		if arr, ok := v.([]any); ok {
			return arr
		}
		return []any{v}
	}
	cur := []any{v}
	for _, seg := range strings.Split(path, ".") {
		var next []any
		for _, c := range cur {
			switch t := c.(type) {
			case []any:
				for _, e := range t {
					if m, ok := e.(map[string]any); ok {
						if x, ok := m[seg]; ok {
							next = append(next, x)
						}
					}
				}
			case map[string]any:
				if x, ok := t[seg]; ok {
					next = append(next, x)
				}
			}
		}
		cur = next
	}
	var out []any
	for _, c := range cur {
		if arr, ok := c.([]any); ok {
			out = append(out, arr...)
		} else {
			out = append(out, c)
		}
	}

	return out
}

// field reads one column of one item. "Tags.Name" is the AWS tag list shape.
func field(item any, path string) string {
	if path == "." {
		return fmt.Sprint(item)
	}
	last := false
	if p, ok := strings.CutSuffix(path, "|last"); ok {
		path, last = p, true
	}
	if t, ok := strings.CutPrefix(path, "Tags."); ok {
		m, _ := item.(map[string]any)
		tags, _ := m["Tags"].([]any)
		for _, e := range tags {
			if em, ok := e.(map[string]any); ok && em["Key"] == t {
				return fmt.Sprint(em["Value"])
			}
		}
		return ""
	}
	vals := walk(item, path)
	if len(vals) == 0 || vals[0] == nil {
		return ""
	}
	s := fmt.Sprint(vals[0])
	if last {
		s = s[strings.LastIndex(s, "/")+1:]
	}

	return s
}

// parseCloud turns a CLI's JSON into rows following the service definition.
func parseCloud(s cloudService, raw []byte) ([][]string, error) {
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("unreadable %s output: %w", cliOf(s.Provider), err)
	}
	var rows [][]string
	for _, item := range walk(doc, s.List) {
		row := make([]string, len(s.Paths))
		for i, p := range s.Paths {
			row[i] = field(item, p)
		}
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i][0] < rows[j][0] })

	return rows, nil
}

var cloudStates = map[string]string{
	"running": "running", "available": "running", "active": "running", "succeeded": "running", "ready": "running",
	"vm running": "running", "provisioned": "running", "enabled": "running",
	"pending": "pending", "creating": "pending", "updating": "pending", "starting": "pending", "provisioning": "pending",
	"stopping": "pending", "modifying": "pending", "reconciling": "pending", "vm starting": "pending", "deleting": "terminating",
	"stopped": "completed", "terminated": "completed", "deallocated": "completed", "vm deallocated": "completed",
	"vm stopped": "completed", "suspended": "completed",
	"shutting-down": "terminating", "failed": "crashloopbackoff", "error": "crashloopbackoff",
	"impaired": "crashloopbackoff", "degraded": "crashloopbackoff",
}

func cloudStateKey(state string) string {
	if k, ok := cloudStates[strings.ToLower(strings.TrimSpace(state))]; ok {
		return k
	}

	return ""
}

func cloudText(s cloudService, rows [][]string) string {
	if len(rows) == 0 {
		return fmt.Sprintf("%sno %s %s found%s\n", tagDim, s.Provider, s.Name, tagReset)
	}
	widths := make([]int, len(s.Cols))
	for i, c := range s.Cols {
		widths[i] = len(c)
	}
	for _, r := range rows {
		for i, v := range r {
			widths[i] = min(max(widths[i], len([]rune(v))), 44)
		}
	}
	var sb strings.Builder
	cols := make([]string, len(s.Cols))
	for i, c := range s.Cols {
		cols[i] = pad(c, widths[i])
	}
	sb.WriteString(heading(cols...))
	for _, r := range rows {
		tag := ""
		if s.State >= 0 {
			if k := cloudStateKey(r[s.State]); k != "" {
				tag = statusTag(k)
			}
		}
		parts := make([]string, len(r))
		for i, v := range r {
			parts[i] = pad(truncate(v, widths[i]), widths[i])
		}
		line := strings.Join(parts, "  ")
		if tag != "" {
			fmt.Fprintf(&sb, "%s%s%s\n", tag, line, tagReset)
		} else {
			sb.WriteString(line + "\n")
		}
	}
	fmt.Fprintf(&sb, "\n%s%d %s%s\n", tagDim, len(rows), s.Name, tagReset)

	return sb.String()
}

func truncate(s string, w int) string {
	r := []rune(s)
	if len(r) <= w {
		return s
	}

	return string(r[:w-1]) + "…"
}

func parseJSON(b []byte) (any, error) {
	var doc any
	err := json.Unmarshal(b, &doc)

	return doc, err
}
