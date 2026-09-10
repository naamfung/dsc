package constraint_test

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/subst"
)

func TestSolver_TruthyFalsy(t *testing.T) {
	path := constraint.Path{Root: "x"}
	base := map[constraint.PathKey]typ.Type{
		path.Key(): typ.NewUnion(typ.String, typ.Nil, typ.Boolean),
	}
	s := constraint.Solver{}

	truthy := s.Apply(constraint.NewConjunction(constraint.Truthy{Path: path}), base)
	wantTruthy := typ.NewUnion(typ.String, typ.True)

	if got := truthy[path.Key()]; !typ.TypeEquals(got, wantTruthy) {
		t.Fatalf("truthy got %v want %v", got, wantTruthy)
	}

	falsy := s.Apply(constraint.NewConjunction(constraint.Falsy{Path: path}), base)
	wantFalsy := typ.NewUnion(typ.Nil, typ.LiteralBool(false))

	if got := falsy[path.Key()]; !typ.TypeEquals(got, wantFalsy) {
		t.Fatalf("falsy got %v want %v", got, wantFalsy)
	}
}

func TestSolver_IsNil_NotNil(t *testing.T) {
	path := constraint.Path{Root: "x"}
	base := map[constraint.PathKey]typ.Type{
		path.Key(): typ.NewOptional(typ.String),
	}
	s := constraint.Solver{}

	isNil := s.Apply(constraint.NewConjunction(constraint.IsNil{Path: path}), base)
	if got := isNil[path.Key()]; !typ.TypeEquals(got, typ.Nil) {
		t.Fatalf("isnil got %v want %v", got, typ.Nil)
	}

	notNil := s.Apply(constraint.NewConjunction(constraint.NotNil{Path: path}), base)
	if got := notNil[path.Key()]; !typ.TypeEquals(got, typ.String) {
		t.Fatalf("notnil got %v want %v", got, typ.String)
	}
}

func TestSolver_IsNil_OrderIndependentWithTruthy(t *testing.T) {
	path := constraint.Path{Root: "x"}
	base := map[constraint.PathKey]typ.Type{
		path.Key(): typ.NewOptional(typ.String),
	}
	s := constraint.Solver{}

	cases := []struct {
		name        string
		constraints []constraint.Constraint
	}{
		{
			name: "truthy then isnil",
			constraints: []constraint.Constraint{
				constraint.Truthy{Path: path},
				constraint.IsNil{Path: path},
			},
		},
		{
			name: "isnil then truthy",
			constraints: []constraint.Constraint{
				constraint.IsNil{Path: path},
				constraint.Truthy{Path: path},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := s.Apply(tc.constraints, base)
			if got := out[path.Key()]; !typ.TypeEquals(got, typ.Never) {
				t.Fatalf("got %v want %v", got, typ.Never)
			}
		})
	}
}

func TestSolver_ApplyToSingle_IsNil_OrderIndependentWithTruthy(t *testing.T) {
	path := constraint.Path{Root: "x"}
	key := path.Key()
	base := typ.NewOptional(typ.String)
	resolve := func(p constraint.Path) constraint.PathKey { return p.Key() }
	s := constraint.Solver{}

	cases := []struct {
		name        string
		constraints []constraint.Constraint
	}{
		{
			name: "truthy then isnil",
			constraints: []constraint.Constraint{
				constraint.Truthy{Path: path},
				constraint.IsNil{Path: path},
			},
		},
		{
			name: "isnil then truthy",
			constraints: []constraint.Constraint{
				constraint.IsNil{Path: path},
				constraint.Truthy{Path: path},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := s.ApplyToSingle(tc.constraints, key, base, resolve); !typ.TypeEquals(got, typ.Never) {
				t.Fatalf("got %v want %v", got, typ.Never)
			}
		})
	}
}

func TestSolver_HasType_BuiltinAndHash(t *testing.T) {
	path := constraint.Path{Root: "x"}
	rec := typ.NewRecord().Field("value", typ.String).Build()
	union := typ.NewUnion(typ.String, typ.Number, rec)
	base := map[constraint.PathKey]typ.Type{
		path.Key(): union,
	}

	resolver := func(k narrow.TypeKey) typ.Type {
		switch k.Kind {
		case narrow.TypeKeyBuiltin:
			switch kind.FromString(k.Name) {
			case kind.String:
				return typ.String
			case kind.Number:
				return typ.Number
			case kind.Boolean:
				return typ.Boolean
			case kind.Nil:
				return typ.Nil
			}
		case narrow.TypeKeyHash:
			if k.Hash == rec.Hash() {
				return rec
			}
		}

		return nil
	}

	s := constraint.Solver{Env: constraint.Env{ResolveType: resolver}}

	hasString := s.Apply(constraint.NewConjunction(constraint.HasType{Path: path, Type: narrow.BuiltinTypeKey("string")}), base)
	if got := hasString[path.Key()]; !typ.TypeEquals(got, typ.String) {
		t.Fatalf("hasType string got %v want %v", got, typ.String)
	}

	hasRec := s.Apply(constraint.NewConjunction(constraint.HasType{Path: path, Type: narrow.HashTypeKey(rec.Hash())}), base)
	if got := hasRec[path.Key()]; !typ.TypeEquals(got, rec) {
		t.Fatalf("hasType hash got %v want %v", got, rec)
	}
}

func TestSolver_HasType_FieldLiteralNarrowsParent(t *testing.T) {
	response := constraint.Path{Root: "response"}
	success := response.Field("success")

	okRec := typ.NewRecord().
		Field("success", typ.True).
		Field("result", typ.String).
		Build()
	errRec := typ.NewRecord().
		Field("success", typ.False).
		Field("error_message", typ.String).
		Build()

	base := map[constraint.PathKey]typ.Type{
		response.Key(): typ.NewUnion(okRec, errRec),
		success.Key():  typ.NewUnion(typ.True, typ.False),
	}

	resolveType := func(key narrow.TypeKey) typ.Type {
		if key == narrow.HashTypeKey(typ.False.Hash()) {
			return typ.False
		}
		return nil
	}

	s := constraint.Solver{
		Env: constraint.Env{
			ResolveType: resolveType,
			Resolver:    core.Resolver(),
		},
	}

	out := s.Apply(constraint.NewConjunction(
		constraint.HasType{Path: success, Type: narrow.HashTypeKey(typ.False.Hash())},
	), base)

	if got := out[success.Key()]; !typ.TypeEquals(got, typ.False) {
		t.Fatalf("field type got %v want %v", got, typ.False)
	}
	if got := out[response.Key()]; !typ.TypeEquals(got, errRec) {
		t.Fatalf("parent union got %v want %v", got, errRec)
	}
}

func TestSolver_NotHasType_FieldLiteralNarrowsParent(t *testing.T) {
	response := constraint.Path{Root: "response"}
	success := response.Field("success")

	okRec := typ.NewRecord().
		Field("success", typ.True).
		Field("result", typ.String).
		Build()
	errRec := typ.NewRecord().
		Field("success", typ.False).
		Field("error_message", typ.String).
		Build()

	base := map[constraint.PathKey]typ.Type{
		response.Key(): typ.NewUnion(okRec, errRec),
		success.Key():  typ.NewUnion(typ.True, typ.False),
	}

	resolveType := func(key narrow.TypeKey) typ.Type {
		if key == narrow.HashTypeKey(typ.False.Hash()) {
			return typ.False
		}
		return nil
	}

	s := constraint.Solver{
		Env: constraint.Env{
			ResolveType: resolveType,
			Resolver:    core.Resolver(),
		},
	}

	out := s.Apply(constraint.NewConjunction(
		constraint.NotHasType{Path: success, Type: narrow.HashTypeKey(typ.False.Hash())},
	), base)

	if got := out[success.Key()]; !typ.TypeEquals(got, typ.True) {
		t.Fatalf("field type got %v want %v", got, typ.True)
	}
	if got := out[response.Key()]; !typ.TypeEquals(got, okRec) {
		t.Fatalf("parent union got %v want %v", got, okRec)
	}
}

func TestSolver_FieldEquals_Literal(t *testing.T) {
	path := constraint.Path{Root: "x"}
	event := typ.NewRecord().Field("kind", typ.LiteralString("event")).Build()
	timeout := typ.NewRecord().Field("kind", typ.LiteralString("timeout")).Build()
	base := map[constraint.PathKey]typ.Type{
		path.Key(): typ.NewUnion(event, timeout),
	}

	queryField := func(t typ.Type, field string) (typ.Type, bool) {
		if rec, ok := t.(*typ.Record); ok {
			if f := rec.GetField(field); f != nil {
				return f.Type, true
			}
		}

		return nil, false
	}

	s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: queryField}}}
	set := constraint.NewConjunction(constraint.FieldEquals{Target: path, Field: "kind", Value: typ.LiteralString("event")})

	out := s.Apply(set, base)
	if got := out[path.Key()]; !typ.TypeEquals(got, event) {
		t.Fatalf("field equals got %v want %v", got, event)
	}
}

func TestSolver_FieldEqualsPath_IntersectionPreserved(t *testing.T) {
	target := constraint.Path{Root: "event"}
	value := constraint.Path{Root: "exit"}

	eventRec := typ.NewRecord().Field("kind", typ.String).Build()
	eventMethods := typ.NewInterface("EventMethods", []typ.Method{
		{Name: "payload", Type: typ.Func().Returns(typ.Any).Build()},
	})
	eventType := typ.NewIntersection(eventRec, eventMethods)

	base := map[constraint.PathKey]typ.Type{
		target.Key(): eventType,
		value.Key():  typ.String,
	}

	s := constraint.Solver{Env: constraint.Env{Resolver: core.Resolver()}}
	set := constraint.NewConjunction(constraint.FieldEqualsPath{Target: target, Field: "kind", Value: value})

	out := s.Apply(set, base)
	got := out[target.Key()]
	if got == nil || got.Kind() == kind.Never {
		t.Fatalf("field equals path on intersection returned %v, want %v", got, eventType)
	}
	if !typ.TypeEquals(got, eventType) {
		t.Fatalf("field equals path on intersection got %v want %v", got, eventType)
	}
}

func TestSolver_EqPath(t *testing.T) {
	left := constraint.Path{Root: "a"}
	right := constraint.Path{Root: "b"}
	base := map[constraint.PathKey]typ.Type{
		left.Key():  typ.NewUnion(typ.String, typ.Number),
		right.Key(): typ.String,
	}
	s := constraint.Solver{}

	out := s.Apply(constraint.NewConjunction(constraint.NewEqPath(left, right)), base)
	if got := out[left.Key()]; !typ.TypeEquals(got, typ.String) {
		t.Fatalf("eq left got %v want %v", got, typ.String)
	}

	if got := out[right.Key()]; !typ.TypeEquals(got, typ.String) {
		t.Fatalf("eq right got %v want %v", got, typ.String)
	}
}

func TestSolver_FieldEqualsPath(t *testing.T) {
	target := constraint.Path{Root: "x"}
	value := constraint.Path{Root: "y"}
	event := typ.NewRecord().Field("kind", typ.LiteralString("event")).Build()
	timeout := typ.NewRecord().Field("kind", typ.LiteralString("timeout")).Build()

	base := map[constraint.PathKey]typ.Type{
		target.Key(): typ.NewUnion(event, timeout),
		value.Key():  typ.LiteralString("event"),
	}

	queryField := func(t typ.Type, field string) (typ.Type, bool) {
		if rec, ok := t.(*typ.Record); ok {
			if f := rec.GetField(field); f != nil {
				return f.Type, true
			}
		}

		return nil, false
	}

	s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: queryField}}}
	set := constraint.NewConjunction(constraint.FieldEqualsPath{Target: target, Field: "kind", Value: value})

	out := s.Apply(set, base)
	if got := out[target.Key()]; !typ.TypeEquals(got, event) {
		t.Fatalf("field eq path target got %v want %v", got, event)
	}

	if got := out[value.Key()]; !typ.TypeEquals(got, typ.LiteralString("event")) {
		t.Fatalf("field eq path value got %v want %v", got, typ.LiteralString("event"))
	}
}

func TestSolver_FieldEqualsPath_ValueUnion_NoChange(t *testing.T) {
	target := constraint.Path{Root: "x"}
	value := constraint.Path{Root: "y"}
	event := typ.NewRecord().Field("kind", typ.LiteralString("event")).Build()
	timeout := typ.NewRecord().Field("kind", typ.LiteralString("timeout")).Build()

	base := map[constraint.PathKey]typ.Type{
		target.Key(): typ.NewUnion(event, timeout),
		value.Key():  typ.NewUnion(typ.LiteralString("event"), typ.LiteralString("timeout")),
	}

	s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: unionQueryField}}}
	set := constraint.NewConjunction(constraint.FieldEqualsPath{Target: target, Field: "kind", Value: value})

	out := s.Apply(set, base)
	if got := out[target.Key()]; !typ.TypeEquals(got, base[target.Key()]) {
		t.Fatalf("expected no change to target, got %v", got)
	}

	if got := out[value.Key()]; !typ.TypeEquals(got, base[value.Key()]) {
		t.Fatalf("expected no change to value, got %v", got)
	}
}

func TestSolver_FieldEqualsPath_ChannelLike(t *testing.T) {
	target := constraint.Path{Root: "result"}
	value := constraint.Path{Root: "timeout"}

	chEvent := typ.NewRecord().Field("id", typ.LiteralString("event")).Build()
	chTimeout := typ.NewRecord().Field("id", typ.LiteralString("timeout")).Build()
	resultEvent := typ.NewRecord().Field("channel", chEvent).Field("value", typ.String).Build()
	resultTimeout := typ.NewRecord().Field("channel", chTimeout).Field("value", typ.Number).Build()

	base := map[constraint.PathKey]typ.Type{
		target.Key(): typ.NewUnion(resultEvent, resultTimeout),
		value.Key():  chTimeout,
	}

	s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: unionQueryField}}}
	set := constraint.NewConjunction(constraint.FieldEqualsPath{Target: target, Field: "channel", Value: value})

	out := s.Apply(set, base)
	if got := out[target.Key()]; !typ.TypeEquals(got, resultTimeout) {
		t.Fatalf("expected result narrowed to timeout case, got %v", got)
	}

	if got := out[value.Key()]; !typ.TypeEquals(got, chTimeout) {
		t.Fatalf("expected timeout unchanged, got %v", got)
	}
}

func TestSolver_FieldNotEqualsPath_ExcludesMatchingVariant(t *testing.T) {
	tests := []struct {
		name       string
		tagField   string
		matchTag   string
		excludeTag string
		fieldName  string
	}{
		{"channel like", "id", "event", "timeout", "channel"},
		{"int/str variant", "__tag", "str", "int", "channel"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := constraint.Path{Root: "result"}
			value := constraint.Path{Root: "excludeValue"}

			chMatch := typ.NewRecord().Field(tt.tagField, typ.LiteralString(tt.matchTag)).Build()
			chExclude := typ.NewRecord().Field(tt.tagField, typ.LiteralString(tt.excludeTag)).Build()
			resultMatch := typ.NewRecord().Field(tt.fieldName, chMatch).Field("value", typ.String).Build()
			resultExclude := typ.NewRecord().Field(tt.fieldName, chExclude).Field("value", typ.Number).Build()

			base := map[constraint.PathKey]typ.Type{
				target.Key(): typ.NewUnion(resultMatch, resultExclude),
				value.Key():  chExclude,
			}

			s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: unionQueryField}}}
			set := constraint.NewConjunction(constraint.FieldNotEqualsPath{Target: target, Field: tt.fieldName, Value: value})

			out := s.Apply(set, base)
			if got := out[target.Key()]; !typ.TypeEquals(got, resultMatch) {
				t.Fatalf("expected result narrowed to match case, got %v", got)
			}
		})
	}
}

// FieldNotEqualsPath should NOT exclude when field or value are non-singleton.
func TestSolver_FieldNotEqualsPath_NoExclusion(t *testing.T) {
	variantStr := typ.NewRecord().Field("role", typ.String).Build()
	variantNum := typ.NewRecord().Field("role", typ.Number).Build()
	union := typ.NewUnion(variantStr, variantNum)
	interA := typ.NewRecord().Field("role", typ.String).Build()
	interB := typ.NewRecord().Field("department", typ.String).Build()
	inter := typ.NewIntersection(interA, interB)

	tests := []struct {
		name       string
		targetType typ.Type
		field      string
		valueType  typ.Type
	}{
		{"non-singleton field", typ.NewRecord().Field("kind", typ.String).Build(), "kind", typ.String},
		{"value any", typ.NewRecord().Field("role", typ.String).Build(), "role", typ.Any},
		{"field any", typ.NewRecord().Field("kind", typ.Any).Build(), "kind", typ.String},
		{"different non-singleton", typ.NewRecord().Field("role", typ.String).Build(), "role", typ.Number},
		{"union non-singleton", union, "role", typ.String},
		{"intersection non-singleton", inter, "role", typ.String},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := constraint.Path{Root: "x"}
			value := constraint.Path{Root: "v"}
			base := map[constraint.PathKey]typ.Type{
				target.Key(): tt.targetType,
				value.Key():  tt.valueType,
			}

			s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: unionQueryField}}}
			set := constraint.NewConjunction(constraint.FieldNotEqualsPath{Target: target, Field: tt.field, Value: value})

			out := s.Apply(set, base)
			got := out[target.Key()]
			if got == nil || got.Kind() == kind.Never {
				t.Fatalf("expected non-never, got %v", got)
			}
			if !typ.TypeEquals(got, tt.targetType) {
				t.Fatalf("expected type unchanged, got %v", got)
			}
		})
	}
}

// FieldNotEquals (literal) should NOT exclude when the field is non-singleton or type mismatches.
func TestSolver_FieldNotEquals_Literal_NoExclusion(t *testing.T) {
	tests := []struct {
		name    string
		recType *typ.Record
		field   string
		literal *typ.Literal
	}{
		{
			name:    "non-singleton field",
			recType: typ.NewRecord().Field("kind", typ.String).Build(),
			field:   "kind",
			literal: typ.LiteralString("event"),
		},
		{
			name:    "different kind",
			recType: typ.NewRecord().Field("count", typ.Number).Build(),
			field:   "count",
			literal: typ.LiteralString("x"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := constraint.Path{Root: "x"}
			base := map[constraint.PathKey]typ.Type{path.Key(): tt.recType}
			s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: unionQueryField}}}
			set := constraint.NewConjunction(constraint.FieldNotEquals{Target: path, Field: tt.field, Value: tt.literal})

			out := s.Apply(set, base)
			got := out[path.Key()]
			if got == nil || got.Kind() == kind.Never {
				t.Fatalf("expected non-never, got %v", got)
			}
			if !typ.TypeEquals(got, tt.recType) {
				t.Fatalf("expected type unchanged, got %v", got)
			}
		})
	}
}

// FieldNotEqualsPath should NOT exclude when the value is a non-singleton union of literals.
// Example: {kind: "event"} with kind ~= ("event" | "timeout") should remain unchanged.
func TestSolver_FieldNotEqualsPath_ValueUnionNoExclusion(t *testing.T) {
	target := constraint.Path{Root: "evt"}
	value := constraint.Path{Root: "k"}

	eventRec := typ.NewRecord().Field("kind", typ.LiteralString("event")).Build()
	valueUnion := typ.NewUnion(typ.LiteralString("event"), typ.LiteralString("timeout"))

	base := map[constraint.PathKey]typ.Type{
		target.Key(): eventRec,
		value.Key():  valueUnion,
	}

	s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: unionQueryField}}}
	set := constraint.NewConjunction(constraint.FieldNotEqualsPath{Target: target, Field: "kind", Value: value})

	out := s.Apply(set, base)
	got := out[target.Key()]
	if got == nil || got.Kind() == kind.Never {
		t.Fatalf("expected non-never after FieldNotEqualsPath with union value, got %v", got)
	}
	if !typ.TypeEquals(got, eventRec) {
		t.Fatalf("expected event record unchanged, got %v", got)
	}
}

// Repeated FieldNotEqualsPath constraints with non-singleton values should not collapse to never.
func TestSolver_FieldNotEqualsPath_RepeatedNonSingletonNoExclusion(t *testing.T) {
	target := constraint.Path{Root: "err"}
	valueA := constraint.Path{Root: "a"}
	valueB := constraint.Path{Root: "b"}

	errType := typ.NewRecord().Field("kind", typ.String).Build()
	base := map[constraint.PathKey]typ.Type{
		target.Key(): errType,
		valueA.Key(): typ.String,
		valueB.Key(): typ.String,
	}

	s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: unionQueryField}}}
	set := constraint.NewConjunction(
		constraint.FieldNotEqualsPath{Target: target, Field: "kind", Value: valueA},
		constraint.FieldNotEqualsPath{Target: target, Field: "kind", Value: valueB},
	)

	out := s.Apply(set, base)
	got := out[target.Key()]
	if got == nil || got.Kind() == kind.Never {
		t.Fatalf("expected non-never after repeated FieldNotEqualsPath, got %v", got)
	}
	if !typ.TypeEquals(got, errType) {
		t.Fatalf("expected err type unchanged, got %v", got)
	}
}

// FieldNotEqualsPath on different fields with non-singleton values should not exclude.
func TestSolver_FieldNotEqualsPath_MultipleFieldsNonSingletonNoExclusion(t *testing.T) {
	target := constraint.Path{Root: "meta"}
	valRole := constraint.Path{Root: "role_expected"}
	valDept := constraint.Path{Root: "dept_expected"}

	metaType := typ.NewRecord().
		Field("role", typ.String).
		Field("department", typ.String).
		Build()
	base := map[constraint.PathKey]typ.Type{
		target.Key():  metaType,
		valRole.Key(): typ.String,
		valDept.Key(): typ.String,
	}

	s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: unionQueryField}}}
	set := constraint.NewConjunction(
		constraint.FieldNotEqualsPath{Target: target, Field: "role", Value: valRole},
		constraint.FieldNotEqualsPath{Target: target, Field: "department", Value: valDept},
	)

	out := s.Apply(set, base)
	got := out[target.Key()]
	if got == nil || got.Kind() == kind.Never {
		t.Fatalf("expected non-never after multiple FieldNotEqualsPath, got %v", got)
	}
	if !typ.TypeEquals(got, metaType) {
		t.Fatalf("expected meta type unchanged, got %v", got)
	}
}

// FieldNotEquals (literal) on union with non-singleton field types should not exclude.
func TestSolver_FieldNotEquals_Literal_UnionNonSingletonNoExclusion(t *testing.T) {
	path := constraint.Path{Root: "meta"}
	variantA := typ.NewRecord().Field("role", typ.String).Field("dept", typ.String).Build()
	variantB := typ.NewRecord().Field("role", typ.String).Field("dept", typ.Number).Build()
	union := typ.NewUnion(variantA, variantB)

	base := map[constraint.PathKey]typ.Type{
		path.Key(): union,
	}

	s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: unionQueryField}}}
	set := constraint.NewConjunction(constraint.FieldNotEquals{Target: path, Field: "role", Value: typ.LiteralString("admin")})

	out := s.Apply(set, base)
	got := out[path.Key()]
	if got == nil || got.Kind() == kind.Never {
		t.Fatalf("expected non-never after FieldNotEquals on union, got %v", got)
	}
	if !typ.TypeEquals(got, union) {
		t.Fatalf("expected union unchanged, got %v", got)
	}
}

// FieldNotEqualsPath should not exclude when field type is optional non-singleton.
func TestSolver_FieldNotEqualsPath_FieldOptionalNoExclusion(t *testing.T) {
	target := constraint.Path{Root: "meta"}
	value := constraint.Path{Root: "expected"}

	metaType := typ.NewRecord().Field("role", typ.NewOptional(typ.String)).Build()
	base := map[constraint.PathKey]typ.Type{
		target.Key(): metaType,
		value.Key():  typ.String,
	}

	s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: unionQueryField}}}
	set := constraint.NewConjunction(constraint.FieldNotEqualsPath{Target: target, Field: "role", Value: value})

	out := s.Apply(set, base)
	got := out[target.Key()]
	if got == nil || got.Kind() == kind.Never {
		t.Fatalf("expected non-never for optional field, got %v", got)
	}
	if !typ.TypeEquals(got, metaType) {
		t.Fatalf("expected meta type unchanged, got %v", got)
	}
}

// FieldNotEqualsPath should keep unions with mixed singleton/non-singleton field types.
// Only singleton-matching variants may be excluded.
func TestSolver_FieldNotEqualsPath_MixedSingletonUnion(t *testing.T) {
	target := constraint.Path{Root: "r"}
	value := constraint.Path{Root: "v"}

	variantSingleton := typ.NewRecord().Field("kind", typ.LiteralString("event")).Build()
	variantWide := typ.NewRecord().Field("kind", typ.String).Build()
	union := typ.NewUnion(variantSingleton, variantWide)

	base := map[constraint.PathKey]typ.Type{
		target.Key(): union,
		value.Key():  typ.String, // non-singleton
	}

	s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: unionQueryField}}}
	set := constraint.NewConjunction(constraint.FieldNotEqualsPath{Target: target, Field: "kind", Value: value})

	out := s.Apply(set, base)
	got := out[target.Key()]
	if got == nil || got.Kind() == kind.Never {
		t.Fatalf("expected non-never for mixed union, got %v", got)
	}
	// Since value is non-singleton, union should remain unchanged.
	if !typ.TypeEquals(got, union) {
		t.Fatalf("expected union unchanged, got %v", got)
	}
}

// FieldNotEqualsPath should not exclude with a placeholder value path (Symbol=0).
func TestSolver_FieldNotEqualsPath_PlaceholderValueNoExclusion(t *testing.T) {
	target := constraint.Path{Root: "meta"}
	placeholder := constraint.Path{Root: "$placeholder"}

	metaType := typ.NewRecord().Field("role", typ.String).Build()
	base := map[constraint.PathKey]typ.Type{
		target.Key(): metaType,
	}

	s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: unionQueryField}}}
	set := constraint.NewConjunction(constraint.FieldNotEqualsPath{Target: target, Field: "role", Value: placeholder})

	out := s.Apply(set, base)
	got := out[target.Key()]
	if got == nil || got.Kind() == kind.Never {
		t.Fatalf("expected non-never for placeholder value path, got %v", got)
	}
	if !typ.TypeEquals(got, metaType) {
		t.Fatalf("expected meta type unchanged, got %v", got)
	}
}

// IndexNotEqualsPath should not exclude when key/value are non-singleton.
func TestSolver_IndexNotEqualsPath_NonSingletonNoExclusion(t *testing.T) {
	target := constraint.Path{Root: "t"}
	value := constraint.Path{Root: "v"}

	tableType := typ.NewRecord().Field("k", typ.String).Build()
	base := map[constraint.PathKey]typ.Type{
		target.Key(): tableType,
		value.Key():  typ.String,
	}

	s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: unionQueryField, IndexFunc: core.Index}}}
	set := constraint.NewConjunction(constraint.IndexNotEqualsPath{Target: target, Key: typ.LiteralString("k"), Value: value})

	out := s.Apply(set, base)
	got := out[target.Key()]
	if got == nil || got.Kind() == kind.Never {
		t.Fatalf("expected non-never for IndexNotEqualsPath with non-singleton, got %v", got)
	}
	if !typ.TypeEquals(got, tableType) {
		t.Fatalf("expected table type unchanged, got %v", got)
	}
}

// Nested path (obj.inner.kind ~= expected) with non-singleton value should not exclude.
func TestSolver_FieldNotEqualsPath_NestedPathNonSingletonNoExclusion(t *testing.T) {
	target := constraint.Path{
		Root:     "obj",
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "inner"}},
	}
	value := constraint.Path{Root: "expected"}

	innerType := typ.NewRecord().Field("kind", typ.String).Build()
	base := map[constraint.PathKey]typ.Type{
		target.Key(): innerType,
		value.Key():  typ.String,
	}

	s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: unionQueryField}}}
	set := constraint.NewConjunction(constraint.FieldNotEqualsPath{Target: target, Field: "kind", Value: value})

	out := s.Apply(set, base)
	got := out[target.Key()]
	if got == nil || got.Kind() == kind.Never {
		t.Fatalf("expected non-never for nested path non-singleton, got %v", got)
	}
	if !typ.TypeEquals(got, innerType) {
		t.Fatalf("expected inner type unchanged, got %v", got)
	}
}

// Alias-wrapped record should behave like its target for non-singleton exclusion.
func TestSolver_FieldNotEqualsPath_AliasNonSingletonNoExclusion(t *testing.T) {
	target := constraint.Path{Root: "meta"}
	value := constraint.Path{Root: "expected"}

	rec := typ.NewRecord().Field("role", typ.String).Build()
	alias := typ.NewAlias("Meta", rec)

	base := map[constraint.PathKey]typ.Type{
		target.Key(): alias,
		value.Key():  typ.String,
	}

	queryField := func(t typ.Type, field string) (typ.Type, bool) {
		if a, ok := t.(*typ.Alias); ok {
			return unionQueryField(a.Target, field)
		}
		return unionQueryField(t, field)
	}

	s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: queryField}}}
	set := constraint.NewConjunction(constraint.FieldNotEqualsPath{Target: target, Field: "role", Value: value})

	out := s.Apply(set, base)
	got := out[target.Key()]
	if got == nil || got.Kind() == kind.Never {
		t.Fatalf("expected non-never for alias record, got %v", got)
	}
	if !typ.TypeEquals(got, alias) {
		t.Fatalf("expected alias type unchanged, got %v", got)
	}
}

// Instantiated generics should not be excluded by non-singleton FieldNotEqualsPath.
func TestSolver_FieldNotEqualsPath_InstantiatedGenericNoExclusion(t *testing.T) {
	target := constraint.Path{Root: "box"}
	value := constraint.Path{Root: "expected"}

	tp := typ.NewTypeParam("T", nil)
	gen := typ.NewGeneric("Box", []*typ.TypeParam{tp}, typ.NewRecord().Field("value", tp).Build())
	inst := typ.Instantiate(gen, typ.String)

	base := map[constraint.PathKey]typ.Type{
		target.Key(): inst,
		value.Key():  typ.String,
	}

	queryField := func(t typ.Type, field string) (typ.Type, bool) {
		if inst, ok := t.(*typ.Instantiated); ok {
			if expanded := subst.ExpandInstantiated(inst); expanded != nil {
				return unionQueryField(expanded, field)
			}
		}
		return unionQueryField(t, field)
	}

	s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: queryField}}}
	set := constraint.NewConjunction(constraint.FieldNotEqualsPath{Target: target, Field: "value", Value: value})

	out := s.Apply(set, base)
	got := out[target.Key()]
	if got == nil || got.Kind() == kind.Never {
		t.Fatalf("expected non-never for instantiated generic, got %v", got)
	}
	if !typ.TypeEquals(got, inst) {
		t.Fatalf("expected instantiated type unchanged, got %v", got)
	}
}

// Combination: FieldNotEqualsPath (non-singleton) + NotHasType (unrelated) should not exclude.
func TestSolver_FieldNotEqualsPath_WithNotHasType_NoExclusion(t *testing.T) {
	target := constraint.Path{Root: "meta"}
	value := constraint.Path{Root: "expected"}

	metaType := typ.NewRecord().Field("role", typ.String).Build()
	base := map[constraint.PathKey]typ.Type{
		target.Key(): metaType,
		value.Key():  typ.String,
	}

	set := constraint.NewConjunction(
		constraint.FieldNotEqualsPath{Target: target, Field: "role", Value: value},
		constraint.NotHasType{Path: target, Type: narrow.BuiltinTypeKey("function")},
	)

	s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: unionQueryField}}}
	out := s.Apply(set, base)
	got := out[target.Key()]
	if got == nil || got.Kind() == kind.Never {
		t.Fatalf("expected non-never for combined constraints, got %v", got)
	}
	if !typ.TypeEquals(got, metaType) {
		t.Fatalf("expected meta type unchanged, got %v", got)
	}
}

// Interleaved constraints (ANDing): mix literal exclusion and non-singleton inequality.
func TestSolver_FieldNotEquals_InterleavedConstraints(t *testing.T) {
	path := constraint.Path{Root: "evt"}
	value := constraint.Path{Root: "expected"}

	a := typ.NewRecord().Field("kind", typ.LiteralString("a")).Build()
	b := typ.NewRecord().Field("kind", typ.LiteralString("b")).Build()
	w := typ.NewRecord().Field("kind", typ.String).Build()
	union := typ.NewUnion(a, b, w)

	base := map[constraint.PathKey]typ.Type{
		path.Key():  union,
		value.Key(): typ.String,
	}

	set := constraint.NewConjunction(
		constraint.FieldNotEquals{Target: path, Field: "kind", Value: typ.LiteralString("a")},
		constraint.FieldNotEqualsPath{Target: path, Field: "kind", Value: value},
		constraint.NotHasType{Path: path, Type: narrow.BuiltinTypeKey("function")},
	)

	s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: unionQueryField}}}
	out := s.Apply(set, base)
	got := out[path.Key()]
	if got == nil || got.Kind() == kind.Never {
		t.Fatalf("expected non-never after interleaved constraints, got %v", got)
	}

	want := typ.NewUnion(b, w) // only "a" excluded
	if !typ.TypeEquals(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestSolver_IndexLiteral_Record(t *testing.T) {
	target := constraint.Path{Root: "x"}
	event := typ.NewRecord().Field("kind", typ.LiteralString("event")).Build()
	timeout := typ.NewRecord().Field("kind", typ.LiteralString("timeout")).Build()

	base := map[constraint.PathKey]typ.Type{
		target.Key(): typ.NewUnion(event, timeout),
	}

	tests := []struct {
		name       string
		constraint constraint.Constraint
		expect     typ.Type
	}{
		{"equals selects event", constraint.IndexEquals{Target: target, Key: typ.LiteralString("kind"), Value: typ.LiteralString("event")}, event},
		{"not equals excludes event", constraint.IndexNotEquals{Target: target, Key: typ.LiteralString("kind"), Value: typ.LiteralString("event")}, timeout},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := constraint.Solver{Env: constraint.Env{Resolver: core.Resolver()}}
			out := s.Apply(constraint.NewConjunction(tt.constraint), base)
			if got := out[target.Key()]; !typ.TypeEquals(got, tt.expect) {
				t.Fatalf("expected %v, got %v", tt.expect, got)
			}
		})
	}
}

func TestSolver_IndexEquals_LiteralInt_ArrayUnion(t *testing.T) {
	target := constraint.Path{Root: "x"}
	arrString := typ.NewArray(typ.String)
	arrNumber := typ.NewArray(typ.Number)
	base := map[constraint.PathKey]typ.Type{
		target.Key(): typ.NewUnion(arrString, arrNumber),
	}

	s := constraint.Solver{Env: constraint.Env{Resolver: core.Resolver()}}
	set := constraint.NewConjunction(constraint.IndexEquals{Target: target, Key: typ.LiteralInt(1), Value: typ.LiteralString("event")})
	out := s.Apply(set, base)

	if got := out[target.Key()]; !typ.TypeEquals(got, arrString) {
		t.Fatalf("expected array narrowed to string elements, got %v", got)
	}
}

func TestSolver_IndexEqualsPath_NarrowsValue(t *testing.T) {
	target := constraint.Path{Root: "x"}
	value := constraint.Path{Root: "y"}
	event := typ.NewRecord().Field("kind", typ.LiteralString("event")).Build()

	base := map[constraint.PathKey]typ.Type{
		target.Key(): event,
		value.Key():  typ.NewUnion(typ.LiteralString("event"), typ.LiteralString("timeout")),
	}

	s := constraint.Solver{Env: constraint.Env{Resolver: core.Resolver()}}
	set := constraint.NewConjunction(constraint.IndexEqualsPath{Target: target, Key: typ.LiteralString("kind"), Value: value})

	out := s.Apply(set, base)
	if got := out[value.Key()]; !typ.TypeEquals(got, typ.LiteralString("event")) {
		t.Fatalf("expected value narrowed to literal event, got %v", got)
	}
}

func TestSolver_HasType_BuiltinNarrowsWithoutResolver(t *testing.T) {
	path := constraint.Path{Root: "x"}
	base := map[constraint.PathKey]typ.Type{
		path.Key(): typ.NewUnion(typ.String, typ.Number),
	}
	s := constraint.Solver{}

	out := s.Apply(constraint.NewConjunction(constraint.HasType{Path: path, Type: narrow.BuiltinTypeKey("string")}), base)
	if got := out[path.Key()]; !typ.TypeEquals(got, typ.String) {
		t.Fatalf("expected builtin narrowing to String, got %v", got)
	}
}

func unionQueryField(t typ.Type, field string) (typ.Type, bool) {
	switch v := t.(type) {
	case *typ.Record:
		if f := v.GetField(field); f != nil {
			return f.Type, true
		}
	case *typ.Optional:
		if ft, ok := unionQueryField(v.Inner, field); ok && ft != nil {
			return typ.NewOptional(ft), true
		}

		return nil, false
	case *typ.Intersection:
		var parts []typ.Type

		for _, m := range v.Members {
			if ft, ok := unionQueryField(m, field); ok && ft != nil {
				parts = append(parts, ft)
			}
		}

		if len(parts) == 0 {
			return nil, false
		}

		if len(parts) == 1 {
			return parts[0], true
		}

		return typ.NewIntersection(parts...), true
	case *typ.Union:
		var members []typ.Type

		for _, m := range v.Members {
			ft, ok := unionQueryField(m, field)
			if !ok || ft == nil {
				return nil, false
			}

			members = append(members, ft)
		}

		if len(members) == 0 {
			return nil, false
		}

		return typ.NewUnion(members...), true
	}

	return nil, false
}

func TestSolver_FieldEquals_OptionalTarget(t *testing.T) {
	path := constraint.Path{Root: "x"}
	event := typ.NewRecord().Field("kind", typ.LiteralString("event")).Build()
	base := map[constraint.PathKey]typ.Type{
		path.Key(): typ.NewOptional(event),
	}

	s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: unionQueryField}}}

	out := s.Apply(constraint.NewConjunction(constraint.FieldEquals{Target: path, Field: "kind", Value: typ.LiteralString("event")}), base)
	if got := out[path.Key()]; !typ.TypeEquals(got, event) {
		t.Fatalf("expected optional target narrowed to event, got %v", got)
	}
}

func TestSolver_FieldEquals_CompositePath(t *testing.T) {
	path := constraint.Path{Root: "x", Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "inner"}}}
	event := typ.NewRecord().Field("kind", typ.LiteralString("event")).Build()
	timeout := typ.NewRecord().Field("kind", typ.LiteralString("timeout")).Build()
	base := map[constraint.PathKey]typ.Type{
		path.Key(): typ.NewUnion(event, timeout),
	}

	s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: unionQueryField}}}

	out := s.Apply(constraint.NewConjunction(constraint.FieldEquals{Target: path, Field: "kind", Value: typ.LiteralString("event")}), base)
	if got := out[path.Key()]; !typ.TypeEquals(got, event) {
		t.Fatalf("expected composite path narrowed to event, got %v", got)
	}
}

func TestSolver_FieldEquals_IntersectionField(t *testing.T) {
	path := constraint.Path{Root: "x"}
	field := typ.NewIntersection(typ.String, typ.LiteralString("event"))
	event := typ.NewRecord().Field("kind", field).Build()
	timeout := typ.NewRecord().Field("kind", typ.LiteralString("timeout")).Build()
	base := map[constraint.PathKey]typ.Type{
		path.Key(): typ.NewUnion(event, timeout),
	}

	s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: unionQueryField}}}

	out := s.Apply(constraint.NewConjunction(constraint.FieldEquals{Target: path, Field: "kind", Value: typ.LiteralString("event")}), base)
	if got := out[path.Key()]; !typ.TypeEquals(got, event) {
		t.Fatalf("expected intersection field match to narrow, got %v", got)
	}
}

func TestSolver_FieldEqualsPath_CompositeTarget(t *testing.T) {
	target := constraint.Path{Root: "x", Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "inner"}}}
	value := constraint.Path{Root: "y"}
	event := typ.NewRecord().Field("kind", typ.LiteralString("event")).Build()
	timeout := typ.NewRecord().Field("kind", typ.LiteralString("timeout")).Build()

	base := map[constraint.PathKey]typ.Type{
		target.Key(): typ.NewUnion(event, timeout),
		value.Key():  typ.LiteralString("event"),
	}

	s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: unionQueryField}}}

	out := s.Apply(constraint.NewConjunction(constraint.FieldEqualsPath{Target: target, Field: "kind", Value: value}), base)
	if got := out[target.Key()]; !typ.TypeEquals(got, event) {
		t.Fatalf("expected composite target narrowed to event, got %v", got)
	}
}

func TestSolver_HasType_AnyUnknown(t *testing.T) {
	path := constraint.Path{Root: "x"}
	resolver := func(k narrow.TypeKey) typ.Type {
		if k.Kind == narrow.TypeKeyBuiltin && k.Name == "string" {
			return typ.String
		}

		return nil
	}
	s := constraint.Solver{Env: constraint.Env{ResolveType: resolver}}

	outAny := s.Apply(constraint.NewConjunction(constraint.HasType{Path: path, Type: narrow.BuiltinTypeKey("string")}), map[constraint.PathKey]typ.Type{
		path.Key(): typ.Any,
	})
	if got := outAny[path.Key()]; !typ.TypeEquals(got, typ.String) {
		t.Fatalf("expected Any narrowed to String, got %v", got)
	}

	outUnknown := s.Apply(constraint.NewConjunction(constraint.HasType{Path: path, Type: narrow.BuiltinTypeKey("string")}), map[constraint.PathKey]typ.Type{
		path.Key(): typ.Unknown,
	})
	if got := outUnknown[path.Key()]; !typ.TypeEquals(got, typ.String) {
		t.Fatalf("expected Unknown narrowed to String, got %v", got)
	}
}

func TestSolver_EqPath_WithAny(t *testing.T) {
	left := constraint.Path{Root: "a"}
	right := constraint.Path{Root: "b"}
	base := map[constraint.PathKey]typ.Type{
		left.Key():  typ.Any,
		right.Key(): typ.String,
	}
	s := constraint.Solver{}

	out := s.Apply(constraint.NewConjunction(constraint.NewEqPath(left, right)), base)
	if got := out[left.Key()]; !typ.TypeEquals(got, typ.String) {
		t.Fatalf("expected Any intersect String -> String, got %v", got)
	}
}

func TestSolver_EqPath_MissingPath_NoChange(t *testing.T) {
	left := constraint.Path{Root: "a"}
	right := constraint.Path{Root: "b"}
	base := map[constraint.PathKey]typ.Type{
		left.Key(): typ.String,
	}
	s := constraint.Solver{}

	out := s.Apply(constraint.NewConjunction(constraint.NewEqPath(left, right)), base)
	if got := out[left.Key()]; !typ.TypeEquals(got, typ.String) {
		t.Fatalf("expected no change for missing path, got %v", got)
	}
}

func TestSolver_EqPath_PartialOverlap(t *testing.T) {
	left := constraint.Path{Root: "a"}
	right := constraint.Path{Root: "b"}
	base := map[constraint.PathKey]typ.Type{
		left.Key():  typ.NewUnion(typ.String, typ.Number),
		right.Key(): typ.NewUnion(typ.Number, typ.Boolean),
	}
	s := constraint.Solver{}

	out := s.Apply(constraint.NewConjunction(constraint.NewEqPath(left, right)), base)
	if got := out[left.Key()]; !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("expected left narrowed to Number, got %v", got)
	}

	if got := out[right.Key()]; !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("expected right narrowed to Number, got %v", got)
	}
}

func TestSolver_FieldEquals_NoResolver_NoChange(t *testing.T) {
	path := constraint.Path{Root: "x"}
	event := typ.NewRecord().Field("kind", typ.LiteralString("event")).Build()
	base := map[constraint.PathKey]typ.Type{
		path.Key(): event,
	}
	s := constraint.Solver{}

	out := s.Apply(constraint.NewConjunction(constraint.FieldEquals{Target: path, Field: "kind", Value: typ.LiteralString("event")}), base)
	if got := out[path.Key()]; !typ.TypeEquals(got, event) {
		t.Fatalf("expected no change without Resolver, got %v", got)
	}
}

func TestSolver_IndexEquals_NoResolver_NoChange(t *testing.T) {
	path := constraint.Path{Root: "x"}
	arrString := typ.NewArray(typ.String)
	base := map[constraint.PathKey]typ.Type{
		path.Key(): arrString,
	}
	s := constraint.Solver{}

	out := s.Apply(constraint.NewConjunction(constraint.IndexEquals{Target: path, Key: typ.LiteralInt(1), Value: typ.LiteralString("event")}), base)
	if got := out[path.Key()]; !typ.TypeEquals(got, arrString) {
		t.Fatalf("expected no change without Resolver, got %v", got)
	}
}

func TestSolver_FieldEquals_OptionalRecord(t *testing.T) {
	path := constraint.Path{Root: "x"}
	event := typ.NewRecord().Field("kind", typ.LiteralString("event")).Build()
	timeout := typ.NewRecord().Field("kind", typ.LiteralString("timeout")).Build()
	base := map[constraint.PathKey]typ.Type{
		path.Key(): typ.NewUnion(typ.NewOptional(event), timeout),
	}
	s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: unionQueryField}}}

	out := s.Apply(constraint.NewConjunction(constraint.FieldEquals{Target: path, Field: "kind", Value: typ.LiteralString("event")}), base)
	if got := out[path.Key()]; !typ.TypeEquals(got, event) {
		t.Fatalf("expected optional record narrowed to event, got %v", got)
	}
}

func TestSolver_FieldEqualsPath_NarrowsValue(t *testing.T) {
	target := constraint.Path{Root: "x"}
	value := constraint.Path{Root: "y"}
	event := typ.NewRecord().Field("kind", typ.LiteralString("event")).Build()

	base := map[constraint.PathKey]typ.Type{
		target.Key(): event,
		value.Key():  typ.NewUnion(typ.LiteralString("event"), typ.LiteralString("timeout")),
	}

	queryField := func(t typ.Type, field string) (typ.Type, bool) {
		if rec, ok := t.(*typ.Record); ok {
			if f := rec.GetField(field); f != nil {
				return f.Type, true
			}
		}

		return nil, false
	}

	s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: queryField}}}
	set := constraint.NewConjunction(constraint.FieldEqualsPath{Target: target, Field: "kind", Value: value})

	out := s.Apply(set, base)
	if got := out[value.Key()]; !typ.TypeEquals(got, typ.LiteralString("event")) {
		t.Fatalf("expected value narrowed to literal event, got %v", got)
	}
}

func TestSolver_TruthyFalsy_AnyUnknown(t *testing.T) {
	path := constraint.Path{Root: "x"}
	s := constraint.Solver{}

	outAnyFalsy := s.Apply(constraint.NewConjunction(constraint.Falsy{Path: path}), map[constraint.PathKey]typ.Type{path.Key(): typ.Any})
	wantFalsy := typ.NewUnion(typ.Nil, typ.LiteralBool(false))

	if got := outAnyFalsy[path.Key()]; !typ.TypeEquals(got, wantFalsy) {
		t.Fatalf("expected falsy(any) -> nil|false, got %v", got)
	}

	outUnknownFalsy := s.Apply(constraint.NewConjunction(constraint.Falsy{Path: path}), map[constraint.PathKey]typ.Type{path.Key(): typ.Unknown})
	if got := outUnknownFalsy[path.Key()]; !typ.TypeEquals(got, wantFalsy) {
		t.Fatalf("expected falsy(unknown) -> nil|false, got %v", got)
	}
}

func TestSolver_OrderInvariance(t *testing.T) {
	path := constraint.Path{Root: "x"}
	resolver := func(k narrow.TypeKey) typ.Type {
		if k.Kind == narrow.TypeKeyBuiltin && k.Name == "string" {
			return typ.String
		}

		return nil
	}
	s := constraint.Solver{Env: constraint.Env{ResolveType: resolver}}
	base := map[constraint.PathKey]typ.Type{
		path.Key(): typ.NewUnion(typ.String, typ.Nil),
	}

	setA := constraint.NewConjunction(constraint.HasType{Path: path, Type: narrow.BuiltinTypeKey("string")}, constraint.NotNil{Path: path})
	setB := constraint.NewConjunction(constraint.NotNil{Path: path}, constraint.HasType{Path: path, Type: narrow.BuiltinTypeKey("string")})

	outA := s.Apply(setA, base)
	outB := s.Apply(setB, base)

	if !typ.TypeEquals(outA[path.Key()], outB[path.Key()]) {
		t.Fatalf("expected order invariance, got %v vs %v", outA[path.Key()], outB[path.Key()])
	}

	if !typ.TypeEquals(outA[path.Key()], typ.String) {
		t.Fatalf("expected final type String, got %v", outA[path.Key()])
	}
}

func TestSolver_MixedConstraints_FixedPoint(t *testing.T) {
	a := constraint.Path{Root: "a"}
	b := constraint.Path{Root: "b"}
	resolver := func(k narrow.TypeKey) typ.Type {
		if k.Kind == narrow.TypeKeyBuiltin && k.Name == "string" {
			return typ.String
		}

		return nil
	}
	s := constraint.Solver{Env: constraint.Env{ResolveType: resolver}}
	base := map[constraint.PathKey]typ.Type{
		a.Key(): typ.Any,
		b.Key(): typ.NewUnion(typ.String, typ.Number),
	}
	set := constraint.NewConjunction(
		constraint.NewEqPath(a, b),
		constraint.HasType{Path: b, Type: narrow.BuiltinTypeKey("string")},
	)

	out := s.Apply(set, base)
	if got := out[a.Key()]; !typ.TypeEquals(got, typ.String) {
		t.Fatalf("expected a to converge to String, got %v", got)
	}

	if got := out[b.Key()]; !typ.TypeEquals(got, typ.String) {
		t.Fatalf("expected b to converge to String, got %v", got)
	}
}

func TestSolver_NotHasType_Builtin(t *testing.T) {
	path := constraint.Path{Root: "x"}
	base := map[constraint.PathKey]typ.Type{
		path.Key(): typ.NewUnion(typ.String, typ.Number, typ.Boolean),
	}
	s := constraint.Solver{}

	out := s.Apply(constraint.NewConjunction(constraint.NotHasType{Path: path, Type: narrow.BuiltinTypeKey("string")}), base)
	want := typ.NewUnion(typ.Number, typ.Boolean)
	if got := out[path.Key()]; !typ.TypeEquals(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestSolver_NotHasType_Hash(t *testing.T) {
	path := constraint.Path{Root: "x"}
	msgRec := typ.NewRecord().Field("channel", typ.String).Field("value", typ.String).Build()
	timeRec := typ.NewRecord().Field("channel", typ.Number).Field("value", typ.Number).Build()
	union := typ.NewUnion(msgRec, timeRec)
	base := map[constraint.PathKey]typ.Type{
		path.Key(): union,
	}

	resolver := func(k narrow.TypeKey) typ.Type {
		switch k.Kind {
		case narrow.TypeKeyHash:
			if k.Hash == timeRec.Hash() {
				return timeRec
			}
			if k.Hash == msgRec.Hash() {
				return msgRec
			}
		}
		return nil
	}

	s := constraint.Solver{Env: constraint.Env{ResolveType: resolver}}
	out := s.Apply(constraint.NewConjunction(constraint.NotHasType{Path: path, Type: narrow.HashTypeKey(timeRec.Hash())}), base)

	if got := out[path.Key()]; !typ.TypeEquals(got, msgRec) {
		t.Fatalf("NotHasType hash: expected %v, got %v", msgRec, got)
	}
}

func TestSolver_HasField_NarrowsUnion(t *testing.T) {
	path := constraint.Path{Root: "x"}
	withField := typ.NewRecord().Field("name", typ.String).Build()
	withoutField := typ.NewRecord().Field("age", typ.Number).Build()
	base := map[constraint.PathKey]typ.Type{
		path.Key(): typ.NewUnion(withField, withoutField),
	}

	s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: unionQueryField}}}
	out := s.Apply(constraint.NewConjunction(constraint.HasField{Path: path, Field: "name"}), base)

	if got := out[path.Key()]; !typ.TypeEquals(got, withField) {
		t.Fatalf("expected record with name field, got %v", got)
	}
}

func TestSolver_FieldNotEquals_Literal(t *testing.T) {
	path := constraint.Path{Root: "x"}
	event := typ.NewRecord().Field("kind", typ.LiteralString("event")).Build()
	timeout := typ.NewRecord().Field("kind", typ.LiteralString("timeout")).Build()
	base := map[constraint.PathKey]typ.Type{
		path.Key(): typ.NewUnion(event, timeout),
	}

	s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: unionQueryField}}}
	set := constraint.NewConjunction(constraint.FieldNotEquals{Target: path, Field: "kind", Value: typ.LiteralString("event")})

	out := s.Apply(set, base)
	if got := out[path.Key()]; !typ.TypeEquals(got, timeout) {
		t.Fatalf("expected timeout variant only, got %v", got)
	}
}

func TestSolver_IndexNotEqualsPath_ExcludesVariant(t *testing.T) {
	target := constraint.Path{Root: "x"}
	value := constraint.Path{Root: "y"}
	event := typ.NewRecord().Field("kind", typ.LiteralString("event")).Build()
	timeout := typ.NewRecord().Field("kind", typ.LiteralString("timeout")).Build()

	base := map[constraint.PathKey]typ.Type{
		target.Key(): typ.NewUnion(event, timeout),
		value.Key():  typ.LiteralString("event"),
	}

	s := constraint.Solver{Env: constraint.Env{Resolver: core.Resolver()}}
	set := constraint.NewConjunction(constraint.IndexNotEqualsPath{Target: target, Key: typ.LiteralString("kind"), Value: value})

	out := s.Apply(set, base)
	if got := out[target.Key()]; !typ.TypeEquals(got, timeout) {
		t.Fatalf("expected timeout variant only, got %v", got)
	}
}

func TestSolver_NotEqPath_NarrowsForSingletonTypes(t *testing.T) {
	// NotEqPath should narrow for singleton types (nil, literals)
	// because value inequality implies type difference for singletons
	left := constraint.Path{Root: "a"}
	right := constraint.Path{Root: "b"}

	base := map[constraint.PathKey]typ.Type{
		left.Key():  typ.NewUnion(typ.String, typ.Nil),
		right.Key(): typ.Nil,
	}

	s := constraint.Solver{}
	set := constraint.NewConjunction(constraint.NewNotEqPath(left, right))

	out := s.Apply(set, base)
	if got := out[left.Key()]; !typ.TypeEquals(got, typ.String) {
		t.Fatalf("expected left narrowed to string (nil excluded), got %v", got)
	}
}

func TestSolver_NotEqPath_NoNarrowingForStructuralTypes(t *testing.T) {
	// NotEqPath should NOT narrow for structural types (records/tables)
	// because in Lua, x ~= y compares by reference, not structure.
	// Two different objects of the same type can be not equal.
	left := constraint.Path{Root: "a"}
	right := constraint.Path{Root: "b"}

	typeA := typ.NewRecord().Field("tag", typ.LiteralString("a")).Build()
	typeB := typ.NewRecord().Field("tag", typ.LiteralString("b")).Build()

	base := map[constraint.PathKey]typ.Type{
		left.Key():  typ.NewUnion(typeA, typeB),
		right.Key(): typeA,
	}

	s := constraint.Solver{}
	set := constraint.NewConjunction(constraint.NewNotEqPath(left, right))

	out := s.Apply(set, base)
	// Should NOT narrow - both variants are possible after x ~= y
	expected := typ.NewUnion(typeA, typeB)
	if got := out[left.Key()]; !typ.TypeEquals(got, expected) {
		t.Fatalf("expected no narrowing for structural types, got %v", got)
	}
}

func TestSolver_NotEqPath_NarrowsForLiterals(t *testing.T) {
	// NotEqPath should narrow when comparing with literal types
	left := constraint.Path{Root: "a"}
	right := constraint.Path{Root: "b"}

	litA := typ.LiteralString("a")
	litB := typ.LiteralString("b")

	base := map[constraint.PathKey]typ.Type{
		left.Key():  typ.NewUnion(litA, litB),
		right.Key(): litA,
	}

	s := constraint.Solver{}
	set := constraint.NewConjunction(constraint.NewNotEqPath(left, right))

	out := s.Apply(set, base)
	if got := out[left.Key()]; !typ.TypeEquals(got, litB) {
		t.Fatalf("expected left narrowed to \"b\" literal, got %v", got)
	}
}

func TestSolver_TruthyFalsy_BooleanDiscriminant(t *testing.T) {
	path := constraint.Path{Root: "r", Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "ok"}}}
	parent := constraint.Path{Root: "r"}
	okVariant := typ.NewRecord().Field("ok", typ.True).Field("value", typ.String).Build()
	errVariant := typ.NewRecord().Field("ok", typ.False).Field("value", typ.Number).Build()

	base := map[constraint.PathKey]typ.Type{
		path.Key():   typ.Boolean,
		parent.Key(): typ.NewUnion(okVariant, errVariant),
	}

	tests := []struct {
		name       string
		constraint constraint.Constraint
		expect     typ.Type
	}{
		{"truthy selects ok", constraint.Truthy{Path: path}, okVariant},
		{"falsy selects err", constraint.Falsy{Path: path}, errVariant},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: unionQueryField}}}
			out := s.Apply(constraint.NewConjunction(tt.constraint), base)
			if got := out[parent.Key()]; !typ.TypeEquals(got, tt.expect) {
				t.Fatalf("expected %v, got %v", tt.expect, got)
			}
		})
	}
}

func TestSolver_FieldLiteral_NestedPath_PropagatesUp(t *testing.T) {
	meta := constraint.Path{Root: "r", Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "meta"}}}
	parent := constraint.Path{Root: "r"}

	typeA := typ.NewRecord().Field("meta", typ.NewRecord().Field("tag", typ.LiteralString("a")).Field("v", typ.String).Build()).Build()
	typeB := typ.NewRecord().Field("meta", typ.NewRecord().Field("tag", typ.LiteralString("b")).Field("v", typ.Number).Build()).Build()

	base := map[constraint.PathKey]typ.Type{
		meta.Key():   typ.NewUnion(typ.NewRecord().Field("tag", typ.LiteralString("a")).Field("v", typ.String).Build(), typ.NewRecord().Field("tag", typ.LiteralString("b")).Field("v", typ.Number).Build()),
		parent.Key(): typ.NewUnion(typeA, typeB),
	}

	tests := []struct {
		name       string
		constraint constraint.Constraint
		expect     typ.Type
	}{
		{"equals selects A", constraint.FieldEquals{Target: meta, Field: "tag", Value: typ.LiteralString("a")}, typeA},
		{"not equals excludes A", constraint.FieldNotEquals{Target: meta, Field: "tag", Value: typ.LiteralString("a")}, typeB},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: unionQueryField}}}
			out := s.Apply(constraint.NewConjunction(tt.constraint), base)
			if got := out[parent.Key()]; !typ.TypeEquals(got, tt.expect) {
				t.Fatalf("expected %v, got %v", tt.expect, got)
			}
		})
	}
}

func TestSolver_FieldPath_StructuralEquality(t *testing.T) {
	target := constraint.Path{Root: "result"}
	value := constraint.Path{Root: "ch1"}

	// Create two SEPARATE instances of structurally identical channel types.
	// This simulates source code parsing where types from different places
	// are structurally equal but not pointer-equal.
	chIntInUnion := typ.NewRecord().Field("__tag", typ.LiteralString("int")).Build()
	chStrInUnion := typ.NewRecord().Field("__tag", typ.LiteralString("str")).Build()
	resultInt := typ.NewRecord().Field("channel", chIntInUnion).Field("value", typ.Number).Build()
	resultStr := typ.NewRecord().Field("channel", chStrInUnion).Field("value", typ.String).Build()

	// ch1 is a DIFFERENT instance with same structure as chIntInUnion
	ch1Type := typ.NewRecord().Field("__tag", typ.LiteralString("int")).Build()

	base := map[constraint.PathKey]typ.Type{
		target.Key(): typ.NewUnion(resultInt, resultStr),
		value.Key():  ch1Type,
	}

	tests := []struct {
		name       string
		constraint constraint.Constraint
		expect     typ.Type
	}{
		{
			name:       "equals selects matching",
			constraint: constraint.FieldEqualsPath{Target: target, Field: "channel", Value: value},
			expect:     resultInt,
		},
		{
			name:       "not equals excludes matching",
			constraint: constraint.FieldNotEqualsPath{Target: target, Field: "channel", Value: value},
			expect:     resultStr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: unionQueryField}}}
			set := constraint.NewConjunction(tt.constraint)
			out := s.Apply(set, base)
			if got := out[target.Key()]; !typ.TypeEquals(got, tt.expect) {
				t.Fatalf("expected %v, got %v", tt.expect, got)
			}
		})
	}
}

func TestSolver_ApplyToSingle_NotNil(t *testing.T) {
	path := constraint.Path{Root: "x"}
	resolve := func(p constraint.Path) constraint.PathKey {
		return constraint.PathKey(p.Root + "@1")
	}
	target := constraint.PathKey("x@1")
	base := typ.NewOptional(typ.String)

	s := constraint.Solver{}
	set := constraint.NewConjunction(constraint.NotNil{Path: path})

	got := s.ApplyToSingle(set, target, base, resolve)
	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("ApplyToSingle NotNil: expected string, got %v", got)
	}
}

func TestSolver_ApplyToSingle_MismatchedPath(t *testing.T) {
	pathX := constraint.Path{Root: "x"}
	resolve := func(p constraint.Path) constraint.PathKey {
		return constraint.PathKey(p.Root + "@1")
	}
	target := constraint.PathKey("y@1") // Different from x@1
	base := typ.NewOptional(typ.String)

	s := constraint.Solver{}
	set := constraint.NewConjunction(constraint.NotNil{Path: pathX})

	got := s.ApplyToSingle(set, target, base, resolve)
	if !typ.TypeEquals(got, base) {
		t.Fatalf("ApplyToSingle mismatched path: expected base unchanged, got %v", got)
	}
}

func TestSolver_ApplyToSingle_Truthy(t *testing.T) {
	path := constraint.Path{Root: "x"}
	resolve := func(p constraint.Path) constraint.PathKey {
		return constraint.PathKey(p.Root + "@1")
	}
	target := constraint.PathKey("x@1")
	base := typ.NewUnion(typ.String, typ.Nil, typ.Boolean)

	s := constraint.Solver{}
	set := constraint.NewConjunction(constraint.Truthy{Path: path})

	got := s.ApplyToSingle(set, target, base, resolve)
	want := typ.NewUnion(typ.String, typ.True)
	if !typ.TypeEquals(got, want) {
		t.Fatalf("ApplyToSingle Truthy: expected %v, got %v", want, got)
	}
}

func TestSolver_ApplyToSingle_HasType(t *testing.T) {
	path := constraint.Path{Root: "x"}
	resolve := func(p constraint.Path) constraint.PathKey {
		return constraint.PathKey(p.Root + "@1")
	}
	target := constraint.PathKey("x@1")
	base := typ.NewUnion(typ.String, typ.Number)

	resolver := func(k narrow.TypeKey) typ.Type {
		if k.Kind == narrow.TypeKeyBuiltin && k.Name == "string" {
			return typ.String
		}
		return nil
	}
	s := constraint.Solver{Env: constraint.Env{ResolveType: resolver}}
	set := constraint.NewConjunction(constraint.HasType{Path: path, Type: narrow.BuiltinTypeKey("string")})

	got := s.ApplyToSingle(set, target, base, resolve)
	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("ApplyToSingle HasType: expected string, got %v", got)
	}
}

func TestSolver_ApplyToSingle_MultipleConstraints(t *testing.T) {
	path := constraint.Path{Root: "x"}
	resolve := func(p constraint.Path) constraint.PathKey {
		return constraint.PathKey(p.Root + "@1")
	}
	target := constraint.PathKey("x@1")
	base := typ.NewUnion(typ.String, typ.Nil)

	resolver := func(k narrow.TypeKey) typ.Type {
		if k.Kind == narrow.TypeKeyBuiltin && k.Name == "string" {
			return typ.String
		}
		return nil
	}
	s := constraint.Solver{Env: constraint.Env{ResolveType: resolver}}
	set := constraint.NewConjunction(
		constraint.NotNil{Path: path},
		constraint.HasType{Path: path, Type: narrow.BuiltinTypeKey("string")},
	)

	got := s.ApplyToSingle(set, target, base, resolve)
	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("ApplyToSingle multiple: expected string, got %v", got)
	}
}

func TestSolver_ApplyToSingle_EmptySet(t *testing.T) {
	resolve := func(p constraint.Path) constraint.PathKey {
		return constraint.PathKey(p.Root + "@1")
	}
	target := constraint.PathKey("x@1")
	base := typ.NewOptional(typ.String)

	s := constraint.Solver{}
	set := constraint.NewConjunction()

	got := s.ApplyToSingle(set, target, base, resolve)
	if !typ.TypeEquals(got, base) {
		t.Fatalf("ApplyToSingle empty set: expected base unchanged, got %v", got)
	}
}

func TestSolver_ApplyToSingle_NilBase(t *testing.T) {
	path := constraint.Path{Root: "x"}
	resolve := func(p constraint.Path) constraint.PathKey {
		return constraint.PathKey(p.Root + "@1")
	}
	target := constraint.PathKey("x@1")

	s := constraint.Solver{}
	set := constraint.NewConjunction(constraint.NotNil{Path: path})

	got := s.ApplyToSingle(set, target, nil, resolve)
	if got != nil {
		t.Fatalf("ApplyToSingle nil base: expected nil, got %v", got)
	}
}

// TestSolver_ApplyToSingle_FieldPath tests that FieldEqualsPath and FieldNotEqualsPath narrow
// a union type by keeping/excluding variants where the field type matches the value's type.
func TestSolver_ApplyToSingle_FieldPath(t *testing.T) {
	channelIntType := typ.NewRecord().Field("value", typ.Integer).Build()
	channelStrType := typ.NewRecord().Field("value", typ.String).Build()
	resultIntVariant := typ.NewRecord().Field("channel", channelIntType).Field("value", typ.Integer).Build()
	resultStrVariant := typ.NewRecord().Field("channel", channelStrType).Field("value", typ.String).Build()
	resultType := typ.NewUnion(resultIntVariant, resultStrVariant)

	resultPath := constraint.Path{Root: "result"}
	timeoutPath := constraint.Path{Root: "timeout"}

	resolve := func(p constraint.Path) constraint.PathKey {
		return constraint.PathKey(p.Root + "@1")
	}
	pathTypeAt := func(key constraint.PathKey) typ.Type {
		if key == "timeout@1" {
			return channelStrType
		}
		return nil
	}
	target := constraint.PathKey("result@1")

	tests := []struct {
		name       string
		constraint constraint.Constraint
		want       typ.Type
	}{
		{
			"equals selects matching",
			constraint.FieldEqualsPath{Target: resultPath, Field: "channel", Value: timeoutPath},
			resultStrVariant,
		},
		{
			"not equals excludes matching",
			constraint.FieldNotEqualsPath{Target: resultPath, Field: "channel", Value: timeoutPath},
			resultIntVariant,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := constraint.Solver{
				Env: constraint.Env{Resolver: core.Resolver(), PathTypeAt: pathTypeAt},
			}
			got := s.ApplyToSingle(constraint.NewConjunction(tt.constraint), target, resultType, resolve)
			if !typ.TypeEquals(got, tt.want) {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

// TestSolver_HasType_Intersection tests that HasType narrows each member of an Intersection.
func TestSolver_HasType_Intersection(t *testing.T) {
	path := constraint.Path{Root: "x"}
	// Intersection of (string|number) & (string|boolean)
	inter := typ.NewIntersection(
		typ.NewUnion(typ.String, typ.Number),
		typ.NewUnion(typ.String, typ.Boolean),
	)
	base := map[constraint.PathKey]typ.Type{
		path.Key(): inter,
	}

	s := constraint.Solver{}
	out := s.Apply(constraint.NewConjunction(constraint.HasType{Path: path, Type: narrow.BuiltinTypeKey("string")}), base)

	// Both members should narrow to string, resulting in string & string = string
	if got := out[path.Key()]; !typ.TypeEquals(got, typ.String) {
		t.Fatalf("HasType on Intersection: expected string, got %v", got)
	}
}

// TestSolver_NotNil_Intersection tests that NotNil removes nil from each member of an Intersection.
func TestSolver_NotNil_Intersection(t *testing.T) {
	path := constraint.Path{Root: "x"}
	// Intersection of (string?) & (number?)
	inter := typ.NewIntersection(
		typ.NewOptional(typ.String),
		typ.NewOptional(typ.Number),
	)
	base := map[constraint.PathKey]typ.Type{
		path.Key(): inter,
	}

	s := constraint.Solver{}
	out := s.Apply(constraint.NewConjunction(constraint.NotNil{Path: path}), base)

	// Both should narrow to non-optional, resulting in string & number
	got := out[path.Key()]
	if got.Kind() != kind.Intersection {
		t.Fatalf("NotNil on Intersection: expected Intersection kind, got %v", got.Kind())
	}
}

// TestSolver_Truthy_Intersection tests that Truthy narrows each member of an Intersection.
func TestSolver_Truthy_Intersection(t *testing.T) {
	path := constraint.Path{Root: "x"}
	// Intersection of (string|nil) & (string|false)
	inter := typ.NewIntersection(
		typ.NewUnion(typ.String, typ.Nil),
		typ.NewUnion(typ.String, typ.LiteralBool(false)),
	)
	base := map[constraint.PathKey]typ.Type{
		path.Key(): inter,
	}

	s := constraint.Solver{}
	out := s.Apply(constraint.NewConjunction(constraint.Truthy{Path: path}), base)

	// Both should narrow to string, resulting in string & string = string
	if got := out[path.Key()]; !typ.TypeEquals(got, typ.String) {
		t.Fatalf("Truthy on Intersection: expected string, got %v", got)
	}
}

// TestSolver_NotHasType_Intersection tests that NotHasType excludes from each member of an Intersection.
func TestSolver_NotHasType_Intersection(t *testing.T) {
	path := constraint.Path{Root: "x"}
	// Intersection of (string|number) & (string|boolean)
	inter := typ.NewIntersection(
		typ.NewUnion(typ.String, typ.Number),
		typ.NewUnion(typ.String, typ.Boolean),
	)
	base := map[constraint.PathKey]typ.Type{
		path.Key(): inter,
	}

	s := constraint.Solver{}
	out := s.Apply(constraint.NewConjunction(constraint.NotHasType{Path: path, Type: narrow.BuiltinTypeKey("string")}), base)

	// Both should exclude string: number & boolean
	got := out[path.Key()]
	if got.Kind() != kind.Intersection {
		t.Fatalf("NotHasType on Intersection: expected Intersection kind, got %v", got.Kind())
	}
	inter2 := got.(*typ.Intersection)
	if len(inter2.Members) != 2 {
		t.Fatalf("NotHasType on Intersection: expected 2 members, got %d", len(inter2.Members))
	}
}

// TestSolver_FieldEquals_Intersection tests narrowing an Intersection by field literal match.
func TestSolver_FieldEquals_Intersection(t *testing.T) {
	path := constraint.Path{Root: "x"}
	event := typ.NewRecord().Field("kind", typ.LiteralString("event")).Build()
	timeout := typ.NewRecord().Field("kind", typ.LiteralString("timeout")).Build()
	// Intersection of (event|timeout) & (event|timeout)
	inter := typ.NewIntersection(
		typ.NewUnion(event, timeout),
		typ.NewUnion(event, timeout),
	)
	base := map[constraint.PathKey]typ.Type{
		path.Key(): inter,
	}

	s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: unionQueryField}}}
	set := constraint.NewConjunction(constraint.FieldEquals{Target: path, Field: "kind", Value: typ.LiteralString("event")})

	out := s.Apply(set, base)
	// Both should narrow to event, resulting in event & event = event
	if got := out[path.Key()]; !typ.TypeEquals(got, event) {
		t.Fatalf("FieldEquals on Intersection: expected event, got %v", got)
	}
}

// TestSolver_HasField_Intersection tests narrowing an Intersection with HasField.
func TestSolver_HasField_Intersection(t *testing.T) {
	path := constraint.Path{Root: "x"}
	withName := typ.NewRecord().Field("name", typ.String).Build()
	withAge := typ.NewRecord().Field("age", typ.Number).Build()
	withBoth := typ.NewRecord().Field("name", typ.String).Field("age", typ.Number).Build()
	// Intersection of (withName|withAge) & (withBoth|withAge)
	inter := typ.NewIntersection(
		typ.NewUnion(withName, withAge),
		typ.NewUnion(withBoth, withAge),
	)
	base := map[constraint.PathKey]typ.Type{
		path.Key(): inter,
	}

	s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: unionQueryField}}}
	out := s.Apply(constraint.NewConjunction(constraint.HasField{Path: path, Field: "name"}), base)

	// First member narrows to withName, second member narrows to withBoth
	got := out[path.Key()]
	if got.Kind() != kind.Intersection {
		t.Fatalf("HasField on Intersection: expected Intersection kind, got %v", got.Kind())
	}
}

// TestSolver_HasType_Instantiated tests narrowing an Instantiated generic type.
func TestSolver_HasType_Instantiated(t *testing.T) {
	path := constraint.Path{Root: "x"}
	// Generic: Box<T> = { value: T }
	tParam := typ.NewTypeParam("T", nil)
	boxBody := typ.NewRecord().Field("value", tParam).Build()
	boxGeneric := typ.NewGeneric("Box", []*typ.TypeParam{tParam}, boxBody)

	// Instantiate: Box<string|number>
	boxStringOrNumber := typ.Instantiate(boxGeneric, typ.NewUnion(typ.String, typ.Number))

	base := map[constraint.PathKey]typ.Type{
		path.Key(): boxStringOrNumber,
	}

	resolver := func(k narrow.TypeKey) typ.Type {
		if k.Kind == narrow.TypeKeyBuiltin && k.Name == "table" {
			return typ.NewRecord().Build()
		}
		return nil
	}
	s := constraint.Solver{Env: constraint.Env{ResolveType: resolver}}

	// HasType table should match the instantiated record
	out := s.Apply(constraint.NewConjunction(constraint.HasType{Path: path, Type: narrow.BuiltinTypeKey("table")}), base)

	// Should pass through (instantiated types that are records match table)
	got := out[path.Key()]
	if got == nil || got.Kind() == kind.Never {
		t.Fatalf("HasType table on Instantiated Record: expected non-never, got %v", got)
	}
}

// TestSolver_FieldEquals_Instantiated tests narrowing an Instantiated type via field access.
func TestSolver_FieldEquals_Instantiated(t *testing.T) {
	path := constraint.Path{Root: "x"}
	// Generic: Tagged<T> = { tag: T, value: string }
	tParam := typ.NewTypeParam("T", nil)
	taggedBody := typ.NewRecord().Field("tag", tParam).Field("value", typ.String).Build()
	taggedGeneric := typ.NewGeneric("Tagged", []*typ.TypeParam{tParam}, taggedBody)

	// Instantiate: Tagged<"event"|"timeout">
	taggedEventOrTimeout := typ.Instantiate(taggedGeneric, typ.NewUnion(
		typ.LiteralString("event"),
		typ.LiteralString("timeout"),
	))

	base := map[constraint.PathKey]typ.Type{
		path.Key(): taggedEventOrTimeout,
	}

	// Query field that understands generics would need to expand the Instantiated type
	queryField := func(t typ.Type, field string) (typ.Type, bool) {
		// For Instantiated types, we need to handle them specially
		if inst, ok := t.(*typ.Instantiated); ok {
			if rec, ok := inst.Generic.Body.(*typ.Record); ok {
				if f := rec.GetField(field); f != nil {
					// If field type is a TypeParam, return the corresponding arg
					if tp, ok := f.Type.(*typ.TypeParam); ok {
						for i, p := range inst.Generic.TypeParams {
							if p.Name == tp.Name && i < len(inst.TypeArgs) {
								return inst.TypeArgs[i], true
							}
						}
					}
					return f.Type, true
				}
			}
		}
		if rec, ok := t.(*typ.Record); ok {
			if f := rec.GetField(field); f != nil {
				return f.Type, true
			}
		}
		return nil, false
	}

	s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: queryField}}}
	set := constraint.NewConjunction(constraint.FieldEquals{Target: path, Field: "tag", Value: typ.LiteralString("event")})

	out := s.Apply(set, base)
	got := out[path.Key()]
	// The instantiated type should be narrowed (implementation detail how)
	if got == nil {
		t.Fatalf("FieldEquals on Instantiated: expected non-nil result")
	}
}

// Regression: filterExclude must handle top-level Optional.
// Excluding the inner type from Optional<T> should leave nil.
func TestSolver_FieldNotEquals_OptionalExclusion(t *testing.T) {
	path := constraint.Path{Root: "x"}
	rec := typ.NewRecord().Field("role", typ.LiteralString("admin")).Build()
	optRec := typ.NewOptional(rec)

	base := map[constraint.PathKey]typ.Type{
		path.Key(): optRec,
	}

	s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: unionQueryField}}}
	set := constraint.NewConjunction(constraint.FieldNotEquals{
		Target: path,
		Field:  "role",
		Value:  typ.LiteralString("admin"),
	})

	out := s.Apply(set, base)
	got := out[path.Key()]

	if got == nil {
		t.Fatal("expected non-nil result for Optional exclusion")
	}
	if !typ.TypeEquals(got, typ.Nil) {
		t.Errorf("FieldNotEquals on Optional<Record{role:'admin'}> = %v, want nil", got)
	}
}

// Regression: typesEquivalent must use pointer identity for interfaces.
// Two structurally identical interfaces from different declarations must not
// be considered equivalent, even if they have the same hash.
func TestSolver_FieldNotEqualsPath_InterfaceIdentity(t *testing.T) {
	path := constraint.Path{Root: "r"}
	valuePath := constraint.Path{Root: "v"}

	// Create two interfaces with different names so they have different hashes.
	// This ensures the union doesn't deduplicate them, letting us test pointer identity.
	iface1 := typ.NewInterface("ChannelA", nil)
	iface2 := typ.NewInterface("ChannelB", nil)

	rec1 := typ.NewRecord().Field("ch", iface1).Build()
	rec2 := typ.NewRecord().Field("ch", iface2).Build()

	base := map[constraint.PathKey]typ.Type{
		path.Key():      typ.NewUnion(rec1, rec2),
		valuePath.Key(): iface1,
	}

	s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: unionQueryField}}}
	set := constraint.NewConjunction(constraint.FieldNotEqualsPath{
		Target: path,
		Field:  "ch",
		Value:  valuePath,
	})

	out := s.Apply(set, base)
	got := out[path.Key()]

	// rec1 has ch: iface1, which equals the value type iface1 (same pointer),
	// so rec1 should be excluded.
	// rec2 has ch: iface2, which is NOT equal to iface1 (different pointer),
	// so rec2 should be kept.
	if got == nil {
		t.Fatal("expected non-nil result after interface exclusion")
	}
	if !typ.TypeEquals(got, rec2) {
		t.Errorf("interface identity: got %v, want rec2 (ch: iface2)", got)
	}
}

// Regression: nested field exclusion must use exact-literal matching.
// Excluding user.role == "admin" from a union where one member has role: string
// must keep the string-typed member.
func TestSolver_FieldNotEquals_NestedExactLiteral(t *testing.T) {
	userPath := constraint.Path{
		Root:     "r",
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "user"}},
	}
	parentPath := constraint.Path{Root: "r"}

	adminRole := typ.NewRecord().Field("role", typ.LiteralString("admin")).Build()
	stringRole := typ.NewRecord().Field("role", typ.String).Build()

	typeA := typ.NewRecord().Field("user", adminRole).Build()
	typeB := typ.NewRecord().Field("user", stringRole).Build()

	base := map[constraint.PathKey]typ.Type{
		userPath.Key():   typ.NewUnion(adminRole, stringRole),
		parentPath.Key(): typ.NewUnion(typeA, typeB),
	}

	s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: unionQueryField}}}
	set := constraint.NewConjunction(constraint.FieldNotEquals{
		Target: userPath,
		Field:  "role",
		Value:  typ.LiteralString("admin"),
	})

	out := s.Apply(set, base)
	gotParent := out[parentPath.Key()]

	if gotParent == nil {
		t.Fatal("expected non-nil parent after nested field exclusion")
	}
	if !typ.TypeEquals(gotParent, typeB) {
		t.Errorf("nested exclusion of user.role=='admin': parent = %v, want typeB (role: string)", gotParent)
	}
}

// TestSolver_MultiplePathsMultipleConstraints tests applying constraints across multiple paths.
func TestSolver_MultiplePathsMultipleConstraints(t *testing.T) {
	pathX := constraint.Path{Root: "x"}
	pathY := constraint.Path{Root: "y"}
	pathZ := constraint.Path{Root: "z"}

	typeA := typ.NewRecord().Field("tag", typ.LiteralString("a")).Build()
	typeB := typ.NewRecord().Field("tag", typ.LiteralString("b")).Build()
	typeC := typ.NewRecord().Field("tag", typ.LiteralString("c")).Build()

	base := map[constraint.PathKey]typ.Type{
		pathX.Key(): typ.NewUnion(typeA, typeB, typeC),
		pathY.Key(): typ.NewUnion(typeA, typeB, typeC),
		pathZ.Key(): typ.NewUnion(typeA, typeB, typeC),
	}

	s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: unionQueryField}}}
	set := constraint.NewConjunction(
		constraint.FieldEquals{Target: pathX, Field: "tag", Value: typ.LiteralString("a")},
		constraint.FieldEquals{Target: pathY, Field: "tag", Value: typ.LiteralString("b")},
		constraint.FieldNotEquals{Target: pathZ, Field: "tag", Value: typ.LiteralString("c")},
	)

	out := s.Apply(set, base)

	if got := out[pathX.Key()]; !typ.TypeEquals(got, typeA) {
		t.Errorf("x: got %v, want %v", got, typeA)
	}
	if got := out[pathY.Key()]; !typ.TypeEquals(got, typeB) {
		t.Errorf("y: got %v, want %v", got, typeB)
	}
	// z should exclude C, leaving A | B
	gotZ := out[pathZ.Key()]
	expectedZ := typ.NewUnion(typeA, typeB)
	if !typ.TypeEquals(gotZ, expectedZ) && !typ.TypeEquals(gotZ, typ.NewUnion(typeB, typeA)) {
		t.Errorf("z: got %v, want %v", gotZ, expectedZ)
	}
}

// TestSolver_DeepNestedPathNarrowing tests constraint solving through deep field chains.
// Note: Apply() narrows child paths directly; parent propagation requires explicit path
// registration. Use ApplyToSingle for ancestor narrowing.
func TestSolver_DeepNestedPathNarrowing(t *testing.T) {
	// Path: x.a.b
	pathDeep := constraint.Path{
		Root: "x",
		Segments: []constraint.Segment{
			{Kind: constraint.SegmentField, Name: "a"},
			{Kind: constraint.SegmentField, Name: "b"},
		},
	}

	// Build deeply nested types
	matchC := typ.NewRecord().Field("c", typ.LiteralString("match")).Build()
	otherC := typ.NewRecord().Field("c", typ.LiteralString("other")).Build()

	base := map[constraint.PathKey]typ.Type{
		pathDeep.Key(): typ.NewUnion(matchC, otherC),
	}

	s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: unionQueryField}}}
	set := constraint.NewConjunction(
		constraint.FieldEquals{Target: pathDeep, Field: "c", Value: typ.LiteralString("match")},
	)

	out := s.Apply(set, base)

	gotDeep := out[pathDeep.Key()]
	if !typ.TypeEquals(gotDeep, matchC) {
		t.Errorf("deep path: got %v, want %v", gotDeep, matchC)
	}
}

// TestSolver_TransitivePathEquality tests transitive constraint propagation via path equality.
func TestSolver_TransitivePathEquality(t *testing.T) {
	pathX := constraint.Path{Root: "x"}
	pathY := constraint.Path{Root: "y"}
	pathZ := constraint.Path{Root: "z"}

	typeA := typ.NewRecord().Field("tag", typ.LiteralString("a")).Build()
	typeB := typ.NewRecord().Field("tag", typ.LiteralString("b")).Build()

	base := map[constraint.PathKey]typ.Type{
		pathX.Key(): typ.NewUnion(typeA, typeB),
		pathY.Key(): typ.NewUnion(typeA, typeB),
		pathZ.Key(): typ.NewUnion(typeA, typeB),
	}

	// x == y AND y == z AND z.tag == "a"
	// Should narrow all three to typeA
	s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: unionQueryField}}}
	set := constraint.NewConjunction(
		constraint.EqPath{Left: pathX, Right: pathY},
		constraint.EqPath{Left: pathY, Right: pathZ},
		constraint.FieldEquals{Target: pathZ, Field: "tag", Value: typ.LiteralString("a")},
	)

	out := s.Apply(set, base)

	if got := out[pathZ.Key()]; !typ.TypeEquals(got, typeA) {
		t.Errorf("z: got %v, want %v", got, typeA)
	}
	if got := out[pathY.Key()]; !typ.TypeEquals(got, typeA) {
		t.Errorf("y: got %v, want %v", got, typeA)
	}
	if got := out[pathX.Key()]; !typ.TypeEquals(got, typeA) {
		t.Errorf("x: got %v, want %v", got, typeA)
	}
}

// TestSolver_CombinedPositiveAndNegativeConstraints tests mixing equals and not-equals.
func TestSolver_CombinedPositiveAndNegativeConstraints(t *testing.T) {
	pathX := constraint.Path{Root: "x"}

	// Union with 4 variants
	typeA := typ.NewRecord().Field("kind", typ.LiteralString("a")).Field("active", typ.True).Build()
	typeB := typ.NewRecord().Field("kind", typ.LiteralString("b")).Field("active", typ.True).Build()
	typeC := typ.NewRecord().Field("kind", typ.LiteralString("c")).Field("active", typ.False).Build()
	typeD := typ.NewRecord().Field("kind", typ.LiteralString("d")).Field("active", typ.False).Build()

	base := map[constraint.PathKey]typ.Type{
		pathX.Key(): typ.NewUnion(typeA, typeB, typeC, typeD),
	}

	// active == true AND kind ~= "a"
	// Should narrow to typeB only
	s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: unionQueryField}}}
	set := constraint.NewConjunction(
		constraint.FieldEquals{Target: pathX, Field: "active", Value: typ.True},
		constraint.FieldNotEquals{Target: pathX, Field: "kind", Value: typ.LiteralString("a")},
	)

	out := s.Apply(set, base)
	got := out[pathX.Key()]
	if !typ.TypeEquals(got, typeB) {
		t.Errorf("combined constraints: got %v, want %v", got, typeB)
	}
}

// TestSolver_IndexEqualsWithUnion tests index constraint on union of tuples/arrays.
func TestSolver_IndexEqualsWithUnion(t *testing.T) {
	pathX := constraint.Path{Root: "x"}

	tuple1 := typ.NewTuple(typ.LiteralString("ok"), typ.Number)
	tuple2 := typ.NewTuple(typ.LiteralString("err"), typ.String)
	tuple3 := typ.NewTuple(typ.LiteralString("pending"), typ.Boolean)

	base := map[constraint.PathKey]typ.Type{
		pathX.Key(): typ.NewUnion(tuple1, tuple2, tuple3),
	}

	queryIndex := func(t typ.Type, key typ.Type) (typ.Type, bool) {
		if tup, ok := t.(*typ.Tuple); ok {
			if lit, ok := key.(*typ.Literal); ok && lit.Base == kind.Integer {
				idx := int(lit.Value.(int64)) - 1 // Lua 1-based
				if idx >= 0 && idx < len(tup.Elements) {
					return tup.Elements[idx], true
				}
			}
		}
		return nil, false
	}

	s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{IndexFunc: queryIndex}}}
	set := constraint.NewConjunction(
		constraint.IndexEquals{Target: pathX, Key: typ.LiteralInt(1), Value: typ.LiteralString("ok")},
	)

	out := s.Apply(set, base)
	got := out[pathX.Key()]
	if !typ.TypeEquals(got, tuple1) {
		t.Errorf("index equals: got %v, want %v", got, tuple1)
	}
}

// TestSolver_NestedOptionalNarrowing tests narrowing through optional types.
func TestSolver_NestedOptionalNarrowing(t *testing.T) {
	pathX := constraint.Path{Root: "x"}

	innerA := typ.NewRecord().Field("kind", typ.LiteralString("a")).Build()
	innerB := typ.NewRecord().Field("kind", typ.LiteralString("b")).Build()

	base := map[constraint.PathKey]typ.Type{
		pathX.Key(): typ.NewOptional(typ.NewUnion(innerA, innerB)),
	}

	s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: unionQueryField}}}

	// First constraint: x ~= nil
	set1 := constraint.NewConjunction(constraint.NotNil{Path: pathX})
	out1 := s.Apply(set1, base)
	got1 := out1[pathX.Key()]
	expectedNonNil := typ.NewUnion(innerA, innerB)
	if !typ.TypeEquals(got1, expectedNonNil) && !typ.TypeEquals(got1, typ.NewUnion(innerB, innerA)) {
		t.Errorf("after NotNil: got %v, want %v", got1, expectedNonNil)
	}

	// Second constraint: x.kind == "a" (starting from non-nil state)
	set2 := constraint.NewConjunction(constraint.FieldEquals{Target: pathX, Field: "kind", Value: typ.LiteralString("a")})
	out2 := s.Apply(set2, out1)
	got2 := out2[pathX.Key()]
	if !typ.TypeEquals(got2, innerA) {
		t.Errorf("after FieldEquals: got %v, want %v", got2, innerA)
	}
}

// TestSolver_BooleanFieldNarrowing tests truthy/falsy narrowing via boolean fields.
func TestSolver_BooleanFieldNarrowing(t *testing.T) {
	pathX := constraint.Path{Root: "x"}
	pathXEnabled := constraint.Path{
		Root:     "x",
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "enabled"}},
	}

	enabledType := typ.NewRecord().Field("enabled", typ.True).Field("data", typ.String).Build()
	disabledType := typ.NewRecord().Field("enabled", typ.False).Field("data", typ.Number).Build()

	base := map[constraint.PathKey]typ.Type{
		pathX.Key():        typ.NewUnion(enabledType, disabledType),
		pathXEnabled.Key(): typ.Boolean,
	}

	s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: unionQueryField}}}

	// Truthy on enabled field
	setTruthy := constraint.NewConjunction(constraint.Truthy{Path: pathXEnabled})
	outTruthy := s.Apply(setTruthy, base)
	gotTruthy := outTruthy[pathX.Key()]
	if !typ.TypeEquals(gotTruthy, enabledType) {
		t.Errorf("truthy enabled: got %v, want %v", gotTruthy, enabledType)
	}

	// Falsy on enabled field
	setFalsy := constraint.NewConjunction(constraint.Falsy{Path: pathXEnabled})
	outFalsy := s.Apply(setFalsy, base)
	gotFalsy := outFalsy[pathX.Key()]
	if !typ.TypeEquals(gotFalsy, disabledType) {
		t.Errorf("falsy enabled: got %v, want %v", gotFalsy, disabledType)
	}
}

// TestSolver_ConstraintConvergence tests that solver reaches fixed point.
func TestSolver_ConstraintConvergence(t *testing.T) {
	pathX := constraint.Path{Root: "x"}
	pathY := constraint.Path{Root: "y"}

	typeAB := typ.NewUnion(
		typ.NewRecord().Field("tag", typ.LiteralString("a")).Build(),
		typ.NewRecord().Field("tag", typ.LiteralString("b")).Build(),
	)

	base := map[constraint.PathKey]typ.Type{
		pathX.Key(): typeAB,
		pathY.Key(): typeAB,
	}

	// Multiple constraints that interact
	s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: unionQueryField}}}
	set := constraint.NewConjunction(
		constraint.EqPath{Left: pathX, Right: pathY},
		constraint.FieldEquals{Target: pathX, Field: "tag", Value: typ.LiteralString("a")},
	)

	out := s.Apply(set, base)

	// Both should converge to typeA
	typeA := typ.NewRecord().Field("tag", typ.LiteralString("a")).Build()
	if got := out[pathX.Key()]; !typ.TypeEquals(got, typeA) {
		t.Errorf("x should converge to typeA, got %v", got)
	}
	if got := out[pathY.Key()]; !typ.TypeEquals(got, typeA) {
		t.Errorf("y should converge to typeA, got %v", got)
	}
}

// TestSolver_HasFieldNarrowing tests narrowing by field existence.
func TestSolver_HasFieldNarrowing(t *testing.T) {
	pathX := constraint.Path{Root: "x"}

	withExtra := typ.NewRecord().Field("tag", typ.String).Field("extra", typ.Number).Build()
	withoutExtra := typ.NewRecord().Field("tag", typ.String).Build()

	base := map[constraint.PathKey]typ.Type{
		pathX.Key(): typ.NewUnion(withExtra, withoutExtra),
	}

	s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: unionQueryField}}}
	set := constraint.NewConjunction(constraint.HasField{Path: pathX, Field: "extra"})

	out := s.Apply(set, base)
	got := out[pathX.Key()]
	if !typ.TypeEquals(got, withExtra) {
		t.Errorf("HasField: got %v, want %v", got, withExtra)
	}
}

// TestSolver_TypeKeyHashNarrowing tests narrowing by type hash.
func TestSolver_TypeKeyHashNarrowing(t *testing.T) {
	pathX := constraint.Path{Root: "x"}

	recA := typ.NewRecord().Field("value", typ.String).Build()
	recB := typ.NewRecord().Field("value", typ.Number).Build()

	base := map[constraint.PathKey]typ.Type{
		pathX.Key(): typ.NewUnion(recA, recB, typ.String, typ.Number),
	}

	resolver := func(k narrow.TypeKey) typ.Type {
		if k.Kind == narrow.TypeKeyHash && k.Hash == recA.Hash() {
			return recA
		}
		return nil
	}

	s := constraint.Solver{Env: constraint.Env{ResolveType: resolver}}
	set := constraint.NewConjunction(constraint.HasType{Path: pathX, Type: narrow.HashTypeKey(recA.Hash())})

	out := s.Apply(set, base)
	got := out[pathX.Key()]
	if !typ.TypeEquals(got, recA) {
		t.Errorf("HasType hash: got %v, want %v", got, recA)
	}
}

// TestSolver_MixedIndexAndFieldConstraints tests combining index and field constraints.
func TestSolver_MixedIndexAndFieldConstraints(t *testing.T) {
	pathX := constraint.Path{Root: "x"}

	// Records with array field
	arrA := typ.NewTuple(typ.LiteralString("ok"), typ.String)
	arrB := typ.NewTuple(typ.LiteralString("err"), typ.String)
	recA := typ.NewRecord().Field("result", arrA).Field("tag", typ.LiteralString("success")).Build()
	recB := typ.NewRecord().Field("result", arrB).Field("tag", typ.LiteralString("failure")).Build()

	base := map[constraint.PathKey]typ.Type{
		pathX.Key(): typ.NewUnion(recA, recB),
	}

	s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: unionQueryField}}}
	set := constraint.NewConjunction(
		constraint.FieldEquals{Target: pathX, Field: "tag", Value: typ.LiteralString("success")},
	)

	out := s.Apply(set, base)
	got := out[pathX.Key()]
	if !typ.TypeEquals(got, recA) {
		t.Errorf("mixed constraints: got %v, want %v", got, recA)
	}
}

// TestApplyToSingle_DeepFieldChain tests ApplyToSingle with deep field chains.
func TestApplyToSingle_DeepFieldChain(t *testing.T) {
	// Path: x.a.b.c
	pathABC := constraint.Path{
		Root: "x",
		Segments: []constraint.Segment{
			{Kind: constraint.SegmentField, Name: "a"},
			{Kind: constraint.SegmentField, Name: "b"},
			{Kind: constraint.SegmentField, Name: "c"},
		},
	}
	targetPath := constraint.Path{Root: "x"}

	// Build deeply nested types
	dMatch := typ.NewRecord().Field("d", typ.LiteralString("match")).Build()
	dOther := typ.NewRecord().Field("d", typ.LiteralString("other")).Build()
	cMatch := typ.NewRecord().Field("c", dMatch).Build()
	cOther := typ.NewRecord().Field("c", dOther).Build()
	bMatch := typ.NewRecord().Field("b", cMatch).Build()
	bOther := typ.NewRecord().Field("b", cOther).Build()
	matchType := typ.NewRecord().Field("a", bMatch).Build()
	otherType := typ.NewRecord().Field("a", bOther).Build()

	union := typ.NewUnion(matchType, otherType)

	s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: unionQueryField}}}
	constraints := []constraint.Constraint{
		constraint.FieldEquals{Target: pathABC, Field: "d", Value: typ.LiteralString("match")},
	}

	resolve := func(p constraint.Path) constraint.PathKey {
		return p.Key()
	}

	result := s.ApplyToSingle(constraints, targetPath.Key(), union, resolve)
	if !typ.TypeEquals(result, matchType) {
		t.Errorf("ApplyToSingle deep chain: got %v, want %v", result, matchType)
	}
}

// TestSolver_EqPath_TransitiveFieldPropagation tests that field constraints
// propagate transitively across equal paths:
// Given: x == y && y.tag == "event"
// Expect: x.tag also narrows (x gets narrowed via transitive propagation)
func TestSolver_EqPath_TransitiveFieldPropagation(t *testing.T) {
	pathX := constraint.Path{Root: "x"}
	pathY := constraint.Path{Root: "y"}

	event := typ.NewRecord().Field("tag", typ.LiteralString("event")).Build()
	timeout := typ.NewRecord().Field("tag", typ.LiteralString("timeout")).Build()

	base := map[constraint.PathKey]typ.Type{
		pathX.Key(): typ.NewUnion(event, timeout),
		pathY.Key(): typ.NewUnion(event, timeout),
	}

	s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: unionQueryField}}}
	constraints := constraint.NewConjunction(
		constraint.EqPath{Left: pathX, Right: pathY},
		constraint.FieldEquals{Target: pathY, Field: "tag", Value: typ.LiteralString("event")},
	)

	out := s.Apply(constraints, base)

	// y should be narrowed to event
	if got := out[pathY.Key()]; !typ.TypeEquals(got, event) {
		t.Errorf("y: got %v, want %v", got, event)
	}

	// x should also be narrowed to event via transitive propagation
	if got := out[pathX.Key()]; !typ.TypeEquals(got, event) {
		t.Errorf("x: got %v, want %v (transitive from y)", got, event)
	}
}

// TestSolver_IndexEquals_IntegerLiteral tests that integer literals properly narrow
// unions containing integer-typed members.
func TestSolver_IndexEquals_IntegerLiteral(t *testing.T) {
	pathX := constraint.Path{Root: "x"}

	// Tuples with different first elements
	tupleOne := typ.NewTuple(typ.LiteralInt(1), typ.String)
	tupleTwo := typ.NewTuple(typ.LiteralInt(2), typ.String)

	base := map[constraint.PathKey]typ.Type{
		pathX.Key(): typ.NewUnion(tupleOne, tupleTwo),
	}

	s := constraint.Solver{Env: constraint.Env{Resolver: core.Resolver()}}
	constraints := constraint.NewConjunction(
		constraint.IndexEquals{Target: pathX, Key: typ.LiteralInt(1), Value: typ.LiteralInt(1)},
	)

	out := s.Apply(constraints, base)

	// Should narrow to tupleOne since first index is 1
	if got := out[pathX.Key()]; !typ.TypeEquals(got, tupleOne) {
		t.Errorf("IndexEquals integer literal: got %v, want %v", got, tupleOne)
	}
}

// TestSolver_FieldEquals_IntegerField tests that FieldEquals with integer literals
// narrows unions with integer-typed fields.
func TestSolver_FieldEquals_IntegerField(t *testing.T) {
	pathX := constraint.Path{Root: "x"}

	rec1 := typ.NewRecord().Field("code", typ.LiteralInt(1)).Build()
	rec2 := typ.NewRecord().Field("code", typ.LiteralInt(2)).Build()
	rec3 := typ.NewRecord().Field("code", typ.Integer).Build()

	base := map[constraint.PathKey]typ.Type{
		pathX.Key(): typ.NewUnion(rec1, rec2, rec3),
	}

	s := constraint.Solver{Env: constraint.Env{Resolver: &core.FuncResolver{FieldFunc: unionQueryField}}}
	constraints := constraint.NewConjunction(
		constraint.FieldEquals{Target: pathX, Field: "code", Value: typ.LiteralInt(1)},
	)

	out := s.Apply(constraints, base)

	// rec1 exactly matches (code is literal 1)
	// rec3 could match (code is integer, which includes 1)
	// rec2 should be excluded (code is literal 2, which cannot be 1)
	want := typ.NewUnion(rec1, rec3)
	if got := out[pathX.Key()]; !typ.TypeEquals(got, want) {
		t.Errorf("FieldEquals integer literal: got %v, want %v", got, want)
	}
}

// TestSolver_KeyOf tests that KeyOf constraint does not narrow types directly.
func TestSolver_KeyOf(t *testing.T) {
	table := constraint.Path{Root: "t", Symbol: 1}
	key := constraint.Path{Root: "k", Symbol: 2}

	mapType := typ.NewMap(typ.String, typ.Number)
	base := map[constraint.PathKey]typ.Type{
		table.Key(): mapType,
		key.Key():   typ.String,
	}

	s := constraint.Solver{}
	constraints := constraint.NewConjunction(constraint.KeyOf{Table: table, Key: key})

	out := s.Apply(constraints, base)

	// KeyOf should not change the types
	if got := out[table.Key()]; !typ.TypeEquals(got, mapType) {
		t.Errorf("KeyOf should not narrow table type: got %v, want %v", got, mapType)
	}
	if got := out[key.Key()]; !typ.TypeEquals(got, typ.String) {
		t.Errorf("KeyOf should not narrow key type: got %v, want %v", got, typ.String)
	}
}

// TestHasKeyOfConstraint tests the HasKeyOfConstraint function.
func TestHasKeyOfConstraint(t *testing.T) {
	table := constraint.Path{Root: "t", Symbol: 1}
	key := constraint.Path{Root: "k", Symbol: 2}
	otherTable := constraint.Path{Root: "other", Symbol: 3}
	otherKey := constraint.Path{Root: "other_key", Symbol: 4}

	ko := constraint.KeyOf{Table: table, Key: key}
	cond := constraint.FromConstraints(ko)

	// Should find matching KeyOf in single disjunct
	if !constraint.HasKeyOfConstraint(cond, table, key, nil) {
		t.Error("Should find KeyOf(table, key)")
	}

	// Should not find with different table
	if constraint.HasKeyOfConstraint(cond, otherTable, key, nil) {
		t.Error("Should not find KeyOf(otherTable, key)")
	}

	// Should not find with different key
	if constraint.HasKeyOfConstraint(cond, table, otherKey, nil) {
		t.Error("Should not find KeyOf(table, otherKey)")
	}

	// Should not find in empty condition
	if constraint.HasKeyOfConstraint(constraint.TrueCondition(), table, key, nil) {
		t.Error("Should not find KeyOf in TrueCondition")
	}

	// Should not find in FalseCondition
	if constraint.HasKeyOfConstraint(constraint.FalseCondition(), table, key, nil) {
		t.Error("Should not find KeyOf in FalseCondition")
	}

	// KeyOf only in one disjunct - NOT guaranteed (DNF semantics)
	otherCond := constraint.FromConstraints(constraint.NotNil{Path: table})
	orCondPartial := constraint.Or(cond, otherCond)
	if constraint.HasKeyOfConstraint(orCondPartial, table, key, nil) {
		t.Error("Should NOT find KeyOf when missing from some disjuncts")
	}

	// KeyOf in ALL disjuncts - guaranteed
	koAndOther := constraint.FromConstraints(ko, constraint.Truthy{Path: otherTable})
	orCondFull := constraint.Or(cond, koAndOther)
	if !constraint.HasKeyOfConstraint(orCondFull, table, key, nil) {
		t.Error("Should find KeyOf when present in all disjuncts")
	}
}

// TestHasKeyOfConstraint_WithResolver tests path resolution.
func TestHasKeyOfConstraint_WithResolver(t *testing.T) {
	table := constraint.Path{Root: "t", Symbol: 1}
	key := constraint.Path{Root: "k", Symbol: 2}

	ko := constraint.KeyOf{Table: table, Key: key}
	cond := constraint.FromConstraints(ko)

	resolver := func(p constraint.Path) constraint.PathKey {
		return p.Key()
	}

	// Same paths with resolver
	if !constraint.HasKeyOfConstraint(cond, table, key, resolver) {
		t.Error("Should find KeyOf with resolver")
	}

	// Different paths with same key via resolver
	sameKeyTable := constraint.Path{Root: "different", Symbol: 1}
	if !constraint.HasKeyOfConstraint(cond, sameKeyTable, key, resolver) {
		t.Error("Should find KeyOf when resolver returns same key")
	}
}

// TestHasKeyOfConstraint_EmptyResolverResult tests that empty resolver results don't cause false positives.
func TestHasKeyOfConstraint_EmptyResolverResult(t *testing.T) {
	table := constraint.Path{Root: "t", Symbol: 1}
	key := constraint.Path{Root: "k", Symbol: 2}
	otherTable := constraint.Path{Root: "other", Symbol: 99}

	ko := constraint.KeyOf{Table: table, Key: key}
	cond := constraint.FromConstraints(ko)

	// Resolver that always returns empty string
	badResolver := func(p constraint.Path) constraint.PathKey {
		return ""
	}

	// Should NOT match because empty == empty is a false positive
	// The fix should ignore empty resolver results
	if constraint.HasKeyOfConstraint(cond, otherTable, key, badResolver) {
		t.Error("Empty resolver should not cause false positive match")
	}

	// Resolver that returns empty for some paths
	partialResolver := func(p constraint.Path) constraint.PathKey {
		if p.Symbol == 1 {
			return "sym1"
		}
		return ""
	}

	// Should match on table (both resolve to "sym1"), key uses direct equality
	if !constraint.HasKeyOfConstraint(cond, table, key, partialResolver) {
		t.Error("Partial resolver should still find match when paths are equal")
	}
}

func TestSolver_WorkSkippingOptimization(t *testing.T) {
	// Create more than 100 constraints to trigger the work-skipping optimization path
	paths := make([]constraint.Path, 120)
	for i := range paths {
		paths[i] = constraint.NewPath(cfg.SymbolID(i+1), "x")
	}

	base := make(map[constraint.PathKey]typ.Type)
	for _, p := range paths {
		base[p.Key()] = typ.NewUnion(typ.String, typ.Nil)
	}

	// Create many NotNil constraints
	constraints := make([]constraint.Constraint, len(paths))
	for i, p := range paths {
		constraints[i] = constraint.NotNil{Path: p}
	}

	s := constraint.Solver{}
	result := s.Apply(constraints, base)

	// All paths should be narrowed to string (nil removed)
	for _, p := range paths {
		if got := result[p.Key()]; !typ.TypeEquals(got, typ.String) {
			t.Errorf("Path %s: got %v, want string", p.String(), got)
		}
	}
}

func TestSolver_WorkSkippingWithChanges(t *testing.T) {
	// Create a scenario where work-skipping would matter:
	// some constraints depend on others
	paths := make([]constraint.Path, 150)
	for i := range paths {
		paths[i] = constraint.NewPath(cfg.SymbolID(i+1), "v")
	}

	base := make(map[constraint.PathKey]typ.Type)
	for _, p := range paths {
		base[p.Key()] = typ.NewUnion(typ.String, typ.Number, typ.Nil)
	}

	// Create constraints
	var constraints []constraint.Constraint

	// First 50 paths: NotNil
	for i := 0; i < 50; i++ {
		constraints = append(constraints, constraint.NotNil{Path: paths[i]})
	}

	// Next 50 paths: HasType string
	tk := narrow.BuiltinTypeKey("string")
	resolver := func(k narrow.TypeKey) typ.Type {
		if k.Kind == narrow.TypeKeyBuiltin && k.Name == "string" {
			return typ.String
		}
		return nil
	}

	for i := 50; i < 100; i++ {
		constraints = append(constraints, constraint.HasType{Path: paths[i], Type: tk})
	}

	// Last 50 paths: Truthy
	for i := 100; i < 150; i++ {
		constraints = append(constraints, constraint.Truthy{Path: paths[i]})
	}

	s := constraint.Solver{Env: constraint.Env{ResolveType: resolver}}
	result := s.Apply(constraints, base)

	// First 50: should be string|number (nil removed)
	expected := typ.NewUnion(typ.String, typ.Number)
	for i := 0; i < 50; i++ {
		if got := result[paths[i].Key()]; !typ.TypeEquals(got, expected) {
			t.Errorf("Path %d: got %v, want %v", i, got, expected)
		}
	}

	// Next 50: should be string
	for i := 50; i < 100; i++ {
		if got := result[paths[i].Key()]; !typ.TypeEquals(got, typ.String) {
			t.Errorf("Path %d: got %v, want string", i, got)
		}
	}

	// Last 50: should be string|number (nil and false removed)
	for i := 100; i < 150; i++ {
		if got := result[paths[i].Key()]; !typ.TypeEquals(got, expected) {
			t.Errorf("Path %d: got %v, want %v", i, got, expected)
		}
	}
}
