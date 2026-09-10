package validate

func init() {
	Register("pattern", validatePattern)
}

func validatePattern(val any, arg any) *Error {
	s, ok := asString(val)
	if !ok {
		return nil
	}
	pattern, ok := arg.(string)
	if !ok {
		return &Error{Message: "invalid pattern argument", Constraint: "pattern"}
	}
	re := GetRegex(pattern)
	if re == nil {
		return &Error{Message: "invalid regex", Constraint: "pattern"}
	}
	if !re.MatchString(s) {
		return &Error{Message: "pattern mismatch", Got: val, Expected: pattern, Constraint: "pattern"}
	}
	return nil
}

func asString(val any) (string, bool) {
	switch v := val.(type) {
	case string:
		return v, true
	case stringer:
		return v.String(), true
	}
	return "", false
}
