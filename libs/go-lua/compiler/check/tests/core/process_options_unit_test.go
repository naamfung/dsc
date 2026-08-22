package core

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

// TestProcessOptionsCallability reproduces the error:
// "expected function, got unknown & fun() -> {trap_links: boolean}"
//
// This occurs when manifest types are intersected with Unknown.
func TestProcessOptionsCallability(t *testing.T) {
	// process.Options type
	processOptionsType := typ.NewRecord().
		Field("trap_links", typ.Boolean).
		Build()

	// Module methods as interface
	moduleMethodsType := typ.NewInterface("process", []typ.Method{
		{Name: "id", Type: typ.Func().Returns(typ.String, typ.NewOptional(typ.String)).Build()},
		{Name: "pid", Type: typ.Func().Returns(typ.String).Build()},
		{Name: "get_options", Type: typ.Func().
			Returns(processOptionsType).
			Build()},
		{Name: "set_options", Type: typ.Func().
			Param("opts", processOptionsType).
			Returns(typ.Boolean, typ.NewOptional(typ.String)).
			Build()},
	})

	// Module fields as record
	moduleFieldsType := typ.NewRecord().
		Field("some_field", typ.String).
		Build()

	// Create manifest with intersection export (like wippy does)
	processManifest := io.NewManifest("process")
	processManifest.SetExport(typ.NewIntersection(moduleMethodsType, moduleFieldsType))

	tests := []struct {
		name      string
		code      string
		wantError bool
	}{
		{
			name: "simple_get_options_call",
			code: `
local function main()
    local opts = process.get_options()
    return opts
end
`,
			wantError: false,
		},
		{
			name: "simple_set_options_call",
			code: `
local function main()
    local ok, err = process.set_options({ trap_links = true })
    return ok
end
`,
			wantError: false,
		},
		{
			name: "get_and_set_options",
			code: `
local function main()
    local opts = process.get_options()
    local ok, err = process.set_options({ trap_links = false })
    return ok
end
`,
			wantError: false,
		},
		{
			name: "pid_call",
			code: `
local function main()
    local pid = process.pid()
    return pid
end
`,
			wantError: false,
		},
		{
			name: "id_call",
			code: `
local function main()
    local id, err = process.id()
    return id
end
`,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := testutil.Check(tt.code,
				testutil.WithStdlib(),
				testutil.WithManifest("process", processManifest))

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

// TestInterfaceMethodCallability tests that interface methods are callable.
func TestInterfaceMethodCallability(t *testing.T) {
	// Simple interface with a method
	ifaceType := typ.NewInterface("mymodule", []typ.Method{
		{Name: "foo", Type: typ.Func().Returns(typ.String).Build()},
	})

	manifest := io.NewManifest("mymodule")
	manifest.SetExport(ifaceType)

	code := `
local function main()
    local result = mymodule.foo()
    return result
end
`
	result := testutil.Check(code, testutil.WithStdlib(), testutil.WithManifest("mymodule", manifest))

	if result.HasError() {
		for _, d := range result.Diagnostics {
			if d.Severity == diag.SeverityError {
				t.Logf("error at line %d: %s", d.Position.Line, d.Message)
			}
		}
		t.Errorf("expected no errors, got errors")
	}
}

// TestIntersectionMethodCallability tests that intersection type methods are callable.
func TestIntersectionMethodCallability(t *testing.T) {
	ifaceType := typ.NewInterface("mymodule", []typ.Method{
		{Name: "foo", Type: typ.Func().Returns(typ.String).Build()},
	})

	recordType := typ.NewRecord().
		Field("bar", typ.Number).
		Build()

	intersectionType := typ.NewIntersection(ifaceType, recordType)

	manifest := io.NewManifest("mymodule")
	manifest.SetExport(intersectionType)

	code := `
local function main()
    local result = mymodule.foo()
    local num = mymodule.bar
    return result
end
`
	result := testutil.Check(code, testutil.WithStdlib(), testutil.WithManifest("mymodule", manifest))

	if result.HasError() {
		for _, d := range result.Diagnostics {
			if d.Severity == diag.SeverityError {
				t.Logf("error at line %d: %s", d.Position.Line, d.Message)
			}
		}
		t.Errorf("expected no errors, got errors")
	}
}

// TestUnknownInIntersectionCausesCallError reproduces the exact bug pattern.
// When a module type is Unknown & Interface, field lookup returns Unknown & Function,
// and the call checker should still extract the function and call it.
func TestUnknownInIntersectionCausesCallError(t *testing.T) {
	processOptionsType := typ.NewRecord().
		Field("trap_links", typ.Boolean).
		Build()

	moduleMethodsType := typ.NewInterface("process", []typ.Method{
		{Name: "get_options", Type: typ.Func().Returns(processOptionsType).Build()},
	})

	// BUG PATTERN: Unknown intersected with the interface
	buggyExport := typ.NewIntersection(typ.Unknown, moduleMethodsType)

	t.Logf("Buggy export type: %s", buggyExport.String())

	// Check what field lookup returns
	fieldType, ok := core.Field(buggyExport, "get_options")
	t.Logf("Field lookup ok=%v type=%v", ok, fieldType)
	if fieldType != nil {
		t.Logf("Field type string: %s", fieldType.String())
		t.Logf("Field kind: %v", fieldType.Kind())
	}

	processManifest := io.NewManifest("process")
	processManifest.SetExport(buggyExport)

	result := testutil.Check(`
local function main()
    local opts = process.get_options()
    return opts
end
`, testutil.WithStdlib(), testutil.WithManifest("process", processManifest))

	for _, d := range result.Diagnostics {
		t.Logf("diagnostic [%s]: %s", d.Severity, d.Message)
	}

	// The current implementation should handle this correctly
	// If it doesn't, this test will fail and show the error
	if result.HasError() {
		t.Errorf("BUG: call failed with unknown & fun() intersection")
	}
}

// TestDebugProcessType logs what type is synthesized for process.get_options.
func TestDebugProcessType(t *testing.T) {
	processOptionsType := typ.NewRecord().
		Field("trap_links", typ.Boolean).
		Build()

	moduleMethodsType := typ.NewInterface("process", []typ.Method{
		{Name: "get_options", Type: typ.Func().
			Returns(processOptionsType).
			Build()},
	})

	moduleFieldsType := typ.NewRecord().
		Field("some_field", typ.String).
		Build()

	intersectionExport := typ.NewIntersection(moduleMethodsType, moduleFieldsType)

	processManifest := io.NewManifest("process")
	processManifest.SetExport(intersectionExport)

	t.Logf("Manifest export type: %T = %s", intersectionExport, intersectionExport.String())

	// Check what type.Method returns for get_options
	if inter, ok := intersectionExport.(*typ.Intersection); ok {
		t.Logf("Intersection members: %d", len(inter.Members))
		for i, m := range inter.Members {
			t.Logf("  Member %d: %T = %s", i, m, m.String())
			if iface, ok := m.(*typ.Interface); ok {
				t.Logf("    Interface %s has %d methods", iface.Name, len(iface.Methods))
				for _, meth := range iface.Methods {
					t.Logf("      Method: %s = %s", meth.Name, meth.Type.String())
				}
			}
		}
	}

	code := `
local function main()
    local opts = process.get_options()
    return opts
end
`
	result := testutil.Check(code, testutil.WithStdlib(), testutil.WithManifest("process", processManifest))

	// Log all diagnostics
	for _, d := range result.Diagnostics {
		t.Logf("diagnostic [%s] at line %d: %s", d.Severity, d.Position.Line, d.Message)
	}

	if result.HasError() {
		t.Errorf("expected no errors")
	}
}
