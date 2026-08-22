type Adder = {
    value: number,
    add: (self: Adder, n: number) -> number
}
local a: Adder = {
    value = 0,
    add = function(self: Adder, n: number): number
        self.value = self.value + n
        return self.value
    end
}
local r: number = a:add(5)
