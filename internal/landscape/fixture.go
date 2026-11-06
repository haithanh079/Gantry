package landscape

import "github.com/go2engle/gantry/internal/entity"

// FixtureCatalog is the representative catalog used to check landscape migration.
// It is the shape described by the landscape spec: 22 deployedIn entries and no
// Deployment entities yet. The catalog YAML itself is not stored in this repo.
func FixtureCatalog() []*entity.Entity {
	envs := []struct {
		name, typ, provider, region string
	}{
		{"local", "development", "docker", "local"},
		{"staging", "staging", "aws", "us-east-1"},
		{"production", "production", "aws", "us-east-1"},
	}
	var entities []*entity.Entity
	for _, env := range envs {
		entities = append(entities, &entity.Entity{
			Kind:     "Environment",
			Metadata: entity.EntityMetadata{Name: env.name, Namespace: "default"},
			Spec:     map[string]any{"type": env.typ, "provider": env.provider, "region": env.region},
		})
	}
	type item struct {
		kind, name, system, owner string
		envs                      []string
		extra                     map[string]any
	}
	items := []item{
		{kind: "Service", name: "nakama", system: "Nakama", owner: "platform", envs: []string{"local", "staging", "production"}},
		{kind: "Service", name: "upe-service", system: "UPE", owner: "upe", envs: []string{"staging", "production"}},
		{kind: "Service", name: "upe-admin", system: "UPE", owner: "upe", envs: []string{"staging", "production"}},
		{kind: "Infrastructure", name: "aurora", system: "UPE", owner: "upe", envs: []string{"staging", "production"}, extra: map[string]any{"type": "database", "engine": "aurora-postgres"}},
		{kind: "Infrastructure", name: "dynamodb-upe", system: "UPE", owner: "upe", envs: []string{"staging", "production"}, extra: map[string]any{"type": "database", "engine": "dynamodb"}},
		{kind: "Infrastructure", name: "msk", system: "UPE", owner: "upe", envs: []string{"staging", "production"}, extra: map[string]any{"type": "queue", "engine": "kafka"}},
		{kind: "Infrastructure", name: "neptune", system: "UPE", owner: "upe", envs: []string{"staging", "production"}, extra: map[string]any{"type": "graph-db", "engine": "neptune"}},
		{kind: "Infrastructure", name: "s3-app-storage", system: "UPE", owner: "upe", envs: []string{"staging", "production"}, extra: map[string]any{"type": "bucket", "engine": "s3"}},
		{kind: "Infrastructure", name: "s3-ecs-env", system: "UPE", owner: "upe", envs: []string{"staging", "production"}, extra: map[string]any{"type": "bucket", "engine": "s3"}},
		{kind: "Infrastructure", name: "s3-telemetry", system: "UPE", owner: "upe", envs: []string{"staging", "production"}, extra: map[string]any{"type": "bucket", "engine": "s3"}},
		{kind: "Infrastructure", name: "postgres-local", system: "Nakama", owner: "platform", envs: []string{"local"}, extra: map[string]any{"type": "database", "engine": "postgres"}},
	}
	for _, item := range items {
		spec := map[string]any{"system": item.system}
		for k, v := range item.extra {
			spec[k] = v
		}
		deployed := make([]any, 0, len(item.envs))
		for _, env := range item.envs {
			deployed = append(deployed, map[string]any{"kind": "Environment", "name": env, "namespace": "default"})
		}
		spec["deployedIn"] = deployed
		if item.kind == "Service" {
			spec["type"] = "backend"
			spec["lifecycle"] = "production"
		}
		entities = append(entities, &entity.Entity{
			Kind: item.kind,
			Metadata: entity.EntityMetadata{
				Name: item.name, Namespace: "default", Owner: item.owner,
			},
			Spec: spec,
		})
	}
	return entities
}
