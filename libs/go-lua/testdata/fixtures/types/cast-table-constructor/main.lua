local raw: any = {name = "test", count = 42}
local config: {name: string, count: integer} = {
    name = tostring(raw.name),
    count = integer(raw.count)
}
