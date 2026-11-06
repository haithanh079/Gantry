package handlers

import (
	"context"
	"fmt"

	"github.com/go2engle/gantry/internal/entity"
	"github.com/go2engle/gantry/internal/events"
	"github.com/go2engle/gantry/internal/gitops"
	"github.com/go2engle/gantry/internal/landscape"
)

// LandscapeMigrationResult is the dry-run or apply result of migrate landscape.
type LandscapeMigrationResult struct {
	Applied     bool               `json:"applied"`
	Deployments int                `json:"deployments"`
	Changes     []landscape.Change `json:"changes"`
}

// ApplyLandscapeMigration plans Deployment and Project entities from deployedIn.
// apply false returns the plan. apply true validates and writes it. A second
// apply is a no-op for deployments that already exist.
func (h *Handlers) ApplyLandscapeMigration(ctx context.Context, apply bool) (LandscapeMigrationResult, error) {
	model, err := landscape.LoadModel(ctx, h.DB, nil)
	if err != nil {
		return LandscapeMigrationResult{}, err
	}
	plan := landscape.PlanMigration(model)
	result := LandscapeMigrationResult{
		Applied: apply, Deployments: plan.DeploymentCreates(), Changes: plan.Changes,
	}
	if !apply {
		return result, nil
	}
	var refs []gitops.ChangeRef
	for _, change := range plan.Changes {
		e := change.Entity
		if e == nil {
			continue
		}
		e.SetDefaults()
		if h.Validator != nil {
			if err := h.Validator.Validate(e); err != nil {
				return result, fmt.Errorf("validate %s/%s: %w", e.Kind, e.Metadata.Name, err)
			}
		}
		switch change.Action {
		case "create":
			if err := h.DB.CreateEntity(ctx, e); err != nil {
				return result, fmt.Errorf("create %s/%s: %w", e.Kind, e.Metadata.Name, err)
			}
			h.publishEntity(events.EntityCreated, e)
		case "update":
			if err := h.DB.UpdateEntity(ctx, e); err != nil {
				return result, fmt.Errorf("update %s/%s: %w", e.Kind, e.Metadata.Name, err)
			}
			h.publishEntity(events.EntityUpdated, e)
		default:
			return result, fmt.Errorf("unknown migration action %q", change.Action)
		}
		refs = append(refs, gitops.ChangeRef{
			Kind: e.Kind, Namespace: e.Metadata.Namespace, Name: e.Metadata.Name, Action: "write",
		})
	}
	if h.GitOps != nil && len(refs) > 0 {
		h.GitOps.QueueChanges(refs)
	}
	return result, nil
}

func (h *Handlers) publishEntity(eventType events.EventType, e *entity.Entity) {
	if h.Events == nil || e == nil {
		return
	}
	h.Events.Publish(events.Event{
		Type: eventType,
		Data: map[string]any{
			"kind": e.Kind, "name": e.Metadata.Name, "namespace": e.Metadata.Namespace,
		},
	})
}
