package landscape

import (
	"sort"

	"github.com/go2engle/gantry/internal/entity"
)

// MatchHighlights returns highlights that touch at least one node or edge in scope.
// Members the caller cannot see are already absent from the model, so they are not returned.
func (m *Model) MatchHighlights(nodes []Node, edges []Edge) []HighlightMatch {
	nodeSet := map[string]Node{}
	for _, node := range nodes {
		nodeSet[node.ID] = node
	}
	edgeSet := map[string]Edge{}
	for _, edge := range edges {
		edgeSet[edge.ID] = edge
	}
	var out []HighlightMatch
	for _, highlight := range m.OfKind("Highlight") {
		matchedNodes := map[string]bool{}
		matchedEdges := map[string]bool{}
		for _, ref := range entity.SpecRefs(highlight.Spec, "members") {
			id := entity.ResolveRef(highlight, ref)
			if edge, ok := edgeSet["data:"+string(id)]; ok {
				matchedEdges[edge.ID] = true
				matchedNodes[edge.From] = true
				matchedNodes[edge.To] = true
			}
			for _, nodeID := range m.expandMember(id, nodeSet) {
				matchedNodes[nodeID] = true
			}
		}
		for _, item := range specSlice(highlight.Spec, "edges") {
			obj, _ := item.(map[string]any)
			from, okFrom := specRef(obj, "from")
			to, okTo := specRef(obj, "to")
			if !okFrom || !okTo {
				continue
			}
			fromID := string(entity.ResolveRef(highlight, from))
			toID := string(entity.ResolveRef(highlight, to))
			for _, edge := range edges {
				if edge.Layer == "network" && edge.From == fromID && edge.To == toID {
					matchedEdges[edge.ID] = true
				}
			}
		}
		if sel, ok := highlight.Spec["selector"].(map[string]any); ok {
			for _, node := range nodes {
				if selectorMatchesNode(m, sel, node) {
					matchedNodes[node.ID] = true
				}
			}
			for _, edge := range edges {
				if edge.Layer == "data" && selectorMatchesDataEdge(sel, edge) {
					matchedEdges[edge.ID] = true
					matchedNodes[edge.From] = true
					matchedNodes[edge.To] = true
				}
			}
		}
		if specBool(highlight.Spec, "includeNeighbors") || !specHas(highlight.Spec, "includeNeighbors") {
			for _, edge := range edges {
				if matchedNodes[edge.From] && matchedNodes[edge.To] {
					matchedEdges[edge.ID] = true
				}
			}
		}
		if len(matchedNodes) == 0 && len(matchedEdges) == 0 {
			continue
		}
		match := HighlightMatch{
			Ref:       string(entity.IDOf(highlight)),
			Name:      highlight.Metadata.Name,
			Color:     specString(highlight.Spec, "color"),
			DefaultOn: specBool(highlight.Spec, "defaultOn"),
			NodeIDs:   keys(matchedNodes),
			EdgeIDs:   keys(matchedEdges),
		}
		if highlight.Metadata.Title != "" {
			match.Name = highlight.Metadata.Title
		}
		out = append(out, match)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ref < out[j].Ref })
	return out
}

func (m *Model) expandMember(id entity.CanonicalRef, nodes map[string]Node) []string {
	if _, ok := nodes[string(id)]; ok {
		return []string{string(id)}
	}
	target := m.Get(id)
	if target == nil {
		return nil
	}
	var out []string
	switch target.Kind {
	case "Service", "Infrastructure":
		for _, view := range m.Deployments() {
			if view.Target == id {
				if _, ok := nodes[string(view.ID)]; ok {
					out = append(out, string(view.ID))
				}
			}
		}
	case "Project":
		for _, view := range m.Deployments() {
			if view.Project == id {
				if _, ok := nodes[string(view.ID)]; ok {
					out = append(out, string(view.ID))
				}
			}
		}
	case "Department":
		for _, view := range m.Deployments() {
			if view.Department == id || view.DepartmentName == target.Metadata.Name {
				if _, ok := nodes[string(view.ID)]; ok {
					out = append(out, string(view.ID))
				}
			}
		}
	case "DataFlow":
		for _, field := range []string{"source", "target", "actor"} {
			ref, ok := specRef(target.Spec, field)
			if !ok {
				continue
			}
			endpoint := string(entity.ResolveRef(target, ref))
			if _, ok := nodes[endpoint]; ok {
				out = append(out, endpoint)
			}
		}
	}
	return out
}

func selectorMatchesNode(m *Model, sel map[string]any, node Node) bool {
	if !selectorList(sel, "kinds", node.Kind) {
		return false
	}
	env, _ := node.Meta["environment"].(string)
	cloud, _ := node.Meta["cloud"].(string)
	project, _ := node.Meta["project"].(string)
	department, _ := node.Meta["department"].(string)
	if !selectorList(sel, "environments", env) || !selectorList(sel, "clouds", cloud) || !selectorList(sel, "projects", project) || !selectorList(sel, "departments", department) {
		return false
	}
	if labels, ok := sel["labels"].(map[string]any); ok && len(labels) > 0 {
		e := m.Get(entity.CanonicalRef(node.ID))
		if e == nil {
			return false
		}
		for k, v := range labels {
			want, _ := v.(string)
			if e.Metadata.Labels[k] != want {
				return false
			}
		}
	}
	if node.Kind == "DataFlow" {
		return false
	}
	if len(specStrings(sel, "dataClasses")) > 0 || len(specStrings(sel, "transports")) > 0 || len(specStrings(sel, "modes")) > 0 {
		return false
	}
	return selectorConstrains(sel)
}

func selectorMatchesDataEdge(sel map[string]any, edge Edge) bool {
	if !selectorList(sel, "kinds", "DataFlow") {
		return false
	}
	if !selectorOverlap(sel, "dataClasses", edge.DataClasses) || !selectorList(sel, "transports", edge.Transport) || !selectorList(sel, "modes", edge.Mode) {
		return false
	}
	if len(specStrings(sel, "environments")) > 0 || len(specStrings(sel, "clouds")) > 0 || len(specStrings(sel, "projects")) > 0 || len(specStrings(sel, "departments")) > 0 {
		return false
	}
	if labels, ok := sel["labels"].(map[string]any); ok && len(labels) > 0 {
		return false
	}
	return true
}

func selectorConstrains(sel map[string]any) bool {
	for _, field := range []string{"kinds", "environments", "clouds", "projects", "departments", "dataClasses", "transports", "modes"} {
		if len(specStrings(sel, field)) > 0 {
			return true
		}
	}
	if labels, ok := sel["labels"].(map[string]any); ok && len(labels) > 0 {
		return true
	}
	return false
}

func selectorList(sel map[string]any, field, value string) bool {
	list := specStrings(sel, field)
	if len(list) == 0 {
		return true
	}
	return containsFold(list, value)
}

func selectorOverlap(sel map[string]any, field string, values []string) bool {
	list := specStrings(sel, field)
	if len(list) == 0 {
		return true
	}
	for _, value := range values {
		if containsFold(list, value) {
			return true
		}
	}
	return false
}

func specHas(spec map[string]any, field string) bool {
	if spec == nil {
		return false
	}
	_, ok := spec[field]
	return ok
}

func keys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k, ok := range set {
		if ok {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}
