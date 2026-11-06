package db

import (
	"context"
	"testing"
)

func TestUpsertLandscapeFindingsResolvesStaleRows(t *testing.T) {
	ctx := context.Background()
	database := newTestDB(t)
	open := []LandscapeFinding{{
		Rule: "missing-owner", Category: "finding", Severity: "warning",
		SubjectRef: "Service/default/nakama", Details: map[string]any{},
	}}
	if err := database.UpsertLandscapeFindings(ctx, "missing-owner", open); err != nil {
		t.Fatal(err)
	}
	if err := database.UpsertLandscapeFindings(ctx, "missing-owner", nil); err != nil {
		t.Fatal(err)
	}
	rows, err := database.ListLandscapeFindings(ctx, LandscapeFindingFilter{Rule: "missing-owner"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Status != "resolved" {
		t.Fatalf("rows = %#v", rows)
	}
	again, err := database.ListLandscapeFindings(ctx, LandscapeFindingFilter{Status: "open"})
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 0 {
		t.Fatalf("open findings = %#v", again)
	}
}
