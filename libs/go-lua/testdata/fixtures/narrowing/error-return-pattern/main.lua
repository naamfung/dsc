local function getData(): (string?, string?)
    return "data", nil
end
local data, err = getData()
if err then
    error(err)
end
local s: string = data
