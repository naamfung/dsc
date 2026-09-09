package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// Regression guard for the framework test helper pattern:
// a local any-typed function variable is initialized from process.send,
// checked for truthiness, and called from nested closures.
func TestProcessSendProxy_AnyTypedGuardedCallsRemainCallable(t *testing.T) {
	processManifest := io.NewManifest("process")
	processManifest.SetExport(typ.NewRecord().
		Field("pid", typ.String).
		Field("send", typ.Func().
			Param("pid", typ.String).
			Param("topic", typ.String).
			Param("payload", typ.Any).
			Returns(typ.Boolean).
			Build()).
		Build())

	source := `
local _original_process_send: any = nil
if process and process.send then
	_original_process_send = process.send
end

local _default_context = {
	message_topic = "test:update",
	target_pid = "pid-1",
}

local function create_process_send_proxy(replacement)
	return function(pid, topic, payload)
		if topic == _default_context.message_topic or topic:match("^test:") then
			if _original_process_send then
				return _original_process_send(pid, topic, payload)
			end
		end
		return replacement(pid, topic, payload)
	end
end

local function _update_send_message_function()
	if _default_context.target_pid and _original_process_send then
		_original_process_send(_default_context.target_pid, _default_context.message_topic, {
			type = "ping",
			data = {}
		})
	end
end

local function setup_process_integration(options)
	if not process or not process.pid then
		return false
	end
	if type(options) ~= "table" or not options.pid then
		return false
	end
	_default_context.target_pid = options.pid

	if not _original_process_send and process.send then
		_original_process_send = process.send
	end

	_update_send_message_function()
	return true
end

local proxy = create_process_send_proxy(function(pid, topic, payload)
	return true
end)

setup_process_integration({ pid = "pid-3" })
_update_send_message_function()
proxy("pid-2", "test:case:start", {})
`

	result := testutil.Check(source,
		testutil.WithStdlib(),
		testutil.WithManifest("process", processManifest),
	)

	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityError {
			t.Logf("error at line %d: %s", d.Position.Line, d.Message)
		}
	}

	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
