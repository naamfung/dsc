type SearchArgs = {query: string, limit: number?}
type FetchArgs = {url: string, method: string?}
type ComputeArgs = {expression: string}

type SearchTool = {type: "search", name: string, args: SearchArgs}
type FetchTool = {type: "fetch", name: string, args: FetchArgs}
type ComputeTool = {type: "compute", name: string, args: ComputeArgs}

type Tool = SearchTool | FetchTool | ComputeTool

type ToolResult = {
    tool_name: string,
    output: string,
    success: boolean,
}

local M = {}
M.Tool = Tool
M.ToolResult = ToolResult

function M.search(query: string, limit: number?): SearchTool
    return {type = "search", name = "web_search", args = {query = query, limit = limit}}
end

function M.fetch(url: string, method: string?): FetchTool
    return {type = "fetch", name = "http_fetch", args = {url = url, method = method}}
end

function M.compute(expr: string): ComputeTool
    return {type = "compute", name = "calculator", args = {expression = expr}}
end

return M
