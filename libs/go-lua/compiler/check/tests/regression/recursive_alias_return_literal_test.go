package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestRecursiveAliasReturnLiteralWithSelfMethod(t *testing.T) {
	source := `
		type Message = {
			_topic: string,
			topic: (self: Message) -> string,
		}

		local function make(): Message
			return {
				_topic = "test",
				topic = function(s: Message): string
					return s._topic
				end,
			}
		end

		local msg = make()
		local topic: string = msg:topic()
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestRecursiveAliasReturnLiteralNestedInRecordField(t *testing.T) {
	source := `
		type Message = {
			_topic: string,
			topic: (self: Message) -> string,
		}

		type MsgCh = {__tag: "msg"}
		type Result = {channel: MsgCh, value: Message, ok: boolean}

		local function select_fn(msg_ch: MsgCh): Result
			return {
				channel = msg_ch,
				value = {
					_topic = "test",
					topic = function(s: Message): string
						return s._topic
					end,
				},
				ok = true,
			}
		end

		local msg_ch: MsgCh = {__tag = "msg"}
		local result = select_fn(msg_ch)
		local topic: string = result.value:topic()
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
