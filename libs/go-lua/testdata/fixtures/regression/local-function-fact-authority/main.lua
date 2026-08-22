type Entry = {id: string, meta: {type: string, suite: string?, order: number?}?}

local function make()
    local obj = { x = 1 }
    local function init()
        obj.get_x = function(self): number
            return self.x
        end
    end
    init()
    return obj
end

local built = make()
local n: number = built:get_x()

local function make_async()
    local obj = {}
    coroutine.spawn(function()
        obj.get_value = function(self): number
            return 42
        end
    end)
    return obj
end

local async_obj = make_async()
local v: number = async_obj:get_value()

local function sorted_keys(t)
    local keys = {}
    for k in pairs(t) do
        table.insert(keys, k)
    end
    table.sort(keys)
    return keys
end

local function group_by_suite(entries: {Entry})
    local suites = {}
    local no_suite = {}

    for _, entry in ipairs(entries) do
        local suite = entry.meta and entry.meta.suite
        if suite then
            suites[suite] = suites[suite] or {}
            table.insert(suites[suite], entry)
        else
            table.insert(no_suite, entry)
        end
    end

    return suites, no_suite
end

local entries: {Entry} = {}
local suites, no_suite = group_by_suite(entries)
local suite_names = sorted_keys(suites)

for _, name in ipairs(suite_names) do
    local tests: {Entry} = suites[name]
end

local uncategorized: {Entry} = no_suite
