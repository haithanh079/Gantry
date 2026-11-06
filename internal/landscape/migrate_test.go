package landscape

import (
	"testing"

	"github.com/go2engle/gantry/internal/entity"
)

func TestFixtureMigrationCreates22DeploymentsAndIsIdempotent(t *testing.T) {
	entities := FixtureCatalog()
	plan := PlanMigration(Load(entities))
	if got := plan.DeploymentCreates(); got != 22 {
		t.Fatalf("deployments = %d, want 22 (changes %d)", got, len(plan.Changes))
	}
	again := PlanMigration(Load(applyPlanInMemory(entities, plan)))
	if got := again.DeploymentCreates(); got != 0 {
		t.Fatalf("second plan created %d deployments", got)
	}
	if len(again.Changes) != 0 {
		t.Fatalf("second plan was not empty: %d changes", len(again.Changes))
	}
}

func applyPlanInMemory(entities []*entity.Entity, plan MigrationPlan) []*entity.Entity {
	replaced := map[entity.CanonicalRef]*entity.Entity{}
	for _, change := range plan.Changes {
		if change.Action == "update" && change.Entity != nil {
			replaced[entity.IDOf(change.Entity)] = change.Entity
		}
	}
	out := make([]*entity.Entity, 0, len(entities)+len(plan.Changes))
	for _, e := range entities {
		if next, ok := replaced[entity.IDOf(e)]; ok {
			out = append(out, next)
			continue
		}
		out = append(out, e)
	}
	for _, change := range plan.Changes {
		if change.Action == "create" && change.Entity != nil {
			out = append(out, change.Entity)
		}
	}
	return out
}
