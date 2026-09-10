package core

import (
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// binaryOpCompute resolves the result type of a binary operator expression.
//
// This implements Lua's operator semantics including:
//   - Type coercion rules (numbers stay numbers, strings can concat)
//   - Integer preservation for certain operations
//   - Metamethod fallback for non-primitive types
//   - Union type distribution
//
// The function handles any/unknown propagation and unwraps aliases before
// delegating to specific operator handlers.
func binaryOpCompute(left typ.Type, op string, right typ.Type) typ.Type {
	if left == nil || right == nil {
		return nil
	}

	// Unwrap type aliases to get underlying types
	left = unwrap.Alias(left)
	right = unwrap.Alias(right)

	if top, ok := binaryOpTopTypes(left, op, right); ok {
		return top
	}

	// Handle unions - distribute operation over members
	if u, ok := left.(*typ.Union); ok {
		return binaryOpUnion(u, op, right)
	}

	if u, ok := right.(*typ.Union); ok {
		return binaryOpRightUnion(left, op, u)
	}

	switch op {
	// Arithmetic - with metamethod fallback
	case "+", "-", "*", "/", "%", "^":
		return binaryArithmeticWithMeta(left, op, right)
	case "//":
		return binaryIntDivWithMeta(left, right)

	// Bitwise - Lua 5.3+
	case "&", "|", "~":
		return binaryBitwiseWithMeta(left, op, right)
	case "<<", ">>":
		return binaryShiftWithMeta(left, op, right)

	// Concatenation - with metamethod fallback
	case "..":
		return binaryConcatWithMeta(left, right)

	// Comparison
	case "==", "~=":
		return typ.Boolean
	case "<", "<=":
		return binaryComparisonWithMeta(left, op, right)
	case ">", ">=":
		// > and >= use __lt and __le with swapped operands
		return binaryComparisonWithMeta(right, swapCompOp(op), left)

	// Logical
	case "and":
		return binaryAnd(left, right)
	case "or":
		return binaryOr(left, right)

	default:
		return nil
	}
}

// BinaryOp resolves a binary operator result type without using an engine.
//
// This is the pure, non-memoized version of binary operator resolution.
// See Engine.BinaryOp for the memoized version and full documentation.
func BinaryOp(left typ.Type, op string, right typ.Type) typ.Type {
	return binaryOpCompute(left, op, right)
}

// unaryOpCompute resolves the result type of a unary operator expression.
//
// Handles Lua's unary operators:
//   - "-": arithmetic negation
//   - "#": length (strings, tables)
//   - "~": bitwise NOT (Lua 5.3+)
//   - "not": logical negation (always boolean)
//
// Includes metamethod fallback for non-primitive types.
func unaryOpCompute(op string, operand typ.Type) typ.Type {
	if operand == nil {
		return nil
	}

	// Unwrap type aliases to get underlying type
	operand = unwrap.Alias(operand)

	if top, ok := unaryOpTopType(op, operand); ok {
		return top
	}

	// Handle unions
	if u, ok := operand.(*typ.Union); ok {
		return unaryOpUnion(op, u)
	}

	switch op {
	case "-":
		return unaryMinusWithMeta(operand)
	case "#":
		return unaryLengthWithMeta(operand)
	case "~":
		return unaryBnotWithMeta(operand)
	case "not":
		return typ.Boolean
	default:
		return nil
	}
}

// UnaryOp resolves a unary operator result type without using an engine.
//
// This is the pure, non-memoized version of unary operator resolution.
// See Engine.UnaryOp for the memoized version and full documentation.
func UnaryOp(op string, operand typ.Type) typ.Type {
	return unaryOpCompute(op, operand)
}

// binaryArithmetic handles basic arithmetic without metamethod fallback.
// Returns integer when both operands are integers (except ^ which promotes).
// Returns number when either operand is a number.
// Returns nil for non-numeric operands (caller should try metamethods).
func binaryArithmetic(left, right typ.Type, op string) typ.Type {
	leftNum := isNumeric(left)
	rightNum := isNumeric(right)

	if leftNum && rightNum {
		if IsIntegerType(left) && IsIntegerType(right) {
			switch op {
			case "^":
				return typ.Number
			default:
				return typ.Integer
			}
		}

		return typ.Number
	}

	// Check metamethods for non-numeric operands
	return nil
}

// binaryArithmeticWithMeta tries arithmetic with metamethod fallback.
// First attempts direct arithmetic, then checks left and right operands for
// appropriate metamethods (__add, __sub, __mul, etc.).
func binaryArithmeticWithMeta(left typ.Type, op string, right typ.Type) typ.Type {
	// Try direct arithmetic first
	if result := binaryArithmetic(left, right, op); result != nil {
		return result
	}

	// Map operator to metamethod name
	metaName := opToMetamethod(op)
	if metaName == "" {
		return nil
	}

	// Try left operand's metamethod first
	if mt, ok := GetMetamethod(left, metaName); ok {
		return metamethodReturnType(mt)
	}

	// Try right operand's metamethod
	if mt, ok := GetMetamethod(right, metaName); ok {
		return metamethodReturnType(mt)
	}

	return nil
}

// opToMetamethod maps Lua operators to their corresponding metamethod names.
// Returns empty string for operators without metamethods (logical operators).
func opToMetamethod(op string) string {
	switch op {
	case "+":
		return "__add"
	case "-":
		return "__sub"
	case "*":
		return "__mul"
	case "/":
		return "__div"
	case "%":
		return "__mod"
	case "^":
		return "__pow"
	case "//":
		return "__idiv"
	case "..":
		return "__concat"
	case "<":
		return "__lt"
	case "<=":
		return "__le"
	case "#":
		return "__len"
	default:
		return ""
	}
}

// metamethodReturnType extracts the return type from a metamethod.
// Metamethods are functions; this returns the first return type or Unknown.
func metamethodReturnType(mt typ.Type) typ.Type {
	if fn, ok := mt.(*typ.Function); ok {
		if len(fn.Returns) > 0 {
			return fn.Returns[0]
		}
	}

	return typ.Unknown
}

// binaryIntDiv handles floor division (//) which always returns integer.
func binaryIntDiv(left, right typ.Type) typ.Type {
	if !isNumeric(left) || !isNumeric(right) {
		return nil
	}

	return typ.Integer
}

// binaryBitwise handles bitwise AND (&), OR (|), XOR (~) operators.
// Lua 5.3+ feature. Requires integral operands, always returns integer.
func binaryBitwise(left, right typ.Type) typ.Type {
	if !isIntegral(left) || !isIntegral(right) {
		return nil
	}

	return typ.Integer
}

// binaryBitwiseWithMeta handles bitwise operators with metamethod fallback.
func binaryBitwiseWithMeta(left typ.Type, op string, right typ.Type) typ.Type {
	if result := binaryBitwise(left, right); result != nil {
		return result
	}

	// Map operator to metamethod
	var metaName string

	switch op {
	case "&":
		metaName = "__band"
	case "|":
		metaName = "__bor"
	case "~":
		metaName = "__bxor"
	default:
		return nil
	}

	if mt, ok := GetMetamethod(left, metaName); ok {
		return metamethodReturnType(mt)
	}

	if mt, ok := GetMetamethod(right, metaName); ok {
		return metamethodReturnType(mt)
	}

	return nil
}

// binaryShift handles left shift (<<) and right shift (>>) operators.
// Lua 5.3+ feature. Requires integral operands, always returns integer.
func binaryShift(left, right typ.Type) typ.Type {
	if !isIntegral(left) || !isIntegral(right) {
		return nil
	}

	return typ.Integer
}

// binaryShiftWithMeta handles shift operators with metamethod fallback.
func binaryShiftWithMeta(left typ.Type, op string, right typ.Type) typ.Type {
	if result := binaryShift(left, right); result != nil {
		return result
	}

	var metaName string

	switch op {
	case "<<":
		metaName = "__shl"
	case ">>":
		metaName = "__shr"
	default:
		return nil
	}

	if mt, ok := GetMetamethod(left, metaName); ok {
		return metamethodReturnType(mt)
	}

	if mt, ok := GetMetamethod(right, metaName); ok {
		return metamethodReturnType(mt)
	}

	return nil
}

// isIntegral returns true if the type can be used in bitwise operations.
// Includes integers, numbers (truncated at runtime), and their literals.
func isIntegral(t typ.Type) bool {
	switch t.Kind() {
	case kind.Integer:
		return true
	case kind.Number:
		return true // Numbers are truncated to integers
	case kind.Literal:
		lit := t.(*typ.Literal)
		return lit.Base == kind.Integer || lit.Base == kind.Number
	}

	return false
}

// binaryIntDivWithMeta handles floor division with metamethod fallback.
func binaryIntDivWithMeta(left, right typ.Type) typ.Type {
	if result := binaryIntDiv(left, right); result != nil {
		return result
	}

	// Try __idiv metamethod
	if mt, ok := GetMetamethod(left, "__idiv"); ok {
		return metamethodReturnType(mt)
	}

	if mt, ok := GetMetamethod(right, "__idiv"); ok {
		return metamethodReturnType(mt)
	}

	return nil
}

// binaryConcat handles string concatenation (..) without metamethod fallback.
// Lua auto-converts numbers to strings for concatenation.
func binaryConcat(left, right typ.Type) typ.Type {
	leftOk := isStringish(left)
	rightOk := isStringish(right)

	if !leftOk || !rightOk {
		return nil
	}

	return typ.String
}

// binaryConcatWithMeta handles concatenation with metamethod fallback.
func binaryConcatWithMeta(left, right typ.Type) typ.Type {
	if result := binaryConcat(left, right); result != nil {
		return result
	}

	// Try __concat metamethod
	if mt, ok := GetMetamethod(left, "__concat"); ok {
		return metamethodReturnType(mt)
	}

	if mt, ok := GetMetamethod(right, "__concat"); ok {
		return metamethodReturnType(mt)
	}

	return nil
}

// binaryComparison handles ordered comparison (<, <=, >, >=) without metamethods.
// Requires both operands to be numeric or both to be strings.
// No auto-coercion between numbers and strings for comparison.
func binaryComparison(left, right typ.Type) typ.Type {
	// Both numeric or both string (strict string check, no auto-coercion)
	if isNumeric(left) && isNumeric(right) {
		return typ.Boolean
	}

	if isStrictString(left) && isStrictString(right) {
		return typ.Boolean
	}

	return nil
}

// binaryComparisonWithMeta handles comparison with metamethod fallback.
func binaryComparisonWithMeta(left typ.Type, op string, right typ.Type) typ.Type {
	if result := binaryComparison(left, right); result != nil {
		return result
	}

	// Try metamethod (__lt or __le)
	metaName := opToMetamethod(op)
	if metaName == "" {
		return nil
	}

	// Check both operands for metamethod
	if _, ok := GetMetamethod(left, metaName); ok {
		return typ.Boolean
	}

	if _, ok := GetMetamethod(right, metaName); ok {
		return typ.Boolean
	}

	return nil
}

// swapCompOp swaps comparison operators for > and >= handling.
// Lua implements > and >= using __lt and __le with swapped operands.
func swapCompOp(op string) string {
	switch op {
	case ">":
		return "<"
	case ">=":
		return "<="
	default:
		return op
	}
}

// binaryAnd handles the "and" operator.
// Lua semantics: returns left if left is falsy, otherwise returns right.
// Type-wise: if left can be falsy, result is falsy_part(left) | right.
func binaryAnd(left, right typ.Type) typ.Type {
	// and returns left if falsy, else right
	// Type is: left if left is falsy type, else right
	falsy := falsyPart(left)
	if falsy == nil {
		return right
	}

	truthy := truthyPart(left)
	if truthy == nil {
		return falsy
	}

	return typ.NewUnion(falsy, right)
}

// binaryOr handles the "or" operator.
// Lua semantics: returns left if left is truthy, otherwise returns right.
// Type-wise: if left can be truthy, result is truthy_part(left) | right.
func binaryOr(left, right typ.Type) typ.Type {
	// or returns left if truthy, else right
	truthy := truthyPart(left)
	if truthy == nil {
		return right
	}

	falsy := falsyPart(left)
	if falsy == nil {
		return left
	}

	return typ.NewUnion(truthy, right)
}

// unaryMinus handles arithmetic negation without metamethod fallback.
// Preserves integer type; promotes to number otherwise.
func unaryMinus(operand typ.Type) typ.Type {
	if !isNumeric(operand) {
		return nil
	}

	if operand.Kind() == kind.Integer {
		return typ.Integer
	}
	if lit, ok := operand.(*typ.Literal); ok && lit.Base == kind.Integer {
		return typ.Integer
	}

	return typ.Number
}

// unaryMinusWithMeta handles negation with __unm metamethod fallback.
func unaryMinusWithMeta(operand typ.Type) typ.Type {
	if result := unaryMinus(operand); result != nil {
		return result
	}

	// Try __unm metamethod
	if mt, ok := GetMetamethod(operand, "__unm"); ok {
		return metamethodReturnType(mt)
	}

	return nil
}

// unaryLength handles the # operator without metamethod fallback.
// Returns integer for strings, arrays, tuples, records, and maps.
func unaryLength(operand typ.Type) typ.Type {
	if unwrap.IsBuiltinTableTop(operand) {
		return typ.Integer
	}

	switch operand.Kind() {
	case kind.String, kind.Array, kind.Tuple:
		return typ.Integer
	case kind.Record:
		return typ.Integer
	case kind.Map:
		return typ.Integer
	case kind.Literal:
		if operand.(*typ.Literal).Base == kind.String {
			return typ.Integer
		}
		return nil
	default:
		return nil
	}
}

// unaryLengthWithMeta handles length with __len metamethod fallback.
func unaryLengthWithMeta(operand typ.Type) typ.Type {
	if result := unaryLength(operand); result != nil {
		return result
	}

	// Try __len metamethod
	if mt, ok := GetMetamethod(operand, "__len"); ok {
		return metamethodReturnType(mt)
	}

	return nil
}

// unaryBnot handles bitwise NOT (~) operator without metamethod fallback.
// Lua 5.3+ feature. Requires integral operand, returns integer.
func unaryBnot(operand typ.Type) typ.Type {
	if !isIntegral(operand) {
		return nil
	}

	return typ.Integer
}

// unaryBnotWithMeta handles bitwise NOT with __bnot metamethod fallback.
func unaryBnotWithMeta(operand typ.Type) typ.Type {
	if result := unaryBnot(operand); result != nil {
		return result
	}

	// Try __bnot metamethod
	if mt, ok := GetMetamethod(operand, "__bnot"); ok {
		return metamethodReturnType(mt)
	}

	return nil
}

// binaryOpAny returns the result type when operating on "any" type.
// Comparison operators return boolean; others return any.
func binaryOpAny(op string) typ.Type {
	switch op {
	case "==", "~=", "<", "<=", ">", ">=":
		return typ.Boolean
	case "and", "or":
		return typ.Any
	default:
		return typ.Any
	}
}

// unaryOpAny returns the result type when operating on "any" type.
// "not" returns boolean; "#" returns integer; others return any.
func unaryOpAny(op string) typ.Type {
	switch op {
	case "not":
		return typ.Boolean
	case "#":
		return typ.Integer
	default:
		return typ.Any
	}
}

// binaryOpTopTypes resolves any/unknown combinations before regular operator flow.
func binaryOpTopTypes(left typ.Type, op string, right typ.Type) (typ.Type, bool) {
	leftUnknown := typ.IsUnknown(left)
	rightUnknown := typ.IsUnknown(right)

	// For arithmetic operators, unknown + numeric preserves numeric result shape.
	if leftUnknown || rightUnknown {
		switch op {
		case "+", "-", "*", "/", "%", "^", "//":
			if leftUnknown && isNumeric(right) {
				return numericResultType(op, right), true
			}
			if rightUnknown && isNumeric(left) {
				return numericResultType(op, left), true
			}
		}
		return typ.Unknown, true
	}

	if typ.IsAny(left) || typ.IsAny(right) {
		return binaryOpAny(op), true
	}

	return nil, false
}

// unaryOpTopType resolves any/unknown for unary operators.
func unaryOpTopType(op string, operand typ.Type) (typ.Type, bool) {
	if typ.IsUnknown(operand) {
		if op == "#" {
			return typ.Integer, true
		}
		return typ.Unknown, true
	}
	if typ.IsAny(operand) {
		return unaryOpAny(op), true
	}
	return nil, false
}

// binaryOpUnion distributes a binary operator over left union members.
// The result is the union of results for each member with the right operand.
func binaryOpUnion(u *typ.Union, op string, right typ.Type) typ.Type {
	var results []typ.Type

	seen := make(map[uint64]bool)

	for _, m := range u.Members {
		if r := binaryOpCompute(m, op, right); r != nil {
			h := r.Hash()
			if !seen[h] {
				seen[h] = true

				results = append(results, r)
			}
		}
	}

	if len(results) == 0 {
		return nil
	}

	if len(results) == 1 {
		return results[0]
	}

	return typ.NewUnion(results...)
}

// binaryOpRightUnion distributes a binary operator over right union members.
// The result is the union of results for the left operand with each member.
func binaryOpRightUnion(left typ.Type, op string, u *typ.Union) typ.Type {
	var results []typ.Type

	seen := make(map[uint64]bool)

	for _, m := range u.Members {
		if r := binaryOpCompute(left, op, m); r != nil {
			h := r.Hash()
			if !seen[h] {
				seen[h] = true

				results = append(results, r)
			}
		}
	}

	if len(results) == 0 {
		return nil
	}

	if len(results) == 1 {
		return results[0]
	}

	return typ.NewUnion(results...)
}

// unaryOpUnion distributes a unary operator over union members.
// The result is the union of results for each member.
func unaryOpUnion(op string, u *typ.Union) typ.Type {
	var results []typ.Type

	seen := make(map[uint64]bool)

	for _, m := range u.Members {
		if r := unaryOpCompute(op, m); r != nil {
			h := r.Hash()
			if !seen[h] {
				seen[h] = true

				results = append(results, r)
			}
		}
	}

	if len(results) == 0 {
		return nil
	}

	if len(results) == 1 {
		return results[0]
	}

	return typ.NewUnion(results...)
}

// isNumeric returns true if the type represents a numeric value.
// Includes number, integer, and numeric literals.
func isNumeric(t typ.Type) bool {
	switch t.Kind() {
	case kind.Number, kind.Integer:
		return true
	case kind.Literal:
		lit := t.(*typ.Literal)
		return lit.Base == kind.Number || lit.Base == kind.Integer
	}

	return false
}

// IsIntegerType checks if a type represents an integer (integer kind or integer literal).
// Used to determine if arithmetic operations preserve integer type.
func IsIntegerType(t typ.Type) bool {
	switch t.Kind() {
	case kind.Integer:
		return true
	case kind.Literal:
		return t.(*typ.Literal).Base == kind.Integer
	default:
		return false
	}
}

// numericResultType returns the result type for arithmetic with one known operand.
// Used when one operand is unknown but the other is numeric. Helps provide better
// type inference by propagating integer vs number distinction where possible.
func numericResultType(op string, known typ.Type) typ.Type {
	switch op {
	case "//":
		return typ.Integer
	case "^":
		return typ.Number
	default:
		if IsIntegerType(known) {
			return typ.Integer
		}
		return typ.Number
	}
}

// isStringish returns true if the type can be used in string concatenation.
// Includes strings, string literals, and numbers (Lua auto-converts numbers).
func isStringish(t typ.Type) bool {
	switch t.Kind() {
	case kind.String:
		return true
	case kind.Literal:
		return t.(*typ.Literal).Base == kind.String
	case kind.Number, kind.Integer:
		return true // Lua auto-converts numbers to strings for concat
	}

	return false
}

// isStrictString returns true only for actual string types (no number coercion).
// Used for comparison operators where numbers and strings cannot be compared.
func isStrictString(t typ.Type) bool {
	if t == nil {
		return false
	}
	switch t.Kind() {
	case kind.String:
		return true
	case kind.Literal:
		return t.(*typ.Literal).Base == kind.String
	}

	return false
}

// falsyPart extracts the falsy subset of a type.
// In Lua, only nil and false are falsy. This function returns the part of a
// type that could be falsy at runtime, used for "and" operator semantics.
// Returns nil if the type cannot be falsy.
func falsyPart(t typ.Type) typ.Type {
	if t == nil {
		return nil
	}

	return typ.Visit(t, typ.Visitor[typ.Type]{
		Optional: func(o *typ.Optional) typ.Type {
			return typ.Nil
		},
		Union: func(u *typ.Union) typ.Type {
			var parts []typ.Type

			for _, m := range u.Members {
				if fp := falsyPart(m); fp != nil {
					parts = append(parts, fp)
				}
			}

			if len(parts) == 0 {
				return nil
			}

			return typ.NewUnion(parts...)
		},
		Intersection: func(in *typ.Intersection) typ.Type {
			var parts []typ.Type

			for _, m := range in.Members {
				fp := falsyPart(m)
				if fp == nil {
					return nil
				}

				parts = append(parts, fp)
			}

			if len(parts) == 0 {
				return nil
			}

			return typ.NewIntersection(parts...)
		},
		Alias: func(a *typ.Alias) typ.Type {
			if a.Target == nil {
				return nil
			}

			return falsyPart(a.Target)
		},
		Literal: func(lit *typ.Literal) typ.Type {
			if b, ok := lit.Value.(bool); ok && !b {
				return typ.False
			}

			return nil
		},
		Default: func(t typ.Type) typ.Type {
			switch t.Kind() {
			case kind.Nil:
				return typ.Nil
			case kind.Boolean:
				return typ.False
			default:
				return nil
			}
		},
	})
}

// truthyPart extracts the truthy subset of a type.
// In Lua, everything except nil and false is truthy. This function returns
// the part of a type that could be truthy at runtime, used for "or" semantics.
// Returns nil if the type cannot be truthy (only nil or false).
func truthyPart(t typ.Type) typ.Type {
	if t == nil {
		return nil
	}

	return typ.Visit(t, typ.Visitor[typ.Type]{
		Optional: func(o *typ.Optional) typ.Type {
			return o.Inner
		},
		Union: func(u *typ.Union) typ.Type {
			var parts []typ.Type

			for _, m := range u.Members {
				if tp := truthyPart(m); tp != nil {
					parts = append(parts, tp)
				}
			}

			if len(parts) == 0 {
				return nil
			}

			return typ.NewUnion(parts...)
		},
		Intersection: func(in *typ.Intersection) typ.Type {
			var parts []typ.Type

			for _, m := range in.Members {
				tp := truthyPart(m)
				if tp == nil {
					return nil
				}

				parts = append(parts, tp)
			}

			if len(parts) == 0 {
				return nil
			}

			return typ.NewIntersection(parts...)
		},
		Alias: func(a *typ.Alias) typ.Type {
			if a.Target == nil {
				return nil
			}

			return truthyPart(a.Target)
		},
		Literal: func(lit *typ.Literal) typ.Type {
			if b, ok := lit.Value.(bool); ok && !b {
				return nil
			}

			return t
		},
		Default: func(t typ.Type) typ.Type {
			switch t.Kind() {
			case kind.Nil:
				return nil
			case kind.Boolean:
				return typ.True
			default:
				return t
			}
		},
	})
}
