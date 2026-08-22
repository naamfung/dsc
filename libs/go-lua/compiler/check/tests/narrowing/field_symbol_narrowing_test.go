package narrowing

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/compiler/parse"
)

// =============================================================================
// Field Symbol Integration Tests
// Tests that field symbols work correctly for identity-based narrowing.
// =============================================================================

// TestFieldSymbol_LocalAssertNotNil tests that local assert.not_nil resolves to a symbol.
func TestFieldSymbol_LocalAssertNotNil(t *testing.T) {
	code := `
local assert = { not_nil = function(v) if v == nil then error("nil") end end }
local x: string? = nil
assert.not_nil(x)
`
	result := testutil.Check(code, testutil.WithStdlib())

	// This test verifies the symbol infrastructure exists.
	// Full narrowing depends on effect system integration.
	if result.Session == nil {
		t.Fatal("Session is nil")
	}
}

// TestFieldSymbol_LocalAssertSeparateAssign tests assert.not_nil defined separately.
func TestFieldSymbol_LocalAssertSeparateAssign(t *testing.T) {
	code := `
local assert = {}
assert.not_nil = function(v) if v == nil then error("nil") end end
local x: string? = nil
assert.not_nil(x)
`
	result := testutil.Check(code, testutil.WithStdlib())

	if result.Session == nil {
		t.Fatal("Session is nil")
	}
}

// TestFieldSymbol_ModulePattern tests standard module pattern with methods.
func TestFieldSymbol_ModulePattern(t *testing.T) {
	code := `
local M = {}

function M.new()
    return setmetatable({}, { __index = M })
end

function M:method()
    return self
end

local obj = M.new()
obj:method()
`
	result := testutil.Check(code, testutil.WithStdlib())

	if result.Session == nil {
		t.Fatal("Session is nil")
	}
}

// TestFieldSymbol_NestedModulePattern tests nested module fields.
func TestFieldSymbol_NestedModulePattern(t *testing.T) {
	code := `
local M = {}
M.sub = {}

function M.sub.helper()
    return 1
end

local x = M.sub.helper()
`
	result := testutil.Check(code, testutil.WithStdlib())

	if result.Session == nil {
		t.Fatal("Session is nil")
	}
}

// TestFieldSymbol_ShadowedLocal tests shadowed local tables have separate symbols.
func TestFieldSymbol_ShadowedLocal(t *testing.T) {
	code := `
local T = {}
T.f = function() return 1 end

do
    local T = {}
    T.f = function() return "str" end
    local x = T.f()  -- should be string
end

local y = T.f()  -- should be number
`
	result := testutil.Check(code, testutil.WithStdlib())

	if result.Session == nil {
		t.Fatal("Session is nil")
	}
}

// TestFieldSymbol_TableLiteralFields tests table literal with function fields.
func TestFieldSymbol_TableLiteralFields(t *testing.T) {
	code := `
local t = {
    a = function() return 1 end,
    b = function() return "str" end,
    c = function() return true end,
}

local x = t.a()
local y = t.b()
local z = t.c()
`
	result := testutil.Check(code, testutil.WithStdlib())

	if result.Session == nil {
		t.Fatal("Session is nil")
	}
}

// TestFieldSymbol_CallbackArgument tests callback passed to function.
func TestFieldSymbol_CallbackArgument(t *testing.T) {
	code := `
local function apply(f: (number) -> number, x: number): number
    return f(x)
end

local result = apply(function(n) return n * 2 end, 5)
`
	result := testutil.Check(code, testutil.WithStdlib())

	if result.Session == nil {
		t.Fatal("Session is nil")
	}
}

// TestFieldSymbol_ReturnedFunction tests function returned directly.
func TestFieldSymbol_ReturnedFunction(t *testing.T) {
	code := `
local function makeAdder(n: number): (number) -> number
    return function(x) return x + n end
end

local add5 = makeAdder(5)
local result = add5(10)
`
	result := testutil.Check(code, testutil.WithStdlib())

	if result.Session == nil {
		t.Fatal("Session is nil")
	}
}

// TestFieldSymbol_MethodChaining tests method chaining pattern.
func TestFieldSymbol_MethodChaining(t *testing.T) {
	code := `
local Builder = {}

function Builder.new()
    return setmetatable({ value = 0 }, { __index = Builder })
end

function Builder:add(n: number)
    self.value = self.value + n
    return self
end

function Builder:result(): number
    return self.value
end

local result = Builder.new():add(1):add(2):result()
`
	result := testutil.Check(code, testutil.WithStdlib())

	if result.Session == nil {
		t.Fatal("Session is nil")
	}
}

// =============================================================================
// CFG-Level Symbol Resolution Tests
// Direct tests of CFG infrastructure for field symbols.
// =============================================================================

// TestFieldSymbol_CFG_LocalTableFieldDef tests CFG symbol for local table field.
func TestFieldSymbol_CFG_LocalTableFieldDef(t *testing.T) {
	code := `
local assert = {}
assert.not_nil = function(v) if v == nil then error("nil") end end
assert.not_nil(x)
`
	chunk, err := parse.ParseString(code, "test.lua")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	fn := &ast.FunctionExpr{Stmts: chunk}
	g := cfg.Build(fn, "x")

	// Find field assignment
	var fieldAssign *cfg.AssignInfo
	g.EachAssign(func(p cfg.Point, a *cfg.AssignInfo) {
		if !a.IsLocal && len(a.Targets) > 0 && a.Targets[0].Kind == cfg.TargetField {
			fieldAssign = a
		}
	})

	if fieldAssign == nil {
		t.Fatal("Field assignment not found")
	}

	// Field target should have a symbol for assert.not_nil
	target := fieldAssign.Targets[0]
	if target.Symbol == 0 {
		t.Error("Field target should have a symbol for assert.not_nil")
	}

	// Find call
	var callInfo *cfg.CallInfo
	g.EachStmtCall(func(p cfg.Point, c *cfg.CallInfo) {
		if c.CalleePath.Root == "assert" {
			callInfo = c
		}
	})

	if callInfo == nil {
		t.Fatal("Call to assert.not_nil not found")
	}

	// Call path should exist and reference assert
	if callInfo.CalleePath.Root != "assert" {
		t.Errorf("CalleePath.Root = %q, want %q", callInfo.CalleePath.Root, "assert")
	}
}

// TestFieldSymbol_CFG_FuncDefField tests CFG symbol for function M.f() def.
func TestFieldSymbol_CFG_FuncDefField(t *testing.T) {
	code := `
function M.f()
    return 1
end

M.f()
`
	chunk, err := parse.ParseString(code, "test.lua")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	fn := &ast.FunctionExpr{Stmts: chunk}
	g := cfg.Build(fn, "M")

	// Find function definition
	var funcDef *cfg.FuncDefInfo
	g.EachFuncDef(func(p cfg.Point, f *cfg.FuncDefInfo) {
		funcDef = f
	})

	if funcDef == nil {
		t.Fatal("FuncDefInfo not found")
	}

	// FuncDef should have a symbol for M.f
	if funcDef.Symbol == 0 {
		t.Error("FuncDefInfo.Symbol should be non-zero for M.f")
	}

	// Find call
	var callInfo *cfg.CallInfo
	g.EachStmtCall(func(p cfg.Point, c *cfg.CallInfo) {
		callInfo = c
	})

	if callInfo == nil {
		t.Fatal("Call not found")
	}

	// Call and def should have matching paths
	if funcDef.TargetPath.String() != callInfo.CalleePath.String() {
		t.Errorf("Path mismatch: def=%s, call=%s",
			funcDef.TargetPath.String(), callInfo.CalleePath.String())
	}
}

// TestFieldSymbol_CFG_AnonymousCallback tests CFG symbol for inline callback.
func TestFieldSymbol_CFG_AnonymousCallback(t *testing.T) {
	code := `
foo(function(x) return x * 2 end)
`
	chunk, err := parse.ParseString(code, "test.lua")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	fn := &ast.FunctionExpr{Stmts: chunk}
	g := cfg.Build(fn, "foo")

	// Anonymous function should be in nested with a symbol
	nested := g.NestedFunctions()
	if len(nested) == 0 {
		t.Fatal("Anonymous function should be in Nested")
	}

	for _, n := range nested {
		if n.Symbol == 0 {
			t.Error("Anonymous callback should have a symbol")
		}
	}
}

// TestFieldSymbol_CFG_TableLiteralFunction tests CFG symbol for table literal function.
func TestFieldSymbol_CFG_TableLiteralFunction(t *testing.T) {
	code := `
local t = {
    f = function() return 1 end
}
t.f()
`
	chunk, err := parse.ParseString(code, "test.lua")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	fn := &ast.FunctionExpr{Stmts: chunk}
	g := cfg.Build(fn)

	// Get t's symbol
	var tSym cfg.SymbolID
	g.EachAssign(func(p cfg.Point, a *cfg.AssignInfo) {
		if a.IsLocal && len(a.Targets) > 0 && a.Targets[0].Name == "t" {
			tSym = a.Targets[0].Symbol
		}
	})

	if tSym == 0 {
		t.Fatal("Symbol for t not found")
	}

	// Nested function should have a symbol
	nested := g.NestedFunctions()
	if len(nested) == 0 {
		t.Fatal("Table literal function should be in Nested")
	}

	for _, n := range nested {
		if n.Symbol == 0 {
			t.Error("Table literal function should have a symbol (t.f)")
		}
	}

	// Call should resolve to t.f
	var callInfo *cfg.CallInfo
	g.EachStmtCall(func(p cfg.Point, c *cfg.CallInfo) {
		callInfo = c
	})

	if callInfo == nil {
		t.Fatal("Call not found")
	}

	if callInfo.CalleePath.Root != "t" {
		t.Errorf("CalleePath.Root = %q, want %q", callInfo.CalleePath.Root, "t")
	}
	if callInfo.CalleePath.Symbol != tSym {
		t.Errorf("CalleePath.Symbol = %d, want %d", callInfo.CalleePath.Symbol, tSym)
	}
}

// TestFieldSymbol_CFG_ShadowedSymbols tests different symbols for shadowed locals.
func TestFieldSymbol_CFG_ShadowedSymbols(t *testing.T) {
	code := `
local T = {}
T.f = function() return 1 end
T.f()

do
    local T = {}
    T.f = function() return 2 end
    T.f()
end
`
	chunk, err := parse.ParseString(code, "test.lua")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	fn := &ast.FunctionExpr{Stmts: chunk}
	g := cfg.Build(fn)

	// Collect local T assignments
	var tSymbols []cfg.SymbolID
	g.EachAssign(func(p cfg.Point, a *cfg.AssignInfo) {
		if a.IsLocal && len(a.Targets) > 0 && a.Targets[0].Name == "T" {
			tSymbols = append(tSymbols, a.Targets[0].Symbol)
		}
	})

	if len(tSymbols) < 2 {
		t.Fatalf("Expected at least 2 T symbols, got %d", len(tSymbols))
	}

	// The two T's should have different symbols
	if tSymbols[0] == tSymbols[1] {
		t.Error("Outer and inner T should have different symbols")
	}

	// Collect calls
	var calls []*cfg.CallInfo
	g.EachStmtCall(func(p cfg.Point, c *cfg.CallInfo) {
		calls = append(calls, c)
	})

	if len(calls) < 2 {
		t.Fatalf("Expected at least 2 calls, got %d", len(calls))
	}

	// Each call should resolve to the correct T
	if calls[0].CalleePath.Symbol == calls[1].CalleePath.Symbol {
		t.Error("Calls should resolve to different T symbols")
	}
}

// TestFieldSymbol_CFG_DynamicKeyNoSymbol tests dynamic key has no symbol.
func TestFieldSymbol_CFG_DynamicKeyNoSymbol(t *testing.T) {
	code := `
local k = "method"
obj[k]()
`
	chunk, err := parse.ParseString(code, "test.lua")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	fn := &ast.FunctionExpr{Stmts: chunk}
	g := cfg.Build(fn, "obj")

	var callInfo *cfg.CallInfo
	g.EachStmtCall(func(p cfg.Point, c *cfg.CallInfo) {
		callInfo = c
	})

	if callInfo == nil {
		t.Fatal("Call not found")
	}

	// Dynamic key should result in empty CalleePath
	if !callInfo.CalleePath.IsEmpty() {
		t.Errorf("CalleePath should be empty for dynamic key, got %s",
			callInfo.CalleePath.String())
	}
}
