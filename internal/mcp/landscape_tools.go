package mcp

import (
	"context"
	"fmt"

	"github.com/go2engle/gantry/internal/api/handlers"
	"github.com/go2engle/gantry/internal/api/middleware"
	"github.com/go2engle/gantry/internal/db"
	"github.com/go2engle/gantry/internal/entity"
	"github.com/go2engle/gantry/internal/landscape"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type landscapeQueryInput struct {
	GroupBy    string `json:"groupBy,omitempty" jsonschema:"org, cloud, or env"`
	Level      string `json:"level,omitempty" jsonschema:"company, project, or deployment"`
	Layers     string `json:"layers,omitempty" jsonschema:"comma-separated network,data"`
	Env        string `json:"env,omitempty"`
	Cloud      string `json:"cloud,omitempty"`
	Region     string `json:"region,omitempty"`
	Runtime    string `json:"runtime,omitempty"`
	Team       string `json:"team,omitempty"`
	Project    string `json:"project,omitempty"`
	Department string `json:"department,omitempty"`
	Lifecycle  string `json:"lifecycle,omitempty"`
}

type blastInput struct {
	Kind      string `json:"kind" jsonschema:"entity kind, usually Deployment, Service, or Infrastructure"`
	Namespace string `json:"namespace,omitempty" jsonschema:"defaults to default"`
	Name      string `json:"name" jsonschema:"entity name"`
}

type findingsInput struct {
	Category string `json:"category,omitempty"`
	Severity string `json:"severity,omitempty"`
	Rule     string `json:"rule,omitempty"`
	Status   string `json:"status,omitempty" jsonschema:"open or resolved"`
}

type migrateLandscapeInput struct {
	Apply bool `json:"apply,omitempty" jsonschema:"when true, write the plan; otherwise return a dry run"`
}

type markRefInput struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

type markEdgeInput struct {
	From markRefInput `json:"from"`
	To   markRefInput `json:"to"`
}

type markEntitiesInput struct {
	Highlight   string          `json:"highlight" jsonschema:"Highlight entity name; created if it does not exist"`
	Namespace   string          `json:"namespace,omitempty"`
	Color       string          `json:"color,omitempty" jsonschema:"mark-1 through mark-6"`
	Add         []markRefInput  `json:"add,omitempty"`
	Remove      []markRefInput  `json:"remove,omitempty"`
	AddEdges    []markEdgeInput `json:"addEdges,omitempty"`
	RemoveEdges []markEdgeInput `json:"removeEdges,omitempty"`
}

func registerLandscapeTools(srv *mcpsdk.Server, h *handlers.Handlers) {
	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "get_landscape_inventory",
		Description: "Service by environment inventory for the landscape, grouped by project. Requires landscape:read.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in landscapeQueryInput) (*mcpsdk.CallToolResult, any, error) {
		return landscapeInventoryTool(ctx, h, in)
	})
	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "get_landscape_dataflows",
		Description: "Data flows visible under the same filters as the landscape map. Requires landscape:read.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in landscapeQueryInput) (*mcpsdk.CallToolResult, any, error) {
		return landscapeDataFlowsTool(ctx, h, in)
	})
	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "get_blast_radius",
		Description: "Catalog blast radius of one entity: reverse connections, dependencies, and data flows. Requires landscape:read.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in blastInput) (*mcpsdk.CallToolResult, any, error) {
		return blastRadiusTool(ctx, h, in)
	})
	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "list_findings",
		Description: "Open or resolved landscape findings. Requires landscape:read.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in findingsInput) (*mcpsdk.CallToolResult, any, error) {
		return listFindingsTool(ctx, h, in)
	})
	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "mark_entities",
		Description: "Add or remove catalog entities and edges on a Highlight. Creates the Highlight if needed. Requires entity:write and developer role. Validation, audit, and GitOps follow the normal entity write path.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in markEntitiesInput) (*mcpsdk.CallToolResult, any, error) {
		return markEntitiesTool(ctx, h, in)
	})
	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "migrate_landscape",
		Description: "Plan or apply Deployment entities from deployedIn. Dry-run unless apply is true. Requires developer role.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in migrateLandscapeInput) (*mcpsdk.CallToolResult, any, error) {
		return migrateLandscapeTool(ctx, h, in)
	})
}

func landscapeQueryFrom(in landscapeQueryInput) landscape.GraphQuery {
	layers := map[string]bool{}
	if in.Layers != "" {
		for _, part := range splitComma(in.Layers) {
			layers[part] = true
		}
	}
	return landscape.GraphQuery{
		GroupBy: in.GroupBy, Level: in.Level, Layers: layers,
		Env: in.Env, Cloud: in.Cloud, Region: in.Region, Runtime: in.Runtime,
		Team: in.Team, Project: in.Project, Department: in.Department, Lifecycle: in.Lifecycle,
	}
}

func splitComma(raw string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(raw); i++ {
		if i == len(raw) || raw[i] == ',' {
			part := trimSpace(raw[start:i])
			if part != "" {
				out = append(out, part)
			}
			start = i + 1
		}
	}
	return out
}

func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}

func requireLandscapeRead(ctx context.Context, h *handlers.Handlers) error {
	if !h.CanAccess(ctx, "landscape", "*", "read") {
		return fmt.Errorf("insufficient permissions: landscape:read")
	}
	return nil
}

func loadReadableModel(ctx context.Context, h *handlers.Handlers) (*landscape.Model, error) {
	return landscape.LoadModel(ctx, h.DB, func(entities []*entity.Entity) []*entity.Entity {
		return h.FilterEntitiesByAccess(ctx, entities)
	})
}

func landscapeInventoryTool(ctx context.Context, h *handlers.Handlers, in landscapeQueryInput) (*mcpsdk.CallToolResult, any, error) {
	if err := requireLandscapeRead(ctx, h); err != nil {
		return nil, nil, err
	}
	model, err := loadReadableModel(ctx, h)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(model.Inventory(landscapeQueryFrom(in)))
}

func landscapeDataFlowsTool(ctx context.Context, h *handlers.Handlers, in landscapeQueryInput) (*mcpsdk.CallToolResult, any, error) {
	if err := requireLandscapeRead(ctx, h); err != nil {
		return nil, nil, err
	}
	model, err := loadReadableModel(ctx, h)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"dataflows": model.DataFlows(landscapeQueryFrom(in))})
}

func blastRadiusTool(ctx context.Context, h *handlers.Handlers, in blastInput) (*mcpsdk.CallToolResult, any, error) {
	if err := requireLandscapeRead(ctx, h); err != nil {
		return nil, nil, err
	}
	ns := in.Namespace
	if ns == "" {
		ns = entity.DefaultNamespace
	}
	model, err := loadReadableModel(ctx, h)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(model.BlastRadius(entity.CanonicalRef(in.Kind + "/" + ns + "/" + in.Name)))
}

func listFindingsTool(ctx context.Context, h *handlers.Handlers, in findingsInput) (*mcpsdk.CallToolResult, any, error) {
	if err := requireLandscapeRead(ctx, h); err != nil {
		return nil, nil, err
	}
	rows, err := h.DB.ListLandscapeFindings(ctx, db.LandscapeFindingFilter{
		Category: in.Category, Severity: in.Severity, Rule: in.Rule, Status: in.Status,
	})
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"findings": rows})
}

func markEntitiesTool(ctx context.Context, h *handlers.Handlers, in markEntitiesInput) (*mcpsdk.CallToolResult, any, error) {
	if err := requireDeveloperRole(ctx); err != nil {
		return nil, nil, err
	}
	if !h.CanAccess(ctx, "entity", "Highlight", "write") {
		return nil, nil, fmt.Errorf("insufficient permissions to write Highlight entities")
	}
	claims := middleware.GetClaims(ctx)
	userID, userName := "", ""
	if claims != nil {
		userID, userName = claims.UserID, claims.Username
	}
	entityOut, created, err := h.MarkLandscapeEntities(ctx, handlers.MarkRequest{
		Highlight: in.Highlight, Namespace: in.Namespace, Color: in.Color,
		Add: toMarkRefs(in.Add), Remove: toMarkRefs(in.Remove),
		AddEdges: toMarkEdges(in.AddEdges), RemoveEdges: toMarkEdges(in.RemoveEdges),
	}, userID, userName)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(map[string]any{"entity": entityOut, "created": created})
}

func migrateLandscapeTool(ctx context.Context, h *handlers.Handlers, in migrateLandscapeInput) (*mcpsdk.CallToolResult, any, error) {
	if err := requireDeveloperRole(ctx); err != nil {
		return nil, nil, err
	}
	result, err := h.ApplyLandscapeMigration(ctx, in.Apply)
	if err != nil {
		return nil, nil, err
	}
	return jsonResult(result)
}

func toMarkRefs(in []markRefInput) []handlers.MarkRef {
	out := make([]handlers.MarkRef, len(in))
	for i, ref := range in {
		out[i] = handlers.MarkRef{Kind: ref.Kind, Name: ref.Name, Namespace: ref.Namespace}
	}
	return out
}

func toMarkEdges(in []markEdgeInput) []handlers.MarkEdge {
	out := make([]handlers.MarkEdge, len(in))
	for i, edge := range in {
		out[i] = handlers.MarkEdge{
			From: handlers.MarkRef{Kind: edge.From.Kind, Name: edge.From.Name, Namespace: edge.From.Namespace},
			To:   handlers.MarkRef{Kind: edge.To.Kind, Name: edge.To.Name, Namespace: edge.To.Namespace},
		}
	}
	return out
}
