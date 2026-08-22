local function process(data, callback)
    if callback ~= nil then
        callback(data)
    end
end
process("test")
