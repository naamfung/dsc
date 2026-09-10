type PageInfo = {
    id: string,
    name: string,
    secure: boolean,
}

type PageDetail = PageInfo & {
    data_func: string?,
    template_set: string?,
}

local function takes_string(name: string)
    return name
end

local function get_page_data(page: PageDetail?)
    if not page or not page.data_func or page.data_func == "" then
        return {}, nil
    end

    local name: string = page.data_func
    takes_string(page.data_func)
    return {}, nil
end

return get_page_data
