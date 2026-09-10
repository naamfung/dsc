package typ

import "testing"

func TestVisit_AnnotatedDispatchesToInner(t *testing.T) {
	ann := NewAnnotated(NewMap(String, Any), []Annotation{{Name: "x"}})
	got := Visit(ann, Visitor[string]{
		Map: func(*Map) string { return "map" },
		Default: func(Type) string {
			return "default"
		},
	})
	if got != "map" {
		t.Fatalf("expected map visitor, got %q", got)
	}
}

func TestVisit_AnnotatedAliasDispatchesToAlias(t *testing.T) {
	alias := NewAlias("T", String)
	ann := NewAnnotated(alias, []Annotation{{Name: "x"}})
	got := Visit(ann, Visitor[string]{
		Alias: func(*Alias) string { return "alias" },
		Default: func(Type) string {
			return "default"
		},
	})
	if got != "alias" {
		t.Fatalf("expected alias visitor, got %q", got)
	}
}

func TestVisit_AnnotatedNilInnerFallsBackToDefault(t *testing.T) {
	ann := &Annotated{Inner: nil, Annotations: []Annotation{{Name: "x"}}}
	got := Visit(ann, Visitor[string]{
		Default: func(Type) string {
			return "default"
		},
	})
	if got != "default" {
		t.Fatalf("expected default visitor, got %q", got)
	}
}
