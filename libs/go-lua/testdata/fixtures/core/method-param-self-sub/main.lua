type T = { eq: (self: T, other: T) -> boolean }
local t: T = { eq = function(self, other) return self == other end }
local ok = t:eq(t)
