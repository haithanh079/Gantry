package handlers

import (
	"context"
	"errors"
	"fmt"

	"github.com/go2engle/gantry/internal/db"
	"github.com/go2engle/gantry/internal/entity"
	"github.com/go2engle/gantry/internal/events"
	"github.com/go2engle/gantry/internal/gitops"
)

// MarkRef is one catalog ref added to or removed from a Highlight.
type MarkRef struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

// MarkEdge is one highlighted connection.
type MarkEdge struct {
	From MarkRef `json:"from"`
	To   MarkRef `json:"to"`
}

// MarkRequest edits a Highlight through the normal entity write path.
type MarkRequest struct {
	Highlight   string
	Namespace   string
	Color       string
	Add         []MarkRef
	Remove      []MarkRef
	AddEdges    []MarkEdge
	RemoveEdges []MarkEdge
}

// MarkLandscapeEntities creates the Highlight when it is missing and merges members and edges.
func (h *Handlers) MarkLandscapeEntities(ctx context.Context, req MarkRequest, userID, userName string) (*entity.Entity, bool, error) {
	if req.Highlight == "" {
		return nil, false, fmt.Errorf("highlight name is required")
	}
	ns := req.Namespace
	if ns == "" {
		ns = entity.DefaultNamespace
	}
	existing, err := h.DB.GetEntity(ctx, "Highlight", ns, req.Highlight)
	created := false
	var current *entity.Entity
	if err != nil && !errors.Is(err, entity.ErrEntityNotFound) {
		return nil, false, err
	}
	if errors.Is(err, entity.ErrEntityNotFound) {
		color := req.Color
		if color == "" {
			color = "mark-1"
		}
		current = &entity.Entity{
			Kind:     "Highlight",
			Metadata: entity.EntityMetadata{Name: req.Highlight, Namespace: ns, Title: req.Highlight},
			Spec:     map[string]any{"color": color, "members": []any{}, "edges": []any{}},
		}
		created = true
	} else {
		current = existing
		if current.Spec == nil {
			current.Spec = map[string]any{}
		}
		if req.Color != "" {
			current.Spec["color"] = req.Color
		}
	}
	members := map[string]entity.Ref{}
	for _, ref := range entity.SpecRefs(current.Spec, "members") {
		members[markKey(ref)] = ref
	}
	for _, ref := range req.Add {
		r := entity.Ref{Kind: ref.Kind, Name: ref.Name, Namespace: ref.Namespace}
		members[markKey(r)] = r
	}
	for _, ref := range req.Remove {
		delete(members, markKey(entity.Ref{Kind: ref.Kind, Name: ref.Name, Namespace: ref.Namespace}))
	}
	edges := map[string]MarkEdge{}
	for _, edge := range specEdges(current.Spec) {
		edges[markKey(toRef(edge.From))+"->"+markKey(toRef(edge.To))] = edge
	}
	for _, edge := range req.AddEdges {
		edges[markKey(toRef(edge.From))+"->"+markKey(toRef(edge.To))] = edge
	}
	for _, edge := range req.RemoveEdges {
		delete(edges, markKey(toRef(edge.From))+"->"+markKey(toRef(edge.To)))
	}
	memberList := make([]any, 0, len(members))
	for _, ref := range members {
		memberList = append(memberList, refMap(ref))
	}
	edgeList := make([]any, 0, len(edges))
	for _, edge := range edges {
		edgeList = append(edgeList, map[string]any{"from": refMap(toRef(edge.From)), "to": refMap(toRef(edge.To))})
	}
	current.Spec["members"] = memberList
	current.Spec["edges"] = edgeList
	current.SetDefaults()
	if h.Validator != nil {
		if err := h.Validator.Validate(current); err != nil {
			return nil, false, fmt.Errorf("validation failed: %w", err)
		}
	}
	if created {
		if err := h.DB.CreateEntity(ctx, current); err != nil {
			return nil, false, err
		}
		h.publishEntity(events.EntityCreated, current)
		h.auditMark(ctx, userID, userName, "entity.created", current, "")
	} else {
		if err := h.DB.UpdateEntity(ctx, current); err != nil {
			return nil, false, err
		}
		h.publishEntity(events.EntityUpdated, current)
		h.auditMark(ctx, userID, userName, "entity.updated", current, "")
	}
	if h.GitOps != nil {
		h.GitOps.QueueChanges([]gitops.ChangeRef{{
			Kind: current.Kind, Namespace: current.Metadata.Namespace, Name: current.Metadata.Name, Action: "write",
		}})
	}
	return current, created, nil
}

func (h *Handlers) auditMark(ctx context.Context, userID, userName, action string, e *entity.Entity, before string) {
	if h.DB == nil {
		return
	}
	_ = h.DB.CreateAuditEntry(ctx, &db.AuditEntry{
		UserID: userID, UserName: userName, Action: action,
		ResourceType: e.Kind, ResourceName: e.Metadata.Name, BeforeState: before, Source: "mcp",
	})
}

func toRef(r MarkRef) entity.Ref {
	return entity.Ref{Kind: r.Kind, Name: r.Name, Namespace: r.Namespace}
}

func markKey(r entity.Ref) string {
	ns := r.Namespace
	if ns == "" {
		ns = entity.DefaultNamespace
	}
	return r.Kind + "/" + ns + "/" + r.Name
}

func refMap(r entity.Ref) map[string]any {
	out := map[string]any{"kind": r.Kind, "name": r.Name}
	if r.Namespace != "" {
		out["namespace"] = r.Namespace
	}
	return out
}

func specEdges(spec map[string]any) []MarkEdge {
	raw, _ := spec["edges"].([]any)
	var out []MarkEdge
	for _, item := range raw {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		from, okFrom := entity.RefFromSpec(obj["from"])
		to, okTo := entity.RefFromSpec(obj["to"])
		if !okFrom || !okTo {
			continue
		}
		out = append(out, MarkEdge{
			From: MarkRef{Kind: from.Kind, Name: from.Name, Namespace: from.Namespace},
			To:   MarkRef{Kind: to.Kind, Name: to.Name, Namespace: to.Namespace},
		})
	}
	return out
}
