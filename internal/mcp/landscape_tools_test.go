package mcp

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/go2engle/gantry/internal/api/handlers"
	"github.com/go2engle/gantry/internal/api/middleware"
	"github.com/go2engle/gantry/internal/auth"
	"github.com/go2engle/gantry/internal/config"
	"github.com/go2engle/gantry/internal/db"
	"github.com/go2engle/gantry/internal/entity"
	"github.com/go2engle/gantry/internal/events"
	"github.com/go2engle/gantry/internal/landscape"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func testHandlers(t *testing.T) *handlers.Handlers {
	t.Helper()
	dataDir := t.TempDir()
	cfg := &config.Config{
		DBType: "sqlite", DBDSN: filepath.Join(dataDir, "gantry.db"),
		DataDir: dataDir, EncryptionKey: "test-encryption-key", JWTSecret: "test",
	}
	database, err := db.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.Migrate(); err != nil {
		t.Fatal(err)
	}
	validator, err := entity.NewSchemaValidator("")
	if err != nil {
		t.Fatal(err)
	}
	return &handlers.Handlers{DB: database, Events: events.New(), Validator: validator, Version: "test"}
}

func authCtx(role string) context.Context {
	return middleware.ContextWithAuth(context.Background(), &auth.Claims{
		UserID: "user-1", Username: "dev", Role: role,
	}, role)
}

func TestLandscapeToolsInventoryAndMigrate(t *testing.T) {
	h := testHandlers(t)
	ctx := context.Background()
	for _, e := range landscape.FixtureCatalog() {
		e.SetDefaults()
		if err := h.DB.CreateEntity(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	_, result, err := landscapeInventoryTool(authCtx("viewer"), h, landscapeQueryInput{})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("empty inventory result")
	}
	_, _, err = landscapeInventoryTool(context.Background(), h, landscapeQueryInput{})
	if err == nil {
		t.Fatal("expected auth failure")
	}
	_, migrated, err := migrateLandscapeTool(authCtx("developer"), h, migrateLandscapeInput{Apply: true})
	if err != nil {
		t.Fatal(err)
	}
	body, ok := migrated.(handlers.LandscapeMigrationResult)
	if !ok || body.Deployments != 22 {
		t.Fatalf("migration %#v", migrated)
	}
	_, again, err := migrateLandscapeTool(authCtx("developer"), h, migrateLandscapeInput{Apply: true})
	if err != nil {
		t.Fatal(err)
	}
	second, _ := again.(handlers.LandscapeMigrationResult)
	if second.Deployments != 0 {
		t.Fatalf("second apply created %d deployments", second.Deployments)
	}
	_, _, err = migrateLandscapeTool(authCtx("viewer"), h, migrateLandscapeInput{})
	if err == nil {
		t.Fatal("viewer should not migrate")
	}
}

func TestMarkEntitiesCreatesHighlight(t *testing.T) {
	h := testHandlers(t)
	_, result, err := markEntitiesTool(authCtx("developer"), h, markEntitiesInput{
		Highlight: "egress",
		Color:     "mark-3",
		Add:       []markRefInput{{Kind: "Service", Name: "nakama", Namespace: "default"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := result.(map[string]any)
	if body["created"] != true {
		t.Fatalf("created %#v", body["created"])
	}
	stored, err := h.DB.GetEntity(context.Background(), "Highlight", "default", "egress")
	if err != nil {
		t.Fatal(err)
	}
	if stored.Spec["color"] != "mark-3" {
		t.Fatalf("spec %#v", stored.Spec)
	}
	_, _, err = markEntitiesTool(authCtx("viewer"), h, markEntitiesInput{Highlight: "other"})
	if err == nil {
		t.Fatal("viewer should not mark")
	}
}

func TestLandscapeToolsAreRegistered(t *testing.T) {
	h := testHandlers(t)
	srv := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "gantry", Version: "test"}, nil)
	registerLandscapeTools(srv, h)
	ctx := context.Background()
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "test"}, nil)
	cTransport, sTransport := mcpsdk.NewInMemoryTransports()
	if _, err := srv.Connect(ctx, sTransport, nil); err != nil {
		t.Fatal(err)
	}
	session, err := client.Connect(ctx, cTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"get_landscape_inventory": false, "get_blast_radius": false, "list_findings": false,
		"get_landscape_dataflows": false, "mark_entities": false, "migrate_landscape": false,
	}
	for _, tool := range listed.Tools {
		if _, ok := want[tool.Name]; ok {
			want[tool.Name] = true
		}
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("tool %s not registered", name)
		}
	}
}
