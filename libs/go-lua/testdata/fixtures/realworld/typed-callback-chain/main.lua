local types = require("types")
local stream = require("stream")

local collected_chunks: {string} = {}
local collected_tools: {types.ToolCall} = {}
local final_result: types.StreamResult? = nil

type ContentEvent = {type: "content", data: string}
type ToolCallEvent = {type: "tool_call", id: string, name: string, arguments: {[string]: any}}
type DoneEvent = {type: "done", reason: string?, usage: types.Usage?}
type StreamEvent = ContentEvent | ToolCallEvent | DoneEvent

local events: {StreamEvent} = {
    {type = "content", data = "Hello "},
    {type = "content", data = "world"},
    {type = "tool_call", id = "t1", name = "search", arguments = {query = "test"}},
    {type = "done", reason = "end_turn", usage = {input_tokens = 10, output_tokens = 20}},
}

local result, err = stream.process(events, {
    on_content = function(chunk: string)
        table.insert(collected_chunks, chunk)
    end,
    on_tool_call = function(call: types.ToolCall)
        table.insert(collected_tools, call)
        local name: string = call.name
        local id: string = call.id
    end,
    on_done = function(result: types.StreamResult)
        final_result = result
        local content: string = result.content
        local tokens: number = result.usage.input_tokens
    end,
})

if result then
    local content: string = result.content
    local tool_count: number = #result.tool_calls
    local reason: string? = result.finish_reason
    local input: number = result.usage.input_tokens
    local output: number = result.usage.output_tokens
end
