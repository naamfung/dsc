package contract

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/typ"
)

func TestNewSpec(t *testing.T) {
	spec := NewSpec()
	if spec == nil {
		t.Fatal("NewSpec returned nil")
	}

	if !spec.Effects.Pure() {
		t.Error("Effects should be pure (empty)")
	}

	if spec.Callbacks == nil {
		t.Error("Callbacks map should be initialized")
	}

	if len(spec.Requires.AllConstraints()) != 0 {
		t.Error("Requires should be empty")
	}

	if len(spec.Ensures.AllConstraints()) != 0 {
		t.Error("Ensures should be empty")
	}
}

func TestSpecWithRequires(t *testing.T) {
	t.Run("single predicate", func(t *testing.T) {
		spec := NewSpec().WithRequires(constraint.IsNil{Path: root("x")})
		if len(spec.Requires.AllConstraints()) != 1 {
			t.Errorf("Expected 1 require, got %d", len(spec.Requires.AllConstraints()))
		}
	})

	t.Run("multiple predicates", func(t *testing.T) {
		spec := NewSpec().WithRequires(
			constraint.IsNil{Path: root("x")},
			constraint.NotNil{Path: root("y")},
		)
		if len(spec.Requires.AllConstraints()) != 2 {
			t.Errorf("Expected 2 requires, got %d", len(spec.Requires.AllConstraints()))
		}
	})

	t.Run("chained calls", func(t *testing.T) {
		spec := NewSpec().
			WithRequires(constraint.IsNil{Path: root("x")}).
			WithRequires(constraint.NotNil{Path: root("y")})
		if len(spec.Requires.AllConstraints()) != 2 {
			t.Errorf("Expected 2 requires, got %d", len(spec.Requires.AllConstraints()))
		}
	})

	t.Run("returns same spec", func(t *testing.T) {
		spec := NewSpec()
		result := spec.WithRequires(constraint.IsNil{Path: root("x")})

		if result != spec {
			t.Error("WithRequires should return the same spec for chaining")
		}
	})
}

func TestSpecWithEnsures(t *testing.T) {
	t.Run("single predicate", func(t *testing.T) {
		spec := NewSpec().WithEnsures(constraint.NotNil{Path: root("ret[0]")})
		if len(spec.Ensures.AllConstraints()) != 1 {
			t.Errorf("Expected 1 ensure, got %d", len(spec.Ensures.AllConstraints()))
		}
	})

	t.Run("multiple predicates", func(t *testing.T) {
		spec := NewSpec().WithEnsures(
			constraint.NotNil{Path: root("ret[0]")},
			constraint.IsNil{Path: root("ret[1]")},
		)
		if len(spec.Ensures.AllConstraints()) != 2 {
			t.Errorf("Expected 2 ensures, got %d", len(spec.Ensures.AllConstraints()))
		}
	})

	t.Run("chained calls", func(t *testing.T) {
		spec := NewSpec().
			WithEnsures(constraint.NotNil{Path: root("ret[0]")}).
			WithEnsures(constraint.IsNil{Path: root("ret[1]")})
		if len(spec.Ensures.AllConstraints()) != 2 {
			t.Errorf("Expected 2 ensures, got %d", len(spec.Ensures.AllConstraints()))
		}
	})

	t.Run("returns same spec", func(t *testing.T) {
		spec := NewSpec()
		result := spec.WithEnsures(constraint.NotNil{Path: root("x")})

		if result != spec {
			t.Error("WithEnsures should return the same spec for chaining")
		}
	})
}

func TestSpecWithEffects(t *testing.T) {
	t.Run("single effect", func(t *testing.T) {
		spec := NewSpec().WithEffects(effect.Mutate{Target: effect.ParamRef{Index: 0}})
		if spec.Effects.Pure() {
			t.Error("Effects should not be pure")
		}
	})

	t.Run("multiple effects", func(t *testing.T) {
		spec := NewSpec().WithEffects(
			effect.Mutate{Target: effect.ParamRef{Index: 0}},
			effect.Throw{},
		)
		if !spec.Effects.HasMutate() {
			t.Error("Should have mutate effect")
		}

		if !spec.Effects.HasThrow() {
			t.Error("Should have throw effect")
		}
	})

	t.Run("chained calls", func(t *testing.T) {
		spec := NewSpec().
			WithEffects(effect.Mutate{Target: effect.ParamRef{Index: 0}}).
			WithEffects(effect.Throw{})
		if !spec.Effects.HasMutate() {
			t.Error("Should have mutate effect")
		}

		if !spec.Effects.HasThrow() {
			t.Error("Should have throw effect")
		}
	})

	t.Run("returns same spec", func(t *testing.T) {
		spec := NewSpec()
		result := spec.WithEffects(effect.Throw{})

		if result != spec {
			t.Error("WithEffects should return the same spec for chaining")
		}
	})
}

func TestSpecWithEffectRow(t *testing.T) {
	t.Run("set effect row", func(t *testing.T) {
		row := effect.Row{Labels: []effect.Label{effect.Throw{}, effect.IO{}}}

		spec := NewSpec().WithEffectRow(row)
		if !spec.Effects.HasThrow() {
			t.Error("Should have throw effect")
		}

		if !spec.Effects.HasIO() {
			t.Error("Should have IO effect")
		}
	})

	t.Run("union with existing", func(t *testing.T) {
		spec := NewSpec().
			WithEffects(effect.Mutate{Target: effect.ParamRef{Index: 0}}).
			WithEffectRow(effect.Throws())
		if !spec.Effects.HasMutate() {
			t.Error("Should have mutate effect")
		}

		if !spec.Effects.HasThrow() {
			t.Error("Should have throw effect")
		}
	})

	t.Run("returns same spec", func(t *testing.T) {
		spec := NewSpec()
		result := spec.WithEffectRow(effect.Empty)

		if result != spec {
			t.Error("WithEffectRow should return the same spec for chaining")
		}
	})
}

func TestSpecWithCallback(t *testing.T) {
	t.Run("add callback", func(t *testing.T) {
		cb := PredicateSpec(0)
		spec := NewSpec().WithCallback(1, cb)

		if spec.Callbacks[1] != cb {
			t.Error("Callback not stored correctly")
		}
	})

	t.Run("multiple callbacks", func(t *testing.T) {
		cb1 := PredicateSpec(0)
		cb2 := MapperSpec(0)

		spec := NewSpec().
			WithCallback(1, cb1).
			WithCallback(2, cb2)
		if spec.Callbacks[1] != cb1 {
			t.Error("First callback not stored correctly")
		}

		if spec.Callbacks[2] != cb2 {
			t.Error("Second callback not stored correctly")
		}
	})

	t.Run("overwrite callback", func(t *testing.T) {
		cb1 := PredicateSpec(0)
		cb2 := MapperSpec(0)
		spec := NewSpec().
			WithCallback(1, cb1).
			WithCallback(1, cb2)

		if spec.Callbacks[1] != cb2 {
			t.Error("Callback should be overwritten")
		}
	})

	t.Run("returns same spec", func(t *testing.T) {
		spec := NewSpec()
		result := spec.WithCallback(0, PredicateSpec(0))

		if result != spec {
			t.Error("WithCallback should return the same spec for chaining")
		}
	})
}

func TestPredicateSpec(t *testing.T) {
	tests := []struct {
		name       string
		inputParam int
	}{
		{"param 0", 0},
		{"param 1", 1},
		{"param 5", 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := PredicateSpec(tt.inputParam)
			if !spec.ReturnsBoolean {
				t.Error("Predicate spec should return boolean")
			}

			if !spec.Pure {
				t.Error("Predicate spec should be pure")
			}

			if spec.InputSource.Index != tt.inputParam {
				t.Errorf("InputSource.Index = %d, want %d", spec.InputSource.Index, tt.inputParam)
			}

			if spec.Cardinality != CardAtMostOncePerElement {
				t.Error("Cardinality should be CardAtMostOncePerElement")
			}
		})
	}
}

func TestMapperSpec(t *testing.T) {
	tests := []struct {
		name       string
		inputParam int
	}{
		{"param 0", 0},
		{"param 1", 1},
		{"param 5", 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := MapperSpec(tt.inputParam)
			if spec.ReturnsBoolean {
				t.Error("Mapper spec should not return boolean")
			}

			if !spec.Pure {
				t.Error("Mapper spec should be pure")
			}

			if spec.InputSource.Index != tt.inputParam {
				t.Errorf("InputSource.Index = %d, want %d", spec.InputSource.Index, tt.inputParam)
			}

			if spec.Cardinality != CardOncePerElement {
				t.Error("Cardinality should be CardOncePerElement")
			}
		})
	}
}

func TestSpecString(t *testing.T) {
	tests := []struct {
		name     string
		spec     *Spec
		contains []string
	}{
		{
			name:     "nil spec",
			spec:     nil,
			contains: []string{"Spec{}"},
		},
		{
			name:     "empty spec",
			spec:     NewSpec(),
			contains: []string{"requires=0", "ensures=0", "callbacks=0"},
		},
		{
			name:     "spec with requires",
			spec:     NewSpec().WithRequires(constraint.IsNil{Path: root("x")}),
			contains: []string{"requires=1"},
		},
		{
			name:     "spec with ensures",
			spec:     NewSpec().WithEnsures(constraint.NotNil{Path: root("x")}),
			contains: []string{"ensures=1"},
		},
		{
			name:     "spec with callbacks",
			spec:     NewSpec().WithCallback(0, PredicateSpec(0)),
			contains: []string{"callbacks=1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			str := tt.spec.String()
			for _, s := range tt.contains {
				if !strings.Contains(str, s) {
					t.Errorf("String() = %q, should contain %q", str, s)
				}
			}
		})
	}
}

func TestSpecEffectRow(t *testing.T) {
	tests := []struct {
		name     string
		spec     *Spec
		wantPure bool
	}{
		{"nil spec", nil, true},
		{"empty spec", NewSpec(), true},
		{"spec with effects", NewSpec().WithEffects(effect.Throw{}), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.spec.EffectRow()
			if got.Pure() != tt.wantPure {
				t.Errorf("EffectRow().Pure() = %v, want %v", got.Pure(), tt.wantPure)
			}
		})
	}
}

func TestCardinalityString(t *testing.T) {
	tests := []struct {
		card     Cardinality
		expected string
	}{
		{CardOncePerElement, "once_per_element"},
		{CardAtMostOncePerElement, "at_most_once_per_element"},
		{CardExactlyOnce, "exactly_once"},
		{CardAtMostOnce, "at_most_once"},
		{CardUnknown, "unknown"},
		{Cardinality(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.card.String(); got != tt.expected {
				t.Errorf("Cardinality(%d).String() = %q, want %q", tt.card, got, tt.expected)
			}
		})
	}
}

func TestSpecHasMutation(t *testing.T) {
	tests := []struct {
		name     string
		spec     *Spec
		expected bool
	}{
		{"nil spec", nil, false},
		{"empty spec", NewSpec(), false},
		{"with throw effect", NewSpec().WithEffects(effect.Throw{}), false},
		{
			"with mutate effect",
			NewSpec().WithEffects(effect.Mutate{Target: effect.ParamRef{Index: 0}}),
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.spec.HasMutation(); got != tt.expected {
				t.Errorf("HasMutation() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSpecGetMutation(t *testing.T) {
	tests := []struct {
		name    string
		spec    *Spec
		wantNil bool
	}{
		{"nil spec", nil, true},
		{"empty spec", NewSpec(), true},
		{"with throw effect", NewSpec().WithEffects(effect.Throw{}), true},
		{
			"with mutate effect",
			NewSpec().WithEffects(effect.Mutate{Target: effect.ParamRef{Index: 0}}),
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.spec.GetMutation()
			if (got == nil) != tt.wantNil {
				t.Errorf("GetMutation() nil = %v, want nil = %v", got == nil, tt.wantNil)
			}
		})
	}
}

func TestSpecGetMutationAt(t *testing.T) {
	spec := NewSpec().WithEffects(
		effect.Mutate{Target: effect.ParamRef{Index: 0}},
		effect.Mutate{Target: effect.ParamRef{Index: 2}},
	)

	tests := []struct {
		name     string
		spec     *Spec
		paramIdx int
		wantNil  bool
	}{
		{"nil spec", nil, 0, true},
		{"empty spec", NewSpec(), 0, true},
		{"param 0 exists", spec, 0, false},
		{"param 1 missing", spec, 1, true},
		{"param 2 exists", spec, 2, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.spec.GetMutationAt(tt.paramIdx)
			if (got == nil) != tt.wantNil {
				t.Errorf("GetMutationAt(%d) nil = %v, want nil = %v", tt.paramIdx, got == nil, tt.wantNil)
			}
		})
	}
}

func TestSpecGetReturnLength(t *testing.T) {
	spec := NewSpec().WithEffects(
		effect.ReturnLength{ReturnIndex: 0, Length: constraint.Const{Value: 10}},
	)

	tests := []struct {
		name    string
		spec    *Spec
		retIdx  int
		wantNil bool
	}{
		{"nil spec", nil, 0, true},
		{"empty spec", NewSpec(), 0, true},
		{"ret 0 exists", spec, 0, false},
		{"ret 1 missing", spec, 1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.spec.GetReturnLength(tt.retIdx)
			if (got == nil) != tt.wantNil {
				t.Errorf("GetReturnLength(%d) nil = %v, want nil = %v", tt.retIdx, got == nil, tt.wantNil)
			}
		})
	}
}

func TestSpecGetReturnType(t *testing.T) {
	spec := NewSpec().WithEffects(
		effect.Return{ReturnIndex: 0, Transform: effect.ElementOf{Source: effect.ParamRef{Index: 0}}},
	)

	tests := []struct {
		name    string
		spec    *Spec
		retIdx  int
		wantNil bool
	}{
		{"nil spec", nil, 0, true},
		{"empty spec", NewSpec(), 0, true},
		{"ret 0 exists", spec, 0, false},
		{"ret 1 missing", spec, 1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.spec.GetReturnType(tt.retIdx)
			if (got == nil) != tt.wantNil {
				t.Errorf("GetReturnType(%d) nil = %v, want nil = %v", tt.retIdx, got == nil, tt.wantNil)
			}
		})
	}
}

func TestSpecReturnCases(t *testing.T) {
	spec := NewSpec().
		WithReturnCase(constraint.FromConstraints(
			constraint.FieldEquals{
				Target: root("param[1]"),
				Field:  "message",
				Value:  typ.LiteralBool(true),
			},
		), typ.String).
		WithDefaultReturn(typ.Any)

	cases := spec.GetReturnCases()
	if len(cases) != 1 {
		t.Fatalf("expected 1 return case, got %d", len(cases))
	}

	if spec.GetReturnDefault() != typ.Any {
		t.Fatalf("expected default return any, got %v", spec.GetReturnDefault())
	}
}

func TestSpecGetCallback(t *testing.T) {
	cb := PredicateSpec(0)
	spec := NewSpec().WithCallback(1, cb)

	tests := []struct {
		name     string
		spec     *Spec
		paramIdx int
		wantNil  bool
	}{
		{"nil spec", nil, 0, true},
		{"empty spec", NewSpec(), 0, true},
		{"param 0 missing", spec, 0, true},
		{"param 1 exists", spec, 1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.spec.GetCallback(tt.paramIdx)
			if (got == nil) != tt.wantNil {
				t.Errorf("GetCallback(%d) nil = %v, want nil = %v", tt.paramIdx, got == nil, tt.wantNil)
			}
		})
	}
}

func TestSpecIsFilter(t *testing.T) {
	tests := []struct {
		name     string
		spec     *Spec
		expected bool
	}{
		{"nil spec", nil, false},
		{"empty spec", NewSpec(), false},
		{
			"with predicate callback",
			NewSpec().WithCallback(1, PredicateSpec(0)),
			true,
		},
		{
			"with mapper callback",
			NewSpec().WithCallback(1, MapperSpec(0)),
			false,
		},
		{
			"mixed callbacks - one predicate",
			NewSpec().
				WithCallback(1, MapperSpec(0)).
				WithCallback(2, PredicateSpec(0)),
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.spec.IsFilter(); got != tt.expected {
				t.Errorf("IsFilter() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSpecGetIterator(t *testing.T) {
	spec := NewSpec().WithEffects(
		effect.Iterator{Source: effect.ParamRef{Index: 0}, Kind: effect.IterateIndexed},
	)

	tests := []struct {
		name    string
		spec    *Spec
		wantNil bool
	}{
		{"nil spec", nil, true},
		{"empty spec", NewSpec(), true},
		{"with iterator", spec, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.spec.GetIterator()
			if (got == nil) != tt.wantNil {
				t.Errorf("GetIterator() nil = %v, want nil = %v", got == nil, tt.wantNil)
			}
		})
	}
}

func TestSpecGetTableMutator(t *testing.T) {
	spec := NewSpec().WithEffects(
		effect.TableMutator{Target: effect.ParamRef{Index: 0}, Value: effect.ParamRef{Index: 1}},
	)

	tests := []struct {
		name    string
		spec    *Spec
		wantNil bool
	}{
		{"nil spec", nil, true},
		{"empty spec", NewSpec(), true},
		{"with table mutator", spec, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.spec.GetTableMutator()
			if (got == nil) != tt.wantNil {
				t.Errorf("GetTableMutator() nil = %v, want nil = %v", got == nil, tt.wantNil)
			}
		})
	}
}

func TestSpecIsIndexedIterator(t *testing.T) {
	tests := []struct {
		name     string
		spec     *Spec
		expected bool
	}{
		{"nil spec", nil, false},
		{"empty spec", NewSpec(), false},
		{
			"with indexed iterator",
			NewSpec().WithEffects(effect.Iterator{
				Source: effect.ParamRef{Index: 0},
				Kind:   effect.IterateIndexed,
			}),
			true,
		},
		{
			"with keyed iterator",
			NewSpec().WithEffects(effect.Iterator{
				Source: effect.ParamRef{Index: 0},
				Kind:   effect.IterateKeyed,
			}),
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.spec.IsIndexedIterator(); got != tt.expected {
				t.Errorf("IsIndexedIterator() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSpecIsKeyedIterator(t *testing.T) {
	tests := []struct {
		name     string
		spec     *Spec
		expected bool
	}{
		{"nil spec", nil, false},
		{"empty spec", NewSpec(), false},
		{
			"with indexed iterator",
			NewSpec().WithEffects(effect.Iterator{
				Source: effect.ParamRef{Index: 0},
				Kind:   effect.IterateIndexed,
			}),
			false,
		},
		{
			"with keyed iterator",
			NewSpec().WithEffects(effect.Iterator{
				Source: effect.ParamRef{Index: 0},
				Kind:   effect.IterateKeyed,
			}),
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.spec.IsKeyedIterator(); got != tt.expected {
				t.Errorf("IsKeyedIterator() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSpecIsTableMutator(t *testing.T) {
	tests := []struct {
		name     string
		spec     *Spec
		expected bool
	}{
		{"nil spec", nil, false},
		{"empty spec", NewSpec(), false},
		{
			"with table mutator",
			NewSpec().WithEffects(effect.TableMutator{
				Target: effect.ParamRef{Index: 0},
				Value:  effect.ParamRef{Index: 1},
			}),
			true,
		},
		{
			"with other effect",
			NewSpec().WithEffects(effect.Throw{}),
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.spec.IsTableMutator(); got != tt.expected {
				t.Errorf("IsTableMutator() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestCallbackSpecFields(t *testing.T) {
	t.Run("predicate spec fields", func(t *testing.T) {
		cb := PredicateSpec(2)
		if cb.InputSource.Index != 2 {
			t.Errorf("InputSource.Index = %d, want 2", cb.InputSource.Index)
		}

		if !cb.ReturnsBoolean {
			t.Error("ReturnsBoolean should be true")
		}

		if cb.Cardinality != CardAtMostOncePerElement {
			t.Errorf("Cardinality = %v, want CardAtMostOncePerElement", cb.Cardinality)
		}

		if !cb.Pure {
			t.Error("Pure should be true")
		}
	})

	t.Run("mapper spec fields", func(t *testing.T) {
		cb := MapperSpec(3)
		if cb.InputSource.Index != 3 {
			t.Errorf("InputSource.Index = %d, want 3", cb.InputSource.Index)
		}

		if cb.ReturnsBoolean {
			t.Error("ReturnsBoolean should be false")
		}

		if cb.Cardinality != CardOncePerElement {
			t.Errorf("Cardinality = %v, want CardOncePerElement", cb.Cardinality)
		}

		if !cb.Pure {
			t.Error("Pure should be true")
		}
	})
}

func TestSpecChaining(t *testing.T) {
	spec := NewSpec().
		WithRequires(constraint.IsNil{Path: root("x")}).
		WithEnsures(constraint.NotNil{Path: root("ret[0]")}).
		WithEffects(effect.Throw{}).
		WithCallback(1, PredicateSpec(0))

	if len(spec.Requires.AllConstraints()) != 1 {
		t.Errorf("Requires len = %d, want 1", len(spec.Requires.AllConstraints()))
	}

	if len(spec.Ensures.AllConstraints()) != 1 {
		t.Errorf("Ensures len = %d, want 1", len(spec.Ensures.AllConstraints()))
	}

	if !spec.Effects.HasThrow() {
		t.Error("Should have throw effect")
	}

	if spec.GetCallback(1) == nil {
		t.Error("Callback at 1 should not be nil")
	}
}

func TestCallbackSpecEnvOverlayEquality(t *testing.T) {
	env := map[string]typ.Type{
		"migration": typ.Func().Param("name", typ.String).Returns(typ.Nil).Build(),
		"database":  typ.Func().Returns(typ.Any).Build(),
	}

	t.Run("equal overlays", func(t *testing.T) {
		a := &CallbackSpec{
			InputSource: effect.ParamRef{Index: 0},
			Cardinality: CardExactlyOnce,
			EnvOverlay:  env,
		}
		b := &CallbackSpec{
			InputSource: effect.ParamRef{Index: 0},
			Cardinality: CardExactlyOnce,
			EnvOverlay:  env,
		}

		if !a.Equals(b) {
			t.Error("identical EnvOverlay maps should be equal")
		}
	})

	t.Run("different overlay values", func(t *testing.T) {
		a := &CallbackSpec{
			InputSource: effect.ParamRef{Index: 0},
			Cardinality: CardExactlyOnce,
			EnvOverlay:  map[string]typ.Type{"x": typ.String},
		}
		b := &CallbackSpec{
			InputSource: effect.ParamRef{Index: 0},
			Cardinality: CardExactlyOnce,
			EnvOverlay:  map[string]typ.Type{"x": typ.Number},
		}

		if a.Equals(b) {
			t.Error("different EnvOverlay values should not be equal")
		}
	})

	t.Run("different overlay keys", func(t *testing.T) {
		a := &CallbackSpec{
			InputSource: effect.ParamRef{Index: 0},
			Cardinality: CardExactlyOnce,
			EnvOverlay:  map[string]typ.Type{"x": typ.String},
		}
		b := &CallbackSpec{
			InputSource: effect.ParamRef{Index: 0},
			Cardinality: CardExactlyOnce,
			EnvOverlay:  map[string]typ.Type{"y": typ.String},
		}

		if a.Equals(b) {
			t.Error("different EnvOverlay keys should not be equal")
		}
	})

	t.Run("nil vs empty overlay", func(t *testing.T) {
		a := &CallbackSpec{InputSource: effect.ParamRef{Index: 0}, Cardinality: CardExactlyOnce}
		b := &CallbackSpec{InputSource: effect.ParamRef{Index: 0}, Cardinality: CardExactlyOnce, EnvOverlay: map[string]typ.Type{}}

		if !a.Equals(b) {
			t.Error("nil and empty EnvOverlay should be equal")
		}
	})

	t.Run("nil vs populated overlay", func(t *testing.T) {
		a := &CallbackSpec{InputSource: effect.ParamRef{Index: 0}, Cardinality: CardExactlyOnce}
		b := &CallbackSpec{
			InputSource: effect.ParamRef{Index: 0},
			Cardinality: CardExactlyOnce,
			EnvOverlay:  map[string]typ.Type{"x": typ.String},
		}

		if a.Equals(b) {
			t.Error("nil and populated EnvOverlay should not be equal")
		}
	})
}

func TestCallbackSpecWithEnvOverlay(t *testing.T) {
	env := map[string]typ.Type{"up": typ.Func().Returns(typ.Nil).Build()}
	cb := &CallbackSpec{InputSource: effect.ParamRef{Index: 0}, Cardinality: CardExactlyOnce}
	result := cb.WithEnvOverlay(env)

	if result != cb {
		t.Error("WithEnvOverlay should return the same CallbackSpec for chaining")
	}

	if len(cb.EnvOverlay) != 1 {
		t.Errorf("EnvOverlay len = %d, want 1", len(cb.EnvOverlay))
	}
}

func TestCallbackSpecClone(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		var cb *CallbackSpec
		if cb.Clone() != nil {
			t.Error("Clone of nil should return nil")
		}
	})

	t.Run("without overlay", func(t *testing.T) {
		cb := PredicateSpec(0)
		clone := cb.Clone()

		if !cb.Equals(clone) {
			t.Error("clone should equal original")
		}

		if clone == cb {
			t.Error("clone should be a different pointer")
		}
	})

	t.Run("with overlay", func(t *testing.T) {
		cb := &CallbackSpec{
			InputSource: effect.ParamRef{Index: 0},
			Cardinality: CardExactlyOnce,
			EnvOverlay: map[string]typ.Type{
				"db": typ.Func().Returns(typ.Any).Build(),
			},
		}

		clone := cb.Clone()

		if !cb.Equals(clone) {
			t.Error("clone should equal original")
		}

		// Mutating the clone's overlay should not affect the original.
		clone.EnvOverlay["extra"] = typ.String

		if len(cb.EnvOverlay) != 1 {
			t.Error("original overlay should be unaffected by clone mutation")
		}
	})
}

func TestExtractSpec(t *testing.T) {
	t.Run("nil type", func(t *testing.T) {
		if ExtractSpec(nil) != nil {
			t.Error("ExtractSpec(nil) should return nil")
		}
	})

	t.Run("non-function type", func(t *testing.T) {
		if ExtractSpec(typ.String) != nil {
			t.Error("ExtractSpec(string) should return nil")
		}
	})

	t.Run("function without spec", func(t *testing.T) {
		fn := typ.Func().Param("x", typ.String).Returns(typ.Number).Build()
		if ExtractSpec(fn) != nil {
			t.Error("ExtractSpec on function without spec should return nil")
		}
	})

	t.Run("function with spec", func(t *testing.T) {
		spec := NewSpec().WithRequires(constraint.NotNil{Path: root("x")})
		fn := typ.Func().Param("x", typ.String).Returns(typ.Number).Spec(spec).Build()
		extracted := ExtractSpec(fn)
		if extracted == nil {
			t.Fatal("ExtractSpec should return the spec")
		}
		if !extracted.Equals(spec) {
			t.Error("extracted spec should equal original")
		}
	})
}

func TestWithExprRequires(t *testing.T) {
	t.Run("add expr requires", func(t *testing.T) {
		spec := NewSpec().WithExprRequires(
			constraint.ExprCompare{
				Rel:   constraint.ExprGt,
				Left:  constraint.Param{Index: 0},
				Right: constraint.Const{Value: 0},
			},
		)
		if len(spec.ExprRequires) != 1 {
			t.Errorf("expected 1 expr require, got %d", len(spec.ExprRequires))
		}
	})

	t.Run("chain expr requires", func(t *testing.T) {
		spec := NewSpec().
			WithExprRequires(constraint.ExprCompare{Rel: constraint.ExprGt}).
			WithExprRequires(constraint.ExprCompare{Rel: constraint.ExprLt})
		if len(spec.ExprRequires) != 2 {
			t.Errorf("expected 2 expr requires, got %d", len(spec.ExprRequires))
		}
	})
}

func TestWithExprEnsures(t *testing.T) {
	t.Run("add expr ensures", func(t *testing.T) {
		spec := NewSpec().WithExprEnsures(
			constraint.ExprCompare{
				Rel:   constraint.ExprEq,
				Left:  constraint.Ret{Index: 0},
				Right: constraint.Param{Index: 0},
			},
		)
		if len(spec.ExprEnsures) != 1 {
			t.Errorf("expected 1 expr ensure, got %d", len(spec.ExprEnsures))
		}
	})
}

func TestReturnCaseEquals(t *testing.T) {
	cond1 := constraint.FromConstraints(constraint.NotNil{Path: root("x")})
	cond2 := constraint.FromConstraints(constraint.IsNil{Path: root("x")})

	t.Run("equal cases", func(t *testing.T) {
		a := ReturnCase{When: cond1, Type: typ.String}
		b := ReturnCase{When: cond1, Type: typ.String}
		if !a.Equals(b) {
			t.Error("equal cases should be equal")
		}
	})

	t.Run("different conditions", func(t *testing.T) {
		a := ReturnCase{When: cond1, Type: typ.String}
		b := ReturnCase{When: cond2, Type: typ.String}
		if a.Equals(b) {
			t.Error("different conditions should not be equal")
		}
	})

	t.Run("different types", func(t *testing.T) {
		a := ReturnCase{When: cond1, Type: typ.String}
		b := ReturnCase{When: cond1, Type: typ.Number}
		if a.Equals(b) {
			t.Error("different types should not be equal")
		}
	})

	t.Run("nil types equal", func(t *testing.T) {
		a := ReturnCase{When: cond1, Type: nil}
		b := ReturnCase{When: cond1, Type: nil}
		if !a.Equals(b) {
			t.Error("both nil types should be equal")
		}
	})

	t.Run("one nil type", func(t *testing.T) {
		a := ReturnCase{When: cond1, Type: typ.String}
		b := ReturnCase{When: cond1, Type: nil}
		if a.Equals(b) {
			t.Error("one nil type should not be equal")
		}
	})
}

func TestReturnSpecEquals(t *testing.T) {
	cond := constraint.FromConstraints(constraint.NotNil{Path: root("x")})

	t.Run("both nil", func(t *testing.T) {
		var a, b *ReturnSpec
		if !a.Equals(b) {
			t.Error("both nil should be equal")
		}
	})

	t.Run("one nil", func(t *testing.T) {
		a := &ReturnSpec{Default: typ.String}
		if a.Equals(nil) {
			t.Error("non-nil should not equal nil")
		}
	})

	t.Run("different case count", func(t *testing.T) {
		a := &ReturnSpec{Cases: []ReturnCase{{When: cond, Type: typ.String}}}
		b := &ReturnSpec{}
		if a.Equals(b) {
			t.Error("different case counts should not be equal")
		}
	})

	t.Run("different cases", func(t *testing.T) {
		a := &ReturnSpec{Cases: []ReturnCase{{When: cond, Type: typ.String}}}
		b := &ReturnSpec{Cases: []ReturnCase{{When: cond, Type: typ.Number}}}
		if a.Equals(b) {
			t.Error("different cases should not be equal")
		}
	})

	t.Run("different defaults", func(t *testing.T) {
		a := &ReturnSpec{Default: typ.String}
		b := &ReturnSpec{Default: typ.Number}
		if a.Equals(b) {
			t.Error("different defaults should not be equal")
		}
	})

	t.Run("one nil default", func(t *testing.T) {
		a := &ReturnSpec{Default: typ.String}
		b := &ReturnSpec{Default: nil}
		if a.Equals(b) {
			t.Error("one nil default should not be equal")
		}
	})

	t.Run("equal specs", func(t *testing.T) {
		a := &ReturnSpec{
			Cases:   []ReturnCase{{When: cond, Type: typ.String}},
			Default: typ.Number,
		}
		b := &ReturnSpec{
			Cases:   []ReturnCase{{When: cond, Type: typ.String}},
			Default: typ.Number,
		}
		if !a.Equals(b) {
			t.Error("equal specs should be equal")
		}
	})
}

func TestSpecEquals(t *testing.T) {
	t.Run("nil comparison", func(t *testing.T) {
		var s *Spec
		if !s.Equals(nil) {
			t.Error("nil spec should equal nil")
		}
	})

	t.Run("nil vs non-nil", func(t *testing.T) {
		s := NewSpec()
		var n *Spec
		if n.Equals(s) {
			t.Error("nil should not equal non-nil")
		}
	})

	t.Run("non-spec type", func(t *testing.T) {
		s := NewSpec()
		if s.Equals("not a spec") {
			t.Error("spec should not equal string")
		}
	})

	t.Run("different requires", func(t *testing.T) {
		a := NewSpec().WithRequires(constraint.NotNil{Path: root("x")})
		b := NewSpec().WithRequires(constraint.IsNil{Path: root("x")})
		if a.Equals(b) {
			t.Error("different requires should not be equal")
		}
	})

	t.Run("different ensures", func(t *testing.T) {
		a := NewSpec().WithEnsures(constraint.NotNil{Path: root("x")})
		b := NewSpec().WithEnsures(constraint.IsNil{Path: root("x")})
		if a.Equals(b) {
			t.Error("different ensures should not be equal")
		}
	})

	t.Run("different expr requires", func(t *testing.T) {
		a := NewSpec().WithExprRequires(constraint.ExprCompare{Rel: constraint.ExprGt})
		b := NewSpec().WithExprRequires(constraint.ExprCompare{Rel: constraint.ExprLt})
		if a.Equals(b) {
			t.Error("different expr requires should not be equal")
		}
	})

	t.Run("different expr ensures", func(t *testing.T) {
		a := NewSpec().WithExprEnsures(constraint.ExprCompare{Rel: constraint.ExprGt})
		b := NewSpec().WithExprEnsures(constraint.ExprCompare{Rel: constraint.ExprLt})
		if a.Equals(b) {
			t.Error("different expr ensures should not be equal")
		}
	})

	t.Run("different effects", func(t *testing.T) {
		a := NewSpec().WithEffects(effect.IO{})
		b := NewSpec().WithEffects(effect.Throw{})
		if a.Equals(b) {
			t.Error("different effects should not be equal")
		}
	})

	t.Run("different callbacks", func(t *testing.T) {
		a := NewSpec().WithCallback(0, PredicateSpec(0))
		b := NewSpec().WithCallback(0, MapperSpec(0))
		if a.Equals(b) {
			t.Error("different callbacks should not be equal")
		}
	})

	t.Run("different return specs", func(t *testing.T) {
		a := NewSpec().WithDefaultReturn(typ.String)
		b := NewSpec().WithDefaultReturn(typ.Number)
		if a.Equals(b) {
			t.Error("different return specs should not be equal")
		}
	})

	t.Run("equal specs", func(t *testing.T) {
		a := NewSpec().
			WithRequires(constraint.NotNil{Path: root("x")}).
			WithEnsures(constraint.NotNil{Path: root("ret")}).
			WithExprRequires(constraint.ExprCompare{Rel: constraint.ExprGt}).
			WithEffects(effect.IO{}).
			WithCallback(0, PredicateSpec(0)).
			WithDefaultReturn(typ.String)
		b := NewSpec().
			WithRequires(constraint.NotNil{Path: root("x")}).
			WithEnsures(constraint.NotNil{Path: root("ret")}).
			WithExprRequires(constraint.ExprCompare{Rel: constraint.ExprGt}).
			WithEffects(effect.IO{}).
			WithCallback(0, PredicateSpec(0)).
			WithDefaultReturn(typ.String)
		if !a.Equals(b) {
			t.Error("equal specs should be equal")
		}
	})
}

func TestIsSpecInfo(_ *testing.T) {
	spec := NewSpec()
	spec.IsSpecInfo()
}

func root(name string) constraint.Path {
	return constraint.Path{Root: name}
}
