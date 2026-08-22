type ContentPart = {type: string, text: string}
type Message = {role: string, content: {ContentPart}, name: string?}

type PromptBuilder = {
    _messages: {Message},
    system: (self: PromptBuilder, content: string) -> PromptBuilder,
    user: (self: PromptBuilder, content: string) -> PromptBuilder,
    assistant: (self: PromptBuilder, content: string) -> PromptBuilder,
    with_name: (self: PromptBuilder, name: string) -> PromptBuilder,
    build: (self: PromptBuilder) -> {Message},
    count: (self: PromptBuilder) -> number,
    clone: (self: PromptBuilder) -> PromptBuilder,
}

local function text(content: string): ContentPart
    return {type = "text", text = content}
end

local function add_message(builder: PromptBuilder, role: string, content: string, name: string?): PromptBuilder
    table.insert(builder._messages, {
        role = role,
        content = {text(content)},
        name = name
    })
    return builder
end

local M = {}

function M.new(): PromptBuilder
    local builder: PromptBuilder = {
        _messages = {},
        system = function(self: PromptBuilder, content: string): PromptBuilder
            return add_message(self, "system", content)
        end,
        user = function(self: PromptBuilder, content: string): PromptBuilder
            return add_message(self, "user", content)
        end,
        assistant = function(self: PromptBuilder, content: string): PromptBuilder
            return add_message(self, "assistant", content)
        end,
        with_name = function(self: PromptBuilder, name: string): PromptBuilder
            local last = self._messages[#self._messages]
            if last then
                last.name = name
            end
            return self
        end,
        build = function(self: PromptBuilder): {Message}
            return self._messages
        end,
        count = function(self: PromptBuilder): number
            return #self._messages
        end,
        clone = function(self: PromptBuilder): PromptBuilder
            local new_builder = M.new()
            for _, msg in ipairs(self._messages) do
                table.insert(new_builder._messages, msg)
            end
            return new_builder
        end,
    }
    return builder
end

return M
