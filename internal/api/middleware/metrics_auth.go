package middleware

import (
	"net/http"
	"strings"
)

// MetricsAuth requires a bearer token or ?token= query param when token is non-empty.
func MetricsAuth(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if token == "" {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if metricsTokenMatches(r, token) {
				next.ServeHTTP(w, r)
				return
			}
			http.Error(w, `{"error":"authentication required"}`, http.StatusUnauthorized)
		})
	}
}

func metricsTokenMatches(r *http.Request, expected string) bool {
	if q := r.URL.Query().Get("token"); q != "" && q == expected {
		return true
	}
	header := r.Header.Get("Authorization")
	if header == "" {
		return false
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return false
	}
	return parts[1] == expected
}
