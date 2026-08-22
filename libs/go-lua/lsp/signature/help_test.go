package signature

import (
	"testing"

	"github.com/wippyai/go-lua/lsp/index"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/typ"
)

func TestNewProvider(t *testing.T) {
	symbols := index.NewSymbolIndex()
	callGraph := index.NewCallGraph()

	provider := NewProvider(symbols, callGraph)

	if provider.symbols != symbols {
		t.Error("expected symbols to be set")
	}
	if provider.callGraph != callGraph {
		t.Error("expected callGraph to be set")
	}
}

func TestProvider_Help_NilCallGraph(t *testing.T) {
	provider := NewProvider(nil, nil)
	result := provider.Help("test.lua", 1, 1)
	if result != nil {
		t.Error("expected nil result for nil callGraph")
	}
}

func TestProvider_Help_NoCallAtPosition(t *testing.T) {
	symbols := index.NewSymbolIndex()
	callGraph := index.NewCallGraph()
	provider := NewProvider(symbols, callGraph)

	result := provider.Help("test.lua", 1, 1)
	if result != nil {
		t.Error("expected nil result when no call at position")
	}
}

func TestProvider_Help_FunctionNotFound(t *testing.T) {
	symbols := index.NewSymbolIndex()
	callGraph := index.NewCallGraph()

	callGraph.AddCall("test.lua", "main", diag.Span{StartLine: 1, EndLine: 1},
		"unknown.lua", "unknownFunc", diag.Span{}, diag.Span{StartLine: 5, StartCol: 1, EndLine: 5, EndCol: 15})

	provider := NewProvider(symbols, callGraph)
	result := provider.Help("test.lua", 5, 5)
	if result != nil {
		t.Error("expected nil result when function not found")
	}
}

func TestProvider_Help_SymbolNotFunction(t *testing.T) {
	symbols := index.NewSymbolIndex()
	callGraph := index.NewCallGraph()

	// Add a function symbol but with wrong type (edge case)
	symbols.AddDefinition("lib.lua", "notAFunc", index.SymbolFunction, typ.String,
		diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 10}, "")

	callGraph.AddCall("test.lua", "main", diag.Span{StartLine: 1, EndLine: 1},
		"lib.lua", "notAFunc", diag.Span{StartLine: 1, EndLine: 1}, diag.Span{StartLine: 5, StartCol: 1, EndLine: 5, EndCol: 15})

	provider := NewProvider(symbols, callGraph)
	result := provider.Help("test.lua", 5, 5)
	if result != nil {
		t.Error("expected nil result when symbol has wrong type")
	}
}

func TestProvider_Help_Success(t *testing.T) {
	symbols := index.NewSymbolIndex()
	callGraph := index.NewCallGraph()

	funcType := typ.Func().
		Param("count", typ.Number).
		Param("text", typ.String).
		Returns(typ.Boolean).
		Build()

	symbols.AddDefinition("lib.lua", "process", index.SymbolFunction, funcType,
		diag.Span{StartLine: 1, StartCol: 10, EndLine: 1, EndCol: 17}, "")

	callGraph.AddCall("test.lua", "main", diag.Span{StartLine: 1, EndLine: 1},
		"lib.lua", "process", diag.Span{StartLine: 1, EndLine: 1}, diag.Span{StartLine: 5, StartCol: 1, EndLine: 5, EndCol: 20})

	provider := NewProvider(symbols, callGraph)
	result := provider.Help("test.lua", 5, 10)

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Signatures) != 1 {
		t.Fatalf("expected 1 signature, got %d", len(result.Signatures))
	}
	if len(result.Signatures[0].Parameters) != 2 {
		t.Errorf("expected 2 parameters, got %d", len(result.Signatures[0].Parameters))
	}
	if result.Signatures[0].Parameters[0].Label != "count: number" {
		t.Errorf("unexpected param label: %s", result.Signatures[0].Parameters[0].Label)
	}
}

func TestProvider_Help_FunctionVariable(t *testing.T) {
	symbols := index.NewSymbolIndex()
	callGraph := index.NewCallGraph()

	funcType := typ.Func().
		Param("x", typ.Number).
		Returns(typ.Number).
		Build()

	symbols.AddDefinition("test.lua", "f", index.SymbolVariable, funcType,
		diag.Span{StartLine: 2, StartCol: 1, EndLine: 2, EndCol: 1}, "")

	callGraph.AddCall("test.lua", "main", diag.Span{StartLine: 1, EndLine: 1},
		"test.lua", "f", diag.Span{StartLine: 2, EndLine: 2}, diag.Span{StartLine: 5, StartCol: 1, EndLine: 5, EndCol: 5})

	provider := NewProvider(symbols, callGraph)
	result := provider.Help("test.lua", 5, 3)

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Signatures) != 1 {
		t.Fatalf("expected 1 signature, got %d", len(result.Signatures))
	}
	if len(result.Signatures[0].Parameters) != 1 {
		t.Errorf("expected 1 parameter, got %d", len(result.Signatures[0].Parameters))
	}
	if result.Signatures[0].Parameters[0].Label != "x: number" {
		t.Errorf("unexpected param label: %s", result.Signatures[0].Parameters[0].Label)
	}
}

func TestProvider_Help_VariadicFunction(t *testing.T) {
	symbols := index.NewSymbolIndex()
	callGraph := index.NewCallGraph()

	funcType := typ.Func().
		Param("format", typ.String).
		Variadic(typ.Number).
		Returns(typ.String).
		Build()

	symbols.AddDefinition("lib.lua", "printf", index.SymbolFunction, funcType,
		diag.Span{StartLine: 1, StartCol: 10, EndLine: 1, EndCol: 16}, "")

	callGraph.AddCall("test.lua", "main", diag.Span{},
		"lib.lua", "printf", diag.Span{StartLine: 1, EndLine: 1}, diag.Span{StartLine: 5, StartCol: 1, EndLine: 5, EndCol: 30})

	provider := NewProvider(symbols, callGraph)
	result := provider.Help("test.lua", 5, 15)

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Signatures[0].Parameters) != 2 {
		t.Errorf("expected 2 parameters (including variadic), got %d", len(result.Signatures[0].Parameters))
	}
	if result.Signatures[0].Parameters[1].Label != "...: number" {
		t.Errorf("unexpected variadic label: %s", result.Signatures[0].Parameters[1].Label)
	}
}

func TestProvider_Help_VariadicNoParams(t *testing.T) {
	symbols := index.NewSymbolIndex()
	callGraph := index.NewCallGraph()

	funcType := typ.Func().
		Variadic(typ.String).
		Build()

	symbols.AddDefinition("lib.lua", "print", index.SymbolFunction, funcType,
		diag.Span{StartLine: 1, StartCol: 10, EndLine: 1, EndCol: 15}, "")

	callGraph.AddCall("test.lua", "main", diag.Span{},
		"lib.lua", "print", diag.Span{StartLine: 1, EndLine: 1}, diag.Span{StartLine: 5, StartCol: 1, EndLine: 5, EndCol: 20})

	provider := NewProvider(symbols, callGraph)
	result := provider.Help("test.lua", 5, 10)

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Signatures[0].Parameters) != 1 {
		t.Errorf("expected 1 parameter (variadic only), got %d", len(result.Signatures[0].Parameters))
	}
	if result.Signatures[0].Parameters[0].Label != "...: string" {
		t.Errorf("unexpected variadic label: %s", result.Signatures[0].Parameters[0].Label)
	}
}

func TestFindActiveParameter_SingleParam(t *testing.T) {
	symbols := index.NewSymbolIndex()
	callGraph := index.NewCallGraph()
	provider := NewProvider(symbols, callGraph)

	edge := &index.CallEdge{
		CallSpan: diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 10},
	}
	funcType := typ.Func().Param("x", typ.Number).Build()

	active := provider.findActiveParameter(edge, 1, 5, funcType)
	if active != 0 {
		t.Errorf("expected active param 0, got %d", active)
	}
}

func TestFindActiveParameter_MultiLine(t *testing.T) {
	symbols := index.NewSymbolIndex()
	callGraph := index.NewCallGraph()
	provider := NewProvider(symbols, callGraph)

	edge := &index.CallEdge{
		CallSpan: diag.Span{StartLine: 1, StartCol: 1, EndLine: 4, EndCol: 2},
	}
	funcType := typ.Func().
		Param("a", typ.Number).
		Param("b", typ.String).
		Param("c", typ.Boolean).
		Build()

	// Line 2 (offset 1) -> param 1
	active := provider.findActiveParameter(edge, 2, 5, funcType)
	if active != 1 {
		t.Errorf("expected active param 1, got %d", active)
	}

	// Line 5 (offset 4) but only 3 params -> param 2 (last)
	active = provider.findActiveParameter(edge, 5, 5, funcType)
	if active != 2 {
		t.Errorf("expected active param 2 (last), got %d", active)
	}
}

func TestFindActiveParameter_SameLine(t *testing.T) {
	symbols := index.NewSymbolIndex()
	callGraph := index.NewCallGraph()
	provider := NewProvider(symbols, callGraph)

	edge := &index.CallEdge{
		CallSpan: diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 31},
	}
	funcType := typ.Func().
		Param("a", typ.Number).
		Param("b", typ.String).
		Param("c", typ.Boolean).
		Build()

	// Near start -> param 0
	active := provider.findActiveParameter(edge, 1, 5, funcType)
	if active != 0 {
		t.Errorf("expected active param 0, got %d", active)
	}

	// Middle -> param 1
	active = provider.findActiveParameter(edge, 1, 15, funcType)
	if active != 1 {
		t.Errorf("expected active param 1, got %d", active)
	}

	// Near end -> param 2
	active = provider.findActiveParameter(edge, 1, 28, funcType)
	if active != 2 {
		t.Errorf("expected active param 2, got %d", active)
	}

	// Past end -> param 2 (clamped to last)
	active = provider.findActiveParameter(edge, 1, 50, funcType)
	if active != 2 {
		t.Errorf("expected active param 2 (clamped), got %d", active)
	}
}

func TestFindActiveParameter_ZeroWidth(t *testing.T) {
	symbols := index.NewSymbolIndex()
	callGraph := index.NewCallGraph()
	provider := NewProvider(symbols, callGraph)

	edge := &index.CallEdge{
		CallSpan: diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 1},
	}
	funcType := typ.Func().
		Param("a", typ.Number).
		Param("b", typ.String).
		Build()

	active := provider.findActiveParameter(edge, 1, 1, funcType)
	if active != 0 {
		t.Errorf("expected active param 0 for zero-width, got %d", active)
	}
}

func TestFindActiveParameter_CursorBeforeCall(t *testing.T) {
	symbols := index.NewSymbolIndex()
	callGraph := index.NewCallGraph()
	provider := NewProvider(symbols, callGraph)

	edge := &index.CallEdge{
		CallSpan: diag.Span{StartLine: 1, StartCol: 10, EndLine: 1, EndCol: 30},
	}
	funcType := typ.Func().
		Param("a", typ.Number).
		Param("b", typ.String).
		Build()

	active := provider.findActiveParameter(edge, 1, 5, funcType)
	if active != 0 {
		t.Errorf("expected active param 0 for cursor before call, got %d", active)
	}
}

func TestFindActiveParameter_SmallCallWidth(t *testing.T) {
	symbols := index.NewSymbolIndex()
	callGraph := index.NewCallGraph()
	provider := NewProvider(symbols, callGraph)

	// Very small call span with many params - paramWidth will be 0
	edge := &index.CallEdge{
		CallSpan: diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 3},
	}
	funcType := typ.Func().
		Param("a", typ.Number).
		Param("b", typ.String).
		Param("c", typ.Boolean).
		Param("d", typ.Number).
		Build()

	active := provider.findActiveParameter(edge, 1, 2, funcType)
	if active != 0 {
		t.Errorf("expected active param 0 for small width, got %d", active)
	}
}

func TestBuildSignatureInfo(t *testing.T) {
	funcType := typ.Func().
		Param("count", typ.Number).
		Param("text", typ.String).
		Returns(typ.Boolean).
		Build()

	sig := buildSignatureInfo("myFunc", funcType)

	if sig.Label == "" {
		t.Error("expected non-empty label")
	}
	if len(sig.Parameters) != 2 {
		t.Fatalf("expected 2 parameters, got %d", len(sig.Parameters))
	}
	if sig.Parameters[0].Label != "count: number" {
		t.Errorf("unexpected first param label: %s", sig.Parameters[0].Label)
	}
	if sig.Parameters[1].Label != "text: string" {
		t.Errorf("unexpected second param label: %s", sig.Parameters[1].Label)
	}
}

func TestTypeString(t *testing.T) {
	if typeString(nil) != "any" {
		t.Error("expected 'any' for nil type")
	}
	if typeString(typ.Number) != "number" {
		t.Errorf("expected 'number', got '%s'", typeString(typ.Number))
	}
}

func TestFindFunctionSymbol_NilSymbols(t *testing.T) {
	provider := NewProvider(nil, index.NewCallGraph())
	result := provider.findFunctionSymbol("", "anyFunc")
	if result != nil {
		t.Error("expected nil for nil symbols")
	}
}

func TestFindFunctionSymbol_NotFound(t *testing.T) {
	symbols := index.NewSymbolIndex()
	provider := NewProvider(symbols, index.NewCallGraph())
	result := provider.findFunctionSymbol("", "nonexistent")
	if result != nil {
		t.Error("expected nil for non-existent function")
	}
}

func TestFindFunctionSymbol_FoundVariable(t *testing.T) {
	symbols := index.NewSymbolIndex()
	symbols.AddDefinition("test.lua", "myVar", index.SymbolVariable, nil,
		diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 5}, "")

	provider := NewProvider(symbols, index.NewCallGraph())
	result := provider.findFunctionSymbol("test.lua", "myVar")
	if result != nil {
		t.Error("expected nil when only variable exists, not function")
	}
}

func TestFormatFullSignature(t *testing.T) {
	funcType := typ.Func().
		Param("x", typ.Number).
		Returns(typ.String).
		Build()

	sig := formatFullSignature("test", funcType)
	// Function.String() produces the signature representation
	if sig == "" {
		t.Error("expected non-empty signature")
	}
}
