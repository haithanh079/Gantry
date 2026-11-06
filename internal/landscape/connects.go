package landscape

import "github.com/go2engle/gantry/internal/entity"

// Connection is a network edge between two deployments.
type Connection struct {
	From     entity.CanonicalRef
	To       entity.CanonicalRef
	Protocol string
	Port     int
	Note     string
	Inferred bool
}

var defaultPorts = map[string]int{
	"postgres":        5432,
	"aurora-postgres": 5432,
	"mysql":           3306,
	"aurora-mysql":    3306,
	"neptune":         8182,
	"redis":           6379,
	"kafka":           9092,
}

// Connections returns declared connectsTo edges plus same-environment edges
// inferred from Service.dependsOn. A declared edge replaces the inferred edge
// for the same deployment pair. DataFlows never become connections.
func (m *Model) Connections() []Connection {
	views := m.Deployments()
	byTargetEnv := map[string]DeploymentView{}
	for _, view := range views {
		byTargetEnv[string(view.Target)+"|"+string(view.Environment)] = view
	}

	var inferred []Connection
	for _, svc := range m.OfKind("Service") {
		for _, dep := range entity.SpecRefs(svc.Spec, "dependsOn") {
			targetID := entity.ResolveRef(svc, dep)
			for _, pair := range deploymentsOf(views, entity.IDOf(svc)) {
				envID, from := pair.env, pair.view
				to, ok := byTargetEnv[string(targetID)+"|"+string(envID)]
				if !ok || from.ID == to.ID {
					continue
				}
				port := 0
				if target := m.Get(targetID); target != nil && target.Kind == "Infrastructure" {
					port = defaultPorts[specString(target.Spec, "engine")]
				}
				inferred = append(inferred, Connection{
					From: from.ID, To: to.ID, Protocol: "tcp", Port: port, Inferred: true,
				})
			}
		}
	}

	declaredPairs := map[string]bool{}
	var declared []Connection
	for _, view := range views {
		for _, item := range specSlice(view.Entity.Spec, "connectsTo") {
			obj, _ := item.(map[string]any)
			ref, ok := specRef(obj, "deployment")
			if !ok {
				continue
			}
			to := entity.ResolveRef(view.Entity, ref)
			declaredPairs[string(view.ID)+"|"+string(to)] = true
			protocol := specString(obj, "protocol")
			if protocol == "" {
				protocol = "tcp"
			}
			port := 0
			switch n := obj["port"].(type) {
			case int:
				port = n
			case float64:
				port = int(n)
			}
			note, _ := obj["note"].(string)
			declared = append(declared, Connection{
				From: view.ID, To: to, Protocol: protocol, Port: port, Note: note,
			})
		}
	}

	out := declared
	for _, edge := range inferred {
		if declaredPairs[string(edge.From)+"|"+string(edge.To)] {
			continue
		}
		out = append(out, edge)
	}
	return out
}

func deploymentsOf(views []DeploymentView, target entity.CanonicalRef) []struct {
	env  entity.CanonicalRef
	view DeploymentView
} {
	var out []struct {
		env  entity.CanonicalRef
		view DeploymentView
	}
	for _, view := range views {
		if view.Target == target {
			out = append(out, struct {
				env  entity.CanonicalRef
				view DeploymentView
			}{env: view.Environment, view: view})
		}
	}
	return out
}
