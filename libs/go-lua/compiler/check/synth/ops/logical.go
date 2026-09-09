package ops

import (
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

// LogicalAndTyped synthesizes the type of a Lua 'and' expression.
//
// Lua semantics: `a and b` returns `a` if `a` is falsy, otherwise returns `b`.
// The expression short-circuits: `b` is not evaluated if `a` is falsy.
//
// Type analysis:
//   - If left is definitely truthy: result is right's type
//   - If left is definitely falsy: result is left's type
//   - Otherwise: union of (falsy part of left) and right
//
// Examples:
//   - `true and x` -> type of x
//   - `nil and x` -> nil
//   - `string? and number` -> nil | number
func LogicalAndTyped(left, right typ.Type) typ.Type {
	left = ExtractFirstValue(left)
	right = ExtractFirstValue(right)

	// Unknown left type: cannot determine result
	if left == nil {
		return typ.Unknown
	}

	// Never type propagates (unreachable code)
	if left.Kind().IsNever() {
		return typ.Never
	}

	if right != nil && right.Kind().IsNever() {
		return typ.Never
	}

	// If left is definitely truthy (cannot be nil or false), result is right
	if !CanBeFalsy(left) {
		return right
	}

	// If left is definitely falsy (nil or false), result is left
	if IsFalsy(left) {
		return left
	}

	// Otherwise, could be either the falsy part of left or right
	falsyLeft := narrow.ToFalsy(left)
	if falsyLeft == nil || falsyLeft.Kind().IsNever() {
		return right
	}
	// Unknown/any right branch must remain dominant. Using plain union here can
	// collapse to falsy-only because typ.NewUnion treats unknown as non-informative.
	if typ.IsUnknown(right) {
		return typ.Unknown
	}
	if typ.IsAny(right) {
		return typ.Any
	}

	return typ.JoinBranchOutcome(falsyLeft, right)
}

// LogicalOrTyped synthesizes the type of a Lua 'or' expression.
//
// Lua semantics: `a or b` returns `a` if `a` is truthy, otherwise returns `b`.
// The expression short-circuits: `b` is not evaluated if `a` is truthy.
//
// Type analysis:
//   - If left is definitely truthy: result is left's type
//   - If left is definitely falsy: result is right's type
//   - Otherwise: union of (truthy part of left) and right
//
// Canonical policy: merge via typ.JoinBranchOutcome to preserve runtime
// uncertainty while still preferring concrete alternatives over soft placeholders.
func LogicalOrTyped(left, right typ.Type) typ.Type {
	left = ExtractFirstValue(left)
	right = ExtractFirstValue(right)

	// Unknown left type: cannot determine result
	if left == nil {
		return typ.Unknown
	}

	// Never type propagates (unreachable code)
	if left.Kind().IsNever() {
		return typ.Never
	}

	if right != nil && right.Kind().IsNever() {
		return typ.Never
	}

	// If left is definitely truthy, result is left
	if !CanBeFalsy(left) {
		return left
	}

	// If left is definitely falsy, result is right
	if IsFalsy(left) {
		return right
	}

	// Otherwise, could be either the truthy part of left or right
	truthyLeft := narrow.ToTruthy(left)
	if truthyLeft == nil || truthyLeft.Kind().IsNever() {
		return right
	}

	return typ.JoinBranchOutcome(truthyLeft, right)
}
