package auth

import "testing"

func TestEvaluateAccessDenyWinsOverAllow(t *testing.T) {
	rules := []PermissionCheck{
		{ResourceType: "entity", ResourceFilter: "Service", Action: "write", Effect: "allow"},
		{ResourceType: "entity", ResourceFilter: "Service", Action: "write", Effect: "deny"},
	}
	if EvaluateAccess("developer", rules, "entity", "Service", "write") {
		t.Fatal("expected deny to win")
	}
}

func TestEvaluateAccessFallsBackToRoleWithoutRules(t *testing.T) {
	if !EvaluateAccess("developer", nil, "entity", "Service", "write") {
		t.Fatal("expected developer to write without explicit rules")
	}
	if EvaluateAccess("viewer", nil, "entity", "Service", "write") {
		t.Fatal("expected viewer to be denied write without explicit rules")
	}
}

func TestEvaluateAccessExplicitAllow(t *testing.T) {
	rules := []PermissionCheck{
		{ResourceType: "entity", ResourceFilter: "Service", Action: "read", Effect: "allow"},
	}
	if !EvaluateAccess("viewer", rules, "entity", "Service", "read") {
		t.Fatal("expected explicit allow for viewer read on Service")
	}
}
