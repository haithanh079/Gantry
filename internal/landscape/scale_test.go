package landscape

import (
	"fmt"
	"testing"
	"time"

	"github.com/go2engle/gantry/internal/entity"
)

func TestGraphStaysUnder300msAt500Deployments(t *testing.T) {
	entities := scaleCatalog(500, 40, 5)
	model := Load(entities)
	start := time.Now()
	graph := model.BuildGraph(GraphQuery{Level: "deployment", GroupBy: "org", Layers: map[string]bool{"network": true, "data": true}})
	elapsed := time.Since(start)
	t.Logf("deployment graph: %s nodes=%d edges=%d", elapsed, len(graph.Nodes), len(graph.Edges))
	if elapsed > 300*time.Millisecond {
		t.Fatalf("graph build took %s, want under 300ms", elapsed)
	}
	if len(graph.Nodes) < 500 {
		t.Fatalf("nodes = %d, want at least 500", len(graph.Nodes))
	}
}

func scaleCatalog(deployments, flows, highlights int) []*entity.Entity {
	var entities []*entity.Entity
	envs := []string{"development", "qc", "staging", "production"}
	for _, name := range envs {
		entities = append(entities, &entity.Entity{
			Kind: "Environment", Metadata: entity.EntityMetadata{Name: name, Namespace: "default"},
			Spec: map[string]any{"type": name, "provider": "aws", "region": "us-east-1"},
		})
	}
	for i := 0; i < 4; i++ {
		entities = append(entities, &entity.Entity{
			Kind: "Department", Metadata: entity.EntityMetadata{Name: fmt.Sprintf("dept-%d", i), Namespace: "default"},
			Spec: map[string]any{},
		})
	}
	for i := 0; i < 10; i++ {
		entities = append(entities, &entity.Entity{
			Kind: "Project", Metadata: entity.EntityMetadata{Name: fmt.Sprintf("game-%d", i), Namespace: "default"},
			Spec: map[string]any{"type": "game", "department": map[string]any{"kind": "Department", "name": fmt.Sprintf("dept-%d", i%4)}},
		})
	}
	services := (deployments + len(envs) - 1) / len(envs)
	for i := 0; i < services; i++ {
		entities = append(entities, &entity.Entity{
			Kind:     "Service",
			Metadata: entity.EntityMetadata{Name: fmt.Sprintf("svc-%d", i), Namespace: "default", Owner: "platform"},
			Spec: map[string]any{
				"lifecycle": "production",
				"projects":  []any{map[string]any{"kind": "Project", "name": fmt.Sprintf("game-%d", i%10)}},
			},
		})
	}
	made := 0
	for i := 0; i < services && made < deployments; i++ {
		for _, env := range envs {
			if made >= deployments {
				break
			}
			entities = append(entities, &entity.Entity{
				Kind: "Deployment",
				Metadata: entity.EntityMetadata{
					Name: fmt.Sprintf("svc-%d-%s", i, env), Namespace: "default", Owner: "platform",
					Annotations: map[string]string{"gantry.io/migrated-provider": "aws"},
				},
				Spec: map[string]any{
					"target":      map[string]any{"kind": "Service", "name": fmt.Sprintf("svc-%d", i)},
					"environment": map[string]any{"kind": "Environment", "name": env},
					"regions":     []any{"us-east-1"},
					"runtime":     map[string]any{"platform": "ecs"},
				},
			})
			made++
		}
	}
	for i := 0; i < flows; i++ {
		from := i % deployments
		to := (i + 1) % deployments
		entities = append(entities, &entity.Entity{
			Kind: "DataFlow", Metadata: entity.EntityMetadata{Name: fmt.Sprintf("flow-%d", i), Namespace: "default"},
			Spec: map[string]any{
				"source": map[string]any{"kind": "Deployment", "name": fmt.Sprintf("svc-%d-%s", from/len(envs), envs[from%len(envs)])},
				"target": map[string]any{"kind": "Deployment", "name": fmt.Sprintf("svc-%d-%s", to/len(envs), envs[to%len(envs)])},
				"actor":  map[string]any{"kind": "Deployment", "name": fmt.Sprintf("svc-%d-%s", from/len(envs), envs[from%len(envs)])},
				"mode":   "batch", "transport": "public-internet", "dataClasses": []any{"assets"},
			},
		})
	}
	for i := 0; i < highlights; i++ {
		entities = append(entities, &entity.Entity{
			Kind: "Highlight", Metadata: entity.EntityMetadata{Name: fmt.Sprintf("layer-%d", i), Namespace: "default"},
			Spec: map[string]any{
				"color": fmt.Sprintf("mark-%d", (i%6)+1), "defaultOn": i == 0,
				"members": []any{map[string]any{"kind": "Deployment", "name": fmt.Sprintf("svc-%d-%s", i, envs[0])}},
			},
		})
	}
	return entities
}
