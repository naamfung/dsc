function counter(): () -> number
    local count = 0
    return function(): number
        count = count + 1
        return count
    end
end
