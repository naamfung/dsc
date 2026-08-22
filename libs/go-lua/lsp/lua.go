package lsp

// LuaKeywords contains all Lua reserved words.
var LuaKeywords = []string{
	"and", "break", "do", "else", "elseif", "end",
	"false", "for", "function", "goto", "if", "in",
	"local", "nil", "not", "or", "repeat", "return",
	"then", "true", "until", "while",
}

// LuaKeywordSet provides O(1) keyword lookup.
var LuaKeywordSet = func() map[string]bool {
	m := make(map[string]bool, len(LuaKeywords))
	for _, kw := range LuaKeywords {
		m[kw] = true
	}
	return m
}()

// IsLuaKeyword returns true if name is a Lua reserved word.
func IsLuaKeyword(name string) bool {
	return LuaKeywordSet[name]
}

// LuaBuiltins contains standard Lua built-in functions.
var LuaBuiltins = map[string]bool{
	"_G": true, "_ENV": true, "_VERSION": true,
	"print": true, "error": true, "assert": true,
	"type": true, "pairs": true, "ipairs": true,
	"next": true, "select": true, "tonumber": true,
	"tostring": true, "pcall": true, "xpcall": true,
	"require": true, "load": true, "loadfile": true,
	"dofile": true, "rawget": true, "rawset": true,
	"rawequal": true, "rawlen": true, "setmetatable": true,
	"getmetatable": true, "collectgarbage": true,
}

// IsLuaBuiltin returns true if name is a Lua built-in.
func IsLuaBuiltin(name string) bool {
	return LuaBuiltins[name]
}
