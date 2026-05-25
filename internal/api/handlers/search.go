package handlers

import (
	"net/http"

	"github.com/go2engle/gantry/internal/search"
)

// Search handles GET /search?q=. It performs a full-text search using the
// search service and returns matching results.
func (h *Handlers) Search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	results, err := h.SearchSvc.Search(r.Context(), q)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}

	if results == nil {
		results = make([]*search.Result, 0)
	}

	results = h.FilterSearchResultsByAccess(r.Context(), results)

	writeJSON(w, http.StatusOK, results)
}
