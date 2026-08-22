package kind

import "testing"

func TestKindString(t *testing.T) {
	tests := []struct {
		k    Kind
		want string
	}{
		{Nil, "nil"},
		{Boolean, "boolean"},
		{Number, "number"},
		{Integer, "integer"},
		{String, "string"},
		{Any, "any"},
		{Unknown, "unknown"},
		{Never, "never"},
		{Optional, "optional"},
		{Union, "union"},
		{Function, "function"},
		{Array, "array"},
		{Record, "record"},
	}

	for _, tt := range tests {
		if got := tt.k.String(); got != tt.want {
			t.Errorf("Kind(%d).String() = %q, want %q", tt.k, got, tt.want)
		}
	}
}

func TestKindStringOutOfRange(t *testing.T) {
	k := Kind(9999)
	if got := k.String(); got != "unknown" {
		t.Errorf("out of range Kind.String() = %q, want %q", got, "unknown")
	}
}

func TestKindIsPrimitive(t *testing.T) {
	primitives := []Kind{Nil, Boolean, Number, Integer, String}
	for _, k := range primitives {
		if !k.IsPrimitive() {
			t.Errorf("%s should be primitive", k)
		}
	}

	nonPrimitives := []Kind{Any, Unknown, Never, Union, Function, Array, Record}
	for _, k := range nonPrimitives {
		if k.IsPrimitive() {
			t.Errorf("%s should not be primitive", k)
		}
	}
}

func TestKindIsComposite(t *testing.T) {
	composites := []Kind{Union, Intersection, Tuple, Array, Map, Record, Function}
	for _, k := range composites {
		if !k.IsComposite() {
			t.Errorf("%s should be composite", k)
		}
	}

	nonComposites := []Kind{Nil, Boolean, Number, Any, Ref, TypeParam}
	for _, k := range nonComposites {
		if k.IsComposite() {
			t.Errorf("%s should not be composite", k)
		}
	}
}

func TestKindIsDeferred(t *testing.T) {
	deferred := []Kind{Ref, TypeParam, TypeVar, FieldAccess, IndexAccess}
	for _, k := range deferred {
		if !k.IsDeferred() {
			t.Errorf("%s should be deferred", k)
		}
	}

	nonDeferred := []Kind{Nil, Number, Function, Array, Union}
	for _, k := range nonDeferred {
		if k.IsDeferred() {
			t.Errorf("%s should not be deferred", k)
		}
	}
}

func TestKindValuesUnique(t *testing.T) {
	seen := make(map[Kind]string)
	kinds := []Kind{
		Nil, Boolean, Number, Integer, String, Any, Unknown, Never,
		Optional, Union, Intersection, Tuple, Function, Array, Map, Record,
		Sum, Interface, Alias, Generic, Instantiated, Platform, Literal,
		Self, Ref, Meta, TypeParam, TypeVar, Refined, FieldAccess, IndexAccess,
	}

	for _, k := range kinds {
		name := k.String()
		if existing, ok := seen[k]; ok {
			t.Errorf("duplicate Kind value: %d used for both %q and %q", k, existing, name)
		}

		seen[k] = name
	}
}

func TestKindNamesComplete(t *testing.T) {
	// Verify all kinds have non-empty names
	for k := Nil; k <= IndexAccess; k++ {
		name := k.String()
		if name == "" {
			t.Errorf("Kind(%d) has no name", k)
		}
	}
}

func TestKindIsPlaceholder(t *testing.T) {
	placeholders := []Kind{Any, Unknown}
	for _, k := range placeholders {
		if !k.IsPlaceholder() {
			t.Errorf("%s should be placeholder", k)
		}
	}

	nonPlaceholders := []Kind{Nil, Boolean, Number, Integer, String, Never, Union, Function, Array, Record, Ref}
	for _, k := range nonPlaceholders {
		if k.IsPlaceholder() {
			t.Errorf("%s should not be placeholder", k)
		}
	}
}

func TestKindIsConcrete(t *testing.T) {
	concrete := []Kind{Nil, Boolean, Number, Integer, String, Union, Function, Array, Record}
	for _, k := range concrete {
		if !k.IsConcrete() {
			t.Errorf("%s should be concrete", k)
		}
	}

	nonConcrete := []Kind{Any, Unknown, Never}
	for _, k := range nonConcrete {
		if k.IsConcrete() {
			t.Errorf("%s should not be concrete", k)
		}
	}
}

func TestKindIsNever(t *testing.T) {
	if !Never.IsNever() {
		t.Error("Never should return true for IsNever()")
	}

	nonNever := []Kind{Nil, Boolean, Number, Any, Unknown, Union, Function}
	for _, k := range nonNever {
		if k.IsNever() {
			t.Errorf("%s should not be Never", k)
		}
	}
}

func TestKindIsTopOrBottom(t *testing.T) {
	topOrBottom := []Kind{Any, Unknown, Never}
	for _, k := range topOrBottom {
		if !k.IsTopOrBottom() {
			t.Errorf("%s should be top-or-bottom", k)
		}
	}

	others := []Kind{Nil, Boolean, Number, Integer, String, Union, Function, Array, Record}
	for _, k := range others {
		if k.IsTopOrBottom() {
			t.Errorf("%s should not be top-or-bottom", k)
		}
	}
}
