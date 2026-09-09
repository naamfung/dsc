package core

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

func TestTryDiscriminatedUnionMember_SingleMatch(t *testing.T) {
	successType := typ.NewRecord().
		Field("kind", typ.LiteralString("success")).
		Field("value", typ.String).
		Build()
	errorType := typ.NewRecord().
		Field("kind", typ.LiteralString("error")).
		Field("message", typ.String).
		Build()
	unionType := typ.NewUnion(successType, errorType)

	table := &ast.TableExpr{
		Fields: []*ast.Field{
			{
				Key:   &ast.StringExpr{Value: "kind"},
				Value: &ast.StringExpr{Value: "success"},
			},
			{
				Key:   &ast.StringExpr{Value: "value"},
				Value: &ast.StringExpr{Value: "hello"},
			},
		},
	}

	match := TryDiscriminatedUnionMember(table, unionType)
	if match == nil {
		t.Fatal("expected to find matching member")
	}
	if match.MemberIndex < 0 {
		t.Errorf("expected valid index, got %d", match.MemberIndex)
	}
	rec := match.Member.(*typ.Record)
	kindField := rec.GetField("kind")
	if kindField == nil {
		t.Fatal("expected kind field")
	}
	lit, ok := kindField.Type.(*typ.Literal)
	if !ok || lit.Value != "success" {
		t.Errorf("expected kind='success', got %v", kindField.Type)
	}
}

func TestTryDiscriminatedUnionMember_ErrorVariant(t *testing.T) {
	successType := typ.NewRecord().
		Field("kind", typ.LiteralString("success")).
		Field("value", typ.String).
		Build()
	errorType := typ.NewRecord().
		Field("kind", typ.LiteralString("error")).
		Field("message", typ.String).
		Build()
	unionType := typ.NewUnion(successType, errorType)

	table := &ast.TableExpr{
		Fields: []*ast.Field{
			{
				Key:   &ast.StringExpr{Value: "kind"},
				Value: &ast.StringExpr{Value: "error"},
			},
			{
				Key:   &ast.StringExpr{Value: "message"},
				Value: &ast.StringExpr{Value: "failed"},
			},
		},
	}

	match := TryDiscriminatedUnionMember(table, unionType)
	if match == nil {
		t.Fatal("expected to find matching member")
	}
	if match.MemberIndex < 0 {
		t.Errorf("expected valid index, got %d", match.MemberIndex)
	}
	rec := match.Member.(*typ.Record)
	kindField := rec.GetField("kind")
	if kindField == nil {
		t.Fatal("expected kind field")
	}
	lit, ok := kindField.Type.(*typ.Literal)
	if !ok || lit.Value != "error" {
		t.Errorf("expected kind='error', got %v", kindField.Type)
	}
}

func TestTryDiscriminatedUnionMember_NoMatchPending(t *testing.T) {
	successType := typ.NewRecord().
		Field("kind", typ.LiteralString("success")).
		Build()
	errorType := typ.NewRecord().
		Field("kind", typ.LiteralString("error")).
		Build()
	unionType := typ.NewUnion(successType, errorType)

	table := &ast.TableExpr{
		Fields: []*ast.Field{
			{
				Key:   &ast.StringExpr{Value: "kind"},
				Value: &ast.StringExpr{Value: "pending"},
			},
		},
	}

	match := TryDiscriminatedUnionMember(table, unionType)
	if match != nil {
		t.Errorf("expected no match, got %v", match.Member)
	}
}

func TestTryDiscriminatedUnionMember_NoLiteralFields(t *testing.T) {
	successType := typ.NewRecord().
		Field("kind", typ.LiteralString("success")).
		Build()
	unionType := typ.NewUnion(successType)

	table := &ast.TableExpr{
		Fields: []*ast.Field{
			{
				Key:   &ast.StringExpr{Value: "value"},
				Value: &ast.NumberExpr{Value: "42"},
			},
		},
	}

	match := TryDiscriminatedUnionMember(table, unionType)
	if match != nil {
		t.Errorf("expected no match for table without literal string fields")
	}
}

func TestCompatibleFunctionFromUnion_SingleMatch(t *testing.T) {
	fn1 := typ.Func().Param("x", typ.String).Returns(typ.Number).Build()
	fn2 := typ.Func().Param("x", typ.Number).Param("y", typ.Number).Returns(typ.String).Build()
	unionType := typ.NewUnion(fn1, fn2)

	compatible := CompatibleFunctionFromUnion(1, unionType)
	if compatible == nil {
		t.Fatal("expected compatible function")
	}
	if len(compatible.Params) != 1 {
		t.Errorf("expected 1 param, got %d", len(compatible.Params))
	}
}

func TestCompatibleFunctionFromUnion_MergeWhenMultipleMatch(t *testing.T) {
	fn1 := typ.Func().Param("x", typ.String).Returns(typ.Number).Build()
	fn2 := typ.Func().Param("x", typ.Number).Returns(typ.String).Build()
	unionType := typ.NewUnion(fn1, fn2)

	compatible := CompatibleFunctionFromUnion(1, unionType)
	if compatible == nil {
		t.Fatal("expected compatible function")
	}
	if len(compatible.Params) != 1 {
		t.Fatalf("expected 1 param, got %d", len(compatible.Params))
	}
	paramType := compatible.Params[0].Type
	if _, ok := paramType.(*typ.Union); !ok {
		t.Errorf("expected union param type, got %T", paramType)
	}
}

func TestTryDiscriminatedUnionMember_MultipleDiscriminants(t *testing.T) {
	type1 := typ.NewRecord().
		Field("type", typ.LiteralString("a")).
		Field("status", typ.LiteralString("ok")).
		Field("value", typ.String).
		Build()
	type2 := typ.NewRecord().
		Field("type", typ.LiteralString("a")).
		Field("status", typ.LiteralString("err")).
		Field("error", typ.String).
		Build()
	type3 := typ.NewRecord().
		Field("type", typ.LiteralString("b")).
		Field("status", typ.LiteralString("ok")).
		Field("data", typ.Number).
		Build()
	unionType := typ.NewUnion(type1, type2, type3)

	table := &ast.TableExpr{
		Fields: []*ast.Field{
			{Key: &ast.StringExpr{Value: "type"}, Value: &ast.StringExpr{Value: "a"}},
			{Key: &ast.StringExpr{Value: "status"}, Value: &ast.StringExpr{Value: "err"}},
			{Key: &ast.StringExpr{Value: "error"}, Value: &ast.StringExpr{Value: "failed"}},
		},
	}

	match := TryDiscriminatedUnionMember(table, unionType)
	if match == nil {
		t.Fatal("expected to find matching member")
	}
	if match.MemberIndex < 0 {
		t.Errorf("expected valid index, got %d", match.MemberIndex)
	}
	rec := match.Member.(*typ.Record)
	if rec.GetField("error") == nil {
		t.Error("expected error field in matched member")
	}
}

func TestTryDiscriminatedUnionMember_AmbiguousMatch(t *testing.T) {
	type1 := typ.NewRecord().
		Field("kind", typ.LiteralString("item")).
		Field("value", typ.String).
		Build()
	type2 := typ.NewRecord().
		Field("kind", typ.LiteralString("item")).
		Field("count", typ.Number).
		Build()
	unionType := typ.NewUnion(type1, type2)

	table := &ast.TableExpr{
		Fields: []*ast.Field{
			{Key: &ast.StringExpr{Value: "kind"}, Value: &ast.StringExpr{Value: "item"}},
		},
	}

	match := TryDiscriminatedUnionMember(table, unionType)
	if match != nil {
		t.Error("expected no match for ambiguous discriminant")
	}
}

func TestTryDiscriminatedUnionMember_IdentKey(t *testing.T) {
	type1 := typ.NewRecord().
		Field("kind", typ.LiteralString("test")).
		Field("value", typ.String).
		Build()
	type2 := typ.NewRecord().
		Field("kind", typ.LiteralString("other")).
		Field("data", typ.Number).
		Build()
	unionType := typ.NewUnion(type1, type2)

	table := &ast.TableExpr{
		Fields: []*ast.Field{
			{Key: &ast.IdentExpr{Value: "kind"}, Value: &ast.StringExpr{Value: "test"}},
		},
	}

	match := TryDiscriminatedUnionMember(table, unionType)
	if match == nil {
		t.Fatal("expected match with ident key")
	}
	rec := match.Member.(*typ.Record)
	if rec.GetField("value") == nil {
		t.Error("expected value field in matched member")
	}
}

func TestCompatibleFunctionFromUnion_NoMatch(t *testing.T) {
	unionType := typ.NewUnion(typ.String, typ.Number)

	compatible := CompatibleFunctionFromUnion(1, unionType)
	if compatible != nil {
		t.Error("expected nil for union with no function types")
	}
}

func TestCompatibleFunctionFromUnion_WithVariadic(t *testing.T) {
	fn1 := typ.Func().Param("x", typ.String).Variadic(typ.Number).Returns(typ.String).Build()
	fn2 := typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()
	unionType := typ.NewUnion(fn1, fn2)

	compatible := CompatibleFunctionFromUnion(1, unionType)
	if compatible == nil {
		t.Fatal("expected compatible function")
	}
}

func TestCompatibleFunctionFromUnion_ArityUsesVariadicCompatibility(t *testing.T) {
	variadic := typ.Func().Param("x", typ.String).Variadic(typ.Number).Returns(typ.String).Build()
	fixed := typ.Func().Param("x", typ.Boolean).Param("y", typ.Boolean).Returns(typ.Boolean).Build()
	unionType := typ.NewUnion(variadic, fixed)

	compatible := CompatibleFunctionFromUnion(3, unionType)
	if compatible == nil {
		t.Fatal("expected variadic function to be selected")
	}
	if compatible.Variadic == nil {
		t.Fatalf("expected variadic compatible function, got %v", compatible)
	}
	if len(compatible.Params) != 1 || !typ.TypeEquals(compatible.Params[0].Type, typ.String) {
		t.Fatalf("expected variadic signature to be preserved, got %v", compatible)
	}
}

func TestCompatibleFunctionFromUnion_ArityUsesOptionalCompatibility(t *testing.T) {
	oneArg := typ.Func().Param("x", typ.String).Returns(typ.Number).Build()
	optionalSecond := typ.Func().Param("x", typ.Number).OptParam("y", typ.Number).Returns(typ.String).Build()
	unionType := typ.NewUnion(oneArg, optionalSecond)

	compatible := CompatibleFunctionFromUnion(1, unionType)
	if compatible == nil {
		t.Fatal("expected compatible function for optional-arity match")
	}
	if len(compatible.Params) != 2 {
		t.Fatalf("expected merged signature with optional second param, got %v", compatible)
	}
	if !compatible.Params[1].Optional {
		t.Fatalf("expected second param to stay optional, got %v", compatible)
	}
}

func TestCompatibleFunctionFromUnion_NoArityCompatibleFunction(t *testing.T) {
	fn1 := typ.Func().Param("x", typ.String).Returns(typ.Number).Build()
	fn2 := typ.Func().Param("x", typ.Number).Param("y", typ.Number).Returns(typ.String).Build()
	unionType := typ.NewUnion(fn1, fn2)

	compatible := CompatibleFunctionFromUnion(3, unionType)
	if compatible != nil {
		t.Fatalf("expected nil when no overload accepts arity 3, got %v", compatible)
	}
}

func TestIsLiteralStringType(t *testing.T) {
	if !unwrap.IsLiteralString(typ.LiteralString("test")) {
		t.Error("expected true for literal string")
	}
	if unwrap.IsLiteralString(typ.String) {
		t.Error("expected false for string type")
	}
	if unwrap.IsLiteralString(typ.LiteralInt(42)) {
		t.Error("expected false for literal int")
	}
	if unwrap.IsLiteralString(typ.Number) {
		t.Error("expected false for number type")
	}
}
