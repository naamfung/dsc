package cond

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/resolve"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestBranchConditions_ZeroValue(t *testing.T) {
	var bc BranchConditions
	if bc.OnTrue.HasConstraints() {
		t.Error("zero value OnTrue should have no constraints")
	}
	if bc.OnFalse.HasConstraints() {
		t.Error("zero value OnFalse should have no constraints")
	}
}

func TestConditionExtractor_ZeroValue(t *testing.T) {
	var ce ConditionExtractor
	if ce.P != 0 {
		t.Error("zero value P should be 0")
	}
	if ce.SC != nil {
		t.Error("zero value SC should be nil")
	}
	if ce.Inputs != nil {
		t.Error("zero value Inputs should be nil")
	}
}

func TestConditionExtractor_ConstraintsFromBranch_NilInfo(t *testing.T) {
	ce := &ConditionExtractor{}
	result := ce.ConstraintsFromBranch(nil)
	if result.OnTrue.HasConstraints() {
		t.Error("nil info should produce no OnTrue constraints")
	}
	if result.OnFalse.HasConstraints() {
		t.Error("nil info should produce no OnFalse constraints")
	}
}

func TestConditionExtractor_ConstraintsFromConditionExpr_NilExpr(t *testing.T) {
	ce := &ConditionExtractor{}
	result := ce.ConstraintsFromConditionExpr(nil)
	if !result.OnTrue.IsTrue() {
		t.Error("nil expr should produce TrueCondition for OnTrue")
	}
	if !result.OnFalse.IsTrue() {
		t.Error("nil expr should produce TrueCondition for OnFalse")
	}
}

func TestConditionExtractor_ConditionFromExpr_NilExpr(t *testing.T) {
	ce := &ConditionExtractor{}
	result := ce.ConditionFromExpr(nil)
	if !result.IsTrue() {
		t.Error("nil expr should produce TrueCondition")
	}
}

func TestConditionExtractor_ConditionFromExpr_TrueExpr(t *testing.T) {
	ce := &ConditionExtractor{}
	result := ce.ConditionFromExpr(&ast.TrueExpr{})
	if !result.IsTrue() {
		t.Error("TrueExpr should produce TrueCondition")
	}
}

func TestConditionExtractor_ConditionFromExpr_FalseExpr(t *testing.T) {
	ce := &ConditionExtractor{}
	result := ce.ConditionFromExpr(&ast.FalseExpr{})
	if !result.IsFalse() {
		t.Error("FalseExpr should produce FalseCondition")
	}
}

func TestConditionExtractor_ConditionFromEquality_BothNil(t *testing.T) {
	ce := &ConditionExtractor{}
	result := ce.ConditionFromEquality(nil, nil)
	if !result.IsTrue() {
		t.Error("nil operands should produce TrueCondition")
	}
}

func TestConditionExtractor_ConditionFromInequality_BothNil(t *testing.T) {
	ce := &ConditionExtractor{}
	result := ce.ConditionFromInequality(nil, nil)
	if !result.IsTrue() {
		t.Error("nil operands should produce TrueCondition")
	}
}

func TestResolveClassifyReturnExpr_NilExpr(t *testing.T) {
	result := resolve.ClassifyReturnExpr(nil)
	if result != flow.ReturnUnknown {
		t.Errorf("nil expr should return ReturnUnknown, got %v", result)
	}
}

func TestResolveClassifyReturnExpr_TrueExpr(t *testing.T) {
	result := resolve.ClassifyReturnExpr(&ast.TrueExpr{})
	if result != flow.ReturnTrue {
		t.Errorf("TrueExpr should return ReturnTrue, got %v", result)
	}
}

func TestResolveClassifyReturnExpr_FalseExpr(t *testing.T) {
	result := resolve.ClassifyReturnExpr(&ast.FalseExpr{})
	if result != flow.ReturnFalse {
		t.Errorf("FalseExpr should return ReturnFalse, got %v", result)
	}
}

func TestNumericConstraintsFromExpr_NilExpr(t *testing.T) {
	result := NumericConstraintsFromExpr(nil, 0, nil)
	if len(result) != 0 {
		t.Error("nil expr should return empty slice")
	}
}

func TestExtractReturnExprConstraints_NilExpr(t *testing.T) {
	result := ExtractReturnExprConstraints(nil, 0, nil, nil, nil, nil, nil, nil)
	if result.OnTrue.HasConstraints() {
		t.Error("nil expr should produce no OnTrue constraints")
	}
	if result.OnFalse.HasConstraints() {
		t.Error("nil expr should produce no OnFalse constraints")
	}
}

func TestExtractReturnExprConstraints_IdentExpr(t *testing.T) {
	expr := &ast.IdentExpr{Value: "x"}
	result := ExtractReturnExprConstraints(expr, 0, nil, nil, nil, nil, nil, nil)
	if result.OnTrue.HasConstraints() {
		t.Error("simple ident should produce no OnTrue constraints")
	}
}

func TestChannelValueConstraint_NilBindings(t *testing.T) {
	result := ChannelValueConstraint(constraint.Path{}, nil, 0, nil, nil, nil)
	if result != nil {
		t.Error("nil bindings should return nil")
	}
}

func TestExtractChannelElementType_NilType(t *testing.T) {
	result := ExtractChannelElementType(nil)
	if result != nil {
		t.Error("nil type should return nil")
	}
}

func TestExtractChannelElementType_NonChannel(t *testing.T) {
	result := ExtractChannelElementType(typ.String)
	if result != nil {
		t.Error("non-channel type should return nil")
	}
}

func TestEmitIndexEqualsLiteral_NilValue(t *testing.T) {
	result := EmitIndexEqualsLiteral(constraint.Path{Root: "x"}, typ.String, nil)
	if len(result) != 1 {
		t.Error("should return single constraint even with nil value")
	}
}

func TestEmitIndexEqualsLiteral_LiteralKey(t *testing.T) {
	key := typ.LiteralString("key")
	value := typ.LiteralString("value")
	result := EmitIndexEqualsLiteral(constraint.Path{Root: "x"}, key, value)
	if len(result) != 1 {
		t.Errorf("literal key should produce 1 constraint, got %d", len(result))
	}
}

func TestEmitIndexEqualsLiteral_UnionKey(t *testing.T) {
	key := typ.NewUnion(typ.LiteralString("a"), typ.LiteralString("b"))
	value := typ.LiteralString("value")
	result := EmitIndexEqualsLiteral(constraint.Path{Root: "x"}, key, value)
	if len(result) != 2 {
		t.Errorf("union key with 2 literals should produce 2 constraints, got %d", len(result))
	}
}

func TestEmitIndexEqualsPath_LiteralKey_Equals(t *testing.T) {
	key := typ.LiteralString("key")
	target := constraint.Path{Root: "x"}
	value := constraint.Path{Root: "y"}
	result := EmitIndexEqualsPath(target, key, value, true)
	if len(result) != 1 {
		t.Errorf("literal key equals should produce 1 constraint, got %d", len(result))
	}
}

func TestEmitIndexEqualsPath_LiteralKey_NotEquals(t *testing.T) {
	key := typ.LiteralString("key")
	target := constraint.Path{Root: "x"}
	value := constraint.Path{Root: "y"}
	result := EmitIndexEqualsPath(target, key, value, false)
	if len(result) != 1 {
		t.Errorf("literal key not equals should produce 1 constraint, got %d", len(result))
	}
}

func TestTypeKeyFromStringExpr(t *testing.T) {
	expr := &ast.StringExpr{Value: "string"}
	key, ok := typeKeyFromStringExpr(expr)
	if !ok {
		t.Error("string expr should return ok=true")
	}
	expected := narrow.BuiltinTypeKey("string")
	if key != expected {
		t.Errorf("expected %v, got %v", expected, key)
	}
}

func TestTypeKeyFromStringExpr_NonString(t *testing.T) {
	expr := &ast.NumberExpr{Value: "42"}
	_, ok := typeKeyFromStringExpr(expr)
	if ok {
		t.Error("number expr should return ok=false")
	}
}

func TestTypeKeyFromStringExpr_UnknownBuiltin(t *testing.T) {
	expr := &ast.StringExpr{Value: "entry"}
	_, ok := typeKeyFromStringExpr(expr)
	if ok {
		t.Error("unknown type() string should return ok=false")
	}
}

func TestConditionFromEquality_PathLiteralRegistersTypeKey(t *testing.T) {
	inputs := &flow.Inputs{TypeKeys: make(map[uint64]typ.Type)}
	ce := &ConditionExtractor{Inputs: inputs}
	cond := ce.ConditionFromEquality(&ast.IdentExpr{Value: "x"}, &ast.StringExpr{Value: "a"})
	if !cond.HasConstraints() {
		t.Fatal("expected constraints for x == \"a\"")
	}
	items := cond.MustConstraints()
	if len(items) != 1 {
		t.Fatalf("expected exactly one constraint, got %d", len(items))
	}
	hasType, ok := items[0].(constraint.HasType)
	if !ok {
		t.Fatalf("expected HasType constraint, got %T", items[0])
	}
	if hasType.Type.Kind != narrow.TypeKeyHash {
		t.Fatalf("expected hash type key, got %v", hasType.Type.Kind)
	}
	resolved := inputs.TypeKeys[hasType.Type.Hash]
	if !typ.TypeEquals(resolved, typ.LiteralString("a")) {
		t.Fatalf("expected registered literal type \"a\", got %v", resolved)
	}
}

func TestConditionFromInequality_PathLiteralUsesNotHasType(t *testing.T) {
	inputs := &flow.Inputs{TypeKeys: make(map[uint64]typ.Type)}
	ce := &ConditionExtractor{Inputs: inputs}
	cond := ce.ConditionFromInequality(&ast.IdentExpr{Value: "x"}, &ast.NumberExpr{Value: "1"})
	if !cond.HasConstraints() {
		t.Fatal("expected constraints for x ~= 1")
	}
	items := cond.MustConstraints()
	if len(items) != 1 {
		t.Fatalf("expected exactly one constraint, got %d", len(items))
	}
	notHasType, ok := items[0].(constraint.NotHasType)
	if !ok {
		t.Fatalf("expected NotHasType constraint, got %T", items[0])
	}
	if notHasType.Type.Kind != narrow.TypeKeyHash {
		t.Fatalf("expected hash type key, got %v", notHasType.Type.Kind)
	}
	resolved := inputs.TypeKeys[notHasType.Type.Hash]
	if !typ.TypeEquals(resolved, typ.LiteralInt(1)) {
		t.Fatalf("expected registered literal type 1, got %v", resolved)
	}
}

func TestConditionFromEquality_PathFalseLiteralUsesPortableConstraints(t *testing.T) {
	ce := &ConditionExtractor{}
	cond := ce.ConditionFromEquality(&ast.IdentExpr{Value: "x"}, &ast.FalseExpr{})
	if !cond.HasConstraints() {
		t.Fatal("expected constraints for x == false")
	}
	items := cond.MustConstraints()
	if len(items) != 2 {
		t.Fatalf("expected two constraints, got %d", len(items))
	}
	hasFalsy := false
	hasNotNil := false
	for _, c := range items {
		if _, ok := c.(constraint.Falsy); ok {
			hasFalsy = true
		}
		if _, ok := c.(constraint.NotNil); ok {
			hasNotNil = true
		}
	}
	if !hasFalsy || !hasNotNil {
		t.Fatalf("expected Falsy+NotNil, got %v", items)
	}
}

func TestConditionFromEquality_PathTrueLiteralUsesPortableConstraints(t *testing.T) {
	ce := &ConditionExtractor{}
	cond := ce.ConditionFromEquality(&ast.IdentExpr{Value: "x"}, &ast.TrueExpr{})
	if !cond.HasConstraints() {
		t.Fatal("expected constraints for x == true")
	}
	items := cond.MustConstraints()
	if len(items) != 2 {
		t.Fatalf("expected two constraints, got %d", len(items))
	}
	hasTruthy := false
	hasBoolType := false
	for _, c := range items {
		if _, ok := c.(constraint.Truthy); ok {
			hasTruthy = true
		}
		if hasType, ok := c.(constraint.HasType); ok && hasType.Type == narrow.BuiltinTypeKey("boolean") {
			hasBoolType = true
		}
	}
	if !hasTruthy || !hasBoolType {
		t.Fatalf("expected Truthy+HasType(boolean), got %v", items)
	}
}
