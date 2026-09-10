package typ

// CaptureMode indicates how a variable is captured by a closure.
type CaptureMode int

const (
	CaptureUnknown CaptureMode = iota
	CaptureByValue             // immutable, safe to copy at closure creation
	CaptureByRef               // mutable or escapes, needs upvalue
)

func (m CaptureMode) String() string {
	switch m {
	case CaptureByValue:
		return "by-value"
	case CaptureByRef:
		return "by-ref"
	default:
		return "unknown"
	}
}

// CaptureInfo describes a single captured variable.
type CaptureInfo struct {
	Name    string
	Type    Type
	Mode    CaptureMode
	Escapes bool
	Mutated bool
}

// NeedsUpvalue returns true if this capture requires a heap-allocated upvalue.
func (c CaptureInfo) NeedsUpvalue() bool {
	if c.Mode == CaptureByRef || c.Mode == CaptureUnknown {
		return true
	}

	return c.Mutated || c.Escapes
}
