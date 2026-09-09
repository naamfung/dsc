local function getData(): (string?, string?)
    return "data", nil
end
local data, err = getData()
local s: string = data -- expect-error
