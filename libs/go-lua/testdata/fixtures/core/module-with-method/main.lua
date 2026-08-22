local Counter = {count = 0}
function Counter:increment()
    self.count = self.count + 1
end
function Counter:get(): number
    return self.count
end
Counter:increment()
local n: number = Counter:get()
