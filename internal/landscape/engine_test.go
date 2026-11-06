package landscape

import (
	"testing"

	"github.com/go2engle/gantry/internal/entity"
)

func ref(kind, name string) map[string]any {
	return map[string]any{"kind": kind, "name": name}
}

func ent(kind, ns, name, owner string, spec map[string]any) *entity.Entity {
	e := &entity.Entity{Kind: kind, Metadata: entity.EntityMetadata{Name: name, Namespace: ns, Owner: owner}, Spec: spec}
	e.SetDefaults()
	return e
}

func musicFixture() *Model {
	return Load([]*entity.Entity{
		ent("Department", "default", "music", "ada", map[string]any{"type": "department"}),
		ent("Project", "music", "personalize", "ada", map[string]any{"type": "platform", "department": ref("Department", "music")}),
		ent("Environment", "music", "production", "", map[string]any{"type": "production", "provider": "aws", "region": "ap-southeast-1"}),
		ent("Environment", "music", "qc", "", map[string]any{"type": "qc", "provider": "aws", "region": "ap-southeast-1"}),
		ent("CloudAccount", "music", "aws-prod", "", map[string]any{"provider": "aws", "accountId": "111"}),
		ent("Service", "music", "upe", "personalize", map[string]any{
			"lifecycle": "production",
			"projects":  []any{ref("Project", "personalize")},
			"dependsOn": []any{ref("Infrastructure", "aurora")},
		}),
		ent("Infrastructure", "music", "aurora", "personalize", map[string]any{
			"type": "database", "engine": "aurora-postgres",
			"projects": []any{ref("Project", "personalize")},
		}),
		ent("Infrastructure", "music", "r2-assets", "", map[string]any{"type": "bucket", "engine": "r2"}),
		ent("Deployment", "music", "upe-production", "personalize", map[string]any{
			"target": ref("Service", "upe"), "environment": ref("Environment", "production"),
			"cloudAccount": ref("CloudAccount", "aws-prod"),
			"regions":      []any{"ap-southeast-1"},
			"runtime":      map[string]any{"platform": "ecs"},
		}),
		ent("Deployment", "music", "aurora-production", "personalize", map[string]any{
			"target": ref("Infrastructure", "aurora"), "environment": ref("Environment", "production"),
			"regions": []any{"ap-southeast-1"}, "runtime": map[string]any{"platform": "managed"},
			"connectsTo": []any{map[string]any{"deployment": ref("Deployment", "upe-qc"), "protocol": "tcp", "port": float64(5432)}},
		}),
		ent("Deployment", "music", "upe-qc", "", map[string]any{
			"target": ref("Service", "upe"), "environment": ref("Environment", "qc"),
			"runtime": map[string]any{"platform": "ecs"},
		}),
		ent("Deployment", "music", "asset-sync-production", "personalize", map[string]any{
			"target": ref("Service", "upe"), "environment": ref("Environment", "production"),
			"runtime": map[string]any{"platform": "ecs"},
		}),
		ent("DataFlow", "music", "r2-to-s3", "personalize", map[string]any{
			"source": ref("Infrastructure", "r2-assets"), "target": ref("Deployment", "aurora-production"),
			"actor": ref("Deployment", "asset-sync-production"), "mode": "batch", "transport": "public-internet",
			"dataClasses": []any{"assets"},
		}),
		ent("DataFlow", "music", "pii-down", "", map[string]any{
			"source": ref("Deployment", "aurora-production"), "target": ref("Deployment", "upe-qc"),
			"mode": "backup", "transport": "public-internet", "dataClasses": []any{"pii"},
		}),
		ent("Highlight", "music", "egress", "", map[string]any{
			"color": "mark-3", "defaultOn": true, "includeNeighbors": false,
			"selector": map[string]any{"transports": []any{"public-internet"}},
		}),
	})
}

func TestInferredConnectsToUsesEnginePortAndDeclaredOverrides(t *testing.T) {
	m := musicFixture()
	var inferred, declared int
	for _, conn := range m.Connections() {
		if conn.From == "Deployment/music/upe-production" && conn.To == "Deployment/music/aurora-production" {
			inferred++
			if !conn.Inferred || conn.Port != 5432 {
				t.Fatalf("inferred edge = %#v", conn)
			}
		}
		if conn.From == "Deployment/music/aurora-production" && conn.To == "Deployment/music/upe-qc" {
			declared++
			if conn.Inferred {
				t.Fatal("cross-env edge must stay declared")
			}
		}
	}
	if inferred != 1 || declared != 1 {
		t.Fatalf("inferred=%d declared=%d all=%#v", inferred, declared, m.Connections())
	}
	for _, conn := range m.Connections() {
		if conn.From == "DataFlow/music/r2-to-s3" || conn.To == "DataFlow/music/r2-to-s3" {
			t.Fatal("data flow became a connection")
		}
	}
}

func TestGraphLayersAndHighlight(t *testing.T) {
	graph := musicFixture().BuildGraph(GraphQuery{Level: "deployment", Layers: map[string]bool{"network": true, "data": true}})
	var dataEdges int
	for _, edge := range graph.Edges {
		if edge.Layer == "data" && edge.DataFlowRef == "DataFlow/music/r2-to-s3" {
			dataEdges++
			if edge.Transport != "public-internet" || edge.ActorRef != "Deployment/music/asset-sync-production" {
				t.Fatalf("data edge %#v", edge)
			}
		}
	}
	if dataEdges != 1 {
		t.Fatalf("data edges = %d, graph=%#v", dataEdges, graph.Edges)
	}
	if len(graph.Highlights) != 1 || !graph.Highlights[0].DefaultOn {
		t.Fatalf("highlights %#v", graph.Highlights)
	}
	company := musicFixture().BuildGraph(GraphQuery{Level: "company"})
	if len(company.Nodes) == 0 {
		t.Fatal("company graph empty")
	}
}

func TestHighlightHidesInaccessibleMember(t *testing.T) {
	full := musicFixture()
	var visible []*entity.Entity
	for _, e := range full.Entities {
		if e.Kind == "DataFlow" && e.Metadata.Name == "pii-down" {
			continue
		}
		visible = append(visible, e)
	}
	hidden := Load(visible)
	graph := hidden.BuildGraph(GraphQuery{Level: "deployment"})
	for _, h := range graph.Highlights {
		for _, id := range h.NodeIDs {
			if id == "DataFlow/music/pii-down" {
				t.Fatal("hidden data flow leaked through highlight")
			}
		}
	}
}

func TestPlanAFindingRules(t *testing.T) {
	found := Evaluate(musicFixture(), PlanARules())
	if len(found["cross-env-dependency"]) != 1 {
		t.Fatalf("cross-env %#v", found["cross-env-dependency"])
	}
	if len(found["dataflow-sensitive-to-lower-env"]) != 1 {
		t.Fatalf("sensitive lower %#v", found["dataflow-sensitive-to-lower-env"])
	}
	if len(found["dataflow-sensitive-public"]) != 1 {
		t.Fatalf("sensitive public %#v", found["dataflow-sensitive-public"])
	}
	if len(found["dataflow-no-actor"]) != 1 {
		t.Fatalf("no actor %#v", found["dataflow-no-actor"])
	}
	if len(found["missing-owner"]) == 0 {
		t.Fatal("expected missing owner on upe-qc")
	}
}

func TestBlastRadiusIncludesCallerAndDataFlow(t *testing.T) {
	blast := musicFixture().BlastRadius("Deployment/music/aurora-production")
	joined := ""
	for _, g := range blast.Groups {
		for _, d := range g.Deployments {
			joined += d + " "
		}
		for _, f := range g.DataFlows {
			joined += f + " "
		}
	}
	if !contains(joined, "Deployment/music/upe-production") || !contains(joined, "DataFlow/music/r2-to-s3") {
		t.Fatalf("blast %s groups %#v", joined, blast.Groups)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || (len(s) > 0 && (stringIndex(s, sub) >= 0)))
}

func stringIndex(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
