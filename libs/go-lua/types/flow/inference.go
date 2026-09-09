// inference.go implements function refinement inference from flow analysis results.
//
// Function refinements describe how a function's return value relates to its parameters.
// For predicate functions (returning boolean), refinements capture when parameters are
// narrowed based on the return value (OnTrue/OnFalse). For assert-style functions,
// refinements capture constraints that hold after the function returns (OnReturn).
//
// Refinements enable interprocedural narrowing: calling a predicate function in a
// conditional allows the checker to narrow argument types in the appropriate branch.
package flow

import (
	"fmt"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

// ParamInfo describes a function parameter for effect inference.
//
// Both Name and Symbol are needed: Name for user-facing constraint paths,
// Symbol for SSA-based path resolution. Type is included for completeness
// but not currently used during inference.
type ParamInfo struct {
	Name   string
	Symbol cfg.SymbolID
	Type   typ.Type
}

// InferFunctionRefinement computes a FunctionRefinement from solved flow analysis.
//
// This is the post-flow variant that uses the complete flow solution to determine
// which constraints hold at each return point. It examines return points to compute:
//
//   - OnTrue: Constraints that hold when returning true (boolean predicates)
//   - OnFalse: Constraints that hold when returning false (boolean predicates)
//   - OnReturn: Constraints that hold on normal return (assert-style functions)
//   - Terminates: Whether the function never returns normally
//
// For boolean-returning functions, OnTrue/OnFalse are populated based on return
// statement analysis. Return statements with literal true/false are classified via
// ReturnKinds. Return statements with predicate expressions use ReturnConstraints.
//
// For non-boolean functions, OnReturn is populated from path conditions at return points.
//
// All constraints are converted to use placeholders ($0, $1, ...) for parameters,
// enabling the effect to be instantiated with actual arguments at call sites.
//
// Example: For function `function is_string(x) return type(x) == "string" end`:
//   - OnTrue: HasType($0, string)
//   - OnFalse: NotHasType($0, string)
func InferFunctionRefinement(
	solution *Solution,
	g *cfg.CFG,
	params []ParamInfo,
	returnType typ.Type,
) *constraint.FunctionRefinement {
	if solution == nil || g == nil {
		return nil
	}

	src := refinementSource{conditionAt: solution.ConditionAt}
	if solution.inputs != nil {
		src.returnConstraints = solution.inputs.ReturnConstraints
		src.returnKinds = solution.inputs.ReturnKinds
	}

	return inferFunctionRefinementCore(src, g, params, returnType)
}

// InferFunctionRefinementFromInputs computes a FunctionRefinement without running full flow analysis.
//
// This is the pre-flow variant that uses only the extracted return constraints without
// propagating conditions through the CFG. It produces conservative effects based on:
//
//   - ReturnConstraints: Direct constraint extraction from return expressions
//   - CFG structure: Detecting terminating functions (no return/exit nodes)
//
// The pre-flow variant is faster but less precise than post-flow inference because
// it cannot account for path conditions from prior conditionals. It's suitable for
// bootstrapping refinement extraction before the full type checking pass.
//
// Example: For function `function assert_string(x) assert(type(x) == "string") end`:
//   - OnReturn: HasType($0, string) (from assert expression)
//   - Terminates: false (has implicit return via exit)
func InferFunctionRefinementFromInputs(
	inputs *Inputs,
	g *cfg.CFG,
	params []ParamInfo,
	returnType typ.Type,
) *constraint.FunctionRefinement {
	if inputs == nil || g == nil {
		return nil
	}

	src := refinementSource{returnConstraints: inputs.ReturnConstraints}

	return inferFunctionRefinementCore(src, g, params, returnType)
}

// refinementSource abstracts the data sources for refinement inference.
//
// This struct allows the same inference algorithm to work with both pre-flow
// and post-flow data by providing optional access to different data sources:
//
//   - returnConstraints: Always available, contains constraints extracted from
//     return expressions (e.g., return type(x) == "string" yields HasType(x, string))
//
//   - returnKinds: Post-flow only, classifies return statements as ReturnTrue,
//     ReturnFalse, or ReturnUnknown based on constant analysis
//
//   - conditionAt: Post-flow only, returns the full DNF condition at a CFG point
//     after propagation
//
// Nil fields indicate the data is unavailable, causing the inference to skip
// the corresponding analysis step.
type refinementSource struct {
	returnConstraints map[cfg.Point]ReturnExprConstraints
	returnKinds       map[cfg.Point]ReturnKind
	conditionAt       func(cfg.Point) constraint.Condition
}

// inferFunctionRefinementCore is the shared implementation for effect inference.
//
// This method walks the CFG to find all return and exit points, collecting
// constraints that hold at each. The algorithm:
//
//  1. Build parameter maps for constraint filtering and placeholder substitution
//  2. Iterate over CFG points in RPO order, processing NodeReturn and NodeExit
//  3. For each return point, combine base condition with return expression constraints
//  4. Accumulate conditions into OnTrue/OnFalse/OnReturn based on return kind
//  5. Filter to keep only parameter-referencing constraints
//  6. Substitute parameter names/symbols with placeholders ($0, $1, ...)
//  7. Detect terminating functions (no return nodes, exit unreachable)
//
// The refinementSource parameter routes queries to the appropriate data source
// (pre-flow inputs vs. post-flow solution).
func inferFunctionRefinementCore(
	src refinementSource,
	g *cfg.CFG,
	params []ParamInfo,
	returnType typ.Type,
) *constraint.FunctionRefinement {
	// Build param Symbol -> index and name -> index maps
	paramIndex := make(map[cfg.SymbolID]int)
	paramNameIndex := make(map[string]int)
	for i, p := range params {
		if p.Symbol != 0 {
			paramIndex[p.Symbol] = i
		}
		if p.Name != "" {
			paramNameIndex[p.Name] = i
		}
	}

	// Count return nodes to detect terminating functions
	hasReturnNode := false

	// Collect conditions at return points
	var onTrueCond constraint.Condition
	var onFalseCond constraint.Condition
	var onReturnCond constraint.Condition
	var exitCond constraint.Condition

	returnsBool := isBooleanReturnType(returnType)

	for _, p := range g.RPO() {
		node := g.Node(p)
		if node == nil {
			continue
		}

		// Consider both return and exit nodes for constraint collection
		if node.Kind != cfg.NodeReturn && node.Kind != cfg.NodeExit {
			continue
		}

		if node.Kind == cfg.NodeReturn {
			hasReturnNode = true
		}

		baseCond := constraint.TrueCondition()
		if src.conditionAt != nil {
			baseCond = src.conditionAt(p)
		}

		// Check for return expression constraints from predicate/assert calls
		if src.returnConstraints != nil {
			if rc, ok := src.returnConstraints[p]; ok {
				isPredicate := rc.OnTrue.HasConstraints() && rc.OnFalse.HasConstraints()
				if returnsBool || isPredicate {
					if rc.OnTrue.HasConstraints() {
						cond := constraint.And(baseCond, rc.OnTrue)
						onTrueCond = orCondition(onTrueCond, cond)
					}
					if rc.OnFalse.HasConstraints() {
						cond := constraint.And(baseCond, rc.OnFalse)
						onFalseCond = orCondition(onFalseCond, cond)
					}
				} else {
					if rc.OnTrue.HasConstraints() {
						cond := constraint.And(baseCond, rc.OnTrue)
						onReturnCond = orCondition(onReturnCond, cond)
					}
				}
				continue
			}
		}

		// For explicit boolean returns, check for literal true/false return statements
		if returnsBool && src.returnKinds != nil {
			switch src.returnKinds[p] {
			case ReturnTrue:
				onTrueCond = orCondition(onTrueCond, baseCond)
				continue
			case ReturnFalse:
				onFalseCond = orCondition(onFalseCond, baseCond)
				continue
			}
		}

		// For non-boolean or unclassified returns at NodeReturn (not NodeExit),
		// collect baseCond only if no return expression constraint was found.
		// Exit nodes don't represent actual return statements with expressions.
		if node.Kind == cfg.NodeReturn && src.conditionAt != nil {
			onReturnCond = orCondition(onReturnCond, baseCond)
		}

		// Collect exit node conditions for implicit returns
		if node.Kind == cfg.NodeExit && src.conditionAt != nil && baseCond.HasConstraints() {
			exitCond = orCondition(exitCond, baseCond)
		}
	}

	// For non-boolean functions without explicit return nodes,
	// merge exit conditions into onReturnCond (implicit fallthrough return)
	if !returnsBool && !hasReturnNode && !exitCond.IsFalse() {
		onReturnCond = orCondition(onReturnCond, exitCond)
	}

	if !onTrueCond.IsFalse() {
		onTrueCond = substituteToPlaceholdersCondition(filterParamCondition(onTrueCond, paramIndex, paramNameIndex), paramIndex, paramNameIndex)
	}
	if !onFalseCond.IsFalse() {
		onFalseCond = substituteToPlaceholdersCondition(filterParamCondition(onFalseCond, paramIndex, paramNameIndex), paramIndex, paramNameIndex)
	}
	if !onReturnCond.IsFalse() {
		onReturnCond = substituteToPlaceholdersCondition(filterParamCondition(onReturnCond, paramIndex, paramNameIndex), paramIndex, paramNameIndex)
	}

	exitHasPredecessors := len(graphPredecessors(g, g.Exit())) > 0
	terminates := !hasReturnNode && !exitHasPredecessors

	eff := &constraint.FunctionRefinement{
		OnTrue:     onTrueCond,
		OnFalse:    onFalseCond,
		OnReturn:   onReturnCond,
		Terminates: terminates,
	}
	if eff.IsEmpty() {
		return nil
	}
	return eff
}

// isBooleanReturnType checks if the return type is boolean.
//
// A function is considered to return boolean if its return type is:
//   - The boolean primitive type
//   - A boolean literal (true or false)
//
// This determines whether the function qualifies as a predicate for
// OnTrue/OnFalse effect inference.
func isBooleanReturnType(t typ.Type) bool {
	if t == nil {
		return false
	}

	switch t.Kind() {
	case kind.Boolean:
		return true
	case kind.Literal:
		if lit, ok := t.(*typ.Literal); ok {
			return lit.Base == kind.Boolean
		}
	}

	return false
}

// filterParamConstraints returns only constraints that reference function parameters.
//
// Effects are only meaningful when they constrain parameter types. Constraints that
// reference only local variables or globals are filtered out since they cannot be
// used at call sites for argument narrowing.
//
// Uses Symbol-based identity (SSA) as primary lookup, with name-based fallback for
// constraints that only have Root names (e.g., from pre-flow extraction).
func filterParamConstraints(set []constraint.Constraint, paramIndex map[cfg.SymbolID]int, paramNameIndex map[string]int) []constraint.Constraint {
	if len(set) == 0 {
		return nil
	}

	var filtered []constraint.Constraint

	for _, c := range set {
		references := false
		for _, path := range c.Paths() {
			if path.Symbol != 0 {
				if _, ok := paramIndex[path.Symbol]; ok {
					references = true
					break
				}
			}
			if path.Root != "" && paramNameIndex != nil {
				if _, ok := paramNameIndex[path.Root]; ok {
					references = true
					break
				}
			}
		}
		if references {
			filtered = append(filtered, c)
		}
	}

	if len(filtered) == 0 {
		return nil
	}

	return constraint.NewConjunction(filtered...)
}

// substituteToPlaceholders converts parameter references to placeholders ($0, $1, ...).
//
// Effects must be portable across call sites, so parameter-specific names/symbols
// are replaced with positional placeholders. The placeholder $N refers to the Nth
// parameter (0-indexed).
//
// This substitution enables effect instantiation: at a call site, $0 is replaced
// with the actual first argument's path, $1 with the second, etc.
//
// Uses Symbol-based identity (SSA) as primary lookup, with name-based fallback.
func substituteToPlaceholders(set []constraint.Constraint, paramIndex map[cfg.SymbolID]int, paramNameIndex map[string]int) []constraint.Constraint {
	if len(set) == 0 {
		return nil
	}

	placeholders := make(map[cfg.SymbolID]string, len(paramIndex))
	for sym, idx := range paramIndex {
		placeholders[sym] = fmt.Sprintf("$%d", idx)
	}

	placeholdersByName := make(map[string]string, len(paramNameIndex))
	for name, idx := range paramNameIndex {
		placeholdersByName[name] = fmt.Sprintf("$%d", idx)
	}

	var substituted []constraint.Constraint
	for _, c := range set {
		if sub := substitutePathsInConstraint(c, placeholders, placeholdersByName); sub != nil {
			substituted = append(substituted, sub)
		}
	}

	if len(substituted) == 0 {
		return nil
	}

	return constraint.NewConjunction(substituted...)
}

// substitutePathsInConstraint replaces path roots with placeholders.
//
// This function implements the core substitution logic for a single constraint.
// It handles each constraint type, replacing parameter paths with placeholder roots.
// The constraint type determines which paths to substitute:
//
//   - Unary constraints (Truthy, IsNil, HasType, etc.): Substitute Path
//   - Binary path constraints (EqPath, FieldEqualsPath): Substitute both paths
//   - Field/Index constraints: Substitute Target path
//
// Returns nil if no parameter reference is found (constraint references only
// locals/globals), causing the constraint to be filtered out of the effect.
func substitutePathsInConstraint(c constraint.Constraint, placeholders map[cfg.SymbolID]string, placeholdersByName map[string]string) constraint.Constraint {
	return constraint.VisitConstraint(c, constraint.ConstraintVisitor[constraint.Constraint]{
		Truthy: func(v constraint.Truthy) constraint.Constraint {
			if newRoot, ok := lookupPlaceholder(v.Path, placeholders, placeholdersByName); ok {
				return constraint.Truthy{Path: pathWithNewRoot(v.Path, newRoot)}
			}
			return nil
		},
		Falsy: func(v constraint.Falsy) constraint.Constraint {
			if newRoot, ok := lookupPlaceholder(v.Path, placeholders, placeholdersByName); ok {
				return constraint.Falsy{Path: pathWithNewRoot(v.Path, newRoot)}
			}
			return nil
		},
		IsNil: func(v constraint.IsNil) constraint.Constraint {
			if newRoot, ok := lookupPlaceholder(v.Path, placeholders, placeholdersByName); ok {
				return constraint.IsNil{Path: pathWithNewRoot(v.Path, newRoot)}
			}
			return nil
		},
		NotNil: func(v constraint.NotNil) constraint.Constraint {
			if newRoot, ok := lookupPlaceholder(v.Path, placeholders, placeholdersByName); ok {
				return constraint.NotNil{Path: pathWithNewRoot(v.Path, newRoot)}
			}
			return nil
		},
		HasType: func(v constraint.HasType) constraint.Constraint {
			if newRoot, ok := lookupPlaceholder(v.Path, placeholders, placeholdersByName); ok {
				return constraint.HasType{Path: pathWithNewRoot(v.Path, newRoot), Type: v.Type}
			}
			return nil
		},
		NotHasType: func(v constraint.NotHasType) constraint.Constraint {
			if newRoot, ok := lookupPlaceholder(v.Path, placeholders, placeholdersByName); ok {
				return constraint.NotHasType{Path: pathWithNewRoot(v.Path, newRoot), Type: v.Type}
			}
			return nil
		},
		HasField: func(v constraint.HasField) constraint.Constraint {
			if newRoot, ok := lookupPlaceholder(v.Path, placeholders, placeholdersByName); ok {
				return constraint.HasField{Path: pathWithNewRoot(v.Path, newRoot), Field: v.Field}
			}
			return nil
		},
		FieldEquals: func(v constraint.FieldEquals) constraint.Constraint {
			if newRoot, ok := lookupPlaceholder(v.Target, placeholders, placeholdersByName); ok {
				return constraint.FieldEquals{Target: pathWithNewRoot(v.Target, newRoot), Field: v.Field, Value: v.Value}
			}
			return nil
		},
		FieldNotEquals: func(v constraint.FieldNotEquals) constraint.Constraint {
			if newRoot, ok := lookupPlaceholder(v.Target, placeholders, placeholdersByName); ok {
				return constraint.FieldNotEquals{Target: pathWithNewRoot(v.Target, newRoot), Field: v.Field, Value: v.Value}
			}
			return nil
		},
		IndexEquals: func(v constraint.IndexEquals) constraint.Constraint {
			if newRoot, ok := lookupPlaceholder(v.Target, placeholders, placeholdersByName); ok {
				return constraint.IndexEquals{Target: pathWithNewRoot(v.Target, newRoot), Key: v.Key, Value: v.Value}
			}
			return nil
		},
		IndexNotEquals: func(v constraint.IndexNotEquals) constraint.Constraint {
			if newRoot, ok := lookupPlaceholder(v.Target, placeholders, placeholdersByName); ok {
				return constraint.IndexNotEquals{Target: pathWithNewRoot(v.Target, newRoot), Key: v.Key, Value: v.Value}
			}
			return nil
		},
		EqPath: func(v constraint.EqPath) constraint.Constraint {
			leftRoot, leftOk := lookupPlaceholder(v.Left, placeholders, placeholdersByName)
			rightRoot, rightOk := lookupPlaceholder(v.Right, placeholders, placeholdersByName)
			if leftOk && rightOk {
				return constraint.NewEqPath(
					pathWithNewRoot(v.Left, leftRoot),
					pathWithNewRoot(v.Right, rightRoot),
				)
			}
			if leftOk {
				return constraint.NewEqPath(pathWithNewRoot(v.Left, leftRoot), v.Right)
			}
			if rightOk {
				return constraint.NewEqPath(v.Left, pathWithNewRoot(v.Right, rightRoot))
			}
			return nil
		},
		NotEqPath: func(v constraint.NotEqPath) constraint.Constraint {
			leftRoot, leftOk := lookupPlaceholder(v.Left, placeholders, placeholdersByName)
			rightRoot, rightOk := lookupPlaceholder(v.Right, placeholders, placeholdersByName)
			if leftOk && rightOk {
				return constraint.NewNotEqPath(
					pathWithNewRoot(v.Left, leftRoot),
					pathWithNewRoot(v.Right, rightRoot),
				)
			}
			if leftOk {
				return constraint.NewNotEqPath(pathWithNewRoot(v.Left, leftRoot), v.Right)
			}
			if rightOk {
				return constraint.NewNotEqPath(v.Left, pathWithNewRoot(v.Right, rightRoot))
			}
			return nil
		},
		FieldEqualsPath: func(v constraint.FieldEqualsPath) constraint.Constraint {
			targetRoot, targetOk := lookupPlaceholder(v.Target, placeholders, placeholdersByName)
			valueRoot, valueOk := lookupPlaceholder(v.Value, placeholders, placeholdersByName)
			if !targetOk && !valueOk {
				return nil
			}
			target := v.Target
			value := v.Value
			if targetOk {
				target = pathWithNewRoot(v.Target, targetRoot)
			}
			if valueOk {
				value = pathWithNewRoot(v.Value, valueRoot)
			}
			return constraint.FieldEqualsPath{Target: target, Field: v.Field, Value: value}
		},
		FieldNotEqualsPath: func(v constraint.FieldNotEqualsPath) constraint.Constraint {
			targetRoot, targetOk := lookupPlaceholder(v.Target, placeholders, placeholdersByName)
			valueRoot, valueOk := lookupPlaceholder(v.Value, placeholders, placeholdersByName)
			if !targetOk && !valueOk {
				return nil
			}
			target := v.Target
			value := v.Value
			if targetOk {
				target = pathWithNewRoot(v.Target, targetRoot)
			}
			if valueOk {
				value = pathWithNewRoot(v.Value, valueRoot)
			}
			return constraint.FieldNotEqualsPath{Target: target, Field: v.Field, Value: value}
		},
		IndexEqualsPath: func(v constraint.IndexEqualsPath) constraint.Constraint {
			targetRoot, targetOk := lookupPlaceholder(v.Target, placeholders, placeholdersByName)
			valueRoot, valueOk := lookupPlaceholder(v.Value, placeholders, placeholdersByName)
			if !targetOk && !valueOk {
				return nil
			}
			target := v.Target
			value := v.Value
			if targetOk {
				target = pathWithNewRoot(v.Target, targetRoot)
			}
			if valueOk {
				value = pathWithNewRoot(v.Value, valueRoot)
			}
			return constraint.IndexEqualsPath{Target: target, Key: v.Key, Value: value}
		},
		IndexNotEqualsPath: func(v constraint.IndexNotEqualsPath) constraint.Constraint {
			targetRoot, targetOk := lookupPlaceholder(v.Target, placeholders, placeholdersByName)
			valueRoot, valueOk := lookupPlaceholder(v.Value, placeholders, placeholdersByName)
			if !targetOk && !valueOk {
				return nil
			}
			target := v.Target
			value := v.Value
			if targetOk {
				target = pathWithNewRoot(v.Target, targetRoot)
			}
			if valueOk {
				value = pathWithNewRoot(v.Value, valueRoot)
			}
			return constraint.IndexNotEqualsPath{Target: target, Key: v.Key, Value: value}
		},
		Default: func(constraint.Constraint) constraint.Constraint {
			return nil
		},
	})
}

// lookupPlaceholder returns the placeholder for a path if it references a parameter.
//
// Lookup priority:
//  1. Symbol-based: If path has Symbol, look up in placeholders map (SSA identity)
//  2. Name-based: If path has Root, look up in placeholdersByName (string identity)
//
// Returns ("$N", true) if the path references parameter N, or ("", false) otherwise.
func lookupPlaceholder(p constraint.Path, placeholders map[cfg.SymbolID]string, placeholdersByName map[string]string) (string, bool) {
	if p.Symbol != 0 {
		if placeholder, ok := placeholders[p.Symbol]; ok {
			return placeholder, true
		}
	}
	if p.Root != "" && placeholdersByName != nil {
		if placeholder, ok := placeholdersByName[p.Root]; ok {
			return placeholder, true
		}
	}
	return "", false
}

// pathWithNewRoot creates a new path with a placeholder root.
//
// The Symbol is cleared because placeholder paths ($0, $1, ...) are templates
// that will be instantiated with actual argument paths at call sites. The original
// SSA symbol is not valid outside the defining function's scope.
//
// Segments are preserved so field/index paths can reference argument members:
// $0.field narrows the first argument's field, not just the argument itself.
func pathWithNewRoot(p constraint.Path, newRoot string) constraint.Path {
	return constraint.Path{
		Root:     newRoot,
		Symbol:   0,
		Segments: p.Segments,
	}
}

// filterParamCondition filters a DNF condition to keep only parameter-referencing constraints.
//
// Applies filterParamConstraints to each disjunct. If any disjunct becomes empty
// (all constraints filtered out), the entire condition becomes True (no restriction).
// This is sound: an empty disjunct in DNF is True, and True OR anything is True.
func filterParamCondition(cond constraint.Condition, paramIndex map[cfg.SymbolID]int, paramNameIndex map[string]int) constraint.Condition {
	if cond.IsFalse() {
		return cond
	}

	var disjuncts [][]constraint.Constraint
	for _, d := range cond.Disjuncts {
		filtered := filterParamConstraints(d, paramIndex, paramNameIndex)
		if len(filtered) == 0 {
			return constraint.TrueCondition()
		}
		disjuncts = append(disjuncts, filtered)
	}

	if len(disjuncts) == 0 {
		return constraint.TrueCondition()
	}
	return constraint.FromDisjuncts(disjuncts)
}

// substituteToPlaceholdersCondition substitutes parameter paths in a DNF condition.
//
// Applies substituteToPlaceholders to each disjunct. If any disjunct becomes empty
// (no constraints survive substitution), the entire condition becomes True.
func substituteToPlaceholdersCondition(cond constraint.Condition, paramIndex map[cfg.SymbolID]int, paramNameIndex map[string]int) constraint.Condition {
	if cond.IsFalse() {
		return cond
	}

	var disjuncts [][]constraint.Constraint
	for _, d := range cond.Disjuncts {
		sub := substituteToPlaceholders(d, paramIndex, paramNameIndex)
		if len(sub) == 0 {
			return constraint.TrueCondition()
		}
		disjuncts = append(disjuncts, sub)
	}

	if len(disjuncts) == 0 {
		return constraint.TrueCondition()
	}
	return constraint.FromDisjuncts(disjuncts)
}

// orCondition combines two conditions with OR, handling empty/false cases.
//
// This is the accumulator pattern for building OnTrue/OnFalse/OnReturn conditions
// from multiple return points. Each return point contributes a path condition,
// and OR combines them (a value can come from any return point).
//
// If either condition is False (zero disjuncts), returns the other unchanged.
// This avoids polluting the result with unreachable return point conditions.
func orCondition(acc, next constraint.Condition) constraint.Condition {
	if acc.IsFalse() {
		return next
	}
	if next.IsFalse() {
		return acc
	}
	return constraint.Or(acc, next)
}
