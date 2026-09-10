local function parse_pair(s: string): (string, string)
    local k, v = s:match("(%w+)=(%w+)")
    return k or "", v or ""
end
