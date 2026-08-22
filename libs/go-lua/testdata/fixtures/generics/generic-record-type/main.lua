type Box<T> = {value: T}
local b: Box<number> = {value = 42}
local n: number = b.value
