type Entry = {
    id: string,
    data: {[string]: any},
}

local pages = {}

local function qualify_id(entry_id, relative_id)
    return entry_id .. ":" .. relative_id
end

function pages.build_page(entry: Entry)
    local data_func = entry.data.data_func
    if data_func and data_func ~= "" then
        data_func = qualify_id(entry.id, data_func)
    end

    local page = {}
    page.data_func = data_func
    return page
end

return pages
