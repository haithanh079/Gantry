package handlers

import (
	"context"
	"net/http"

	"github.com/go2engle/gantry/internal/api/middleware"
	"github.com/go2engle/gantry/internal/auth"
	"github.com/go2engle/gantry/internal/entity"
	"github.com/go2engle/gantry/internal/search"
)

// CanAccess reports whether the authenticated user may perform action on a resource.
func (h *Handlers) CanAccess(ctx context.Context, resourceType, resourceFilter, action string) bool {
	claims := middleware.GetClaims(ctx)
	if claims == nil {
		return false
	}

	effectiveRole := middleware.GetEffectiveRole(ctx)
	if effectiveRole == "" {
		effectiveRole = claims.Role
	}

	rules, err := h.DB.GetEffectiveRules(ctx, claims.UserID)
	if err != nil {
		rules = nil
	}

	checks := make([]auth.PermissionCheck, len(rules))
	for i, r := range rules {
		checks[i] = auth.PermissionCheck{
			SubjectType:    r.SubjectType,
			SubjectID:      r.SubjectID,
			ResourceType:   r.ResourceType,
			ResourceFilter: r.ResourceFilter,
			Action:         r.Action,
			Effect:         r.Effect,
		}
	}

	return auth.EvaluateAccess(effectiveRole, checks, resourceType, resourceFilter, action)
}

// RequireAccess writes 403 and returns false when access is denied.
func (h *Handlers) RequireAccess(w http.ResponseWriter, r *http.Request, resourceType, resourceFilter, action string) bool {
	if h.CanAccess(r.Context(), resourceType, resourceFilter, action) {
		return true
	}
	writeError(w, http.StatusForbidden, "insufficient permissions")
	return false
}

// FilterEntitiesByAccess returns entities the caller may read.
func (h *Handlers) FilterEntitiesByAccess(ctx context.Context, entities []*entity.Entity) []*entity.Entity {
	if len(entities) == 0 {
		return entities
	}
	filtered := make([]*entity.Entity, 0, len(entities))
	for _, e := range entities {
		if h.CanAccess(ctx, "entity", e.Kind, "read") {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

// FilterSearchResultsByAccess returns search hits the caller may read.
func (h *Handlers) FilterSearchResultsByAccess(ctx context.Context, results []*search.Result) []*search.Result {
	if len(results) == 0 {
		return results
	}
	filtered := make([]*search.Result, 0, len(results))
	for _, r := range results {
		if h.CanAccess(ctx, "entity", r.Kind, "read") {
			filtered = append(filtered, r)
		}
	}
	return filtered
}
