package stdlib

import "github.com/wippyai/go-lua/types/typ"

var utf8Methods = typ.NewRecord().
	Field("char", typ.Func().Variadic(typ.Integer).Returns(typ.String).Build()).
	Field("charpattern", typ.String).
	Field("codepoint", typ.Func().Param("s", typ.String).OptParam("i", typ.Integer).OptParam("j", typ.Integer).Returns(typ.Integer).Build()).
	Field("codes", typ.Func().Param("s", typ.String).Returns(typ.Any).Build()).
	Field("len", typ.Func().Param("s", typ.String).OptParam("i", typ.Integer).OptParam("j", typ.Integer).Returns(typ.NewOptional(typ.Integer), typ.NewOptional(typ.Integer)).Build()).
	Field("offset", typ.Func().Param("s", typ.String).Param("n", typ.Integer).OptParam("i", typ.Integer).Returns(typ.NewOptional(typ.Integer)).Build()).
	Build()

// UTF8Lib provides types for Lua's UTF-8 encoding functions.
var UTF8Lib typ.Type = utf8Methods
