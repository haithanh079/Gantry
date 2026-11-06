package entity

import "testing"

func TestResolveRefNamespaceFallback(t *testing.T) {
	container := &Entity{Kind: "Service", Metadata: EntityMetadata{Name: "nakama", Namespace: "music"}}
	tests := []struct {
		name string
		ref  Ref
		want CanonicalRef
	}{
		{name: "explicit namespace", ref: Ref{Kind: "Infrastructure", Name: "aurora", Namespace: "platform"}, want: "Infrastructure/platform/aurora"},
		{name: "inherit container", ref: Ref{Kind: "Infrastructure", Name: "aurora"}, want: "Infrastructure/music/aurora"},
		{name: "default when container missing", ref: Ref{Kind: "Team", Name: "personalize"}, want: "Team/default/personalize"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := container
			if tt.name == "default when container missing" {
				c = nil
			}
			if got := ResolveRef(c, tt.ref); got != tt.want {
				t.Fatalf("ResolveRef() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseCanonicalRef(t *testing.T) {
	ref, err := ParseCanonicalRef("Deployment/music/nakama-production")
	if err != nil {
		t.Fatal(err)
	}
	if ref.Kind != "Deployment" || ref.Namespace != "music" || ref.Name != "nakama-production" {
		t.Fatalf("parsed %#v", ref)
	}
	if _, err := ParseCanonicalRef("Service/checkout"); err == nil {
		t.Fatal("expected error for legacy two-part ref")
	}
}

func TestRefFromSpec(t *testing.T) {
	obj, ok := RefFromSpec(map[string]any{"kind": "Service", "name": "nakama", "namespace": "music"})
	if !ok || obj.Namespace != "music" {
		t.Fatalf("object ref %#v ok=%v", obj, ok)
	}
	legacy, ok := RefFromSpec("Infrastructure/aurora")
	if !ok || legacy.Kind != "Infrastructure" || legacy.Name != "aurora" || legacy.Namespace != "" {
		t.Fatalf("legacy ref %#v ok=%v", legacy, ok)
	}
	canon, ok := RefFromSpec("Deployment/music/nakama-production")
	if !ok || canon.Namespace != "music" || canon.Name != "nakama-production" {
		t.Fatalf("canonical string %#v ok=%v", canon, ok)
	}
	if _, ok := RefFromSpec(map[string]any{"kind": "Service"}); ok {
		t.Fatal("missing name should not parse")
	}
}

func TestIDOf(t *testing.T) {
	e := &Entity{Kind: "Service", Metadata: EntityMetadata{Name: "nakama"}}
	if got := IDOf(e); got != "Service/default/nakama" {
		t.Fatalf("IDOf empty namespace = %q", got)
	}
}
