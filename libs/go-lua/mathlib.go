package lua

import (
	"math"
	"math/rand/v2"
)

func OpenMath(L *LState) int {
	mod := L.RegisterGoModule(MathLibName, mathFuncs).(*LTable)
	mod.RawSetString("pi", LNumber(math.Pi))
	mod.RawSetString("huge", LNumber(math.MaxFloat64))
	mod.RawSetString("maxinteger", LInteger(math.MaxInt64))
	mod.RawSetString("mininteger", LInteger(math.MinInt64))
	L.Push(mod)
	return 1
}

var mathFuncs = map[string]LGoFunc{
	"abs":        mathAbs,
	"acos":       mathAcos,
	"asin":       mathAsin,
	"atan":       mathAtan,
	"atan2":      mathAtan2,
	"ceil":       mathCeil,
	"cos":        mathCos,
	"cosh":       mathCosh,
	"deg":        mathDeg,
	"exp":        mathExp,
	"floor":      mathFloor,
	"fmod":       mathFmod,
	"frexp":      mathFrexp,
	"ldexp":      mathLdexp,
	"log":        mathLog,
	"log10":      mathLog10,
	"max":        mathMax,
	"min":        mathMin,
	"mod":        mathMod,
	"modf":       mathModf,
	"pow":        mathPow,
	"rad":        mathRad,
	"random":     mathRandom,
	"randomseed": mathRandomSeed,
	"sin":        mathSin,
	"sinh":       mathSinh,
	"sqrt":       mathSqrt,
	"tan":        mathTan,
	"tanh":       mathTanh,
	"tointeger":  mathToInteger,
	"type":       mathType,
	"ult":        mathUlt,
}

func mathAbs(L *LState) int {
	L.Push(LNumber(math.Abs(float64(L.CheckNumber(1)))))
	return 1
}

func mathAcos(L *LState) int {
	L.Push(LNumber(math.Acos(float64(L.CheckNumber(1)))))
	return 1
}

func mathAsin(L *LState) int {
	L.Push(LNumber(math.Asin(float64(L.CheckNumber(1)))))
	return 1
}

func mathAtan(L *LState) int {
	L.Push(LNumber(math.Atan(float64(L.CheckNumber(1)))))
	return 1
}

func mathAtan2(L *LState) int {
	L.Push(LNumber(math.Atan2(float64(L.CheckNumber(1)), float64(L.CheckNumber(2)))))
	return 1
}

func mathCeil(L *LState) int {
	L.Push(LNumber(math.Ceil(float64(L.CheckNumber(1)))))
	return 1
}

func mathCos(L *LState) int {
	L.Push(LNumber(math.Cos(float64(L.CheckNumber(1)))))
	return 1
}

func mathCosh(L *LState) int {
	L.Push(LNumber(math.Cosh(float64(L.CheckNumber(1)))))
	return 1
}

func mathDeg(L *LState) int {
	L.Push(LNumber(float64(L.CheckNumber(1)) * 180 / math.Pi))
	return 1
}

func mathExp(L *LState) int {
	L.Push(LNumber(math.Exp(float64(L.CheckNumber(1)))))
	return 1
}

func mathFloor(L *LState) int {
	L.Push(LNumber(math.Floor(float64(L.CheckNumber(1)))))
	return 1
}

func mathFmod(L *LState) int {
	L.Push(LNumber(math.Mod(float64(L.CheckNumber(1)), float64(L.CheckNumber(2)))))
	return 1
}

func mathFrexp(L *LState) int {
	v1, v2 := math.Frexp(float64(L.CheckNumber(1)))
	L.Push(LNumber(v1))
	L.Push(LNumber(v2))
	return 2
}

func mathLdexp(L *LState) int {
	L.Push(LNumber(math.Ldexp(float64(L.CheckNumber(1)), L.CheckInt(2))))
	return 1
}

func mathLog(L *LState) int {
	L.Push(LNumber(math.Log(float64(L.CheckNumber(1)))))
	return 1
}

func mathLog10(L *LState) int {
	L.Push(LNumber(math.Log10(float64(L.CheckNumber(1)))))
	return 1
}

func mathMax(L *LState) int {
	if L.GetTop() == 0 {
		L.RaiseError("wrong number of arguments")
	}
	maxVal := L.CheckNumber(1)
	top := L.GetTop()
	for i := 2; i <= top; i++ {
		v := L.CheckNumber(i)
		if v > maxVal {
			maxVal = v
		}
	}
	L.Push(maxVal)
	return 1
}

func mathMin(L *LState) int {
	if L.GetTop() == 0 {
		L.RaiseError("wrong number of arguments")
	}
	minVal := L.CheckNumber(1)
	top := L.GetTop()
	for i := 2; i <= top; i++ {
		v := L.CheckNumber(i)
		if v < minVal {
			minVal = v
		}
	}
	L.Push(minVal)
	return 1
}

func mathMod(L *LState) int {
	lhs := L.CheckNumber(1)
	rhs := L.CheckNumber(2)
	L.Push(luaModulo(lhs, rhs))
	return 1
}

func mathModf(L *LState) int {
	v1, v2 := math.Modf(float64(L.CheckNumber(1)))
	L.Push(LNumber(v1))
	L.Push(LNumber(v2))
	return 2
}

func mathPow(L *LState) int {
	L.Push(LNumber(math.Pow(float64(L.CheckNumber(1)), float64(L.CheckNumber(2)))))
	return 1
}

func mathRad(L *LState) int {
	L.Push(LNumber(float64(L.CheckNumber(1)) * math.Pi / 180))
	return 1
}

func mathRandom(L *LState) int {
	switch L.GetTop() {
	case 0:
		L.Push(LNumber(rand.Float64()))
	case 1:
		m := L.CheckInt(1)
		if m < 1 {
			L.RaiseError("interval is empty")
		}
		L.Push(LNumber(rand.IntN(m) + 1))
	default:
		m := L.CheckInt(1)
		n := L.CheckInt(2)
		if m > n {
			L.RaiseError("interval is empty")
		}
		L.Push(LNumber(rand.IntN(n-m+1) + m))
	}
	return 1
}

func mathRandomSeed(L *LState) int {
	// rand/v2 doesn't require explicit seeding - it auto-seeds from crypto/rand
	// This function is kept for Lua compatibility but is effectively a no-op
	// If explicit seeding is needed, use a custom rand.Source
	return 0
}

func mathSin(L *LState) int {
	L.Push(LNumber(math.Sin(float64(L.CheckNumber(1)))))
	return 1
}

func mathSinh(L *LState) int {
	L.Push(LNumber(math.Sinh(float64(L.CheckNumber(1)))))
	return 1
}

func mathSqrt(L *LState) int {
	L.Push(LNumber(math.Sqrt(float64(L.CheckNumber(1)))))
	return 1
}

func mathTan(L *LState) int {
	L.Push(LNumber(math.Tan(float64(L.CheckNumber(1)))))
	return 1
}

func mathTanh(L *LState) int {
	L.Push(LNumber(math.Tanh(float64(L.CheckNumber(1)))))
	return 1
}

func mathToInteger(L *LState) int {
	v := L.Get(1)
	switch n := v.(type) {
	case LInteger:
		L.Push(n)
		return 1
	case LNumber:
		if float64(n) == math.Trunc(float64(n)) && !math.IsInf(float64(n), 0) && !math.IsNaN(float64(n)) {
			L.Push(LInteger(n))
			return 1
		}
	case LString:
		if num, err := parseNumber(string(n)); err == nil {
			if float64(num) == math.Trunc(float64(num)) && !math.IsInf(float64(num), 0) {
				L.Push(LInteger(num))
				return 1
			}
		}
	}
	L.Push(LNil)
	return 1
}

func mathType(L *LState) int {
	v := L.Get(1)
	switch v.(type) {
	case LInteger:
		L.Push(LString("integer"))
	case LNumber:
		L.Push(LString("float"))
	default:
		L.Push(LNil)
	}
	return 1
}

func mathUlt(L *LState) int {
	m := uint64(L.CheckInt64(1))
	n := uint64(L.CheckInt64(2))
	L.Push(LBool(m < n))
	return 1
}

//
