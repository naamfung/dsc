type Address = {street: string, city: string}
type Person = {name: string, address: Address}
local data: any = {name = "Alice", address = {street = "123 Main", city = "NYC"}}
local p = Person(data)
local name = p.name
local city = p.address.city
