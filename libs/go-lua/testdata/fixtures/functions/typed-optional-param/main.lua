local function log(msg: string, level: string?)
    local lvl = level or "INFO"
    print(lvl .. ": " .. msg)
end
log("hello")
log("hello", "DEBUG")
