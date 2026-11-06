package landscape

import (
	"context"
	"strings"

	"github.com/go2engle/gantry/internal/entity"
)

// Lister lists catalog entities. Empty kind and namespace mean "no filter".
type Lister interface {
	ListEntities(ctx context.Context, kind, namespace string) ([]*entity.Entity, error)
}

// AccessFilter drops entities the caller may not read. Nil keeps every entity.
type AccessFilter func([]*entity.Entity) []*entity.Entity

// DanglingRef is a ref that does not resolve to an entity in the model.
type DanglingRef struct {
	Subject entity.CanonicalRef
	Field   string
	Target  entity.CanonicalRef
}

// Model is an in-memory catalog indexed by canonical ref.
type Model struct {
	Entities []*entity.Entity
	ByID     map[entity.CanonicalRef]*entity.Entity
	Dangling []DanglingRef
}

// LoadModel lists entities, applies the access filter, and indexes them.
func LoadModel(ctx context.Context, lister Lister, filter AccessFilter) (*Model, error) {
	entities, err := lister.ListEntities(ctx, "", "")
	if err != nil {
		return nil, err
	}
	if filter != nil {
		entities = filter(entities)
	}
	return Load(entities), nil
}

// Load indexes an already-filtered entity slice.
func Load(entities []*entity.Entity) *Model {
	m := &Model{ByID: map[entity.CanonicalRef]*entity.Entity{}}
	for _, e := range entities {
		if e == nil {
			continue
		}
		m.Entities = append(m.Entities, e)
		m.ByID[entity.IDOf(e)] = e
	}
	m.collectDangling()
	return m
}

func (m *Model) Get(id entity.CanonicalRef) *entity.Entity {
	if m == nil {
		return nil
	}
	return m.ByID[id]
}

func (m *Model) OfKind(kind string) []*entity.Entity {
	var out []*entity.Entity
	for _, e := range m.Entities {
		if e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}

func (m *Model) resolve(container *entity.Entity, ref entity.Ref) (entity.CanonicalRef, *entity.Entity) {
	id := entity.ResolveRef(container, ref)
	return id, m.Get(id)
}

func (m *Model) noteDangling(subject *entity.Entity, field string, ref entity.Ref) {
	id, target := m.resolve(subject, ref)
	if target != nil {
		return
	}
	m.Dangling = append(m.Dangling, DanglingRef{Subject: entity.IDOf(subject), Field: field, Target: id})
}

func (m *Model) collectDangling() {
	refFields := []string{"dependsOn", "deployedIn", "projects", "department"}
	for _, e := range m.Entities {
		for _, field := range refFields {
			if field == "department" {
				if ref, ok := specRef(e.Spec, "department"); ok {
					m.noteDangling(e, field, ref)
				}
				continue
			}
			for _, ref := range entity.SpecRefs(e.Spec, field) {
				m.noteDangling(e, field, ref)
			}
		}
		switch e.Kind {
		case "Deployment":
			for _, field := range []string{"target", "environment", "cloudAccount", "network"} {
				if ref, ok := specRef(e.Spec, field); ok {
					m.noteDangling(e, field, ref)
				}
			}
			for _, item := range specSlice(e.Spec, "connectsTo") {
				obj, _ := item.(map[string]any)
				if ref, ok := specRef(obj, "deployment"); ok {
					m.noteDangling(e, "connectsTo", ref)
				}
			}
		case "DataFlow":
			for _, field := range []string{"source", "target", "actor"} {
				if ref, ok := specRef(e.Spec, field); ok {
					m.noteDangling(e, field, ref)
				}
			}
		case "Highlight":
			for _, ref := range entity.SpecRefs(e.Spec, "members") {
				m.noteDangling(e, "members", ref)
			}
		}
	}
}

func specRef(spec map[string]any, field string) (entity.Ref, bool) {
	if spec == nil {
		return entity.Ref{}, false
	}
	return entity.RefFromSpec(spec[field])
}

func specSlice(spec map[string]any, field string) []any {
	if spec == nil {
		return nil
	}
	items, _ := spec[field].([]any)
	return items
}

func specString(spec map[string]any, field string) string {
	if spec == nil {
		return ""
	}
	s, _ := spec[field].(string)
	return s
}

func specStrings(spec map[string]any, field string) []string {
	var out []string
	for _, item := range specSlice(spec, field) {
		if s, ok := item.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

func specBool(spec map[string]any, field string) bool {
	if spec == nil {
		return false
	}
	b, _ := spec[field].(bool)
	return b
}

// DeploymentView is a deployment plus the catalog axes the landscape reads.
type DeploymentView struct {
	Entity         *entity.Entity
	ID             entity.CanonicalRef
	Target         entity.CanonicalRef
	TargetKind     string
	Environment    entity.CanonicalRef
	EnvName        string
	EnvType        string
	Cloud          string
	Regions        []string
	Runtime        string
	Owner          string
	Lifecycle      string
	Project        entity.CanonicalRef
	ProjectName    string
	Department     entity.CanonicalRef
	DepartmentName string
}

// Deployments returns one view per Deployment entity.
func (m *Model) Deployments() []DeploymentView {
	var out []DeploymentView
	for _, e := range m.OfKind("Deployment") {
		view := DeploymentView{Entity: e, ID: entity.IDOf(e), Owner: e.Metadata.Owner, Regions: specStrings(e.Spec, "regions")}
		if ref, ok := specRef(e.Spec, "target"); ok {
			view.Target, _ = m.resolve(e, ref)
			view.TargetKind = ref.Kind
		}
		if ref, ok := specRef(e.Spec, "environment"); ok {
			id, env := m.resolve(e, ref)
			view.Environment = id
			view.EnvName = ref.Name
			if env != nil {
				view.EnvName = env.Metadata.Name
				view.EnvType = specString(env.Spec, "type")
			}
		}
		if ref, ok := specRef(e.Spec, "cloudAccount"); ok {
			_, account := m.resolve(e, ref)
			if account != nil {
				view.Cloud = specString(account.Spec, "provider")
			}
		}
		if view.Cloud == "" && e.Metadata.Annotations != nil {
			view.Cloud = e.Metadata.Annotations["gantry.io/migrated-provider"]
		}
		if runtime, ok := e.Spec["runtime"].(map[string]any); ok {
			view.Runtime, _ = runtime["platform"].(string)
		}
		if target := m.Get(view.Target); target != nil {
			if view.Owner == "" {
				view.Owner = target.Metadata.Owner
			}
			view.Lifecycle = specString(target.Spec, "lifecycle")
			if projects := entity.SpecRefs(target.Spec, "projects"); len(projects) > 0 {
				id, project := m.resolve(target, projects[0])
				view.Project = id
				view.ProjectName = projects[0].Name
				if project != nil {
					view.ProjectName = project.Metadata.Name
					if dref, ok := specRef(project.Spec, "department"); ok {
						did, dept := m.resolve(project, dref)
						view.Department = did
						view.DepartmentName = dref.Name
						if dept != nil {
							view.DepartmentName = dept.Metadata.Name
						}
					}
				}
			}
		}
		if view.DepartmentName == "" {
			ns := e.Metadata.Namespace
			if ns == "" {
				ns = entity.DefaultNamespace
			}
			view.DepartmentName = ns
			if dept := m.departmentByNamespace(ns); dept != nil {
				view.Department = entity.IDOf(dept)
				view.DepartmentName = dept.Metadata.Name
			}
		}
		out = append(out, view)
	}
	return out
}

func (m *Model) departmentByNamespace(ns string) *entity.Entity {
	for _, e := range m.OfKind("Department") {
		if e.Metadata.Name == ns || e.Metadata.Namespace == ns {
			return e
		}
	}
	return nil
}

func (m *Model) deploymentByID(id entity.CanonicalRef) (DeploymentView, bool) {
	for _, view := range m.Deployments() {
		if view.ID == id {
			return view, true
		}
	}
	return DeploymentView{}, false
}

func containsFold(list []string, want string) bool {
	for _, item := range list {
		if strings.EqualFold(item, want) {
			return true
		}
	}
	return false
}
