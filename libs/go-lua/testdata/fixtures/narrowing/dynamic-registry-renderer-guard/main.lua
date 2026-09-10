local page_registry = require("page_registry")

local function takes_string(name: string)
    return name
end

local function get_page_data(page)
    if not page or not page.data_func or page.data_func == "" then
        return {}, nil
    end

    local name: string = page.data_func -- expect-error: cannot assign string | true to string
    takes_string(page.data_func) -- expect-error: argument 1: expected string, got string | true
    return {}, nil
end

local page = page_registry.build_page({
    id = "demo",
    data = { data_func = "load_data" },
})

return get_page_data(page) -- expect-error: expected {data_func?: boolean | string
