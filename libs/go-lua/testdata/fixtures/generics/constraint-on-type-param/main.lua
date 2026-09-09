type Printable = {tostring: (self: Printable) -> string}
local function print_it<T: Printable>(x: T): string
    return x:tostring()
end
