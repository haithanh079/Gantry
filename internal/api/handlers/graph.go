package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// GetEntityGraph returns the relationship graph centered on the given entity.
func (h *Handlers) GetEntityGraph(w http.ResponseWriter, r *http.Request) {
	kind := chi.URLParam(r, "kind")
	name := chi.URLParam(r, "name")
	namespace := r.URL.Query().Get("namespace")
	if namespace == "" {
		namespace = "default"
	}

	if !h.RequireAccess(w, r, "entity", kind, "read") {
		return
	}

	graph, err := h.DB.GetEntityGraph(r.Context(), kind, namespace, name)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, graph)
}
