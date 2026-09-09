package stdlib

import (
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/typ"
)

var coroutineMethods = typ.NewRecord().
	Field("close", typ.Func().Param("co", typ.Any).Returns(typ.Boolean, typ.NewOptional(typ.Any)).Build()).
	Field("create", typ.Func().Param("f", typ.Any).Returns(typ.Any).Build()).
	Field("isyieldable", typ.Func().OptParam("co", typ.Any).Returns(typ.Boolean).Build()).
	Field("resume", typ.Func().Param("co", typ.Any).Variadic(typ.Any).Returns(typ.Boolean, typ.Any).Build()).
	Field("running", typ.Func().Returns(typ.Any, typ.Boolean).Build()).
	Field("spawn", typ.Func().
		Param("f", typ.Any).
		Spec(contract.NewSpec().WithCallback(0, &contract.CallbackSpec{Cardinality: contract.CardExactlyOnce})).
		Build()).
	Field("status", typ.Func().Param("co", typ.Any).Returns(typ.String).Build()).
	Field("wrap", typ.Func().Param("f", typ.Any).Returns(typ.Any).Build()).
	Field("yield", typ.Func().Variadic(typ.Any).Returns(typ.Any).Build()).
	Build()

// CoroutineLib provides types for Lua's coroutine management functions.
var CoroutineLib typ.Type = coroutineMethods
