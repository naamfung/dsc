local page_registry = require("page_registry")

local function takes_string(name: string)
    return name
end

local function get_page_data(page)
    if not page or not page.data_func or page.data_func == "" then
        return {}, nil
    end

    local name: string = page.data_func
    takes_string(page.data_func)
    return {}, nil
end

local page, err = page_registry.get("demo:home")
if err then
    return nil, err
end

return get_page_data(page)
