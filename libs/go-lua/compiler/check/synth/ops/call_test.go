package ops

import (
	"testing"

	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/typ"
)

func TestCallWithGenericInference_Simple(t *testing.T) {
	fn := typ.Func().
		Param("x", typ.Integer).
		Param("y", typ.String).
		Returns(typ.Boolean).
		Build()

	ctx := db.NewQueryContext(db.New())
	def := CallDef{
		Callee: fn,
		Args:   []typ.Type{typ.Integer, typ.String},
	}

	result := CallWithGenericInference(ctx, def)
	if result.Type != typ.Boolean {
		t.Errorf("expected boolean, got %v", result.Type)
	}

	if len(result.Errors) > 0 {
		t.Errorf("unexpected errors: %v", result.Errors)
	}
}

func TestCallWithGenericInference_TooFewArgs(t *testing.T) {
	fn := typ.Func().
		Param("x", typ.Integer).
		Param("y", typ.String).
		Returns(typ.Boolean).
		Build()

	ctx := db.NewQueryContext(db.New())
	def := CallDef{
		Callee: fn,
		Args:   []typ.Type{typ.Integer},
	}

	result := CallWithGenericInference(ctx, def)
	if len(result.Errors) == 0 {
		t.Error("expected wrong arity error")
	}
}

func TestCallWithGenericInference_TooManyArgs(t *testing.T) {
	fn := typ.Func().
		Param("x", typ.Integer).
		Returns(typ.Boolean).
		Build()

	ctx := db.NewQueryContext(db.New())
	def := CallDef{
		Callee: fn,
		Args:   []typ.Type{typ.Integer, typ.String, typ.Boolean},
	}

	result := CallWithGenericInference(ctx, def)
	if len(result.Errors) == 0 {
		t.Error("expected wrong arity error")
	}
}

func TestCallWithGenericInference_ZeroParamAllowsExtraArgs(t *testing.T) {
	fn := typ.Func().
		Returns(typ.Boolean).
		Build()

	ctx := db.NewQueryContext(db.New())
	def := CallDef{
		Callee: fn,
		Args:   []typ.Type{typ.Integer, typ.String},
	}

	result := CallWithGenericInference(ctx, def)
	if len(result.Errors) > 0 {
		t.Fatalf("zero-param function should accept extra args, got: %v", result.Errors)
	}
}

func TestCallWithGenericInference_Variadic(t *testing.T) {
	fn := typ.Func().
		Param("x", typ.Integer).
		Variadic(typ.String).
		Returns(typ.Boolean).
		Build()

	ctx := db.NewQueryContext(db.New())
	def := CallDef{
		Callee: fn,
		Args:   []typ.Type{typ.Integer, typ.String, typ.String, typ.String},
	}

	result := CallWithGenericInference(ctx, def)
	if len(result.Errors) > 0 {
		t.Errorf("variadic should accept extra args: %v", result.Errors)
	}
}

func TestCallWithGenericInference_Mismatch(t *testing.T) {
	fn := typ.Func().
		Param("x", typ.Integer).
		Returns(typ.Boolean).
		Build()

	ctx := db.NewQueryContext(db.New())
	def := CallDef{
		Callee: fn,
		Args:   []typ.Type{typ.String},
	}

	result := CallWithGenericInference(ctx, def)
	if len(result.Errors) == 0 {
		t.Error("expected type mismatch error")
	}
}

func TestCallWithGenericInference_NotCallable(t *testing.T) {
	ctx := db.NewQueryContext(db.New())
	def := CallDef{
		Callee: typ.Integer,
		Args:   []typ.Type{typ.Integer},
	}

	result := CallWithGenericInference(ctx, def)
	if len(result.Errors) == 0 {
		t.Error("expected not callable error")
	}
}

func TestCallWithGenericInference_NilCallee(t *testing.T) {
	ctx := db.NewQueryContext(db.New())
	def := CallDef{
		Callee: nil,
		Args:   []typ.Type{},
	}

	result := CallWithGenericInference(ctx, def)
	if len(result.Errors) == 0 {
		t.Error("expected error for nil callee")
	}
}

func TestCallWithGenericInference_AnyCallee(t *testing.T) {
	ctx := db.NewQueryContext(db.New())
	def := CallDef{
		Callee: typ.Any,
		Args:   []typ.Type{typ.Integer},
	}

	result := CallWithGenericInference(ctx, def)
	if result.Type != typ.Any {
		t.Errorf("calling any should return any, got %v", result.Type)
	}
}

func TestCallWithGenericInference_MultiReturn(t *testing.T) {
	fn := typ.Func().
		Param("x", typ.Integer).
		Returns(typ.String, typ.Boolean).
		Build()

	ctx := db.NewQueryContext(db.New())
	def := CallDef{
		Callee: fn,
		Args:   []typ.Type{typ.Integer},
	}

	result := CallWithGenericInference(ctx, def)
	if result.Type == nil {
		t.Fatal("expected non-nil result type")
	}

	tuple, ok := result.Type.(*typ.Tuple)
	if !ok {
		t.Fatalf("expected tuple, got %T", result.Type)
	}

	if len(tuple.Elements) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(tuple.Elements))
	}

	if tuple.Elements[0] != typ.String {
		t.Errorf("first return should be string, got %v", tuple.Elements[0])
	}

	if tuple.Elements[1] != typ.Boolean {
		t.Errorf("second return should be boolean, got %v", tuple.Elements[1])
	}

	if len(result.Returns) != 2 {
		t.Fatalf("expected 2 return slots, got %d", len(result.Returns))
	}
	if result.Returns[0] != typ.String || result.Returns[1] != typ.Boolean {
		t.Fatalf("unexpected return vector: %v", result.Returns)
	}
}

func TestCallWithGenericInference_SingleTupleReturnPreservesArityOne(t *testing.T) {
	tupleValue := typ.NewTuple(typ.Integer, typ.String)
	fn := typ.Func().
		Returns(tupleValue).
		Build()

	ctx := db.NewQueryContext(db.New())
	def := CallDef{Callee: fn}

	result := CallWithGenericInference(ctx, def)
	if result.Type != tupleValue {
		t.Fatalf("expected tuple-valued return, got %v", result.Type)
	}
	if len(result.Returns) != 1 {
		t.Fatalf("expected 1 return slot, got %d", len(result.Returns))
	}
	if result.Returns[0] != tupleValue {
		t.Fatalf("expected first return slot to be tuple value, got %v", result.Returns[0])
	}
}

func TestCallWithGenericInference_NestedAliasFunctionIsCallable(t *testing.T) {
	fn := typ.Func().
		Param("x", typ.String).
		Returns(typ.Integer).
		Build()
	moduleAlias := typ.NewAlias("ModuleHandler", fn)
	localAlias := typ.NewAlias("Handler", moduleAlias)

	ctx := db.NewQueryContext(db.New())
	def := CallDef{
		Callee: localAlias,
		Args:   []typ.Type{typ.String},
	}

	result := CallWithGenericInference(ctx, def)
	if len(result.Errors) > 0 {
		t.Fatalf("expected nested alias callee to unwrap to function, got errors: %v", result.Errors)
	}
	if result.Type != typ.Integer {
		t.Fatalf("expected integer return through nested aliases, got %v", result.Type)
	}
}

func TestCallWithGenericInference_UnionCarriesMergedReturnVector(t *testing.T) {
	one := typ.Func().Returns(typ.String).Build()
	two := typ.Func().Returns(typ.String, typ.Boolean).Build()

	ctx := db.NewQueryContext(db.New())
	def := CallDef{
		Callee: typ.NewUnion(one, two),
	}

	result := CallWithGenericInference(ctx, def)
	if len(result.Returns) != 2 {
		t.Fatalf("expected merged arity 2, got %d", len(result.Returns))
	}
	if result.Returns[0] != typ.String {
		t.Fatalf("expected first merged slot string, got %v", result.Returns[0])
	}
	wantSecond := typ.NewUnion(typ.Nil, typ.Boolean)
	if !typ.TypeEquals(result.Returns[1], wantSecond) {
		t.Fatalf("expected second merged slot %v, got %v", wantSecond, result.Returns[1])
	}
}

func TestCallWithGenericInference_NoReturn(t *testing.T) {
	fn := typ.Func().
		Param("x", typ.Integer).
		Build()

	ctx := db.NewQueryContext(db.New())
	def := CallDef{
		Callee: fn,
		Args:   []typ.Type{typ.Integer},
	}

	result := CallWithGenericInference(ctx, def)
	if result.Type != typ.Nil {
		t.Errorf("function with no return should return nil, got %v", result.Type)
	}
}

func TestCallDef_Fields(t *testing.T) {
	def := CallDef{
		Callee:     typ.Func().Build(),
		Args:       []typ.Type{typ.Integer},
		IsMethod:   true,
		Receiver:   typ.String,
		MethodName: "test",
	}
	if def.Callee == nil {
		t.Error("callee should not be nil")
	}

	if len(def.Args) != 1 {
		t.Error("should have 1 arg")
	}

	if !def.IsMethod {
		t.Error("should be method call")
	}

	if def.Receiver != typ.String {
		t.Error("receiver should be string")
	}

	if def.MethodName != "test" {
		t.Error("method name should be test")
	}
}

func TestCallErrorKind_Constants(t *testing.T) {
	if ErrNotCallable != 0 {
		t.Error("ErrNotCallable should be 0")
	}

	if ErrWrongArity != 1 {
		t.Error("ErrWrongArity should be 1")
	}

	if ErrTypeMismatch != 2 {
		t.Error("ErrTypeMismatch should be 2")
	}

	if ErrOptionalCall != 3 {
		t.Error("ErrOptionalCall should be 3")
	}
}

func TestCallWithGenericInference_OptionalCallee(t *testing.T) {
	fn := typ.Func().Returns(typ.String).Build()
	optFn := typ.NewOptional(fn)

	ctx := db.NewQueryContext(db.New())
	def := CallDef{
		Callee: optFn,
		Args:   []typ.Type{},
	}

	result := CallWithGenericInference(ctx, def)
	hasOptError := false

	for _, err := range result.Errors {
		if err.Kind == ErrOptionalCall {
			hasOptError = true
		}
	}

	if !hasOptError {
		t.Error("should report optional call error")
	}
}

func TestInferCall_Function(t *testing.T) {
	fn := typ.Func().
		Param("x", typ.Integer).
		Returns(typ.String).
		Build()

	ctx := db.NewQueryContext(db.New())
	def := CallDef{
		Callee: fn,
		Args:   []typ.Type{typ.Integer},
	}

	infer := InferCall(ctx, def)
	if infer.Kind != InferKindFunction {
		t.Errorf("expected InferKindFunction, got %v", infer.Kind)
	}

	if infer.Instantiated == nil {
		t.Fatal("expected non-nil instantiated")
	}

	if len(infer.ExpectedArgs) != 1 {
		t.Fatalf("expected 1 expected arg, got %d", len(infer.ExpectedArgs))
	}

	if infer.ExpectedArgs[0] != typ.Integer {
		t.Errorf("expected arg 0 = integer, got %v", infer.ExpectedArgs[0])
	}
}

func TestInferCall_UnionAggregatesExpectedArgsAcrossMembers(t *testing.T) {
	arr := typ.NewArray(typ.Any)
	m := typ.NewMap(typ.String, typ.Any)
	fnArr := typ.Func().Param("t", arr).Returns(typ.Nil).Build()
	fnMap := typ.Func().Param("t", m).Returns(typ.Nil).Build()
	union := typ.NewUnion(fnArr, fnMap)

	ctx := db.NewQueryContext(db.New())
	infer := InferCall(ctx, CallDef{
		Callee: union,
		Args:   []typ.Type{typ.Unknown},
	})

	if infer.Kind != InferKindUnion {
		t.Fatalf("expected union infer kind, got %v", infer.Kind)
	}
	if len(infer.ExpectedArgs) != 1 {
		t.Fatalf("expected one aggregated expected arg, got %d", len(infer.ExpectedArgs))
	}
	expected, ok := infer.ExpectedArgs[0].(*typ.Union)
	if !ok {
		t.Fatalf("expected aggregated arg to be union, got %T (%v)", infer.ExpectedArgs[0], infer.ExpectedArgs[0])
	}
	hasArr := false
	hasMap := false
	for _, member := range expected.Members {
		if typ.TypeEquals(member, arr) {
			hasArr = true
		}
		if typ.TypeEquals(member, m) {
			hasMap = true
		}
	}
	if !hasArr || !hasMap {
		t.Fatalf("expected aggregated union to include array and map, got %v", infer.ExpectedArgs[0])
	}
}

func TestFinishCall_ShortCircuit(t *testing.T) {
	ctx := db.NewQueryContext(db.New())
	def := CallDef{
		Callee: typ.Any,
		Args:   []typ.Type{},
	}

	infer := InferCall(ctx, def)
	result := FinishCall(ctx, def, infer)

	if result.Type != typ.Any {
		t.Errorf("expected any, got %v", result.Type)
	}
}

func TestReInfer_NonGeneric(t *testing.T) {
	fn := typ.Func().
		Param("x", typ.Integer).
		Returns(typ.String).
		Build()

	ctx := db.NewQueryContext(db.New())
	def := CallDef{
		Callee: fn,
		Args:   []typ.Type{typ.Integer},
	}

	infer := InferCall(ctx, def)
	reInferred := ReInfer(ctx, def, infer)

	if reInferred.Kind != infer.Kind {
		t.Error("non-generic should return same result")
	}
}

func TestHasExplicitSelfSimple_RejectsTopLikeFirstParam(t *testing.T) {
	receiver := typ.NewRecord().Field("id", typ.String).Build()

	anyFirst := typ.Func().
		Param("options", typ.Any).
		Returns(typ.Boolean).
		Build()
	if hasExplicitSelfSimple(anyFirst, receiver) {
		t.Fatal("any first param should not be treated as explicit self")
	}

	unknownFirst := typ.Func().
		Param("value", typ.Unknown).
		Returns(typ.Boolean).
		Build()
	if hasExplicitSelfSimple(unknownFirst, receiver) {
		t.Fatal("unknown first param should not be treated as explicit self")
	}
}

func TestHasExplicitSelfSimple_StillAcceptsExplicitPatterns(t *testing.T) {
	receiver := typ.NewRecord().Field("id", typ.String).Build()

	namedSelf := typ.Func().
		Param("self", typ.Any).
		Returns(typ.Boolean).
		Build()
	if !hasExplicitSelfSimple(namedSelf, receiver) {
		t.Fatal("parameter named self should be treated as explicit self")
	}

	matchingType := typ.Func().
		Param("receiver", receiver).
		Returns(typ.Boolean).
		Build()
	if !hasExplicitSelfSimple(matchingType, receiver) {
		t.Fatal("receiver-compatible first param should be treated as explicit self")
	}
}

func TestHasExplicitSelfSimple_RejectsTopLikeReceiver(t *testing.T) {
	receiver := typ.Any
	optionsType := typ.NewRecord().
		Field("count", typ.NewOptional(typ.Number)).
		Build()

	optionsFirst := typ.Func().
		Param("options", typ.NewOptional(optionsType)).
		Returns(typ.Boolean).
		Build()
	if hasExplicitSelfSimple(optionsFirst, receiver) {
		t.Fatal("top-like receiver should not trigger explicit self inference")
	}

	tpOptions := &typ.TypeParam{Name: "T", Constraint: typ.NewOptional(optionsType)}
	genericFirst := typ.Func().
		Param("options", tpOptions).
		Returns(typ.Boolean).
		Build()
	if hasExplicitSelfSimple(genericFirst, receiver) {
		t.Fatal("top-like receiver should not trigger explicit self inference for constrained type params")
	}
}

func TestHasExplicitSelfSimple_AcceptsLiteralReceiverAgainstPrimitiveParam(t *testing.T) {
	receiver := typ.LiteralString("hello")
	fn := typ.Func().
		Param("s", typ.String).
		Param("start", typ.Integer).
		Returns(typ.String).
		Build()

	if !hasExplicitSelfSimple(fn, receiver) {
		t.Fatal("literal string receiver should match explicit primitive self param")
	}
}

func TestCallFunction_MethodOnLiteralReceiverConsumesSelf(t *testing.T) {
	fn := typ.Func().
		Param("s", typ.String).
		Param("start", typ.Integer).
		Param("finish", typ.Integer).
		Returns(typ.String).
		Build()

	ctx := db.NewQueryContext(db.New())
	result := callFunction(
		ctx,
		nil,
		fn,
		[]typ.Type{typ.Integer, typ.Integer},
		typ.LiteralString("abc"),
		true,
		false,
		nil,
	)

	if len(result.Errors) != 0 {
		t.Fatalf("expected no call errors, got: %v", result.Errors)
	}
	if result.Type != typ.String {
		t.Fatalf("expected string return type, got: %v", result.Type)
	}
}

func TestCallFunction_UnknownParamStillRequired(t *testing.T) {
	fn := typ.Func().
		Param("x", typ.Unknown).
		Returns(typ.Boolean).
		Build()

	ctx := db.NewQueryContext(db.New())
	result := callFunction(ctx, nil, fn, nil, nil, false, false, nil)

	if len(result.Errors) == 0 {
		t.Fatal("expected arity error for missing required unknown param")
	}
}

func TestCallFunction_RequiredAfterOptionalStillRequiresPosition(t *testing.T) {
	fn := typ.Func().
		OptParam("a", typ.Number).
		Param("b", typ.Number).
		Returns(typ.Number).
		Build()

	ctx := db.NewQueryContext(db.New())
	result := callFunction(ctx, nil, fn, []typ.Type{typ.Number}, nil, false, false, nil)

	if len(result.Errors) == 0 {
		t.Fatal("expected arity error when required param appears after optional")
	}
}

func TestCallFunction_MethodAlwaysConsumesReceiver(t *testing.T) {
	fn := typ.Func().
		Param("x", typ.Number).
		Returns(typ.Number).
		Build()

	ctx := db.NewQueryContext(db.New())
	result := callFunction(
		ctx,
		nil,
		fn,
		[]typ.Type{typ.Number},
		typ.String,
		true,
		true,
		nil,
	)

	if len(result.Errors) == 0 {
		t.Fatal("expected method call to fail when signature does not accept receiver")
	}
}

func TestCallFunction_ZeroParamAllowsExtraArgs(t *testing.T) {
	fn := typ.Func().
		Returns(typ.Boolean).
		Build()

	ctx := db.NewQueryContext(db.New())
	result := callFunction(ctx, nil, fn, []typ.Type{typ.Number, typ.String}, nil, false, false, nil)

	if len(result.Errors) != 0 {
		t.Fatalf("zero-param function should accept extra args, got: %v", result.Errors)
	}
	if result.Type != typ.Boolean {
		t.Fatalf("expected boolean return type, got: %v", result.Type)
	}
}
