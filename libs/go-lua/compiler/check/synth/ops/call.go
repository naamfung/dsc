// Package ops implements type synthesis operations for the type checker.
// The call.go file handles function call type synthesis, including generic
// type inference, method resolution, and argument type checking.
//
// # TWO-PHASE CALL SYNTHESIS
//
// Function calls with callback arguments require two-phase synthesis to enable
// contextual typing. The phases are:
//
//  1. InferCall: Resolve callee, infer type arguments, compute ExpectedArgs
//  2. Re-synthesize function literal arguments using ExpectedArgs from phase 1
//  3. ReInfer: Re-infer type arguments with updated argument types
//  4. FinishCall: Check arguments against parameters, compute return type
//
// For simple calls without callbacks, CallWithGenericInference combines phases.
//
// # GENERIC TYPE INFERENCE
//
// Generic functions have their type parameters inferred from argument types.
// The inference algorithm:
//   - Collects constraints from argument-to-parameter matching
//   - Uses ExpectedReturn for bidirectional inference when available
//   - Instantiates the function with inferred type arguments
//
// Example:
//
//	function get<T>(): T? end
//	local x: string? = get()  -- T inferred as string from ExpectedReturn
//
// # METHOD CALL HANDLING
//
// Method calls (obj:method(args)) follow Lua call semantics:
//   - The receiver is passed as the first runtime argument
//   - Remaining arguments map positionally after the receiver
//   - Self type substitution is applied in parameter and return types
//
// UNION/INTERSECTION CALLEES
//
// Union callees succeed if any member function accepts the arguments.
// The return type is the union of successful member return types.
//
// Intersection callees require all members to accept the arguments.
// The return type is the intersection of all member return types.
//
// # ERROR HANDLING
//
// Call errors are accumulated in CallResult.Errors without stopping synthesis.
// This allows partial type information even when calls have type errors.
// Error kinds include: arity mismatches, type mismatches, optional calls,
// and generic inference failures.
package ops

import (
	"fmt"

	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/subst"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// CallResult contains the synthesized return type and any errors from call checking.
//
// Type is the computed return type (typ.Tuple for multiple returns, typ.Nil for void).
// Errors contains type mismatches, arity problems, and other call-related issues.
type CallResult struct {
	Type    typ.Type    // Packed return type (or tuple for multiple returns)
	Returns []typ.Type  // Expression-adjusted return vector
	Errors  []CallError // Type errors detected
}

// CallError describes a type error in a function call.
type CallError struct {
	Kind    CallErrorKind
	Message string
	ArgIdx  int // For argument errors (1-based)
}

// CallErrorKind identifies the type of call error.
type CallErrorKind int

const (
	ErrNotCallable CallErrorKind = iota
	ErrWrongArity
	ErrTypeMismatch
	ErrOptionalCall
	ErrTypeInference
)

// CallDef describes a function call for type synthesis.
//
// For direct calls (fn(args)):
//   - Set Callee to the function type
//   - Set Args to synthesized argument types
//   - Leave IsMethod=false
//
// For method calls (obj:method(args)):
//   - Set Receiver to the object type
//   - Set MethodName to the method name
//   - Set Args to synthesized argument types (excluding receiver)
//   - Set IsMethod=true
//   - Set Query for method lookup
//
// For generic calls, TypeArgs can provide explicit type arguments.
// If empty, type arguments are inferred from Args.
type CallDef struct {
	Callee     typ.Type     // The function being called
	Args       []typ.Type   // Argument types
	TypeArgs   []typ.Type   // Explicit type arguments for generic calls
	IsMethod   bool         // True if this is a method call (obj:method)
	Receiver   typ.Type     // Receiver type for method calls
	MethodName string       // Method name for method calls
	Query      core.TypeOps // Method/field resolver
	// ForceMethodReceiver consumes receiver as first runtime argument even when
	// function shape alone does not imply explicit self.
	ForceMethodReceiver bool

	// ExpectedReturn provides contextual typing for generic inference.
	// When set, the expected return type guides type parameter inference
	// for generic functions (e.g., inferring T in `get<T>(): T?` from `local x: string? = get()`).
	ExpectedReturn typ.Type
}

// InferKind identifies the type of callee after resolution.
type InferKind int

const (
	InferKindFunction     InferKind = iota // Callee is a function
	InferKindUnion                         // Callee is a union of functions
	InferKindIntersection                  // Callee is an intersection of functions
	InferKindAny                           // Callee is any type
	InferKindUnknown                       // Callee is unknown type
	InferKindNotCallable                   // Callee is not callable
)

// InferResult contains results from the first phase of two-phase call synthesis.
//
// The two-phase approach enables contextual typing for callback arguments:
// 1. InferCall: Resolve callee, infer type arguments, compute expected arg types
// 2. Re-synthesize function literal arguments using ExpectedArgs
// 3. ReInfer: Re-infer type arguments with updated argument types
// 4. FinishCall: Check arguments and compute return type
//
// For non-callback cases, use CallWithGenericInference for single-phase synthesis.
//
// Fields:
//   - Kind: Classification of the resolved callee
//   - Callee: The resolved callee type after method lookup
//   - ExpectedArgs: Expected types for each argument position (for contextual typing)
//   - ExpectedVariadic: Expected type for variadic arguments
//   - ShortCircuit: If non-nil, FinishCall returns this immediately (for any/unknown)
type InferResult struct {
	// Kind indicates the type of resolved callee.
	Kind InferKind

	// Callee is the resolved callee type after method lookup and unwrapping.
	Callee typ.Type

	// Receiver is the resolved receiver type (after optional unwrapping).
	Receiver typ.Type

	// IsMethod indicates whether this is a method call.
	IsMethod bool
	// ForceMethodReceiver carries CallDef.ForceMethodReceiver through FinishCall/ReInfer.
	ForceMethodReceiver bool

	// TypeArgs contains the type arguments (explicit or inferred).
	TypeArgs []typ.Type

	// Instantiated is the function after generic instantiation (nil if not generic).
	Instantiated *typ.Function

	// Function is the base function (before or after instantiation).
	Function *typ.Function

	// ExpectedArgs contains the expected type for each argument position.
	// For method calls with explicit self, the receiver is NOT included.
	// Self substitution is already applied.
	ExpectedArgs []typ.Type

	// ExpectedVariadic is the expected type for variadic arguments (nil if not variadic).
	ExpectedVariadic typ.Type

	// Errors contains errors accumulated during inference (e.g., optional call errors).
	Errors []CallError

	// ShortCircuit is the return type when callee is any/unknown (non-nil means skip FinishCall).
	ShortCircuit typ.Type
}

// resolvedCallee is the result of resolving a call target.
type resolvedCallee struct {
	callee   typ.Type
	receiver typ.Type
	errors   []CallError
}

// resolveCallee resolves the call target from a CallDef, handling method lookup,
// receiver unwrapping, optional callee unwrapping, and instantiated substitution.
func resolveCallee(ctx *db.QueryContext, def CallDef) (*resolvedCallee, *CallResult) {
	var errors []CallError
	callee := def.Callee
	receiver := def.Receiver

	if def.IsMethod {
		if def.Query == nil {
			r := CallResult{Type: typ.Unknown, Errors: []CallError{{Kind: ErrNotCallable, Message: "nil call query"}}}
			return nil, &r
		}

		if receiver == nil {
			r := CallResult{Type: typ.Unknown, Errors: []CallError{{Kind: ErrNotCallable, Message: "nil receiver"}}}
			return nil, &r
		}

		if receiver.Kind() == kind.Optional {
			errors = append(errors, CallError{
				Kind:    ErrOptionalCall,
				Message: "cannot call method on optional value without nil check",
			})
			receiver = receiver.(*typ.Optional).Inner
		}

		methodType, ok := def.Query.Method(ctx, receiver, def.MethodName)
		if !ok {
			methodType, ok = def.Query.Field(ctx, receiver, def.MethodName)
		}

		if !ok {
			r := CallResult{
				Type:   typ.Unknown,
				Errors: []CallError{{Kind: ErrNotCallable, Message: formatUnionMethodError(ctx, receiver, def.MethodName, def.Query)}},
			}
			return nil, &r
		}

		if inst, ok := unwrap.Alias(receiver).(*typ.Instantiated); ok && inst.Generic != nil {
			methodType = subst.Params(methodType, inst.Generic.TypeParams, inst.TypeArgs)
		}
		callee = methodType
	} else if callee == nil {
		r := CallResult{Type: typ.Unknown, Errors: []CallError{{Kind: ErrNotCallable, Message: "nil callee"}}}
		return nil, &r
	}

	if callee.Kind() == kind.Optional {
		errors = append(errors, CallError{
			Kind:    ErrOptionalCall,
			Message: "cannot call optional value without nil check",
		})
		callee = callee.(*typ.Optional).Inner
	}

	return &resolvedCallee{callee: callee, receiver: receiver, errors: errors}, nil
}

// unwrapCallee performs alias, generic body, and instantiated unwrapping.
func unwrapCallee(callee typ.Type) typ.Type {
	callee = unwrap.Alias(callee)

	if g, ok := callee.(*typ.Generic); ok {
		callee = g.Body
	}

	if inst, ok := callee.(*typ.Instantiated); ok {
		resolved, err := core.ResolveInstantiated(inst)
		if err == nil && resolved != nil {
			callee = resolved
		}
	}

	return unwrap.Alias(callee)
}

// inferAndCall performs generic type inference and calls the instantiated function.
func inferAndCall(ctx *db.QueryContext, fn *typ.Function, def CallDef, isMethod bool, receiver typ.Type, errors []CallError) CallResult {
	var typeArgs []typ.Type
	if len(def.TypeArgs) > 0 {
		typeArgs = def.TypeArgs
	} else {
		var err error
		typeArgs, err = InferTypeArgsWithExpectedAndMode(fn, def.Args, isMethod, receiver, nil, false)
		if err != nil {
			errors = append(errors, CallError{Kind: ErrTypeInference, Message: err.Error()})
			return singleValueCallResult(typ.Unknown, errors)
		}
	}

	instantiated := InstantiateFunction(fn, typeArgs)

	return callFunction(ctx, def.Query, instantiated, def.Args, receiver, isMethod, def.ForceMethodReceiver, errors)
}

// InferCall performs the first phase of call synthesis: callee resolution,
// type argument inference, and expected argument type computation.
// The returned InferResult contains ExpectedArgs that can be used to
// re-synthesize function literal arguments with contextual typing.
// Call FinishCall with updated arguments to complete the call.
func InferCall(ctx *db.QueryContext, def CallDef) InferResult {
	resolved, early := resolveCallee(ctx, def)
	if early != nil {
		return InferResult{
			Kind:                InferKindNotCallable,
			Errors:              early.Errors,
			ShortCircuit:        early.Type,
			ForceMethodReceiver: def.ForceMethodReceiver,
		}
	}

	callee := resolved.callee
	errors := resolved.errors
	receiver := resolved.receiver
	isMethod := def.IsMethod

	if typ.IsAny(callee) {
		return InferResult{
			Kind:                InferKindAny,
			Callee:              callee,
			Receiver:            receiver,
			IsMethod:            isMethod,
			ForceMethodReceiver: def.ForceMethodReceiver,
			Errors:              errors,
			ShortCircuit:        typ.Any,
		}
	}

	if typ.IsUnknown(callee) {
		return InferResult{
			Kind:                InferKindUnknown,
			Callee:              callee,
			Receiver:            receiver,
			IsMethod:            isMethod,
			ForceMethodReceiver: def.ForceMethodReceiver,
			Errors:              errors,
			ShortCircuit:        typ.Unknown,
		}
	}
	if typ.IsNever(callee) {
		// Calls in unreachable paths (callee narrowed to never) should not emit
		// secondary "not callable" errors. Preserve dead-branch semantics by
		// short-circuiting with never.
		return InferResult{
			Kind:                InferKindUnknown,
			Callee:              callee,
			Receiver:            receiver,
			IsMethod:            isMethod,
			ForceMethodReceiver: def.ForceMethodReceiver,
			Errors:              errors,
			ShortCircuit:        typ.Never,
		}
	}

	callee = unwrapCallee(callee)

	if callee.Kind() == kind.Union {
		return inferUnion(ctx, callee.(*typ.Union), def, isMethod, receiver, errors)
	}

	if callee.Kind() == kind.Intersection {
		return InferResult{
			Kind:                InferKindIntersection,
			Callee:              callee,
			Receiver:            receiver,
			IsMethod:            isMethod,
			ForceMethodReceiver: def.ForceMethodReceiver,
			Errors:              errors,
		}
	}

	fn, ok := callee.(*typ.Function)
	if !ok {
		return InferResult{
			Kind:   InferKindNotCallable,
			Callee: callee,
			Errors: append(errors, CallError{Kind: ErrNotCallable, Message: fmt.Sprintf("expected function, got %s", typ.FormatShort(callee))}),
		}
	}

	return inferFunction(ctx, fn, def, isMethod, receiver, errors)
}

// inferFunction performs inference for a single function callee.
func inferFunction(ctx *db.QueryContext, fn *typ.Function, def CallDef, isMethod bool, receiver typ.Type, errors []CallError) InferResult {
	result := InferResult{
		Kind:                InferKindFunction,
		Callee:              fn,
		Receiver:            receiver,
		IsMethod:            isMethod,
		ForceMethodReceiver: def.ForceMethodReceiver,
		Function:            fn,
		Errors:              errors,
	}

	if len(fn.TypeParams) == 0 {
		result.Instantiated = fn
		result.ExpectedArgs, result.ExpectedVariadic = computeExpectedArgs(ctx, def.Query, fn, isMethod, receiver, def.ForceMethodReceiver)
		return result
	}

	var typeArgs []typ.Type
	if len(def.TypeArgs) > 0 {
		typeArgs = def.TypeArgs
	} else {
		var err error
		// Use bidirectional inference with expected return type when available
		typeArgs, err = InferTypeArgsWithExpectedAndMode(fn, def.Args, isMethod, receiver, def.ExpectedReturn, def.ForceMethodReceiver)
		if err != nil {
			result.Errors = append(result.Errors, CallError{Kind: ErrTypeInference, Message: err.Error()})
			return result
		}
	}

	result.TypeArgs = typeArgs
	result.Instantiated = InstantiateFunction(fn, typeArgs)
	result.ExpectedArgs, result.ExpectedVariadic = computeExpectedArgs(ctx, def.Query, result.Instantiated, isMethod, receiver, def.ForceMethodReceiver)

	return result
}

// inferUnion handles inference for union callees.
// For unions, we attempt to infer each member separately and return
// expected types from the first successful inference.
func inferUnion(ctx *db.QueryContext, u *typ.Union, def CallDef, isMethod bool, receiver typ.Type, errors []CallError) InferResult {
	result := InferResult{
		Kind:                InferKindUnion,
		Callee:              u,
		Receiver:            receiver,
		IsMethod:            isMethod,
		ForceMethodReceiver: def.ForceMethodReceiver,
		Errors:              errors,
	}

	// Aggregate expected argument types across all callable union members.
	// Selecting only the first member is order-dependent and can over-specialize
	// contextual typing for overloaded built-ins (for example pairs/ipairs).
	var (
		aggExpected []typ.Type
		aggVariadic typ.Type
		found       bool
	)
	for _, member := range u.Members {
		fn, ok := member.(*typ.Function)
		if !ok {
			continue
		}

		instantiated := fn
		typeArgs := []typ.Type(nil)
		if len(fn.TypeParams) > 0 {
			if len(def.TypeArgs) > 0 {
				typeArgs = def.TypeArgs
			} else {
				var err error
				typeArgs, err = InferTypeArgsWithExpectedAndMode(fn, def.Args, isMethod, receiver, def.ExpectedReturn, def.ForceMethodReceiver)
				if err != nil {
					continue
				}
			}
			instantiated = InstantiateFunction(fn, typeArgs)
		}

		expectedArgs, expectedVariadic := computeExpectedArgs(ctx, def.Query, instantiated, isMethod, receiver, def.ForceMethodReceiver)
		if !found {
			found = true
			result.Function = fn
			result.TypeArgs = typeArgs
			result.Instantiated = instantiated
			aggExpected = append([]typ.Type(nil), expectedArgs...)
			aggVariadic = expectedVariadic
			continue
		}
		aggExpected = mergeExpectedArgVectors(aggExpected, expectedArgs)
		aggVariadic = typ.JoinPreferNonSoft(aggVariadic, expectedVariadic)
	}

	if found {
		result.ExpectedArgs = aggExpected
		result.ExpectedVariadic = aggVariadic
	}

	return result
}

func mergeExpectedArgVectors(a, b []typ.Type) []typ.Type {
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}
	if maxLen == 0 {
		return nil
	}
	out := make([]typ.Type, maxLen)
	for i := 0; i < maxLen; i++ {
		var ai, bi typ.Type
		if i < len(a) {
			ai = a[i]
		}
		if i < len(b) {
			bi = b[i]
		}
		out[i] = typ.JoinPreferNonSoft(ai, bi)
	}
	return out
}

// computeExpectedArgs computes the expected type for each argument position.
func computeExpectedArgs(ctx *db.QueryContext, query core.TypeOps, fn *typ.Function, isMethod bool, receiver typ.Type, forceMethodReceiver bool) ([]typ.Type, typ.Type) {
	if fn == nil {
		return nil, nil
	}

	paramOffset := 0
	if methodConsumesReceiver(ctx, query, fn, receiver, isMethod, forceMethodReceiver) {
		paramOffset = 1
	}

	// Compute expected args for non-self params
	numArgs := len(fn.Params) - paramOffset
	if numArgs < 0 {
		numArgs = 0
	}

	expected := make([]typ.Type, numArgs)
	for i := 0; i < numArgs; i++ {
		paramIdx := i + paramOffset
		if paramIdx < len(fn.Params) {
			expected[i] = fn.Params[paramIdx].Type
			if isMethod && receiver != nil {
				expected[i] = subst.Self(expected[i], receiver)
			}
		}
	}

	var variadic typ.Type
	if fn.Variadic != nil {
		variadic = fn.Variadic
		if isMethod && receiver != nil {
			variadic = subst.Self(variadic, receiver)
		}
	}

	return expected, variadic
}

// FinishCall completes the call synthesis using the inference result.
// If InferResult.ShortCircuit is non-nil, this returns immediately with that type.
// Otherwise, performs full argument checking and return type computation.
func FinishCall(ctx *db.QueryContext, def CallDef, infer InferResult) CallResult {
	if infer.ShortCircuit != nil {
		return singleValueCallResult(infer.ShortCircuit, infer.Errors)
	}

	switch infer.Kind {
	case InferKindNotCallable:
		return singleValueCallResult(typ.Unknown, infer.Errors)

	case InferKindAny:
		return singleValueCallResult(typ.Any, infer.Errors)

	case InferKindUnknown:
		return singleValueCallResult(typ.Unknown, infer.Errors)

	case InferKindUnion:
		return callUnionWithGenericInference(
			ctx,
			infer.Callee.(*typ.Union),
			def,
			infer.IsMethod,
			infer.Receiver,
			infer.ForceMethodReceiver,
			infer.Errors,
		)

	case InferKindIntersection:
		return callIntersection(ctx, def.Query, infer.Callee.(*typ.Intersection), def.Args, infer.Receiver, infer.IsMethod, infer.ForceMethodReceiver, infer.Errors)

	case InferKindFunction:
		fn := infer.Instantiated
		if fn == nil {
			fn = infer.Function
		}
		if fn == nil {
			return singleValueCallResult(typ.Unknown, infer.Errors)
		}
		return callFunction(ctx, def.Query, fn, def.Args, infer.Receiver, infer.IsMethod, infer.ForceMethodReceiver, infer.Errors)
	}

	return singleValueCallResult(typ.Unknown, infer.Errors)
}

// ReInfer performs re-inference after arguments have been updated.
// This is used when function literal arguments are re-synthesized with
// contextual typing, requiring fresh type argument inference.
func ReInfer(ctx *db.QueryContext, def CallDef, prev InferResult) InferResult {
	if prev.Kind != InferKindFunction || prev.Function == nil {
		return prev
	}

	fn := prev.Function
	if len(fn.TypeParams) == 0 {
		return prev
	}

	if len(def.TypeArgs) > 0 {
		return prev
	}

	typeArgs, err := InferTypeArgsWithExpectedAndMode(fn, def.Args, prev.IsMethod, prev.Receiver, nil, prev.ForceMethodReceiver)
	if err != nil {
		result := prev
		result.Errors = append(result.Errors, CallError{Kind: ErrTypeInference, Message: err.Error()})
		return result
	}

	result := prev
	result.TypeArgs = typeArgs
	result.Instantiated = InstantiateFunction(fn, typeArgs)
	result.ExpectedArgs, result.ExpectedVariadic = computeExpectedArgs(ctx, def.Query, result.Instantiated, prev.IsMethod, prev.Receiver, prev.ForceMethodReceiver)

	return result
}

// ExpectedArgType returns the expected type for an argument at the given index.
// Uses ExpectedArgs for regular params and ExpectedVariadic for extra args.
// Returns nil if the index is out of range or no expected type is available.
func (r *InferResult) ExpectedArgType(idx int) typ.Type {
	if idx < 0 {
		return nil
	}
	if idx < len(r.ExpectedArgs) {
		return r.ExpectedArgs[idx]
	}
	return r.ExpectedVariadic
}

// callIntersection handles calling an intersection type.
// All function members are called with the same args; if any member fails, the whole call fails.
// The return type is the intersection of all member return types.
func callIntersection(ctx *db.QueryContext, query core.TypeOps, inter *typ.Intersection, args []typ.Type, receiver typ.Type, isMethod bool, forceMethodReceiver bool, baseErrors []CallError) CallResult {
	var returnTypes []typ.Type
	var returnVectors [][]typ.Type

	for _, member := range inter.Members {
		if member.Kind().IsPlaceholder() {
			continue
		}

		fn, ok := member.(*typ.Function)
		if !ok {
			return CallResult{
				Type:    typ.Unknown,
				Returns: []typ.Type{typ.Unknown},
				Errors:  append(baseErrors, CallError{Kind: ErrNotCallable, Message: fmt.Sprintf("intersection member is not callable: %s", typ.FormatShort(member))}),
			}
		}

		seedErrors := append([]CallError(nil), baseErrors...)
		result := callFunction(ctx, query, fn, args, receiver, isMethod, forceMethodReceiver, seedErrors)
		if hasHardErrors(result.Errors[len(seedErrors):]) {
			return result
		}

		returnTypes = append(returnTypes, result.Type)
		returnVectors = append(returnVectors, normalizedCallReturns(result))
	}

	if len(returnTypes) == 0 {
		return singleValueCallResult(typ.Unknown, baseErrors)
	}

	if len(returnTypes) == 1 {
		return callResultFromReturns(returnVectors[0], baseErrors)
	}

	if returns, ok := intersectReturnVectors(returnVectors); ok {
		return callResultFromReturns(returns, baseErrors)
	}

	return CallResult{
		Type:    typ.NewIntersection(returnTypes...),
		Returns: []typ.Type{typ.NewIntersection(returnTypes...)},
		Errors:  baseErrors,
	}
}

// callUnionWithGenericInference handles calling a union of functions where each
// member may be generic. Per-member generic inference is applied before calling.
// Union semantics: the call succeeds if any member succeeds.
func callUnionWithGenericInference(ctx *db.QueryContext, u *typ.Union, def CallDef, isMethod bool, receiver typ.Type, forceMethodReceiver bool, baseErrors []CallError) CallResult {
	var validReturns [][]typ.Type
	var allReturns [][]typ.Type
	var hardErrors []CallError

	for _, member := range u.Members {
		fn, ok := member.(*typ.Function)
		if !ok {
			hardErrors = append(hardErrors, CallError{Kind: ErrNotCallable, Message: fmt.Sprintf("expected function, got %s", typ.FormatShort(member))})
			continue
		}

		seedErrors := append([]CallError(nil), baseErrors...)
		var result CallResult
		if len(fn.TypeParams) == 0 {
			result = callFunction(ctx, def.Query, fn, def.Args, receiver, isMethod, forceMethodReceiver, seedErrors)
		} else {
			result = inferAndCall(ctx, fn, def, isMethod, receiver, seedErrors)
		}
		allReturns = append(allReturns, normalizedCallReturns(result))

		if hasHardErrors(result.Errors[len(seedErrors):]) {
			hardErrors = append(hardErrors, result.Errors...)
			continue
		}

		validReturns = append(validReturns, normalizedCallReturns(result))
	}

	if len(validReturns) > 0 {
		return callResultFromReturns(mergeReturnVectors(validReturns), baseErrors)
	}

	if len(allReturns) > 0 {
		return callResultFromReturns(mergeReturnVectors(allReturns), uniqueCallErrors(hardErrors))
	}

	return singleValueCallResult(typ.Unknown, uniqueCallErrors(hardErrors))
}

func mergeReturnVectors(vectors [][]typ.Type) []typ.Type {
	if len(vectors) == 0 {
		return []typ.Type{typ.Unknown}
	}
	if len(vectors) == 1 {
		return copyTypeSlice(vectors[0])
	}

	maxLen := 0
	for _, returns := range vectors {
		if len(returns) > maxLen {
			maxLen = len(returns)
		}
	}
	if maxLen == 0 {
		return []typ.Type{typ.Nil}
	}

	merged := make([]typ.Type, maxLen)
	for i := 0; i < maxLen; i++ {
		slotTypes := make([]typ.Type, 0, len(vectors))
		for _, returns := range vectors {
			if i < len(returns) {
				slotTypes = append(slotTypes, returns[i])
			} else {
				slotTypes = append(slotTypes, typ.Nil)
			}
		}
		merged[i] = typ.NewUnion(slotTypes...)
	}
	return merged
}

func methodConsumesReceiver(ctx *db.QueryContext, query core.TypeOps, fn *typ.Function, receiver typ.Type, isMethod bool, forceMethodReceiver bool) bool {
	if !isMethod || receiver == nil || fn == nil {
		return false
	}
	if forceMethodReceiver {
		return true
	}
	return hasExplicitSelf(ctx, query, fn, receiver)
}

func methodConsumesReceiverSimple(fn *typ.Function, receiver typ.Type, isMethod bool, forceMethodReceiver bool) bool {
	if !isMethod || receiver == nil || fn == nil {
		return false
	}
	if forceMethodReceiver {
		return true
	}
	return hasExplicitSelfSimple(fn, receiver)
}

func callFunction(ctx *db.QueryContext, query core.TypeOps, fn *typ.Function, args []typ.Type, receiver typ.Type, isMethod bool, forceMethodReceiver bool, errors []CallError) CallResult {
	if fn == nil {
		return singleValueCallResult(typ.Unknown, append(errors, CallError{Kind: ErrNotCallable, Message: "nil function"}))
	}

	argCount := len(args)
	methodHasReceiver := methodConsumesReceiver(ctx, query, fn, receiver, isMethod, forceMethodReceiver)
	if methodHasReceiver {
		argCount++
	}

	minArgs := typ.MinRequiredArgs(fn)
	hasVariadic := fn.Variadic != nil
	allowExtraArgs := len(fn.Params) == 0 && !hasVariadic

	if argCount < minArgs {
		errors = append(errors, CallError{
			Kind:    ErrWrongArity,
			Message: "not enough arguments",
		})
	} else if !hasVariadic && !allowExtraArgs && argCount > len(fn.Params) {
		errors = append(errors, CallError{
			Kind:    ErrWrongArity,
			Message: "too many arguments",
		})
	}

	paramOffset := 0
	if methodHasReceiver {
		paramOffset = 1
	}

	if methodHasReceiver {
		var expectedReceiver typ.Type
		if len(fn.Params) > 0 {
			expectedReceiver = fn.Params[0].Type
		} else if hasVariadic {
			expectedReceiver = fn.Variadic
		}
		if expectedReceiver != nil {
			expectedReceiver = subst.Self(expectedReceiver, receiver)
			if !isSubtypeCheck(ctx, query, receiver, expectedReceiver) {
				errors = append(errors, CallError{
					Kind:    ErrTypeMismatch,
					Message: fmt.Sprintf("method receiver: expected %s, got %s", typ.FormatShort(expectedReceiver), typ.FormatShort(receiver)),
				})
			}
		}
	}

	for i, arg := range args {
		paramIdx := i + paramOffset

		var expectedType typ.Type

		if paramIdx < len(fn.Params) {
			expectedType = fn.Params[paramIdx].Type
		} else if hasVariadic {
			expectedType = fn.Variadic
		} else {
			continue
		}

		if isMethod && receiver != nil {
			expectedType = subst.Self(expectedType, receiver)
		}

		if expectedType != nil && arg != nil {
			if !isSubtypeCheck(ctx, query, arg, expectedType) {
				errors = append(errors, CallError{
					Kind:    ErrTypeMismatch,
					Message: fmt.Sprintf("argument %d: expected %s, got %s", i+1, typ.FormatShort(expectedType), typ.FormatShort(arg)),
					ArgIdx:  i + 1,
				})
			}
		}
	}

	returns := fn.Returns

	if isMethod && receiver != nil {
		returns = resolveSelf(returns, receiver)
	}

	if len(returns) == 0 {
		return singleValueCallResult(typ.Nil, errors)
	}

	return callResultFromReturns(returns, errors)
}

func normalizedCallReturns(result CallResult) []typ.Type {
	if len(result.Returns) > 0 {
		return copyTypeSlice(result.Returns)
	}
	return []typ.Type{result.Type}
}

func callResultFromReturns(returns []typ.Type, errors []CallError) CallResult {
	if len(returns) == 0 {
		return singleValueCallResult(typ.Nil, errors)
	}
	if len(returns) == 1 {
		return CallResult{
			Type:    returns[0],
			Returns: copyTypeSlice(returns),
			Errors:  errors,
		}
	}
	return CallResult{
		Type:    typ.NewTuple(returns...),
		Returns: copyTypeSlice(returns),
		Errors:  errors,
	}
}

func singleValueCallResult(t typ.Type, errors []CallError) CallResult {
	return CallResult{
		Type:    t,
		Returns: []typ.Type{t},
		Errors:  errors,
	}
}

func intersectReturnVectors(vectors [][]typ.Type) ([]typ.Type, bool) {
	if len(vectors) == 0 {
		return nil, false
	}
	if len(vectors) == 1 {
		return copyTypeSlice(vectors[0]), true
	}

	arity := len(vectors[0])
	for _, returns := range vectors[1:] {
		if len(returns) != arity {
			return nil, false
		}
	}

	merged := make([]typ.Type, arity)
	for i := 0; i < arity; i++ {
		slotTypes := make([]typ.Type, 0, len(vectors))
		for _, returns := range vectors {
			slotTypes = append(slotTypes, returns[i])
		}
		if len(slotTypes) == 1 {
			merged[i] = slotTypes[0]
			continue
		}
		merged[i] = typ.NewIntersection(slotTypes...)
	}
	return merged, true
}

func copyTypeSlice(types []typ.Type) []typ.Type {
	if len(types) == 0 {
		return nil
	}
	out := make([]typ.Type, len(types))
	copy(out, types)
	return out
}

func hasHardErrors(errors []CallError) bool {
	for _, err := range errors {
		switch err.Kind {
		case ErrWrongArity, ErrTypeMismatch, ErrNotCallable, ErrTypeInference:
			return true
		}
	}

	return false
}

func uniqueCallErrors(errors []CallError) []CallError {
	seen := make(map[string]bool)
	unique := make([]CallError, 0, len(errors))

	for _, err := range errors {
		key := fmt.Sprintf("%d|%d|%s", err.Kind, err.ArgIdx, err.Message)
		if seen[key] {
			continue
		}

		seen[key] = true

		unique = append(unique, err)
	}

	return unique
}

// hasExplicitSelf checks if function has explicit Self as first param.
func hasExplicitSelf(ctx *db.QueryContext, query core.TypeOps, fn *typ.Function, receiver typ.Type) bool {
	if len(fn.Params) == 0 {
		return false
	}
	receiverMatch := normalizeReceiverForSelfCheck(ctx, query, receiver)
	return hasExplicitSelfCommon(fn, receiver, receiverMatch, func(sub, super typ.Type) bool {
		return isSubtypeCheck(ctx, query, sub, super)
	})
}

func isLocalRefMatch(param typ.Type, receiver typ.Type) bool {
	ref, ok := param.(*typ.Ref)
	if !ok || ref.Module != "" {
		return false
	}

	name, ok := receiverAliasName(receiver)
	if !ok {
		return false
	}

	return ref.Name == name
}

func receiverAliasName(t typ.Type) (string, bool) {
	switch v := t.(type) {
	case *typ.Alias:
		return v.Name, true
	case *typ.Optional:
		return receiverAliasName(v.Inner)
	}

	return "", false
}

// isSubtypeCheck uses memoized query if available, otherwise falls back to package function.
func isSubtypeCheck(ctx *db.QueryContext, query core.TypeOps, sub, super typ.Type) bool {
	if query != nil {
		return query.IsSubtype(ctx, sub, super)
	}
	return subtype.IsSubtype(sub, super)
}

// hasExplicitSelfSimple is a non-memoized version for use in contexts without QueryContext.
func hasExplicitSelfSimple(fn *typ.Function, receiver typ.Type) bool {
	if len(fn.Params) == 0 {
		return false
	}
	receiverMatch := normalizeReceiverForSelfCheck(nil, nil, receiver)
	return hasExplicitSelfCommon(fn, receiver, receiverMatch, subtype.IsSubtype)
}

func hasExplicitSelfCommon(
	fn *typ.Function,
	receiver typ.Type,
	receiverMatch typ.Type,
	isSubtype func(sub, super typ.Type) bool,
) bool {
	if fn == nil || len(fn.Params) == 0 || isSubtype == nil {
		return false
	}

	if name := fn.Params[0].Name; name == "self" || name == "Self" {
		return true
	}

	firstParam := fn.Params[0].Type
	if firstParam == nil {
		return false
	}
	if firstParam.Kind() == kind.Self {
		return true
	}
	if tp, ok := firstParam.(*typ.TypeParam); ok {
		if tp.Constraint != nil && receiverMatch != nil &&
			isExplicitSelfSubtypeCandidate(receiverMatch) &&
			isExplicitSelfSubtypeCandidate(tp.Constraint) &&
			(isSubtype(receiverMatch, tp.Constraint) && isSubtype(tp.Constraint, receiverMatch)) {
			return true
		}
		return false
	}
	// Check if first param is structurally equivalent to the receiver.
	// One-way subtyping is too permissive for structural record types where
	// optional fields can make unrelated shapes look compatible.
	if receiverMatch != nil &&
		isExplicitSelfSubtypeCandidate(receiverMatch) &&
		isExplicitSelfSubtypeCandidate(firstParam) &&
		(isSubtype(receiverMatch, firstParam) && isSubtype(firstParam, receiverMatch)) {
		return true
	}

	return receiver != nil && isLocalRefMatch(firstParam, receiver)
}

func normalizeReceiverForSelfCheck(ctx *db.QueryContext, query core.TypeOps, receiver typ.Type) typ.Type {
	if receiver == nil {
		return nil
	}
	if query != nil {
		if widened := query.Widen(ctx, receiver); widened != nil {
			return widened
		}
	}
	return subtype.Widen(receiver)
}

// isExplicitSelfSubtypeCandidate filters out broad/placeholder shapes that are
// too permissive for implicit self inference (for example `any` and `unknown`).
func isExplicitSelfSubtypeCandidate(t typ.Type) bool {
	if t == nil {
		return false
	}
	// Soft placeholder types are intentionally broad and should not imply
	// implicit receiver consumption in method arity checks.
	return !typ.IsSoft(t, typ.SoftAnnotationPolicy)
}

// resolveSelf replaces Self type with concrete receiver type.
func resolveSelf(returns []typ.Type, receiver typ.Type) []typ.Type {
	result := make([]typ.Type, len(returns))
	for i, r := range returns {
		result[i] = subst.Self(r, receiver)
	}

	return result
}

// CallWithGenericInference synthesizes a call result with generic type inference.
// Wraps InferCall + FinishCall for cases where contextual re-synthesis is not needed.
// For the full two-phase flow with re-synthesis, use InferCall -> ReInfer -> FinishCall.
func CallWithGenericInference(ctx *db.QueryContext, def CallDef) CallResult {
	infer := InferCall(ctx, def)
	return FinishCall(ctx, def, infer)
}

// formatUnionMethodError creates a detailed error message for method access on union types.
func formatUnionMethodError(ctx *db.QueryContext, receiver typ.Type, methodName string, query core.TypeOps) string {
	union, ok := receiver.(*typ.Union)
	if !ok || query == nil {
		return "no method " + methodName
	}

	var hasMethod []string

	var noMethod []string

	for _, member := range union.Members {
		name := unionMemberName(member)
		if _, ok := core.Method(member, methodName); ok {
			hasMethod = append(hasMethod, name)
			continue
		}

		if _, ok := core.Field(member, methodName); ok {
			hasMethod = append(hasMethod, name)
			continue
		}

		noMethod = append(noMethod, name)
	}

	if len(hasMethod) == 0 {
		return "no method " + methodName
	}

	return "no method " + methodName + " (exists on " + joinTypeNames(hasMethod) + ", missing on " + joinTypeNames(noMethod) + ")"
}

func unionMemberName(t typ.Type) string {
	if t == nil {
		return "nil"
	}

	switch v := t.(type) {
	case *typ.Interface:
		return v.Name
	case *typ.Alias:
		return v.Name
	case *typ.Instantiated:
		if v.Generic != nil {
			return v.Generic.Name
		}
	}

	return typ.FormatShort(t)
}

func joinTypeNames(names []string) string {
	if len(names) == 0 {
		return ""
	}

	if len(names) == 1 {
		return names[0]
	}

	result := names[0]

	for i := 1; i < len(names); i++ {
		if i == len(names)-1 {
			result += " and " + names[i]
		} else {
			result += ", " + names[i]
		}
	}

	return result
}
