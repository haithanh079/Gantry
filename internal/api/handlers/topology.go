package handlers

import (
	"net/http"
	"sync"
	"time"

	"github.com/go2engle/gantry/internal/entity"
)

// TopologyNode represents an entity in the topology view.
type TopologyNode struct {
	ID          string         `json:"id"` // canonical "Kind/namespace/name"
	Kind        string         `json:"kind"`
	Name        string         `json:"name"`
	Namespace   string         `json:"namespace,omitempty"`
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description,omitempty"`
	Owner       string         `json:"owner,omitempty"`
	Spec        map[string]any `json:"spec,omitempty"`
	Children    []TopologyNode `json:"children,omitempty"` // related entities nested under this node
}

// TopologyEdge represents a relationship in the topology view.
type TopologyEdge struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Relation string `json:"relation"` // "dependsOn", "deployedIn", "providesApi", "consumesApi", "ownedBy"
}

// TopologyEnvironment is an environment with aggregated entity counts.
type TopologyEnvironment struct {
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Provider    string `json:"provider,omitempty"`
	Region      string `json:"region,omitempty"`
	Type        string `json:"type,omitempty"` // staging, production, development
	EntityCount int    `json:"entityCount"`
}

// TopologyData is the full topology response.
type TopologyData struct {
	Environments []TopologyEnvironment `json:"environments"`
	Nodes        []TopologyNode        `json:"nodes"`
	Edges        []TopologyEdge        `json:"edges"`
}

// In-memory cache for topology data.
var (
	topoCacheMu   sync.RWMutex
	topoCacheData *TopologyData
	topoCacheTime time.Time
)

// GetTopologyData handles GET /api/v1/plugins/topology-explorer/data.
// Returns all environments and all entities with their relationships.
// Optional query param ?environment=name filters to a single environment.
func (h *Handlers) GetTopologyData(w http.ResponseWriter, r *http.Request) {
	p, err := h.DB.GetPlugin(r.Context(), "topology-explorer")
	if err != nil || p == nil {
		writeError(w, http.StatusNotFound, "topology-explorer plugin not installed")
		return
	}
	if !p.Enabled {
		writeError(w, http.StatusBadRequest, "topology-explorer plugin is not enabled")
		return
	}

	envFilter := r.URL.Query().Get("environment")

	// Check cache (30s TTL) only if no filter is applied.
	if envFilter == "" {
		topoCacheMu.RLock()
		if time.Since(topoCacheTime) < 30*time.Second && topoCacheData != nil {
			data := topoCacheData
			topoCacheMu.RUnlock()
			writeJSON(w, http.StatusOK, data)
			return
		}
		topoCacheMu.RUnlock()
	}

	// Fetch all entities.
	all, err := h.DB.ListEntities(r.Context(), "", "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list entities")
		return
	}

	envMap := make(map[string]*TopologyEnvironment)
	var nodes []TopologyNode
	var edges []TopologyEdge

	// First pass: collect Environment entities.
	for _, e := range all {
		if e.Kind != "Environment" {
			continue
		}
		spec := e.Spec
		if spec == nil {
			spec = map[string]any{}
		}
		provider, _ := spec["provider"].(string)
		region, _ := spec["region"].(string)
		envType, _ := spec["type"].(string)
		envMap[e.Metadata.Name] = &TopologyEnvironment{
			Name:        e.Metadata.Name,
			Title:       e.Metadata.Title,
			Description: e.Metadata.Description,
			Provider:    provider,
			Region:      region,
			Type:        envType,
		}
	}

	// If filtering by environment and it doesn't exist, return empty result.
	if envFilter != "" {
		if _, ok := envMap[envFilter]; !ok {
			writeJSON(w, http.StatusOK, TopologyData{
				Environments: []TopologyEnvironment{},
				Nodes:        []TopologyNode{},
				Edges:        []TopologyEdge{},
			})
			return
		}
	}

	// childNodes collects entities whose first dependsOn target is another
	// entity — they will be nested under that parent node.
	type childEntry struct {
		node         TopologyNode
		deployedEnvs []string // from spec.deployedIn
		parent       string   // full node ID (Kind/name) this depends on
	}
	var childEntities []childEntry

	// Second pass: collect all non-Environment entities and build edges.
	for _, e := range all {
		if e.Kind == "Flow" {
			continue
		}

		nodeID := string(entity.IDOf(e))
		spec := e.Spec
		if spec == nil {
			spec = map[string]any{}
		}

		// For Environment entities, add as node but skip relationship extraction for deployment.
		if e.Kind == "Environment" {
			if envFilter == "" || e.Metadata.Name == envFilter {
				nodes = append(nodes, TopologyNode{
					ID:          nodeID,
					Kind:        e.Kind,
					Name:        e.Metadata.Name,
					Namespace:   e.Metadata.Namespace,
					Title:       e.Metadata.Title,
					Description: e.Metadata.Description,
					Owner:       e.Metadata.Owner,
				})
			}
			extractDependsOn(e, nodeID, &edges)
			continue
		}

		// Determine which environments this entity is deployed in.
		deployedEnvs := extractDeployedIn(e)

		// Entities that dependOn another entity are nested as children of that
		// parent rather than appearing as top-level items.
		parentID := findFirstDependency(e)
		if parentID != "" {
			childEntities = append(childEntities, childEntry{
				node: TopologyNode{
					ID:    nodeID,
					Kind:  e.Kind,
					Name:  e.Metadata.Name,
					Title: e.Metadata.Title,
					Owner: e.Metadata.Owner,
				},
				deployedEnvs: deployedEnvs,
				parent:       parentID,
			})
			// Emit edges for this entity.
			for _, envRef := range deployedEnvs {
				edges = append(edges, TopologyEdge{
					From:     nodeID,
					To:       envRef,
					Relation: "deployedIn",
				})
			}
			extractDependsOn(e, nodeID, &edges)
			continue
		}

		// If filtering by environment, skip entities not deployed there.
		if envFilter != "" {
			found := false
			for _, env := range deployedEnvs {
				parsed, err := entity.ParseCanonicalRef(env)
				if err != nil {
					continue
				}
				if parsed.Name == envFilter || env == envFilter {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		nodes = append(nodes, TopologyNode{
			ID:          nodeID,
			Kind:        e.Kind,
			Name:        e.Metadata.Name,
			Namespace:   e.Metadata.Namespace,
			Title:       e.Metadata.Title,
			Description: e.Metadata.Description,
			Owner:       e.Metadata.Owner,
		})

		// Build edges: deployedIn.
		// When filtering by environment, only emit edges (and count entities)
		// for the filtered environment to keep the response self-contained.
		for _, envRef := range deployedEnvs {
			parsed, err := entity.ParseCanonicalRef(envRef)
			if err != nil {
				continue
			}
			if envFilter != "" && parsed.Name != envFilter && envRef != envFilter {
				continue
			}
			if env, ok := envMap[parsed.Name]; ok {
				env.EntityCount++
			}
			edges = append(edges, TopologyEdge{
				From:     nodeID,
				To:       envRef,
				Relation: "deployedIn",
			})
		}

		// Build edges: dependsOn.
		extractDependsOn(e, nodeID, &edges)

		// Build edges: providesApis.
		if raw, ok := spec["providesApis"]; ok {
			if apis, ok := raw.([]any); ok {
				for _, a := range apis {
					apiName, _ := a.(string)
					if apiName != "" {
						to := string(entity.ResolveRef(e, entity.Ref{Kind: "API", Name: apiName}))
						edges = append(edges, TopologyEdge{From: nodeID, To: to, Relation: "providesApi"})
					}
				}
			}
		}

		// Build edges: consumesApis.
		if raw, ok := spec["consumesApis"]; ok {
			if apis, ok := raw.([]any); ok {
				for _, a := range apis {
					apiName, _ := a.(string)
					if apiName != "" {
						to := string(entity.ResolveRef(e, entity.Ref{Kind: "API", Name: apiName}))
						edges = append(edges, TopologyEdge{From: nodeID, To: to, Relation: "consumesApi"})
					}
				}
			}
		}

		// Build edges: ownedBy.
		if e.Metadata.Owner != "" {
			to := string(entity.ResolveRef(e, entity.Ref{Kind: "Team", Name: e.Metadata.Owner}))
			edges = append(edges, TopologyEdge{From: nodeID, To: to, Relation: "ownedBy"})
		}
	}

	// Attach child entities to their parent nodes.
	nodeIdx := make(map[string][]int) // nodeID → indices in nodes[]
	for i, n := range nodes {
		nodeIdx[n.ID] = append(nodeIdx[n.ID], i)
	}
	for _, c := range childEntities {
		idxList, ok := nodeIdx[c.parent]
		if !ok {
			continue
		}
		for _, idx := range idxList {
			nodes[idx].Children = append(nodes[idx].Children, c.node)
		}
	}

	// Build environment list.
	var environments []TopologyEnvironment
	if envFilter != "" {
		if env, ok := envMap[envFilter]; ok {
			environments = append(environments, *env)
		}
	} else {
		for _, env := range envMap {
			environments = append(environments, *env)
		}
	}

	if nodes == nil {
		nodes = []TopologyNode{}
	}
	if edges == nil {
		edges = []TopologyEdge{}
	}
	if environments == nil {
		environments = []TopologyEnvironment{}
	}

	result := &TopologyData{
		Environments: environments,
		Nodes:        nodes,
		Edges:        edges,
	}

	// Cache the unfiltered result.
	if envFilter == "" {
		topoCacheMu.Lock()
		topoCacheData = result
		topoCacheTime = time.Now()
		topoCacheMu.Unlock()
	}

	writeJSON(w, http.StatusOK, result)
}

// GetTopologyStatus handles GET /api/v1/plugins/topology-explorer/status.
// Aggregates health status from status-monitor plugin for entity overlay.
func (h *Handlers) GetTopologyStatus(w http.ResponseWriter, r *http.Request) {
	p, err := h.DB.GetPlugin(r.Context(), "topology-explorer")
	if err != nil || p == nil {
		writeError(w, http.StatusNotFound, "topology-explorer plugin not installed")
		return
	}
	if !p.Enabled {
		writeError(w, http.StatusBadRequest, "topology-explorer plugin is not enabled")
		return
	}

	// Check if status-monitor is enabled; if so, return its data.
	sm, err := h.DB.GetPlugin(r.Context(), "status-monitor")
	if err != nil || sm == nil || !sm.Enabled {
		writeJSON(w, http.StatusOK, map[string]any{})
		return
	}

	// Return cached status-monitor results keyed by provider name.
	// If the cache has not been populated yet, fail explicitly instead of
	// silently returning an empty result set that looks authoritative.
	smCacheMu.RLock()
	results := append([]StatusMonitorResult(nil), smCacheResults...)
	smCacheMu.RUnlock()

	if len(results) == 0 {
		writeError(w, http.StatusServiceUnavailable, "status-monitor data is not available yet")
		return
	}

	statusMap := make(map[string]map[string]string, len(results))
	for _, r := range results {
		statusMap[r.Name] = map[string]string{
			"status":      r.Status,
			"description": r.Description,
		}
	}

	writeJSON(w, http.StatusOK, statusMap)
}

// extractDeployedIn reads spec.deployedIn and returns canonical Environment refs.
func extractDeployedIn(e *entity.Entity) []string {
	var refs []string
	for _, ref := range entity.SpecRefs(e.Spec, "deployedIn") {
		if ref.Kind != "" && ref.Kind != "Environment" {
			continue
		}
		if ref.Kind == "" {
			ref.Kind = "Environment"
		}
		refs = append(refs, string(entity.ResolveRef(e, ref)))
	}
	return refs
}

// extractDependsOn reads spec.dependsOn and appends edges.
func extractDependsOn(e *entity.Entity, fromID string, edges *[]TopologyEdge) {
	for _, ref := range entity.SpecRefs(e.Spec, "dependsOn") {
		*edges = append(*edges, TopologyEdge{
			From:     fromID,
			To:       string(entity.ResolveRef(e, ref)),
			Relation: "dependsOn",
		})
	}
}

// findFirstDependency returns the canonical id of the first non-Environment
// entity listed in spec.dependsOn, so dependent entities nest under that parent.
func findFirstDependency(e *entity.Entity) string {
	for _, ref := range entity.SpecRefs(e.Spec, "dependsOn") {
		if ref.Kind != "" && ref.Kind != "Environment" {
			return string(entity.ResolveRef(e, ref))
		}
	}
	return ""
}
