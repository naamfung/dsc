package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Regression guard: direct dynamic index assignment should infer map key/value
// shape so pairs() keys preserve concrete key types.
func TestIndexedMapPairsKeyInference_DirectIndex(t *testing.T) {
	source := `
		local subscribers = {}
		local cid = tostring("c1")

		if not subscribers[cid] then
			subscribers[cid] = {}
		end

		local subs = subscribers[tostring(cid)]
		if subs then
			local function send(p: string) end
			for pid, _ in pairs(subs) do
				send(pid)
			end
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors for direct dynamic index map inference, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Regression guard: nested dynamic index assignment should widen outer map
// value to a keyed table and keep pairs() key type for downstream calls.
func TestIndexedMapPairsKeyInference_NestedIndex(t *testing.T) {
	source := `
		local subscribers = {}
		local cid = tostring("c1")
		local sub_pid = tostring("p1")

		if not subscribers[cid] then
			subscribers[cid] = {}
		end
		subscribers[cid][sub_pid] = true

		local subs = subscribers[tostring(cid)]
		if subs then
			local function send(p: string) end
			for pid, _ in pairs(subs) do
				send(pid)
			end
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors for nested dynamic index map inference, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Regression guard: inside loop/fixpoint branches, map-element reads should
// derive from predecessor flow state (not placeholder fallbacks) so pairs()
// keeps concrete key types.
func TestIndexedMapPairsKeyInference_LoopFixpoint(t *testing.T) {
	source := `
		local subscribers = {}
		while true do
			local topic = "" :: any
			if topic == "sub" then
				local payload = {} :: any
				local cid = tostring(payload.container_id)
				local sub_pid = tostring(payload.pid)
				if not subscribers[cid] then
					subscribers[cid] = {}
				end
				subscribers[cid][sub_pid] = true
			elseif topic == "log" then
				local payload = {} :: any
				local subs = subscribers[tostring(payload.container_id)]
				if subs then
					local function send(p: string) end
					for pid, _ in pairs(subs) do
						send(pid)
					end
				end
			end
			break
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors for loop fixpoint map-read inference, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
