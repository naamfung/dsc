local tools = require("tools")
local executor = require("executor")

local search = tools.search("lua type system", 5)
local fetch = tools.fetch("https://example.com/api", "POST")
local compute = tools.compute("2 + 2")

local search_result = executor.execute(search)
local sr_output: string = search_result.output
local sr_success: boolean = search_result.success

local batch_results = executor.execute_batch({search, fetch, compute})
for _, result in ipairs(batch_results) do
    local name: string = result.tool_name
    local output: string = result.output
    local ok: boolean = result.success
end
