package constraint

import (
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/subst"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// PathTypeResolver resolves a constraint path key to its type at a program point.
type PathTypeResolver func(PathKey) typ.Type

// Env provides external resolvers needed for constraint solving.
//
// The solver itself is pure and deterministic; Env supplies the context-specific
// resolution functions. All fields are optional - the solver degrades gracefully
// when resolvers are not provided.
//
// Example:
//
//	env := constraint.Env{
//	    ResolveType: func(key narrow.TypeKey) typ.Type { ... },
//	    Resolver:    myFieldResolver,
//	}
//	solver := constraint.Solver{Env: env}
type Env struct {
	// ResolveType converts type keys from HasType constraints to actual types.
	// Required for type guard narrowing (e.g., type(x) == "string").
	ResolveType narrow.TypeResolver

	// Resolver provides field and index type lookups on record/array/map types.
	// Required for field-based narrowing (e.g., x.kind == "success").
	Resolver narrow.Resolver

	// PathTypeAt retrieves the current type of a path at a program point.
	// Used for multi-path constraints like FieldEqualsPath.
	PathTypeAt PathTypeResolver

	// ResolvePath converts unversioned paths to versioned PathKeys.
	// Used for SSA-aware path resolution in ApplyToSingle.
	ResolvePath PathResolver
}

// Field resolves a field type using Resolver.
func (e *Env) Field(t typ.Type, name string) (typ.Type, bool) {
	if e.Resolver != nil {
		return e.Resolver.Field(t, name)
	}
	return nil, false
}

// Index resolves an index type using Resolver.
func (e *Env) Index(t typ.Type, key typ.Type) (typ.Type, bool) {
	if e.Resolver != nil {
		return e.Resolver.Index(t, key)
	}
	return nil, false
}

// HasResolver returns true if Env can resolve field and index queries.
func (e *Env) HasResolver() bool {
	return e.Resolver != nil
}

// Solver applies constraints to type environments to produce narrowed types.
//
// The solver is pure and deterministic: given the same constraints and base
// types, it always produces the same narrowed types. This enables memoization
// and incremental re-analysis.
//
// The solver iterates until a fixed point is reached, propagating narrowing
// information across related paths (e.g., EqPath narrows both endpoints).
type Solver struct {
	Env Env
}

// workSkipThreshold is the minimum number of constraints needed to enable
// path-based work skipping. Below this threshold, the overhead of tracking
// changes exceeds the benefit.
const workSkipThreshold = 100

// Apply applies a conjunction of constraints to base types and returns narrowed types.
//
// The solver iterates through constraints, applying each one and propagating
// narrowing information until no further changes occur. Unknown paths in
// constraints are ignored.
//
// For large constraint sets (>100 constraints), the solver uses path-based work
// skipping: after the first iteration, only constraints that reference paths
// which changed in the previous iteration are re-evaluated.
func (s Solver) Apply(constraints []Constraint, base map[PathKey]typ.Type) map[PathKey]typ.Type {
	if len(base) == 0 {
		return nil
	}
	// Copy base as output to avoid mutating input maps.
	out := make(map[PathKey]typ.Type, len(base))
	for k, v := range base {
		out[k] = v
	}

	if len(constraints) == 0 {
		return out
	}

	// Use optimized path for large constraint sets
	if len(constraints) >= workSkipThreshold {
		return s.applyWithWorkSkipping(constraints, out)
	}

	// Simple path for small constraint sets
	maxIters := len(constraints)*2 + len(out) + 1
	for i := 0; i < maxIters; i++ {
		changed := false

		for _, c := range constraints {
			if applyConstraint(&out, s.Env, c) {
				changed = true
			}
		}

		if !changed {
			break
		}
	}

	return out
}

// applyWithWorkSkipping uses path-based tracking to skip unchanged constraints.
func (s Solver) applyWithWorkSkipping(constraints []Constraint, out map[PathKey]typ.Type) map[PathKey]typ.Type {
	// Build path -> constraint index for work skipping.
	pathConstraints := buildPathConstraintIndex(constraints)

	// Track which paths changed in each iteration.
	changedPaths := make(map[PathKey]struct{}, len(out))

	// First iteration: evaluate all constraints
	for _, c := range constraints {
		applyConstraintTrackChanges(&out, s.Env, c, changedPaths)
	}

	if len(changedPaths) == 0 {
		return out
	}

	// Subsequent iterations: only evaluate constraints affected by changed paths
	maxIters := len(constraints)*2 + len(out)
	for iter := 1; iter < maxIters; iter++ {
		// Collect constraints to re-evaluate based on changed paths
		toEvaluate := collectAffectedConstraints(changedPaths, pathConstraints)
		if len(toEvaluate) == 0 {
			break
		}

		// Clear changed paths for this iteration
		for k := range changedPaths {
			delete(changedPaths, k)
		}

		// Evaluate only affected constraints
		for _, c := range toEvaluate {
			applyConstraintTrackChanges(&out, s.Env, c, changedPaths)
		}

		if len(changedPaths) == 0 {
			break
		}
	}

	return out
}

// buildPathConstraintIndex creates a map from path keys to constraints that reference them.
func buildPathConstraintIndex(constraints []Constraint) map[PathKey][]Constraint {
	index := make(map[PathKey][]Constraint)
	for _, c := range constraints {
		VisitPaths(c, func(p Path) bool {
			if p.IsEmpty() {
				return false
			}
			key := p.Key()
			index[key] = append(index[key], c)
			return false
		})
	}
	return index
}

// collectAffectedConstraints returns unique constraints that reference any of the changed paths.
func collectAffectedConstraints(changedPaths map[PathKey]struct{}, pathConstraints map[PathKey][]Constraint) []Constraint {
	seen := make(map[uint64]struct{})
	var result []Constraint

	for _, pathKey := range SortedPathKeys(changedPaths) {
		for _, c := range pathConstraints[pathKey] {
			h := c.Hash()
			if _, ok := seen[h]; !ok {
				seen[h] = struct{}{}
				result = append(result, c)
			}
		}
	}
	return result
}

// applyConstraintTrackChanges applies a constraint and tracks which paths changed.
func applyConstraintTrackChanges(out *map[PathKey]typ.Type, env Env, c Constraint, changedPaths map[PathKey]struct{}) bool {
	// Snapshot types before applying
	var keys [4]PathKey
	var before [4]typ.Type
	count := 0
	var overflow map[PathKey]typ.Type
	addSnapshot := func(key PathKey) {
		if key == "" {
			return
		}
		for i := 0; i < count; i++ {
			if keys[i] == key {
				return
			}
		}
		t, ok := (*out)[key]
		if !ok {
			return
		}
		if count < len(keys) {
			keys[count] = key
			before[count] = t
			count++
			return
		}
		if overflow == nil {
			overflow = make(map[PathKey]typ.Type, 4)
		}
		overflow[key] = t
	}
	VisitPaths(c, func(p Path) bool {
		if p.IsEmpty() {
			return false
		}
		key := p.Key()
		addSnapshot(key)
		return false
	})

	// Apply the constraint
	changed := applyConstraint(out, env, c)

	// Track which paths actually changed
	if changed {
		for i := 0; i < count; i++ {
			key := keys[i]
			oldType := before[i]
			if newType, ok := (*out)[key]; ok {
				if !typeEqual(oldType, newType) {
					changedPaths[key] = struct{}{}
				}
			}
		}
		for key, oldType := range overflow {
			if newType, ok := (*out)[key]; ok {
				if !typeEqual(oldType, newType) {
					changedPaths[key] = struct{}{}
				}
			}
		}
	}

	return changed
}

// PathResolver resolves unversioned constraint paths to versioned PathKeys.
type PathResolver func(Path) PathKey

// ApplyToSingle applies a conjunction of constraints to a single path, returning the narrowed type.
// Only constraints whose resolved path matches target are applied.
func (s Solver) ApplyToSingle(constraints []Constraint, target PathKey, base typ.Type, resolve PathResolver) typ.Type {
	if base == nil || len(constraints) == 0 {
		return base
	}

	result := base
	for _, c := range constraints {
		result = s.applySingleConstraint(c, target, result, resolve)
		if result == nil || result.Kind().IsNever() {
			return typ.Never
		}
	}

	return result
}

// applySingleConstraint applies a single constraint if it matches the target path.
func (s Solver) applySingleConstraint(c Constraint, target PathKey, t typ.Type, resolve PathResolver) typ.Type {
	return VisitConstraint(c, ConstraintVisitor[typ.Type]{
		Truthy: func(v Truthy) typ.Type {
			if resolve(v.Path) == target {
				return narrow.ToTruthy(t)
			}
			// Parent narrowing: if constraint is on target.field, narrow target by field truthy.
			if parent, field, ok := SplitFieldPath(v.Path); ok {
				if resolve(parent) == target && IsBooleanDiscriminantField(t, field, &s.Env) {
					return narrow.ByFieldLiteral(t, field, typ.True, &s.Env)
				}
			}
			return t
		},
		Falsy: func(v Falsy) typ.Type {
			if resolve(v.Path) == target {
				return narrow.ToFalsy(t)
			}
			// Parent narrowing: if constraint is on target.field, narrow target by field falsy.
			if parent, field, ok := SplitFieldPath(v.Path); ok {
				if resolve(parent) == target && IsBooleanDiscriminantField(t, field, &s.Env) {
					return narrow.ByFieldLiteral(t, field, typ.False, &s.Env)
				}
			}
			return t
		},
		IsNil: func(v IsNil) typ.Type {
			if resolve(v.Path) == target {
				return narrow.FilterByKind(t, kind.Nil)
			}
			return t
		},
		NotNil: func(v NotNil) typ.Type {
			if resolve(v.Path) == target {
				return narrow.RemoveNil(t)
			}
			return t
		},
		HasType: func(v HasType) typ.Type {
			if resolve(v.Path) == target {
				return narrow.ByTypeKey(t, v.Type, s.Env.ResolveType)
			}
			if lit, ok := literalFromTypeKey(v.Type, s.Env.ResolveType); ok {
				if parent, field, hasField := SplitFieldPath(v.Path); hasField && resolve(parent) == target {
					return narrow.ByFieldLiteral(t, field, lit, &s.Env)
				}
			}
			return t
		},
		NotHasType: func(v NotHasType) typ.Type {
			if resolve(v.Path) == target {
				return narrow.ExcludeByTypeKey(t, v.Type, s.Env.ResolveType)
			}
			if lit, ok := literalFromTypeKey(v.Type, s.Env.ResolveType); ok {
				if parent, field, hasField := SplitFieldPath(v.Path); hasField && resolve(parent) == target {
					return narrow.ExcludeByFieldLiteral(t, field, lit, &s.Env)
				}
			}
			return t
		},
		HasField: func(v HasField) typ.Type {
			if resolve(v.Path) == target {
				return narrowByHasField(t, v.Field, &s.Env)
			}
			return t
		},
		FieldEquals: func(v FieldEquals) typ.Type {
			if !s.Env.HasResolver() {
				return t
			}
			if resolve(v.Target) == target {
				return narrow.ByFieldLiteral(t, v.Field, v.Value, &s.Env)
			}
			// Ancestor narrowing: walk up the path chain to find matching ancestor.
			if narrowed := s.narrowAncestorByFieldEquals(v.Target, v.Field, v.Value, target, t, resolve); narrowed != nil {
				return narrowed
			}
			return t
		},
		FieldNotEquals: func(v FieldNotEquals) typ.Type {
			if !s.Env.HasResolver() {
				return t
			}
			if resolve(v.Target) == target {
				return narrow.ExcludeByFieldLiteral(t, v.Field, v.Value, &s.Env)
			}
			// Ancestor narrowing: walk up the path chain to find matching ancestor.
			if narrowed := s.excludeAncestorByFieldNotEquals(v.Target, v.Field, v.Value, target, t, resolve); narrowed != nil {
				return narrowed
			}
			return t
		},
		IndexEquals: func(v IndexEquals) typ.Type {
			if !s.Env.HasResolver() {
				return t
			}
			if resolve(v.Target) == target {
				return narrowByIndexLiteral(t, v.Key, v.Value, &s.Env)
			}
			return t
		},
		IndexNotEquals: func(v IndexNotEquals) typ.Type {
			if !s.Env.HasResolver() {
				return t
			}
			if resolve(v.Target) == target {
				return excludeByIndexLiteral(t, v.Key, v.Value, &s.Env)
			}
			return t
		},
		FieldEqualsPath: func(v FieldEqualsPath) typ.Type {
			// result.channel == timeout -> narrow result by field type matching timeout's type.
			if s.Env.PathTypeAt != nil {
				targetKey := resolve(v.Target)
				valueKey := resolve(v.Value)
				if targetKey == target {
					// Narrow target: keep variants where field type matches value's type.
					valueType := s.Env.PathTypeAt(valueKey)
					if valueType != nil {
						return narrowByFieldType(t, v.Field, valueType, &s.Env)
					}
				}
				if valueKey == target {
					// Narrow value: intersect with the field type of target.
					targetType := s.Env.PathTypeAt(targetKey)
					if targetType != nil {
						if ft, ok := s.Env.Field(targetType, v.Field); ok && ft != nil {
							return narrow.Intersect(t, ft)
						}
					}
				}
			}
			return t
		},
		FieldNotEqualsPath: func(v FieldNotEqualsPath) typ.Type {
			// result.channel ~= timeout -> narrow result by excluding field type matching timeout's type.
			if s.Env.PathTypeAt != nil {
				targetKey := resolve(v.Target)
				valueKey := resolve(v.Value)
				if targetKey == target {
					// Narrow target: exclude variants where field type matches value's type.
					valueType := s.Env.PathTypeAt(valueKey)
					if valueType != nil {
						return excludeByFieldType(t, v.Field, valueType, &s.Env)
					}
				}
			}
			return t
		},
		IndexEqualsPath: func(v IndexEqualsPath) typ.Type {
			// result[key] == value -> narrow result by index type matching value's type.
			if s.Env.PathTypeAt != nil && s.Env.HasResolver() {
				targetKey := resolve(v.Target)
				valueKey := resolve(v.Value)
				if targetKey == target {
					// Narrow target: keep variants where index type matches value's type.
					valueType := s.Env.PathTypeAt(valueKey)
					if valueType != nil {
						return narrowByIndexType(t, v.Key, valueType, &s.Env)
					}
				}
				if valueKey == target {
					// Narrow value: intersect with the index type of target.
					targetType := s.Env.PathTypeAt(targetKey)
					if targetType != nil {
						if it, ok := s.Env.Index(targetType, v.Key); ok && it != nil {
							return narrow.Intersect(t, it)
						}
					}
				}
			}
			return t
		},
		IndexNotEqualsPath: func(v IndexNotEqualsPath) typ.Type {
			// result[key] ~= value -> narrow result by excluding index type matching value's type.
			if s.Env.PathTypeAt != nil && s.Env.HasResolver() {
				targetKey := resolve(v.Target)
				valueKey := resolve(v.Value)
				if targetKey == target {
					// Narrow target: exclude variants where index type matches value's type.
					valueType := s.Env.PathTypeAt(valueKey)
					if valueType != nil {
						return excludeByIndexType(t, v.Key, valueType, &s.Env)
					}
				}
			}
			return t
		},
		KeyOf: func(KeyOf) typ.Type {
			// KeyOf is a metadata constraint that does not narrow types directly.
			// It is used by HasKeyOfConstraint to determine if optional should be removed from index access.
			return t
		},
		Default: func(Constraint) typ.Type {
			return t
		},
	})
}

func applyConstraint(out *map[PathKey]typ.Type, env Env, c Constraint) bool {
	return VisitConstraint(c, ConstraintVisitor[bool]{
		Truthy: func(v Truthy) bool {
			changed := applySinglePath(out, v.Path, func(t typ.Type) typ.Type {
				return narrow.ToTruthy(t)
			})
			// Only narrow parent by field literal for boolean discriminants in unions.
			if parent, field, ok := SplitFieldPath(v.Path); ok {
				if parentType, exists := (*out)[parent.Key()]; exists && IsBooleanDiscriminantField(parentType, field, &env) {
					changed = applySinglePath(out, parent, func(t typ.Type) typ.Type {
						return narrow.ByFieldLiteral(t, field, typ.True, &env)
					}) || changed
				}
			}
			return changed
		},
		Falsy: func(v Falsy) bool {
			changed := applySinglePath(out, v.Path, func(t typ.Type) typ.Type {
				return narrow.ToFalsy(t)
			})
			// Only narrow parent by field literal for boolean discriminants in unions.
			if parent, field, ok := SplitFieldPath(v.Path); ok {
				if parentType, exists := (*out)[parent.Key()]; exists && IsBooleanDiscriminantField(parentType, field, &env) {
					changed = applySinglePath(out, parent, func(t typ.Type) typ.Type {
						return narrow.ByFieldLiteral(t, field, typ.False, &env)
					}) || changed
				}
			}
			return changed
		},
		IsNil: func(v IsNil) bool {
			return applySinglePath(out, v.Path, func(t typ.Type) typ.Type {
				return narrow.FilterByKind(t, kind.Nil)
			})
		},
		NotNil: func(v NotNil) bool {
			return applySinglePath(out, v.Path, func(t typ.Type) typ.Type {
				return narrow.RemoveNil(t)
			})
		},
		HasType: func(v HasType) bool {
			changed := applySinglePath(out, v.Path, func(t typ.Type) typ.Type {
				return narrow.ByTypeKey(t, v.Type, env.ResolveType)
			})
			if lit, ok := literalFromTypeKey(v.Type, env.ResolveType); ok {
				if parent, field, hasField := SplitFieldPath(v.Path); hasField {
					changed = applySinglePath(out, parent, func(t typ.Type) typ.Type {
						return narrow.ByFieldLiteral(t, field, lit, &env)
					}) || changed
				}
			}
			return changed
		},
		NotHasType: func(v NotHasType) bool {
			changed := applySinglePath(out, v.Path, func(t typ.Type) typ.Type {
				return narrow.ExcludeByTypeKey(t, v.Type, env.ResolveType)
			})
			if lit, ok := literalFromTypeKey(v.Type, env.ResolveType); ok {
				if parent, field, hasField := SplitFieldPath(v.Path); hasField {
					changed = applySinglePath(out, parent, func(t typ.Type) typ.Type {
						return narrow.ExcludeByFieldLiteral(t, field, lit, &env)
					}) || changed
				}
			}
			return changed
		},
		HasField: func(v HasField) bool {
			return applySinglePath(out, v.Path, func(t typ.Type) typ.Type {
				return narrowByHasField(t, v.Field, &env)
			})
		},
		FieldEquals: func(v FieldEquals) bool {
			if !env.HasResolver() {
				return false
			}
			changed := applySinglePath(out, v.Target, func(t typ.Type) typ.Type {
				return narrow.ByFieldLiteral(t, v.Field, v.Value, &env)
			})
			// Propagate narrowing to parent paths for nested field equality.
			changed = propagateFieldNarrowingToParents(out, v.Target, v.Field, v.Value, env) || changed
			return changed
		},
		FieldNotEquals: func(v FieldNotEquals) bool {
			if !env.HasResolver() {
				return false
			}
			changed := applySinglePath(out, v.Target, func(t typ.Type) typ.Type {
				return narrow.ExcludeByFieldLiteral(t, v.Field, v.Value, &env)
			})
			// Propagate exclusion to parent paths for nested field equality.
			changed = propagateFieldExclusionToParents(out, v.Target, v.Field, v.Value, env) || changed
			return changed
		},
		IndexEquals: func(v IndexEquals) bool {
			if !env.HasResolver() {
				return false
			}
			return applyIndexEquals(out, v.Target, v.Key, v.Value, env)
		},
		IndexNotEquals: func(v IndexNotEquals) bool {
			if !env.HasResolver() {
				return false
			}
			return applyIndexNotEquals(out, v.Target, v.Key, v.Value, env)
		},
		EqPath: func(v EqPath) bool {
			return applyEqPath(out, v.Left, v.Right)
		},
		NotEqPath: func(v NotEqPath) bool {
			return applyNotEqPath(out, v.Left, v.Right)
		},
		FieldEqualsPath: func(v FieldEqualsPath) bool {
			return applyFieldEqualsPath(out, v.Target, v.Field, v.Value, env)
		},
		FieldNotEqualsPath: func(v FieldNotEqualsPath) bool {
			return applyFieldNotEqualsPath(out, v.Target, v.Field, v.Value, env)
		},
		IndexEqualsPath: func(v IndexEqualsPath) bool {
			return applyIndexEqualsPath(out, v.Target, v.Key, v.Value, env)
		},
		IndexNotEqualsPath: func(v IndexNotEqualsPath) bool {
			return applyIndexNotEqualsPath(out, v.Target, v.Key, v.Value, env)
		},
		KeyOf: func(KeyOf) bool {
			// KeyOf does not narrow types - it's a metadata constraint.
			return false
		},
		Default: func(Constraint) bool {
			return false
		},
	})
}

func applySinglePath(out *map[PathKey]typ.Type, path Path, fn func(typ.Type) typ.Type) bool {
	if path.IsEmpty() {
		return false
	}

	key := path.Key()

	current, ok := (*out)[key]
	if !ok || current == nil {
		return false
	}

	next := fn(current)
	if next == nil {
		return false
	}

	if typeEqual(current, next) {
		return false
	}

	(*out)[key] = next

	return true
}

func applyEqPath(out *map[PathKey]typ.Type, left, right Path) bool {
	if left.IsEmpty() || right.IsEmpty() {
		return false
	}

	lk := left.Key()
	rk := right.Key()
	lt, lok := (*out)[lk]
	rt, rok := (*out)[rk]

	if !lok || !rok || lt == nil || rt == nil {
		return false
	}

	intersection := narrow.Intersect(lt, rt)
	if intersection == nil {
		return false
	}

	changed := false

	if !typeEqual(lt, intersection) {
		(*out)[lk] = intersection
		changed = true
	}

	if !typeEqual(rt, intersection) {
		(*out)[rk] = intersection
		changed = true
	}

	return changed
}

func applyNotEqPath(out *map[PathKey]typ.Type, left, right Path) bool {
	if left.IsEmpty() || right.IsEmpty() {
		return false
	}

	lk := left.Key()
	rk := right.Key()
	lt, lok := (*out)[lk]
	rt, rok := (*out)[rk]

	if !lok || !rok || lt == nil || rt == nil {
		return false
	}

	changed := false

	// Value inequality (x ~= y) only implies type exclusion when the excluded type
	// is a singleton (nil or literal). For structural types like records, x ~= y
	// just means different values of the same type.
	if unwrap.IsSingleton(rt) {
		narrowedLeft := excludeSingletonSubtypes(lt, rt)
		if narrowedLeft != nil && !typeEqual(lt, narrowedLeft) {
			(*out)[lk] = narrowedLeft
			changed = true
		}
	}

	if unwrap.IsSingleton(lt) {
		narrowedRight := excludeSingletonSubtypes(rt, lt)
		if narrowedRight != nil && !typeEqual(rt, narrowedRight) {
			(*out)[rk] = narrowedRight
			changed = true
		}
	}

	return changed
}

// excludeSingletonSubtypes filters a type to remove singleton members that are
// subtypes of the excluded type. Unlike excludeSubtypes, this only narrows when
// dealing with singletons, since value inequality for non-singletons doesn't
// imply type exclusion.
func excludeSingletonSubtypes(t typ.Type, excluded typ.Type) typ.Type {
	if t == nil || excluded == nil {
		return t
	}

	// Only narrow if excluded is a singleton
	if !unwrap.IsSingleton(excluded) {
		return t
	}

	// Handle Optional (T | nil) specially
	if opt, ok := t.(*typ.Optional); ok {
		// If excluding nil, return the inner type
		if excluded.Kind() == kind.Nil {
			return opt.Inner
		}
		// If excluding the inner type (when inner is singleton), return nil
		if unwrap.IsSingleton(opt.Inner) && subtype.IsSubtype(opt.Inner, excluded) {
			return typ.Nil
		}
		return t
	}

	u, ok := t.(*typ.Union)
	if !ok {
		// Single type: if it's a singleton subtype of excluded, return never
		if unwrap.IsSingleton(t) && subtype.IsSubtype(t, excluded) {
			return typ.Never
		}
		return t
	}

	var kept []typ.Type
	for _, m := range u.Members {
		// Only exclude singleton members
		if unwrap.IsSingleton(m) && subtype.IsSubtype(m, excluded) {
			continue
		}
		kept = append(kept, m)
	}

	if len(kept) == 0 {
		return typ.Never
	}

	return typ.NewUnion(kept...)
}

// isExcludableValueType returns true if a value type is precise enough
// to justify exclusion from an inequality constraint.
// This allows identity-like composite types (records/interfaces) and singletons,
// but blocks wide scalar types like string/number/boolean/any/unknown.
func isExcludableValueType(t typ.Type) bool {
	if t == nil {
		return false
	}
	if unwrap.IsSingleton(t) {
		return true
	}
	return typ.Visit(t, typ.Visitor[bool]{
		Alias: func(a *typ.Alias) bool {
			return isExcludableValueType(a.Target)
		},
		Optional: func(*typ.Optional) bool {
			return false
		},
		Union: func(*typ.Union) bool {
			return false
		},
		Intersection: func(*typ.Intersection) bool {
			return false
		},
		Instantiated: func(i *typ.Instantiated) bool {
			if expanded := subst.ExpandInstantiated(i); expanded != nil {
				return isExcludableValueType(expanded)
			}
			return false
		},
		Default: func(t typ.Type) bool {
			k := t.Kind()
			if k.IsPlaceholder() {
				return false
			}
			switch k {
			case kind.Record, kind.Interface, kind.Map, kind.Array, kind.Tuple, kind.Function:
				return true
			case kind.String, kind.Number, kind.Boolean, kind.Integer:
				return false
			default:
				return false
			}
		},
	})
}

func isSingletonValueType(t typ.Type) bool {
	t = unwrap.Alias(t)
	if t == nil {
		return false
	}
	if t.Kind() == kind.Nil {
		return true
	}
	_, isLiteral := t.(*typ.Literal)
	return isLiteral
}

func applyFieldEqualsPath(out *map[PathKey]typ.Type, target Path, field string, value Path, env Env) bool {
	if target.IsEmpty() || value.IsEmpty() || field == "" {
		return false
	}

	if !env.HasResolver() {
		return false
	}

	tk := target.Key()
	vk := value.Key()
	targetType, tok := (*out)[tk]
	valueType, vok := (*out)[vk]

	if !tok || !vok || targetType == nil || valueType == nil {
		return false
	}

	changed := false

	narrowedTarget := narrowByFieldType(targetType, field, valueType, &env)
	if narrowedTarget != nil && !typeEqual(targetType, narrowedTarget) {
		(*out)[tk] = narrowedTarget
		targetType = narrowedTarget
		changed = true
	}

	if ft, ok := env.Field(targetType, field); ok && ft != nil {
		narrowedValue := narrow.Intersect(valueType, ft)
		if narrowedValue != nil && !typeEqual(valueType, narrowedValue) {
			(*out)[vk] = narrowedValue
			changed = true
		}
	}

	// Propagate to parent path for nested field narrowing
	changed = propagateFieldEqualsPathToParents(out, target, field, valueType, env) || changed

	return changed
}

func applyFieldNotEqualsPath(out *map[PathKey]typ.Type, target Path, field string, value Path, env Env) bool {
	if target.IsEmpty() || value.IsEmpty() || field == "" {
		return false
	}

	if !env.HasResolver() {
		return false
	}

	tk := target.Key()
	vk := value.Key()
	targetType, tok := (*out)[tk]
	valueType, vok := (*out)[vk]

	if !tok || !vok || targetType == nil || valueType == nil {
		return false
	}

	changed := false

	narrowedTarget := excludeByFieldType(targetType, field, valueType, &env)
	if narrowedTarget != nil && !typeEqual(targetType, narrowedTarget) {
		(*out)[tk] = narrowedTarget
		changed = true
	}

	// Propagate to parent path for nested field exclusion
	changed = propagateFieldNotEqualsPathToParents(out, target, field, valueType, env) || changed

	return changed
}

func applyIndexEquals(out *map[PathKey]typ.Type, target Path, key typ.Type, lit *typ.Literal, env Env) bool {
	if target.IsEmpty() || key == nil || lit == nil {
		return false
	}

	if !env.HasResolver() {
		return false
	}

	tk := target.Key()

	targetType, ok := (*out)[tk]
	if !ok || targetType == nil {
		return false
	}

	narrowed := narrowByIndexLiteral(targetType, key, lit, &env)
	if narrowed == nil || typeEqual(targetType, narrowed) {
		return false
	}

	(*out)[tk] = narrowed

	return true
}

func applyIndexEqualsPath(out *map[PathKey]typ.Type, target Path, key typ.Type, value Path, env Env) bool {
	if target.IsEmpty() || value.IsEmpty() || key == nil {
		return false
	}

	if !env.HasResolver() {
		return false
	}

	tk := target.Key()
	vk := value.Key()
	targetType, tok := (*out)[tk]
	valueType, vok := (*out)[vk]

	if !tok || !vok || targetType == nil || valueType == nil {
		return false
	}

	changed := false

	narrowedTarget := narrowByIndexType(targetType, key, valueType, &env)
	if narrowedTarget != nil && !typeEqual(targetType, narrowedTarget) {
		(*out)[tk] = narrowedTarget
		targetType = narrowedTarget
		changed = true
	}

	if it, ok := env.Index(targetType, key); ok && it != nil {
		narrowedValue := narrow.Intersect(valueType, it)
		if narrowedValue != nil && !typeEqual(valueType, narrowedValue) {
			(*out)[vk] = narrowedValue
			changed = true
		}
	}

	return changed
}

func applyIndexNotEquals(out *map[PathKey]typ.Type, target Path, key typ.Type, lit *typ.Literal, env Env) bool {
	if target.IsEmpty() || key == nil || lit == nil {
		return false
	}

	if !env.HasResolver() {
		return false
	}

	tk := target.Key()

	targetType, ok := (*out)[tk]
	if !ok || targetType == nil {
		return false
	}

	narrowed := excludeByIndexLiteral(targetType, key, lit, &env)
	if narrowed == nil || typeEqual(targetType, narrowed) {
		return false
	}

	(*out)[tk] = narrowed

	return true
}

func applyIndexNotEqualsPath(out *map[PathKey]typ.Type, target Path, key typ.Type, value Path, env Env) bool {
	if target.IsEmpty() || value.IsEmpty() || key == nil {
		return false
	}

	if !env.HasResolver() {
		return false
	}

	tk := target.Key()
	vk := value.Key()
	targetType, tok := (*out)[tk]
	valueType, vok := (*out)[vk]

	if !tok || !vok || targetType == nil || valueType == nil {
		return false
	}

	changed := false

	narrowedTarget := excludeByIndexType(targetType, key, valueType, &env)
	if narrowedTarget != nil && !typeEqual(targetType, narrowedTarget) {
		(*out)[tk] = narrowedTarget
		changed = true
	}

	return changed
}

func typeEqual(a, b typ.Type) bool {
	if a == b {
		return true
	}

	if a == nil || b == nil {
		return false
	}

	// Use structural equality for change detection in the solver.
	// Bidirectional subtype checking would incorrectly treat
	// {kind: "a"} | {kind: string} as equivalent to {kind: string},
	// preventing exclusion constraints from being applied.
	return typ.TypeEquals(a, b)
}

// narrowByHasField narrows a type to members that have the specified field.
// For unions, filters to members that have the field.
// For intersections, narrows each member.
// Non-union/intersection types are returned unchanged if they have the field, nil otherwise.
func narrowByHasField(t typ.Type, field string, resolver narrow.Resolver) typ.Type {
	if t == nil || field == "" {
		return t
	}

	// Narrow through aliases so HasField can refine aliased unions.
	if alias, ok := t.(*typ.Alias); ok {
		narrowed := narrowByHasField(alias.Target, field, resolver)
		if narrowed == nil || narrowed.Kind().IsNever() {
			return typ.Never
		}
		return narrowed
	}

	unwrapped := unwrap.Alias(t)

	// For intersections, narrow each member
	if inter, ok := unwrapped.(*typ.Intersection); ok {
		var narrowed []typ.Type
		for _, m := range inter.Members {
			nm := narrowByHasField(m, field, resolver)
			if nm == nil {
				return typ.Never
			}
			narrowed = append(narrowed, nm)
		}
		return typ.NewIntersection(narrowed...)
	}

	// For unions, filter to members that have the field
	if u, ok := unwrapped.(*typ.Union); ok {
		var members []typ.Type
		for _, m := range u.Members {
			nm := narrowByHasField(m, field, resolver)
			if nm != nil && !nm.Kind().IsNever() {
				members = append(members, nm)
			}
		}
		if len(members) == 0 {
			return typ.Never
		}
		if len(members) == 1 {
			return members[0]
		}
		return typ.NewUnion(members...)
	}

	// For non-unions, check if the field exists
	if hasField(t, field, resolver) {
		return t
	}
	return nil
}

// hasField checks if a type has a specific field.
func hasField(t typ.Type, field string, resolver narrow.Resolver) bool {
	if t == nil {
		return false
	}

	// any/unknown have all fields
	if t.Kind().IsPlaceholder() {
		return true
	}

	// Use resolver if available
	if resolver != nil {
		_, ok := resolver.Field(t, field)
		return ok
	}

	// Fallback: check records directly
	if rec, ok := t.(*typ.Record); ok {
		return rec.GetField(field) != nil
	}
	if iface, ok := t.(*typ.Interface); ok {
		for _, m := range iface.Methods {
			if m.Name == field {
				return true
			}
		}
	}

	return false
}

func narrowByIndexLiteral(t typ.Type, key typ.Type, lit *typ.Literal, resolver narrow.Resolver) typ.Type {
	if t == nil || key == nil || lit == nil || resolver == nil {
		return t
	}

	return narrow.FilterByMatch(t, func(m typ.Type) bool {
		return indexMatchesLiteral(m, key, lit, resolver)
	}, false)
}

func excludeByIndexLiteral(t typ.Type, key typ.Type, lit *typ.Literal, resolver narrow.Resolver) typ.Type {
	if t == nil || key == nil || lit == nil || resolver == nil {
		return t
	}

	// For exclusion (index ~= value), only exclude types where
	// the index IS exactly the literal, not types where index MIGHT contain it.
	return narrow.FilterByMatch(t, func(m typ.Type) bool {
		return indexIsExactlyLiteral(m, key, lit, resolver)
	}, true)
}

func narrowByFieldType(t typ.Type, field string, other typ.Type, resolver narrow.Resolver) typ.Type {
	if t == nil || other == nil || field == "" || resolver == nil {
		return t
	}

	// Prefer exact type-instance matches when the target is a union.
	// This allows narrowing by nominal identity even when structures overlap.
	if u := unwrap.Union(t); u != nil {
		if narrowed := narrowUnionByFieldInstance(u, field, other, resolver); narrowed != nil {
			return narrowed
		}
	}

	return narrow.FilterByMatch(t, func(m typ.Type) bool {
		return fieldMatchesType(m, field, other, resolver)
	}, false)
}

func excludeByFieldType(t typ.Type, field string, other typ.Type, resolver narrow.Resolver) typ.Type {
	if t == nil || other == nil || field == "" || resolver == nil {
		return t
	}
	if !isExcludableValueType(other) {
		return t
	}

	// Path inequality (`x.field ~= y`) may exclude union members only when the
	// compared field type identifies a unique member.
	if u := unwrap.Union(t); u != nil {
		if narrowed, ok := excludeUnionByFieldInstance(u, field, other, resolver); ok {
			return narrowed
		}
		if narrowed, ok := excludeUnionByFieldEquivalent(u, field, other, resolver); ok {
			return narrowed
		}
		return t
	}

	// Non-union targets can still represent many runtime identities.
	// For path inequality (x.field ~= y), excluding by type equivalence is only
	// sound when y is a singleton value (literal/nil).
	if !isSingletonValueType(other) {
		return t
	}

	return narrow.FilterByMatch(t, func(m typ.Type) bool {
		return fieldMatchesTypeExact(m, field, other, resolver)
	}, true)
}

func narrowByIndexType(t typ.Type, key typ.Type, other typ.Type, resolver narrow.Resolver) typ.Type {
	if t == nil || key == nil || other == nil || resolver == nil {
		return t
	}

	return narrow.FilterByMatch(t, func(m typ.Type) bool {
		return indexMatchesType(m, key, other, resolver)
	}, false)
}

func excludeByIndexType(t typ.Type, key typ.Type, other typ.Type, resolver narrow.Resolver) typ.Type {
	if t == nil || key == nil || other == nil || resolver == nil {
		return t
	}
	if !isExcludableValueType(other) {
		return t
	}

	return narrow.FilterByMatch(t, func(m typ.Type) bool {
		return indexMatchesTypeExact(m, key, other, resolver)
	}, true)
}

func fieldMatchesType(t typ.Type, field string, other typ.Type, resolver narrow.Resolver) bool {
	if resolver == nil || t == nil || other == nil {
		return false
	}

	fieldType, ok := resolver.Field(t, field)
	if !ok || fieldType == nil {
		return false
	}

	return narrow.TypesOverlap(fieldType, other)
}

func indexMatchesType(t typ.Type, key typ.Type, other typ.Type, resolver narrow.Resolver) bool {
	if resolver == nil || t == nil || key == nil || other == nil {
		return false
	}

	indexType, ok := resolver.Index(t, key)
	if !ok || indexType == nil {
		return false
	}

	return narrow.TypesOverlap(indexType, other)
}

// fieldMatchesTypeExact checks if a record's field type is exactly equivalent to another type.
// Used for exclusion: only exclude variants where field type definitely equals the comparison type.
func fieldMatchesTypeExact(t typ.Type, field string, other typ.Type, resolver narrow.Resolver) bool {
	if resolver == nil || t == nil || other == nil {
		return false
	}

	fieldType, ok := resolver.Field(t, field)
	if !ok || fieldType == nil {
		return false
	}

	return typesEquivalent(fieldType, other)
}

func narrowUnionByFieldInstance(u *typ.Union, field string, other typ.Type, resolver narrow.Resolver) typ.Type {
	if u == nil || other == nil || field == "" || resolver == nil {
		return nil
	}
	var matches []typ.Type
	for _, m := range u.Members {
		ft, ok := resolver.Field(m, field)
		if !ok || ft == nil {
			continue
		}
		if sameTypeInstance(ft, other) {
			matches = append(matches, m)
		}
	}
	if len(matches) == 0 {
		return nil
	}
	if len(matches) == 1 {
		return matches[0]
	}
	return typ.NewUnion(matches...)
}

func excludeUnionByFieldInstance(u *typ.Union, field string, other typ.Type, resolver narrow.Resolver) (typ.Type, bool) {
	if u == nil || other == nil || field == "" || resolver == nil {
		return nil, false
	}
	var keep []typ.Type
	matchCount := 0
	for _, m := range u.Members {
		ft, ok := resolver.Field(m, field)
		if !ok || ft == nil {
			keep = append(keep, m)
			continue
		}
		if sameTypeInstance(ft, other) {
			matchCount++
			continue
		}
		keep = append(keep, m)
	}
	if matchCount == 0 {
		return nil, false
	}
	// Field/path equality is identity-based at runtime. If multiple union variants
	// share the same field type instance, instance matching cannot identify which
	// variant equals the compared path, so exclusion would be unsound.
	if matchCount > 1 {
		return nil, false
	}
	if len(keep) == 0 {
		return typ.Never, true
	}
	if len(keep) == 1 {
		return keep[0], true
	}
	return typ.NewUnion(keep...), true
}

func excludeUnionByFieldEquivalent(u *typ.Union, field string, other typ.Type, resolver narrow.Resolver) (typ.Type, bool) {
	if u == nil || other == nil || field == "" || resolver == nil {
		return nil, false
	}
	matchCount := 0
	keep := make([]typ.Type, 0, len(u.Members))
	for _, m := range u.Members {
		ft, ok := resolver.Field(m, field)
		if !ok || ft == nil {
			keep = append(keep, m)
			continue
		}
		if typesEquivalent(ft, other) {
			matchCount++
			continue
		}
		keep = append(keep, m)
	}
	// Structural equivalence is only safe for exclusion when it identifies a
	// single union member. Multiple matches are ambiguous for path identity.
	if matchCount != 1 {
		return nil, false
	}
	if len(keep) == 0 {
		return typ.Never, true
	}
	if len(keep) == 1 {
		return keep[0], true
	}
	return typ.NewUnion(keep...), true
}

func sameTypeInstance(a, b typ.Type) bool {
	if a == nil || b == nil {
		return false
	}
	if a == b {
		return true
	}
	if aa, ok := a.(*typ.Alias); ok {
		return sameTypeInstance(aa.Target, b)
	}
	if bb, ok := b.(*typ.Alias); ok {
		return sameTypeInstance(a, bb.Target)
	}
	if oa, ok := a.(*typ.Optional); ok {
		return sameTypeInstance(oa.Inner, b)
	}
	if ob, ok := b.(*typ.Optional); ok {
		return sameTypeInstance(a, ob.Inner)
	}
	return false
}

// indexMatchesTypeExact checks if an indexed type is exactly equivalent to another type.
// Used for exclusion: only exclude variants where index type definitely equals the comparison type.
func indexMatchesTypeExact(t typ.Type, key typ.Type, other typ.Type, resolver narrow.Resolver) bool {
	if resolver == nil || t == nil || key == nil || other == nil {
		return false
	}

	indexType, ok := resolver.Index(t, key)
	if !ok || indexType == nil {
		return false
	}

	return typesEquivalent(indexType, other)
}

// typesEquivalent checks if two types are equivalent for field/index matching.
// For named interfaces, requires pointer identity (not structural equivalence).
// For instantiated generics, uses bidirectional subtype to handle
// structurally identical types that may have different hashes.
func typesEquivalent(a, b typ.Type) bool {
	if a == nil || b == nil {
		return false
	}

	// Named interfaces require pointer identity, not structural equivalence.
	// Two interfaces with the same name/structure from different declarations
	// are not equivalent. Check this first to avoid hash collision issues.
	_, aIsInterface := a.(*typ.Interface)
	_, bIsInterface := b.(*typ.Interface)
	if aIsInterface || bIsInterface {
		return a == b
	}

	// Structural equality check via Equals.
	if a.Equals(b) {
		return true
	}

	// For instantiated generics and structural types, use bidirectional subtype.
	// This handles cases where the same generic is instantiated with equivalent
	// type arguments but has different internal hashes.
	return subtype.IsSubtype(a, b) && subtype.IsSubtype(b, a)
}

// indexIsExactlyLiteral returns true only if the index type IS exactly the literal.
func indexIsExactlyLiteral(t typ.Type, key typ.Type, lit *typ.Literal, resolver narrow.Resolver) bool {
	if resolver == nil || t == nil || key == nil || lit == nil {
		return false
	}

	indexType, ok := resolver.Index(t, key)
	if !ok || indexType == nil {
		return false
	}

	return narrow.TypeIsExactlyLiteral(indexType, lit)
}

func indexMatchesLiteral(t typ.Type, key typ.Type, lit *typ.Literal, resolver narrow.Resolver) bool {
	if resolver == nil || t == nil || key == nil || lit == nil {
		return false
	}

	indexType, ok := resolver.Index(t, key)
	if !ok || indexType == nil {
		return false
	}

	return typ.TypeMatchesLiteral(indexType, lit)
}

// IsBooleanDiscriminantField checks if a field is a boolean literal discriminant
// in a union type. Parent narrowing for Truthy/Falsy should only apply when
// the field type is a boolean literal (true/false) that distinguishes union variants.
func IsBooleanDiscriminantField(parentType typ.Type, field string, resolver narrow.Resolver) bool {
	if parentType == nil || resolver == nil {
		return false
	}

	// Only consider unions for discriminant narrowing
	u := unwrap.Union(parentType)
	if u == nil {
		return false
	}

	// Check if the field is a boolean literal in any union member
	hasBoolLiteral := false
	for _, m := range u.Members {
		ft, ok := resolver.Field(m, field)
		if !ok || ft == nil {
			continue
		}
		// Check if field type is a boolean literal (true or false)
		if lit, ok := ft.(*typ.Literal); ok && lit.Base == kind.Boolean {
			hasBoolLiteral = true
			break
		}
	}

	return hasBoolLiteral
}

func literalFromTypeKey(key narrow.TypeKey, resolve narrow.TypeResolver) (*typ.Literal, bool) {
	if resolve == nil || key.IsZero() {
		return nil, false
	}
	resolved := resolve(key)
	if resolved == nil {
		return nil, false
	}
	lit, ok := unwrap.Alias(resolved).(*typ.Literal)
	if !ok || lit == nil {
		return nil, false
	}
	return lit, true
}

// SplitFieldPath splits a path into its parent path and field name.
// Returns false if the path has no field segments.
// The returned parent path owns its own segment slice (safe to mutate).
func SplitFieldPath(path Path) (parent Path, field string, ok bool) {
	if path.IsEmpty() || len(path.Segments) == 0 {
		return Path{}, "", false
	}
	last := path.Segments[len(path.Segments)-1]
	if last.Kind != SegmentField {
		return Path{}, "", false
	}
	parent = Path{Root: path.Root, Symbol: path.Symbol, Version: path.Version}
	if len(path.Segments) > 1 {
		parent.Segments = append(parent.Segments, path.Segments[:len(path.Segments)-1]...)
	}
	return parent, last.Name, true
}

// propagateFieldNarrowingToParents propagates field narrowing up the path chain.
// For r.a.b == "x", this narrows r based on a.b == "x".
func propagateFieldNarrowingToParents(out *map[PathKey]typ.Type, target Path, field string, value *typ.Literal, env Env) bool {
	if !env.HasResolver() {
		return false
	}

	parent, parentField, ok := SplitFieldPath(target)
	if !ok {
		return false
	}

	pk := parent.Key()
	parentType, pok := (*out)[pk]
	if !pok || parentType == nil {
		return false
	}

	// Narrow parent by nested field path: parentField.field == value
	narrowed := narrowByNestedFieldLiteral(parentType, parentField, field, value, &env)
	if narrowed == nil || typeEqual(parentType, narrowed) {
		return false
	}

	(*out)[pk] = narrowed
	return true
}

// propagateFieldExclusionToParents propagates field exclusion up the path chain.
// For r.a.b != "x", this narrows r to exclude members where a.b == "x".
func propagateFieldExclusionToParents(out *map[PathKey]typ.Type, target Path, field string, value *typ.Literal, env Env) bool {
	if !env.HasResolver() {
		return false
	}

	parent, parentField, ok := SplitFieldPath(target)
	if !ok {
		return false
	}

	pk := parent.Key()
	parentType, pok := (*out)[pk]
	if !pok || parentType == nil {
		return false
	}

	// Exclude from parent by nested field path: parentField.field == value
	narrowed := excludeByNestedFieldLiteral(parentType, parentField, field, value, &env)
	if narrowed == nil || typeEqual(parentType, narrowed) {
		return false
	}

	(*out)[pk] = narrowed
	return true
}

// narrowByNestedFieldLiteral narrows a union by keeping members where field1.field2 matches the literal.
func narrowByNestedFieldLiteral(t typ.Type, field1, field2 string, lit *typ.Literal, resolver narrow.Resolver) typ.Type {
	if t == nil || lit == nil || resolver == nil || field1 == "" || field2 == "" {
		return t
	}

	return narrow.FilterByMatch(t, func(m typ.Type) bool {
		return nestedFieldMatchesLiteral(m, field1, field2, lit, resolver)
	}, false)
}

// excludeByNestedFieldLiteral narrows a union by excluding members where field1.field2 is exactly the literal.
func excludeByNestedFieldLiteral(t typ.Type, field1, field2 string, lit *typ.Literal, resolver narrow.Resolver) typ.Type {
	if t == nil || lit == nil || resolver == nil || field1 == "" || field2 == "" {
		return t
	}

	return narrow.FilterByMatch(t, func(m typ.Type) bool {
		return nestedFieldIsExactlyLiteral(m, field1, field2, lit, resolver)
	}, true)
}

// nestedFieldIsExactlyLiteral checks if t.field1.field2 IS exactly the literal.
// Used for exclusion where only exact matches should be excluded.
func nestedFieldIsExactlyLiteral(t typ.Type, field1, field2 string, lit *typ.Literal, resolver narrow.Resolver) bool {
	if resolver == nil || t == nil || lit == nil {
		return false
	}

	f1Type, ok := resolver.Field(t, field1)
	if !ok || f1Type == nil {
		return false
	}

	return narrow.FieldIsExactlyLiteral(f1Type, field2, lit, resolver)
}

// nestedFieldMatchesLiteral checks if t.field1.field2 can match the literal.
func nestedFieldMatchesLiteral(t typ.Type, field1, field2 string, lit *typ.Literal, resolver narrow.Resolver) bool {
	if resolver == nil || t == nil || lit == nil {
		return false
	}

	// Get the type of field1
	f1Type, ok := resolver.Field(t, field1)
	if !ok || f1Type == nil {
		return false
	}

	// Check if field1's type has field2 that matches the literal
	return narrow.FieldMatchesLiteral(f1Type, field2, lit, resolver)
}

// propagateFieldEqualsPathToParents propagates field-path narrowing up the path chain.
// For r.a.channel == ch, this narrows r based on a.channel matching ch's type.
func propagateFieldEqualsPathToParents(out *map[PathKey]typ.Type, target Path, field string, valueType typ.Type, env Env) bool {
	if !env.HasResolver() {
		return false
	}

	parent, parentField, ok := SplitFieldPath(target)
	if !ok {
		return false
	}

	pk := parent.Key()
	parentType, pok := (*out)[pk]
	if !pok || parentType == nil {
		return false
	}

	narrowed := narrowByNestedFieldType(parentType, parentField, field, valueType, &env)
	if narrowed == nil || typeEqual(parentType, narrowed) {
		return false
	}

	(*out)[pk] = narrowed
	return true
}

// propagateFieldNotEqualsPathToParents propagates field-path exclusion up the path chain.
func propagateFieldNotEqualsPathToParents(out *map[PathKey]typ.Type, target Path, field string, valueType typ.Type, env Env) bool {
	if !env.HasResolver() {
		return false
	}

	parent, parentField, ok := SplitFieldPath(target)
	if !ok {
		return false
	}

	pk := parent.Key()
	parentType, pok := (*out)[pk]
	if !pok || parentType == nil {
		return false
	}

	narrowed := excludeByNestedFieldType(parentType, parentField, field, valueType, &env)
	if narrowed == nil || typeEqual(parentType, narrowed) {
		return false
	}

	(*out)[pk] = narrowed
	return true
}

// narrowByNestedFieldType narrows a union by keeping members where field1.field2 overlaps with valueType.
func narrowByNestedFieldType(t typ.Type, field1, field2 string, valueType typ.Type, resolver narrow.Resolver) typ.Type {
	if t == nil || valueType == nil || resolver == nil || field1 == "" || field2 == "" {
		return t
	}

	return narrow.FilterByMatch(t, func(m typ.Type) bool {
		return nestedFieldMatchesType(m, field1, field2, valueType, resolver)
	}, false)
}

// excludeByNestedFieldType narrows a union by excluding members where field1.field2 overlaps with valueType.
func excludeByNestedFieldType(t typ.Type, field1, field2 string, valueType typ.Type, resolver narrow.Resolver) typ.Type {
	if t == nil || valueType == nil || resolver == nil || field1 == "" || field2 == "" {
		return t
	}
	if !isExcludableValueType(valueType) {
		return t
	}

	return narrow.FilterByMatch(t, func(m typ.Type) bool {
		return nestedFieldMatchesType(m, field1, field2, valueType, resolver)
	}, true)
}

// nestedFieldMatchesType checks if t.field1.field2 overlaps with valueType.
func nestedFieldMatchesType(t typ.Type, field1, field2 string, valueType typ.Type, resolver narrow.Resolver) bool {
	if resolver == nil || t == nil || valueType == nil {
		return false
	}

	f1Type, ok := resolver.Field(t, field1)
	if !ok || f1Type == nil {
		return false
	}

	return fieldMatchesType(f1Type, field2, valueType, resolver)
}

// narrowAncestorByFieldEquals walks up the path chain to check if the target is an ancestor
// of the constraint path. If so, narrows the ancestor type by the field constraint.
func (s Solver) narrowAncestorByFieldEquals(constraintPath Path, field string, value *typ.Literal, target PathKey, t typ.Type, resolve PathResolver) typ.Type {
	if !s.Env.HasResolver() {
		return nil
	}

	// Walk up the path: if target is x and constraintPath is x.a.b.c,
	// check if x.a.b.c is a child of x, then narrow x by filtering
	// to variants where a.b.c.field == value
	if len(constraintPath.Segments) == 0 {
		return nil
	}

	// Check if constraintPath is a descendant of target
	ancestor := Path{Root: constraintPath.Root, Symbol: constraintPath.Symbol}
	if resolve(ancestor) != target {
		return nil
	}

	// Build the segment chain from target to the field
	segments := make([]string, len(constraintPath.Segments)+1)
	for i, seg := range constraintPath.Segments {
		if seg.Kind != SegmentField {
			return nil
		}
		segments[i] = seg.Name
	}
	segments[len(constraintPath.Segments)] = field

	// Filter union variants where the nested path matches the literal
	return filterByDeepNestedFieldLiteral(t, segments, value, &s.Env)
}

// excludeAncestorByFieldNotEquals walks up the path chain to check if the target is an ancestor
// of the constraint path. If so, excludes variants where the field matches.
func (s Solver) excludeAncestorByFieldNotEquals(constraintPath Path, field string, value *typ.Literal, target PathKey, t typ.Type, resolve PathResolver) typ.Type {
	if !s.Env.HasResolver() {
		return nil
	}

	if len(constraintPath.Segments) == 0 {
		return nil
	}

	ancestor := Path{Root: constraintPath.Root, Symbol: constraintPath.Symbol}
	if resolve(ancestor) != target {
		return nil
	}

	segments := make([]string, len(constraintPath.Segments)+1)
	for i, seg := range constraintPath.Segments {
		if seg.Kind != SegmentField {
			return nil
		}
		segments[i] = seg.Name
	}
	segments[len(constraintPath.Segments)] = field

	return excludeByDeepNestedFieldLiteral(t, segments, value, &s.Env)
}

// filterByDeepNestedFieldLiteral filters a type to variants where the nested field path equals the literal.
func filterByDeepNestedFieldLiteral(t typ.Type, segments []string, lit *typ.Literal, resolver narrow.Resolver) typ.Type {
	if t == nil || len(segments) == 0 || lit == nil || resolver == nil {
		return t
	}

	return narrow.FilterByMatch(t, func(m typ.Type) bool {
		return nestedPathMatchesLiteral(m, segments, lit, resolver)
	}, false)
}

// excludeByDeepNestedFieldLiteral excludes variants where the nested field path exactly equals the literal.
func excludeByDeepNestedFieldLiteral(t typ.Type, segments []string, lit *typ.Literal, resolver narrow.Resolver) typ.Type {
	if t == nil || len(segments) == 0 || lit == nil || resolver == nil {
		return t
	}

	return narrow.FilterByMatch(t, func(m typ.Type) bool {
		return nestedPathIsExactlyLiteral(m, segments, lit, resolver)
	}, true)
}

// nestedPathMatchesLiteral checks if traversing the segment path through t leads to a type that overlaps with lit.
func nestedPathMatchesLiteral(t typ.Type, segments []string, lit *typ.Literal, resolver narrow.Resolver) bool {
	current := t
	for _, seg := range segments {
		next, ok := resolver.Field(current, seg)
		if !ok || next == nil {
			return false
		}
		current = next
	}
	return narrow.TypesOverlap(current, lit)
}

// nestedPathIsExactlyLiteral checks if traversing the segment path through t leads to exactly the literal.
func nestedPathIsExactlyLiteral(t typ.Type, segments []string, lit *typ.Literal, resolver narrow.Resolver) bool {
	current := t
	for _, seg := range segments {
		next, ok := resolver.Field(current, seg)
		if !ok || next == nil {
			return false
		}
		current = next
	}
	return typ.TypeEquals(current, lit)
}

// HasKeyOfConstraint checks if a KeyOf constraint is guaranteed active in the condition.
// In DNF, a constraint is guaranteed only if it appears in ALL disjuncts.
func HasKeyOfConstraint(cond Condition, tablePath, keyPath Path, resolve PathResolver) bool {
	if !cond.HasConstraints() {
		return false
	}
	if cond.IsFalse() {
		return false
	}

	numDisjuncts := cond.NumDisjuncts()
	if numDisjuncts == 0 {
		return false
	}

	for i := 0; i < numDisjuncts; i++ {
		disjunct := cond.DisjunctConstraints(i)
		found := false
		for _, c := range disjunct {
			if ko, ok := c.(KeyOf); ok {
				tableMatch := ko.Table.Equal(tablePath)
				keyMatch := ko.Key.Equal(keyPath)
				if !tableMatch && resolve != nil {
					koTableKey := resolve(ko.Table)
					tablePathKey := resolve(tablePath)
					// Only use resolver result if both keys are non-empty
					if koTableKey != "" && tablePathKey != "" {
						tableMatch = koTableKey == tablePathKey
					}
				}
				if !keyMatch && resolve != nil {
					koKeyKey := resolve(ko.Key)
					keyPathKey := resolve(keyPath)
					// Only use resolver result if both keys are non-empty
					if koKeyKey != "" && keyPathKey != "" {
						keyMatch = koKeyKey == keyPathKey
					}
				}
				if tableMatch && keyMatch {
					found = true
					break
				}
			}
		}
		if !found {
			return false
		}
	}
	return true
}
