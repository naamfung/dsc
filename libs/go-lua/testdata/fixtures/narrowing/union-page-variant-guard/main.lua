type TemplatePage = {
    kind: "template",
    id: string,
    data_func: string?,
    template_set: string,
}

type ComponentPage = {
    kind: "component",
    id: string,
    url: string,
}

type Page = TemplatePage | ComponentPage

local function takes_string(name: string)
    return name
end

local function get_page_data(page: Page?)
    if not page or not page.data_func or page.data_func == "" then
        return {}, nil
    end

    local name: string = page.data_func
    takes_string(page.data_func)
    return {}, nil
end

return get_page_data
