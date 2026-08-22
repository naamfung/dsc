package stdlib

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/typ"
)

// MathMinSpec defines the contract for math.min ensuring result equals minimum of inputs.
func MathMinSpec() *contract.Spec {
	return contract.NewSpec().
		WithExprEnsures(
			constraint.EqExpr(constraint.R(0), constraint.MinExpr(constraint.P(0), constraint.P(1))),
		)
}

// MathMaxSpec defines the contract for math.max ensuring result equals maximum of inputs.
func MathMaxSpec() *contract.Spec {
	return contract.NewSpec().
		WithExprEnsures(
			constraint.EqExpr(constraint.R(0), constraint.MaxExpr(constraint.P(0), constraint.P(1))),
		)
}

var mathMethods = typ.NewRecord().
	Field("abs", typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()).
	Field("acos", typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()).
	Field("asin", typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()).
	Field("atan", typ.Func().Param("y", typ.Number).OptParam("x", typ.Number).Returns(typ.Number).Build()).
	Field("atan2", typ.Func().Param("y", typ.Number).Param("x", typ.Number).Returns(typ.Number).Build()).
	Field("ceil", typ.Func().Param("x", typ.Number).Returns(typ.Integer).Build()).
	Field("cos", typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()).
	Field("cosh", typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()).
	Field("deg", typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()).
	Field("exp", typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()).
	Field("floor", typ.Func().Param("x", typ.Number).Returns(typ.Integer).Build()).
	Field("fmod", typ.Func().Param("x", typ.Number).Param("y", typ.Number).Returns(typ.Number).Build()).
	Field("frexp", typ.Func().Param("x", typ.Number).Returns(typ.Number, typ.Integer).Build()).
	Field("huge", typ.Number).
	Field("ldexp", typ.Func().Param("m", typ.Number).Param("e", typ.Integer).Returns(typ.Number).Build()).
	Field("log", typ.Func().Param("x", typ.Number).OptParam("base", typ.Number).Returns(typ.Number).Build()).
	Field("log10", typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()).
	Field("max", typ.Func().Param("x", typ.Number).Variadic(typ.Number).Returns(typ.Number).Spec(MathMaxSpec()).Build()).
	Field("maxinteger", typ.Integer).
	Field("min", typ.Func().Param("x", typ.Number).Variadic(typ.Number).Returns(typ.Number).Spec(MathMinSpec()).Build()).
	Field("mininteger", typ.Integer).
	Field("mod", typ.Func().Param("x", typ.Number).Param("y", typ.Number).Returns(typ.Number).Build()).
	Field("modf", typ.Func().Param("x", typ.Number).Returns(typ.Integer, typ.Number).Build()).
	Field("pi", typ.Number).
	Field("pow", typ.Func().Param("x", typ.Number).Param("y", typ.Number).Returns(typ.Number).Build()).
	Field("rad", typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()).
	Field("random", typ.Func().OptParam("m", typ.Integer).OptParam("n", typ.Integer).Returns(typ.Number).Build()).
	Field("randomseed", typ.Func().OptParam("x", typ.Integer).OptParam("y", typ.Integer).Build()).
	Field("sin", typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()).
	Field("sinh", typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()).
	Field("sqrt", typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()).
	Field("tan", typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()).
	Field("tanh", typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()).
	Field("tointeger", typ.Func().Param("x", typ.Any).Returns(typ.NewOptional(typ.Integer)).Build()).
	Field("type", typ.Func().Param("x", typ.Any).Returns(typ.NewOptional(typ.String)).Build()).
	Field("ult", typ.Func().Param("m", typ.Integer).Param("n", typ.Integer).Returns(typ.Boolean).Build()).
	Build()

// MathLib provides types for Lua's math library functions and constants.
var MathLib typ.Type = mathMethods
