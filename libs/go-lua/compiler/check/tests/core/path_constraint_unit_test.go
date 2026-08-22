package core

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// 9) Path Constraints

func TestPathConstraint_NestedFieldEquality(t *testing.T) {
	source := `
		type A = {a: {b: "x", data: string}}
		type B = {a: {b: "y", data: number}}

		function process(r: A | B)
			if r.a.b == "x" then
				local s: string = r.a.data
			else
				local n: number = r.a.data
			end
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for nested field equality, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestPathConstraint_NestedOptionalNilCheck(t *testing.T) {
	source := `
		type Data = {
			config: {
				value: string?
			}?
		}

		function process(d: Data)
			if d.config ~= nil then
				if d.config.value ~= nil then
					local s: string = d.config.value
				end
			end
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for nested optional nil check, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestPathConstraint_DeepPath(t *testing.T) {
	source := `
		type Config = {
			server: {
				host: string,
				port: number
			}
		}

		function get_port(c: Config): number
			return c.server.port
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for deep path, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestPathConstraint_IndexPath(t *testing.T) {
	source := `
		local t: {items: {name: string}[]} = {items = {{name = "a"}, {name = "b"}}}
		local name: string = t.items[1].name
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for index path, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestPathConstraint_FieldThenIndex(t *testing.T) {
	source := `
		type Data = {
			values: number[]
		}

		function get_first(d: Data): number
			return d.values[1]
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for field then index, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestPathConstraint_NestedUnionNarrowing(t *testing.T) {
	source := `
		type Inner = {ok: true, value: string} | {ok: false, error: string}
		type Outer = {result: Inner}

		function process(o: Outer)
			if o.result.ok then
				local v: string = o.result.value
			else
				local e: string = o.result.error
			end
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for nested union narrowing, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
