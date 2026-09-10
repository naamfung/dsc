local function greet(name, greeting)
    local msg = greeting or "Hello"
    return msg .. ", " .. name
end
greet("World")
