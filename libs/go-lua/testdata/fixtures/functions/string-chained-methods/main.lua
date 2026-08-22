local function clean(s: string): string
    return s:lower():gsub("%s+", "")
end
