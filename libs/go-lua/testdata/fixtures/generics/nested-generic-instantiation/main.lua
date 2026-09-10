type Box<T> = {value: T}
type DoubleBox<T> = Box<Box<T>>
local db: DoubleBox<number> = {value = {value = 42}}
local n: number = db.value.value
