package core

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestDiag_SimpleFieldAssign(t *testing.T) {
	source := `
		local function create()
			local obj = {}
			obj._name = "hello"
			return obj
		end
		local o = create()
		local n = o._name
	`
	result := testutil.Check(source, testutil.WithStdlib())
	for _, d := range result.Diagnostics {
		t.Logf("diag: [%s] %s (line %d)", d.Code.Name(), d.Message, d.Position.Line)
	}
	if result.HasError() {
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestDiag_FieldAccessOnSelf(t *testing.T) {
	source := `
		local function create(name: string)
			local obj = {}
			obj._name = name
			function obj:get_name()
				return self._name
			end
			return obj
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	for _, d := range result.Diagnostics {
		t.Logf("diag: [%s] %s (line %d)", d.Code.Name(), d.Message, d.Position.Line)
	}
	if result.HasError() {
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestDiag_NilFieldNarrow(t *testing.T) {
	source := `
		local function create()
			local obj = {}
			obj._items = nil
			function obj:check()
				if self._items then
					local first = self._items[1]
				end
			end
			return obj
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	for _, d := range result.Diagnostics {
		t.Logf("diag: [%s] %s (line %d)", d.Code.Name(), d.Message, d.Position.Line)
	}
	if result.HasError() {
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
