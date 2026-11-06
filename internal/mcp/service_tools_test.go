package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/go2engle/gantry/internal/entity"
)

func TestApplyServiceUpdateKeepsUnspecifiedFields(t *testing.T) {
	e := &entity.Entity{
		Kind: "Service",
		Metadata: entity.EntityMetadata{
			Name:        "payments",
			Namespace:   "default",
			Title:       "Payments",
			Description: "takes money",
			Owner:       "platform",
			Tags:        []string{"money"},
		},
		Spec: specFrom(t, map[string]any{
			"type":      "backend",
			"lifecycle": "production",
			"repoUrl":   "https://github.com/acme/payments",
		}),
	}
	title := "Billing"
	applyServiceUpdate(e, updateServiceInput{Name: "payments", Title: &title})

	if e.Metadata.Title != "Billing" {
		t.Fatalf("title = %q", e.Metadata.Title)
	}
	if e.Metadata.Description != "takes money" || e.Metadata.Owner != "platform" {
		t.Fatalf("metadata was overwritten: %+v", e.Metadata)
	}
	if e.Spec["type"] != "backend" || e.Spec["lifecycle"] != "production" || e.Spec["repoUrl"] != "https://github.com/acme/payments" {
		t.Fatalf("spec was overwritten: %#v", e.Spec)
	}
	if err := validateService(t, e); err != nil {
		t.Fatal(err)
	}
}

func TestApplyServiceLinkRelationAndURL(t *testing.T) {
	e := &entity.Entity{
		Kind: "Service",
		Metadata: entity.EntityMetadata{
			Name:      "payments",
			Namespace: "default",
			Title:     "Payments",
		},
		Spec: specFrom(t, map[string]any{
			"type":      "backend",
			"lifecycle": "production",
		}),
	}

	addDep := linkServiceInput{
		Name: "payments", Target: "relation", Op: "add", Relation: "dependsOn",
		Kind: "Service", TargetName: "auth",
	}
	if err := applyServiceLink(e, addDep); err != nil {
		t.Fatal(err)
	}
	if err := applyServiceLink(e, addDep); err != nil {
		t.Fatal(err)
	}
	deps := objectRefs(e.Spec, "dependsOn")
	if len(deps) != 1 || stringField(deps[0], "name") != "auth" {
		t.Fatalf("dependsOn = %#v", e.Spec["dependsOn"])
	}

	if err := applyServiceLink(e, linkServiceInput{
		Name: "payments", Target: "relation", Op: "add", Relation: "providesApi", TargetName: "payments-api",
	}); err != nil {
		t.Fatal(err)
	}
	if err := applyServiceLink(e, linkServiceInput{
		Name: "payments", Target: "relation", Op: "add", Relation: "owner", Kind: "Team", TargetName: "platform",
	}); err != nil {
		t.Fatal(err)
	}
	if e.Metadata.Owner != "platform" {
		t.Fatalf("owner = %q", e.Metadata.Owner)
	}

	if err := applyServiceLink(e, linkServiceInput{
		Name: "payments", Target: "url", Op: "add", Title: "Docs", URL: "https://docs.example/payments", Icon: "docs",
	}); err != nil {
		t.Fatal(err)
	}
	if err := applyServiceLink(e, linkServiceInput{
		Name: "payments", Target: "url", Op: "add", Title: "Docs", URL: "https://docs.example/payments", Icon: "docs",
	}); err != nil {
		t.Fatal(err)
	}
	links := objectRefs(e.Spec, "links")
	if len(links) != 1 {
		t.Fatalf("links = %#v", e.Spec["links"])
	}
	if e.Spec["type"] != "backend" || e.Spec["lifecycle"] != "production" {
		t.Fatalf("link overwrote spec: %#v", e.Spec)
	}
	if err := validateService(t, e); err != nil {
		t.Fatal(err)
	}

	if err := applyServiceLink(e, linkServiceInput{
		Name: "payments", Target: "relation", Op: "remove", Relation: "dependsOn", Kind: "Service", TargetName: "auth",
	}); err != nil {
		t.Fatal(err)
	}
	if err := applyServiceLink(e, linkServiceInput{
		Name: "payments", Target: "relation", Op: "remove", Relation: "dependsOn", Kind: "Service", TargetName: "missing",
	}); err != nil {
		t.Fatal(err)
	}
	if _, ok := e.Spec["dependsOn"]; ok {
		t.Fatalf("dependsOn still present: %#v", e.Spec["dependsOn"])
	}
	if err := applyServiceLink(e, linkServiceInput{
		Name: "payments", Target: "url", Op: "remove", URL: "https://docs.example/payments",
	}); err != nil {
		t.Fatal(err)
	}
	if _, ok := e.Spec["links"]; ok {
		t.Fatalf("links still present: %#v", e.Spec["links"])
	}
	if got := stringItems(e.Spec, "providesApis"); len(got) != 1 || got[0] != "payments-api" {
		t.Fatalf("providesApis = %#v", e.Spec["providesApis"])
	}
	if err := validateService(t, e); err != nil {
		t.Fatal(err)
	}
}

func specFrom(t *testing.T, spec map[string]any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func validateService(t *testing.T, e *entity.Entity) error {
	t.Helper()
	v, err := entity.NewSchemaValidator("")
	if err != nil {
		t.Fatal(err)
	}
	err = v.Validate(e)
	if err != nil && strings.Contains(err.Error(), "additional properties") {
		t.Fatalf("schema rejected spec: %v\n%#v", err, e.Spec)
	}
	return err
}
