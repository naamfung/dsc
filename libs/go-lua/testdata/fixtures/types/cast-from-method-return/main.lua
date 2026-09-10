type Data = {value: string}
local obj = {
    getData = function(self): any
        return {value = "test"}
    end
}
local d = Data(obj:getData())
local v = d.value
