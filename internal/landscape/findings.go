package landscape

import (
	"strings"

	"github.com/go2engle/gantry/internal/entity"
)

// Finding is one open landscape finding before it is stored.
type Finding struct {
	Rule     string
	Category string
	Severity string
	Subject  entity.CanonicalRef
	Details  map[string]any
}

// Rule evaluates one finding rule against a model.
type Rule struct {
	ID       string
	Category string
	Severity string
	Eval     func(*Model) []Finding
}

// PlanARules are the catalog-only rules. Later plans register more.
func PlanARules() []Rule {
	return []Rule{
		{ID: "missing-owner", Category: "finding", Severity: "warning", Eval: missingOwner},
		{ID: "dangling-ref", Category: "finding", Severity: "warning", Eval: danglingRef},
		{ID: "cross-env-dependency", Category: "finding", Severity: "error", Eval: crossEnvDependency},
		{ID: "dataflow-sensitive-to-lower-env", Category: "finding", Severity: "error", Eval: sensitiveToLowerEnv},
		{ID: "dataflow-sensitive-public", Category: "finding", Severity: "warning", Eval: sensitivePublic},
		{ID: "dataflow-no-actor", Category: "finding", Severity: "info", Eval: dataFlowNoActor},
	}
}

// Evaluate runs every rule and stamps rule metadata onto each finding.
func Evaluate(m *Model, rules []Rule) map[string][]Finding {
	out := map[string][]Finding{}
	for _, rule := range rules {
		found := rule.Eval(m)
		for i := range found {
			found[i].Rule = rule.ID
			found[i].Category = rule.Category
			found[i].Severity = rule.Severity
			if found[i].Details == nil {
				found[i].Details = map[string]any{}
			}
		}
		out[rule.ID] = found
	}
	return out
}

func missingOwner(m *Model) []Finding {
	var out []Finding
	for _, e := range m.Entities {
		if e.Kind != "Service" && e.Kind != "Deployment" {
			continue
		}
		if strings.TrimSpace(e.Metadata.Owner) != "" {
			continue
		}
		out = append(out, Finding{Subject: entity.IDOf(e)})
	}
	return out
}

func danglingRef(m *Model) []Finding {
	var out []Finding
	for _, item := range m.Dangling {
		out = append(out, Finding{Subject: item.Subject, Details: map[string]any{"field": item.Field, "target": string(item.Target)}})
	}
	return out
}

func crossEnvDependency(m *Model) []Finding {
	var out []Finding
	views := map[entity.CanonicalRef]DeploymentView{}
	for _, view := range m.Deployments() {
		views[view.ID] = view
	}
	for _, view := range m.Deployments() {
		for _, item := range specSlice(view.Entity.Spec, "connectsTo") {
			obj, _ := item.(map[string]any)
			ref, ok := specRef(obj, "deployment")
			if !ok {
				continue
			}
			to := entity.ResolveRef(view.Entity, ref)
			target, ok := views[to]
			if !ok {
				continue
			}
			if isProd(view.EnvType) && isLower(target.EnvType) {
				out = append(out, Finding{Subject: view.ID, Details: map[string]any{"to": string(to), "fromEnv": view.EnvType, "toEnv": target.EnvType}})
			}
		}
	}
	return out
}

func sensitiveToLowerEnv(m *Model) []Finding {
	var out []Finding
	for _, flow := range m.OfKind("DataFlow") {
		if !sensitive(flow) {
			continue
		}
		sourceEnvs := endpointEnvTypes(m, flow, "source")
		targetEnvs := endpointEnvTypes(m, flow, "target")
		if anyProd(sourceEnvs) && anyLower(targetEnvs) {
			out = append(out, Finding{Subject: entity.IDOf(flow), Details: map[string]any{"dataClasses": specStrings(flow.Spec, "dataClasses")}})
		}
	}
	return out
}

func sensitivePublic(m *Model) []Finding {
	var out []Finding
	for _, flow := range m.OfKind("DataFlow") {
		if sensitive(flow) && specString(flow.Spec, "transport") == "public-internet" {
			out = append(out, Finding{Subject: entity.IDOf(flow)})
		}
	}
	return out
}

func dataFlowNoActor(m *Model) []Finding {
	var out []Finding
	for _, flow := range m.OfKind("DataFlow") {
		if specString(flow.Spec, "mode") == "replication" {
			continue
		}
		if _, ok := specRef(flow.Spec, "actor"); ok {
			continue
		}
		out = append(out, Finding{Subject: entity.IDOf(flow), Details: map[string]any{"mode": specString(flow.Spec, "mode")}})
	}
	return out
}

func sensitive(flow *entity.Entity) bool {
	for _, class := range specStrings(flow.Spec, "dataClasses") {
		if class == "pii" || class == "payment" {
			return true
		}
	}
	return false
}

func endpointEnvTypes(m *Model, flow *entity.Entity, field string) []string {
	ref, ok := specRef(flow.Spec, field)
	if !ok {
		return nil
	}
	id := entity.ResolveRef(flow, ref)
	if view, ok := m.deploymentByID(id); ok {
		return []string{view.EnvType}
	}
	var types []string
	for _, view := range m.Deployments() {
		if view.Target == id && view.EnvType != "" {
			types = append(types, view.EnvType)
		}
	}
	return types
}

func isProd(envType string) bool { return envType == "production" }

func isLower(envType string) bool {
	switch envType {
	case "development", "qc", "sandbox":
		return true
	default:
		return false
	}
}

func anyProd(types []string) bool {
	for _, t := range types {
		if isProd(t) {
			return true
		}
	}
	return false
}

func anyLower(types []string) bool {
	for _, t := range types {
		if isLower(t) {
			return true
		}
	}
	return false
}
