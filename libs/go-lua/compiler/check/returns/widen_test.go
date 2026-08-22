package returns

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

func TestWidenFacts_DoesNotOverrideReturnSummariesWithNarrowReturns(t *testing.T) {
	prev := api.Facts{
		FunctionFacts: api.FunctionFacts{
			1: {Summary: []typ.Type{typ.Integer}},
		},
		ReturnSummaries: api.ReturnSummaries{
			1: []typ.Type{typ.Integer},
		},
	}
	next := api.Facts{
		FunctionFacts: api.FunctionFacts{
			1: {Narrow: []typ.Type{typ.Nil}},
		},
		NarrowReturns: api.NarrowReturnSummaries{
			1: []typ.Type{typ.Nil},
		},
	}

	merged := WidenFacts(prev, next)
	got := merged.ReturnSummaries[1]
	if len(got) != 1 || !typ.TypeEquals(got[0], typ.Integer) {
		t.Fatalf("expected ReturnSummaries[1]=integer, got %v", got)
	}
}

func TestWidenFacts_ElidesOptionalFromNarrowReturns(t *testing.T) {
	prev := api.Facts{
		FunctionFacts: api.FunctionFacts{
			1: {Summary: []typ.Type{typ.NewOptional(typ.Integer)}},
		},
		ReturnSummaries: api.ReturnSummaries{
			1: []typ.Type{typ.NewOptional(typ.Integer)},
		},
	}
	next := api.Facts{
		FunctionFacts: api.FunctionFacts{
			1: {Narrow: []typ.Type{typ.Integer}},
		},
		NarrowReturns: api.NarrowReturnSummaries{
			1: []typ.Type{typ.Integer},
		},
	}

	merged := WidenFacts(prev, next)
	got := merged.ReturnSummaries[1]
	if len(got) != 1 || !typ.TypeEquals(got[0], typ.Integer) {
		t.Fatalf("expected ReturnSummaries[1]=integer, got %v", got)
	}
}

func TestWidenReturnSummaries_RefinesOptionalForFirstOrderTypes(t *testing.T) {
	prev := api.ReturnSummaries{
		1: []typ.Type{typ.NewOptional(typ.Integer)},
	}
	next := api.ReturnSummaries{
		1: []typ.Type{typ.Integer},
	}

	merged := WidenReturnSummaries(prev, next)
	got := merged[1]
	if len(got) != 1 || !typ.TypeEquals(got[0], typ.Integer) {
		t.Fatalf("expected integer after first-order refinement, got %v", got)
	}
}

func TestWidenReturnSummaries_UsesMonotoneJoinForHigherOrderReturns(t *testing.T) {
	nestedUnknown := typ.NewRecord().
		Field("next", typ.Func().Returns(typ.Unknown).Build()).
		Build()
	nestedString := typ.NewRecord().
		Field("next", typ.Func().Returns(typ.String).Build()).
		Build()

	base := typ.NewRecord().
		Field("build", typ.Func().Returns(nestedUnknown).Build()).
		Build()
	refined := typ.NewRecord().
		Field("build", typ.Func().Returns(nestedString).Build()).
		Build()

	prev := api.ReturnSummaries{
		1: []typ.Type{base},
	}
	next := api.ReturnSummaries{
		1: []typ.Type{refined},
	}

	merged := WidenReturnSummaries(prev, next)
	got := merged[1]
	if len(got) != 1 || !typ.TypeEquals(got[0], base) {
		t.Fatalf("expected stable upper bound for higher-order return, got %v", got)
	}
}

func TestWidenReturnSummaries_InterfaceMethodsDoNotBlockOptionalElision(t *testing.T) {
	dbType := typ.NewInterface("sql.DB", []typ.Method{
		{
			Name: "release",
			Type: typ.Func().
				Param("self", typ.Self).
				Returns(typ.Boolean, typ.NewOptional(typ.LuaError)).
				Build(),
		},
	})

	prev := api.ReturnSummaries{
		1: []typ.Type{typ.NewOptional(dbType)},
	}
	next := api.ReturnSummaries{
		1: []typ.Type{dbType},
	}

	merged := WidenReturnSummaries(prev, next)
	got := merged[1]
	if len(got) != 1 || !typ.TypeEquals(got[0], dbType) {
		t.Fatalf("expected optional elision for interface return, got %v", got)
	}
}

func TestMergeFunctionReturnsIfSameShape_GenericFunctions(t *testing.T) {
	prev := typ.Func().
		TypeParam("T", nil).
		Returns(typ.String).
		Build()
	next := typ.Func().
		TypeParam("T", nil).
		Returns(typ.Integer).
		Build()

	mergedType, ok := mergeFunctionReturnsIfSameShape(prev, next)
	if !ok {
		t.Fatal("expected generic same-shape functions to merge")
	}
	merged, ok := mergedType.(*typ.Function)
	if !ok {
		t.Fatalf("expected merged function type, got %T", mergedType)
	}
	if len(merged.TypeParams) != 1 || merged.TypeParams[0] == nil || merged.TypeParams[0].Name != "T" {
		t.Fatalf("expected merged generic type parameter T, got %+v", merged.TypeParams)
	}
	if len(merged.Returns) != 1 {
		t.Fatalf("expected one return, got %d", len(merged.Returns))
	}
	want := typ.NewUnion(typ.String, typ.Integer)
	if !typ.TypeEquals(merged.Returns[0], want) {
		t.Fatalf("expected merged return %v, got %v", want, merged.Returns[0])
	}
}

func TestMergeFunctionReturnsIfSameShape_GenericTypeParamsMustMatch(t *testing.T) {
	prev := typ.Func().
		TypeParam("T", nil).
		Returns(typ.String).
		Build()
	next := typ.Func().
		TypeParam("U", nil).
		Returns(typ.Integer).
		Build()

	_, ok := mergeFunctionReturnsIfSameShape(prev, next)
	if ok {
		t.Fatal("expected mismatched generic params not to merge")
	}
}

func TestMergeFuncTypes_DoesNotRegressToNarrowerNilReturn(t *testing.T) {
	prev := typ.Func().
		Returns(typ.NewOptional(typ.Integer)).
		Build()
	next := typ.Func().
		Returns(typ.Nil).
		Build()

	merged := MergeFunctionFactType(prev, next)
	fn, ok := merged.(*typ.Function)
	if !ok || len(fn.Returns) != 1 {
		t.Fatalf("expected merged function return, got %T", merged)
	}
	if !typ.TypeEquals(fn.Returns[0], typ.NewOptional(typ.Integer)) {
		t.Fatalf("expected integer? return after merge, got %v", fn.Returns[0])
	}
}

func TestMergeFunctionReturnsIfSameShape_NormalizesLeakedTypeParams(t *testing.T) {
	prev := typ.Func().
		Returns(typ.NewTypeParam("T", nil)).
		Build()
	next := typ.Func().
		Returns(typ.Integer).
		Build()

	mergedType, ok := mergeFunctionReturnsIfSameShape(prev, next)
	if !ok {
		t.Fatal("expected same-shape functions to merge")
	}
	merged, ok := mergedType.(*typ.Function)
	if !ok || len(merged.Returns) != 1 {
		t.Fatalf("expected merged function return, got %T", mergedType)
	}
	if !typ.TypeEquals(merged.Returns[0], typ.Integer) {
		t.Fatalf("expected leaked type param to normalize to integer, got %v", merged.Returns[0])
	}
}

func TestMergeFuncTypes_PrefersWiderSupertypeOnSubtypeRelation(t *testing.T) {
	merged := MergeFunctionFactType(typ.Integer, typ.Number)
	if !typ.TypeEquals(merged, typ.Number) {
		t.Fatalf("expected wider supertype number, got %v", merged)
	}

	merged = MergeFunctionFactType(typ.Number, typ.Integer)
	if !typ.TypeEquals(merged, typ.Number) {
		t.Fatalf("expected wider supertype number, got %v", merged)
	}
}

func TestMergeFuncTypes_IsCommutativeForIncomparableSignatures(t *testing.T) {
	coarse := typ.Func().
		Param("entries", typ.Any).
		Returns(typ.Integer).
		Build()
	refined := typ.Func().
		Param("entries", typ.NewArray(typ.String)).
		Returns(typ.Integer).
		Build()

	forward := MergeFunctionFactType(coarse, refined)
	reverse := MergeFunctionFactType(refined, coarse)
	if !typ.TypeEquals(forward, reverse) {
		t.Fatalf("expected commutative merge result, got forward=%v reverse=%v", forward, reverse)
	}
}

func TestMergeFuncTypes_AliasInputsUseCanonicalJoin(t *testing.T) {
	coarse := typ.NewAlias("CoarseFn", typ.Func().
		Param("entries", typ.Any).
		Returns(typ.Integer).
		Build())
	refined := typ.NewAlias("RefinedFn", typ.Func().
		Param("entries", typ.NewArray(typ.String)).
		Returns(typ.Integer).
		Build())

	forward := MergeFunctionFactType(coarse, refined)
	reverse := MergeFunctionFactType(refined, coarse)
	if !typ.TypeEquals(forward, reverse) {
		t.Fatalf("expected commutative alias merge result, got forward=%v reverse=%v", forward, reverse)
	}
}

func TestMergeFuncTypes_MapVsOpenRecordUsesCanonicalJoin(t *testing.T) {
	coarse := typ.Func().
		Param("t", typ.NewRecord().SetOpen(true).Build()).
		Returns(typ.String).
		Build()
	refined := typ.Func().
		Param("t", typ.NewMap(typ.String, typ.NewArray(typ.String))).
		Returns(typ.String).
		Build()

	forward := MergeFunctionFactType(coarse, refined)
	reverse := MergeFunctionFactType(refined, coarse)
	if !typ.TypeEquals(forward, reverse) {
		t.Fatalf("expected commutative map/open-record merge result, got forward=%v reverse=%v", forward, reverse)
	}
}

func TestWidenLiteralSigs_DoesNotNarrowComparableSignature(t *testing.T) {
	lit := &ast.FunctionExpr{}

	prev := api.LiteralSigs{
		lit: typ.Func().Returns(typ.Number).Build(),
	}
	next := api.LiteralSigs{
		lit: typ.Func().Returns(typ.Integer).Build(),
	}

	merged := WidenLiteralSigs(prev, next)
	got := merged[lit]
	if got == nil {
		t.Fatal("expected merged literal signature")
	}
	if len(got.Returns) != 1 {
		t.Fatalf("expected one return, got %d", len(got.Returns))
	}
	if !subtype.IsSubtype(prev[lit].Returns[0], got.Returns[0]) {
		t.Fatalf("expected merged return to be supertype of prev (%v), got %v", prev[lit].Returns[0], got.Returns[0])
	}
	if !subtype.IsSubtype(next[lit].Returns[0], got.Returns[0]) {
		t.Fatalf("expected merged return to be supertype of next (%v), got %v", next[lit].Returns[0], got.Returns[0])
	}
	if typ.TypeEquals(got.Returns[0], next[lit].Returns[0]) {
		t.Fatalf("expected merged return not to regress to narrower next-only type %v", got.Returns[0])
	}
}

func TestWidenLiteralSigs_PrefersMergedSameShapeSignature(t *testing.T) {
	lit := &ast.FunctionExpr{}

	prev := api.LiteralSigs{
		lit: typ.Func().Returns(typ.String).Build(),
	}
	next := api.LiteralSigs{
		lit: typ.Func().Returns(typ.Integer).Build(),
	}

	merged := WidenLiteralSigs(prev, next)
	got := merged[lit]
	if got == nil {
		t.Fatal("expected merged literal signature")
	}
	if len(got.Returns) != 1 {
		t.Fatalf("expected one return, got %d", len(got.Returns))
	}
	want := typ.NewUnion(typ.String, typ.Integer)
	if !typ.TypeEquals(got.Returns[0], want) {
		t.Fatalf("expected merged return %v, got %v", want, got.Returns[0])
	}
}

func TestTypeContainsFunction_IgnoresInterfaceMethodSignatures(t *testing.T) {
	iface := typ.NewInterface("Reader", []typ.Method{
		{
			Name: "next",
			Type: typ.Func().
				Param("self", typ.Self).
				Returns(typ.Func().Returns(typ.String).Build()).
				Build(),
		},
	})
	if typeContainsFunction(iface) {
		t.Fatalf("expected interface method signatures to be ignored, got true")
	}
}

func TestHasHigherOrderGrowthRisk_DetectsFunctionReturningFunction(t *testing.T) {
	tp := typ.Func().
		Returns(typ.Func().Returns(typ.String).Build()).
		Build()
	if !hasHigherOrderGrowthRisk(tp) {
		t.Fatalf("expected higher-order growth risk to be detected")
	}
}

func TestMethodTypeHasSelfRecursiveReturn_IgnoresInterfaceMethods(t *testing.T) {
	owner := typ.NewRecord().Field("id", typ.String).Build()
	methodType := typ.NewInterface("HasBuild", []typ.Method{
		{
			Name: "build",
			Type: typ.Func().
				Param("self", typ.Self).
				Returns(owner).
				Build(),
		},
	})
	if methodTypeHasSelfRecursiveReturn(methodType, owner) {
		t.Fatalf("expected interface method signatures to be ignored for self-recursive detection")
	}
}
