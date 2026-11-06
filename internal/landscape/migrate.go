package landscape

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/go2engle/gantry/internal/entity"
)

// Change is one entity a landscape migration would create or update.
type Change struct {
	Action string         `json:"action"`
	Entity *entity.Entity `json:"entity"`
}

// MigrationPlan is the dry-run result of gantry migrate landscape.
type MigrationPlan struct {
	Changes []Change `json:"changes"`
}

// PlanMigration turns each deployedIn into a Deployment and proposes a Project per system.
func PlanMigration(m *Model) MigrationPlan {
	var plan MigrationPlan
	projects := map[string]*entity.Entity{}
	for _, e := range m.Entities {
		if e.Kind != "Service" && e.Kind != "Infrastructure" {
			continue
		}
		system := specString(e.Spec, "system")
		var projectRef entity.Ref
		if system != "" {
			name := slug(system)
			projectRef = entity.Ref{Kind: "Project", Name: name, Namespace: namespaceOf(e)}
			key := string(entity.ResolveRef(e, projectRef))
			if projects[key] == nil && m.Get(entity.CanonicalRef(key)) == nil {
				projects[key] = &entity.Entity{
					Kind: "Project",
					Metadata: entity.EntityMetadata{
						Name: name, Namespace: namespaceOf(e), Title: system,
					},
					Spec: map[string]any{"type": "internal-tool", "status": "live"},
				}
			}
		}
		if system != "" && len(entity.SpecRefs(e.Spec, "projects")) == 0 {
			updated := cloneEntity(e)
			updated.Spec["projects"] = []any{map[string]any{"kind": projectRef.Kind, "name": projectRef.Name, "namespace": projectRef.Namespace}}
			plan.Changes = append(plan.Changes, Change{Action: "update", Entity: updated})
		}
		for _, envRef := range entity.SpecRefs(e.Spec, "deployedIn") {
			if envRef.Kind == "" {
				envRef.Kind = "Environment"
			}
			envID := entity.ResolveRef(e, envRef)
			env := m.Get(envID)
			envName := envRef.Name
			if env != nil {
				envName = env.Metadata.Name
			}
			name := fmt.Sprintf("%s-%s", e.Metadata.Name, envName)
			dep := &entity.Entity{
				Kind: "Deployment",
				Metadata: entity.EntityMetadata{
					Name: name, Namespace: namespaceOf(e), Owner: e.Metadata.Owner,
					Annotations: map[string]string{},
				},
				Spec: map[string]any{
					"target":      map[string]any{"kind": e.Kind, "name": e.Metadata.Name, "namespace": namespaceOf(e)},
					"environment": map[string]any{"kind": "Environment", "name": envName, "namespace": envNamespace(env, e, envRef)},
				},
			}
			if env != nil {
				if region := specString(env.Spec, "region"); region != "" {
					dep.Spec["regions"] = []any{region}
				}
				if provider := specString(env.Spec, "provider"); provider != "" {
					dep.Metadata.Annotations["gantry.io/migrated-provider"] = strings.ToLower(provider)
				}
			}
			if m.Get(entity.IDOf(dep)) != nil {
				continue
			}
			plan.Changes = append(plan.Changes, Change{Action: "create", Entity: dep})
		}
	}
	for _, project := range projects {
		plan.Changes = append(plan.Changes, Change{Action: "create", Entity: project})
	}
	return plan
}

// DeploymentCreates counts Deployment entities the plan would create.
func (p MigrationPlan) DeploymentCreates() int {
	n := 0
	for _, change := range p.Changes {
		if change.Action == "create" && change.Entity != nil && change.Entity.Kind == "Deployment" {
			n++
		}
	}
	return n
}

func namespaceOf(e *entity.Entity) string {
	if e.Metadata.Namespace == "" {
		return entity.DefaultNamespace
	}
	return e.Metadata.Namespace
}

func envNamespace(env *entity.Entity, owner *entity.Entity, ref entity.Ref) string {
	if ref.Namespace != "" {
		return ref.Namespace
	}
	if env != nil && env.Metadata.Namespace != "" {
		return env.Metadata.Namespace
	}
	return namespaceOf(owner)
}

func cloneEntity(e *entity.Entity) *entity.Entity {
	copy := *e
	copy.Metadata.Annotations = cloneMap(e.Metadata.Annotations)
	copy.Metadata.Labels = cloneMap(e.Metadata.Labels)
	copy.Spec = map[string]any{}
	for k, v := range e.Spec {
		copy.Spec[k] = v
	}
	return &copy
}

func cloneMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := map[string]string{}
	for k, v := range in {
		out[k] = v
	}
	return out
}

var nonSlug = regexp.MustCompile(`[^a-z0-9.-]+`)

func slug(value string) string {
	s := strings.ToLower(strings.TrimSpace(value))
	s = strings.ReplaceAll(s, " ", "-")
	s = nonSlug.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-.")
	if s == "" {
		return "project"
	}
	return s
}
