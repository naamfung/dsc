package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/io"
)

const toolHandlerProtocolModule = `
type AppError = {
    code: string,
    message: string,
    retryable: boolean,
}

type ToolCallMessage = {
    tool: string,
    arguments: {[string]: any},
}

type ToolResult = {
    tool: string,
    content: string,
    cached: boolean,
}

type SessionState = {
    flags: {[string]: boolean},
}

type ToolResultResult = {ok: true, value: ToolResult} | {ok: false, error: AppError}
type ToolHandler = (SessionState, ToolCallMessage) -> ToolResultResult

local M = {}
M.AppError = AppError
M.ToolCallMessage = ToolCallMessage
M.ToolResult = ToolResult
M.SessionState = SessionState
M.ToolResultResult = ToolResultResult
M.ToolHandler = ToolHandler

return M
`

const toolHandlerBuilderModule = `
local protocol = require("protocol")

type ToolResultResult = protocol.ToolResultResult

local M = {}

function M.build(): protocol.ToolHandler
    return function(state: protocol.SessionState, msg: protocol.ToolCallMessage): ToolResultResult
        local value = msg.arguments["value"]
        if type(value) ~= "string" then
            return {
                ok = false,
                error = {
                    code = "invalid",
                    message = "value must be string",
                    retryable = false,
                },
            }
        end

        if state.flags["flagged"] then
            return {
                ok = true,
                value = {
                    tool = msg.tool,
                    content = "flagged:" .. value,
                    cached = false,
                },
            }
        end

        return {
            ok = true,
            value = {
                tool = msg.tool,
                content = value,
                cached = false,
            },
        }
    end
end

return M
`

func exportModule(t *testing.T, name, source string, opts ...testutil.Option) *io.Manifest {
	t.Helper()

	baseOpts := []testutil.Option{testutil.WithStdlib()}
	baseOpts = append(baseOpts, opts...)
	result := testutil.CheckAndExport(source, name, baseOpts...)
	if result.HasError() {
		t.Fatalf("%s export failed: %v", name, result.Errors)
	}
	encoded, err := io.EncodeManifest(result.Manifest)
	if err != nil {
		t.Fatalf("EncodeManifest(%s) failed: %v", name, err)
	}
	decoded, err := io.DecodeManifest(encoded)
	if err != nil {
		t.Fatalf("DecodeManifest(%s) failed: %v", name, err)
	}
	return decoded
}

func TestToolHandlerReturnPrecisionAcrossRegistryLayers(t *testing.T) {
	protocol := exportModule(t, "protocol", toolHandlerProtocolModule)
	builder := exportModule(t, "builder", toolHandlerBuilderModule, testutil.WithManifest("protocol", protocol))

	opts := []testutil.Option{
		testutil.WithStdlib(),
		testutil.WithManifest("protocol", protocol),
		testutil.WithManifest("builder", builder),
	}

	t.Run("direct builder call", func(t *testing.T) {
		result := testutil.Check(`
local protocol = require("protocol")
local builder = require("builder")

local handler: protocol.ToolHandler = builder.build()
local out = handler({flags = {}}, {tool = "search", arguments = {value = "x"}})

if out.ok then
    local tool: string = out.value.tool
    local content: string = out.value.content
else
    local code: string = out.error.code
    local retryable: boolean = out.error.retryable
end
`, opts...)
		if result.HasError() {
			t.Fatalf("direct builder call lost precision: %v", testutil.ErrorMessages(result.Diagnostics))
		}
	})

	t.Run("map lookup call", func(t *testing.T) {
		result := testutil.Check(`
local protocol = require("protocol")
local builder = require("builder")

local handlers: {[string]: protocol.ToolHandler} = {
    search = builder.build(),
}

local handler = handlers["search"]
if not handler then
    return nil
end

local out = handler({flags = {}}, {tool = "search", arguments = {value = "x"}})
if out.ok then
    local tool: string = out.value.tool
    local content: string = out.value.content
else
    local code: string = out.error.code
    local retryable: boolean = out.error.retryable
end
`, opts...)
		if result.HasError() {
			t.Fatalf("map lookup call lost precision: %v", testutil.ErrorMessages(result.Diagnostics))
		}
	})

	t.Run("receiver field map lookup call", func(t *testing.T) {
		result := testutil.Check(`
local protocol = require("protocol")
local builder = require("builder")

type Engine = {
    handlers: {[string]: protocol.ToolHandler},
    run: (self: Engine, name: string) -> (),
}

local Engine = {}
Engine.__index = Engine

function Engine:run(name: string)
    local handler = self.handlers[name]
    if not handler then
        return nil
    end

    local out = handler({flags = {}}, {tool = name, arguments = {value = "x"}})
    if out.ok then
        local tool: string = out.value.tool
        local content: string = out.value.content
    else
        local code: string = out.error.code
        local retryable: boolean = out.error.retryable
    end
end

local e: Engine = {
    handlers = {search = builder.build()},
    run = Engine.run,
}

setmetatable(e, Engine)
e:run("search")
`, opts...)
		if result.HasError() {
			t.Fatalf("receiver field map lookup call lost precision: %v", testutil.ErrorMessages(result.Diagnostics))
		}
	})

	t.Run("receiver field map lookup with narrowed message", func(t *testing.T) {
		result := testutil.Check(`
local protocol = require("protocol")
local builder = require("builder")

type Message = {kind: "user", content: string} | protocol.ToolCallMessage

type Store = {
    state: protocol.SessionState,
}

type Engine = {
    handlers: {[string]: protocol.ToolHandler},
    run: (self: Engine, store: Store, msg: Message) -> (),
}

local Engine = {}
Engine.__index = Engine

function Engine:run(store: Store, msg: Message)
    if msg.kind ~= "tool_call" then
        return nil
    end

    local handler = self.handlers[msg.tool]
    if not handler then
        return nil
    end

    local out = handler(store.state, msg)
    if out.ok then
        local tool: string = out.value.tool
        local content: string = out.value.content
    else
        local code: string = out.error.code
        local retryable: boolean = out.error.retryable
    end
end

local e: Engine = {
    handlers = {search = builder.build()},
    run = Engine.run,
}

setmetatable(e, Engine)
e:run({state = {flags = {}}}, {kind = "tool_call", tool = "search", arguments = {value = "x"}})
`, opts...)
		if result.HasError() {
			t.Fatalf("receiver field map lookup with narrowed message lost precision: %v", testutil.ErrorMessages(result.Diagnostics))
		}
	})

	t.Run("split receiver field then map lookup with narrowed message", func(t *testing.T) {
		result := testutil.Check(`
local protocol = require("protocol")
local builder = require("builder")

type Message = {kind: "user", content: string} | protocol.ToolCallMessage

type Store = {
    state: protocol.SessionState,
}

type Engine = {
    handlers: {[string]: protocol.ToolHandler},
    run: (self: Engine, store: Store, msg: Message) -> (),
}

local Engine = {}
Engine.__index = Engine

function Engine:run(store: Store, msg: Message)
    if msg.kind ~= "tool_call" then
        return nil
    end

    local handlers = self.handlers
    local handler = handlers[msg.tool]
    if not handler then
        return nil
    end

    local out = handler(store.state, msg)
    if out.ok then
        local tool: string = out.value.tool
        local content: string = out.value.content
    else
        local code: string = out.error.code
        local retryable: boolean = out.error.retryable
    end
end

local e: Engine = {
    handlers = {search = builder.build()},
    run = Engine.run,
}

setmetatable(e, Engine)
e:run({state = {flags = {}}}, {kind = "tool_call", tool = "search", arguments = {value = "x"}})
`, opts...)
		if result.HasError() {
			t.Fatalf("split receiver field then map lookup with narrowed message lost precision: %v", testutil.ErrorMessages(result.Diagnostics))
		}
	})

	t.Run("direct builder call with narrowed message", func(t *testing.T) {
		result := testutil.Check(`
local protocol = require("protocol")
local builder = require("builder")

type Message = {kind: "user", content: string} | protocol.ToolCallMessage

local handler: protocol.ToolHandler = builder.build()
local msg: Message = {kind = "tool_call", tool = "search", arguments = {value = "x"}}

if msg.kind ~= "tool_call" then
    return nil
end

local out = handler({flags = {}}, msg)
if out.ok then
    local tool: string = out.value.tool
    local content: string = out.value.content
else
    local code: string = out.error.code
    local retryable: boolean = out.error.retryable
end
`, opts...)
		if result.HasError() {
			t.Fatalf("direct builder call with narrowed message lost precision: %v", testutil.ErrorMessages(result.Diagnostics))
		}
	})

	t.Run("map lookup call with narrowed message", func(t *testing.T) {
		result := testutil.Check(`
local protocol = require("protocol")
local builder = require("builder")

type Message = {kind: "user", content: string} | protocol.ToolCallMessage

local handlers: {[string]: protocol.ToolHandler} = {
    search = builder.build(),
}

local handler = handlers["search"]
local msg: Message = {kind = "tool_call", tool = "search", arguments = {value = "x"}}
if not handler or msg.kind ~= "tool_call" then
    return nil
end

local out = handler({flags = {}}, msg)
if out.ok then
    local tool: string = out.value.tool
    local content: string = out.value.content
else
    local code: string = out.error.code
    local retryable: boolean = out.error.retryable
end
`, opts...)
		if result.HasError() {
			t.Fatalf("map lookup call with narrowed message lost precision: %v", testutil.ErrorMessages(result.Diagnostics))
		}
	})

	t.Run("function param map lookup with narrowed message", func(t *testing.T) {
		result := testutil.Check(`
local protocol = require("protocol")
local builder = require("builder")

type Message = {kind: "user", content: string} | protocol.ToolCallMessage

local function run(handlers: {[string]: protocol.ToolHandler}, state: protocol.SessionState, msg: Message)
    if msg.kind ~= "tool_call" then
        return nil
    end

    local key: string = msg.tool
    local handler = handlers[msg.tool]
    if not handler then
        return nil
    end

    local out = handler(state, msg)
    if out.ok then
        local tool: string = out.value.tool
        local content: string = out.value.content
    else
        local code: string = out.error.code
        local retryable: boolean = out.error.retryable
    end
end

run({search = builder.build()}, {flags = {}}, {kind = "tool_call", tool = "search", arguments = {value = "x"}})
`, opts...)
		if result.HasError() {
			t.Fatalf("function param map lookup with narrowed message lost precision: %v", testutil.ErrorMessages(result.Diagnostics))
		}
	})

	t.Run("function param direct handler with narrowed message", func(t *testing.T) {
		result := testutil.Check(`
local protocol = require("protocol")
local builder = require("builder")

type Message = {kind: "user", content: string} | protocol.ToolCallMessage

local function run(handler: protocol.ToolHandler, state: protocol.SessionState, msg: Message)
    if msg.kind ~= "tool_call" then
        return nil
    end

    local out = handler(state, msg)
    if out.ok then
        local tool: string = out.value.tool
        local content: string = out.value.content
    else
        local code: string = out.error.code
        local retryable: boolean = out.error.retryable
    end
end

run(builder.build(), {flags = {}}, {kind = "tool_call", tool = "search", arguments = {value = "x"}})
`, opts...)
		if result.HasError() {
			t.Fatalf("function param direct handler with narrowed message lost precision: %v", testutil.ErrorMessages(result.Diagnostics))
		}
	})

	t.Run("function param map lookup with string key", func(t *testing.T) {
		result := testutil.Check(`
local protocol = require("protocol")
local builder = require("builder")

local function run(handlers: {[string]: protocol.ToolHandler}, state: protocol.SessionState, key: string)
    local handler = handlers[key]
    if not handler then
        return nil
    end

    local out = handler(state, {tool = key, arguments = {value = "x"}})
    if out.ok then
        local tool: string = out.value.tool
        local content: string = out.value.content
    else
        local code: string = out.error.code
        local retryable: boolean = out.error.retryable
    end
end

run({search = builder.build()}, {flags = {}}, "search")
`, opts...)
		if result.HasError() {
			t.Fatalf("function param map lookup with string key lost precision: %v", testutil.ErrorMessages(result.Diagnostics))
		}
	})
}
