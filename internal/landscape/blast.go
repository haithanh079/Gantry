package landscape

import (
	"sort"

	"github.com/go2engle/gantry/internal/entity"
)

// BlastGroup collects affected deployments and data flows under one project.
type BlastGroup struct {
	Project    string   `json:"project"`
	Department string   `json:"department,omitempty"`
	Deployments []string `json:"deployments"`
	DataFlows  []string `json:"dataFlows"`
}

// BlastRadius is the catalog-only impact of an entity.
type BlastRadius struct {
	Subject entity.CanonicalRef `json:"subject"`
	Groups  []BlastGroup        `json:"groups"`
}

// BlastRadius walks connectsTo and dependsOn backwards from subject and adds
// data flows where subject is the source, target, or actor.
func (m *Model) BlastRadius(subject entity.CanonicalRef) BlastRadius {
	seeds := map[entity.CanonicalRef]bool{}
	if e := m.Get(subject); e != nil && e.Kind == "Deployment" {
		seeds[subject] = true
	}
	if e := m.Get(subject); e != nil && (e.Kind == "Service" || e.Kind == "Infrastructure") {
		for _, view := range m.Deployments() {
			if view.Target == subject {
				seeds[view.ID] = true
			}
		}
	}
	if len(seeds) == 0 {
		seeds[subject] = true
	}

	incoming := map[entity.CanonicalRef][]entity.CanonicalRef{}
	for _, conn := range m.Connections() {
		incoming[conn.To] = append(incoming[conn.To], conn.From)
	}
	affected := map[entity.CanonicalRef]bool{}
	queue := make([]entity.CanonicalRef, 0, len(seeds))
	for id := range seeds {
		queue = append(queue, id)
		affected[id] = true
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, from := range incoming[cur] {
			if affected[from] {
				continue
			}
			affected[from] = true
			queue = append(queue, from)
		}
		if view, ok := m.deploymentByID(cur); ok {
			target := m.Get(view.Target)
			if target != nil && target.Kind == "Service" {
				for _, other := range m.OfKind("Service") {
					for _, dep := range entity.SpecRefs(other.Spec, "dependsOn") {
						if entity.ResolveRef(other, dep) == view.Target {
							for _, depView := range m.Deployments() {
								if depView.Target == entity.IDOf(other) && depView.Environment == view.Environment && !affected[depView.ID] {
									affected[depView.ID] = true
									queue = append(queue, depView.ID)
								}
							}
						}
					}
				}
			}
		}
	}

	flowHits := map[string]bool{}
	for _, flow := range m.OfKind("DataFlow") {
		hit := false
		for _, field := range []string{"source", "target", "actor"} {
			ref, ok := specRef(flow.Spec, field)
			if !ok {
				continue
			}
			id := entity.ResolveRef(flow, ref)
			if id == subject || affected[id] || seeds[id] {
				hit = true
			}
			if e := m.Get(id); e != nil && (e.Kind == "Service" || e.Kind == "Infrastructure") && id == subject {
				hit = true
			}
		}
		if hit {
			flowHits[string(entity.IDOf(flow))] = true
		}
	}

	type group struct {
		department  string
		deployments map[string]bool
		flows       map[string]bool
	}
	groups := map[string]*group{}
	add := func(project, department, deployment, flow string) {
		if project == "" {
			project = "unassigned"
		}
		g := groups[project]
		if g == nil {
			g = &group{department: department, deployments: map[string]bool{}, flows: map[string]bool{}}
			groups[project] = g
		}
		if deployment != "" {
			g.deployments[deployment] = true
		}
		if flow != "" {
			g.flows[flow] = true
		}
	}
	for id := range affected {
		view, ok := m.deploymentByID(id)
		if !ok || id == subject {
			continue
		}
		add(view.ProjectName, view.DepartmentName, string(id), "")
	}
	for flowID := range flowHits {
		flow := m.Get(entity.CanonicalRef(flowID))
		project, department := "", ""
		if ref, ok := specRef(flow.Spec, "actor"); ok {
			if view, ok := m.deploymentByID(entity.ResolveRef(flow, ref)); ok {
				project, department = view.ProjectName, view.DepartmentName
			}
		}
		add(project, department, "", flowID)
	}
	result := BlastRadius{Subject: subject}
	var names []string
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		g := groups[name]
		result.Groups = append(result.Groups, BlastGroup{
			Project: name, Department: g.department,
			Deployments: keys(g.deployments), DataFlows: keys(g.flows),
		})
	}
	if result.Groups == nil {
		result.Groups = []BlastGroup{}
	}
	return result
}
