type PageInfo = {
    id: string,
    name: string,
    secure: boolean,
}

type PageDetail = PageInfo & {
    data_func: string?,
    template_set: string?,
}

local pages = {}

local function qualify_id(ns: string, relative_id: string): string
    return ns .. ":" .. relative_id
end

function pages.get(page_id: string): (PageDetail?, string?)
    if not page_id then
        return nil, "Page ID is required"
    end

    local data_func: string? = "load_data"
    if data_func and data_func ~= "" then
        data_func = qualify_id("demo", data_func)
    end

    local page = {
        id = page_id,
        name = "Demo Page",
        secure = false,
        template_set = "demo:set",
    }
    page.data_func = data_func

    return page, nil
end

return pages
