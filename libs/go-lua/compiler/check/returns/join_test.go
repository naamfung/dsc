package returns

import (
	"testing"

	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	typjoin "github.com/wippyai/go-lua/types/typ/join"
)

func TestJoinReturnVectors_Empty(t *testing.T) {
	result := typjoin.ReturnVectors(nil, nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestJoinReturnVectors_AEmpty(t *testing.T) {
	b := []typ.Type{typ.String}
	result := typjoin.ReturnVectors(nil, b)
	if len(result) != 1 || result[0] != typ.String {
		t.Errorf("expected [string], got %v", result)
	}
}

func TestJoinReturnVectors_BEmpty(t *testing.T) {
	a := []typ.Type{typ.Number}
	result := typjoin.ReturnVectors(a, nil)
	if len(result) != 1 || result[0] != typ.Number {
		t.Errorf("expected [number], got %v", result)
	}
}

func TestJoinReturnVectors_SameLength(t *testing.T) {
	a := []typ.Type{typ.String}
	b := []typ.Type{typ.Number}
	result := typjoin.ReturnVectors(a, b)
	if len(result) != 1 {
		t.Errorf("expected length 1, got %d", len(result))
	}
}

func TestTypJoinReturnSlot_PreservesUnknownOverNil(t *testing.T) {
	got := typ.JoinReturnSlot(typ.Unknown, typ.Nil)
	if !typ.TypeEquals(got, typ.Unknown) {
		t.Fatalf("typ.JoinReturnSlot(unknown, nil) = %v, want unknown", got)
	}

	got = typ.JoinReturnSlot(typ.Nil, typ.Unknown)
	if !typ.TypeEquals(got, typ.Unknown) {
		t.Fatalf("typ.JoinReturnSlot(nil, unknown) = %v, want unknown", got)
	}
}

func TestReturnTypesAllNil(t *testing.T) {
	if !ReturnTypesAllNil([]typ.Type{typ.Nil}) {
		t.Fatal("expected [nil] to be nil-only")
	}
	if ReturnTypesAllNil([]typ.Type{typ.Nil, typ.Unknown}) {
		t.Fatal("expected [nil, unknown] to not be nil-only")
	}
	if ReturnTypesAllNil(nil) {
		t.Fatal("expected empty return vector to not be nil-only")
	}
}

func TestJoinReturnVectors_DifferentLengths(t *testing.T) {
	a := []typ.Type{typ.String, typ.Number}
	b := []typ.Type{typ.Boolean}
	result := typjoin.ReturnVectors(a, b)
	if len(result) != 2 {
		t.Errorf("expected length 2, got %d", len(result))
	}
}

func TestReturnTypesEqual_Empty(t *testing.T) {
	if !ReturnTypesEqual(nil, nil) {
		t.Error("nil slices should be equal")
	}
}

func TestReturnTypesEqual_DifferentLength(t *testing.T) {
	a := []typ.Type{typ.String}
	b := []typ.Type{typ.String, typ.Number}
	if ReturnTypesEqual(a, b) {
		t.Error("different lengths should not be equal")
	}
}

func TestReturnTypesEqual_Same(t *testing.T) {
	a := []typ.Type{typ.String, typ.Number}
	b := []typ.Type{typ.String, typ.Number}
	if !ReturnTypesEqual(a, b) {
		t.Error("same types should be equal")
	}
}

func TestReturnTypesEqual_Different(t *testing.T) {
	a := []typ.Type{typ.String}
	b := []typ.Type{typ.Number}
	if ReturnTypesEqual(a, b) {
		t.Error("different types should not be equal")
	}
}

func TestReturnTypesRefine_EmptyA(t *testing.T) {
	b := []typ.Type{typ.String}
	if ReturnTypesRefine(nil, b) {
		t.Error("empty a should not refine b")
	}
}

func TestReturnTypesRefine_EmptyB(t *testing.T) {
	a := []typ.Type{typ.String}
	if !ReturnTypesRefine(a, nil) {
		t.Error("a should refine empty b")
	}
}

func TestReturnTypesRefine_Same(t *testing.T) {
	a := []typ.Type{typ.String}
	b := []typ.Type{typ.String}
	if !ReturnTypesRefine(a, b) {
		t.Error("same types should refine")
	}
}

func TestReturnTypesRefine_DifferentLength(t *testing.T) {
	a := []typ.Type{typ.String, typ.Number}
	b := []typ.Type{typ.String}
	if ReturnTypesRefine(a, b) {
		t.Error("different lengths should not refine")
	}
}

func TestReturnTypesExtendRecord_Empty(t *testing.T) {
	if ReturnTypesExtendRecord(nil, nil) {
		t.Error("empty vectors should not extend")
	}
}

func TestReturnTypesExtendRecord_NotRecords(t *testing.T) {
	a := []typ.Type{typ.String}
	b := []typ.Type{typ.String}
	if ReturnTypesExtendRecord(a, b) {
		t.Error("non-records should not extend")
	}
}

func TestReturnTypesExtendRecord_RecordExtends(t *testing.T) {
	oldRec := typ.NewRecord().Field("x", typ.Number).Build()
	newRec := typ.NewRecord().Field("x", typ.Number).Field("y", typ.Number).Build()
	a := []typ.Type{newRec}
	b := []typ.Type{oldRec}
	if !ReturnTypesExtendRecord(a, b) {
		t.Error("record with more fields should extend")
	}
}

func TestReturnTypesElideOptional_Empty(t *testing.T) {
	if ReturnTypesElideOptional(nil, nil) {
		t.Error("empty vectors should not elide")
	}
}

func TestSelectPreferredReturnVector_Refinement(t *testing.T) {
	preferred, ok := SelectPreferredReturnVector([]typ.Type{typ.String}, []typ.Type{typ.NewOptional(typ.String)})
	if !ok {
		t.Fatal("expected preferred vector")
	}
	if len(preferred) != 1 || !typ.TypeEquals(preferred[0], typ.String) {
		t.Fatalf("expected refined string return, got %v", preferred)
	}
}

func TestSelectPreferredReturnVector_AvoidsNilOnlyRegression(t *testing.T) {
	preferred, ok := SelectPreferredReturnVector([]typ.Type{typ.Nil}, []typ.Type{typ.NewOptional(typ.String)})
	if !ok {
		t.Fatal("expected preferred vector")
	}
	if len(preferred) != 1 || !typ.TypeEquals(preferred[0], typ.NewOptional(typ.String)) {
		t.Fatalf("expected non-nil summary to be preserved, got %v", preferred)
	}
}

func TestSelectPreferredReturnVector_RejectsStaleNilOnly(t *testing.T) {
	preferred, ok := SelectPreferredReturnVector([]typ.Type{typ.NewOptional(typ.String)}, []typ.Type{typ.Nil})
	if !ok {
		t.Fatal("expected preferred vector")
	}
	if len(preferred) != 1 || !typ.TypeEquals(preferred[0], typ.NewOptional(typ.String)) {
		t.Fatalf("expected new non-nil summary to replace nil-only, got %v", preferred)
	}
}

func TestSelectPreferredReturnVector_RecordExtension(t *testing.T) {
	oldRec := typ.NewRecord().Field("x", typ.Number).Build()
	newRec := typ.NewRecord().Field("x", typ.Number).Field("y", typ.String).Build()

	preferred, ok := SelectPreferredReturnVector([]typ.Type{newRec}, []typ.Type{oldRec})
	if !ok {
		t.Fatal("expected preferred vector")
	}
	if len(preferred) != 1 || !typ.TypeEquals(preferred[0], newRec) {
		t.Fatalf("expected record extension to be preferred, got %v", preferred)
	}
}

func TestSelectRefiningReturnVector_Refinement(t *testing.T) {
	refined := []typ.Type{typ.String}
	baseline := []typ.Type{typ.NewOptional(typ.String)}

	got, ok := SelectRefiningReturnVector(refined, baseline)
	if !ok {
		t.Fatal("expected refinement to be selected")
	}
	if len(got) != 1 || !typ.TypeEquals(got[0], typ.String) {
		t.Fatalf("expected refined return vector, got %v", got)
	}
}

func TestSelectRefiningReturnVector_DoesNotSelectOlderNarrowerBaseline(t *testing.T) {
	candidate := []typ.Type{typ.Any}
	baseline := []typ.Type{typ.False}

	_, ok := SelectRefiningReturnVector(candidate, baseline)
	if ok {
		t.Fatal("did not expect baseline-narrower relation to select candidate")
	}
}

func TestReturnTypesFillNilSlots(t *testing.T) {
	candidate := []typ.Type{typ.NewMap(typ.Unknown, typ.NewArray(typ.Unknown)), typ.NewArray(typ.Unknown)}
	baseline := []typ.Type{typ.NewMap(typ.Unknown, typ.NewArray(typ.Unknown)), typ.Nil}
	if !ReturnTypesFillNilSlots(candidate, baseline) {
		t.Fatalf("expected candidate to fill nil slot: candidate=%v baseline=%v", candidate, baseline)
	}
}

func TestMergeReturnSummary_PrefersCandidateRefinement(t *testing.T) {
	existing := []typ.Type{typ.NewOptional(typ.String)}
	candidate := []typ.Type{typ.String}

	merged := MergeReturnSummary(existing, candidate)
	if len(merged) != 1 || !typ.TypeEquals(merged[0], typ.String) {
		t.Fatalf("expected refined candidate return, got %v", merged)
	}
}

func TestMergeReturnSummary_FillsNilSlotWithCandidateEvidence(t *testing.T) {
	existing := []typ.Type{
		typ.NewMap(typ.Unknown, typ.NewArray(typ.Unknown)),
		typ.Nil,
	}
	candidate := []typ.Type{
		typ.NewMap(typ.Unknown, typ.NewArray(typ.Unknown)),
		typ.NewArray(typ.Unknown),
	}

	merged := MergeReturnSummary(existing, candidate)
	if len(merged) != 2 {
		t.Fatalf("expected two return slots, got %v", merged)
	}
	if !typ.TypeEquals(merged[1], typ.NewArray(typ.Unknown)) {
		t.Fatalf("expected nil slot to be filled with array evidence, got %v", merged[1])
	}
}

func TestMergeFunctionFactType_MergesSameShapeReturnsCanonically(t *testing.T) {
	existing := typ.Func().
		Param("x", typ.String).
		Returns(typ.NewOptional(typ.Integer)).
		Build()
	candidate := typ.Func().
		Param("x", typ.String).
		Returns(typ.Integer).
		Build()

	merged := MergeFunctionFactType(existing, candidate)
	fn, ok := merged.(*typ.Function)
	if !ok || len(fn.Returns) != 1 {
		t.Fatalf("expected merged function, got %T", merged)
	}
	if !typ.TypeEquals(fn.Returns[0], typ.Integer) {
		t.Fatalf("expected refined return integer, got %v", fn.Returns[0])
	}
}

func TestMergeFunctionFactType_PrefersConcreteParamOverSoftAny(t *testing.T) {
	existing := typ.Func().
		Param("x", typ.Any).
		Returns(typ.String).
		Build()
	candidate := typ.Func().
		Param("x", typ.String).
		Returns(typ.String).
		Build()

	merged := MergeFunctionFactType(existing, candidate)
	fn, ok := merged.(*typ.Function)
	if !ok {
		t.Fatalf("expected merged function, got %T", merged)
	}
	if len(fn.Params) != 1 || !typ.TypeEquals(fn.Params[0].Type, typ.String) {
		t.Fatalf("expected param refined to string, got %+v", fn.Params)
	}
}

func TestMergeFunctionFactType_WidensParamToCoverObservedCallsites(t *testing.T) {
	existing := typ.Func().
		Param("t", typ.NewArray(typ.Any)).
		Returns(typ.String).
		Build()
	candidate := typ.Func().
		Param("t", typ.NewMap(typ.String, typ.Any)).
		Returns(typ.String).
		Build()

	merged := MergeFunctionFactType(existing, candidate)
	fn, ok := merged.(*typ.Function)
	if !ok {
		t.Fatalf("expected merged function, got %T", merged)
	}
	if len(fn.Params) != 1 {
		t.Fatalf("expected one param, got %+v", fn.Params)
	}
	if typ.TypeEquals(fn.Params[0].Type, typ.NewArray(typ.Any)) {
		t.Fatalf("expected param widening beyond array-only shape, got %v", fn.Params[0].Type)
	}
	wantMap := typ.NewMap(typ.String, typ.Any)
	if !subtype.IsSubtype(wantMap, fn.Params[0].Type) {
		t.Fatalf("expected merged param to admit map callsite evidence, got %v", fn.Params[0].Type)
	}
}

func TestMergeFunctionFactType_DoesNotDropNonFunctionUnionMembers(t *testing.T) {
	fn := typ.Func().Param("x", typ.String).Returns(typ.String).Build()
	existing := typ.NewUnion(fn, typ.Number)
	candidate := typ.Func().Param("x", typ.String).Returns(typ.String).Build()

	merged := MergeFunctionFactType(existing, candidate)
	u, ok := merged.(*typ.Union)
	if !ok {
		t.Fatalf("expected union to be preserved, got %T", merged)
	}
	hasNumber := false
	for _, m := range u.Members {
		if typ.TypeEquals(m, typ.Number) {
			hasNumber = true
			break
		}
	}
	if !hasNumber {
		t.Fatalf("expected merged union to retain non-function member, got %v", merged)
	}
}

func TestMergeFunctionFactType_CollapsesCompatibleFunctionVariants(t *testing.T) {
	base := typ.Func().
		OptParam("entries", typ.Any).
		Returns(typ.NewMap(typ.Unknown, typ.NewArray(typ.Unknown))).
		Build()
	refinedEntry := typ.NewRecord().Field("id", typ.String).Build()
	refined := typ.Func().
		OptParam("entries", typ.NewArray(refinedEntry)).
		Returns(typ.NewMap(typ.String, typ.NewArray(refinedEntry))).
		Build()

	merged := MergeFunctionFactType(base, refined)
	fn, ok := merged.(*typ.Function)
	if !ok {
		t.Fatalf("expected function after compatible-variant collapse, got %T", merged)
	}
	if len(fn.Params) != 1 || !typ.TypeEquals(fn.Params[0].Type, typ.NewArray(refinedEntry)) {
		t.Fatalf("expected refined param type to win, got %+v", fn.Params)
	}
	if len(fn.Returns) != 1 || !typ.TypeEquals(fn.Returns[0], typ.NewMap(typ.String, typ.NewArray(refinedEntry))) {
		t.Fatalf("expected refined return map, got %v", fn.Returns)
	}
}

func TestMergeFunctionFactType_DoesNotCollapseParamToNilWhenOptionalInfoExists(t *testing.T) {
	existing := typ.Func().
		OptParam("tests", typ.Nil).
		Returns(typ.Integer).
		Build()
	candidate := typ.Func().
		OptParam("tests", typ.NewOptional(typ.NewArray(typ.Any))).
		Returns(typ.Integer).
		Build()

	merged := MergeFunctionFactType(existing, candidate)
	fn, ok := merged.(*typ.Function)
	if !ok {
		t.Fatalf("expected function, got %T", merged)
	}
	want := typ.NewOptional(typ.NewArray(typ.Any))
	if len(fn.Params) != 1 || !typ.TypeEquals(fn.Params[0].Type, want) {
		t.Fatalf("expected param type %v, got %+v", want, fn.Params)
	}
}

func TestMergeReturnSummary_ElidesOptionalForInterfaceFieldRecords(t *testing.T) {
	txType := typ.NewInterface("sql.Tx", []typ.Method{
		{Name: "rollback", Type: typ.Func().Param("self", typ.Self).Build()},
	})
	dbType := typ.NewInterface("sql.DB", []typ.Method{
		{Name: "begin", Type: typ.Func().Param("self", typ.Self).Returns(txType, typ.NewOptional(typ.LuaError)).Build()},
		{Name: "release", Type: typ.Func().Param("self", typ.Self).Build()},
	})

	existing := []typ.Type{
		typ.NewRecord().
			Field("db", typ.NewOptional(dbType)).
			Field("tx", typ.NewOptional(txType)).
			Build(),
	}
	candidate := []typ.Type{
		typ.NewRecord().
			Field("db", dbType).
			Field("tx", txType).
			Build(),
	}

	merged := MergeReturnSummary(existing, candidate)
	if len(merged) != 1 || !typ.TypeEquals(merged[0], candidate[0]) {
		t.Fatalf("expected candidate optional-elision to win, got %v", merged)
	}
}

func TestWithSummaryOrUnknown_AppliesSummaryToPlaceholderReturns(t *testing.T) {
	fn := typ.Func().
		Param("x", typ.String).
		Returns(typ.Unknown).
		Build()
	summary := []typ.Type{typ.Integer}

	got := WithSummaryOrUnknown(fn, summary)
	if got == nil || len(got.Returns) != 1 {
		t.Fatalf("expected function return, got %v", got)
	}
	if !typ.TypeEquals(got.Returns[0], typ.Integer) {
		t.Fatalf("expected summary return integer, got %v", got.Returns[0])
	}
}

func TestWithSummaryOrUnknown_DefaultsToUnknownWhenMissing(t *testing.T) {
	fn := typ.Func().Param("x", typ.String).Build()
	got := WithSummaryOrUnknown(fn, nil)
	if got == nil || len(got.Returns) != 1 {
		t.Fatalf("expected one default return, got %v", got)
	}
	if !typ.TypeEquals(got.Returns[0], typ.Unknown) {
		t.Fatalf("expected default unknown return, got %v", got.Returns[0])
	}
}

func TestTypeExtendsRecord_NilTypes(t *testing.T) {
	if TypeExtendsRecord(nil, typ.String) {
		t.Error("nil a should not extend")
	}
	if TypeExtendsRecord(typ.String, nil) {
		t.Error("nil b should not extend")
	}
}

func TestTypeExtendsRecord_NotRecord(t *testing.T) {
	if TypeExtendsRecord(typ.String, typ.String) {
		t.Error("non-record should not extend")
	}
}

func TestNormalizeReturnVector_Empty(t *testing.T) {
	result := NormalizeReturnVector(nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestNormalizeReturnVector_ReplacesNil(t *testing.T) {
	input := []typ.Type{typ.String, nil, typ.Number}
	result := NormalizeReturnVector(input)
	if len(result) != 3 {
		t.Fatalf("expected length 3, got %d", len(result))
	}
	if result[0] != typ.String {
		t.Error("first element should be string")
	}
	if result[1] != typ.Nil {
		t.Error("nil should be replaced with typ.Nil")
	}
	if result[2] != typ.Number {
		t.Error("third element should be number")
	}
}

// Regression tests for recordSuperset map component handling.

func TestRecordSuperset_BothHaveMapComponent(t *testing.T) {
	oldRec := typ.NewRecord().MapComponent(typ.String, typ.Number).Build()
	newRec := typ.NewRecord().MapComponent(typ.String, typ.Number).Field("x", typ.Number).Build()
	a := []typ.Type{newRec}
	b := []typ.Type{oldRec}
	if !ReturnTypesExtendRecord(a, b) {
		t.Error("record with same map component and additional fields should extend")
	}
}

func TestRecordSuperset_OldHasNoMapComponent(t *testing.T) {
	oldRec := typ.NewRecord().Field("x", typ.Number).Build()
	newRec := typ.NewRecord().Field("x", typ.Number).Field("y", typ.String).Build()
	a := []typ.Type{newRec}
	b := []typ.Type{oldRec}
	if !ReturnTypesExtendRecord(a, b) {
		t.Error("record with additional fields should extend record without map component")
	}
}

func TestAlignFunctionTypeWithSummary_AppliesStrictRefinement(t *testing.T) {
	fn := typ.Func().
		Param("entries", typ.Any).
		Returns(typ.NewMap(typ.Unknown, typ.NewArray(typ.Unknown))).
		Build()

	summary := []typ.Type{
		typ.NewRecord().
			SetOpen(true).
			MapComponent(typ.String, typ.NewArray(typ.Unknown)).
			Build(),
	}

	aligned, changed := AlignFunctionTypeWithSummary(fn, summary)
	if !changed {
		t.Fatal("expected alignment to apply strict refinement summary")
	}
	if aligned == nil || len(aligned.Returns) != 1 {
		t.Fatalf("unexpected aligned returns: %v", aligned)
	}
	got := aligned.Returns[0]
	rec, ok := got.(*typ.Record)
	if !ok || !rec.HasMapComponent() {
		t.Fatalf("expected record map-component return, got %T (%v)", got, got)
	}
	if !typ.TypeEquals(rec.MapKey, typ.String) {
		t.Fatalf("expected map key string, got %v", rec.MapKey)
	}
}

func TestAlignFunctionTypeWithSummary_ReplacesOpenTopRecordWithStructuredSummary(t *testing.T) {
	openTop := typ.NewRecord().SetOpen(true).Build()
	fn := typ.Func().Returns(openTop).Build()
	summary := []typ.Type{typ.NewArray(typ.Unknown)}

	aligned, changed := AlignFunctionTypeWithSummary(fn, summary)
	if !changed {
		t.Fatal("expected open-top placeholder to be replaced by structured summary")
	}
	if aligned == nil || len(aligned.Returns) != 1 {
		t.Fatalf("unexpected aligned function: %v", aligned)
	}
	if !typ.TypeEquals(aligned.Returns[0], summary[0]) {
		t.Fatalf("expected %v, got %v", summary[0], aligned.Returns[0])
	}
}

func TestAlignFunctionTypeWithSummary_DoesNotDowngradeStructuredToPlaceholder(t *testing.T) {
	structured := typ.NewRecord().Field("get_x", typ.Func().Build()).Build()
	fn := typ.Func().Returns(structured).Build()
	summary := []typ.Type{typ.Any}

	aligned, changed := AlignFunctionTypeWithSummary(fn, summary)
	if changed {
		t.Fatalf("expected no downgrade change, got %v", aligned)
	}
	if aligned == nil || len(aligned.Returns) != 1 {
		t.Fatalf("unexpected aligned function: %v", aligned)
	}
	if !typ.TypeEquals(aligned.Returns[0], structured) {
		t.Fatalf("expected %v, got %v", structured, aligned.Returns[0])
	}
}

func TestMergeReturnSummary_PrefersRuntimePossibleSummaryOverNeverArtifact(t *testing.T) {
	bad := []typ.Type{
		typ.NewUnion(
			typ.NewRecord().
				Field("success", typ.True).
				Field("result", typ.NewRecord().OptField("data", typ.Never).Build()).
				Build(),
			typ.NewRecord().
				Field("success", typ.False).
				Field("error", typ.LiteralString("missing")).
				Build(),
		),
	}
	good := []typ.Type{
		typ.NewUnion(
			typ.NewRecord().
				Field("success", typ.True).
				Field("result", typ.NewRecord().OptField("data", typ.Unknown).Build()).
				Build(),
			typ.NewRecord().
				Field("success", typ.False).
				Field("error", typ.LiteralString("missing")).
				Build(),
		),
	}

	got := MergeReturnSummary(bad, good)
	if !ReturnTypesEqual(got, good) {
		t.Fatalf("MergeReturnSummary(%v, %v) = %v, want %v", bad, good, got, good)
	}
}

func TestAlignFunctionTypeWithSummary_RepairsNestedNeverArtifact(t *testing.T) {
	bad := typ.NewUnion(
		typ.NewRecord().
			Field("success", typ.True).
			Field("result", typ.NewRecord().OptField("data", typ.Never).Build()).
			Build(),
		typ.NewRecord().
			Field("success", typ.False).
			Field("error", typ.LiteralString("missing")).
			Build(),
	)
	good := typ.NewUnion(
		typ.NewRecord().
			Field("success", typ.True).
			Field("result", typ.NewRecord().OptField("data", typ.Unknown).Build()).
			Build(),
		typ.NewRecord().
			Field("success", typ.False).
			Field("error", typ.LiteralString("missing")).
			Build(),
	)

	fn := typ.Func().Returns(bad).Build()
	aligned, changed := AlignFunctionTypeWithSummary(fn, []typ.Type{good})
	if !changed {
		t.Fatal("expected never-artifact repair to update function returns")
	}
	if aligned == nil || len(aligned.Returns) != 1 || !typ.TypeEquals(aligned.Returns[0], good) {
		t.Fatalf("aligned returns = %v, want %v", aligned, good)
	}
}

func TestRecordSuperset_NewHasMapComponentOldDoesNot(t *testing.T) {
	oldRec := typ.NewRecord().Field("x", typ.Number).Build()
	newRec := typ.NewRecord().Field("x", typ.Number).MapComponent(typ.String, typ.Any).Build()
	a := []typ.Type{newRec}
	b := []typ.Type{oldRec}
	if !ReturnTypesExtendRecord(a, b) {
		t.Error("record with additional map component should extend record without it")
	}
}

func TestRecordSuperset_IncompatibleMapComponent(t *testing.T) {
	oldRec := typ.NewRecord().MapComponent(typ.Number, typ.String).Build()
	newRec := typ.NewRecord().MapComponent(typ.String, typ.Number).Build()
	a := []typ.Type{newRec}
	b := []typ.Type{oldRec}
	if ReturnTypesExtendRecord(a, b) {
		t.Error("record with incompatible map component should not extend")
	}
}

// Regression: recordSuperset should use && not || for map component check.
// This test verifies the fix by checking that the code uses HasMapComponent semantics.
func TestTypeExtendsRecord_MapComponentConsistency(t *testing.T) {
	// When old has map component, new must have compatible map component
	oldRec := typ.NewRecord().MapComponent(typ.String, typ.Number).Build()
	newRec := typ.NewRecord().Field("x", typ.Number).Build()
	if TypeExtendsRecord(newRec, oldRec) {
		t.Error("record without map component should not extend record with map component")
	}
}

func TestMergeReturnSummary_PrefersStructuredCollectionOverOpenTopRecordField(t *testing.T) {
	weak := []typ.Type{
		typ.NewRecord().
			Field("messages", typ.NewRecord().SetOpen(true).Build()).
			Field("system", typ.Nil).
			Build(),
	}
	strong := []typ.Type{
		typ.NewRecord().
			Field("messages", typ.NewArray(
				typ.NewRecord().Field("role", typ.Unknown).Build(),
			)).
			Field("system", typ.Nil).
			Build(),
	}

	merged := MergeReturnSummary(weak, strong)
	if len(merged) != 1 {
		t.Fatalf("expected one return slot, got %d", len(merged))
	}

	ret, ok := merged[0].(*typ.Record)
	if !ok {
		t.Fatalf("expected record return, got %T (%v)", merged[0], merged[0])
	}
	msgField := ret.GetField("messages")
	if msgField == nil {
		t.Fatalf("expected messages field in merged return: %v", merged[0])
	}
	if _, ok := msgField.Type.(*typ.Array); !ok {
		t.Fatalf("expected messages field to remain array-like, got %T (%v)", msgField.Type, msgField.Type)
	}
}

func TestMergeReturnSummary_PromotesTopLevelStructuredOverOpenTop(t *testing.T) {
	weak := []typ.Type{
		typ.NewRecord().SetOpen(true).Build(),
	}
	strong := []typ.Type{
		typ.NewArray(typ.Any),
	}

	merged := MergeReturnSummary(weak, strong)
	if len(merged) != 1 {
		t.Fatalf("expected one return slot, got %d", len(merged))
	}
	if _, ok := merged[0].(*typ.Array); !ok {
		t.Fatalf("expected top-level array after merge, got %T (%v)", merged[0], merged[0])
	}
}
