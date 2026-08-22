package core

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/narrow"
)

// TestSameNameDifferentScopesHaveDifferentSymbols verifies that variables
// with the same name in different scopes produce different narrowing behavior.
func TestSameNameDifferentScopesHaveDifferentSymbols(t *testing.T) {
	// Inner x shadows outer x; each should have distinct identity
	// so narrowing on one doesn't affect the other
	tests := []testutil.Case{
		{
			Name: "shadowed_variable_distinct_narrowing",
			Code: `
local function test(x)
    if type(x) == "number" then
        do
            local x = "hello"
            local y: string = x
        end
        local z: number = x
    end
end
`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "shadowed_variable_wrong_type",
			Code: `
local function test(x)
    if type(x) == "number" then
        do
            local x = "hello"
            local y: integer = x
        end
    end
end
`,
			WantError: true,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestNarrowingUsesSymbolNotName verifies that type narrowing applies
// to the specific symbol, not to any variable with the same name.
func TestNarrowingUsesSymbolNotName(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "parameter_shadowed_by_local",
			Code: `
local function check(t)
    if t ~= nil then
        local t = nil
        local x: nil = t
    end
end
`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "shadowed_loses_narrowing",
			Code: `
local function check(t)
    if t ~= nil then
        local t = nil
        local _ = t.field
    end
end
`,
			WantError: true,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestPathKeyUsesSymbolIdentity verifies that Path.Key() produces
// different keys for paths with different SymbolIDs, even if Root matches.
func TestPathKeyUsesSymbolIdentity(t *testing.T) {
	// Create two paths with same Root but different Symbols
	path1 := constraint.Path{Root: "x", Symbol: 100}
	path2 := constraint.Path{Root: "x", Symbol: 200}
	path3 := constraint.Path{Root: "x", Symbol: 100}

	key1 := path1.Key()
	key2 := path2.Key()
	key3 := path3.Key()

	if key1 == key2 {
		t.Errorf("paths with different symbols produced same key: %s", key1)
	}

	if key1 != key3 {
		t.Errorf("paths with same symbol produced different keys: %s vs %s", key1, key3)
	}

	// Verify key format uses symbol (should start with sym)
	if len(key1) < 3 || string(key1)[:3] != "sym" {
		t.Errorf("symbol-based key should start with sym, got: %s", key1)
	}
}

// TestPathEqualUsesSymbolWhenAvailable verifies that Path.Equal()
// compares by Symbol when both paths have Symbol != 0.
func TestPathEqualUsesSymbolWhenAvailable(t *testing.T) {
	// Same symbol, different root names (should be equal)
	p1 := constraint.Path{Root: "x", Symbol: 100}
	p2 := constraint.Path{Root: "y", Symbol: 100}
	if !p1.Equal(p2) {
		t.Error("paths with same symbol should be equal regardless of Root")
	}

	// Different symbol, same root name (should not be equal)
	p3 := constraint.Path{Root: "x", Symbol: 100}
	p4 := constraint.Path{Root: "x", Symbol: 200}
	if p3.Equal(p4) {
		t.Error("paths with different symbols should not be equal")
	}

	// Placeholder paths (Symbol == 0) compare by Root
	p5 := constraint.Path{Root: "$0", Symbol: 0}
	p6 := constraint.Path{Root: "$0", Symbol: 0}
	p7 := constraint.Path{Root: "$1", Symbol: 0}
	if !p5.Equal(p6) {
		t.Error("placeholder paths with same Root should be equal")
	}
	if p5.Equal(p7) {
		t.Error("placeholder paths with different Root should not be equal")
	}
}

// TestPlaceholderSubstitutionPreservesIdentity verifies that when
// placeholder paths ($0, $1) are substituted with argument paths,
// the resulting path has correct Symbol identity.
func TestPlaceholderSubstitutionPreservesIdentity(t *testing.T) {
	// Create a placeholder path
	placeholder := constraint.Path{Root: "$0", Symbol: 0}
	placeholder = placeholder.Append(constraint.Segment{Kind: constraint.SegmentField, Name: "x"})

	// Create an argument path with symbol identity
	argPath := constraint.Path{Root: "t", Symbol: 42}

	// Substitute
	result, ok := placeholder.Substitute([]constraint.Path{argPath})
	if !ok {
		t.Fatal("substitution failed")
	}

	// Result should have the argument's symbol
	if result.Symbol != 42 {
		t.Errorf("substituted path has wrong symbol: got %d, want 42", result.Symbol)
	}

	// Result should have combined segments
	if len(result.Segments) != 1 || result.Segments[0].Name != "x" {
		t.Errorf("substituted path has wrong segments: %v", result.Segments)
	}
}

// TestConstraintPathsUseSymbolIdentity verifies that constraints
// built from expressions use Symbol-based identity.
func TestConstraintPathsUseSymbolIdentity(t *testing.T) {
	// Create a HasType constraint with symbol-rooted path
	path := constraint.Path{Root: "x", Symbol: 123}
	c := constraint.HasType{Path: path, Type: narrow.TypeKey{Kind: narrow.TypeKeyBuiltin, Name: "string"}}

	paths := c.Paths()
	if len(paths) != 1 {
		t.Fatalf("expected 1 path, got %d", len(paths))
	}

	if paths[0].Symbol != 123 {
		t.Errorf("constraint path lost symbol identity: got %d, want 123", paths[0].Symbol)
	}
}

// TestPathHashUsesSymbol verifies that Path.Hash() uses Symbol for identity.
func TestPathHashUsesSymbol(t *testing.T) {
	// Same symbol, different roots should have same hash
	p1 := constraint.Path{Root: "x", Symbol: 100}
	p2 := constraint.Path{Root: "y", Symbol: 100}

	if p1.Hash() != p2.Hash() {
		t.Error("paths with same symbol should have same hash")
	}

	// Different symbols should have different hashes (with high probability)
	p3 := constraint.Path{Root: "x", Symbol: 200}
	if p1.Hash() == p3.Hash() {
		t.Error("paths with different symbols should have different hashes")
	}
}

// TestNarrowingPreservedAcrossScopes verifies that narrowing from outer
// scope is preserved when accessed, but shadowing creates new identity.
func TestNarrowingPreservedAcrossScopes(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "outer_narrowing_preserved_in_inner_scope",
			Code: `
local function test(x)
    if type(x) == "number" then
        do
            local y: number = x
        end
    end
end
`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "function_closure_uses_outer_symbol",
			Code: `
local function test(x)
    if type(x) == "number" then
        local f = function()
            local y: number = x
        end
    end
end
`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestMultipleSameNameVariables verifies that multiple variables with
// the same name in sequence have distinct identities.
func TestMultipleSameNameVariables(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "sequential_same_name_assignments",
			Code: `
local x = 1
local x = "hello"  -- new declaration shadows
local y: string = x  -- x is now string
`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "loop_variable_distinct_from_outer",
			Code: `
local i = "outer"
for i = 1, 10 do
    -- i here is integer, not string
    local x: integer = i  -- should pass
end
-- i here is string again
local y: string = i  -- should pass
`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestPathValidateSymbol verifies the ValidateSymbol invariant check.
func TestPathValidateSymbol(t *testing.T) {
	// Empty path is valid
	empty := constraint.Path{}
	if msg := empty.ValidateSymbol(); msg != "" {
		t.Errorf("empty path should be valid, got: %s", msg)
	}

	// Placeholder path is valid with Symbol=0
	placeholder := constraint.Path{Root: "$0", Symbol: 0}
	if msg := placeholder.ValidateSymbol(); msg != "" {
		t.Errorf("placeholder path should be valid, got: %s", msg)
	}

	// Resolved path with Symbol is valid
	resolved := constraint.Path{Root: "x", Symbol: 100}
	if msg := resolved.ValidateSymbol(); msg != "" {
		t.Errorf("resolved path with symbol should be valid, got: %s", msg)
	}

	// Path with Root but no Symbol is INVALID (unless placeholder)
	invalid := constraint.Path{Root: "x", Symbol: 0}
	if msg := invalid.ValidateSymbol(); msg == "" {
		t.Error("path with Root but no Symbol should be invalid")
	}
}
