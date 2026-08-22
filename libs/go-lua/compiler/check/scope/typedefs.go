package scope

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/typ"
)

// TypeDefResolver resolves a type definition to its concrete type.
//
// This callback is invoked during scope construction when a TypeDef CFG node
// is encountered. The resolver translates the AST type expression into a
// concrete typ.Type, using the current scope state to resolve type references.
//
// Parameters:
//   - name: The type alias name being defined
//   - typeExpr: The AST type expression (right-hand side of the definition)
//   - typeParams: Generic type parameters for the definition (may be empty)
//   - sc: Current scope state for resolving type references
//
// The resolver should return a typ.Generic for parameterized types, or the
// resolved type directly for non-generic definitions.
type TypeDefResolver func(name string, typeExpr ast.TypeExpr, typeParams []ast.TypeParamExpr, sc *State) typ.Type

// BuildTypeDefScopes walks TypeDef nodes in RPO order and returns a scope
// for each point that includes all type definitions visible at that point.
//
// This function implements forward propagation of type definitions through
// the control flow graph. Each TypeDef node introduces a new type alias
// that becomes visible at all subsequent points (in RPO order).
//
// Processing order uses Reverse Post Order (RPO) to ensure that definitions
// are processed before their uses in straight-line code. For generic types,
// the resolved type is stored directly; for non-generic types, an alias
// wrapper preserves the user-defined name.
//
// The returned map provides O(1) lookup of the scope state at any CFG point,
// enabling efficient type resolution during expression synthesis.
func BuildTypeDefScopes(
	graph *cfg.Graph,
	base *State,
	resolver TypeDefResolver,
) map[cfg.Point]*State {
	scopes := make(map[cfg.Point]*State)
	current := base

	for _, p := range graph.RPO() {
		current = applyTypeDefAtPoint(graph, p, current, resolver)
		scopes[p] = current
	}

	return scopes
}

// EnrichWithTypeDefs walks TypeDef nodes in RPO order and returns a scope
// that includes all type definitions from the graph.
//
// This is a convenience wrapper around BuildTypeDefScopes that returns only
// the final accumulated scope. Use this when you need the complete type
// namespace after processing all definitions, but don't need per-point
// scope snapshots.
//
// Common use case: Building the scope for a module's top-level, where all
// type definitions should be visible to subsequent analysis.
func EnrichWithTypeDefs(
	graph *cfg.Graph,
	base *State,
	resolver TypeDefResolver,
) *State {
	current := base

	for _, p := range graph.RPO() {
		current = applyTypeDefAtPoint(graph, p, current, resolver)
	}

	return current
}

func applyTypeDefAtPoint(graph *cfg.Graph, p cfg.Point, current *State, resolver TypeDefResolver) *State {
	if graph == nil || resolver == nil {
		return current
	}
	info := graph.TypeDef(p)
	if info == nil || info.Name == "" || info.TypeExpr == nil {
		return current
	}
	resolved := resolver(info.Name, info.TypeExpr, ToTypeParamExprs(info.TypeParams), current)
	if resolved == nil {
		return current
	}
	if _, isGeneric := resolved.(*typ.Generic); isGeneric {
		return current.WithType(info.Name, resolved)
	}
	return current.WithType(info.Name, typ.NewAlias(info.Name, resolved))
}

// ToTypeParamExprs converts cfg type params to ast type param expressions.
//
// CFG stores type parameters in a simplified form (TypeParamInfo) that captures
// name and constraint. The AST representation (TypeParamExpr) is needed for
// type resolution, which operates on AST nodes. This function bridges the two
// representations.
func ToTypeParamExprs(params []cfg.TypeParamInfo) []ast.TypeParamExpr {
	if len(params) == 0 {
		return nil
	}
	out := make([]ast.TypeParamExpr, len(params))
	for i, p := range params {
		out[i] = ast.TypeParamExpr{Name: p.Name, Constraint: p.Constraint}
	}
	return out
}
