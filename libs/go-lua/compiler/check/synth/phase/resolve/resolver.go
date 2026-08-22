// Package resolve provides type expression resolution for AST type annotations.
//
// The Resolver converts AST type expressions (ast.TypeExpr) into concrete
// type representations (typ.Type). It handles all type expression forms:
//
// Primitive types: nil, boolean, number, integer, string, any, unknown, never
// Composite types: arrays, maps, records, tuples, functions
// Type constructors: optionals (?), unions (|), intersections (&)
// Generic types: instantiation with type arguments
// Advanced types: keyof, index access, conditional types, typeof
//
// Resolution is context-sensitive, using scope.State to resolve:
//   - Type parameters in generic contexts
//   - Named types defined in the current scope
//   - Module imports via qualified names
//
// Recursion is limited to maxTypeDepth to prevent infinite loops
// from recursive type definitions.
package resolve

import (
	"fmt"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth/phase/core"
	typecfg "github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/numparse"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

// maxTypeDepth limits recursion depth during type resolution.
// Prevents infinite loops from recursive or deeply nested type definitions.
const maxTypeDepth = 64

// Resolver converts AST type expressions to concrete type representations.
//
// Thread safety: Resolver is stateless and safe for concurrent use.
type Resolver struct {
	manifests      io.ManifestQuerier
	exprSynth      api.ExprSynth
	bindings       core.ParamSymbolLookup
	moduleBindings *bind.BindingTable
	moduleAliases  map[typecfg.SymbolID]string
}

// Config configures a Resolver.
type Config struct {
	Manifests      io.ManifestQuerier
	ExprSynth      api.ExprSynth
	Bindings       core.ParamSymbolLookup
	ModuleBindings *bind.BindingTable
	ModuleAliases  map[typecfg.SymbolID]string
}

// New creates a new type resolver.
func New(c Config) *Resolver {
	return &Resolver{
		manifests:      c.Manifests,
		exprSynth:      c.ExprSynth,
		bindings:       c.Bindings,
		moduleBindings: c.ModuleBindings,
		moduleAliases:  c.ModuleAliases,
	}
}

// ResolveType converts a type expression AST node to a concrete type.
//
// Returns typ.Unknown for nil expressions or unknown type references.
// Uses scope for resolving named types, type parameters, and self type.
func (r *Resolver) ResolveType(expr ast.TypeExpr, sc *scope.State) typ.Type {
	return r.resolveTypeDepth(expr, sc, 0)
}

// ResolveReturnTypes resolves multiple return type expressions, expanding tuples.
func (r *Resolver) ResolveReturnTypes(types []ast.TypeExpr, sc *scope.State) []typ.Type {
	if len(types) == 0 {
		return nil
	}
	out := make([]typ.Type, 0, len(types))
	for _, rt := range types {
		if tuple, ok := rt.(*ast.TupleTypeExpr); ok {
			for _, elem := range tuple.Elements {
				out = append(out, r.ResolveType(elem, sc))
			}
			continue
		}
		out = append(out, r.ResolveType(rt, sc))
	}
	return out
}

// ResolveFunctionSignature extracts a function signature from annotations only.
//
// Unlike SynthFunctionType which may infer types from the body, this method
// only uses explicit type annotations. Returns nil if fn is nil.
//
// Used for mutual recursion scenarios where function types must be known
// before analyzing bodies (e.g., two functions that call each other).
func (r *Resolver) ResolveFunctionSignature(fn *ast.FunctionExpr, sc *scope.State) *typ.Function {
	if fn == nil {
		return nil
	}
	builder := typ.Func()

	resolveScope := sc
	if len(fn.TypeParams) > 0 {
		typeParams := make(map[string]typ.Type, len(fn.TypeParams))
		for _, tp := range fn.TypeParams {
			var constr typ.Type
			if tp.Constraint != nil {
				constr = r.ResolveType(tp.Constraint, resolveScope)
			}
			typeParams[tp.Name] = typ.NewTypeParam(tp.Name, constr)
			builder = builder.TypeParam(tp.Name, constr)
		}
		resolveScope = resolveScope.WithTypeParams(typeParams)
	}

	implicitSelf := core.HasImplicitSelfParam(fn, r.bindings)
	var implicitSelfType typ.Type
	if implicitSelf && resolveScope != nil && resolveScope.SelfType() != nil {
		implicitSelfType = resolveScope.SelfType()
	}

	core.ApplyParamList(builder, fn, core.ParamListConfig{
		ResolveType:      r.ResolveType,
		ResolveScope:     resolveScope,
		ImplicitSelf:     implicitSelf,
		ImplicitSelfType: implicitSelfType,
	})

	if len(fn.ReturnTypes) > 0 {
		returns := r.ResolveReturnTypes(fn.ReturnTypes, resolveScope)
		if len(returns) > 0 {
			builder.Returns(returns...)
		}
	}

	return builder.Build()
}

// ResolveTypeDef resolves a type alias or generic type definition.
//
// For non-generic types: directly resolves the type expression.
// For generic types: creates a typ.Generic with type parameters that can
// be instantiated later. Supports recursive types via forward reference.
//
// Example:
//
//	type List<T> = {head: T, tail: List<T>?}
//
// Creates a Generic with body referencing itself through the forward ref.
func (r *Resolver) ResolveTypeDef(name string, typeExpr ast.TypeExpr, typeParams []ast.TypeParamExpr, sc *scope.State) typ.Type {
	if len(typeParams) > 0 {
		params := make([]*typ.TypeParam, len(typeParams))
		paramScope := sc
		for i, tp := range typeParams {
			var constr typ.Type
			if tp.Constraint != nil {
				constr = r.ResolveType(tp.Constraint, paramScope)
			}
			params[i] = typ.NewTypeParam(tp.Name, constr)
			paramScope = paramScope.WithTypeParams(map[string]typ.Type{tp.Name: params[i]})
		}

		forwardRef := typ.NewGeneric(name, params, nil)
		bodyScope := paramScope.WithType(name, forwardRef)

		body := r.ResolveType(typeExpr, bodyScope)
		return typ.NewGeneric(name, params, body)
	}
	return r.resolveNonGenericTypeDef(name, typeExpr, sc)
}

// resolveNonGenericTypeDef resolves a non-generic type alias, preserving
// self-recursive aliases as canonical recursive types.
//
// The resolution uses two passes:
//   - Pass 1 builds a provisional body to detect self recursion and seed the
//     recursive placeholder with a structurally correct body.
//   - Pass 2 rebuilds the body once the placeholder has a real body so any
//     enclosing function/record hashes that mention self are finalized against
//     the completed recursive shape rather than an empty placeholder.
func (r *Resolver) resolveNonGenericTypeDef(name string, typeExpr ast.TypeExpr, sc *scope.State) typ.Type {
	self := typ.NewRecursivePlaceholder(name)
	bodyScope := sc.WithType(name, self)

	provisional := r.ResolveType(typeExpr, bodyScope)
	if !containsRecursiveRef(provisional, self, 0) {
		return provisional
	}

	self.SetBody(provisional)
	finalBody := r.ResolveType(typeExpr, bodyScope)
	self.SetBody(finalBody)
	return self
}

func containsRecursiveRef(t typ.Type, self *typ.Recursive, depth int) bool {
	if t == nil || self == nil || typ.DepthExceeded(depth) {
		return false
	}
	if t == self {
		return true
	}

	return typ.Visit(t, typ.Visitor[bool]{
		Optional: func(o *typ.Optional) bool {
			return containsRecursiveRef(o.Inner, self, depth+1)
		},
		Union: func(u *typ.Union) bool {
			for _, m := range u.Members {
				if containsRecursiveRef(m, self, depth+1) {
					return true
				}
			}
			return false
		},
		Intersection: func(in *typ.Intersection) bool {
			for _, m := range in.Members {
				if containsRecursiveRef(m, self, depth+1) {
					return true
				}
			}
			return false
		},
		Array: func(a *typ.Array) bool {
			return containsRecursiveRef(a.Element, self, depth+1)
		},
		Map: func(m *typ.Map) bool {
			return containsRecursiveRef(m.Key, self, depth+1) ||
				containsRecursiveRef(m.Value, self, depth+1)
		},
		Tuple: func(tup *typ.Tuple) bool {
			for _, elem := range tup.Elements {
				if containsRecursiveRef(elem, self, depth+1) {
					return true
				}
			}
			return false
		},
		Function: func(fn *typ.Function) bool {
			for _, p := range fn.Params {
				if containsRecursiveRef(p.Type, self, depth+1) {
					return true
				}
			}
			if containsRecursiveRef(fn.Variadic, self, depth+1) {
				return true
			}
			for _, ret := range fn.Returns {
				if containsRecursiveRef(ret, self, depth+1) {
					return true
				}
			}
			return false
		},
		Record: func(rec *typ.Record) bool {
			for _, f := range rec.Fields {
				if containsRecursiveRef(f.Type, self, depth+1) {
					return true
				}
			}
			return containsRecursiveRef(rec.Metatable, self, depth+1) ||
				containsRecursiveRef(rec.MapKey, self, depth+1) ||
				containsRecursiveRef(rec.MapValue, self, depth+1)
		},
		Alias: func(a *typ.Alias) bool {
			return containsRecursiveRef(a.Target, self, depth+1)
		},
		Interface: func(iface *typ.Interface) bool {
			for _, m := range iface.Methods {
				if containsRecursiveRef(m.Type, self, depth+1) {
					return true
				}
			}
			return false
		},
		Instantiated: func(inst *typ.Instantiated) bool {
			if inst.Generic != nil && containsRecursiveRef(inst.Generic.Body, self, depth+1) {
				return true
			}
			for _, arg := range inst.TypeArgs {
				if containsRecursiveRef(arg, self, depth+1) {
					return true
				}
			}
			return false
		},
		Recursive: func(rec *typ.Recursive) bool {
			return rec == self
		},
		Default: func(typ.Type) bool {
			return false
		},
	})
}

func (r *Resolver) resolveTypeDepth(expr ast.TypeExpr, sc *scope.State, depth int) typ.Type {
	if expr == nil || depth > maxTypeDepth {
		return typ.Unknown
	}

	switch te := expr.(type) {
	case *ast.PrimitiveTypeExpr:
		return r.resolvePrimitive(te, sc)

	case *ast.OptionalTypeExpr:
		inner := r.resolveTypeDepth(te.Inner, sc, depth+1)
		return typ.NewOptional(inner)

	case *ast.UnionTypeExpr:
		return r.resolveUnion(te, sc, depth)

	case *ast.IntersectionTypeExpr:
		return r.resolveIntersection(te, sc, depth)

	case *ast.ArrayTypeExpr:
		elem := r.resolveTypeDepth(te.Element, sc, depth+1)
		if len(te.ElementAnnotations) > 0 {
			elem = r.wrapWithAnnotations(elem, te.ElementAnnotations)
		}
		arr := typ.NewArray(elem)
		if len(te.ArrayAnnotations) > 0 {
			return r.wrapWithAnnotations(arr, te.ArrayAnnotations)
		}
		return arr

	case *ast.MapTypeExpr:
		key := r.resolveTypeDepth(te.Key, sc, depth+1)
		value := r.resolveTypeDepth(te.Value, sc, depth+1)
		return typ.NewMap(key, value)

	case *ast.RecordTypeExpr:
		return r.resolveRecord(te, sc, depth)

	case *ast.FunctionTypeExpr:
		return r.resolveFunction(te, sc, depth)

	case *ast.TypeRefExpr:
		return r.resolveRef(te, sc)

	case *ast.GenericTypeExpr:
		return r.resolveGeneric(te, sc, depth)

	case *ast.LiteralTypeExpr:
		return resolveLiteral(te)

	case *ast.TupleTypeExpr:
		return r.resolveTuple(te, sc, depth)

	case *ast.MetaTypeExpr:
		inner := r.resolveTypeDepth(te.Inner, sc, depth+1)
		return typ.NewMeta(inner)

	case *ast.SelfTypeExpr:
		if sc != nil {
			if self := sc.SelfType(); self != nil {
				return self
			}
		}
		return typ.Self

	case *ast.TypeOfExpr:
		return r.resolveTypeOf(te)

	case *ast.KeyOfExpr:
		inner := r.resolveTypeDepth(te.Inner, sc, depth+1)
		return ComputeKeyOf(inner)

	case *ast.IndexAccessExpr:
		obj := r.resolveTypeDepth(te.Object, sc, depth+1)
		key := r.resolveTypeDepth(te.Index, sc, depth+1)
		return ComputeIndexAccess(obj, key)

	case *ast.ConditionalTypeExpr:
		return r.resolveConditional(te, sc, depth)

	default:
		return typ.Unknown
	}
}

func (r *Resolver) resolvePrimitive(te *ast.PrimitiveTypeExpr, sc *scope.State) typ.Type {
	var base typ.Type
	switch te.Name {
	case "nil":
		base = typ.Nil
	case "boolean":
		base = typ.Boolean
	case "number":
		base = typ.Number
	case "integer":
		base = typ.Integer
	case "string":
		base = typ.String
	case "any":
		base = typ.Any
	case "unknown":
		base = typ.Unknown
	case "never":
		base = typ.Never
	case "Self", "self":
		if sc != nil {
			if self := sc.SelfType(); self != nil {
				base = self
			} else {
				base = typ.Self
			}
		} else {
			base = typ.Self
		}
	default:
		base = r.resolveNamed(te.Name, sc)
	}
	return r.wrapWithAnnotations(base, te.Annotations)
}

func (r *Resolver) wrapWithAnnotations(t typ.Type, astAnnotations []ast.AnnotationExpr) typ.Type {
	if len(astAnnotations) == 0 {
		return t
	}
	annotations := make([]typ.Annotation, 0, len(astAnnotations))
	for _, ann := range astAnnotations {
		var arg any
		if len(ann.Args) > 0 {
			arg = extractLiteralValue(ann.Args[0])
		}
		annotations = append(annotations, typ.Annotation{Name: ann.Name, Arg: arg})
	}
	return typ.NewAnnotated(t, annotations)
}

func extractLiteralValue(expr ast.Expr) any {
	switch e := expr.(type) {
	case *ast.NumberExpr:
		// NumberExpr.Value is a string, parse it
		if f, err := parseNumber(e.Value); err == nil {
			return f
		}
		return nil
	case *ast.StringExpr:
		return e.Value
	case *ast.TrueExpr:
		return true
	case *ast.FalseExpr:
		return false
	default:
		return nil
	}
}

func parseNumber(s string) (float64, error) {
	f, ok := numparse.ParseFloatLiteral(s)
	if !ok {
		return 0, fmt.Errorf("invalid number literal: %q", s)
	}
	return f, nil
}

func (r *Resolver) resolveNamed(name string, sc *scope.State) typ.Type {
	if sc != nil {
		if tp, ok := sc.LookupTypeParam(name); ok {
			return tp
		}
		if t, ok := sc.LookupType(name); ok {
			return t
		}
	}
	return typ.NewRef("", name)
}

func (r *Resolver) resolveUnion(te *ast.UnionTypeExpr, sc *scope.State, depth int) typ.Type {
	if len(te.Types) == 0 {
		return typ.Never
	}
	members := make([]typ.Type, 0, len(te.Types))
	for _, t := range te.Types {
		members = append(members, r.resolveTypeDepth(t, sc, depth+1))
	}
	return typ.NewUnion(members...)
}

func (r *Resolver) resolveIntersection(te *ast.IntersectionTypeExpr, sc *scope.State, depth int) typ.Type {
	if len(te.Types) == 0 {
		return typ.Any
	}
	members := make([]typ.Type, 0, len(te.Types))
	for _, t := range te.Types {
		members = append(members, r.resolveTypeDepth(t, sc, depth+1))
	}
	return typ.NewIntersection(members...)
}

func (r *Resolver) resolveRecord(te *ast.RecordTypeExpr, sc *scope.State, depth int) typ.Type {
	builder := typ.NewRecord()
	for _, f := range te.Fields {
		fieldType := r.resolveTypeDepth(f.Type, sc, depth+1)
		if len(f.Annotations) > 0 {
			annotations := make([]typ.Annotation, 0, len(f.Annotations))
			for _, ann := range f.Annotations {
				var arg any
				if len(ann.Args) > 0 {
					arg = extractLiteralValue(ann.Args[0])
				}
				annotations = append(annotations, typ.Annotation{Name: ann.Name, Arg: arg})
			}
			if len(annotations) > 0 {
				fieldType = typ.NewAnnotated(fieldType, annotations)
			}
		}
		if f.Optional {
			builder.OptField(f.Name, fieldType)
		} else {
			builder.Field(f.Name, fieldType)
		}
	}
	return builder.Build()
}

func (r *Resolver) resolveFunction(te *ast.FunctionTypeExpr, sc *scope.State, depth int) typ.Type {
	builder := typ.Func()

	for _, tp := range te.TypeParams {
		var constr typ.Type
		if tp.Constraint != nil {
			constr = r.resolveTypeDepth(tp.Constraint, sc, depth+1)
		}
		builder.TypeParam(tp.Name, constr)
	}

	for _, p := range te.Params {
		paramType := r.resolveTypeDepth(p.Type, sc, depth+1)
		if _, ok := p.Type.(*ast.OptionalTypeExpr); ok {
			builder.OptParam(p.Name, paramType)
			continue
		}
		builder.Param(p.Name, paramType)
	}

	if te.Variadic != nil {
		builder.Variadic(r.resolveTypeDepth(te.Variadic, sc, depth+1))
	}

	var assertsExpr *ast.AssertsTypeExpr
	var returns []typ.Type
	for _, ret := range te.Returns {
		if ae, ok := ret.(*ast.AssertsTypeExpr); ok {
			assertsExpr = ae
			continue
		}
		if tuple, ok := ret.(*ast.TupleTypeExpr); ok {
			for _, elem := range tuple.Elements {
				returns = append(returns, r.resolveTypeDepth(elem, sc, depth+1))
			}
			continue
		}
		returns = append(returns, r.resolveTypeDepth(ret, sc, depth+1))
	}
	if len(returns) > 0 {
		builder.Returns(returns...)
	}

	if assertsExpr != nil {
		paramIdx := -1
		for i, p := range te.Params {
			if p.Name == assertsExpr.ParamName {
				paramIdx = i
				break
			}
		}
		if paramIdx >= 0 {
			eff := r.buildAssertEffect(paramIdx, assertsExpr.NarrowTo, sc, depth)
			if eff != nil {
				builder.WithRefinement(eff)
			}
		}
	}

	return builder.Build()
}

func (r *Resolver) buildAssertEffect(paramIdx int, narrowTo ast.TypeExpr, sc *scope.State, depth int) *constraint.FunctionRefinement {
	placeholder := fmt.Sprintf("$%d", paramIdx)
	path := constraint.Path{Root: placeholder}

	var constraints []constraint.Constraint
	if narrowTo != nil {
		narrowType := r.resolveTypeDepth(narrowTo, sc, depth+1)
		if narrowType != nil {
			constraints = append(constraints, constraint.HasType{
				Path: path,
				Type: narrow.HashTypeKey(narrowType.Hash()),
			})
		}
	} else {
		constraints = append(constraints, constraint.NotNil{Path: path})
	}

	if len(constraints) == 0 {
		return nil
	}
	return constraint.NewRefinement(constraints, nil, nil)
}

func (r *Resolver) resolveRef(te *ast.TypeRefExpr, sc *scope.State) typ.Type {
	if len(te.Path) == 0 {
		return typ.Unknown
	}

	if len(te.Path) == 1 {
		name := te.Path[0]
		if sc != nil {
			if tp, ok := sc.LookupTypeParam(name); ok {
				return tp
			}
			if t, ok := sc.LookupType(name); ok {
				return t
			}
		}
		return typ.NewRef("", name)
	}

	module := r.resolveModuleAliasPath(te.Path[0])
	for i := 1; i < len(te.Path)-1; i++ {
		module += "." + te.Path[i]
	}
	typeName := te.Path[len(te.Path)-1]

	if r.manifests != nil {
		if manifest := io.LookupManifest(r.manifests, module); manifest != nil {
			if t, ok := manifest.LookupType(typeName); ok {
				return t
			}
		}
	}

	return typ.NewRef(module, typeName)
}

func (r *Resolver) resolveModuleAliasPath(name string) string {
	if name == "" || r == nil || r.moduleBindings == nil || len(r.moduleAliases) == 0 {
		return name
	}

	syms := r.moduleBindings.SymbolsByName(name)
	if len(syms) == 0 {
		return name
	}

	resolved := ""
	for _, sym := range syms {
		path := r.moduleAliases[sym]
		if path == "" {
			continue
		}
		if resolved == "" {
			resolved = path
			continue
		}
		if resolved != path {
			return name
		}
	}

	if resolved == "" {
		return name
	}
	return resolved
}

func (r *Resolver) resolveGeneric(te *ast.GenericTypeExpr, sc *scope.State, depth int) typ.Type {
	if te.Base == nil || len(te.Base.Path) == 0 {
		return typ.Unknown
	}

	name := te.Base.Path[0]
	var baseType typ.Type

	if sc != nil {
		if t, ok := sc.LookupType(name); ok {
			baseType = t
		}
	}

	if baseType == nil {
		return typ.Unknown
	}

	generic, ok := baseType.(*typ.Generic)
	if !ok {
		return baseType
	}

	if len(te.Args) != len(generic.TypeParams) {
		return typ.Unknown
	}

	args := make([]typ.Type, 0, len(te.Args))
	for i, arg := range te.Args {
		argType := r.resolveTypeDepth(arg, sc, depth+1)
		args = append(args, argType)

		if generic.TypeParams[i].Constraint != nil {
			if !subtype.IsSubtype(argType, generic.TypeParams[i].Constraint) {
				return typ.Unknown
			}
		}
	}

	return typ.Instantiate(generic, args...)
}

func resolveLiteral(te *ast.LiteralTypeExpr) typ.Type {
	switch v := te.Value.(type) {
	case string:
		return typ.LiteralString(v)
	case float64:
		return typ.LiteralNumber(v)
	case int64:
		return typ.LiteralInt(v)
	case bool:
		return typ.LiteralBool(v)
	default:
		return typ.Unknown
	}
}

func (r *Resolver) resolveTuple(te *ast.TupleTypeExpr, sc *scope.State, depth int) typ.Type {
	elements := make([]typ.Type, 0, len(te.Elements))
	for _, elem := range te.Elements {
		elements = append(elements, r.resolveTypeDepth(elem, sc, depth+1))
	}
	return typ.NewTuple(elements...)
}

func (r *Resolver) resolveTypeOf(te *ast.TypeOfExpr) typ.Type {
	if te.Expr == nil || r.exprSynth == nil {
		return typ.Unknown
	}
	return r.exprSynth(te.Expr, 0)
}

func (r *Resolver) resolveConditional(te *ast.ConditionalTypeExpr, sc *scope.State, depth int) typ.Type {
	check := r.resolveTypeDepth(te.Check, sc, depth+1)
	extends := r.resolveTypeDepth(te.Extends, sc, depth+1)

	thenFn := func() typ.Type { return r.resolveTypeDepth(te.Then, sc, depth+1) }
	elseFn := func() typ.Type { return r.resolveTypeDepth(te.Else, sc, depth+1) }

	return ComputeConditionalType(check, extends, thenFn, elseFn)
}

// ComputeKeyOf extracts record keys as a union of string literals.
func ComputeKeyOf(t typ.Type) typ.Type {
	switch tt := t.(type) {
	case *typ.Record:
		if len(tt.Fields) == 0 {
			return typ.Never
		}
		keys := make([]typ.Type, 0, len(tt.Fields))
		for _, f := range tt.Fields {
			keys = append(keys, typ.LiteralString(f.Name))
		}
		return typ.NewUnion(keys...)

	case *typ.Map:
		return tt.Key

	case *typ.Array:
		return typ.Integer

	case *typ.Tuple:
		if len(tt.Elements) == 0 {
			return typ.Never
		}
		keys := make([]typ.Type, 0, len(tt.Elements))
		for i := range tt.Elements {
			keys = append(keys, typ.LiteralInt(int64(i+1)))
		}
		return typ.NewUnion(keys...)

	default:
		return typ.Never
	}
}

// ComputeIndexAccess extracts the type at a given key.
func ComputeIndexAccess(obj, key typ.Type) typ.Type {
	switch ot := obj.(type) {
	case *typ.Record:
		if lit, ok := key.(*typ.Literal); ok {
			if lit.Base == kind.String {
				name := lit.Value.(string)
				if f := ot.GetField(name); f != nil {
					return f.Type
				}
			}
		}
		return typ.Unknown

	case *typ.Map:
		return ot.Value

	case *typ.Array:
		return ot.Element

	case *typ.Tuple:
		if lit, ok := key.(*typ.Literal); ok {
			switch lit.Base {
			case kind.Integer:
				idx := int(lit.Value.(int64))
				if idx >= 1 && idx <= len(ot.Elements) {
					return ot.Elements[idx-1]
				}
			case kind.Number:
				idx := int(lit.Value.(float64))
				if idx >= 1 && idx <= len(ot.Elements) {
					return ot.Elements[idx-1]
				}
			}
		}
		return typ.Unknown

	default:
		return typ.Unknown
	}
}

// ComputeConditionalType evaluates T extends U ? A : B.
func ComputeConditionalType(check, extends typ.Type, thenFn, elseFn func() typ.Type) typ.Type {
	if u, ok := check.(*typ.Union); ok {
		results := make([]typ.Type, 0, len(u.Members))
		for _, m := range u.Members {
			results = append(results, ComputeConditionalType(m, extends, thenFn, elseFn))
		}
		return typ.NewUnion(results...)
	}

	if subtype.IsSubtype(check, extends) {
		return thenFn()
	}
	return elseFn()
}
