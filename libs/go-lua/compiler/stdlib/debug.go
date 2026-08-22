package stdlib

import "github.com/wippyai/go-lua/types/typ"

var debugMethods = typ.NewRecord().
	Field("debug", typ.Func().Build()).
	Field("gethook", typ.Func().OptParam("thread", typ.Any).Returns(typ.Any, typ.Any, typ.Any).Build()).
	Field("getinfo", typ.Func().Param("f", typ.Any).OptParam("what", typ.String).OptParam("thread", typ.Any).Returns(typ.NewOptional(typ.Any)).Build()).
	Field("getlocal", typ.Func().Param("level", typ.Integer).Param("local", typ.Integer).Returns(typ.NewOptional(typ.String), typ.Any).Build()).
	Field("getmetatable", typ.Func().Param("value", typ.Any).Returns(typ.NewOptional(typ.Any)).Build()).
	Field("getregistry", typ.Func().Returns(typ.Any).Build()).
	Field("getupvalue", typ.Func().Param("f", typ.Any).Param("up", typ.Integer).Returns(typ.NewOptional(typ.String), typ.Any).Build()).
	Field("getuservalue", typ.Func().Param("u", typ.Any).Param("n", typ.Integer).Returns(typ.Any, typ.Boolean).Build()).
	Field("sethook", typ.Func().Param("hook", typ.Any).Param("mask", typ.String).OptParam("count", typ.Integer).Build()).
	Field("setlocal", typ.Func().Param("level", typ.Integer).Param("local", typ.Integer).Param("value", typ.Any).Returns(typ.NewOptional(typ.String)).Build()).
	Field("setmetatable", func() typ.Type {
		tp := typ.NewTypeParam("T", nil)
		return typ.Func().
			TypeParam("T", nil).
			Param("value", tp).
			Param("table", typ.NewOptional(typ.Any)).
			Returns(tp).
			Build()
	}()).
	Field("setupvalue", typ.Func().Param("f", typ.Any).Param("up", typ.Integer).Param("value", typ.Any).Returns(typ.NewOptional(typ.String)).Build()).
	Field("setuservalue", typ.Func().Param("udata", typ.Any).Param("value", typ.Any).Param("n", typ.Integer).Returns(typ.Any).Build()).
	Field("traceback", typ.Func().OptParam("thread", typ.Any).OptParam("message", typ.Any).OptParam("level", typ.Integer).Returns(typ.String).Build()).
	Field("upvalueid", typ.Func().Param("f", typ.Any).Param("n", typ.Integer).Returns(typ.Any).Build()).
	Field("upvaluejoin", typ.Func().Param("f1", typ.Any).Param("n1", typ.Integer).Param("f2", typ.Any).Param("n2", typ.Integer).Build()).
	Build()

// DebugLib provides types for Lua's debug and introspection functions.
var DebugLib typ.Type = debugMethods
