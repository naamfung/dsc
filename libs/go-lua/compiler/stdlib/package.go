package stdlib

import "github.com/wippyai/go-lua/types/typ"

var packageMethods = typ.NewRecord().
	Field("config", typ.String).
	Field("cpath", typ.String).
	Field("loaded", typ.Any).
	Field("path", typ.String).
	Field("preload", typ.Any).
	Build()

// PackageLib provides types for Lua's module loading configuration.
var PackageLib typ.Type = packageMethods
