package errors

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// TestFunctionRefinementSetup verifies that function refinements are properly set.
func TestFunctionRefinementSetup(t *testing.T) {
	// Create function with refinement
	notNilEffect := constraint.NewRefinement(
		[]constraint.Constraint{constraint.NotNil{Path: constraint.Path{Root: "$0"}}},
		nil, nil,
	)
	fn := typ.Func().
		Param("val", typ.Any).
		WithRefinement(notNilEffect).
		Build()

	if fn.Refinement == nil {
		t.Fatal("Refinement is nil")
	}
	eff, ok := fn.Refinement.(*constraint.FunctionRefinement)
	if !ok {
		t.Fatalf("Refinement is %T, not *constraint.FunctionRefinement", fn.Refinement)
	}
	if len(eff.OnReturn.MustConstraints()) != 1 {
		t.Fatalf("OnReturn.Len() = %d, want 1", len(eff.OnReturn.MustConstraints()))
	}
	t.Logf("Function refinement: %+v", eff)

	// Now put it in a record and verify field access preserves the refinement
	record := typ.NewRecord().
		Field("not_nil", fn).
		Build()

	// Get field via record API
	field := record.GetField("not_nil")
	if field == nil {
		t.Fatal("field not_nil not found")
	}
	fieldFn, ok := field.Type.(*typ.Function)
	if !ok {
		t.Fatalf("field type is %T, not *typ.Function", field.Type)
	}
	if fieldFn.Refinement == nil {
		t.Fatal("field function Refinement is nil after record lookup")
	}
	fieldEff, ok := fieldFn.Refinement.(*constraint.FunctionRefinement)
	if !ok {
		t.Fatalf("field Refinement is %T", fieldFn.Refinement)
	}
	if len(fieldEff.OnReturn.MustConstraints()) != 1 {
		t.Fatalf("field OnReturn.Len() = %d", len(fieldEff.OnReturn.MustConstraints()))
	}
	t.Logf("Field function refinement preserved: %+v", fieldEff)
}

// TestScopeLookupWithManifest verifies that manifest symbols are accessible in scope.
func TestScopeLookupWithManifest(t *testing.T) {
	notNilEffect := constraint.NewRefinement(
		[]constraint.Constraint{constraint.NotNil{Path: constraint.Path{Root: "$0"}}},
		nil, nil,
	)
	assertNotNil := typ.Func().
		Param("val", typ.Any).
		WithRefinement(notNilEffect).
		Build()

	assertType := typ.NewRecord().
		Field("not_nil", assertNotNil).
		Build()

	assertManifest := io.NewManifest("assert")
	assertManifest.SetExport(assertType)

	// Run a minimal check to get scope
	code := `
local function main()
    assert.not_nil(nil)
end
`
	result := testutil.Check(code, testutil.WithStdlib(), testutil.WithManifest("assert", assertManifest))

	// Log diagnostics to see what's happening
	for _, d := range result.Diagnostics {
		t.Logf("diagnostic: %s at line %d", d.Message, d.Position.Line)
	}
}

// TestNeverAfterAssertion reproduces the "expected function, got never" bug
// where a variable becomes never after an assertion that should narrow it.
func TestNeverAfterAssertion(t *testing.T) {
	// Vol type with methods
	infoType := typ.NewRecord().Field("size", typ.Number).Build()
	volType := typ.NewRecord().
		Field("stat", typ.Func().
			Param("path", typ.String).
			Returns(typ.NewUnion(infoType, typ.Nil), typ.NewUnion(typ.String, typ.Nil)).
			Build()).
		Field("mkdir", typ.Func().
			Param("path", typ.String).
			Returns(typ.Boolean, typ.NewUnion(typ.String, typ.Nil)).
			Build()).
		Field("remove", typ.Func().
			Param("path", typ.String).
			Returns(typ.Boolean, typ.NewUnion(typ.String, typ.Nil)).
			Build()).
		Build()

	// fs.get returns (Vol | nil, string | nil)
	fsType := typ.NewRecord().
		Field("get", typ.Func().
			Param("name", typ.String).
			Returns(typ.NewUnion(volType, typ.Nil), typ.NewUnion(typ.String, typ.Nil)).
			Build()).
		Build()

	// assert.not_nil narrows first param to non-nil via OnReturn
	notNilEffect := constraint.NewRefinement(
		[]constraint.Constraint{constraint.NotNil{Path: constraint.Path{Root: "$0"}}}, // OnReturn
		nil, // OnTrue
		nil, // OnFalse
	)
	assertNotNil := typ.Func().
		Param("val", typ.Any).
		OptParam("msg", typ.String).
		WithRefinement(notNilEffect).
		Build()

	// assert.is_nil narrows first param to nil via OnReturn
	isNilEffect := constraint.NewRefinement(
		[]constraint.Constraint{constraint.IsNil{Path: constraint.Path{Root: "$0"}}}, // OnReturn
		nil, // OnTrue
		nil, // OnFalse
	)
	assertIsNil := typ.Func().
		Param("val", typ.Any).
		OptParam("msg", typ.String).
		WithRefinement(isNilEffect).
		Build()

	assertType := typ.NewRecord().
		Field("not_nil", assertNotNil).
		Field("is_nil", assertIsNil).
		Build()

	fsManifest := io.NewManifest("fs")
	fsManifest.SetExport(fsType)

	assertManifest := io.NewManifest("assert")
	assertManifest.SetExport(assertType)

	tests := []struct {
		name      string
		code      string
		wantError bool
	}{
		{
			name: "method_call_after_not_nil_assertion",
			code: `
local function main()
    local vol, err = fs.get("temp")
    assert.not_nil(vol, "vol should exist")

    local info, stat_err = vol:stat("/test.txt")
    return info
end
`,
			wantError: false,
		},
		{
			name: "multiple_method_calls_after_assertion",
			code: `
local function main()
    local vol, err = fs.get("temp")
    assert.not_nil(vol, "vol should exist")

    local info, stat_err = vol:stat("/test.txt")
    local ok, mkdir_err = vol:mkdir("/newdir")
    vol:remove("/oldfile")
    return ok
end
`,
			wantError: false,
		},
		{
			// After reassignment, the new definition gets fresh narrowing.
			// The flow solver tracks definitions per-assignment point.
			name: "reassignment_then_method_call",
			code: `
local function main()
    local vol, err = fs.get("temp")
    assert.not_nil(vol, "vol should exist")

    local info, stat_err = vol:stat("/test.txt")

    vol, err = fs.get("other")
    assert.not_nil(vol, "other vol should exist")

    local ok, mkdir_err = vol:mkdir("/newdir")
    return ok
end
`,
			wantError: false,
		},
		{
			name: "intermediate_assignment_before_method_call",
			code: `
local function main()
    local vol, err = fs.get("temp")
    assert.not_nil(vol, "vol should exist")
    assert.is_nil(err, "no error")

    local info, stat_err = vol:stat("/test.txt")
    return info
end
`,
			wantError: false,
		},
		{
			// Exact pattern from wippy fs/errors.lua:
			// 1. Get vol, assert is_nil (vol should be nil for nonexistent)
			// 2. Reassign vol, assert not_nil (vol should exist now)
			// 3. Call method on vol - should work because current def is narrowed
			name: "wippy_fs_errors_pattern",
			code: `
local function main()
    local vol, err = fs.get("nonexistent")
    assert.is_nil(vol, "non-existent fs returns nil")
    assert.not_nil(err, "non-existent fs returns error")

    vol, err = fs.get("temp")
    assert.not_nil(vol, "temp fs available")
    assert.is_nil(err, "temp fs no error")

    local info, stat_err = vol:stat("/test.txt")
    return info
end
`,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := testutil.Check(tt.code,
				testutil.WithStdlib(),
				testutil.WithManifest("fs", fsManifest),
				testutil.WithManifest("assert", assertManifest))
			if result.HasError() != tt.wantError {
				for _, d := range result.Diagnostics {
					if d.Severity == diag.SeverityError {
						t.Logf("error at line %d: %s", d.Position.Line, d.Message)
					}
				}
				t.Errorf("wantError=%v, gotError=%v", tt.wantError, result.HasError())
			}
		})
	}
}

// TestFlowSolutionDebug traces the flow solution for reassignment pattern.
func TestFlowSolutionDebug(t *testing.T) {
	// Vol type with methods
	infoType := typ.NewRecord().Field("size", typ.Number).Build()
	volType := typ.NewRecord().
		Field("stat", typ.Func().
			Param("path", typ.String).
			Returns(typ.NewUnion(infoType, typ.Nil), typ.NewUnion(typ.String, typ.Nil)).
			Build()).
		Field("mkdir", typ.Func().
			Param("path", typ.String).
			Returns(typ.Boolean, typ.NewUnion(typ.String, typ.Nil)).
			Build()).
		Build()

	fsType := typ.NewRecord().
		Field("get", typ.Func().
			Param("name", typ.String).
			Returns(typ.NewUnion(volType, typ.Nil), typ.NewUnion(typ.String, typ.Nil)).
			Build()).
		Build()

	// assert.not_nil narrows first param to non-nil via OnReturn
	notNilEffect := constraint.NewRefinement(
		[]constraint.Constraint{constraint.NotNil{Path: constraint.Path{Root: "$0"}}},
		nil, nil,
	)
	assertNotNil := typ.Func().
		Param("val", typ.Any).
		OptParam("msg", typ.String).
		WithRefinement(notNilEffect).
		Build()

	assertType := typ.NewRecord().
		Field("not_nil", assertNotNil).
		Build()

	fsManifest := io.NewManifest("fs")
	fsManifest.SetExport(fsType)

	assertManifest := io.NewManifest("assert")
	assertManifest.SetExport(assertType)

	code := `
local function main()
    local vol, err = fs.get("temp")
    assert.not_nil(vol, "vol should exist")

    local info, stat_err = vol:stat("/test.txt")

    vol, err = fs.get("other")
    assert.not_nil(vol, "other vol should exist")

    local ok, mkdir_err = vol:mkdir("/newdir")
    return ok
end
`
	result := testutil.Check(code,
		testutil.WithStdlib(),
		testutil.WithManifest("fs", fsManifest),
		testutil.WithManifest("assert", assertManifest))

	// Iterate over ALL function results, not just the root (chunk wrapper)
	t.Logf("Total functions analyzed: %d", len(result.Session.Results))
	for fnExpr, funcResult := range result.Session.Results {
		if funcResult == nil || funcResult.Graph == nil {
			continue
		}

		// Count calls in this function
		callCount := 0
		funcResult.Graph.EachStmtCall(func(p cfg.Point, info *cfg.CallInfo) {
			if info != nil {
				callCount++
			}
		})

		isRoot := fnExpr == result.Session.RootFunc
		t.Logf("Function (isRoot=%v, calls=%d):", isRoot, callCount)

		// Skip the root (chunk wrapper) - it has no interesting calls
		if isRoot {
			continue
		}

		// Log calls
		t.Log("  Calls:")
		funcResult.Graph.EachStmtCall(func(p cfg.Point, info *cfg.CallInfo) {
			if info != nil {
				t.Logf("    Point %v: CalleeName=%q Method=%q Callee=%T", p, info.CalleeName, info.Method, info.Callee)
			}
		})

		// Log flow inputs
		if funcResult.FlowInputs != nil {
			t.Logf("  EdgeConditions: %d", len(funcResult.FlowInputs.EdgeConditions))
			for i, ec := range funcResult.FlowInputs.EdgeConditions {
				t.Logf("    Edge %d: from=%v to=%v condition=%v", i, ec.From, ec.To, ec.Condition.Disjuncts)
			}
		}

		// Log flow solution
		if funcResult.FlowSolution != nil {
			solution := funcResult.FlowSolution
			t.Log("  Conditions and EdgeValues:")
			for p := cfg.Point(2); p <= 10; p++ {
				cond := solution.ConditionAt(p)
				if cond.HasConstraints() {
					t.Logf("    ConditionAt(%d): %v", p, cond.AllConstraints())
				}
				key := solution.DebugVersionedKey("vol", p)
				if key != "" {
					t.Logf("    VersionedKey(vol, %d): %s", p, key)
					// Also show the type at this versioned key
					typeAtKey := solution.DebugValueAt(key, p)
					if typeAtKey != nil {
						t.Logf("    ValueAt(%s): %v", key, typeAtKey)
					} else {
						t.Logf("    ValueAt(%s): nil", key)
					}
				}
				// Log edge values for edges INTO this point
				for pred := cfg.Point(1); pred < p; pred++ {
					edgeVals := solution.DebugEdgeValues(pred, p)
					if len(edgeVals) > 0 {
						t.Logf("    EdgeValues %d->%d: %v", pred, p, edgeVals)
					}
				}
			}
		}
	}

	if result.HasError() {
		for _, d := range result.Diagnostics {
			if d.Severity == diag.SeverityError {
				t.Logf("error at line %d: %s", d.Position.Line, d.Message)
			}
		}
		t.Errorf("expected no errors")
	}
}

// TestNeverAfterEarlyReturn reproduces the pattern where early return should
// narrow the variable for the rest of the function.
func TestNeverAfterEarlyReturn(t *testing.T) {
	infoType := typ.NewRecord().Field("size", typ.Number).Build()
	volType := typ.NewRecord().
		Field("stat", typ.Func().
			Param("path", typ.String).
			Returns(typ.NewUnion(infoType, typ.Nil), typ.NewUnion(typ.String, typ.Nil)).
			Build()).
		Build()

	fsType := typ.NewRecord().
		Field("get", typ.Func().
			Param("name", typ.String).
			Returns(typ.NewUnion(volType, typ.Nil), typ.NewUnion(typ.String, typ.Nil)).
			Build()).
		Build()

	fsManifest := io.NewManifest("fs")
	fsManifest.SetExport(fsType)

	tests := []struct {
		name      string
		code      string
		wantError bool
	}{
		{
			name: "early_return_on_nil_narrows",
			code: `
local fs = require("fs")

local function main(): boolean
    local vol, err = fs.get("temp")
    if vol == nil then
        return false
    end

    local info, stat_err = vol:stat("/test.txt")
    return true
end
`,
			wantError: false,
		},
		{
			name: "early_return_on_error_narrows",
			code: `
local fs = require("fs")

local function main(): boolean
    local vol, err = fs.get("temp")
    if err then
        return false
    end
    if vol == nil then
        return false
    end

    local info, stat_err = vol:stat("/test.txt")
    return true
end
`,
			wantError: false,
		},
		{
			name: "multiple_early_returns_narrow",
			code: `
local fs = require("fs")

local function main(): boolean
    local vol, err = fs.get("temp")
    if vol == nil then
        return false
    end

    local info, stat_err = vol:stat("/test.txt")
    if info == nil then
        return false
    end

    local size: number = info.size
    return size > 0
end
`,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := testutil.Check(tt.code,
				testutil.WithStdlib(),
				testutil.WithManifest("fs", fsManifest))
			if result.HasError() != tt.wantError {
				for _, d := range result.Diagnostics {
					if d.Severity == diag.SeverityError {
						t.Logf("error at line %d: %s", d.Position.Line, d.Message)
					}
				}
				t.Errorf("wantError=%v, gotError=%v", tt.wantError, result.HasError())
			}
		})
	}
}
