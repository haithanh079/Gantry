package handlers

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go2engle/gantry/internal/db"
	"github.com/go2engle/gantry/internal/entity"
	"github.com/go2engle/gantry/internal/events"
	"github.com/go2engle/gantry/internal/landscape"
)

func (h *Handlers) landscapeModel(r *http.Request) (*landscape.Model, error) {
	return landscape.LoadModel(r.Context(), h.DB, func(entities []*entity.Entity) []*entity.Entity {
		return h.FilterEntitiesByAccess(r.Context(), entities)
	})
}

func landscapeQuery(r *http.Request) landscape.GraphQuery {
	layers := map[string]bool{}
	if raw := r.URL.Query().Get("layers"); raw != "" {
		for _, part := range strings.Split(raw, ",") {
			layers[strings.TrimSpace(part)] = true
		}
	}
	return landscape.GraphQuery{
		GroupBy: r.URL.Query().Get("groupBy"), Level: r.URL.Query().Get("level"), Layers: layers,
		Env: r.URL.Query().Get("env"), Cloud: r.URL.Query().Get("cloud"), Region: r.URL.Query().Get("region"),
		Runtime: r.URL.Query().Get("runtime"), Team: r.URL.Query().Get("team"), Project: r.URL.Query().Get("project"),
		Department: r.URL.Query().Get("department"), Lifecycle: r.URL.Query().Get("lifecycle"),
	}
}

func (h *Handlers) requireLandscape(w http.ResponseWriter, r *http.Request) bool {
	return h.RequireAccess(w, r, "landscape", "*", "read")
}

// GetLandscapeGraph handles GET /api/v1/landscape/graph.
func (h *Handlers) GetLandscapeGraph(w http.ResponseWriter, r *http.Request) {
	if !h.requireLandscape(w, r) {
		return
	}
	model, err := h.landscapeModel(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load landscape")
		return
	}
	writeJSON(w, http.StatusOK, model.BuildGraph(landscapeQuery(r)))
}

// GetLandscapeInventory handles GET /api/v1/landscape/inventory.
func (h *Handlers) GetLandscapeInventory(w http.ResponseWriter, r *http.Request) {
	if !h.requireLandscape(w, r) {
		return
	}
	model, err := h.landscapeModel(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load landscape")
		return
	}
	writeJSON(w, http.StatusOK, model.Inventory(landscapeQuery(r)))
}

// GetLandscapeDataFlows handles GET /api/v1/landscape/dataflows.
func (h *Handlers) GetLandscapeDataFlows(w http.ResponseWriter, r *http.Request) {
	if !h.requireLandscape(w, r) {
		return
	}
	model, err := h.landscapeModel(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load landscape")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"dataflows": model.DataFlows(landscapeQuery(r))})
}

// GetLandscapeBlastRadius handles GET /api/v1/landscape/blast-radius/{kind}/{ns}/{name}.
func (h *Handlers) GetLandscapeBlastRadius(w http.ResponseWriter, r *http.Request) {
	if !h.requireLandscape(w, r) {
		return
	}
	model, err := h.landscapeModel(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load landscape")
		return
	}
	subject := entity.CanonicalRef(chi.URLParam(r, "kind") + "/" + chi.URLParam(r, "ns") + "/" + chi.URLParam(r, "name"))
	writeJSON(w, http.StatusOK, model.BlastRadius(subject))
}

// GetLandscapeFindings handles GET /api/v1/landscape/findings.
func (h *Handlers) GetLandscapeFindings(w http.ResponseWriter, r *http.Request) {
	if !h.requireLandscape(w, r) {
		return
	}
	rows, err := h.DB.ListLandscapeFindings(r.Context(), db.LandscapeFindingFilter{
		Category: r.URL.Query().Get("category"),
		Severity: r.URL.Query().Get("severity"),
		Rule:     r.URL.Query().Get("rule"),
		Status:   r.URL.Query().Get("status"),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list findings")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"findings": rows})
}

// RecomputeLandscapeFindings evaluates Plan A rules and stores the result.
func (h *Handlers) RecomputeLandscapeFindings(ctx context.Context) error {
	model, err := landscape.LoadModel(ctx, h.DB, nil)
	if err != nil {
		return err
	}
	for rule, items := range landscape.Evaluate(model, landscape.PlanARules()) {
		rows := make([]db.LandscapeFinding, 0, len(items))
		for _, item := range items {
			rows = append(rows, db.LandscapeFinding{
				Rule: item.Rule, Category: item.Category, Severity: item.Severity,
				SubjectRef: string(item.Subject), Details: item.Details,
			})
		}
		if err := h.DB.UpsertLandscapeFindings(ctx, rule, rows); err != nil {
			return err
		}
	}
	return nil
}

// FindingsRunner debounces catalog-change recomputation.
type FindingsRunner struct {
	Handlers *Handlers
	Delay    time.Duration
	mu       sync.Mutex
	timer    *time.Timer
}

// Schedule waits Delay after the latest call, then recomputes findings.
func (r *FindingsRunner) Schedule() {
	if r == nil || r.Handlers == nil {
		return
	}
	delay := r.Delay
	if delay == 0 {
		delay = 2 * time.Second
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.timer != nil {
		r.timer.Stop()
	}
	r.timer = time.AfterFunc(delay, func() {
		_ = r.Handlers.RecomputeLandscapeFindings(context.Background())
	})
}

// Subscribe registers the runner on entity change events.
func (r *FindingsRunner) Subscribe(bus *events.Bus) {
	if bus == nil {
		return
	}
	for _, eventType := range []events.EventType{events.EntityCreated, events.EntityUpdated, events.EntityDeleted} {
		bus.Subscribe(eventType, func(events.Event) { r.Schedule() })
	}
}
