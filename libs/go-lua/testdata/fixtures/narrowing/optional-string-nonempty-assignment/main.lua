local function qualify_id(ns: string, relative_id: string): string
    return ns .. relative_id
end

local function build_page(raw: string?)
    local data_func = raw
    if data_func and data_func ~= "" then
        data_func = qualify_id("demo:", data_func)
    end

    local maybe_name: string? = data_func
    local page: {data_func: string?} = {
        data_func = data_func,
    }
    return page, maybe_name
end

return build_page
