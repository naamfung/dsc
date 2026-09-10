package core

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// TestScope_Shadowing tests that variable shadowing works correctly.
func TestScope_Shadowing(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "inner variable shadows outer",
			Code: `
				local x: number = 10
				do
					local x: string = "hello"
					local len: number = #x
				end
				local y: number = x + 1
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "shadowed variable type independent",
			Code: `
				local x = 10
				do
					local x = "hello"
					-- x is string here
				end
				-- x is number here, not string
				local y: number = x
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "function shadows outer variable",
			Code: `
				local x: number = 10
				local function f()
					local x: string = "hello"
					local len = #x
				end
				local y: number = x
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestScope_RequireShadowing tests that require properly shadows module names.
func TestScope_RequireShadowing(t *testing.T) {
	// Create http module (server-side, takes 0-1 args)
	httpManifest := io.NewManifest("http")
	httpType := typ.NewInterface("http", []typ.Method{
		{Name: "request", Type: typ.Func().OptParam("config", typ.Any).Returns(typ.Any).Build()},
	})
	httpManifest.SetExport(httpType)

	// Create http_client module (takes 2-3 args)
	httpClientManifest := io.NewManifest("http_client")
	httpClientType := typ.NewInterface("http_client", []typ.Method{
		{Name: "request", Type: typ.Func().Param("method", typ.String).Param("url", typ.String).OptParam("opts", typ.Any).Returns(typ.Any).Build()},
	})
	httpClientManifest.SetExport(httpClientType)

	source := `
		local http = require("http_client")
		local resp = http.request("OPTIONS", "http://localhost/test", {})
	`

	result := testutil.Check(source,
		testutil.WithStdlib(),
		testutil.WithManifest("http", httpManifest),
		testutil.WithManifest("http_client", httpClientManifest),
	)

	if result.HasError() {
		for _, d := range result.Errors {
			t.Logf("error: %s", d.Message)
		}
		t.Errorf("require shadowing should work correctly")
	}
}

// TestScope_BlockIsolation tests that variables in blocks don't leak out.
func TestScope_BlockIsolation(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "if block isolates variables",
			Code: `
				if true then
					local x = 1
				end
				local y: number = x
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "while block isolates variables",
			Code: `
				while false do
					local x = 1
				end
				local y: number = x
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "for block isolates variables",
			Code: `
				for i = 1, 10 do
					local x = 1
				end
				local y: number = x
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "for loop variable isolated",
			Code: `
				for i = 1, 10 do
				end
				local y: number = i
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "do block isolates variables",
			Code: `
				do
					local x = 1
				end
				local y: number = x
			`,
			WantError: true,
			Stdlib:    true,
		},
		{
			Name: "nested blocks chain correctly",
			Code: `
				local x = 1
				do
					local y = 2
					do
						local z = 3
						local sum: number = x + y + z
					end
					-- z not visible here
					local sum2: number = x + y
				end
				-- y not visible here
				local sum3: number = x
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestScope_ClosureCapture tests that closures capture outer variables correctly.
func TestScope_ClosureCapture(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "closure captures outer variable",
			Code: `
				local x: number = 10
				local function f(): number
					return x
				end
				local y: number = f()
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "closure modifies outer variable",
			Code: `
				local x: number = 0
				local function increment()
					x = x + 1
				end
				increment()
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "nested closure captures",
			Code: `
				local x: number = 10
				local function outer(): () -> number
					local y: number = 20
					return function(): number
						return x + y
					end
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}
