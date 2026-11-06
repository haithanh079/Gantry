package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go2engle/gantry/internal/db"
	"github.com/go2engle/gantry/internal/entity"
)

func TestLandscapeViewerCanReadGraph(t *testing.T) {
	env := newTestServerEnv(t)
	_, token := env.createUser(t, "landscape-viewer", "viewer")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/landscape/graph?level=deployment", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"highlights"`) {
		t.Fatalf("body %s", rec.Body.String())
	}
}

func TestLandscapeRequiresAuth(t *testing.T) {
	env := newTestServerEnv(t)
	rec := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/landscape/inventory", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestLandscapeHiddenKindDoesNotLeakThroughHighlight(t *testing.T) {
	env := newTestServerEnv(t)
	user, token := env.createUser(t, "landscape-limited", "viewer")
	_, adminToken := env.createUser(t, "landscape-admin", "admin")
	ctx := context.Background()
	if err := env.db.CreatePermissionRule(ctx, &db.PermissionRule{
		SubjectType: "user", SubjectID: user.ID, ResourceType: "entity", ResourceFilter: "DataFlow", Action: "read", Effect: "deny",
	}); err != nil {
		t.Fatal(err)
	}
	actor := &entity.Entity{
		Kind: "Deployment", Metadata: entity.EntityMetadata{Name: "sync", Namespace: "default", Owner: "team"},
		Spec: map[string]any{
			"target": map[string]any{"kind": "Service", "name": "sync"},
			"environment": map[string]any{"kind": "Environment", "name": "production"},
			"runtime": map[string]any{"platform": "ecs"},
		},
	}
	flow := &entity.Entity{
		Kind: "DataFlow", Metadata: entity.EntityMetadata{Name: "secret-flow", Namespace: "default"},
		Spec: map[string]any{
			"source": map[string]any{"kind": "Infrastructure", "name": "r2"},
			"target": map[string]any{"kind": "Deployment", "name": "sync"},
			"actor": map[string]any{"kind": "Deployment", "name": "sync"},
			"mode": "batch", "transport": "public-internet",
		},
	}
	highlight := &entity.Entity{
		Kind: "Highlight", Metadata: entity.EntityMetadata{Name: "egress", Namespace: "default", Title: "Egress"},
		Spec: map[string]any{"color": "mark-3", "defaultOn": true, "includeNeighbors": false, "members": []any{map[string]any{"kind": "DataFlow", "name": "secret-flow"}}},
	}
	for _, e := range []*entity.Entity{actor, flow, highlight} {
		e.SetDefaults()
		if err := env.db.CreateEntity(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	adminReq := httptest.NewRequest(http.MethodGet, "/api/v1/landscape/graph?level=deployment&layers=data", nil)
	adminReq.Header.Set("Authorization", "Bearer "+adminToken)
	adminRec := httptest.NewRecorder()
	env.server.Router().ServeHTTP(adminRec, adminReq)
	if adminRec.Code != http.StatusOK || !strings.Contains(adminRec.Body.String(), "secret-flow") {
		t.Fatalf("admin should see the flow, status %d body %s", adminRec.Code, adminRec.Body.String())
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/landscape/graph?level=deployment&layers=data", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "secret-flow") {
		t.Fatalf("hidden data flow leaked: %s", rec.Body.String())
	}
}

func TestLandscapeFindingsRoundTrip(t *testing.T) {
	env := newTestServerEnv(t)
	_, token := env.createUser(t, "landscape-findings", "viewer")
	ownerless := &entity.Entity{Kind: "Service", Metadata: entity.EntityMetadata{Name: "lonely", Namespace: "default"}, Spec: map[string]any{}}
	ownerless.SetDefaults()
	if err := env.db.CreateEntity(context.Background(), ownerless); err != nil {
		t.Fatal(err)
	}
	if err := env.server.Handlers.RecomputeLandscapeFindings(context.Background()); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/landscape/findings?rule=missing-owner&status=open", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	env.server.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Findings []db.LandscapeFinding `json:"findings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Findings) == 0 || body.Findings[0].SubjectRef != "Service/default/lonely" {
		t.Fatalf("findings %#v", body.Findings)
	}
}
