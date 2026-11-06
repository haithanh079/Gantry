package entity

import "testing"

func TestEntityValidateRejectsDisplayNameAsMetadataName(t *testing.T) {
	tests := []struct {
		name       string
		entityName string
	}{
		{name: "spaces", entityName: "Future State"},
		{name: "parentheses", entityName: "future-state-(draft)"},
		{name: "uppercase", entityName: "Future-state"},
		{name: "leading hyphen", entityName: "-future-state"},
		{name: "trailing dot", entityName: "future-state."},
		{name: "underscore", entityName: "future_state"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &Entity{
				Kind: "Flow",
				Metadata: EntityMetadata{
					Name: tt.entityName,
				},
			}

			if err := e.Validate(); err == nil {
				t.Fatalf("Validate() error = nil, want invalid metadata.name")
			}
		})
	}
}

func TestEntityValidateAllowsURLSafeMetadataName(t *testing.T) {
	tests := []string{
		"future-state-draft",
		"payment-api-v2",
		"service.tenant-a",
		"a1",
	}

	for _, name := range tests {
		t.Run(name, func(t *testing.T) {
			e := &Entity{
				Kind: "Service",
				Metadata: EntityMetadata{
					Name: name,
				},
			}

			if err := e.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestSchemaValidatorAllowsHealthCheckURLForAPIAndInfrastructure(t *testing.T) {
	validator, err := NewSchemaValidator("")
	if err != nil {
		t.Fatalf("NewSchemaValidator() error = %v", err)
	}

	tests := []struct {
		name string
		kind string
	}{
		{name: "api", kind: "API"},
		{name: "infrastructure", kind: "Infrastructure"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &Entity{
				Kind: tt.kind,
				Metadata: EntityMetadata{
					Name: "payments-health",
				},
				Spec: map[string]any{
					"healthCheckUrl": "https://example.com/healthz",
				},
			}

			if err := validator.Validate(e); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestLandscapeKindSchemas(t *testing.T) {
	validator, err := NewSchemaValidator("")
	if err != nil {
		t.Fatal(err)
	}
	valid := map[string]map[string]any{
		"Department":   {"type": "platform", "leads": []any{"ada"}},
		"Project":      {"type": "game", "status": "live", "department": map[string]any{"kind": "Department", "name": "music"}},
		"CloudAccount": {"provider": "aws", "accountId": "123", "envType": "production"},
		"Network":      {"provider": "aws", "networkType": "vpc", "region": "ap-southeast-1", "cidrs": []any{"10.0.0.0/16"}},
		"Connectivity": {"type": "vpc-peering", "from": map[string]any{"kind": "Network", "name": "a"}, "to": map[string]any{"kind": "Network", "name": "b"}, "bidirectional": true},
		"Deployment": {
			"target":      map[string]any{"kind": "Service", "name": "nakama"},
			"environment": map[string]any{"kind": "Environment", "name": "production"},
			"runtime":     map[string]any{"platform": "ecs", "cluster": "prod", "workload": "nakama"},
		},
		"DataFlow": {
			"source": map[string]any{"kind": "Infrastructure", "name": "r2-game-assets"},
			"target": map[string]any{"kind": "Deployment", "name": "s3-app-storage-production"},
			"actor":  map[string]any{"kind": "Deployment", "name": "asset-sync-production"},
			"mode":   "batch", "transport": "public-internet", "dataClasses": []any{"assets"},
		},
		"Highlight": {"color": "mark-3", "defaultOn": true, "selector": map[string]any{"transports": []any{"public-internet"}}},
	}
	for kind, spec := range valid {
		e := &Entity{Kind: kind, Metadata: EntityMetadata{Name: "example"}, Spec: spec}
		e.SetDefaults()
		if err := validator.Validate(e); err != nil {
			t.Errorf("%s valid spec rejected: %v", kind, err)
		}
	}
	bad := &Entity{Kind: "DataFlow", Metadata: EntityMetadata{Name: "bad"}, Spec: map[string]any{"mode": "batch", "extra": true}}
	bad.SetDefaults()
	if err := validator.Validate(bad); err == nil {
		t.Fatal("expected additionalProperties rejection")
	}
}
