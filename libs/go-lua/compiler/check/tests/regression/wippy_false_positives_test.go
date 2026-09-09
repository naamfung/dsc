package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// TestChannelSendAfterSelectComparison reproduces the wippy false positive:
// stop_signal:send(true) fails with "expected function, got never"
//
// Pattern from bus_pattern.lua:
//  1. local stop_signal = channel.new(0)
//  2. channel.select with stop_signal:case_receive()
//  3. if result.channel == stop_signal then ... end
//  4. stop_signal:send(true) -- ERROR: expected function, got never
//
// The bug: after comparing result.channel == stop_signal,
// the channel itself is incorrectly narrowed to never.
func TestChannelSendAfterSelectComparison(t *testing.T) {
	chManifest := ChannelManifestWithSend()

	source := `
		local ops_channel = channel.new(256)
		local stop_signal = channel.new(0)

		while true do
			local result = channel.select({
				stop_signal:case_receive(),
				ops_channel:case_receive()
			})

			if result.channel == stop_signal then
				return
			end

			if result.channel == ops_channel then
				stop_signal:send(true)
			end
		end
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("channel", chManifest))

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors for channel:send after select comparison")
	}
}

// TestLocalFunctionShadowsModule reproduces the wippy false positive:
// local process(val) is shadowed by global process module
//
// Pattern from union_types.lua:
//  1. local function process(val: number | string): string ... end
//  2. process(a) -- ERROR: intersection member is not callable
//
// The bug: the local function definition doesn't shadow the global module.
func TestLocalFunctionShadowsModule(t *testing.T) {
	// Create a process module similar to wippy's
	moduleFieldsType := typ.NewRecord().
		Field("event", typ.NewRecord().
			Field("CANCEL", typ.String).
			Field("EXIT", typ.String).
			Build()).
		Build()

	moduleMethodsType := typ.NewInterface("process", []typ.Method{
		{Name: "pid", Type: typ.Func().Returns(typ.String).Build()},
	})

	processManifest := io.NewManifest("process")
	processManifest.SetExport(typ.NewIntersection(moduleMethodsType, moduleFieldsType))

	source := `
		local function process(val: number | string): string
			if type(val) == "number" then
				return "number:" .. tostring(val)
			else
				return "string:" .. val
			end
		end

		local function main(): boolean
			local a: number | string = 42
			local b: number | string = "hello"

			local ra: string = process(a)
			local rb: string = process(b)

			return ra == "number:42" and rb == "string:hello"
		end
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("process", processManifest))

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors when local function shadows module name")
	}
}

// TestChannelSendAfterSelectComparison_MinimalRepro is a minimal reproduction
// of the channel becoming never after comparison.
func TestChannelSendAfterSelectComparison_MinimalRepro(t *testing.T) {
	chManifest := ChannelManifestWithSend()

	source := `
		local ch = channel.new(0)
		local result = channel.select({ ch:case_receive() })
		if result.channel == ch then
			return
		end
		ch:send(true)
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("channel", chManifest))

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Errorf("expected no errors for channel:send after select comparison")
	}
}

// TestChannelSendWithoutSelectComparison verifies channel.send works
// when there's no comparison that could narrow the channel.
func TestChannelSendWithoutSelectComparison(t *testing.T) {
	chManifest := ChannelManifestWithSend()

	source := `
		local ch = channel.new(0)
		ch:send(true)
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("channel", chManifest))

	if result.HasError() {
		for _, d := range result.Diagnostics {
			if d.Severity == diag.SeverityError {
				t.Logf("error at line %d: %s", d.Position.Line, d.Message)
			}
		}
		t.Errorf("expected no errors for simple channel:send")
	}
}

// TestLocalFunctionShadowsModule_SimpleCase tests that a local function
// with the same name as a module shadows it correctly.
func TestLocalFunctionShadowsModule_SimpleCase(t *testing.T) {
	// Create a 'mymodule' module that conflicts with local function
	mathManifest := io.NewManifest("mymodule")
	mathManifest.SetExport(typ.NewInterface("mymodule", []typ.Method{
		{Name: "abs", Type: typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()},
	}))

	source := `
		local function mymodule(x: number): number
			return x * 2
		end

		local result: number = mymodule(5)
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("mymodule", mathManifest))

	if result.HasError() {
		for _, d := range result.Diagnostics {
			if d.Severity == diag.SeverityError {
				t.Logf("error at line %d: %s", d.Position.Line, d.Message)
			}
		}
		t.Errorf("expected no errors when local function shadows module")
	}
}

// TestLocalFunctionShadowsModule_NestedCall tests the case where
// a local function is called from inside another function.
func TestLocalFunctionShadowsModule_NestedCall(t *testing.T) {
	// Create a 'mymodule' module that conflicts with local function
	mathManifest := io.NewManifest("mymodule")
	mathManifest.SetExport(typ.NewInterface("mymodule", []typ.Method{
		{Name: "abs", Type: typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()},
	}))

	source := `
		local function mymodule(x: number): number
			return x * 2
		end

		local function main()
			local result: number = mymodule(5)
			return result
		end
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("mymodule", mathManifest))

	if result.HasError() {
		for _, d := range result.Diagnostics {
			if d.Severity == diag.SeverityError {
				t.Logf("error at line %d: %s", d.Position.Line, d.Message)
			}
		}
		t.Errorf("expected no errors when local function is called from nested scope")
	}
}

// TestLocalFunctionShadowsModule_NestedCall_NoGlobal tests that local functions
// work correctly when there's no conflicting global.
func TestLocalFunctionShadowsModule_NestedCall_NoGlobal(t *testing.T) {
	source := `
		local function helper(x: number): number
			return x * 2
		end

		local function main()
			local result: number = helper(5)
			return result
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())

	if result.HasError() {
		for _, d := range result.Diagnostics {
			if d.Severity == diag.SeverityError {
				t.Logf("error at line %d: %s", d.Position.Line, d.Message)
			}
		}
		t.Errorf("expected no errors for local function call from nested scope (no global conflict)")
	}
}

// TestLocalFunctionShadowsModule_BindingDiagnostic checks binding behavior.
func TestLocalFunctionShadowsModule_BindingDiagnostic(t *testing.T) {
	source := `
		local function mymodule(x: number): number
			return x * 2
		end

		local function main()
			local result: number = mymodule(5)
			return result
		end
	`

	// Create manifest with global "mymodule"
	mathManifest := io.NewManifest("mymodule")
	mathManifest.SetExport(typ.NewInterface("mymodule", []typ.Method{
		{Name: "abs", Type: typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()},
	}))

	checker := testutil.NewChecker(testutil.WithStdlib(), testutil.WithManifest("mymodule", mathManifest))
	sess := checker.Check(source, "test.lua")

	// Check canonical local function types
	if sess != nil && sess.Store != nil && sess.RootResult != nil && sess.RootResult.BaseScope != nil && sess.RootResult.Graph != nil {
		parentHash := sess.Store.GraphParentHashOf(sess.RootResult.Graph.ID())
		parent := sess.Store.Parents()[parentHash]
		funcTypes := sess.Store.GetLocalFuncTypesSnapshot(sess.RootResult.Graph, parent)
		t.Logf("LocalFuncTypes has %d symbols", len(funcTypes))
		for sym, ty := range funcTypes {
			name := ""
			if sess.Store.ModuleBindings() != nil {
				name = sess.Store.ModuleBindings().Name(sym)
			}
			if ty != nil {
				t.Logf("  sym %d (%s): %s", sym, name, ty.String())
			}
		}
	}

	// Check errors
	for _, d := range sess.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
	}
}

// TestLocalFunctionShadowsModule_IntersectionType tests with intersection export type.
func TestLocalFunctionShadowsModule_IntersectionType(t *testing.T) {
	moduleFieldsType := typ.NewRecord().
		Field("event", typ.NewRecord().
			Field("CANCEL", typ.String).
			Build()).
		Build()

	moduleMethodsType := typ.NewInterface("process", []typ.Method{
		{Name: "pid", Type: typ.Func().Returns(typ.String).Build()},
	})

	processManifest := io.NewManifest("process")
	processManifest.SetExport(typ.NewIntersection(moduleMethodsType, moduleFieldsType))

	// Simple case - call at module level
	source := `
		local function process(x: number): number
			return x * 2
		end

		local result: number = process(5)
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("process", processManifest))

	if result.HasError() {
		for _, d := range result.Diagnostics {
			if d.Severity == diag.SeverityError {
				t.Logf("error at line %d: %s", d.Position.Line, d.Message)
			}
		}
		t.Errorf("expected no errors when local function shadows intersection module")
	}
}

// ChannelManifestWithSend creates a channel manifest that includes the send method.
func ChannelManifestWithSend() *io.Manifest {
	m := io.NewManifest("channel")

	// SelectCase<C, T>
	selectCaseType := typ.NewInterface("channel.SelectCase", nil)
	selectCaseChannel := typ.NewTypeParam("C", nil)
	selectCaseValue := typ.NewTypeParam("T", nil)
	selectCaseGeneric := typ.NewGeneric("channel.SelectCase", []*typ.TypeParam{selectCaseChannel, selectCaseValue}, selectCaseType)

	// Channel<T> with send, receive, case_receive methods
	channelElem := typ.NewTypeParam("T", nil)
	channelType := typ.NewInterface("channel.Channel", []typ.Method{
		{
			Name: "send",
			Type: typ.Func().
				Param("self", typ.Self).
				Param("value", channelElem).
				Returns(typ.Boolean).
				Build(),
		},
		{
			Name: "receive",
			Type: typ.Func().Param("self", typ.Self).Returns(channelElem, typ.Boolean).Build(),
		},
		{
			Name: "case_receive",
			Type: typ.Func().
				Param("self", typ.Self).
				Returns(typ.Instantiate(selectCaseGeneric, typ.Self, channelElem)).
				Build(),
		},
	})
	channelGeneric := typ.NewGeneric("channel.Channel", []*typ.TypeParam{channelElem}, channelType)

	// SelectResult = {channel: any, value: unknown, ok: boolean}
	selectResultType := typ.NewRecord().
		Field("channel", typ.Any).
		Field("value", typ.Unknown).
		Field("ok", typ.Boolean).
		Build()

	m.DefineType("Channel", channelGeneric)
	m.DefineType("SelectCase", selectCaseGeneric)
	m.DefineType("SelectResult", selectResultType)

	channelEmpty := typ.Instantiate(channelGeneric, typ.Unknown)

	moduleType := typ.NewInterface("channel", []typ.Method{
		{Name: "new", Type: typ.Func().OptParam("size", typ.Number).Returns(channelEmpty).Build()},
		{Name: "select", Type: typ.Func().Param("cases", typ.Any).Returns(selectResultType).Build()},
	})
	m.SetExport(moduleType)
	return m
}
