package effect

import (
	"fmt"

	"github.com/wippyai/go-lua/types/constraint"
)

// Label represents an atomic effect that a function can have.
//
// Labels are the building blocks of effect rows. Each Label implementation
// describes a specific observable behavior or type-level side effect. Labels
// are combined into Rows to describe complete effect signatures for functions.
//
// All Label implementations must satisfy:
//   - label(): marker method for interface satisfaction
//   - String(): human-readable representation for diagnostics
//   - Equals(other Label): structural equality for row deduplication
//
// Effect categories and their labels:
//
// Control effects describe exceptional control flow:
//   - Throw: function may raise an error via error() or assert()
//   - Diverge: function may not terminate (infinite loops, os.exit)
//   - IO: function performs I/O operations (print, file access)
//
// Mutation effects describe type-level state changes:
//   - Mutate: modifies a parameter's type structure (e.g., widening array elements)
//   - LengthChange: modifies array length by a known delta
//   - TableMutator: specialized mutation for table.insert-like operations
//
// Ownership effects describe value lifecycle for memory optimization:
//   - Borrow: temporary read access, value can be released after call
//   - BorrowAll: all parameters are borrowed (common for pure functions)
//   - Store: persistent storage, value escapes into a data structure
//   - Send: cross-actor transfer, value becomes frozen and shared
//   - Freeze: marks a value as immutable for safe sharing
//
// Return effects describe type derivations from parameters:
//   - Return: how a return type derives from parameter types
//   - ErrorReturn: encodes the Lua (value, error) return pattern
//   - ReturnLength: relates return array length to parameter lengths
//   - CorrelatedReturn: marks returns that are nil/non-nil together
//
// Flow effects describe value identity preservation:
//   - PassThrough: parameter flows unchanged to return position
//   - FlowInto: parameter flows into a field of the returned value
//
// Iterator effects describe iteration semantics:
//   - Iterator: marks ipairs/pairs style iteration with kind
//
// Semantic effects enable special type checker handling:
//   - ModuleLoad: require-like module loading
//   - TypePredicate: type()-like type name inspection
//   - TypeValueMethod: Type:is()-like method on type values
//   - VariadicTransform: select-like variadic manipulation
//   - CallableType: TypeName(x) constructor pattern
type Label interface {
	label()
	String() string
	Equals(other Label) bool
}

// ParamRef references a function parameter by position.
//
// Used throughout effect labels to specify which parameters are affected by
// an effect. The type checker resolves ParamRef indices to actual parameter
// types at each call site.
//
// Special index values:
//   - Non-negative: 0-based index into the fixed parameter list
//   - Negative: relative from runtime argument tail (-1 is last argument)
type ParamRef struct {
	Index int // 0-based index; -1 means the last variadic argument
}

func (p ParamRef) String() string {
	if p.Index == -1 {
		return "param[last]"
	}

	return fmt.Sprintf("param[%d]", p.Index)
}

// Mutate indicates a function mutates a parameter's type structure.
//
// This is the primary effect for functions that modify the type of their
// arguments in ways visible to the type system. The most common use is for
// table.insert, which widens the element type of an array.
//
// Example for table.insert(t, value):
//
//	Mutate{
//	    Target:      ParamRef{Index: 0},               // mutates first param (t)
//	    Transform:   ElementUnion{Source: ParamRef{Index: 1}},  // widens element type with value's type
//	    LengthDelta: constraint.Const{Value: 1},      // length increases by 1
//	}
//
// The type checker applies Mutate effects to update the type environment
// after the call, reflecting the widened type for subsequent code.
type Mutate struct {
	Target      ParamRef        // Which parameter is mutated
	Transform   TypeTransform   // How the type changes (ElementUnion, ToArray, etc.)
	LengthDelta constraint.Expr // Length change expression (+1, +len(param[1]), etc.)
}

func (Mutate) label() {}
func (m Mutate) String() string {
	if m.LengthDelta != nil {
		return fmt.Sprintf("mutate(%s, %s, delta=%s)", m.Target, m.Transform, m.LengthDelta)
	}

	return fmt.Sprintf("mutate(%s, %s)", m.Target, m.Transform)
}
func (m Mutate) Equals(other Label) bool {
	if o, ok := other.(Mutate); ok {
		return m.Target.Index == o.Target.Index &&
			transformEquals(m.Transform, o.Transform) &&
			constraint.ExprEquals(m.LengthDelta, o.LengthDelta)
	}

	return false
}

// TypeTransform describes how a type is transformed by a Mutate effect.
//
// Implementations define specific transformation patterns:
//
//   - ElementUnion: Widens an array's element type by unioning with another param's type.
//     Used for table.insert to track that inserted values expand the element type.
//
//   - ContainerElementUnion: Like ElementUnion but for container types with both key and value.
//     Used for map-like insertion operations.
//
//   - ToArray: Converts an empty table {} to a typed array T[].
//     Used when the first insert determines the array element type.
//
//   - Unchanged: No type transformation, used when only length changes.
type TypeTransform interface {
	transform()
	String() string
}

// ElementUnion widens an array's element type.
type ElementUnion struct {
	Source ParamRef
}

func (ElementUnion) transform() {}
func (e ElementUnion) String() string {
	return fmt.Sprintf("union_elem(%s)", e.Source)
}

// ContainerElementUnion widens a container element type with a value type.
type ContainerElementUnion struct {
	Container ParamRef
	Value     ParamRef
}

func (ContainerElementUnion) transform() {}
func (c ContainerElementUnion) String() string {
	return fmt.Sprintf("union_elem(%s, %s)", c.Container, c.Value)
}

// ToArray transforms {} into T[].
type ToArray struct {
	Element ParamRef
}

func (ToArray) transform() {}
func (t ToArray) String() string {
	return fmt.Sprintf("to_array(%s)", t.Element)
}

// Unchanged indicates no type transformation.
type Unchanged struct{}

func (Unchanged) transform()     {}
func (Unchanged) String() string { return "unchanged" }

// Return indicates how a return value's type is derived from parameters.
//
// This effect enables precise typing for built-in functions that act like
// generics but are implemented in C/native code. The type checker uses
// Return effects to compute concrete return types based on argument types.
//
// Examples:
//
//	table.remove(t): Returns element type of t (or nil)
//	    Return{ReturnIndex: 0, Transform: OptionalElementOf{Source: ParamRef{Index: 0}}}
//
//	table.unpack(t): Returns tuple of element type
//	    Return{ReturnIndex: 0, Transform: ElementOf{Source: ParamRef{Index: 0}}}
//
//	assert(v): Returns v unchanged (passthrough)
//	    Return{ReturnIndex: 0, Transform: SameAs{Source: ParamRef{Index: 0}}}
type Return struct {
	ReturnIndex int        // Which return value (0-based)
	Transform   ReturnType // How to derive the type (ElementOf, SameAs, etc.)
}

func (Return) label() {}
func (r Return) String() string {
	return fmt.Sprintf("ret[%d].type = %s", r.ReturnIndex, r.Transform)
}
func (r Return) Equals(other Label) bool {
	if o, ok := other.(Return); ok {
		return r.ReturnIndex == o.ReturnIndex &&
			returnTypeEquals(r.Transform, o.Transform)
	}

	return false
}

// ErrorReturn indicates correlated error-return semantics.
//
// Encodes the common Lua pattern where functions return (value, nil) on success
// or (nil, error) on failure. The type checker uses this to enable automatic
// narrowing after error checks.
//
// Example for io.open(filename):
//
//	-- Returns (file, nil) on success, (nil, string) on failure
//	ErrorReturn{ValueIndex: 0, ErrorIndex: 1}
//
// After checking the error:
//
//	local file, err = io.open("test.txt")
//	if err then
//	    -- file is nil here
//	else
//	    -- file is file here (non-nil)
//	end
type ErrorReturn struct {
	ValueIndex int // Value return position (0-based)
	ErrorIndex int // Error return position (0-based)
}

func (ErrorReturn) label() {}
func (e ErrorReturn) String() string {
	return fmt.Sprintf("errret(val[%d], err[%d])", e.ValueIndex, e.ErrorIndex)
}
func (e ErrorReturn) Equals(other Label) bool {
	if o, ok := other.(ErrorReturn); ok {
		return e.ValueIndex == o.ValueIndex &&
			e.ErrorIndex == o.ErrorIndex
	}

	return false
}

// ReturnLength indicates how a return value's length relates to parameters.
type ReturnLength struct {
	ReturnIndex int             // Which return value (0-based)
	Length      constraint.Expr // Length expression in terms of parameters
}

func (ReturnLength) label() {}
func (r ReturnLength) String() string {
	return fmt.Sprintf("ret[%d].len = %s", r.ReturnIndex, r.Length)
}
func (r ReturnLength) Equals(other Label) bool {
	if o, ok := other.(ReturnLength); ok {
		return r.ReturnIndex == o.ReturnIndex &&
			constraint.ExprEquals(r.Length, o.Length)
	}

	return false
}

// ReturnType describes how to derive a return type from function parameters.
//
// Each implementation defines a specific type derivation pattern:
//
//   - ElementOf: Returns the element type of an array parameter.
//     table.unpack(t) -> element type of t
//
//   - OptionalElementOf: Returns element type | nil.
//     table.remove(t) -> element type of t, or nil if empty
//
//   - SameAs: Returns the exact type of a parameter (identity).
//     assert(v) -> typeof(v)
//
//   - CallbackReturn: Returns what a callback parameter returns.
//     table.sort(t, cmp) where cmp returns boolean -> boolean
//
//   - ArrayOfCallbackReturn: Returns an array of the callback's return type.
//     table.map(t, fn) -> Array<typeof(fn(element))>
//
//   - DeepElementOf: Recursively extracts non-array leaf types.
//     For nested arrays, returns the innermost element type.
//
//   - StringUnpackValue: Returns the first unpacked value type derived from a
//     string.pack/string.unpack format parameter when that format is known.
//
//   - SelectCaseOfParam: Builds select case from parameter type.
//
//   - SelectResultOfCases: Builds select result from cases and default.
type ReturnType interface {
	returnType()
	String() string
}

// SelectCaseOfParam builds a select case from a parameter type.
type SelectCaseOfParam struct {
	Source ParamRef
}

func (SelectCaseOfParam) returnType() {}
func (s SelectCaseOfParam) String() string {
	return fmt.Sprintf("select_case(%s)", s.Source)
}

// SelectResultOfCases builds a select result from a cases parameter.
type SelectResultOfCases struct {
	Cases   ParamRef
	Default ParamRef
}

func (SelectResultOfCases) returnType() {}
func (s SelectResultOfCases) String() string {
	return fmt.Sprintf("select_result(%s, %s)", s.Cases, s.Default)
}

// ElementOf returns the element type of an array parameter.
type ElementOf struct {
	Source ParamRef
}

func (ElementOf) returnType() {}
func (e ElementOf) String() string {
	return fmt.Sprintf("elem(%s)", e.Source)
}

// OptionalElementOf returns element type | nil.
type OptionalElementOf struct {
	Source ParamRef
}

func (OptionalElementOf) returnType() {}
func (e OptionalElementOf) String() string {
	return fmt.Sprintf("elem(%s) | nil", e.Source)
}

// CallbackReturn uses the return type of a callback parameter.
type CallbackReturn struct {
	CallbackParam ParamRef
}

func (CallbackReturn) returnType() {}
func (c CallbackReturn) String() string {
	return fmt.Sprintf("callback_ret(%s)", c.CallbackParam)
}

// ArrayOfCallbackReturn returns an array of callback's return type.
type ArrayOfCallbackReturn struct {
	CallbackParam ParamRef
}

func (ArrayOfCallbackReturn) returnType() {}
func (a ArrayOfCallbackReturn) String() string {
	return fmt.Sprintf("array(callback_ret(%s))", a.CallbackParam)
}

// SameAs returns the exact type of a parameter.
type SameAs struct {
	Source ParamRef
}

func (SameAs) returnType() {}
func (s SameAs) String() string {
	return fmt.Sprintf("same(%s)", s.Source)
}

// DeepElementOf recursively extracts non-array leaf types.
type DeepElementOf struct {
	Source ParamRef
}

func (DeepElementOf) returnType() {}
func (d DeepElementOf) String() string {
	return fmt.Sprintf("deep_elem(%s)", d.Source)
}

// StringUnpackValue derives the first unpacked value type from a format parameter.
//
// This models builtins like string.unpack where the first returned value depends
// on the literal format string supplied at the call site.
type StringUnpackValue struct {
	Format ParamRef
}

func (StringUnpackValue) returnType() {}
func (s StringUnpackValue) String() string {
	return fmt.Sprintf("string_unpack(%s)", s.Format)
}

// Throw indicates the function may throw an error.
type Throw struct{}

func (Throw) label()         {}
func (Throw) String() string { return KeyThrow }
func (Throw) Equals(other Label) bool {
	_, ok := other.(Throw)
	return ok
}

// Diverge indicates the function may not terminate.
type Diverge struct{}

func (Diverge) label()         {}
func (Diverge) String() string { return KeyDiverge }
func (Diverge) Equals(other Label) bool {
	_, ok := other.(Diverge)
	return ok
}

// IO indicates the function performs I/O operations.
type IO struct{}

func (IO) label()         {}
func (IO) String() string { return "io" }
func (IO) Equals(other Label) bool {
	_, ok := other.(IO)
	return ok
}

// LengthChange indicates a length modification.
type LengthChange struct {
	Target ParamRef
	Delta  int // +1, -1, or 0 for unknown
}

func (LengthChange) label() {}
func (l LengthChange) String() string {
	if l.Delta >= 0 {
		return fmt.Sprintf("len(%s) += %d", l.Target, l.Delta)
	}

	return fmt.Sprintf("len(%s) -= %d", l.Target, -l.Delta)
}
func (l LengthChange) Equals(other Label) bool {
	if o, ok := other.(LengthChange); ok {
		return l.Target.Index == o.Target.Index && l.Delta == o.Delta
	}

	return false
}

// IteratorKind describes the type of iteration.
type IteratorKind int

const (
	// IterateIndexed iterates with integer indices (ipairs-style).
	IterateIndexed IteratorKind = iota
	// IterateKeyed iterates with arbitrary keys (pairs-style).
	IterateKeyed
)

// Iterator indicates the function returns an iterator over a parameter.
//
// This effect enables the type checker to understand iterator factories like
// ipairs and pairs. When a function with Iterator effect is used in a for loop,
// the loop variable types are derived from the source parameter's type.
//
// IteratorKind determines iteration semantics:
//   - IterateIndexed (ipairs): iterates with integer indices 1, 2, 3, ...
//     Loop variables are (integer, element_type)
//   - IterateKeyed (pairs): iterates with arbitrary keys
//     Loop variables are (key_type, value_type)
type Iterator struct {
	Source ParamRef     // Which parameter is being iterated
	Kind   IteratorKind // Type of iteration (indexed or keyed)
}

func (Iterator) label() {}
func (i Iterator) String() string {
	kind := "indexed"
	if i.Kind == IterateKeyed {
		kind = "keyed"
	}

	return fmt.Sprintf("iterator(%s, %s)", i.Source, kind)
}
func (i Iterator) Equals(other Label) bool {
	if o, ok := other.(Iterator); ok {
		return i.Source.Index == o.Source.Index && i.Kind == o.Kind
	}

	return false
}

// TableMutator indicates the function mutates a table parameter.
// This is the semantic effect for table.insert and similar.
type TableMutator struct {
	Target ParamRef // Which parameter is mutated
	Value  ParamRef // Which parameter is the new value
}

func (TableMutator) label() {}
func (t TableMutator) String() string {
	return fmt.Sprintf("table_mutator(%s, %s)", t.Target, t.Value)
}
func (t TableMutator) Equals(other Label) bool {
	if o, ok := other.(TableMutator); ok {
		return t.Target.Index == o.Target.Index && t.Value.Index == o.Value.Index
	}

	return false
}

// Borrow indicates the function only reads a parameter temporarily.
//
// Borrow semantics enable memory optimization: if a value is only borrowed
// by called functions and has no other references, it can be released after
// the last borrowing call returns without needing garbage collection.
//
// Example: print(x) borrows x for the duration of the call, then releases it.
// If x has no other references, it can be immediately freed.
//
// Contrast with Store, which indicates the value escapes into a data structure
// and must be kept alive.
type Borrow struct {
	Param ParamRef // Which parameter is borrowed
}

func (Borrow) label() {}
func (b Borrow) String() string {
	return fmt.Sprintf("borrow(%s)", b.Param)
}
func (b Borrow) Equals(other Label) bool {
	if o, ok := other.(Borrow); ok {
		return b.Param.Index == o.Param.Index
	}

	return false
}

// Store indicates the function stores a parameter persistently.
//
// When a value is stored, it escapes the call site and cannot be released
// without reference counting or garbage collection. The type checker tracks
// Store effects to determine which values may have long lifetimes.
//
// The Into field, when non-negative, indicates which parameter receives the
// stored value (e.g., table.insert stores value into table). When -1, the
// storage destination is unknown (e.g., closure capture).
type Store struct {
	Param ParamRef // Which parameter is stored
	Into  ParamRef // Where it's stored (if known, -1 for unknown)
}

func (Store) label() {}
func (s Store) String() string {
	if s.Into.Index >= 0 {
		return fmt.Sprintf("store(%s into %s)", s.Param, s.Into)
	}

	return fmt.Sprintf("store(%s)", s.Param)
}
func (s Store) Equals(other Label) bool {
	if o, ok := other.(Store); ok {
		return s.Param.Index == o.Param.Index && s.Into.Index == o.Into.Index
	}

	return false
}

// BorrowAll indicates all parameters are only borrowed (read-only, temporary).
// Used for functions like print, tostring that inspect but don't store.
type BorrowAll struct{}

func (BorrowAll) label()         {}
func (BorrowAll) String() string { return KeyBorrowAll }
func (BorrowAll) Equals(other Label) bool {
	_, ok := other.(BorrowAll)
	return ok
}

// Send indicates the function sends parameters to another actor/process.
// The values become frozen (immutable) and shared across actors.
// For process:send(pid, topic, msg...), FromParam marks where msg starts.
type Send struct {
	FromParam int // First param index that is sent (e.g., 2 for msg... in send(pid, topic, msg...))
}

func (Send) label() {}
func (s Send) String() string {
	return fmt.Sprintf("send(params[%d:])", s.FromParam)
}
func (s Send) Equals(other Label) bool {
	if o, ok := other.(Send); ok {
		return s.FromParam == o.FromParam
	}

	return false
}

// CorrelatedReturn indicates same-direction correlation between return values.
//
// This effect marks return positions that are always nil together or always
// non-nil together. It enables the type checker to narrow multiple returns
// with a single nil check.
//
// Example for string.find(s, pattern):
//
//	-- Returns (start, end) on match, (nil, nil) on no match
//	CorrelatedReturn{Indices: []int{0, 1}}
//
// After checking just one return:
//
//	local start, finish = string.find(s, pattern)
//	if start then
//	    -- Both start and finish are non-nil here
//	end
//
// Contrast with ErrorReturn which indicates inverse correlation (one nil means
// other is non-nil).
type CorrelatedReturn struct {
	Indices []int // Return positions that are correlated (0-based)
}

func (CorrelatedReturn) label() {}
func (c CorrelatedReturn) String() string {
	return fmt.Sprintf("correlated_return(%v)", c.Indices)
}
func (c CorrelatedReturn) Equals(other Label) bool {
	if o, ok := other.(CorrelatedReturn); ok {
		if len(c.Indices) != len(o.Indices) {
			return false
		}
		for i := range c.Indices {
			if c.Indices[i] != o.Indices[i] {
				return false
			}
		}
		return true
	}
	return false
}

// Freeze indicates the function freezes a parameter (makes it immutable).
// After freezing, the value can be safely shared across actors.
type Freeze struct {
	Param ParamRef // Which parameter is frozen
}

func (Freeze) label() {}
func (f Freeze) String() string {
	return fmt.Sprintf("freeze(%s)", f.Param)
}
func (f Freeze) Equals(other Label) bool {
	if o, ok := other.(Freeze); ok {
		return f.Param.Index == o.Param.Index
	}

	return false
}

// ModuleLoad indicates a require-like module loading function.
type ModuleLoad struct{}

func (ModuleLoad) label()         {}
func (ModuleLoad) String() string { return "module_load" }
func (ModuleLoad) Equals(other Label) bool {
	_, ok := other.(ModuleLoad)
	return ok
}

// VariadicTransform indicates a select-like variadic transform function.
type VariadicTransform struct{}

func (VariadicTransform) label()         {}
func (VariadicTransform) String() string { return "variadic_transform" }
func (VariadicTransform) Equals(other Label) bool {
	_, ok := other.(VariadicTransform)
	return ok
}

// TypePredicate indicates a type()-like type name predicate function.
type TypePredicate struct{}

func (TypePredicate) label()         {}
func (TypePredicate) String() string { return "type_predicate" }
func (TypePredicate) Equals(other Label) bool {
	_, ok := other.(TypePredicate)
	return ok
}

// TypeValueMethod indicates a Type:is()-like method on type values.
type TypeValueMethod struct{}

func (TypeValueMethod) label()         {}
func (TypeValueMethod) String() string { return "type_value_method" }
func (TypeValueMethod) Equals(other Label) bool {
	_, ok := other.(TypeValueMethod)
	return ok
}

// CallableType indicates a TypeName(x) callable type constructor.
type CallableType struct{}

func (CallableType) label()         {}
func (CallableType) String() string { return "callable_type" }
func (CallableType) Equals(other Label) bool {
	_, ok := other.(CallableType)
	return ok
}

// transformEquals compares two TypeTransform values for structural equality.
func transformEquals(a, b TypeTransform) bool {
	if a == nil && b == nil {
		return true
	}

	if a == nil || b == nil {
		return false
	}
	return VisitTransform(a, TypeTransformVisitor[bool]{
		Unchanged: func(Unchanged) bool {
			_, ok := b.(Unchanged)
			return ok
		},
		ElementUnion: func(av ElementUnion) bool {
			if bv, ok := b.(ElementUnion); ok {
				return av.Source.Index == bv.Source.Index
			}
			return false
		},
		ContainerElementUnion: func(av ContainerElementUnion) bool {
			if bv, ok := b.(ContainerElementUnion); ok {
				return av.Container.Index == bv.Container.Index &&
					av.Value.Index == bv.Value.Index
			}
			return false
		},
		ToArray: func(av ToArray) bool {
			if bv, ok := b.(ToArray); ok {
				return av.Element.Index == bv.Element.Index
			}
			return false
		},
		Default: func(TypeTransform) bool {
			return false
		},
	})
}

// returnTypeEquals compares two ReturnType values for structural equality.
func returnTypeEquals(a, b ReturnType) bool {
	if a == nil && b == nil {
		return true
	}

	if a == nil || b == nil {
		return false
	}
	return VisitReturnType(a, ReturnTypeVisitor[bool]{
		ElementOf: func(av ElementOf) bool {
			if bv, ok := b.(ElementOf); ok {
				return av.Source.Index == bv.Source.Index
			}
			return false
		},
		OptionalElementOf: func(av OptionalElementOf) bool {
			if bv, ok := b.(OptionalElementOf); ok {
				return av.Source.Index == bv.Source.Index
			}
			return false
		},
		CallbackReturn: func(av CallbackReturn) bool {
			if bv, ok := b.(CallbackReturn); ok {
				return av.CallbackParam.Index == bv.CallbackParam.Index
			}
			return false
		},
		ArrayOfCallbackReturn: func(av ArrayOfCallbackReturn) bool {
			if bv, ok := b.(ArrayOfCallbackReturn); ok {
				return av.CallbackParam.Index == bv.CallbackParam.Index
			}
			return false
		},
		SameAs: func(av SameAs) bool {
			if bv, ok := b.(SameAs); ok {
				return av.Source.Index == bv.Source.Index
			}
			return false
		},
		DeepElementOf: func(av DeepElementOf) bool {
			if bv, ok := b.(DeepElementOf); ok {
				return av.Source.Index == bv.Source.Index
			}
			return false
		},
		StringUnpackValue: func(av StringUnpackValue) bool {
			if bv, ok := b.(StringUnpackValue); ok {
				return av.Format.Index == bv.Format.Index
			}
			return false
		},
		SelectCaseOfParam: func(av SelectCaseOfParam) bool {
			if bv, ok := b.(SelectCaseOfParam); ok {
				return av.Source.Index == bv.Source.Index
			}
			return false
		},
		SelectResultOfCases: func(av SelectResultOfCases) bool {
			if bv, ok := b.(SelectResultOfCases); ok {
				return av.Cases.Index == bv.Cases.Index && av.Default.Index == bv.Default.Index
			}
			return false
		},
		Default: func(ReturnType) bool {
			return false
		},
	})
}
