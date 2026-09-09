local page_registry = require("page_registry")

type PageResponse = {
    id: string,
    configOverrides: {[string]: any}?,
}

local all_pages = page_registry.find_all()
local page = all_pages[1]

local page_info: PageResponse = {
    id = page.id,
    configOverrides = page.config_overrides :: {[string]: any}?,
}

local _ = page_info
