type Predicate<T> = (item: T) -> boolean
type Mapper<T, U> = (item: T) -> U
type Reducer<T, A> = (acc: A, item: T) -> A

local M = {}

function M.filter<T>(arr: {T}, pred: Predicate<T>): {T}
    local result: {T} = {}
    for _, item in ipairs(arr) do
        if pred(item) then
            table.insert(result, item)
        end
    end
    return result
end

function M.map<T, U>(arr: {T}, fn: Mapper<T, U>): {U}
    local result: {U} = {}
    for i, item in ipairs(arr) do
        result[i] = fn(item)
    end
    return result
end

function M.reduce<T, A>(arr: {T}, fn: Reducer<T, A>, initial: A): A
    local acc = initial
    for _, item in ipairs(arr) do
        acc = fn(acc, item)
    end
    return acc
end

function M.find<T>(arr: {T}, pred: Predicate<T>): T?
    for _, item in ipairs(arr) do
        if pred(item) then
            return item
        end
    end
    return nil
end

function M.each<T>(arr: {T}, fn: (item: T) -> ())
    for _, item in ipairs(arr) do
        fn(item)
    end
end

function M.flat_map<T, U>(arr: {T}, fn: (item: T) -> {U}): {U}
    local result: {U} = {}
    for _, item in ipairs(arr) do
        for _, sub in ipairs(fn(item)) do
            table.insert(result, sub)
        end
    end
    return result
end

return M
