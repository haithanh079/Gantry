package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoginRateLimiterBurst(t *testing.T) {
	rl := newRateLimiter(10.0/60.0, 5)
	ip := "203.0.113.10"
	for i := 0; i < 5; i++ {
		if !rl.allow(ip) {
			t.Fatalf("expected attempt %d to be allowed", i+1)
		}
	}
	if rl.allow(ip) {
		t.Fatal("expected 6th login attempt to be rate limited")
	}
}

func TestMetricsAuthRequiresTokenWhenConfigured(t *testing.T) {
	const token = "metrics-secret"
	handler := MetricsAuth(token)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with bearer token, got %d", rec.Code)
	}
}
