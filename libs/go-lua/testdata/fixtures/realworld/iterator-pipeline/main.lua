local iter = require("iter")

type User = {name: string, age: number, active: boolean}

local users: {User} = {
    {name = "Alice", age = 30, active = true},
    {name = "Bob", age = 17, active = true},
    {name = "Charlie", age = 25, active = false},
    {name = "Diana", age = 22, active = true},
}

local active = iter.filter(users, function(u: User): boolean
    return u.active
end)

local adults = iter.filter(active, function(u: User): boolean
    return u.age >= 18
end)

local names = iter.map(adults, function(u: User): string
    return u.name
end)

local total_age = iter.reduce(adults, function(acc: number, u: User): number
    return acc + u.age
end, 0)

local first_adult = iter.find(users, function(u: User): boolean
    return u.age >= 18 and u.active
end)

if first_adult then
    local name: string = first_adult.name
    local age: number = first_adult.age
end

local name_lengths = iter.map(names, function(n: string): number
    return #n
end)

local total_len: number = iter.reduce(name_lengths, function(acc: number, n: number): number
    return acc + n
end, 0)
