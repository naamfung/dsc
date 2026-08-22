package narrowing

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Simplest truthy guard - direct assignment after narrowing
func TestTruthyGuard_DirectAssignment(t *testing.T) {
	source := `
		local function f(obj: {from: string?})
			if obj.from then
				local s: string = obj.from
			end
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Table literal uses narrowed value via intermediate
func TestTruthyGuard_TableLiteralUsesNarrowedValue(t *testing.T) {
	source := `
		local function f(obj: {from: string?})
			if obj.from then
				local s: string = obj.from
				local t = {from = s}
				local f: string = t.from
			end
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Table literal with direct field access - tests that obj.from is narrowed when used as field value
func TestTruthyGuard_TableLiteralDirectNarrowing(t *testing.T) {
	// Same as SimpleNoLoop but checking the table construction type directly
	source := `
		local function f(obj: {from: string?})
			if obj.from then
				-- obj.from is narrowed to string here
				-- Table should be {from: string}, not {from: string?}
				local t: {from: string} = {from = obj.from}
			end
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Simple truthy guard without loop
func TestTruthyGuard_SimpleNoLoop(t *testing.T) {
	source := `
		local function f(obj: {from: string?})
			if obj.from then
				local t = {from = obj.from}
				local f: string = t.from
			end
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
