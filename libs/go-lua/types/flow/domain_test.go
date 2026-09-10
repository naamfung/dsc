package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow/domain"
	"github.com/wippyai/go-lua/types/flow/numeric"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

func makeMockEnv(types map[constraint.PathKey]typ.Type) constraint.Env {
	return constraint.Env{
		ResolvePath: func(p constraint.Path) constraint.PathKey {
			if p.Symbol == 0 {
				if p.IsPlaceholder() {
					return constraint.PathKey(p.Root)
				}
				return constraint.PathKey(p.Root)
			}
			return p.Key()
		},
		PathTypeAt: func(key constraint.PathKey) typ.Type {
			if types == nil {
				return nil
			}
			return types[key]
		},
	}
}

func TestTypeDomain_ApplyTruthy(t *testing.T) {
	key := constraint.PathKey("x")
	env := makeMockEnv(map[constraint.PathKey]typ.Type{
		key: typ.NewUnion(typ.String, typ.Nil),
	})

	d := domain.NewTypeDomain(env)
	atom := constraint.AtomTruthy(constraint.TermVar(key))

	ok := d.ApplyAtom(atom)
	if !ok {
		t.Fatal("ApplyAtom should succeed")
	}

	result := d.TypeAt(key)
	if result == nil {
		t.Fatal("expected narrowed type")
	}

	// Should narrow to string (remove nil)
	if result.Kind() != typ.String.Kind() {
		t.Fatalf("expected string, got %v", result)
	}
}

func TestTypeDomain_ApplyFalsy(t *testing.T) {
	key := constraint.PathKey("x")
	env := makeMockEnv(map[constraint.PathKey]typ.Type{
		key: typ.NewUnion(typ.String, typ.Nil),
	})

	d := domain.NewTypeDomain(env)
	atom := constraint.AtomFalsy(constraint.TermVar(key))

	ok := d.ApplyAtom(atom)
	if !ok {
		t.Fatal("ApplyAtom should succeed")
	}

	result := d.TypeAt(key)
	if result == nil {
		t.Fatal("expected narrowed type")
	}

	// Should narrow to nil (remove string)
	if result != typ.Nil {
		t.Fatalf("expected nil, got %v", result)
	}
}

func TestTypeDomain_ApplyIsNil(t *testing.T) {
	key := constraint.PathKey("x")
	env := makeMockEnv(map[constraint.PathKey]typ.Type{
		key: typ.NewUnion(typ.String, typ.Nil),
	})

	d := domain.NewTypeDomain(env)
	atom := constraint.AtomEq(constraint.TermVar(key), constraint.TermNil())

	ok := d.ApplyAtom(atom)
	if !ok {
		t.Fatal("ApplyAtom should succeed")
	}

	result := d.TypeAt(key)
	if result != typ.Nil {
		t.Fatalf("expected nil, got %v", result)
	}
}

func TestTypeDomain_ApplyNotNil(t *testing.T) {
	key := constraint.PathKey("x")
	env := makeMockEnv(map[constraint.PathKey]typ.Type{
		key: typ.NewUnion(typ.String, typ.Nil),
	})

	d := domain.NewTypeDomain(env)
	atom := constraint.AtomNe(constraint.TermVar(key), constraint.TermNil())

	ok := d.ApplyAtom(atom)
	if !ok {
		t.Fatal("ApplyAtom should succeed")
	}

	result := d.TypeAt(key)
	if result == nil {
		t.Fatal("expected narrowed type")
	}

	if result.Kind() != typ.String.Kind() {
		t.Fatalf("expected string, got %v", result)
	}
}

func TestTypeDomain_Join_TypeUnion(t *testing.T) {
	env := makeMockEnv(nil)

	a := domain.NewTypeDomain(env)
	a.Narrowed["x"] = typ.String

	b := domain.NewTypeDomain(env)
	b.Narrowed["x"] = typ.Number

	joined := a.Join(b).(*domain.TypeDomain)
	result := joined.TypeAt("x")

	if result == nil {
		t.Fatal("expected joined type")
	}

	u, ok := result.(*typ.Union)
	if !ok {
		t.Fatalf("expected union, got %T", result)
	}
	if len(u.Members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(u.Members))
	}
}

func TestTypeDomain_Join_DropsNonCommonKeys(t *testing.T) {
	env := makeMockEnv(nil)

	a := domain.NewTypeDomain(env)
	a.Narrowed["x"] = typ.String

	b := domain.NewTypeDomain(env)
	// b has no key "x"

	joined := a.Join(b).(*domain.TypeDomain)
	_, hasKey := joined.Narrowed["x"]
	if hasKey {
		t.Fatal("key should be dropped")
	}
}

func TestNumericDomain_ApplyGeConst(t *testing.T) {
	env := makeMockEnv(nil)
	d := numeric.NewDomain(env)

	key := constraint.PathKey("x")
	atom := constraint.AtomGe(constraint.TermVar(key), constraint.TermConst(5))

	ok := d.ApplyAtom(atom)
	if !ok {
		t.Fatal("ApplyAtom should succeed")
	}

	lower, _, ok := d.State().BoundsFor(key)
	if !ok {
		t.Fatal("expected bounds")
	}
	if lower != 5 {
		t.Fatalf("expected lower 5, got %d", lower)
	}
}

func TestNumericDomain_ApplyLeConst(t *testing.T) {
	env := makeMockEnv(nil)
	d := numeric.NewDomain(env)

	key := constraint.PathKey("x")
	atom := constraint.AtomLe(constraint.TermVar(key), constraint.TermConst(10))

	ok := d.ApplyAtom(atom)
	if !ok {
		t.Fatal("ApplyAtom should succeed")
	}

	_, upper, ok := d.State().BoundsFor(key)
	if !ok {
		t.Fatal("expected bounds")
	}
	if upper != 10 {
		t.Fatalf("expected upper 10, got %d", upper)
	}
}

func TestNumericDomain_ApplyLt(t *testing.T) {
	env := makeMockEnv(nil)
	d := numeric.NewDomain(env)

	x := constraint.PathKey("x")
	y := constraint.PathKey("y")
	atom := constraint.AtomLt(constraint.TermVar(x), constraint.TermVar(y))

	ok := d.ApplyAtom(atom)
	if !ok {
		t.Fatal("ApplyAtom should succeed")
	}

	// x < y is stored - verify via theory solver
	bound, exists := d.Theory().InferRelationalBound(x, y)
	if !exists {
		t.Fatal("expected relation")
	}
	if bound > -1 {
		t.Fatalf("expected bound <= -1, got %d", bound)
	}
}

func TestNumericDomain_Join(t *testing.T) {
	env := makeMockEnv(nil)

	a := numeric.NewDomain(env)
	atomA1 := constraint.AtomGe(constraint.TermVar("x"), constraint.TermConst(0))
	atomA2 := constraint.AtomLe(constraint.TermVar("x"), constraint.TermConst(10))
	a.ApplyAtom(atomA1)
	a.ApplyAtom(atomA2)

	b := numeric.NewDomain(env)
	atomB1 := constraint.AtomGe(constraint.TermVar("x"), constraint.TermConst(5))
	atomB2 := constraint.AtomLe(constraint.TermVar("x"), constraint.TermConst(15))
	b.ApplyAtom(atomB1)
	b.ApplyAtom(atomB2)

	joined := a.Join(b).(*numeric.Domain)

	// Intersection of [0,10] and [5,15] = [5,10]
	lower, upper, ok := joined.State().BoundsFor("x")
	if !ok {
		t.Fatal("expected bounds")
	}
	if lower != 5 || upper != 10 {
		t.Fatalf("expected [5,10], got [%d,%d]", lower, upper)
	}
}

func TestProductDomain_TypeAndNumericCombined(t *testing.T) {
	key := constraint.PathKey("x")

	env := makeMockEnv(map[constraint.PathKey]typ.Type{
		key: typ.NewUnion(typ.String, typ.Number),
	})

	d := NewProductDomain(env)

	// Apply type constraint (hastype number)
	d.ApplyAtom(constraint.AtomHasType(
		constraint.TermVar(key),
		narrow.TypeKey{Kind: narrow.TypeKeyBuiltin, Name: "number"},
	))

	// Apply numeric constraint (x >= 0)
	d.ApplyAtom(constraint.AtomGe(
		constraint.TermVar(key),
		constraint.TermConst(0),
	))

	// Type domain should narrow to number
	typeResult := d.Type.TypeAt(key)
	if typeResult == nil {
		t.Fatal("expected type domain result")
	}
	if typeResult.Kind() != typ.Number.Kind() {
		t.Fatalf("expected number type in type domain, got %v", typeResult)
	}

	// Numeric bounds set
	lower, _, ok := d.Numeric.State().BoundsFor(key)
	if !ok {
		t.Fatal("expected bounds")
	}
	if lower != 0 {
		t.Fatalf("expected lower 0, got %d", lower)
	}
}

func TestProductDomain_ApplyConjunction(t *testing.T) {
	key := constraint.PathKey("x")

	env := makeMockEnv(map[constraint.PathKey]typ.Type{
		key: typ.NewUnion(typ.String, typ.Nil),
	})

	d := NewProductDomain(env)

	// Create constraint set with Truthy
	path := constraint.Path{Root: "x"}
	set := constraint.NewConjunction(constraint.Truthy{Path: path})

	ok := d.ApplyConjunction(set)
	if !ok {
		t.Fatal("ApplyConjunction should succeed")
	}

	// Type domain should narrow to string
	result := d.Type.TypeAt(key)
	if result == nil {
		t.Fatal("expected type")
	}
	if result.Kind() != typ.String.Kind() {
		t.Fatalf("expected string, got %v", result)
	}
}

func TestProductDomain_IsUnsat(t *testing.T) {
	key := constraint.PathKey("x")

	env := makeMockEnv(map[constraint.PathKey]typ.Type{
		key: typ.String,
	})

	d := NewProductDomain(env)

	// Apply contradictory constraint
	d.ApplyAtom(constraint.AtomFalsy(constraint.TermVar(key)))

	if !d.IsUnsat() {
		t.Fatal("expected unsat")
	}
}

func TestProductDomain_Clone(t *testing.T) {
	key := constraint.PathKey("x")

	env := makeMockEnv(map[constraint.PathKey]typ.Type{
		key: typ.NewUnion(typ.String, typ.Nil),
	})

	d := NewProductDomain(env)
	d.ApplyAtom(constraint.AtomTruthy(constraint.TermVar(key)))

	clone := d.Clone().(*ProductDomain)

	// Clone should have the same narrowed type in Type domain
	original := d.Type.TypeAt(key)
	cloned := clone.Type.TypeAt(key)

	if !typ.TypeEquals(original, cloned) {
		t.Fatalf("clone should equal original: orig=%v, clone=%v", original, cloned)
	}

	// Modify clone, original unchanged
	clone.Type.Narrowed[key] = typ.Number
	if d.Type.TypeAt(key).Kind() != typ.String.Kind() {
		t.Fatalf("original should be unchanged, got %v", d.Type.TypeAt(key))
	}
}

func TestProductDomain_Join(t *testing.T) {
	key := constraint.PathKey("x")

	env := makeMockEnv(nil)

	a := NewProductDomain(env)
	a.Type.Narrowed[key] = typ.String

	b := NewProductDomain(env)
	b.Type.Narrowed[key] = typ.Number

	joined := a.Join(b).(*ProductDomain)
	result := joined.TypeAt(key)

	if result == nil {
		t.Fatal("expected joined type")
	}

	u, ok := result.(*typ.Union)
	if !ok {
		t.Fatalf("expected union, got %T", result)
	}
	if len(u.Members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(u.Members))
	}
}

func TestProductDomain_NarrowedChildPaths_IncludesIndexChildren(t *testing.T) {
	env := makeMockEnv(nil)
	d := NewProductDomain(env)

	parent := constraint.PathKey("sym1@1")
	indexChild := constraint.PathKey(`sym1@1["meta.type"]`)
	fieldChild := constraint.PathKey("sym1@1.ok")
	unrelated := constraint.PathKey(`sym2@1["meta.type"]`)

	d.Type.Narrowed[indexChild] = typ.String
	d.Shape.Narrowed[fieldChild] = typ.Boolean
	d.Type.Narrowed[unrelated] = typ.Number

	got := d.NarrowedChildPaths(parent)
	if _, ok := got[indexChild]; !ok {
		t.Fatalf("expected index child narrowing for %q", indexChild)
	}
	if _, ok := got[fieldChild]; !ok {
		t.Fatalf("expected field child narrowing for %q", fieldChild)
	}
	if _, ok := got[unrelated]; ok {
		t.Fatalf("unexpected unrelated child narrowing for %q", unrelated)
	}
}

func TestClassifyAtom(t *testing.T) {
	tests := []struct {
		name     string
		atom     constraint.Atom
		expected domain.AtomClass
	}{
		{
			name:     "HasType is type",
			atom:     constraint.AtomHasType(constraint.TermVar("x"), narrow.TypeKey{}),
			expected: domain.AtomClassType,
		},
		{
			name:     "NotHasType is type",
			atom:     constraint.AtomNotHasType(constraint.TermVar("x"), narrow.TypeKey{}),
			expected: domain.AtomClassType,
		},
		{
			name:     "Truthy is type",
			atom:     constraint.AtomTruthy(constraint.TermVar("x")),
			expected: domain.AtomClassType,
		},
		{
			name:     "Falsy is type",
			atom:     constraint.AtomFalsy(constraint.TermVar("x")),
			expected: domain.AtomClassType,
		},
		{
			name:     "Lt is numeric",
			atom:     constraint.AtomLt(constraint.TermVar("x"), constraint.TermVar("y")),
			expected: domain.AtomClassNumeric,
		},
		{
			name:     "Le is numeric",
			atom:     constraint.AtomLe(constraint.TermVar("x"), constraint.TermConst(5)),
			expected: domain.AtomClassNumeric,
		},
		{
			name:     "Ge is numeric",
			atom:     constraint.AtomGe(constraint.TermVar("x"), constraint.TermConst(5)),
			expected: domain.AtomClassNumeric,
		},
		{
			name:     "Gt is numeric",
			atom:     constraint.AtomGt(constraint.TermVar("x"), constraint.TermVar("y")),
			expected: domain.AtomClassNumeric,
		},
		{
			name:     "ModEq is numeric",
			atom:     constraint.AtomModEq(constraint.TermVar("x"), 2, 0),
			expected: domain.AtomClassNumeric,
		},
		{
			name:     "Eq nil is type",
			atom:     constraint.AtomEq(constraint.TermVar("x"), constraint.TermNil()),
			expected: domain.AtomClassType,
		},
		{
			name:     "Ne nil is type",
			atom:     constraint.AtomNe(constraint.TermVar("x"), constraint.TermNil()),
			expected: domain.AtomClassType,
		},
		{
			name:     "Eq const is numeric",
			atom:     constraint.AtomEq(constraint.TermVar("x"), constraint.TermConst(5)),
			expected: domain.AtomClassNumeric,
		},
		{
			name:     "Eq var is both",
			atom:     constraint.AtomEq(constraint.TermVar("x"), constraint.TermVar("y")),
			expected: domain.AtomClassBoth,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := domain.ClassifyAtom(tt.atom)
			if got != tt.expected {
				t.Errorf("ClassifyAtom() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestProductDomain_MixedTypeAndNumericConstraints(t *testing.T) {
	key := constraint.PathKey("x")

	env := makeMockEnv(map[constraint.PathKey]typ.Type{
		key: typ.NewUnion(typ.String, typ.Number, typ.Nil),
	})

	d := NewProductDomain(env)

	// Apply type constraint via atom (hastype number - narrows to number)
	hasTypeAtom := constraint.AtomHasType(
		constraint.TermVar(key),
		narrow.TypeKey{Kind: narrow.TypeKeyBuiltin, Name: "number"},
	)
	if !d.ApplyAtom(hasTypeAtom) {
		t.Fatal("ApplyAtom(hastype) should succeed")
	}

	// Apply numeric constraint via atom (x >= 0)
	geAtom := constraint.AtomGe(constraint.TermVar(key), constraint.TermConst(0))
	if !d.ApplyAtom(geAtom) {
		t.Fatal("ApplyAtom(ge) should succeed")
	}

	// Apply another numeric constraint (x <= 100)
	leAtom := constraint.AtomLe(constraint.TermVar(key), constraint.TermConst(100))
	if !d.ApplyAtom(leAtom) {
		t.Fatal("ApplyAtom(le) should succeed")
	}

	// Type domain should narrow to number only
	typeResult := d.Type.TypeAt(key)
	if typeResult == nil {
		t.Fatal("expected type domain result")
	}
	if typeResult.Kind() != typ.Number.Kind() {
		t.Fatalf("expected number type, got %v", typeResult)
	}

	// Numeric domain should have bounds [0, 100]
	lower, upper, ok := d.Numeric.State().BoundsFor(key)
	if !ok {
		t.Fatal("expected numeric bounds")
	}
	if lower != 0 {
		t.Fatalf("expected lower 0, got %d", lower)
	}
	if upper != 100 {
		t.Fatalf("expected upper 100, got %d", upper)
	}

	// ProductDomain.TypeAt should return number
	finalType := d.TypeAt(key)
	if finalType == nil {
		t.Fatal("expected ProductDomain.TypeAt result")
	}
	if finalType.Kind() != typ.Number.Kind() {
		t.Fatalf("expected ProductDomain.TypeAt to return number, got %v", finalType)
	}
}

func TestProductDomain_PlaceholderResolution(t *testing.T) {
	placeholderKey := constraint.PathKey("$0")
	versionedKey := constraint.PathKey("sym1@1")

	env := constraint.Env{
		ResolvePath: func(p constraint.Path) constraint.PathKey {
			if p.Symbol == 0 {
				if p.IsPlaceholder() {
					return constraint.PathKey(p.Root)
				}
				return ""
			}
			return versionedKey
		},
		PathTypeAt: func(key constraint.PathKey) typ.Type {
			if key == placeholderKey {
				return typ.NewUnion(typ.String, typ.Nil)
			}
			if key == versionedKey {
				return typ.Number
			}
			return nil
		},
	}

	d := NewProductDomain(env)

	// Apply truthy constraint to placeholder path
	atom := constraint.AtomTruthy(constraint.TermVar(placeholderKey))
	if !d.ApplyAtom(atom) {
		t.Fatal("ApplyAtom should succeed")
	}

	// Placeholder should be narrowed to string (nil removed)
	result := d.Type.TypeAt(placeholderKey)
	if result == nil {
		t.Fatal("expected narrowed type for placeholder")
	}
	if result.Kind() != typ.String.Kind() {
		t.Fatalf("expected string, got %v", result)
	}
}

func TestProductDomain_NonPlaceholderSymbol0Rejected(t *testing.T) {
	env := constraint.Env{
		ResolvePath: func(p constraint.Path) constraint.PathKey {
			if p.Symbol == 0 {
				if p.IsPlaceholder() {
					return constraint.PathKey(p.Root)
				}
				return ""
			}
			return p.Key()
		},
	}

	// Non-placeholder with Symbol=0 should return empty key
	nonPlaceholder := constraint.Path{Root: "regularVar", Symbol: 0}
	resolved := env.ResolvePath(nonPlaceholder)

	if resolved != "" {
		t.Fatalf("non-placeholder Symbol=0 should resolve to empty, got %q", resolved)
	}

	// Placeholder with Symbol=0 should resolve to root
	placeholder := constraint.Path{Root: "$0", Symbol: 0}
	resolved = env.ResolvePath(placeholder)

	if resolved != "$0" {
		t.Fatalf("placeholder should resolve to root, got %q", resolved)
	}
}

func TestProductDomain_EqPathPropagation(t *testing.T) {
	typeA := typ.NewRecord().Field("tag", typ.LiteralString("a")).Build()
	typeB := typ.NewRecord().Field("tag", typ.LiteralString("b")).Build()
	union := typ.NewUnion(typeA, typeB)

	pathX := constraint.Path{Root: "x", Symbol: 1}
	pathY := constraint.Path{Root: "y", Symbol: 2}
	xKey := pathX.Key()
	yKey := pathY.Key()

	resolver := &core.FuncResolver{
		FieldFunc: func(t typ.Type, name string) (typ.Type, bool) {
			if r, ok := t.(*typ.Record); ok {
				for _, f := range r.Fields {
					if f.Name == name {
						return f.Type, true
					}
				}
			}
			return nil, false
		},
	}

	env := constraint.Env{
		ResolvePath: func(p constraint.Path) constraint.PathKey {
			if p.Symbol == 0 {
				if p.IsPlaceholder() {
					return constraint.PathKey(p.Root)
				}
				return ""
			}
			return p.Key()
		},
		PathTypeAt: func(key constraint.PathKey) typ.Type {
			if key == xKey || key == yKey {
				return union
			}
			return nil
		},
		Resolver: resolver,
	}

	d := NewProductDomain(env)

	// Apply EqPath{x, y} AND FieldEquals{y, "tag", "a"}
	constraints := []constraint.Constraint{
		constraint.NewEqPath(pathX, pathY),
		constraint.FieldEquals{Target: pathY, Field: "tag", Value: typ.LiteralString("a")},
	}

	if !d.ApplyConjunction(constraints) {
		t.Fatal("ApplyConjunction should succeed")
	}

	// Check y is narrowed
	yNarrowed := d.Shape.NarrowedTypeAt(yKey)
	if yNarrowed == nil {
		t.Fatal("y should be narrowed")
	}
	t.Logf("y narrowed to: %v", yNarrowed)

	// Check x is narrowed via propagation
	xNarrowed := d.Shape.NarrowedTypeAt(xKey)
	if xNarrowed == nil {
		t.Error("x should be narrowed via EqPath propagation")
		t.Logf("Shape.Narrowed keys: %v", d.Shape.Narrowed)
	} else {
		t.Logf("x narrowed to: %v", xNarrowed)
	}

	// Also verify TypeAt returns narrowed type
	xFromTypeAt := d.TypeAt(xKey)
	if xFromTypeAt == nil {
		t.Error("TypeAt(x) returned nil")
	} else if !typ.TypeEquals(xFromTypeAt, typeA) {
		t.Errorf("TypeAt(x) = %v, want %v", xFromTypeAt, typeA)
	}
}

func TestProductDomain_EqPathPropagation_ViaDNF(t *testing.T) {
	typeA := typ.NewRecord().Field("tag", typ.LiteralString("a")).Build()
	typeB := typ.NewRecord().Field("tag", typ.LiteralString("b")).Build()
	union := typ.NewUnion(typeA, typeB)

	pathX := constraint.Path{Root: "x", Symbol: 1}
	pathY := constraint.Path{Root: "y", Symbol: 2}
	xKey := pathX.Key()
	yKey := pathY.Key()

	resolver := &core.FuncResolver{
		FieldFunc: func(t typ.Type, name string) (typ.Type, bool) {
			if r, ok := t.(*typ.Record); ok {
				for _, f := range r.Fields {
					if f.Name == name {
						return f.Type, true
					}
				}
			}
			return nil, false
		},
	}

	env := constraint.Env{
		ResolvePath: func(p constraint.Path) constraint.PathKey {
			if p.Symbol == 0 {
				if p.IsPlaceholder() {
					return constraint.PathKey(p.Root)
				}
				return ""
			}
			return p.Key()
		},
		PathTypeAt: func(key constraint.PathKey) typ.Type {
			if key == xKey || key == yKey {
				return union
			}
			return nil
		},
		Resolver: resolver,
	}

	// Create constraints
	constraints := []constraint.Constraint{
		constraint.NewEqPath(pathX, pathY),
		constraint.FieldEquals{Target: pathY, Field: "tag", Value: typ.LiteralString("a")},
	}

	// Apply via DNF condition (like solver does)
	cond := constraint.FromConstraints(constraints...)
	d := NewProductDomain(env)

	if !d.ApplyCondition(cond) {
		t.Fatal("ApplyCondition should succeed")
	}

	// Check y is narrowed
	yNarrowed := d.Shape.NarrowedTypeAt(yKey)
	if yNarrowed == nil {
		t.Error("y should be narrowed")
	} else {
		t.Logf("y narrowed to: %v", yNarrowed)
	}

	// Check x is narrowed
	xNarrowed := d.Shape.NarrowedTypeAt(xKey)
	if xNarrowed == nil {
		t.Error("x should be narrowed via EqPath propagation")
		var keys []constraint.PathKey
		for k := range d.Shape.Narrowed {
			keys = append(keys, k)
		}
		t.Logf("Shape.Narrowed keys: %v", keys)
	} else {
		t.Logf("x narrowed to: %v", xNarrowed)
	}

	// Check TypeAt returns correct values
	xFromTypeAt := d.TypeAt(xKey)
	if xFromTypeAt == nil || !typ.TypeEquals(xFromTypeAt, typeA) {
		t.Errorf("TypeAt(x) = %v, want %v", xFromTypeAt, typeA)
	}
}

func TestCongruenceClosurePropagation(t *testing.T) {
	// x, y, z with different symbols
	pathX := constraint.Path{Root: "x", Symbol: 1}
	pathY := constraint.Path{Root: "y", Symbol: 2}
	pathZ := constraint.Path{Root: "z", Symbol: 3}

	types := map[constraint.PathKey]typ.Type{
		pathX.Key(): typ.NewUnion(typ.String, typ.Number),
		pathY.Key(): typ.NewUnion(typ.String, typ.Number, typ.Boolean),
		pathZ.Key(): typ.NewUnion(typ.String, typ.Number, typ.Boolean, typ.Nil),
	}

	env := makeMockEnv(types)
	dom := NewProductDomain(env)

	// Constraints: x == y, y == z, type(x) == "string"
	constraints := []constraint.Constraint{
		constraint.EqPath{Left: pathX, Right: pathY},
		constraint.EqPath{Left: pathY, Right: pathZ},
		constraint.HasType{Path: pathX, Type: narrow.BuiltinTypeKey("string")},
	}

	ok := dom.ApplyConjunction(constraints)
	if !ok {
		t.Fatal("ApplyConjunction should succeed")
	}

	t.Logf("E-graph paths:")
	for _, k := range dom.EGraph.AllPaths() {
		t.Logf("  %s -> root: %s", k, dom.EGraph.Find(k))
	}

	t.Logf("Type narrowings:")
	for k, v := range dom.Type.Narrowed {
		t.Logf("  %s -> %v", k, v)
	}

	// Check x is narrowed
	xType := dom.TypeAt(pathX.Key())
	t.Logf("x type: %v", xType)
	if xType == nil || !typ.TypeEquals(xType, typ.String) {
		t.Errorf("x should be string, got %v", xType)
	}

	// Check y is narrowed via congruence
	yType := dom.TypeAt(pathY.Key())
	t.Logf("y type: %v", yType)
	if yType == nil || !typ.TypeEquals(yType, typ.String) {
		t.Errorf("y should be string via congruence, got %v", yType)
	}

	// Check z is narrowed via congruence
	zType := dom.TypeAt(pathZ.Key())
	t.Logf("z type: %v", zType)
	if zType == nil || !typ.TypeEquals(zType, typ.String) {
		t.Errorf("z should be string via congruence, got %v", zType)
	}
}

func TestCongruenceClosureUnsatOnIncompatibleTypes(t *testing.T) {
	// Test that x == y with incompatible types results in unsat
	pathX := constraint.Path{Root: "x", Symbol: 1}
	pathY := constraint.Path{Root: "y", Symbol: 2}
	xKey := pathX.Key()
	yKey := pathY.Key()

	env := constraint.Env{
		ResolvePath: func(p constraint.Path) constraint.PathKey {
			if p.Symbol == 0 {
				return ""
			}
			return p.Key()
		},
		PathTypeAt: func(key constraint.PathKey) typ.Type {
			switch key {
			case xKey:
				return typ.String // x is string
			case yKey:
				return typ.Number // y is number
			}
			return nil
		},
	}

	dom := NewProductDomain(env)

	// EqPath(x, y) with x:string and y:number should be unsat
	constraints := []constraint.Constraint{
		constraint.NewEqPath(pathX, pathY),
	}

	ok := dom.ApplyConjunction(constraints)
	if ok {
		t.Error("ApplyConjunction should return false for incompatible EqPath types")
	}
	if !dom.IsUnsat() {
		t.Error("Domain should be unsat when EqPath operands have incompatible types")
	}
}

func TestEGraphCongruenceClosurePropagation(t *testing.T) {
	// Test that EqPath constraints propagate narrowings across equivalence classes
	typeA := typ.NewRecord().Field("tag", typ.LiteralString("a")).Build()
	typeB := typ.NewRecord().Field("tag", typ.LiteralString("b")).Build()
	union := typ.NewUnion(typeA, typeB)

	pathX := constraint.Path{Root: "x", Symbol: 1}
	pathY := constraint.Path{Root: "y", Symbol: 2}
	xKey := pathX.Key()
	yKey := pathY.Key()

	resolver := &core.FuncResolver{
		FieldFunc: func(t typ.Type, name string) (typ.Type, bool) {
			if r, ok := t.(*typ.Record); ok {
				for _, f := range r.Fields {
					if f.Name == name {
						return f.Type, true
					}
				}
			}
			return nil, false
		},
	}

	env := constraint.Env{
		ResolvePath: func(p constraint.Path) constraint.PathKey {
			if p.Symbol == 0 {
				if p.IsPlaceholder() {
					return constraint.PathKey(p.Root)
				}
				return ""
			}
			return p.Key()
		},
		PathTypeAt: func(key constraint.PathKey) typ.Type {
			if key == xKey || key == yKey {
				return union
			}
			return nil
		},
		Resolver: resolver,
	}

	d := NewProductDomain(env)

	constraints := []constraint.Constraint{
		constraint.NewEqPath(pathX, pathY),
		constraint.FieldEquals{Target: pathY, Field: "tag", Value: typ.LiteralString("a")},
	}

	if !d.ApplyConjunction(constraints) {
		t.Fatal("ApplyConjunction should succeed")
	}

	// Verify EGraph unified x and y
	if d.EGraph.Find(xKey) != d.EGraph.Find(yKey) {
		t.Error("x and y should be in same equivalence class")
	}

	// Verify both are narrowed
	if d.Shape.NarrowedTypeAt(yKey) == nil {
		t.Error("y should be narrowed")
	}
	if d.Shape.NarrowedTypeAt(xKey) == nil {
		t.Error("x should be narrowed via EqPath propagation")
	}
}
