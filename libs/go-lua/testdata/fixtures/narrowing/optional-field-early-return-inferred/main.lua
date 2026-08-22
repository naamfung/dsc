type Page = {
    data_func: string?,
}

local function load_page(): (Page?, string?)
    return { data_func = "demo" }, nil
end

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

local page, err = load_page()
if err then
    return nil, err
end

return get_page_data(page)
