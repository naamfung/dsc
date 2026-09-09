package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Regression guard: after a local optional union is proven non-nil, a later
// discriminant check must still narrow the whole value to the matching variant.
func TestOptionalDiscriminantNarrowingSupportsVariantBinding(t *testing.T) {
	result := testutil.Check(`
type Allow = {kind: "allow", reason: string}
type Deny = {kind: "deny", reason: string}
type Defer = {kind: "defer", queue: string}
type Decision = Allow | Deny | Defer
type Outcome = {ok: true, value: Decision?} | {ok: false, error: string}

local outcome: Outcome = {
	ok = true,
	value = {
		kind = "defer",
		queue = "review",
	},
}

if not outcome.ok then
	return
end

local decision = outcome.value
if not decision then
	return
end

if decision.kind == "defer" then
	local deferred: Defer = decision
	local queue: string = decision.queue
	end
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected optional discriminant binding to narrow variant, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
