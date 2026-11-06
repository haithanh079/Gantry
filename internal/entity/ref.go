package entity

import (
	"fmt"
	"strings"
)

// Ref is a structured pointer to another catalog entity.
// An empty Namespace means "inherit from the entity that contains this ref".
type Ref struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

// CanonicalRef is the string form used in DB columns, edge ids, APIs, and URLs:
// Kind/namespace/name.
type CanonicalRef string

func (c CanonicalRef) String() string { return string(c) }

// ResolveRef builds a canonical ref. Namespace fallback order is the ref's own
// namespace, then the container entity's namespace, then DefaultNamespace.
func ResolveRef(container *Entity, ref Ref) CanonicalRef {
	ns := strings.TrimSpace(ref.Namespace)
	if ns == "" && container != nil {
		ns = strings.TrimSpace(container.Metadata.Namespace)
	}
	if ns == "" {
		ns = DefaultNamespace
	}
	return CanonicalRef(ref.Kind + "/" + ns + "/" + ref.Name)
}

// IDOf returns the canonical ref of an entity itself.
func IDOf(e *Entity) CanonicalRef {
	if e == nil {
		return ""
	}
	return ResolveRef(nil, Ref{Kind: e.Kind, Name: e.Metadata.Name, Namespace: e.Metadata.Namespace})
}

// ParseCanonicalRef parses Kind/namespace/name. Names and namespaces cannot
// contain slashes (they are URL-safe slugs).
func ParseCanonicalRef(s string) (Ref, error) {
	parts := strings.Split(s, "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return Ref{}, fmt.Errorf("canonical ref %q must be Kind/namespace/name", s)
	}
	return Ref{Kind: parts[0], Namespace: parts[1], Name: parts[2]}, nil
}

// RefFromSpec accepts an object ref ({kind,name,namespace?}) or a legacy
// "kind/name" string. A string that already has three segments is treated as canonical.
func RefFromSpec(v any) (Ref, bool) {
	switch t := v.(type) {
	case map[string]any:
		kind, _ := t["kind"].(string)
		name, _ := t["name"].(string)
		ns, _ := t["namespace"].(string)
		kind = strings.TrimSpace(kind)
		name = strings.TrimSpace(name)
		if kind == "" || name == "" {
			return Ref{}, false
		}
		return Ref{Kind: kind, Name: name, Namespace: strings.TrimSpace(ns)}, true
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return Ref{}, false
		}
		if ref, err := ParseCanonicalRef(s); err == nil {
			return ref, true
		}
		kind, name, ok := strings.Cut(s, "/")
		if !ok || kind == "" || name == "" || strings.Contains(name, "/") {
			return Ref{}, false
		}
		return Ref{Kind: kind, Name: name}, true
	default:
		return Ref{}, false
	}
}

// SpecRefs reads a spec array of object refs (dependsOn, deployedIn, projects, …).
func SpecRefs(spec map[string]any, field string) []Ref {
	if spec == nil {
		return nil
	}
	raw, ok := spec[field]
	if !ok {
		return nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	var out []Ref
	for _, item := range items {
		if ref, ok := RefFromSpec(item); ok {
			out = append(out, ref)
		}
	}
	return out
}
