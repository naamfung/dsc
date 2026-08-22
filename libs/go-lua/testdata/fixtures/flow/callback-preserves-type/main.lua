type Handler = fun(data: string): nil
local function process(items: {string}, handler: Handler)
    for _, item in ipairs(items) do
        handler(item)
    end
end
process({"a", "b"}, function(s: string)
    local upper: string = s:upper()
end)
