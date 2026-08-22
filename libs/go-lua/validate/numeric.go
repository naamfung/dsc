package validate

func init() {
	Register("min", validateMin)
	Register("max", validateMax)
}

func validateMin(val any, arg any) *Error {
	n, ok := toNumber(val)
	if !ok {
		return nil
	}
	minVal := toFloat(arg)
	if n < minVal {
		return &Error{Message: "value below minimum", Got: val, Expected: arg, Constraint: "min"}
	}
	return nil
}

func validateMax(val any, arg any) *Error {
	n, ok := toNumber(val)
	if !ok {
		return nil
	}
	maxVal := toFloat(arg)
	if n > maxVal {
		return &Error{Message: "value above maximum", Got: val, Expected: arg, Constraint: "max"}
	}
	return nil
}

// toNumber extracts float64 from LNumber or LInteger.
// Uses type name matching to avoid import cycle.
func toNumber(val any) (float64, bool) {
	if val == nil {
		return 0, false
	}
	switch v := val.(type) {
	case float64:
		return v, true
	case int64:
		return float64(v), true
	case int:
		return float64(v), true
	default:
		// Check for LNumber (float64 underlying) or LInteger (int64 underlying)
		// by using reflection-free type assertion chain
		if f, ok := asFloat64(val); ok {
			return f, true
		}
		if i, ok := asInt64(val); ok {
			return float64(i), true
		}
	}
	return 0, false
}

func toFloat(arg any) float64 {
	switch n := arg.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case int32:
		return float64(n)
	case float32:
		return float64(n)
	}
	return 0
}

// Interface for types that can convert to float64 (LNumber)
type float64er interface {
	Float64() float64
}

// Interface for types that can convert to int64 (LInteger)
type int64er interface {
	Int64() int64
}

func asFloat64(val any) (float64, bool) {
	if f, ok := val.(float64er); ok {
		return f.Float64(), true
	}
	return 0, false
}

func asInt64(val any) (int64, bool) {
	if i, ok := val.(int64er); ok {
		return i.Int64(), true
	}
	return 0, false
}
