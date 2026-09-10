package stdlib

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/typ"
)

// Iterator function contracts for flow analysis.
var (
	ipairsSpec = contract.NewSpec().WithEffects(
		effect.Iterator{Source: effect.ParamRef{Index: 0}, Kind: effect.IterateIndexed},
	)
	pairsSpec = contract.NewSpec().WithEffects(
		effect.Iterator{Source: effect.ParamRef{Index: 0}, Kind: effect.IterateKeyed},
	)
	pcallSpec = contract.NewSpec().
			WithCallback(0, &contract.CallbackSpec{Cardinality: contract.CardExactlyOnce}).
			WithEffects(effect.Return{
			ReturnIndex: 1,
			Transform:   effect.CallbackReturn{CallbackParam: effect.ParamRef{Index: 0}},
		})
	xpcallSpec = contract.NewSpec().
			WithCallback(0, &contract.CallbackSpec{Cardinality: contract.CardExactlyOnce}).
			WithCallback(1, &contract.CallbackSpec{Cardinality: contract.CardAtMostOnce}).
			WithEffects(effect.Return{
			ReturnIndex: 1,
			Transform:   effect.CallbackReturn{CallbackParam: effect.ParamRef{Index: 0}},
		})
)

// Global function type definitions.
var (
	// assert(v, message?) -> v
	// When assert returns, v is truthy (not nil/false)
	Assert = typ.Func().
		Param("v", typ.Any).
		OptParam("message", typ.String).
		Returns(typ.Any).
		Effects(effect.Throws()).
		WithRefinement(&constraint.FunctionRefinement{
			OnReturn: constraint.FromConstraints(
				constraint.Truthy{Path: constraint.ParamPath(0)},
			),
		}).
		Build()

	// error(message, level?) -> never
	Error = typ.Func().
		Param("message", typ.Any).
		OptParam("level", typ.Number).
		Returns(typ.Never).
		Effects(effect.Throws()).
		Build()

	// getmetatable(object) -> table?
	GetMetatable = typ.Func().
			Param("object", typ.Any).
			Returns(typ.NewOptional(typ.Any)).
			Build()

	// setmetatable(table, metatable) -> table
	SetMetatable = func() typ.Type {
		tp := typ.NewTypeParam("T", nil)
		return typ.Func().
			TypeParam("T", nil).
			Param("table", tp).
			Param("metatable", typ.NewOptional(typ.Any)).
			Returns(tp).
			Effects(effect.StoresParam(1, 0)).
			Build()
	}()

	// ipairs(t) -> iterator, t, 0
	Ipairs = typ.Func().
		Param("t", typ.Any).
		Returns(typ.Any, typ.Any, typ.Integer).
		Spec(ipairsSpec).
		Build()

	// pairs(t) -> iterator, t, nil
	Pairs = typ.Func().
		Param("t", typ.Any).
		Returns(typ.Any, typ.Any, typ.Nil).
		Spec(pairsSpec).
		Build()

	// next(table, index?) -> key?, value?
	Next = typ.Func().
		Param("table", typ.Any).
		OptParam("index", typ.Any).
		Returns(typ.NewOptional(typ.Any), typ.NewOptional(typ.Any)).
		Build()

	// pcall(f, ...) -> success, result...
	Pcall = typ.Func().
		Param("f", typ.Any).
		Variadic(typ.Any).
		Returns(typ.Boolean, typ.Any).
		Effects(effect.BorrowsOnly()).
		Spec(pcallSpec).
		Build()

	// cpcall(f, ...) -> success, result... (coroutine-safe pcall)
	Cpcall = typ.Func().
		Param("f", typ.Any).
		Variadic(typ.Any).
		Returns(typ.Boolean, typ.Any).
		Effects(effect.BorrowsOnly()).
		Spec(pcallSpec).
		Build()

	// xpcall(f, msgh, ...) -> success, result...
	Xpcall = typ.Func().
		Param("f", typ.Any).
		Param("msgh", typ.Any).
		Variadic(typ.Any).
		Returns(typ.Boolean, typ.Any).
		Effects(effect.BorrowsOnly()).
		Spec(xpcallSpec).
		Build()

	// print(...) -> nil
	Print = typ.Func().
		Variadic(typ.Any).
		Effects(effect.BorrowsOnly().With(effect.IO{})).
		Build()

	// type(v) -> string
	Type = typ.Func().
		Param("v", typ.Any).
		Returns(typ.String).
		Effects(effect.BorrowsOnly().With(effect.TypePredicate{})).
		Build()

	// tostring(v) -> string
	ToString = typ.Func().
			Param("v", typ.Any).
			Returns(typ.String).
			Effects(effect.BorrowsOnly()).
			Build()

	// tonumber(v, base?) -> number?
	ToNumber = typ.Func().
			Param("v", typ.Any).
			OptParam("base", typ.Integer).
			Returns(typ.NewOptional(typ.Number)).
			Effects(effect.BorrowsOnly()).
			Build()

	// number(v) -> number
	Number = typ.Func().
		Param("v", typ.Any).
		Returns(typ.Number).
		Effects(effect.BorrowsOnly()).
		Build()

	// integer(v) -> integer
	Integer = typ.Func().
		Param("v", typ.Any).
		Returns(typ.Integer).
		Effects(effect.BorrowsOnly()).
		Build()

	// rawequal(v1, v2) -> boolean
	RawEqual = typ.Func().
			Param("v1", typ.Any).
			Param("v2", typ.Any).
			Returns(typ.Boolean).
			Effects(effect.BorrowsOnly()).
			Build()

	// rawget(table, index) -> any
	RawGet = typ.Func().
		Param("table", typ.Any).
		Param("index", typ.Any).
		Returns(typ.Any).
		Effects(effect.BorrowsOnly()).
		Build()

	// rawset(table, index, value) -> table
	RawSet = typ.Func().
		Param("table", typ.Any).
		Param("index", typ.Any).
		Param("value", typ.Any).
		Returns(typ.Any).
		Effects(effect.StoresParam(2, 0)).
		Build()

	// rawlen(v) -> integer
	RawLen = typ.Func().
		Param("v", typ.Any).
		Returns(typ.Integer).
		Effects(effect.BorrowsOnly()).
		Build()

	// select(index, ...) -> ...
	Select = typ.NewUnion(
		typ.Func().
			Param("index", typ.LiteralString("#")).
			Variadic(typ.Any).
			Returns(typ.Integer).
			Effects(effect.WithVariadicTransform()).
			Build(),
		typ.Func().
			Param("index", typ.Integer).
			Variadic(typ.Any).
			Returns(typ.Any).
			Effects(effect.WithVariadicTransform()).
			Build(),
	)

	// require(modname) -> any
	Require = typ.Func().
		Param("modname", typ.String).
		Returns(typ.Any).
		Effects(effect.WithModuleLoad().With(effect.Throw{})).
		Build()

	// unpack(list, i?, j?) -> ...
	Unpack = typ.Func().
		Param("list", typ.Any).
		OptParam("i", typ.Integer).
		OptParam("j", typ.Integer).
		Returns(typ.Any).
		Effects(effect.BorrowsOnly()).
		Build()

	// dofile(filename?) -> ...
	DoFile = typ.Func().
		OptParam("filename", typ.String).
		Returns(typ.Any).
		Effects(effect.Throws().With(effect.IO{})).
		Build()

	// collectgarbage(opt?, arg?) -> any
	CollectGarbage = typ.Func().
			OptParam("opt", typ.String).
			OptParam("arg", typ.Any).
			Returns(typ.Any).
			Build()
)
