package typ

import (
	"fmt"
	"strconv"

	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/kind"
)

// Literal represents a singleton type containing exactly one value.
//
// Literal types enable precise type checking for constant values:
//   - LiteralBool(true) is a subtype of boolean
//   - LiteralString("foo") is a subtype of string
//   - LiteralInt(42) is a subtype of integer (and number)
//
// Base indicates the underlying primitive kind (Boolean, Number, Integer, String).
// Value holds the actual value with the appropriate Go type.
type Literal struct {
	Base  kind.Kind // Boolean, Number, Integer, or String
	Value any       // bool, float64, int64, or string
	hash  uint64
	str   string
}

// True and False are singleton boolean literals.
var (
	trueHash  = internal.HashCombine(internal.HashCombine(uint64(kind.Literal), uint64(kind.Boolean)), 1)
	falseHash = internal.HashCombine(uint64(kind.Literal), uint64(kind.Boolean))

	True  = &Literal{Base: kind.Boolean, Value: true, hash: trueHash, str: "true"}
	False = &Literal{Base: kind.Boolean, Value: false, hash: falseHash, str: "false"}
)

// LiteralBool returns the canonical boolean literal type.
func LiteralBool(v bool) *Literal {
	if v {
		return True
	}

	return False
}

// LiteralInt creates an integer literal type.
func LiteralInt(v int64) *Literal {
	h := internal.HashCombine(uint64(kind.Literal), uint64(kind.Integer))
	h = internal.HashCombine(h, uint64(v))

	return &Literal{Base: kind.Integer, Value: v, hash: h, str: strconv.FormatInt(v, 10)}
}

// LiteralNumber creates a number literal type.
func LiteralNumber(v float64) *Literal {
	h := internal.HashCombine(uint64(kind.Literal), uint64(kind.Number))
	h = internal.HashCombine(h, uint64(v))

	return &Literal{Base: kind.Number, Value: v, hash: h, str: strconv.FormatFloat(v, 'g', -1, 64)}
}

// LiteralString creates a string literal type.
func LiteralString(v string) *Literal {
	h := internal.HashCombine(uint64(kind.Literal), uint64(kind.String))
	h = internal.HashCombine(h, internal.FnvString(v))

	return &Literal{Base: kind.String, Value: v, hash: h, str: strconv.Quote(v)}
}

func (l *Literal) Kind() kind.Kind { return kind.Literal }

func (l *Literal) String() string {
	if l.str != "" {
		return l.str
	}
	switch l.Base {
	case kind.Boolean:
		if l.Value.(bool) {
			return "true"
		}

		return "false"
	case kind.Integer:
		return strconv.FormatInt(l.Value.(int64), 10)
	case kind.Number:
		return strconv.FormatFloat(l.Value.(float64), 'g', -1, 64)
	case kind.String:
		return fmt.Sprintf("%q", l.Value.(string))
	}

	return fmt.Sprintf("%v", l.Value)
}

func (l *Literal) Hash() uint64 { return l.hash }

func (l *Literal) Equals(other Type) bool {
	if other.Kind() != kind.Literal {
		return false
	}

	ol := other.(*Literal)

	return l.Base == ol.Base && l.Value == ol.Value
}

// LiteralEquals checks if two literals are equal.
func LiteralEquals(a, b *Literal) bool {
	if a == nil || b == nil {
		return false
	}

	if a.Base != b.Base {
		return false
	}

	return a.Value == b.Value
}

// TypeMatchesLiteral checks if a type is compatible with a literal value.
func TypeMatchesLiteral(t Type, lit *Literal) bool {
	if t == nil || lit == nil {
		return false
	}

	if a, ok := t.(*Alias); ok {
		return TypeMatchesLiteral(a.Target, lit)
	}

	if l, ok := t.(*Literal); ok {
		return LiteralEquals(l, lit)
	}

	if u, ok := t.(*Union); ok {
		for _, m := range u.Members {
			if TypeMatchesLiteral(m, lit) {
				return true
			}
		}

		return false
	}

	if opt, ok := t.(*Optional); ok {
		return TypeMatchesLiteral(opt.Inner, lit)
	}

	if inter, ok := t.(*Intersection); ok {
		for _, m := range inter.Members {
			if !TypeMatchesLiteral(m, lit) {
				return false
			}
		}

		return true
	}

	k := t.Kind()
	if k.IsPlaceholder() {
		return true
	}

	switch k {
	case kind.String:
		return lit.Base == kind.String
	case kind.Number:
		return lit.Base == kind.Number || lit.Base == kind.Integer
	case kind.Integer:
		return lit.Base == kind.Integer
	case kind.Boolean:
		return lit.Base == kind.Boolean
	default:
		return false
	}
}
