package landscape

import (
	"fmt"
	"sort"
	"strings"

	"github.com/go2engle/gantry/internal/entity"
)

// GraphQuery selects the landscape map.
type GraphQuery struct {
	GroupBy    string
	Level      string
	Layers     map[string]bool
	Env        string
	Cloud      string
	Region     string
	Runtime    string
	Team       string
	Project    string
	Department string
	Lifecycle  string
}

// Node is a map node. At company and project levels it is an aggregate.
type Node struct {
	ID        string         `json:"id"`
	Kind      string         `json:"kind"`
	Name      string         `json:"name"`
	Namespace string         `json:"namespace,omitempty"`
	Title     string         `json:"title,omitempty"`
	Cluster   string         `json:"cluster,omitempty"`
	ActorOf   []string       `json:"actorOf,omitempty"`
	Meta      map[string]any `json:"meta,omitempty"`
}

// Edge is a network or data-flow edge. Aggregated edges set Count.
type Edge struct {
	ID          string   `json:"id"`
	From        string   `json:"from"`
	To          string   `json:"to"`
	Layer       string   `json:"layer"`
	Relation    string   `json:"relation,omitempty"`
	Count       int      `json:"count,omitempty"`
	Protocol    string   `json:"protocol,omitempty"`
	Port        int      `json:"port,omitempty"`
	Inferred    bool     `json:"inferred,omitempty"`
	DataFlowRef string   `json:"dataFlowRef,omitempty"`
	ActorRef    string   `json:"actorRef,omitempty"`
	Mode        string   `json:"mode,omitempty"`
	Transport   string   `json:"transport,omitempty"`
	DataClasses []string `json:"dataClasses,omitempty"`
}

// Cluster groups nodes for the current groupBy.
type Cluster struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// HighlightMatch is a highlight clipped to the current graph.
type HighlightMatch struct {
	Ref       string   `json:"ref"`
	Name      string   `json:"name"`
	Color     string   `json:"color"`
	DefaultOn bool     `json:"defaultOn"`
	NodeIDs   []string `json:"nodeIds"`
	EdgeIDs   []string `json:"edgeIds"`
}

// GraphResult is the landscape graph payload.
type GraphResult struct {
	Clusters   []Cluster        `json:"clusters"`
	Nodes      []Node           `json:"nodes"`
	Edges      []Edge           `json:"edges"`
	Counts     map[string]int   `json:"counts"`
	Highlights []HighlightMatch `json:"highlights"`
}

// BuildGraph returns the map for the query, including highlight matches.
func (m *Model) BuildGraph(q GraphQuery) GraphResult {
	if q.Level == "" {
		q.Level = "deployment"
	}
	if q.GroupBy == "" {
		q.GroupBy = "org"
	}
	if len(q.Layers) == 0 {
		q.Layers = map[string]bool{"network": true, "data": true}
	}
	views := m.visibleDeployments(q)
	visible := map[entity.CanonicalRef]DeploymentView{}
	for _, view := range views {
		visible[view.ID] = view
	}

	var nodes []Node
	var edges []Edge
	switch q.Level {
	case "company":
		nodes, edges = m.aggregate(q, views, visible, func(v DeploymentView) (string, string, string) {
			id := string(v.Department)
			if id == "" {
				id = "department/" + v.DepartmentName
			}
			return id, "Department", v.DepartmentName
		})
	case "project":
		nodes, edges = m.aggregate(q, views, visible, func(v DeploymentView) (string, string, string) {
			id := string(v.Project)
			name := v.ProjectName
			if id == "" {
				id = "project/unassigned"
				name = "unassigned"
			}
			return id, "Project", name
		})
	default:
		nodes, edges = m.deploymentGraph(q, views, visible)
	}
	if nodes == nil {
		nodes = []Node{}
	}
	if edges == nil {
		edges = []Edge{}
	}
	result := GraphResult{
		Clusters:   clustersOf(nodes),
		Nodes:      nodes,
		Edges:      edges,
		Counts:     map[string]int{"nodes": len(nodes), "edges": len(edges), "deployments": len(views)},
		Highlights: m.MatchHighlights(nodes, edges),
	}
	if result.Highlights == nil {
		result.Highlights = []HighlightMatch{}
	}
	if result.Clusters == nil {
		result.Clusters = []Cluster{}
	}
	return result
}

func (m *Model) visibleDeployments(q GraphQuery) []DeploymentView {
	var out []DeploymentView
	for _, view := range m.Deployments() {
		if q.Env != "" && view.EnvName != q.Env && string(view.Environment) != q.Env {
			continue
		}
		if q.Cloud != "" && !strings.EqualFold(view.Cloud, q.Cloud) {
			continue
		}
		if q.Region != "" && !containsFold(view.Regions, q.Region) {
			continue
		}
		if q.Runtime != "" && view.Runtime != q.Runtime {
			continue
		}
		if q.Team != "" && view.Owner != q.Team {
			continue
		}
		if q.Project != "" && view.ProjectName != q.Project && string(view.Project) != q.Project {
			continue
		}
		if q.Department != "" && view.DepartmentName != q.Department && string(view.Department) != q.Department && view.Entity.Metadata.Namespace != q.Department {
			continue
		}
		if q.Lifecycle != "" && view.Lifecycle != q.Lifecycle {
			continue
		}
		out = append(out, view)
	}
	return out
}

func (m *Model) deploymentGraph(q GraphQuery, views []DeploymentView, visible map[entity.CanonicalRef]DeploymentView) ([]Node, []Edge) {
	nodes := map[string]Node{}
	actorOf := map[string][]string{}
	var edges []Edge
	for _, view := range views {
		nodes[string(view.ID)] = deploymentNode(q, view)
	}
	if q.Layers["network"] {
		for _, conn := range m.Connections() {
			if _, ok := visible[conn.From]; !ok {
				continue
			}
			if _, ok := visible[conn.To]; !ok {
				continue
			}
			id := fmt.Sprintf("network:%s->%s", conn.From, conn.To)
			edges = append(edges, Edge{
				ID: id, From: string(conn.From), To: string(conn.To), Layer: "network",
				Relation: "connectsTo", Count: 1, Protocol: conn.Protocol, Port: conn.Port, Inferred: conn.Inferred,
			})
		}
	}
	if q.Layers["data"] {
		for _, flow := range m.OfKind("DataFlow") {
			if !m.dataFlowVisible(flow, visible, q) {
				continue
			}
			from := m.endpointNode(flow, "source", nodes, q, visible)
			to := m.endpointNode(flow, "target", nodes, q, visible)
			if from == "" || to == "" {
				continue
			}
			flowID := string(entity.IDOf(flow))
			edge := Edge{
				ID: "data:" + flowID, From: from, To: to, Layer: "data", Relation: "dataFlow", Count: 1,
				DataFlowRef: flowID, Mode: specString(flow.Spec, "mode"), Transport: specString(flow.Spec, "transport"),
				DataClasses: specStrings(flow.Spec, "dataClasses"),
			}
			if ref, ok := specRef(flow.Spec, "actor"); ok {
				actor := entity.ResolveRef(flow, ref)
				edge.ActorRef = string(actor)
				actorOf[string(actor)] = append(actorOf[string(actor)], flowID)
			}
			edges = append(edges, edge)
		}
	}
	var nodeList []Node
	for id, node := range nodes {
		node.ActorOf = actorOf[id]
		nodeList = append(nodeList, node)
	}
	sort.Slice(nodeList, func(i, j int) bool { return nodeList[i].ID < nodeList[j].ID })
	return nodeList, edges
}

func (m *Model) dataFlowVisible(flow *entity.Entity, visible map[entity.CanonicalRef]DeploymentView, q GraphQuery) bool {
	if ref, ok := specRef(flow.Spec, "actor"); ok {
		actor := entity.ResolveRef(flow, ref)
		if _, ok := visible[actor]; ok {
			return true
		}
		if m.Get(actor) != nil && len(visible) > 0 && (q.Env != "" || q.Cloud != "" || q.Project != "" || q.Department != "" || q.Team != "" || q.Lifecycle != "" || q.Runtime != "" || q.Region != "") {
			return false
		}
	}
	for _, field := range []string{"source", "target"} {
		ref, ok := specRef(flow.Spec, field)
		if !ok {
			continue
		}
		id := entity.ResolveRef(flow, ref)
		if _, ok := visible[id]; ok {
			return true
		}
		if ref.Kind == "Infrastructure" && m.Get(id) != nil && q.Env == "" && q.Cloud == "" {
			return true
		}
	}
	return false
}

func (m *Model) endpointNode(flow *entity.Entity, field string, nodes map[string]Node, q GraphQuery, visible map[entity.CanonicalRef]DeploymentView) string {
	ref, ok := specRef(flow.Spec, field)
	if !ok {
		return ""
	}
	id := entity.ResolveRef(flow, ref)
	if view, ok := visible[id]; ok {
		return string(view.ID)
	}
	if _, exists := nodes[string(id)]; exists {
		return string(id)
	}
	target := m.Get(id)
	if target == nil {
		return string(id)
	}
	if target.Kind == "Deployment" {
		return ""
	}
	nodes[string(id)] = Node{
		ID: string(id), Kind: target.Kind, Name: target.Metadata.Name, Namespace: target.Metadata.Namespace,
		Title: target.Metadata.Title, Cluster: clusterID(q, DeploymentView{Cloud: specString(target.Spec, "provider"), DepartmentName: target.Metadata.Namespace}),
		Meta: map[string]any{"endpoint": true},
	}
	return string(id)
}

type groupKey func(DeploymentView) (id, kind, name string)

func (m *Model) aggregate(q GraphQuery, views []DeploymentView, visible map[entity.CanonicalRef]DeploymentView, key groupKey) ([]Node, []Edge) {
	type acc struct {
		node  Node
		count int
	}
	groups := map[string]*acc{}
	memberGroup := map[entity.CanonicalRef]string{}
	for _, view := range views {
		id, kind, name := key(view)
		memberGroup[view.ID] = id
		g := groups[id]
		if g == nil {
			g = &acc{node: Node{ID: id, Kind: kind, Name: name, Cluster: clusterID(q, view), Meta: map[string]any{}}}
			groups[id] = g
		}
		g.count++
	}
	edgeAcc := map[string]*Edge{}
	add := func(from, to, layer string, extra Edge) {
		if from == "" || to == "" {
			return
		}
		id := layer + ":" + from + "->" + to
		if from == to {
			id = layer + ":" + from + ":self"
		}
		if existing := edgeAcc[id]; existing != nil {
			existing.Count++
			return
		}
		extra.ID = id
		extra.From = from
		extra.To = to
		extra.Layer = layer
		extra.Count = 1
		edgeAcc[id] = &extra
	}
	if q.Layers["network"] {
		for _, conn := range m.Connections() {
			if _, ok := visible[conn.From]; !ok {
				continue
			}
			if _, ok := visible[conn.To]; !ok {
				continue
			}
			add(memberGroup[conn.From], memberGroup[conn.To], "network", Edge{Relation: "connectsTo"})
		}
	}
	if q.Layers["data"] {
		for _, flow := range m.OfKind("DataFlow") {
			if !m.dataFlowVisible(flow, visible, q) {
				continue
			}
			from := endpointGroup(m, flow, "source", memberGroup)
			to := endpointGroup(m, flow, "target", memberGroup)
			add(from, to, "data", Edge{
				Relation: "dataFlow", DataFlowRef: string(entity.IDOf(flow)),
				Mode: specString(flow.Spec, "mode"), Transport: specString(flow.Spec, "transport"),
				DataClasses: specStrings(flow.Spec, "dataClasses"),
			})
		}
	}
	var nodes []Node
	for _, g := range groups {
		g.node.Meta["deploymentCount"] = g.count
		nodes = append(nodes, g.node)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	var edges []Edge
	for _, edge := range edgeAcc {
		edges = append(edges, *edge)
	}
	sort.Slice(edges, func(i, j int) bool { return edges[i].ID < edges[j].ID })
	return nodes, edges
}

func endpointGroup(m *Model, flow *entity.Entity, field string, memberGroup map[entity.CanonicalRef]string) string {
	ref, ok := specRef(flow.Spec, field)
	if !ok {
		return ""
	}
	id := entity.ResolveRef(flow, ref)
	if g := memberGroup[id]; g != "" {
		return g
	}
	if view, ok := m.deploymentByID(id); ok {
		return memberGroup[view.ID]
	}
	target := m.Get(id)
	if target != nil && target.Kind == "Infrastructure" {
		for _, view := range m.Deployments() {
			if view.Target == id {
				if g := memberGroup[view.ID]; g != "" {
					return g
				}
			}
		}
	}
	return ""
}

func deploymentNode(q GraphQuery, view DeploymentView) Node {
	return Node{
		ID: string(view.ID), Kind: "Deployment", Name: view.Entity.Metadata.Name, Namespace: view.Entity.Metadata.Namespace,
		Title: view.Entity.Metadata.Title, Cluster: clusterID(q, view),
		Meta: map[string]any{
			"environment": view.EnvName, "cloud": view.Cloud, "runtime": view.Runtime,
			"regions": view.Regions, "project": view.ProjectName, "department": view.DepartmentName,
			"owner": view.Owner, "lifecycle": view.Lifecycle,
		},
	}
}

func clusterID(q GraphQuery, view DeploymentView) string {
	switch q.GroupBy {
	case "cloud":
		if view.Cloud == "" {
			return "cloud:unknown"
		}
		return "cloud:" + strings.ToLower(view.Cloud)
	case "env":
		if view.EnvName == "" {
			return "env:unknown"
		}
		return "env:" + view.EnvName
	default:
		if view.DepartmentName == "" {
			return "org:unassigned"
		}
		return "org:" + view.DepartmentName
	}
}

func clustersOf(nodes []Node) []Cluster {
	seen := map[string]bool{}
	var out []Cluster
	for _, node := range nodes {
		if node.Cluster == "" || seen[node.Cluster] {
			continue
		}
		seen[node.Cluster] = true
		out = append(out, Cluster{ID: node.Cluster, Label: node.Cluster})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
