type PageInfo = {
    id: string,
    name: string,
    secure: boolean,
}

type PageDetail = PageInfo & {
    data_func: string?,
}

local function qualify_id(ns: string, relative_id: string): string
    return ns .. relative_id
end

local function build_page(raw: string?): PageDetail
    local data_func = raw
    if data_func and data_func ~= "" then
        data_func = qualify_id("demo:", data_func)
    end

    local page = {
        id = "p",
        name = "n",
        secure = false,
    }
    page.data_func = data_func

    local typed: PageDetail = page
    local maybe_name: string? = page.data_func
    return typed
end

return build_page
