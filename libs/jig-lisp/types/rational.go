package types

import "math/big"

// Rat is an exact rational number, backed by an arbitrary-precision
// math/big.Rat. big.Rat always keeps values in normalized (reduced) form with
// a positive denominator, so 6/2 is represented internally as 3/1.
type Rat struct {
	R *big.Rat
}

// NewRat returns the rational number num/den.
func NewRat(num, den int64) *Rat {
	return &Rat{R: big.NewRat(num, den)}
}

// String implements fmt.Stringer: integral values print as plain integers
// (e.g. "3"), everything else prints as "num/den" (e.g. "10/3").
func (r Rat) String() string {
	return r.R.String()
}

// IsNumber reports whether v is a numeric value: an exact int or Rat, or a
// floating-point number.
func IsNumber(v MalType) bool {
	switch v.(type) {
	case int, Rat, *Rat, float32, float64:
		return true
	default:
		return false
	}
}

// ToBigRat converts an exact number (int or Rat) into a *big.Rat, reporting
// whether the conversion succeeded. Floats are deliberately rejected so exact
// arithmetic cannot be silently polluted by approximations; use ToBigRatAny
// when float participation is wanted (e.g. ordering and equality).
func ToBigRat(v MalType) (*big.Rat, bool) {
	switch n := v.(type) {
	case int:
		return new(big.Rat).SetInt64(int64(n)), true
	case Rat:
		return new(big.Rat).Set(n.R), true
	case *Rat:
		return new(big.Rat).Set(n.R), true
	default:
		return nil, false
	}
}

// ToBigRatAny converts any numeric value (int, Rat, float32, float64) into a
// *big.Rat. Floats are converted to their exact binary value; NaN and
// infinities cannot be represented and report failure.
func ToBigRatAny(v MalType) (*big.Rat, bool) {
	if r, ok := ToBigRat(v); ok {
		return r, true
	}
	switch n := v.(type) {
	case float32:
		r := new(big.Rat).SetFloat64(float64(n))
		return r, r != nil
	case float64:
		r := new(big.Rat).SetFloat64(n)
		return r, r != nil
	default:
		return nil, false
	}
}

// NormRat returns the canonical MalType representation of an exact rational:
// an int when the value is integral and fits a platform int, otherwise a *Rat.
// This keeps plain-integer results as ints for maximum compatibility while
// preserving arbitrary precision for large or fractional values.
func NormRat(r *big.Rat) MalType {
	if r.IsInt() {
		if i := r.Num(); i.IsInt64() {
			return int(i.Int64())
		}
	}
	return &Rat{R: new(big.Rat).Set(r)}
}
