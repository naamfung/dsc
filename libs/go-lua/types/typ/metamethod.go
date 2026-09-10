package typ

// Metamethod represents a Lua metamethod identifier.
type Metamethod string

const (
	MetaIndex    Metamethod = "__index"
	MetaNewIndex Metamethod = "__newindex"
	MetaCall     Metamethod = "__call"
	MetaAdd      Metamethod = "__add"
	MetaSub      Metamethod = "__sub"
	MetaMul      Metamethod = "__mul"
	MetaDiv      Metamethod = "__div"
	MetaMod      Metamethod = "__mod"
	MetaPow      Metamethod = "__pow"
	MetaUnm      Metamethod = "__unm"
	MetaIDiv     Metamethod = "__idiv"
	MetaBand     Metamethod = "__band"
	MetaBor      Metamethod = "__bor"
	MetaBxor     Metamethod = "__bxor"
	MetaBnot     Metamethod = "__bnot"
	MetaShl      Metamethod = "__shl"
	MetaShr      Metamethod = "__shr"
	MetaConcat   Metamethod = "__concat"
	MetaLen      Metamethod = "__len"
	MetaEq       Metamethod = "__eq"
	MetaLt       Metamethod = "__lt"
	MetaLe       Metamethod = "__le"
	MetaToString Metamethod = "__tostring"
	MetaPairs    Metamethod = "__pairs"
	MetaIPairs   Metamethod = "__ipairs"
	MetaGC       Metamethod = "__gc"
	MetaClose    Metamethod = "__close"
	MetaMode     Metamethod = "__mode"
	MetaName     Metamethod = "__name"
)

// IsMetamethod returns true if the name is a valid metamethod.
func IsMetamethod(name string) bool {
	switch Metamethod(name) {
	case MetaIndex, MetaNewIndex, MetaCall,
		MetaAdd, MetaSub, MetaMul, MetaDiv, MetaMod, MetaPow, MetaUnm, MetaIDiv,
		MetaBand, MetaBor, MetaBxor, MetaBnot, MetaShl, MetaShr,
		MetaConcat, MetaLen,
		MetaEq, MetaLt, MetaLe,
		MetaToString, MetaPairs, MetaIPairs,
		MetaGC, MetaClose, MetaMode, MetaName:
		return true
	}

	return false
}

// IsBinaryMetamethod returns true if the metamethod is for binary operators.
func IsBinaryMetamethod(m Metamethod) bool {
	switch m {
	case MetaAdd, MetaSub, MetaMul, MetaDiv, MetaMod, MetaPow, MetaIDiv,
		MetaBand, MetaBor, MetaBxor, MetaShl, MetaShr,
		MetaConcat, MetaEq, MetaLt, MetaLe:
		return true
	}

	return false
}

// IsUnaryMetamethod returns true if the metamethod is for unary operators.
func IsUnaryMetamethod(m Metamethod) bool {
	switch m {
	case MetaUnm, MetaBnot, MetaLen:
		return true
	}

	return false
}

// IsComparisonMetamethod returns true if the metamethod is for comparisons.
func IsComparisonMetamethod(m Metamethod) bool {
	switch m {
	case MetaEq, MetaLt, MetaLe:
		return true
	}

	return false
}

// OperatorToMetamethod maps a binary operator to its metamethod.
func OperatorToMetamethod(op string) (Metamethod, bool) {
	switch op {
	case "+":
		return MetaAdd, true
	case "-":
		return MetaSub, true
	case "*":
		return MetaMul, true
	case "/":
		return MetaDiv, true
	case "%":
		return MetaMod, true
	case "^":
		return MetaPow, true
	case "//":
		return MetaIDiv, true
	case "&":
		return MetaBand, true
	case "|":
		return MetaBor, true
	case "~":
		return MetaBxor, true
	case "<<":
		return MetaShl, true
	case ">>":
		return MetaShr, true
	case "..":
		return MetaConcat, true
	case "==":
		return MetaEq, true
	case "<":
		return MetaLt, true
	case "<=":
		return MetaLe, true
	}

	return "", false
}

// UnaryOperatorToMetamethod maps a unary operator to its metamethod.
func UnaryOperatorToMetamethod(op string) (Metamethod, bool) {
	switch op {
	case "-":
		return MetaUnm, true
	case "~":
		return MetaBnot, true
	case "#":
		return MetaLen, true
	}

	return "", false
}

// Metatabled is an interface for types that can have metamethods.
type Metatabled interface {
	Type
	GetMetamethod(m Metamethod) *Function
	HasMetamethod(m Metamethod) bool
}
