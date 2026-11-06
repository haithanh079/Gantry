package landscape

import (
	"sort"

	"github.com/go2engle/gantry/internal/entity"
)

// InventoryCell is one Service x Environment intersection.
type InventoryCell struct {
	Env       string   `json:"env"`
	Runtime   string   `json:"runtime,omitempty"`
	Region    string   `json:"region,omitempty"`
	Version   string   `json:"version,omitempty"`
	Cloud     string   `json:"cloud,omitempty"`
	Deployment string  `json:"deployment"`
}

// InventoryRow is a service and the environments it is deployed in.
type InventoryRow struct {
	Service   string                   `json:"service"`
	Namespace string                   `json:"namespace,omitempty"`
	Owner     string                   `json:"owner,omitempty"`
	Cells     map[string]InventoryCell `json:"cells"`
}

// InventoryGroup is a project and its services.
type InventoryGroup struct {
	Project string         `json:"project"`
	Rows    []InventoryRow `json:"rows"`
}

// Inventory is the Service x Env matrix plus summary counts.
type Inventory struct {
	Groups []InventoryGroup `json:"groups"`
	KPIs   map[string]int   `json:"kpis"`
	Envs   []string         `json:"envs"`
}

// Inventory builds the matrix using the same filters as the graph.
func (m *Model) Inventory(q GraphQuery) Inventory {
	views := m.visibleDeployments(q)
	type rowAcc struct {
		row InventoryRow
	}
	groups := map[string]map[string]*rowAcc{}
	envs := map[string]bool{}
	clouds := map[string]bool{}
	regions := map[string]bool{}
	projects := map[string]bool{}
	for _, view := range views {
		if view.TargetKind != "" && view.TargetKind != "Service" {
			continue
		}
		project := view.ProjectName
		if project == "" {
			project = "unassigned"
		}
		projects[project] = true
		if groups[project] == nil {
			groups[project] = map[string]*rowAcc{}
		}
		key := string(view.Target)
		row := groups[project][key]
		if row == nil {
			target := m.Get(view.Target)
			name := view.Entity.Metadata.Name
			ns := ""
			owner := view.Owner
			if target != nil {
				name = target.Metadata.Name
				ns = target.Metadata.Namespace
				if owner == "" {
					owner = target.Metadata.Owner
				}
			}
			row = &rowAcc{row: InventoryRow{Service: name, Namespace: ns, Owner: owner, Cells: map[string]InventoryCell{}}}
			groups[project][key] = row
		}
		region := ""
		if len(view.Regions) > 0 {
			region = view.Regions[0]
			regions[region] = true
		}
		if view.Cloud != "" {
			clouds[view.Cloud] = true
		}
		envs[view.EnvName] = true
		row.row.Cells[view.EnvName] = InventoryCell{
			Env: view.EnvName, Runtime: view.Runtime, Region: region, Cloud: view.Cloud,
			Version: specString(view.Entity.Spec, "version"), Deployment: string(view.ID),
		}
	}
	var names []string
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)
	inv := Inventory{KPIs: map[string]int{
		"deployments": len(views),
		"services":    0,
		"clouds":      len(clouds),
		"regions":     len(regions),
		"projects":    len(projects),
		"dataflows":   len(m.DataFlows(q)),
	}}
	for _, name := range names {
		var rows []InventoryRow
		for _, row := range groups[name] {
			rows = append(rows, row.row)
			inv.KPIs["services"]++
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].Service < rows[j].Service })
		inv.Groups = append(inv.Groups, InventoryGroup{Project: name, Rows: rows})
	}
	if inv.Groups == nil {
		inv.Groups = []InventoryGroup{}
	}
	for env := range envs {
		inv.Envs = append(inv.Envs, env)
	}
	sort.Strings(inv.Envs)
	return inv
}

// DataFlowRow is one catalogued data flow.
type DataFlowRow struct {
	Ref         string   `json:"ref"`
	Name        string   `json:"name"`
	Namespace   string   `json:"namespace,omitempty"`
	Source      string   `json:"source"`
	Target      string   `json:"target"`
	Actor       string   `json:"actor,omitempty"`
	Mode        string   `json:"mode,omitempty"`
	Schedule    string   `json:"schedule,omitempty"`
	Transport   string   `json:"transport,omitempty"`
	DataClasses []string `json:"dataClasses,omitempty"`
	Owner       string   `json:"owner,omitempty"`
	Status      string   `json:"status,omitempty"`
}

// DataFlows lists data flows whose actor or an endpoint deployment passes q.
func (m *Model) DataFlows(q GraphQuery) []DataFlowRow {
	visible := map[entity.CanonicalRef]DeploymentView{}
	for _, view := range m.visibleDeployments(q) {
		visible[view.ID] = view
	}
	var rows []DataFlowRow
	for _, flow := range m.OfKind("DataFlow") {
		if !m.dataFlowVisible(flow, visible, q) {
			continue
		}
		row := DataFlowRow{
			Ref: string(entity.IDOf(flow)), Name: flow.Metadata.Name, Namespace: flow.Metadata.Namespace,
			Owner: flow.Metadata.Owner, Mode: specString(flow.Spec, "mode"), Schedule: specString(flow.Spec, "schedule"),
			Transport: specString(flow.Spec, "transport"), DataClasses: specStrings(flow.Spec, "dataClasses"),
			Status: specString(flow.Spec, "status"),
		}
		if ref, ok := specRef(flow.Spec, "source"); ok {
			row.Source = string(entity.ResolveRef(flow, ref))
		}
		if ref, ok := specRef(flow.Spec, "target"); ok {
			row.Target = string(entity.ResolveRef(flow, ref))
		}
		if ref, ok := specRef(flow.Spec, "actor"); ok {
			row.Actor = string(entity.ResolveRef(flow, ref))
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Ref < rows[j].Ref })
	if rows == nil {
		rows = []DataFlowRow{}
	}
	return rows
}
