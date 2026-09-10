package validate

func init() {
	Register("min_len", validateMinLen)
	Register("max_len", validateMaxLen)
}

func validateMinLen(val any, arg any) *Error {
	length, ok := getLength(val)
	if !ok {
		return nil
	}
	minVal := toInt(arg)
	if minVal < 0 {
		return nil
	}
	if length < minVal {
		return &Error{Message: "length below minimum", Got: val, Expected: arg, Constraint: "min_len"}
	}
	return nil
}

func validateMaxLen(val any, arg any) *Error {
	length, ok := getLength(val)
	if !ok {
		return nil
	}
	maxVal := toInt(arg)
	if maxVal < 0 {
		return nil
	}
	if length > maxVal {
		return &Error{Message: "length above maximum", Got: val, Expected: arg, Constraint: "max_len"}
	}
	return nil
}

func toInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case int32:
		return int(n)
	case float32:
		return int(n)
	}
	return 0
}

// lengther interface for types with length (LTable, etc)
type lengther interface {
	Len() int
}

// stringer interface for string types (LString)
type stringer interface {
	String() string
}

func getLength(val any) (int, bool) {
	switch v := val.(type) {
	case string:
		return len(v), true
	case lengther:
		return v.Len(), true
	case stringer:
		return len(v.String()), true
	}
	return 0, false
}
