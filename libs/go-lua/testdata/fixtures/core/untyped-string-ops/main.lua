local function green(s) return "\027[32m" .. s .. "\027[0m" end
local function greet(name)
    if name and #name > 0 then
        return "Hello, " .. name
    end
    return green("stranger")
end
