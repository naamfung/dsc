package effect

import "fmt"

// PassThrough indicates a parameter flows directly to a return position.
//
// When a function returns one of its parameters unchanged, the PassThrough
// effect enables the type checker to preserve the parameter's exact type
// (including refinements) through the call.
//
// This is essential for assertion-style functions:
//
//	function assert_not_nil(val) if val == nil then error() end return val end
//	-- PassThrough{ParamIndex: 0, ReturnIndex: 0}
//
// At the call site:
//
//	local x: string? = maybe_get_string()
//	local y = assert_not_nil(x)  -- y is string (non-nil), not string?
//
// Without PassThrough, the return type would be inferred from the declared
// return type, losing the refinement from the nil check.
type PassThrough struct {
	ParamIndex  int // Which parameter (0-based)
	ReturnIndex int // Which return position (0-based)
}

func (PassThrough) label() {}

func (p PassThrough) String() string {
	return fmt.Sprintf("passthrough(param[%d]→ret[%d])", p.ParamIndex, p.ReturnIndex)
}

func (p PassThrough) Equals(other Label) bool {
	if o, ok := other.(PassThrough); ok {
		return p.ParamIndex == o.ParamIndex && p.ReturnIndex == o.ReturnIndex
	}

	return false
}

// FlowInto indicates a parameter flows into a field of a returned table.
//
// When a function wraps a parameter into a record field, FlowInto enables
// the type checker to derive the record's field type from the parameter type.
//
// Example:
//
//	function wrap(val) return {inner = val} end
//	-- FlowInto{ParamIndex: 0, ReturnIndex: 0, Path: "inner"}
//
// At the call site:
//
//	local wrapped = wrap(42)  -- wrapped has type {inner: integer}
//	local wrapped2 = wrap("hello")  -- wrapped2 has type {inner: string}
//
// The Path field supports dotted paths for nested fields: "data.inner.value".
type FlowInto struct {
	ParamIndex  int    // Which parameter (0-based)
	ReturnIndex int    // Which return position (0-based)
	Path        string // Field path, e.g., "inner" or "data.value"
}

func (FlowInto) label() {}

func (f FlowInto) String() string {
	return fmt.Sprintf("flowinto(param[%d]→ret[%d].%s)", f.ParamIndex, f.ReturnIndex, f.Path)
}

func (f FlowInto) Equals(other Label) bool {
	if o, ok := other.(FlowInto); ok {
		return f.ParamIndex == o.ParamIndex &&
			f.ReturnIndex == o.ReturnIndex &&
			f.Path == o.Path
	}

	return false
}
