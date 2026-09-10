package core

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/hooks"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/query/core"
)

func hasError(diags []diag.Diagnostic) bool {
	for _, d := range diags {
		if d.Severity == diag.SeverityError {
			return true
		}
	}
	return false
}

func errorMessages(diags []diag.Diagnostic) []string {
	var msgs []string
	for _, d := range diags {
		if d.Severity == diag.SeverityError {
			msgs = append(msgs, d.Message)
		}
	}
	return msgs
}

func newChecker() *check.Checker {
	return check.NewChecker(db.New(), check.Deps{Types: core.NewEngine()}, hooks.WithAssign(), hooks.WithReturn(), hooks.WithCall())
}

func TestAssign_BasicTypes(t *testing.T) {
	tests := []struct {
		name      string
		code      string
		wantError bool
	}{
		{"number to number", "local x: number = 42", false},
		{"string to string", "local s: string = 'hello'", false},
		{"boolean to boolean", "local b: boolean = true", false},
		{"nil to nil", "local n: nil = nil", false},
		{"number to string error", "local x: string = 42", true},
		{"string to number error", "local x: number = 'hello'", true},
		{"boolean to number error", "local x: number = true", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sess := newChecker().Check(tt.code, "test.lua")
			gotError := hasError(sess.Diagnostics)
			if gotError != tt.wantError {
				t.Errorf("wantError=%v, gotError=%v, diags=%v", tt.wantError, gotError, errorMessages(sess.Diagnostics))
			}
		})
	}
}

func TestAssign_Tables(t *testing.T) {
	tests := []struct {
		name      string
		code      string
		wantError bool
	}{
		{"array literal", "local arr: {number} = {1, 2, 3}", false},
		{"record literal", "local r: {x: number, y: number} = {x = 1, y = 2}", false},
		{"nested record", "local r: {inner: {value: number}} = {inner = {value = 42}}", false},
		{"empty to record error", "local r: {x: number} = {}", true},
		{"wrong field type", "local r: {x: number} = {x = 'wrong'}", true},
		{"missing field", "local r: {x: number, y: number} = {x = 1}", true},
		{"extra field ok", "local r: {x: number} = {x = 1, y = 2}", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sess := newChecker().Check(tt.code, "test.lua")
			gotError := hasError(sess.Diagnostics)
			if gotError != tt.wantError {
				t.Errorf("wantError=%v, gotError=%v, diags=%v", tt.wantError, gotError, errorMessages(sess.Diagnostics))
			}
		})
	}
}

func TestAssign_Unions(t *testing.T) {
	tests := []struct {
		name      string
		code      string
		wantError bool
	}{
		{"number to union", "local x: number | string = 42", false},
		{"string to union", "local x: number | string = 'hello'", false},
		{"boolean to union error", "local x: number | string = true", true},
		{"nil to nullable", "local x: number | nil = nil", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sess := newChecker().Check(tt.code, "test.lua")
			gotError := hasError(sess.Diagnostics)
			if gotError != tt.wantError {
				t.Errorf("wantError=%v, gotError=%v, diags=%v", tt.wantError, gotError, errorMessages(sess.Diagnostics))
			}
		})
	}
}

func TestAssign_Optional(t *testing.T) {
	tests := []struct {
		name      string
		code      string
		wantError bool
	}{
		{"number to optional", "local x: number? = 42", false},
		{"nil to optional", "local x: number? = nil", false},
		{"string to number? error", "local x: number? = 'hello'", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sess := newChecker().Check(tt.code, "test.lua")
			gotError := hasError(sess.Diagnostics)
			if gotError != tt.wantError {
				t.Errorf("wantError=%v, gotError=%v, diags=%v", tt.wantError, gotError, errorMessages(sess.Diagnostics))
			}
		})
	}
}

func TestAssign_Functions(t *testing.T) {
	tests := []struct {
		name      string
		code      string
		wantError bool
	}{
		{
			name:      "matching signature",
			code:      `local f: (x: number) -> number = function(x: number): number return x end`,
			wantError: false,
		},
		{
			name:      "return mismatch",
			code:      `local f: (x: number) -> string = function(x: number): number return x end`,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sess := newChecker().Check(tt.code, "test.lua")
			gotError := hasError(sess.Diagnostics)
			if gotError != tt.wantError {
				t.Errorf("wantError=%v, gotError=%v, diags=%v", tt.wantError, gotError, errorMessages(sess.Diagnostics))
			}
		})
	}
}

func TestAssign_Reassignment(t *testing.T) {
	tests := []struct {
		name      string
		code      string
		wantError bool
	}{
		{"same type", "local x: number = 1\nx = 2", false},
		{"untyped widens", "local x = 1\nx = 'ok'", false},
		{"typed mismatch", "local x: number = 1\nx = 'wrong'", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sess := newChecker().Check(tt.code, "test.lua")
			gotError := hasError(sess.Diagnostics)
			if gotError != tt.wantError {
				t.Errorf("wantError=%v, gotError=%v, diags=%v", tt.wantError, gotError, errorMessages(sess.Diagnostics))
			}
		})
	}
}

func TestAssign_NestedAccess(t *testing.T) {
	tests := []struct {
		name      string
		code      string
		wantError bool
	}{
		{
			name:      "nested field",
			code:      "local c = {s = {port = 8080}}\nlocal p: number = c.s.port",
			wantError: false,
		},
		{
			name:      "nested wrong type",
			code:      "local c = {s = {port = 8080}}\nlocal p: string = c.s.port",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sess := newChecker().Check(tt.code, "test.lua")
			gotError := hasError(sess.Diagnostics)
			if gotError != tt.wantError {
				t.Errorf("wantError=%v, gotError=%v, diags=%v", tt.wantError, gotError, errorMessages(sess.Diagnostics))
			}
		})
	}
}

func TestAssign_Intersections(t *testing.T) {
	tests := []struct {
		name      string
		code      string
		wantError bool
	}{
		{
			name:      "record to intersection",
			code:      "type P = {name: string} & {age: number}\nlocal p: P = {name = 'A', age = 30}",
			wantError: false,
		},
		{
			name:      "missing intersection field",
			code:      "type P = {name: string} & {age: number}\nlocal p: P = {name = 'A'}",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sess := newChecker().Check(tt.code, "test.lua")
			gotError := hasError(sess.Diagnostics)
			if gotError != tt.wantError {
				t.Errorf("wantError=%v, gotError=%v, diags=%v", tt.wantError, gotError, errorMessages(sess.Diagnostics))
			}
		})
	}
}

func TestAssign_Multiple(t *testing.T) {
	tests := []struct {
		name      string
		code      string
		wantError bool
	}{
		{"match", "local a: number, b: string = 1, 'hi'", false},
		{"mismatch", "local a: number, b: string = 'x', 1", true},
		{"fewer fills nil", "local a: number?, b: string? = 1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sess := newChecker().Check(tt.code, "test.lua")
			gotError := hasError(sess.Diagnostics)
			if gotError != tt.wantError {
				t.Errorf("wantError=%v, gotError=%v, diags=%v", tt.wantError, gotError, errorMessages(sess.Diagnostics))
			}
		})
	}
}

func TestAssign_ErrorMessage(t *testing.T) {
	code := "type C = {port: number}\nlocal c: C = {port = 'x'}"
	sess := newChecker().Check(code, "test.lua")
	if len(sess.Diagnostics) == 0 {
		t.Fatal("expected error")
	}
	msg := sess.Diagnostics[0].Message
	if !strings.Contains(msg, "port") && !strings.Contains(msg, "number") && !strings.Contains(msg, "string") {
		t.Errorf("error should mention field/type: %s", msg)
	}
}

// TestAssign_DynamicTableIndexing tests that tables populated via t[key] = value
// keep sound index semantics: exact dominating writes can be definite, while
// arbitrary map lookups remain optional until proven present.
func TestAssign_DynamicTableIndexing(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "loop_populated_table_field_access",
			Code: `
				type Method = {name: string}
				local methods: {Method} = {{name = "greet"}, {name = "hello"}}
				local method_names: {[string]: boolean} = {}
				for _, m in ipairs(methods) do
					method_names[m.name] = true
				end
				local exists: boolean? = method_names["greet"]
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "dynamic_key_assignment_widens_type",
			Code: `
				local t: {[string]: number} = {}
				local key: string = "foo"
				t[key] = 42
				local val: number = t["foo"]
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "empty_table_with_dynamic_insert",
			Code: `
				local counts = {}
				counts["a"] = 1
				counts["b"] = 2
				local n: number = counts["a"]
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}
