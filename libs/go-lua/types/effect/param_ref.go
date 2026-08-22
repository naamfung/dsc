package effect

// ResolveParamIndex resolves a ParamRef against a runtime argument count.
//
// Non-negative indices are absolute. Negative indices address from the end
// of the runtime argument list (-1 is last).
func ResolveParamIndex(ref ParamRef, argCount int) (int, bool) {
	if argCount <= 0 {
		return 0, false
	}
	idx := ref.Index
	if idx < 0 {
		idx = argCount + idx
	}
	if idx < 0 || idx >= argCount {
		return 0, false
	}
	return idx, true
}
