package stdlib

import (
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/typ"
)

var stringUnpackSpec = contract.NewSpec().WithEffects(
	effect.Return{
		ReturnIndex: 0,
		Transform:   effect.StringUnpackValue{Format: effect.ParamRef{Index: 0}},
	},
)

var stringMethods = typ.NewRecord().
	Field("byte", typ.Func().
		Param("s", typ.String).
		OptParam("i", typ.Integer).
		OptParam("j", typ.Integer).
		Returns(typ.Integer).Build()).
	Field("char", typ.Func().
		Variadic(typ.Integer).
		Returns(typ.String).Build()).
	Field("dump", typ.Func().
		Param("function", typ.Any).
		OptParam("strip", typ.Boolean).
		Returns(typ.String).Build()).
	Field("find", typ.Func().
		Param("s", typ.String).
		Param("pattern", typ.String).
		OptParam("init", typ.Integer).
		OptParam("plain", typ.Boolean).
		Returns(typ.NewOptional(typ.Integer), typ.NewOptional(typ.Integer)).
		Spec(contract.NewSpec().WithEffects(effect.CorrelatedReturn{Indices: []int{0, 1}})).Build()).
	Field("format", typ.Func().
		Param("formatstring", typ.String).
		Variadic(typ.Any).
		Returns(typ.String).Build()).
	Field("gfind", typ.Func().
		Param("s", typ.String).
		Param("pattern", typ.String).
		Returns(typ.Any).Build()).
	Field("gmatch", typ.Func().
		Param("s", typ.String).
		Param("pattern", typ.String).
		Returns(typ.Any).Build()).
	Field("gsub", typ.Func().
		Param("s", typ.String).
		Param("pattern", typ.String).
		Param("repl", typ.Any).
		OptParam("n", typ.Integer).
		Returns(typ.String, typ.Integer).Build()).
	Field("len", typ.Func().
		Param("s", typ.String).
		Returns(typ.Integer).Build()).
	Field("lower", typ.Func().
		Param("s", typ.String).
		Returns(typ.String).Build()).
	Field("match", typ.Func().
		Param("s", typ.String).
		Param("pattern", typ.String).
		OptParam("init", typ.Integer).
		Returns(typ.NewOptional(typ.String)).Build()).
	Field("pack", typ.Func().
		Param("fmt", typ.String).
		Variadic(typ.Any).
		Returns(typ.String).Build()).
	Field("packsize", typ.Func().
		Param("fmt", typ.String).
		Returns(typ.Integer).Build()).
	Field("rep", typ.Func().
		Param("s", typ.String).
		Param("n", typ.Integer).
		OptParam("sep", typ.String).
		Returns(typ.String).Build()).
	Field("reverse", typ.Func().
		Param("s", typ.String).
		Returns(typ.String).Build()).
	Field("sub", typ.Func().
		Param("s", typ.String).
		Param("i", typ.Integer).
		OptParam("j", typ.Integer).
		Returns(typ.String).Build()).
	Field("unpack", typ.Func().
		Param("fmt", typ.String).
		Param("s", typ.String).
		OptParam("pos", typ.Integer).
		Returns(typ.Any).
		Spec(stringUnpackSpec).
		Build()).
	Field("upper", typ.Func().
		Param("s", typ.String).
		Returns(typ.String).Build()).
	Build()

// StringLib provides types for Lua's string library functions.
// These methods are also available on string values via method syntax.
var StringLib typ.Type = stringMethods
