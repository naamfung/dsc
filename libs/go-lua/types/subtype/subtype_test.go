package subtype

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

// TDD order from ARCHITECTURE.md

func TestReflexivity(t *testing.T) {
	types := []typ.Type{
		typ.Nil,
		typ.Boolean,
		typ.Number,
		typ.Integer,
		typ.String,
		typ.Any,
		typ.Unknown,
		typ.Never,
	}
	for _, ty := range types {
		if !IsSubtype(ty, ty) {
			t.Errorf("%s should be subtype of itself", ty)
		}
	}
}

func TestNeverBottom(t *testing.T) {
	types := []typ.Type{
		typ.Nil,
		typ.Boolean,
		typ.Number,
		typ.String,
		typ.Any,
	}
	for _, ty := range types {
		if !IsSubtype(typ.Never, ty) {
			t.Errorf("never should be subtype of %s", ty)
		}
	}
}

func TestAnyTop(t *testing.T) {
	types := []typ.Type{
		typ.Nil,
		typ.Boolean,
		typ.Number,
		typ.String,
		typ.Unknown,
		typ.Never,
	}
	for _, ty := range types {
		if !IsSubtype(ty, typ.Any) {
			t.Errorf("%s should be subtype of any", ty)
		}
	}
}

func TestAnyBottom(t *testing.T) {
	types := []typ.Type{
		typ.Nil,
		typ.Boolean,
		typ.Number,
		typ.String,
	}
	for _, ty := range types {
		if IsSubtype(typ.Any, ty) {
			t.Errorf("any should NOT be subtype of %s", ty)
		}
	}
	// any may flow to unknown (top) if unknown is treated as unconstrained
	if !IsSubtype(typ.Any, typ.Unknown) {
		t.Error("any should be subtype of unknown")
	}
}

func TestUnknownTopNotBottom(t *testing.T) {
	if !IsSubtype(typ.String, typ.Unknown) {
		t.Error("string should be subtype of unknown (top)")
	}
	if IsSubtype(typ.Unknown, typ.String) {
		t.Error("unknown should not be subtype of string")
	}
}

func TestLiteralToBase(t *testing.T) {
	tests := []struct {
		lit  *typ.Literal
		base typ.Type
		want bool
	}{
		{typ.True, typ.Boolean, true},
		{typ.False, typ.Boolean, true},
		{typ.LiteralInt(42), typ.Integer, true},
		{typ.LiteralInt(42), typ.Number, true},
		{typ.LiteralNumber(3.14), typ.Number, true},
		{typ.LiteralString("hello"), typ.String, true},
		{typ.True, typ.String, false},
		{typ.LiteralInt(42), typ.Boolean, false},
	}
	for _, tt := range tests {
		got := IsSubtype(tt.lit, tt.base)
		if got != tt.want {
			t.Errorf("IsSubtype(%s, %s) = %v, want %v", tt.lit, tt.base, got, tt.want)
		}
	}
}

func TestIntegerSubNumber(t *testing.T) {
	if !IsSubtype(typ.Integer, typ.Number) {
		t.Error("integer should be subtype of number")
	}

	if IsSubtype(typ.Number, typ.Integer) {
		t.Error("number should not be subtype of integer")
	}
}

func TestOptional(t *testing.T) {
	optStr := typ.NewOptional(typ.String)

	// T <: T?
	if !IsSubtype(typ.String, optStr) {
		t.Error("string should be subtype of string?")
	}

	// nil <: T?
	if !IsSubtype(typ.Nil, optStr) {
		t.Error("nil should be subtype of string?")
	}

	// T? <: T should be false
	if IsSubtype(optStr, typ.String) {
		t.Error("string? should not be subtype of string")
	}
}

func TestUnionSub(t *testing.T) {
	strOrNum := typ.NewUnion(typ.String, typ.Number)

	// Each member <: union
	if !IsSubtype(typ.String, strOrNum) {
		t.Error("string should be subtype of string|number")
	}

	if !IsSubtype(typ.Number, strOrNum) {
		t.Error("number should be subtype of string|number")
	}

	// Other type not subtype
	if IsSubtype(typ.Boolean, strOrNum) {
		t.Error("boolean should not be subtype of string|number")
	}
}

func TestUnionSuper(t *testing.T) {
	strOrNum := typ.NewUnion(typ.String, typ.Number)

	// union <: any
	if !IsSubtype(strOrNum, typ.Any) {
		t.Error("string|number should be subtype of any")
	}

	// union <: string (not all members match)
	if IsSubtype(strOrNum, typ.String) {
		t.Error("string|number should not be subtype of string")
	}
}

func TestIntersectionSub(t *testing.T) {
	// intersection <: T if any member <: T
	// string&number <: string (any member satisfies)
	inter := typ.NewIntersection(typ.String, typ.Number)
	if !IsSubtype(inter, typ.String) {
		t.Error("string&number should be subtype of string")
	}

	if !IsSubtype(inter, typ.Number) {
		t.Error("string&number should be subtype of number")
	}
}

func TestFunction(t *testing.T) {
	// (string) -> number
	f1 := typ.Func().Param("x", typ.String).Returns(typ.Number).Build()
	// (string) -> number (identical)
	f2 := typ.Func().Param("x", typ.String).Returns(typ.Number).Build()

	if !IsSubtype(f1, f2) {
		t.Error("identical functions should be subtypes")
	}

	// Covariant return: (string) -> integer <: (string) -> number
	f3 := typ.Func().Param("x", typ.String).Returns(typ.Integer).Build()
	if !IsSubtype(f3, f1) {
		t.Error("function with more specific return should be subtype")
	}

	// Contravariant param: (any) -> number <: (string) -> number
	f4 := typ.Func().Param("x", typ.Any).Returns(typ.Number).Build()
	if !IsSubtype(f4, f1) {
		t.Error("function with more general param should be subtype")
	}
}

func TestRecord(t *testing.T) {
	// {name: string}
	r1 := typ.NewRecord().Field("name", typ.String).Build()
	// {name: string, age: number}
	r2 := typ.NewRecord().Field("name", typ.String).Field("age", typ.Number).Build()

	// Width subtyping: more fields <: fewer fields
	if !IsSubtype(r2, r1) {
		t.Error("record with more fields should be subtype")
	}

	if IsSubtype(r1, r2) {
		t.Error("record with fewer fields should not be subtype (missing required)")
	}

	// Optional fields
	r3 := typ.NewRecord().Field("name", typ.String).OptField("age", typ.Number).Build()
	if !IsSubtype(r2, r3) {
		t.Error("record with required field should be subtype of optional")
	}
}

func TestArray(t *testing.T) {
	arrStr := typ.NewArray(typ.String)
	arrAny := typ.NewArray(typ.Any)

	// Arrays are covariant
	if !IsSubtype(arrStr, arrAny) {
		t.Error("string[] should be subtype of any[]")
	}
}

func TestMap(t *testing.T) {
	mapStrNum := typ.NewMap(typ.String, typ.Number)
	mapStrInt := typ.NewMap(typ.String, typ.Integer)

	// Maps are invariant
	if IsSubtype(mapStrInt, mapStrNum) {
		t.Error("map values are invariant")
	}
}

func TestTuple(t *testing.T) {
	t1 := typ.NewTuple(typ.String, typ.Number)
	t2 := typ.NewTuple(typ.String, typ.Integer)

	// Covariant elements
	if !IsSubtype(t2, t1) {
		t.Error("tuple with more specific elements should be subtype")
	}

	// Different lengths
	t3 := typ.NewTuple(typ.String)
	if IsSubtype(t3, t1) {
		t.Error("different length tuples should not be subtypes")
	}
}

func TestCycleDetection(t *testing.T) {
	// Create recursive types via generic
	param := typ.NewTypeParam("T", nil)
	g := typ.NewGeneric("List", []*typ.TypeParam{param}, nil)

	inst1 := typ.Instantiate(g, typ.String)
	inst2 := typ.Instantiate(g, typ.String)

	if !IsSubtype(inst1, inst2) {
		t.Error("identical instantiated types should be subtypes")
	}
}

// Record edge cases

func TestRecordRequiredVsOptional(t *testing.T) {
	// {x: number} <: {x?: number}
	sub := typ.NewRecord().Field("x", typ.Number).Build()
	super := typ.NewRecord().OptField("x", typ.Number).Build()

	if !IsSubtype(sub, super) {
		t.Error("required field should satisfy optional field")
	}

	if IsSubtype(super, sub) {
		t.Error("optional field should not satisfy required field")
	}
}

func TestRecordOptionalSubFieldAllowedWhenSuperTypeAdmitsNil(t *testing.T) {
	// Super field is syntactically required, but type admits nil (string?).
	// Sub optional field should still be accepted.
	sub := typ.NewRecord().OptField("status_not", typ.LiteralString("removed")).Build()
	super := typ.NewRecord().Field("status_not", typ.NewOptional(typ.String)).Build()

	if !IsSubtype(sub, super) {
		t.Error("optional sub field should satisfy required super field when super type admits nil")
	}
}

func TestRecordWidthSubtyping(t *testing.T) {
	// {x, y} <: {x}
	sub := typ.NewRecord().Field("x", typ.Number).Field("y", typ.String).Build()
	super := typ.NewRecord().Field("x", typ.Number).Build()

	if !IsSubtype(sub, super) {
		t.Error("record with more fields should be subtype")
	}
}

func TestRecordReadonlyFieldVsMutable(t *testing.T) {
	readonly := typ.NewRecord().ReadonlyField("x", typ.Number).Build()
	mutable := typ.NewRecord().Field("x", typ.Number).Build()

	// Reflexivity
	if !IsSubtype(readonly, readonly) {
		t.Error("record with readonly field should be subtype of itself")
	}

	if !IsSubtype(mutable, mutable) {
		t.Error("record with mutable field should be subtype of itself")
	}

	// Readonly sub cannot satisfy mutable super (can't write to readonly through mutable interface)
	if IsSubtype(readonly, mutable) {
		t.Error("readonly field should not satisfy mutable field requirement")
	}

	// Mutable sub can satisfy readonly super (reading from mutable is fine)
	readonlySuper := typ.NewRecord().ReadonlyField("x", typ.Number).Build()
	if !IsSubtype(mutable, readonlySuper) {
		t.Error("mutable field should satisfy readonly field requirement")
	}
}

func TestRecordMutableFieldInvariance(t *testing.T) {
	// This tests soundness: prevents writes through a supertype from corrupting the subtype
	// Example: if {pet: Dog} <: {pet: Animal} were allowed, you could:
	//   local d: {pet: Dog} = {pet = Dog.new()}
	//   local a: {pet: Animal} = d  -- upcast
	//   a.pet = Cat.new()           -- write Cat through Animal field
	//   d.pet:bark()                -- d.pet is now a Cat!

	dog := typ.LiteralString("dog")
	animal := typ.NewUnion(typ.LiteralString("dog"), typ.LiteralString("cat"))

	recordDog := typ.NewRecord().Field("pet", dog).Build()
	recordAnimal := typ.NewRecord().Field("pet", animal).Build()

	// Dog <: Animal
	if !IsSubtype(dog, animal) {
		t.Fatal("precondition: dog should be subtype of animal")
	}

	// {pet: Dog} should NOT be subtype of {pet: Animal} for mutable fields
	if IsSubtype(recordDog, recordAnimal) {
		t.Error("record with narrower mutable field should not be subtype (invariance required)")
	}

	// {pet: Animal} should NOT be subtype of {pet: Dog} either
	if IsSubtype(recordAnimal, recordDog) {
		t.Error("record with wider mutable field should not be subtype")
	}

	// Same types should still work
	recordDog2 := typ.NewRecord().Field("pet", dog).Build()
	if !IsSubtype(recordDog, recordDog2) {
		t.Error("records with same mutable field types should be subtypes")
	}
}

func TestRecordMutableTupleFieldLiteralWidening(t *testing.T) {
	super := typ.NewRecord().
		Field("chunks", typ.NewTuple(typ.String, typ.String)).
		Build()

	sub := typ.NewRecord().
		Field("chunks", typ.NewTuple(typ.LiteralString("a"), typ.LiteralString("b"))).
		Build()

	if !IsSubtype(sub, super) {
		t.Error("tuple literal elements should widen in mutable record fields")
	}
}

func TestRecordMutableFieldUnionBranchWidening(t *testing.T) {
	super := typ.NewRecord().
		Field("timeout", typ.NewUnion(typ.Number, typ.String)).
		Build()

	sub := typ.NewRecord().
		Field("timeout", typ.Number).
		Build()

	if !IsSubtype(sub, super) {
		t.Error("concrete union branch should satisfy mutable field via widening")
	}
}

func TestRecordReadonlyFieldCovariance(t *testing.T) {
	// Readonly fields are safe for covariant subtyping
	dog := typ.LiteralString("dog")
	animal := typ.NewUnion(typ.LiteralString("dog"), typ.LiteralString("cat"))

	readonlyDog := typ.NewRecord().ReadonlyField("pet", dog).Build()
	readonlyAnimal := typ.NewRecord().ReadonlyField("pet", animal).Build()

	// {readonly pet: Dog} <: {readonly pet: Animal} is sound
	if !IsSubtype(readonlyDog, readonlyAnimal) {
		t.Error("readonly record with narrower type should be subtype (covariance is sound)")
	}

	// {readonly pet: Animal} should NOT be subtype of {readonly pet: Dog}
	if IsSubtype(readonlyAnimal, readonlyDog) {
		t.Error("readonly record with wider type should not be subtype")
	}
}

func TestRecordMutableToReadonlyCovariance(t *testing.T) {
	// A mutable field can satisfy a readonly super field, with covariant type check
	dog := typ.LiteralString("dog")
	animal := typ.NewUnion(typ.LiteralString("dog"), typ.LiteralString("cat"))

	mutableDog := typ.NewRecord().Field("pet", dog).Build()
	readonlyAnimal := typ.NewRecord().ReadonlyField("pet", animal).Build()

	// {pet: Dog} <: {readonly pet: Animal} is sound
	if !IsSubtype(mutableDog, readonlyAnimal) {
		t.Error("mutable field with narrower type should satisfy readonly super with wider type")
	}
}

// Function edge cases

func TestFunctionNeverReturn(t *testing.T) {
	// () -> never <: () -> number
	f1 := typ.Func().Returns(typ.Never).Build()
	f2 := typ.Func().Returns(typ.Number).Build()

	if !IsSubtype(f1, f2) {
		t.Error("never in return position should be subtype")
	}
}

func TestFunctionAnyParameter(t *testing.T) {
	// (any) -> nil <: (number) -> nil
	f1 := typ.Func().Param("x", typ.Any).Returns(typ.Nil).Build()
	f2 := typ.Func().Param("x", typ.Number).Returns(typ.Nil).Build()

	if !IsSubtype(f1, f2) {
		t.Error("any in parameter position should work")
	}
}

func TestFunctionOptionalContravariance(t *testing.T) {
	// (number?) -> string
	optNum := typ.NewOptional(typ.Number)
	f1 := typ.Func().Param("x", optNum).Returns(typ.String).Build()

	// (number) -> string
	f2 := typ.Func().Param("x", typ.Number).Returns(typ.String).Build()

	// Test reflexivity
	if !IsSubtype(f1, f1) {
		t.Error("function with optional param should be subtype of itself")
	}

	if !IsSubtype(f2, f2) {
		t.Error("function with required param should be subtype of itself")
	}
}

func TestFunctionVariadicSame(t *testing.T) {
	f1 := typ.Func().Param("x", typ.Number).Variadic(typ.String).Returns(typ.Nil).Build()
	f2 := typ.Func().Param("x", typ.Number).Variadic(typ.String).Returns(typ.Nil).Build()

	if !IsSubtype(f1, f2) {
		t.Error("functions with same variadic should be subtypes")
	}
}

func TestFunctionVariadicMismatch(t *testing.T) {
	f1 := typ.Func().Param("x", typ.Number).Variadic(typ.Number).Returns(typ.Nil).Build()
	f2 := typ.Func().Param("x", typ.Number).Variadic(typ.String).Returns(typ.Nil).Build()

	if IsSubtype(f1, f2) {
		t.Error("variadic type mismatch should prevent subtyping")
	}
}

// Optional edge cases

func TestOptionalNested(t *testing.T) {
	// number? <: number??
	opt1 := typ.NewOptional(typ.Number)
	opt2 := typ.NewOptional(opt1)

	if !IsSubtype(opt1, opt2) {
		t.Error("T? should be subtype of T??")
	}
}

// Array/Map edge cases

func TestArrayInvariant(t *testing.T) {
	// Arrays are covariant
	arrInt := typ.NewArray(typ.Integer)
	arrNum := typ.NewArray(typ.Number)

	if !IsSubtype(arrInt, arrNum) {
		t.Error("integer[] should be subtype of number[]")
	}
}

func TestArrayReflexive(t *testing.T) {
	arr := typ.NewArray(typ.Number)
	if !IsSubtype(arr, arr) {
		t.Error("array should be subtype of itself")
	}
}

func TestMapInvariant(t *testing.T) {
	// Maps are invariant
	mapStrInt := typ.NewMap(typ.String, typ.Integer)
	mapStrNum := typ.NewMap(typ.String, typ.Number)

	if IsSubtype(mapStrInt, mapStrNum) {
		t.Error("mutable maps should be invariant in value type")
	}
}

func TestMapReflexive(t *testing.T) {
	m := typ.NewMap(typ.String, typ.Number)
	if !IsSubtype(m, m) {
		t.Error("map should be subtype of itself")
	}
}

// Tuple edge cases

func TestTupleCovariantElements(t *testing.T) {
	// [integer, string] <: [number, string]
	sub := typ.NewTuple(typ.Integer, typ.String)
	super := typ.NewTuple(typ.Number, typ.String)

	if !IsSubtype(sub, super) {
		t.Error("tuple elements should be covariant")
	}
}

func TestTupleDifferentLengths(t *testing.T) {
	t1 := typ.NewTuple(typ.String, typ.Number)
	t2 := typ.NewTuple(typ.String)

	if IsSubtype(t1, t2) || IsSubtype(t2, t1) {
		t.Error("different length tuples should not be subtypes")
	}
}

// Union/Intersection edge cases

func TestUnionNeverElimination(t *testing.T) {
	// never | string = string
	normalized := NormalizeUnion(typ.Never, typ.String)

	if normalized != typ.String {
		t.Errorf("never in union should be eliminated, got %s", normalized)
	}
}

func TestIntersectionAnyElimination(t *testing.T) {
	// any & string = string
	normalized := NormalizeIntersection(typ.Any, typ.String)

	if normalized != typ.String {
		t.Errorf("any in intersection should be eliminated, got %s", normalized)
	}
}

// Literal edge cases

func TestLiteralInUnion(t *testing.T) {
	// "hello" | "world" <: string
	lit1 := typ.LiteralString("hello")
	lit2 := typ.LiteralString("world")
	union := typ.NewUnion(lit1, lit2)

	if !IsSubtype(union, typ.String) {
		t.Error("union of literals should be subtype of base type")
	}

	if IsSubtype(typ.String, union) {
		t.Error("base type should not be subtype of literal union")
	}
}

// Alias edge cases

func TestAliasTransparency(t *testing.T) {
	alias := typ.NewAlias("MyNumber", typ.Number)

	if !IsSubtype(alias, typ.Number) {
		t.Error("alias should be transparent to underlying type")
	}

	if !IsSubtype(typ.Number, alias) {
		t.Error("underlying type should be subtype of alias")
	}
}

// TypeParam edge cases

func TestTypeParamReflexivity(t *testing.T) {
	tp := typ.NewTypeParam("T", typ.Number)

	if !IsSubtype(tp, tp) {
		t.Error("TypeParam should be subtype of itself")
	}
}

// Reflexivity extended

func TestReflexivityComplex(t *testing.T) {
	types := []typ.Type{
		typ.NewOptional(typ.String),
		typ.NewUnion(typ.String, typ.Number),
		typ.NewIntersection(typ.String, typ.Number),
		typ.NewArray(typ.Number),
		typ.NewMap(typ.String, typ.Number),
		typ.NewTuple(typ.String, typ.Number),
		typ.Func().Param("x", typ.String).Returns(typ.Number).Build(),
		typ.NewRecord().Field("x", typ.Number).Build(),
	}
	for _, ty := range types {
		if !IsSubtype(ty, ty) {
			t.Errorf("%s should be subtype of itself", ty)
		}
	}
}

// Instantiated type tests

func TestInstantiatedSameGeneric(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	g := typ.NewGeneric("Box", []*typ.TypeParam{param}, nil)

	inst1 := typ.Instantiate(g, typ.String)
	inst2 := typ.Instantiate(g, typ.String)

	if !IsSubtype(inst1, inst2) {
		t.Error("same instantiation should be subtype")
	}
}

func TestInstantiatedDifferentArgs(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	g := typ.NewGeneric("Box", []*typ.TypeParam{param}, nil)

	instStr := typ.Instantiate(g, typ.String)
	instNum := typ.Instantiate(g, typ.Number)

	if IsSubtype(instStr, instNum) {
		t.Error("different type args should not be subtypes (invariant)")
	}
}

func TestInstantiatedDifferentGenerics(t *testing.T) {
	param1 := typ.NewTypeParam("T", nil)
	param2 := typ.NewTypeParam("T", nil)
	g1 := typ.NewGeneric("Box1", []*typ.TypeParam{param1}, nil)
	g2 := typ.NewGeneric("Box2", []*typ.TypeParam{param2}, nil)

	inst1 := typ.Instantiate(g1, typ.String)
	inst2 := typ.Instantiate(g2, typ.String)

	if IsSubtype(inst1, inst2) {
		t.Error("different generics should not be subtypes")
	}
}

func TestAliasIntersectionUnionMemberSubtype(t *testing.T) {
	eventRecord := typ.NewRecord().Field("kind", typ.String).Build()
	eventMethods := typ.NewInterface("EventMethods", []typ.Method{
		{Name: "payload", Type: typ.Func().Param("self", typ.Self).Returns(typ.Any).Build()},
	})
	eventTarget := typ.NewIntersection(eventRecord, eventMethods)
	eventAlias := typ.NewAlias("Event", eventTarget)
	timeType := typ.NewInterface("Time", nil)
	union := typ.NewUnion(eventAlias, timeType)

	if !IsSubtype(eventAlias, union) {
		t.Error("alias should be subtype of union containing it")
	}

	if !IsSubtype(eventTarget, union) {
		t.Error("alias target should be subtype of union containing alias")
	}
}

func TestFunctionOptionalParams(t *testing.T) {
	// (x: number, y?: string) -> nil
	f1 := typ.Func().
		Param("x", typ.Number).
		OptParam("y", typ.String).
		Returns(typ.Nil).Build()

	// (x: number) -> nil
	f2 := typ.Func().
		Param("x", typ.Number).
		Returns(typ.Nil).Build()

	// Function with optional param should accept fewer args
	if !IsSubtype(f1, f1) {
		t.Error("function with optional param should be subtype of itself")
	}

	if !IsSubtype(f2, f2) {
		t.Error("function without optional should be subtype of itself")
	}
}

func TestFunctionDifferentParamCount(t *testing.T) {
	f1 := typ.Func().Param("x", typ.Number).Returns(typ.Nil).Build()
	f2 := typ.Func().Param("x", typ.Number).Param("y", typ.String).Returns(typ.Nil).Build()

	if IsSubtype(f2, f1) {
		t.Error("function requiring more params should not be subtype")
	}
}

func TestFunctionSubtype_RequiredAfterOptionalIsNotSubtype(t *testing.T) {
	// sub requires 2 positional args because required y appears after optional x.
	sub := typ.Func().
		OptParam("x", typ.Number).
		Param("y", typ.Number).
		Returns(typ.Nil).
		Build()

	// super can be called with a single positional arg.
	super := typ.Func().
		Param("x", typ.Number).
		Returns(typ.Nil).
		Build()

	if IsSubtype(sub, super) {
		t.Fatal("subtype should fail: sub requires 2 args while super requires only 1")
	}
}

func TestFunctionDifferentReturnCount(t *testing.T) {
	f1 := typ.Func().Returns(typ.Number).Build()
	f2 := typ.Func().Returns(typ.Number, typ.String).Build()

	if IsSubtype(f1, f2) {
		t.Error("different return counts should not be subtypes")
	}
}

func TestMapKeyMismatch(t *testing.T) {
	m1 := typ.NewMap(typ.String, typ.Number)
	m2 := typ.NewMap(typ.Number, typ.Number)

	if IsSubtype(m1, m2) {
		t.Error("maps with different key types should not be subtypes")
	}
}

func TestMapValueMismatch(t *testing.T) {
	m1 := typ.NewMap(typ.String, typ.Integer)
	m2 := typ.NewMap(typ.String, typ.Number)

	if IsSubtype(m1, m2) {
		t.Error("maps are invariant in value type")
	}
}

func TestTupleEmptyVsNonEmpty(t *testing.T) {
	empty := typ.NewTuple()
	nonEmpty := typ.NewTuple(typ.String)

	if IsSubtype(empty, nonEmpty) || IsSubtype(nonEmpty, empty) {
		t.Error("empty and non-empty tuples should not be subtypes")
	}
}

func TestTupleElementMismatch(t *testing.T) {
	t1 := typ.NewTuple(typ.String, typ.Number)
	t2 := typ.NewTuple(typ.String, typ.String)

	if IsSubtype(t1, t2) {
		t.Error("tuples with mismatched elements should not be subtypes")
	}
}

func TestRecordFieldTypeMismatch(t *testing.T) {
	r1 := typ.NewRecord().Field("x", typ.String).Build()
	r2 := typ.NewRecord().Field("x", typ.Number).Build()

	if IsSubtype(r1, r2) {
		t.Error("records with different field types should not be subtypes")
	}
}

func TestRecordOptionalVsRequired(t *testing.T) {
	r1 := typ.NewRecord().OptField("x", typ.Number).Build()
	r2 := typ.NewRecord().Field("x", typ.Number).Build()

	if IsSubtype(r1, r2) {
		t.Error("optional field should not satisfy required field")
	}
}

func TestInstantiatedNilGeneric(t *testing.T) {
	inst := &typ.Instantiated{Generic: nil, TypeArgs: []typ.Type{typ.String}}

	// Should not panic
	if IsSubtype(inst, typ.String) {
		t.Error("instantiated with nil generic should not be subtype of string")
	}
}

func TestInstantiatedMismatchedArgs(t *testing.T) {
	param1 := typ.NewTypeParam("T", nil)
	param2 := typ.NewTypeParam("U", nil)
	g := typ.NewGeneric("Pair", []*typ.TypeParam{param1, param2}, nil)

	inst1 := typ.Instantiate(g, typ.String, typ.Number)
	inst2 := typ.Instantiate(g, typ.String, typ.Integer)

	if IsSubtype(inst1, inst2) {
		t.Error("instantiated with different type args should not be subtypes")
	}
}

func TestNilSubtypeOfOptional(t *testing.T) {
	opt := typ.NewOptional(typ.Number)
	if !IsSubtype(typ.Nil, opt) {
		t.Error("nil should be subtype of optional")
	}
}

func TestUnionAllMembersSubtype(t *testing.T) {
	union := typ.NewUnion(typ.Integer, typ.Boolean)

	// All members need to be subtypes
	if IsSubtype(union, typ.Integer) {
		t.Error("union with non-integer member should not be subtype of integer")
	}
}

func TestIntersectionAllMembersNeeded(t *testing.T) {
	rec1 := typ.NewRecord().Field("x", typ.Number).Build()
	rec2 := typ.NewRecord().Field("y", typ.String).Build()
	inter := typ.NewIntersection(rec1, rec2)

	// Intersection should have both fields
	combined := typ.NewRecord().Field("x", typ.Number).Field("y", typ.String).Build()
	if !IsSubtype(combined, inter) {
		t.Error("record with all fields should be subtype of intersection")
	}
}

func TestCheckNilTypes(t *testing.T) {
	if IsSubtype(nil, typ.String) {
		t.Error("nil sub should fail")
	}

	if IsSubtype(typ.String, nil) {
		t.Error("nil super should fail")
	}

	if IsSubtype(nil, nil) {
		t.Error("both nil should fail")
	}
}

// Interface subtyping tests (structural method-set subtyping)

func TestInterfaceSupersetIsSubtype(t *testing.T) {
	// I1 has more methods than I2, so I1 <: I2 (I1 can be used where I2 is expected)
	i1 := typ.NewInterface("I1", []typ.Method{
		{Name: "foo", Type: typ.Func().Returns(typ.Number).Build()},
		{Name: "bar", Type: typ.Func().Param("s", typ.String).Returns(typ.String).Build()},
	})

	i2 := typ.NewInterface("I2", []typ.Method{
		{Name: "foo", Type: typ.Func().Returns(typ.Number).Build()},
	})

	if !IsSubtype(i1, i2) {
		t.Error("interface with superset of methods should be subtype")
	}

	if IsSubtype(i2, i1) {
		t.Error("interface with fewer methods should not be subtype of one with more")
	}
}

func TestInterfaceMissingMethodNotSubtype(t *testing.T) {
	i1 := typ.NewInterface("I1", []typ.Method{
		{Name: "foo", Type: typ.Func().Returns(typ.Number).Build()},
	})

	i2 := typ.NewInterface("I2", []typ.Method{
		{Name: "foo", Type: typ.Func().Returns(typ.Number).Build()},
		{Name: "bar", Type: typ.Func().Returns(typ.String).Build()},
	})

	if IsSubtype(i1, i2) {
		t.Error("interface missing method should not be subtype")
	}
}

func TestInterfaceMethodSignatureVariance(t *testing.T) {
	// Method return type is covariant: if method returns integer, it satisfies number return
	i1 := typ.NewInterface("I1", []typ.Method{
		{Name: "getValue", Type: typ.Func().Returns(typ.Integer).Build()},
	})

	i2 := typ.NewInterface("I2", []typ.Method{
		{Name: "getValue", Type: typ.Func().Returns(typ.Number).Build()},
	})

	if !IsSubtype(i1, i2) {
		t.Error("interface with covariant return type should be subtype")
	}

	// Reverse should not work
	if IsSubtype(i2, i1) {
		t.Error("interface with wider return type should not be subtype")
	}
}

func TestInterfaceMethodParamContravariance(t *testing.T) {
	// Method param type is contravariant: if method accepts number, it satisfies integer param requirement
	i1 := typ.NewInterface("I1", []typ.Method{
		{Name: "process", Type: typ.Func().Param("x", typ.Number).Returns(typ.Nil).Build()},
	})

	i2 := typ.NewInterface("I2", []typ.Method{
		{Name: "process", Type: typ.Func().Param("x", typ.Integer).Returns(typ.Nil).Build()},
	})

	if !IsSubtype(i1, i2) {
		t.Error("interface with contravariant param type should be subtype")
	}
}

func TestInterfaceSelfReflexive(t *testing.T) {
	i := typ.NewInterface("I", []typ.Method{
		{Name: "foo", Type: typ.Func().Returns(typ.String).Build()},
	})

	if !IsSubtype(i, i) {
		t.Error("interface should be subtype of itself")
	}
}

func TestInterfaceDisjointNotSubtype(t *testing.T) {
	// Two interfaces with completely different methods
	i1 := typ.NewInterface("I1", []typ.Method{
		{Name: "foo", Type: typ.Func().Returns(typ.String).Build()},
	})

	i2 := typ.NewInterface("I2", []typ.Method{
		{Name: "bar", Type: typ.Func().Returns(typ.Number).Build()},
	})

	if IsSubtype(i1, i2) {
		t.Error("disjoint interfaces should not be subtypes")
	}

	if IsSubtype(i2, i1) {
		t.Error("disjoint interfaces should not be subtypes (reverse)")
	}
}

// Edge case tests for recursive types

func TestRecursiveSubtypesSelf(t *testing.T) {
	// type Node = { next: Node? }
	rec := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typ.NewRecord().OptField("next", self).Build()
	})

	if !IsSubtype(rec, rec) {
		t.Error("recursive type should be subtype of itself")
	}
}

func TestRecursiveSubtypesEquivalent(t *testing.T) {
	// Two structurally identical recursive types
	rec1 := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typ.NewRecord().OptField("next", self).Build()
	})
	rec2 := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typ.NewRecord().OptField("next", self).Build()
	})

	if !IsSubtype(rec1, rec2) {
		t.Error("structurally equal recursive types should be subtypes")
	}
	if !IsSubtype(rec2, rec1) {
		t.Error("structurally equal recursive types should be subtypes (reverse)")
	}
}

func TestMutualRecursiveSubtypes(t *testing.T) {
	// type A = { b: B? }
	// type B = { a: A? }
	recA := typ.NewRecursivePlaceholder("A")
	recB := typ.NewRecursivePlaceholder("B")
	recA.SetBody(typ.NewRecord().OptField("b", recB).Build())
	recB.SetBody(typ.NewRecord().OptField("a", recA).Build())

	// Each should be subtype of itself
	if !IsSubtype(recA, recA) {
		t.Error("mutual recursive A should be subtype of itself")
	}
	if !IsSubtype(recB, recB) {
		t.Error("mutual recursive B should be subtype of itself")
	}

	// A and B should NOT be subtypes of each other
	if IsSubtype(recA, recB) {
		t.Error("A should not be subtype of B")
	}
	if IsSubtype(recB, recA) {
		t.Error("B should not be subtype of A")
	}
}

func TestTripleMutualRecursive(t *testing.T) {
	// type A = { b: B? }
	// type B = { c: C? }
	// type C = { a: A? }
	recA := typ.NewRecursivePlaceholder("A")
	recB := typ.NewRecursivePlaceholder("B")
	recC := typ.NewRecursivePlaceholder("C")
	recA.SetBody(typ.NewRecord().OptField("b", recB).Build())
	recB.SetBody(typ.NewRecord().OptField("c", recC).Build())
	recC.SetBody(typ.NewRecord().OptField("a", recA).Build())

	// Each should be subtype of itself
	if !IsSubtype(recA, recA) {
		t.Error("A should be subtype of itself")
	}
	if !IsSubtype(recB, recB) {
		t.Error("B should be subtype of itself")
	}
	if !IsSubtype(recC, recC) {
		t.Error("C should be subtype of itself")
	}
}

func TestRecursiveAliasRecordSubtype_WithSelfMethodField(t *testing.T) {
	rec := typ.NewRecursivePlaceholder("Message")
	msgAlias := typ.NewAlias("Message", rec)
	rec.SetBody(typ.NewRecord().
		Field("_topic", typ.String).
		Field("topic", typ.Func().Param("self", rec).Returns(typ.String).Build()).
		Build())

	synthesized := typ.NewRecord().
		Field("_topic", typ.String).
		Field("topic", typ.Func().Param("s", msgAlias).Returns(typ.String).Build()).
		Build()

	if !IsSubtype(synthesized, msgAlias) {
		t.Fatal("record literal with Message-annotated self method should subtype recursive Message alias")
	}
}

func TestRecursiveAliasRecordSubtype_InsideUnionMember(t *testing.T) {
	rec := typ.NewRecursivePlaceholder("Message")
	msgAlias := typ.NewAlias("Message", rec)
	rec.SetBody(typ.NewRecord().
		Field("_topic", typ.String).
		Field("topic", typ.Func().Param("self", rec).Returns(typ.String).Build()).
		Build())

	msgCh := typ.NewAlias("MsgCh", typ.NewRecord().Field("__tag", typ.LiteralString("msg")).Build())
	timerCh := typ.NewAlias("TimerCh", typ.NewRecord().Field("__tag", typ.LiteralString("timer")).Build())
	timer := typ.NewRecord().Field("elapsed", typ.Number).Build()

	result := typ.NewUnion(
		typ.NewRecord().
			Field("channel", msgCh).
			Field("value", msgAlias).
			Field("ok", typ.Boolean).
			Build(),
		typ.NewRecord().
			Field("channel", timerCh).
			Field("value", timer).
			Field("ok", typ.Boolean).
			Build(),
	)

	synthesized := typ.NewRecord().
		Field("channel", msgCh).
		Field("value", typ.NewRecord().
			Field("_topic", typ.String).
			Field("topic", typ.Func().Param("s", msgAlias).Returns(typ.String).Build()).
			Build()).
		Field("ok", typ.True).
		Build()

	if !IsSubtype(synthesized, result) {
		t.Fatal("record literal should subtype union member carrying recursive Message alias")
	}
}

// Edge cases for empty unions and intersections

func TestEmptyUnionIsNever(t *testing.T) {
	// Empty union semantically equals Never which is subtype of any type
	if !IsSubtype(typ.Never, typ.String) {
		t.Error("Never should be subtype of string")
	}
	if !IsSubtype(typ.Never, typ.Number) {
		t.Error("Never should be subtype of number")
	}
}

func TestSingleMemberUnion(t *testing.T) {
	// Single member union should behave like the member itself
	union := typ.NewUnion(typ.String)

	if !IsSubtype(union, typ.String) {
		t.Error("single-member union should be subtype of member")
	}
	if !IsSubtype(typ.String, union) {
		t.Error("member should be subtype of single-member union")
	}
}

// Edge cases for intersection

func TestIntersectionSubtype(t *testing.T) {
	// A & B <: A and A & B <: B
	recA := typ.NewRecord().Field("a", typ.Number).Build()
	recB := typ.NewRecord().Field("b", typ.String).Build()
	inter := typ.NewIntersection(recA, recB)

	// Intersection should be subtype of each component
	if !IsSubtype(inter, recA) {
		t.Error("intersection should be subtype of first component")
	}
	if !IsSubtype(inter, recB) {
		t.Error("intersection should be subtype of second component")
	}
}

func TestSingleMemberIntersection(t *testing.T) {
	inter := typ.NewIntersection(typ.String)

	if !IsSubtype(inter, typ.String) {
		t.Error("single-member intersection should be subtype of member")
	}
}

// Depth limit edge cases

func TestDeeplyNestedRecordSubtype(t *testing.T) {
	// Create deeply nested record type
	inner := typ.Number
	for i := 0; i < 50; i++ {
		inner = typ.NewRecord().Field("nested", inner).Build()
	}

	// Should still handle reflexivity
	if !IsSubtype(inner, inner) {
		t.Error("deeply nested record should be subtype of itself")
	}
}

func TestDeeplyNestedOptional(t *testing.T) {
	// Create deeply nested optional
	inner := typ.String
	for i := 0; i < 20; i++ {
		inner = typ.NewOptional(inner)
	}

	// Base type should be subtype of nested optional
	if !IsSubtype(typ.String, inner) {
		t.Error("string should be subtype of deeply nested optional string")
	}
}

// Alias edge cases

func TestAliasChainSubtype(t *testing.T) {
	// A = B, B = C, C = string
	c := typ.NewAlias("C", typ.String)
	b := typ.NewAlias("B", c)
	a := typ.NewAlias("A", b)

	if !IsSubtype(a, typ.String) {
		t.Error("alias chain should resolve to base type")
	}
	if !IsSubtype(typ.String, a) {
		t.Error("base type should be subtype of alias chain")
	}
	if !IsSubtype(a, b) {
		t.Error("outer alias should be subtype of inner alias")
	}
}

// Function variance edge cases

func TestFunctionReturnCovariance(t *testing.T) {
	// (()) -> integer <: (()) -> number
	fnInt := typ.Func().Returns(typ.Integer).Build()
	fnNum := typ.Func().Returns(typ.Number).Build()

	if !IsSubtype(fnInt, fnNum) {
		t.Error("function with integer return should be subtype of function with number return")
	}
	if IsSubtype(fnNum, fnInt) {
		t.Error("function with number return should not be subtype of function with integer return")
	}
}

func TestFunctionParamContravariance(t *testing.T) {
	// (number) -> nil <: (integer) -> nil
	fnNum := typ.Func().Param("x", typ.Number).Returns(typ.Nil).Build()
	fnInt := typ.Func().Param("x", typ.Integer).Returns(typ.Nil).Build()

	if !IsSubtype(fnNum, fnInt) {
		t.Error("function accepting number should be subtype of function accepting integer")
	}
	if IsSubtype(fnInt, fnNum) {
		t.Error("function accepting integer should not be subtype of function accepting number")
	}
}

func TestFunctionArityMismatch(t *testing.T) {
	fn1 := typ.Func().Param("x", typ.Number).Returns(typ.Nil).Build()
	fn2 := typ.Func().Param("x", typ.Number).Param("y", typ.Number).Returns(typ.Nil).Build()

	if IsSubtype(fn1, fn2) {
		t.Error("function with fewer params should not be subtype")
	}
}

// Array and Map edge cases

func TestArrayCovariance(t *testing.T) {
	// Array<integer> <: Array<number>
	arrInt := typ.NewArray(typ.Integer)
	arrNum := typ.NewArray(typ.Number)

	if !IsSubtype(arrInt, arrNum) {
		t.Error("Array<integer> should be subtype of Array<number>")
	}
}

func TestMapInvariance(t *testing.T) {
	// Maps are invariant (both key and value) because they are mutable
	// Map<string, integer> is NOT a subtype of Map<string, number>
	mapInt := typ.NewMap(typ.String, typ.Integer)
	mapNum := typ.NewMap(typ.String, typ.Number)

	if IsSubtype(mapInt, mapNum) {
		t.Error("Map<string, integer> should NOT be subtype of Map<string, number> due to invariance")
	}
	if IsSubtype(mapNum, mapInt) {
		t.Error("Map<string, number> should NOT be subtype of Map<string, integer> due to invariance")
	}

	// Same maps should be subtypes
	mapInt2 := typ.NewMap(typ.String, typ.Integer)
	if !IsSubtype(mapInt, mapInt2) {
		t.Error("identical maps should be subtypes")
	}
}

func TestMapKeyInvariance(t *testing.T) {
	// Map<integer, string> </: Map<number, string> (keys are invariant)
	mapInt := typ.NewMap(typ.Integer, typ.String)
	mapNum := typ.NewMap(typ.Number, typ.String)

	// Keys are invariant: neither direction should hold for different key types
	if IsSubtype(mapInt, mapNum) {
		t.Error("Map<integer, string> should NOT be subtype of Map<number, string> due to key invariance")
	}
	if IsSubtype(mapNum, mapInt) {
		t.Error("Map<number, string> should NOT be subtype of Map<integer, string> due to key invariance")
	}
}

// Tuple edge cases

func TestTupleSubtype(t *testing.T) {
	tup1 := typ.NewTuple(typ.Integer, typ.String)
	tup2 := typ.NewTuple(typ.Number, typ.String)

	// Tuple with integer should be subtype of tuple with number in same position
	if !IsSubtype(tup1, tup2) {
		t.Error("Tuple<integer, string> should be subtype of Tuple<number, string>")
	}
}

func TestTupleLengthMismatch(t *testing.T) {
	tup1 := typ.NewTuple(typ.Number)
	tup2 := typ.NewTuple(typ.Number, typ.Number)

	if IsSubtype(tup1, tup2) {
		t.Error("shorter tuple should not be subtype of longer tuple")
	}
	if IsSubtype(tup2, tup1) {
		t.Error("longer tuple should not be subtype of shorter tuple")
	}
}

// Literal edge cases

func TestLiteralSubtypeLiteral(t *testing.T) {
	// Same literal values should be subtypes
	lit1 := typ.LiteralString("hello")
	lit2 := typ.LiteralString("hello")

	if !IsSubtype(lit1, lit2) {
		t.Error("identical string literals should be subtypes")
	}

	// Different literal values should not be subtypes
	lit3 := typ.LiteralString("world")
	if IsSubtype(lit1, lit3) {
		t.Error("different string literals should not be subtypes")
	}
}

func TestLiteralIntVsFloat(t *testing.T) {
	intLit := typ.LiteralInt(42)
	floatLit := typ.LiteralNumber(42.0)

	// Both should be subtypes of Number
	if !IsSubtype(intLit, typ.Number) {
		t.Error("int literal should be subtype of number")
	}
	if !IsSubtype(floatLit, typ.Number) {
		t.Error("float literal should be subtype of number")
	}
}

// Record structural subtyping edge cases

func TestRecordExtraFieldsSubtype(t *testing.T) {
	// {a: number, b: string} <: {a: number}
	rec1 := typ.NewRecord().Field("a", typ.Number).Field("b", typ.String).Build()
	rec2 := typ.NewRecord().Field("a", typ.Number).Build()

	if !IsSubtype(rec1, rec2) {
		t.Error("record with extra fields should be subtype")
	}
	if IsSubtype(rec2, rec1) {
		t.Error("record with fewer fields should not be subtype of one with more required")
	}
}

func TestRecordOptionalFields(t *testing.T) {
	// {a: number} <: {a?: number}
	rec1 := typ.NewRecord().Field("a", typ.Number).Build()
	rec2 := typ.NewRecord().OptField("a", typ.Number).Build()

	if !IsSubtype(rec1, rec2) {
		t.Error("record with required field should be subtype of one with optional field")
	}
}

func TestRecordFieldTypeSubtype(t *testing.T) {
	// {value: integer} <: {value: number}
	rec1 := typ.NewRecord().Field("value", typ.Integer).Build()
	rec2 := typ.NewRecord().Field("value", typ.Number).Build()

	if !IsSubtype(rec1, rec2) {
		t.Error("record with subtype field should be subtype")
	}
}

// Record to Map subtyping tests

func TestRecordToMap(t *testing.T) {
	// {name: string, age: number} <: Map<string, string|number>
	rec := typ.NewRecord().Field("name", typ.String).Field("age", typ.Number).Build()
	mapType := typ.NewMap(typ.String, typ.NewUnion(typ.String, typ.Number))

	if !IsSubtype(rec, mapType) {
		t.Error("record should be subtype of compatible map")
	}
}

func TestRecursiveRecordToMap(t *testing.T) {
	rec := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typ.NewRecord().Field("child", self).Build()
	})
	mapType := typ.NewMap(typ.String, rec)

	if !IsSubtype(rec, mapType) {
		t.Error("recursive record should be subtype of compatible recursive map")
	}
}

func TestRecordToMapIncompatibleKey(t *testing.T) {
	rec := typ.NewRecord().Field("name", typ.String).Build()
	mapType := typ.NewMap(typ.Number, typ.String)

	if IsSubtype(rec, mapType) {
		t.Error("record with string keys should not be subtype of map with number keys")
	}
}

func TestRecordToMapIncompatibleValue(t *testing.T) {
	rec := typ.NewRecord().Field("name", typ.Number).Build()
	mapType := typ.NewMap(typ.String, typ.String)

	if IsSubtype(rec, mapType) {
		t.Error("record with number field should not be subtype of map with string values")
	}
}

func TestRecordWithMapComponentToMap(t *testing.T) {
	rec := typ.NewRecord().Field("name", typ.String).MapComponent(typ.String, typ.Number).Build()
	mapType := typ.NewMap(typ.String, typ.NewUnion(typ.String, typ.Number))

	if !IsSubtype(rec, mapType) {
		t.Error("record with map component should be subtype of compatible map")
	}
}

func TestRecordWithMapComponentIncompatible(t *testing.T) {
	rec := typ.NewRecord().Field("name", typ.String).MapComponent(typ.Number, typ.String).Build()
	mapType := typ.NewMap(typ.String, typ.String)

	if IsSubtype(rec, mapType) {
		t.Error("record with incompatible map component should not be subtype")
	}
}

func TestEmptyRecordToMap(t *testing.T) {
	rec := typ.NewRecord().Build()
	mapType := typ.NewMap(typ.String, typ.Number)

	if !IsSubtype(rec, mapType) {
		t.Error("empty record should be subtype of any map")
	}
}

func TestEmptyRecordToArray(t *testing.T) {
	rec := typ.NewRecord().Build()
	arrType := typ.NewArray(typ.Number)

	if !IsSubtype(rec, arrType) {
		t.Error("empty record should be subtype of any array")
	}
}

func TestTableMarkerAcceptsMap(t *testing.T) {
	sub := typ.NewMap(typ.String, typ.Any)
	super := typ.NewInterface("table", nil)

	if !IsSubtype(sub, super) {
		t.Error("map should be subtype of table marker")
	}
}

func TestTableMarkerAcceptsArray(t *testing.T) {
	sub := typ.NewArray(typ.String)
	super := typ.NewInterface("table", nil)

	if !IsSubtype(sub, super) {
		t.Error("array should be subtype of table marker")
	}
}

func TestTableMarkerAcceptsAny(t *testing.T) {
	sub := typ.Any
	super := typ.NewInterface("table", nil)

	if !IsSubtype(sub, super) {
		t.Error("any should be subtype of table marker")
	}
}

func TestMapIsNotSubtypeOfEmptyRecord(t *testing.T) {
	sub := typ.NewMap(typ.String, typ.Any)
	super := typ.NewRecord().Build()

	if IsSubtype(sub, super) {
		t.Error("map should not be subtype of empty record")
	}
}

func TestEmptyRecordToOptionalOnlyRecord(t *testing.T) {
	rec := typ.NewRecord().Build()
	super := typ.NewRecord().
		OptField("tags", typ.NewArray(typ.String)).
		OptField("allowed_ids", typ.NewArray(typ.String)).
		OptField("count", typ.Number).
		Build()

	if !IsSubtype(rec, super) {
		t.Error("empty record should be subtype of record with only optional fields")
	}
}

func TestEmptyRecordToRequiredAnyFieldRecord(t *testing.T) {
	rec := typ.NewRecord().Build()
	super := typ.NewRecord().
		Field("context_merger", typ.Any).
		Build()

	if !IsSubtype(rec, super) {
		t.Error("empty record should be subtype when required field admits nil via any")
	}
}

func TestEmptyRecordToRequiredUnknownFieldRecord(t *testing.T) {
	rec := typ.NewRecord().Build()
	super := typ.NewRecord().
		Field("dynamic", typ.Unknown).
		Build()

	if !IsSubtype(rec, super) {
		t.Error("empty record should be subtype when required field admits nil via unknown")
	}
}

func TestEmptyRecordToRequiredNilUnionFieldRecord(t *testing.T) {
	rec := typ.NewRecord().Build()
	super := typ.NewRecord().
		Field("payload", typ.NewUnion(typ.String, typ.Nil)).
		Build()

	if !IsSubtype(rec, super) {
		t.Error("empty record should be subtype when required field is a union containing nil")
	}
}

func TestEmptyRecordNotSubtypeOfRequiredRecord(t *testing.T) {
	rec := typ.NewRecord().Build()
	super := typ.NewRecord().Field("id", typ.String).Build()

	if IsSubtype(rec, super) {
		t.Error("empty record should not be subtype of record with required fields")
	}
}

// Array to Map subtyping tests

func TestArrayToMap(t *testing.T) {
	arr := typ.NewArray(typ.String)
	mapType := typ.NewMap(typ.Integer, typ.String)

	if !IsSubtype(arr, mapType) {
		t.Error("array should be subtype of map with integer keys and compatible values")
	}
}

func TestArrayToMapIncompatibleKey(t *testing.T) {
	arr := typ.NewArray(typ.String)
	mapType := typ.NewMap(typ.String, typ.String)

	if IsSubtype(arr, mapType) {
		t.Error("array should not be subtype of map with string keys")
	}
}

func TestArrayToMapIncompatibleValue(t *testing.T) {
	arr := typ.NewArray(typ.String)
	mapType := typ.NewMap(typ.Integer, typ.Number)

	if IsSubtype(arr, mapType) {
		t.Error("array of strings should not be subtype of map with number values")
	}
}

// Tuple to Array subtyping tests

func TestTupleToArray(t *testing.T) {
	tup := typ.NewTuple(typ.Integer, typ.Integer, typ.Integer)
	arr := typ.NewArray(typ.Number)

	if !IsSubtype(tup, arr) {
		t.Error("tuple of integers should be subtype of number array")
	}
}

func TestTupleToArrayIncompatible(t *testing.T) {
	tup := typ.NewTuple(typ.String, typ.Number)
	arr := typ.NewArray(typ.Number)

	if IsSubtype(tup, arr) {
		t.Error("tuple with string should not be subtype of number array")
	}
}

func TestEmptyTupleToArray(t *testing.T) {
	tup := typ.NewTuple()
	arr := typ.NewArray(typ.Number)

	if !IsSubtype(tup, arr) {
		t.Error("empty tuple should be subtype of any array")
	}
}

// Tuple to Map subtyping tests

func TestTupleToMap(t *testing.T) {
	tup := typ.NewTuple(typ.String, typ.String)
	mapType := typ.NewMap(typ.Integer, typ.String)

	if !IsSubtype(tup, mapType) {
		t.Error("tuple should be subtype of map with integer keys")
	}
}

func TestTupleToMapIncompatibleKey(t *testing.T) {
	tup := typ.NewTuple(typ.String, typ.String)
	mapType := typ.NewMap(typ.String, typ.String)

	if IsSubtype(tup, mapType) {
		t.Error("tuple should not be subtype of map with string keys")
	}
}

func TestTupleToMapIncompatibleValue(t *testing.T) {
	tup := typ.NewTuple(typ.String, typ.Number)
	mapType := typ.NewMap(typ.Integer, typ.String)

	if IsSubtype(tup, mapType) {
		t.Error("tuple with number should not be subtype of map with string values")
	}
}

// Record to Interface subtyping tests

func TestRecordToInterface(t *testing.T) {
	rec := typ.NewRecord().
		Field("getName", typ.Func().Returns(typ.String).Build()).
		Field("getValue", typ.Func().Returns(typ.Number).Build()).
		Build()

	iface := typ.NewInterface("Named", []typ.Method{
		{Name: "getName", Type: typ.Func().Returns(typ.String).Build()},
	})

	if !IsSubtype(rec, iface) {
		t.Error("record with method should be subtype of interface")
	}
}

func TestRecordToInterfaceMissingMethod(t *testing.T) {
	rec := typ.NewRecord().
		Field("getValue", typ.Func().Returns(typ.Number).Build()).
		Build()

	iface := typ.NewInterface("Named", []typ.Method{
		{Name: "getName", Type: typ.Func().Returns(typ.String).Build()},
	})

	if IsSubtype(rec, iface) {
		t.Error("record missing method should not be subtype of interface")
	}
}

func TestRecordToInterfaceIncompatibleMethod(t *testing.T) {
	rec := typ.NewRecord().
		Field("getName", typ.Func().Returns(typ.Number).Build()).
		Build()

	iface := typ.NewInterface("Named", []typ.Method{
		{Name: "getName", Type: typ.Func().Returns(typ.String).Build()},
	})

	if IsSubtype(rec, iface) {
		t.Error("record with incompatible method should not be subtype of interface")
	}
}

func TestRecordToInterfaceWithSelf(t *testing.T) {
	// Self type in method signatures substituted during comparison
	rec := typ.NewRecord().
		Field("clone", typ.Func().Returns(typ.Any).Build()).
		Build()

	iface := typ.NewInterface("Cloneable", []typ.Method{
		{Name: "clone", Type: typ.Func().Returns(typ.Any).Build()},
	})

	if !IsSubtype(rec, iface) {
		t.Error("record with method should be subtype of interface")
	}
}

// Interface nil handling via the checkInterface method is already tested
// through the main check function's nil handling.

// Marker interface tests

func TestMarkerInterfaceSameName(t *testing.T) {
	i1 := typ.NewInterface("Channel", nil)
	i2 := typ.NewInterface("Channel", nil)

	if !IsSubtype(i1, i2) {
		t.Error("marker interfaces with same name should be subtypes")
	}
}

func TestMarkerInterfaceDifferentName(t *testing.T) {
	i1 := typ.NewInterface("ChannelA", nil)
	i2 := typ.NewInterface("ChannelB", nil)

	if IsSubtype(i1, i2) {
		t.Error("marker interfaces with different names should not be subtypes")
	}
}

// TypeParam constraint tests

func TestTypeParamWithConstraint(t *testing.T) {
	tp := typ.NewTypeParam("T", typ.Number)

	if !IsSubtype(tp, typ.Number) {
		t.Error("type param with number constraint should be subtype of number")
	}
}

func TestTypeParamNoConstraint(t *testing.T) {
	tp := typ.NewTypeParam("T", nil)

	if !IsSubtype(tp, typ.Any) {
		t.Error("unconstrained type param should be subtype of any")
	}
}

func TestTypeToTypeParamWithConstraint(t *testing.T) {
	tp := typ.NewTypeParam("T", typ.Number)

	if !IsSubtype(typ.Integer, tp) {
		t.Error("integer should be subtype of type param with number constraint")
	}
}

func TestTypeToTypeParamNoConstraint(t *testing.T) {
	tp := typ.NewTypeParam("T", nil)

	if !IsSubtype(typ.String, tp) {
		t.Error("any type should be subtype of unconstrained type param")
	}
}

// Meta type tests

func TestMetaTypeSubtype(t *testing.T) {
	m1 := typ.NewMeta(typ.String)
	m2 := typ.NewMeta(typ.String)

	if !IsSubtype(m1, m2) {
		t.Error("meta types with same inner should be subtypes")
	}
}

func TestMetaTypeDifferent(t *testing.T) {
	m1 := typ.NewMeta(typ.String)
	m2 := typ.NewMeta(typ.Number)

	if IsSubtype(m1, m2) {
		t.Error("meta types with different inner should not be subtypes")
	}
}

// Ref and Alias matching tests

func TestRefMatchesAlias(t *testing.T) {
	ref := &typ.Ref{Name: "MyType", Module: ""}
	alias := typ.NewAlias("MyType", typ.String)

	if !IsSubtype(ref, alias) {
		t.Error("local ref should match alias with same name")
	}
	if !IsSubtype(alias, ref) {
		t.Error("alias should match local ref with same name")
	}
}

func TestRefMatchesRef(t *testing.T) {
	ref1 := &typ.Ref{Name: "MyType", Module: ""}
	ref2 := &typ.Ref{Name: "MyType", Module: ""}

	if !IsSubtype(ref1, ref2) {
		t.Error("local refs with same name should match")
	}
}

func TestRefWithModuleNoMatch(_ *testing.T) {
	ref1 := &typ.Ref{Name: "MyType", Module: "mod1"}
	ref2 := &typ.Ref{Name: "MyType", Module: ""}

	// Module-qualified refs don't match local refs via the name shortcut
	// This test just verifies neither crashes
	_ = IsSubtype(ref1, ref2)
	_ = IsSubtype(ref2, ref1)
}

// Function edge cases

func TestFunctionWithMoreReturns(t *testing.T) {
	f1 := typ.Func().Returns(typ.Number, typ.String).Build()
	f2 := typ.Func().Returns(typ.Number).Build()

	if !IsSubtype(f1, f2) {
		t.Error("function with more returns should be subtype")
	}
}

func TestFunctionVariadicVsNonVariadic(t *testing.T) {
	f1 := typ.Func().Param("x", typ.Number).Variadic(typ.Number).Returns(typ.Nil).Build()
	f2 := typ.Func().Param("x", typ.Number).Param("y", typ.Number).Returns(typ.Nil).Build()

	if !IsSubtype(f1, f2) {
		t.Error("variadic function should be subtype of function with more params if variadic covers them")
	}
}

func TestFunctionSubRequiresMoreThanSuperProvides(t *testing.T) {
	// sub requires 2 params, super only provides 1
	f1 := typ.Func().Param("x", typ.Number).Param("y", typ.Number).Returns(typ.Nil).Build()
	f2 := typ.Func().Param("x", typ.Number).Returns(typ.Nil).Build()

	if IsSubtype(f1, f2) {
		t.Error("function requiring more params should not be subtype")
	}
}

// Record map component tests

func TestRecordMapComponentSubtype(t *testing.T) {
	sub := typ.NewRecord().
		Field("name", typ.String).
		MapComponent(typ.String, typ.Number).
		Build()

	super := typ.NewRecord().
		Field("name", typ.String).
		MapComponent(typ.String, typ.Number).
		Build()

	if !IsSubtype(sub, super) {
		t.Error("records with same map components should be subtypes")
	}
}

func TestRecordMapComponentMissing(t *testing.T) {
	sub := typ.NewRecord().Field("name", typ.String).Build()
	super := typ.NewRecord().
		Field("name", typ.String).
		MapComponent(typ.String, typ.Number).
		Build()

	if IsSubtype(sub, super) {
		t.Error("record without map component should not be subtype of record with map component")
	}
}

func TestRecordMapComponentKeyMismatch(t *testing.T) {
	sub := typ.NewRecord().
		Field("name", typ.String).
		MapComponent(typ.Number, typ.String).
		Build()

	super := typ.NewRecord().
		Field("name", typ.String).
		MapComponent(typ.String, typ.String).
		Build()

	if IsSubtype(sub, super) {
		t.Error("records with different map component keys should not be subtypes")
	}
}

func TestRecordMapComponentValueMismatch(t *testing.T) {
	sub := typ.NewRecord().
		Field("name", typ.String).
		MapComponent(typ.String, typ.Number).
		Build()

	super := typ.NewRecord().
		Field("name", typ.String).
		MapComponent(typ.String, typ.String).
		Build()

	if IsSubtype(sub, super) {
		t.Error("records with different map component values should not be subtypes")
	}
}

// Intersection super tests

func TestIntersectionSuperAllMembers(t *testing.T) {
	rec := typ.NewRecord().Field("x", typ.Number).Field("y", typ.String).Build()
	i1 := typ.NewRecord().Field("x", typ.Number).Build()
	i2 := typ.NewRecord().Field("y", typ.String).Build()
	inter := typ.NewIntersection(i1, i2)

	if !IsSubtype(rec, inter) {
		t.Error("record should be subtype of intersection of compatible records")
	}
}

func TestIntersectionSuperNotAllMembers(t *testing.T) {
	rec := typ.NewRecord().Field("x", typ.Number).Build()
	i1 := typ.NewRecord().Field("x", typ.Number).Build()
	i2 := typ.NewRecord().Field("y", typ.String).Build()
	inter := typ.NewIntersection(i1, i2)

	if IsSubtype(rec, inter) {
		t.Error("record missing y should not be subtype of intersection")
	}
}

// Optional subtype tests

func TestOptionalToOptional(t *testing.T) {
	opt1 := typ.NewOptional(typ.Integer)
	opt2 := typ.NewOptional(typ.Number)

	if !IsSubtype(opt1, opt2) {
		t.Error("integer? should be subtype of number?")
	}
}

func TestOptionalSubOfNonOptional(t *testing.T) {
	opt := typ.NewOptional(typ.Number)
	union := typ.NewUnion(typ.Number, typ.Nil)

	if !IsSubtype(opt, union) {
		t.Error("number? should be subtype of number|nil")
	}
}

func TestOptionalNotSubtypeOfUnionWithoutNil(t *testing.T) {
	opt := typ.NewOptional(typ.Number)
	union := typ.NewUnion(typ.Number, typ.String)

	if IsSubtype(opt, union) {
		t.Error("number? should not be subtype of number|string")
	}
}

// Widening tests via record subtyping

func TestRecordMutableFieldWidening_IntegerToNumber(t *testing.T) {
	// {x: integer} should be subtype of {x: number} via widening
	sub := typ.NewRecord().Field("x", typ.Integer).Build()
	super := typ.NewRecord().Field("x", typ.Number).Build()

	if !IsSubtype(sub, super) {
		t.Error("record with integer field should widen to record with number field")
	}
}

func TestRecordMutableFieldWidening_LiteralToBase(t *testing.T) {
	// {x: "hello"} should be subtype of {x: string} via widening
	sub := typ.NewRecord().Field("x", typ.LiteralString("hello")).Build()
	super := typ.NewRecord().Field("x", typ.String).Build()

	if !IsSubtype(sub, super) {
		t.Error("record with literal field should widen to record with base type field")
	}
}

func TestRecordMutableFieldWidening_LiteralIntToNumber(t *testing.T) {
	sub := typ.NewRecord().Field("x", typ.LiteralInt(42)).Build()
	super := typ.NewRecord().Field("x", typ.Number).Build()

	if !IsSubtype(sub, super) {
		t.Error("record with literal int field should widen to record with number field")
	}
}

func TestRecordMutableFieldWidening_LiteralUnionToInteger(t *testing.T) {
	sub := typ.NewRecord().
		Field("x", typ.NewUnion(typ.LiteralInt(0), typ.LiteralInt(8000))).
		Build()
	super := typ.NewRecord().Field("x", typ.Integer).Build()

	if !IsSubtype(sub, super) {
		t.Error("record with integer literal union field should widen to integer field")
	}
}

func TestRecordMutableFieldWidening_LiteralUnionToString(t *testing.T) {
	sub := typ.NewRecord().
		Field("name", typ.NewUnion(typ.LiteralString(""), typ.LiteralString("alpha"))).
		Build()
	super := typ.NewRecord().Field("name", typ.String).Build()

	if !IsSubtype(sub, super) {
		t.Error("record with string literal union field should widen to string field")
	}
}

func TestRecordMutableFieldWidening_LiteralBool(t *testing.T) {
	sub := typ.NewRecord().Field("x", typ.True).Build()
	super := typ.NewRecord().Field("x", typ.Boolean).Build()

	if !IsSubtype(sub, super) {
		t.Error("record with literal bool field should widen to record with boolean field")
	}
}

func TestRecordMutableFieldWidening_NilToOptional(t *testing.T) {
	sub := typ.NewRecord().Field("x", typ.Nil).Build()
	super := typ.NewRecord().Field("x", typ.NewOptional(typ.String)).Build()

	if !IsSubtype(sub, super) {
		t.Error("record with nil field should widen to record with optional field")
	}
}

func TestRecordMutableFieldWidening_NilToUnionWithNil(t *testing.T) {
	sub := typ.NewRecord().Field("x", typ.Nil).Build()
	super := typ.NewRecord().Field("x", typ.NewUnion(typ.String, typ.Nil)).Build()

	if !IsSubtype(sub, super) {
		t.Error("record with nil field should widen to record with union containing nil")
	}
}

func TestRecordMutableFieldWidening_ToAny(t *testing.T) {
	sub := typ.NewRecord().Field("x", typ.String).Build()
	super := typ.NewRecord().Field("x", typ.Any).Build()

	if !IsSubtype(sub, super) {
		t.Error("any field should accept any type via widening")
	}
}

func TestRecordMutableFieldWidening_ToOptional(t *testing.T) {
	sub := typ.NewRecord().Field("x", typ.Integer).Build()
	super := typ.NewRecord().Field("x", typ.NewOptional(typ.Number)).Build()

	if !IsSubtype(sub, super) {
		t.Error("integer should widen to number?")
	}
}

func TestRecordMutableFieldWidening_NestedRecord(t *testing.T) {
	innerSub := typ.NewRecord().Field("y", typ.LiteralInt(1)).Build()
	innerSuper := typ.NewRecord().Field("y", typ.Number).Build()

	sub := typ.NewRecord().Field("x", innerSub).Build()
	super := typ.NewRecord().Field("x", innerSuper).Build()

	if !IsSubtype(sub, super) {
		t.Error("nested record with literal should widen")
	}
}

func TestRecordMutableFieldWidening_Function(t *testing.T) {
	fnSub := typ.Func().Param("x", typ.Number).Returns(typ.LiteralInt(1)).Build()
	fnSuper := typ.Func().Param("x", typ.Number).Returns(typ.Integer).Build()

	sub := typ.NewRecord().Field("f", fnSub).Build()
	super := typ.NewRecord().Field("f", fnSuper).Build()

	if !IsSubtype(sub, super) {
		t.Error("function return type should widen")
	}
}

func TestRecordMutableFieldWidening_FunctionParamMismatch(_ *testing.T) {
	fnSub := typ.Func().Param("x", typ.Integer).Returns(typ.Number).Build()
	fnSuper := typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()

	sub := typ.NewRecord().Field("f", fnSub).Build()
	super := typ.NewRecord().Field("f", fnSuper).Build()

	// Function params are contravariant, so this might fail widening
	// The widening requires params to be equivalent
	// Just verify no panic
	_ = IsSubtype(sub, super)
}

func TestRecordMutableFieldWidening_LiteralNumberToNumber(t *testing.T) {
	sub := typ.NewRecord().Field("x", typ.LiteralNumber(3.14)).Build()
	super := typ.NewRecord().Field("x", typ.Number).Build()

	if !IsSubtype(sub, super) {
		t.Error("literal number should widen to number")
	}
}

func TestRecordMutableFieldWidening_LiteralToOptionalBase(t *testing.T) {
	sub := typ.NewRecord().Field("x", typ.LiteralString("test")).Build()
	super := typ.NewRecord().Field("x", typ.NewOptional(typ.String)).Build()

	if !IsSubtype(sub, super) {
		t.Error("literal string should widen to optional string")
	}
}

// Instantiated edge cases

func TestInstantiatedWithBody(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	body := typ.NewRecord().Field("value", param).Build()
	g := typ.NewGeneric("Box", []*typ.TypeParam{param}, body)

	inst1 := typ.Instantiate(g, typ.String)
	inst2 := typ.Instantiate(g, typ.String)

	if !IsSubtype(inst1, inst2) {
		t.Error("instantiated with same args and body should be subtypes")
	}
}

func TestInstantiatedCrossKind(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	body := typ.NewRecord().Field("value", param).Build()
	g := typ.NewGeneric("Box", []*typ.TypeParam{param}, body)

	inst := typ.Instantiate(g, typ.String)
	rec := typ.NewRecord().Field("value", typ.String).Build()

	// Instantiated should expand to body for comparison
	if !IsSubtype(inst, rec) {
		t.Error("instantiated Box<string> should expand to {value: string}")
	}
}

// Depth limit tests

func TestDeepRecursionDepthLimit(t *testing.T) {
	// Create extremely deep nesting to test depth limit
	inner := typ.Number
	for i := 0; i < 200; i++ {
		inner = typ.NewOptional(inner)
	}

	// Should not stack overflow, should return false due to depth limit
	result := IsSubtype(typ.String, inner)
	if result {
		t.Error("string should not be subtype of deeply nested optional")
	}
}

// Nil interface handling is covered by TestCheckNilTypes

// Additional function widening tests

func TestRecordMutableFieldWidening_FunctionWithVariadic(t *testing.T) {
	fnSub := typ.Func().Param("x", typ.Number).Variadic(typ.String).Returns(typ.Integer).Build()
	fnSuper := typ.Func().Param("x", typ.Number).Variadic(typ.String).Returns(typ.Number).Build()

	sub := typ.NewRecord().Field("f", fnSub).Build()
	super := typ.NewRecord().Field("f", fnSuper).Build()

	if !IsSubtype(sub, super) {
		t.Error("function with same variadic should widen")
	}
}

func TestRecordMutableFieldWidening_FunctionDifferentVariadic(_ *testing.T) {
	fnSub := typ.Func().Param("x", typ.Number).Variadic(typ.Number).Returns(typ.Number).Build()
	fnSuper := typ.Func().Param("x", typ.Number).Variadic(typ.String).Returns(typ.Number).Build()

	sub := typ.NewRecord().Field("f", fnSub).Build()
	super := typ.NewRecord().Field("f", fnSuper).Build()

	// Different variadics - just verify no panic
	_ = IsSubtype(sub, super)
}

func TestRecordMutableFieldWidening_FunctionMissingVariadic(_ *testing.T) {
	fnSub := typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()
	fnSuper := typ.Func().Param("x", typ.Number).Variadic(typ.String).Returns(typ.Number).Build()

	sub := typ.NewRecord().Field("f", fnSub).Build()
	super := typ.NewRecord().Field("f", fnSuper).Build()

	// Missing variadic - just verify no panic
	_ = IsSubtype(sub, super)
}

func TestRecordMutableFieldWidening_FunctionNilParams(t *testing.T) {
	fnSub := typ.Func().Returns(typ.Number).Build()
	fnSuper := typ.Func().Returns(typ.Number).Build()

	sub := typ.NewRecord().Field("f", fnSub).Build()
	super := typ.NewRecord().Field("f", fnSuper).Build()

	if !IsSubtype(sub, super) {
		t.Error("functions with no params should be subtypes")
	}
}

func TestRecordMutableFieldWidening_FunctionDifferentParamCount(_ *testing.T) {
	fnSub := typ.Func().Param("x", typ.Number).Param("y", typ.String).Returns(typ.Number).Build()
	fnSuper := typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()

	sub := typ.NewRecord().Field("f", fnSub).Build()
	super := typ.NewRecord().Field("f", fnSuper).Build()

	// Different param counts - just verify no panic
	_ = IsSubtype(sub, super)
}

func TestRecordMutableFieldWidening_FunctionDifferentOptional(_ *testing.T) {
	fnSub := typ.Func().OptParam("x", typ.Number).Returns(typ.Number).Build()
	fnSuper := typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()

	sub := typ.NewRecord().Field("f", fnSub).Build()
	super := typ.NewRecord().Field("f", fnSuper).Build()

	// Different optional flags - just verify no panic
	_ = IsSubtype(sub, super)
}

func TestRecordMutableFieldWidening_NestedRecordMissingField(_ *testing.T) {
	innerSub := typ.NewRecord().Field("y", typ.Number).Build()
	innerSuper := typ.NewRecord().Field("y", typ.Number).Field("z", typ.String).Build()

	sub := typ.NewRecord().Field("x", innerSub).Build()
	super := typ.NewRecord().Field("x", innerSuper).Build()

	// Inner record missing required field - just verify no panic
	_ = IsSubtype(sub, super)
}

// Additional instantiated tests

func TestInstantiatedDifferentArgCount(t *testing.T) {
	param1 := typ.NewTypeParam("T", nil)
	param2 := typ.NewTypeParam("U", nil)
	g := typ.NewGeneric("Pair", []*typ.TypeParam{param1, param2}, nil)

	inst1 := typ.Instantiate(g, typ.String)             // Only 1 arg
	inst2 := typ.Instantiate(g, typ.String, typ.Number) // 2 args

	if IsSubtype(inst1, inst2) {
		t.Error("instantiated with different arg counts should not be subtypes")
	}
}

// Additional intersection distribution tests

func TestNormalizeIntersectionDeepDistribution(t *testing.T) {
	// Test that distribution respects depth limit
	union := typ.NewUnion(typ.String, typ.Number)

	// Create nested intersection that would require distribution
	result := NormalizeIntersection(union, union)
	if result == nil {
		t.Error("intersection distribution should produce a result")
	}
}
