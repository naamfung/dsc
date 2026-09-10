package constraint

import (
	"sort"

	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

func stopDepth(t typ.Type, depth int) bool {
	return t == nil || typ.DepthExceeded(depth)
}

func stopDepthPattern(pattern, concrete typ.Type, depth int) bool {
	return typ.DepthExceeded(depth) || pattern == nil || concrete == nil
}

// InferSet collects bounds for type variables during generic type inference.
//
// InferSet implements constraint-based type inference using bounds tracking.
// It collects subtype relationships between type variables and concrete types,
// then solves for the most specific types that satisfy all constraints.
//
// # Algorithm
//
// 1. Constraints are added via [InferSet.AddSubtype] and [InferSet.AddEqual]
// 2. Lower bounds (types that are subtypes of the variable) are joined
// 3. Upper bounds (types that the variable must be a subtype of) are met
// 4. Cyclic dependencies are resolved using Tarjan's SCC algorithm
// 5. [InferSet.Solve] returns a substitution mapping variables to concrete types
//
// # Example
//
//	cs := constraint.NewInferSet()
//	tv := typ.NewTypeVar(1)
//
//	// T is a supertype of string (string <: T)
//	cs.AddSubtype(typ.String, tv)
//
//	// T is a subtype of any (T <: any)
//	cs.AddSubtype(tv, typ.Any)
//
//	subst, err := cs.Solve()
//	// subst[1] == typ.String
//
// # Cyclic Dependencies
//
// When type variables form cycles (T1 <: T2 <: T3 <: T1), InferSet uses
// Tarjan's algorithm to find strongly connected components and unifies
// all variables in each SCC to a single representative.
type InferSet struct {
	bounds        map[int]*Bounds
	unsatisfiable bool
}

// NewInferSet creates an empty inference constraint set.
func NewInferSet() *InferSet {
	return &InferSet{
		bounds: make(map[int]*Bounds),
	}
}

func (c *InferSet) getBounds(v *typ.TypeVar) *Bounds {
	if b, ok := c.bounds[v.ID]; ok {
		return b
	}

	b := NewBounds()
	c.bounds[v.ID] = b

	return b
}

// AddSubtype records sub <: super.
// If sub is TypeVar, super becomes upper bound.
// If super is TypeVar, sub becomes lower bound.
func (c *InferSet) AddSubtype(sub, super typ.Type) {
	if sub == nil || super == nil {
		return
	}

	subIsVar := sub.Kind() == kind.TypeVar
	superIsVar := super.Kind() == kind.TypeVar

	if subIsVar {
		v := sub.(*typ.TypeVar)
		if occursIn(v.ID, super) {
			c.unsatisfiable = true
		} else {
			c.getBounds(v).AddUpper(super)
		}
	}

	if superIsVar {
		v := super.(*typ.TypeVar)
		if occursIn(v.ID, sub) {
			c.unsatisfiable = true
		} else {
			c.getBounds(v).AddLower(sub)
		}
	}

	if !subIsVar && !superIsVar {
		if !subtype.IsSubtype(sub, super) {
			c.unsatisfiable = true
		}
	}
}

// AddEqual records that two types must be equal.
func (c *InferSet) AddEqual(a, b typ.Type) {
	c.AddSubtype(a, b)
	c.AddSubtype(b, a)
}

// Solve attempts to solve all constraints.
func (c *InferSet) Solve() (InferSubstitution, error) {
	if c.unsatisfiable {
		return nil, &UnsatisfiableError{}
	}

	result := make(InferSubstitution)
	sccMap := c.unifyCircularDependencies()

	for _, id := range sortedBoundIDs(c.bounds) {
		bounds := c.bounds[id]
		solved, err := bounds.Solve()
		if err != nil {
			return nil, err
		}

		if solved != nil {
			result[id] = solved
		}
	}

	for id, rep := range sccMap {
		if id != rep {
			if result[rep] != nil {
				result[id] = result[rep]
			} else {
				delete(result, id)
			}
		}
	}

	for iter := 0; iter < 100; iter++ {
		changed := false

		for _, id := range sortedSubstIDs(result) {
			t := result[id]
			newT := result.Apply(t)
			if newT != t {
				result[id] = newT
				changed = true
			}
		}

		if !changed {
			break
		}
	}

	return result, nil
}

func (c *InferSet) unifyCircularDependencies() map[int]int {
	sccMap := make(map[int]int)
	graph := make(map[int][]int)

	for _, id := range sortedBoundIDs(c.bounds) {
		bounds := c.bounds[id]
		for _, upper := range bounds.Upper {
			if tv, ok := upper.(*typ.TypeVar); ok {
				graph[id] = append(graph[id], tv.ID)
			}
		}

		for _, lower := range bounds.Lower {
			if tv, ok := lower.(*typ.TypeVar); ok {
				graph[tv.ID] = append(graph[tv.ID], id)
			}
		}
	}

	for id, neighbors := range graph {
		if len(neighbors) > 1 {
			sort.Ints(neighbors)
			graph[id] = neighbors
		}
	}

	visited := make(map[int]bool)
	stack := make([]int, 0)
	inStack := make(map[int]bool)
	index := make(map[int]int)
	lowlink := make(map[int]int)
	indexCounter := 0

	var tarjan func(v int)
	tarjan = func(v int) {
		index[v] = indexCounter
		lowlink[v] = indexCounter
		indexCounter++

		stack = append(stack, v)

		inStack[v] = true
		visited[v] = true

		for _, w := range graph[v] {
			if !visited[w] {
				tarjan(w)

				if lowlink[w] < lowlink[v] {
					lowlink[v] = lowlink[w]
				}
			} else if inStack[w] {
				if index[w] < lowlink[v] {
					lowlink[v] = index[w]
				}
			}
		}

		if lowlink[v] == index[v] {
			var scc []int

			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]

				delete(inStack, w)

				scc = append(scc, w)

				if w == v {
					break
				}
			}

			if len(scc) > 1 {
				sort.Ints(scc)
				c.unifySCC(scc)

				representative := scc[0]
				for _, id := range scc {
					sccMap[id] = representative
				}
			}
		}
	}

	allNodes := make(map[int]bool)

	for id := range c.bounds {
		allNodes[id] = true
	}

	for _, neighbors := range graph {
		for _, n := range neighbors {
			allNodes[n] = true
		}
	}

	nodes := make([]int, 0, len(allNodes))
	for id := range allNodes {
		nodes = append(nodes, id)
	}
	sort.Ints(nodes)

	for _, id := range nodes {
		if !visited[id] {
			tarjan(id)
		}
	}

	return sccMap
}

func sortedBoundIDs(bounds map[int]*Bounds) []int {
	if len(bounds) == 0 {
		return nil
	}
	ids := make([]int, 0, len(bounds))
	for id := range bounds {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids
}

func sortedSubstIDs(subst InferSubstitution) []int {
	if len(subst) == 0 {
		return nil
	}
	ids := make([]int, 0, len(subst))
	for id := range subst {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids
}

func (c *InferSet) unifySCC(scc []int) {
	if len(scc) <= 1 {
		return
	}

	representative := scc[0]

	repBounds := c.bounds[representative]
	if repBounds == nil {
		repBounds = NewBounds()
		c.bounds[representative] = repBounds
	}

	sccSet := make(map[int]bool)

	for _, id := range scc {
		sccSet[id] = true
	}

	newLower := make([]typ.Type, 0)

	for _, lower := range repBounds.Lower {
		if tv, ok := lower.(*typ.TypeVar); ok && sccSet[tv.ID] {
			continue
		}

		newLower = append(newLower, lower)
	}

	newUpper := make([]typ.Type, 0)

	for _, upper := range repBounds.Upper {
		if tv, ok := upper.(*typ.TypeVar); ok && sccSet[tv.ID] {
			continue
		}

		newUpper = append(newUpper, upper)
	}

	repBounds.Lower = newLower
	repBounds.Upper = newUpper

	for i := 1; i < len(scc); i++ {
		id := scc[i]

		bounds := c.bounds[id]
		if bounds == nil {
			continue
		}

		for _, lower := range bounds.Lower {
			if tv, ok := lower.(*typ.TypeVar); ok && sccSet[tv.ID] {
				continue
			}

			repBounds.AddLower(lower)
		}

		for _, upper := range bounds.Upper {
			if tv, ok := upper.(*typ.TypeVar); ok && sccSet[tv.ID] {
				continue
			}

			repBounds.AddUpper(upper)
		}

		c.bounds[id] = repBounds
	}
}

func canContainTypeVar(k kind.Kind) bool {
	switch k {
	case kind.Optional, kind.Union, kind.Intersection, kind.Array,
		kind.Map, kind.Tuple, kind.Function, kind.Record, kind.Alias,
		kind.TypeVar, kind.Instantiated:
		return true
	default:
		return false
	}
}

func occursIn(varID int, t typ.Type) bool {
	seen := make(map[typ.Type]bool)
	return walkTypeMemo(t, 0, seen, func(inner typ.Type) bool {
		if tv, ok := inner.(*typ.TypeVar); ok {
			return tv.ID == varID
		}
		return false
	})
}

func walkTypeMemo(t typ.Type, depth int, seen map[typ.Type]bool, pred func(typ.Type) bool) bool {
	if stopDepth(t, depth) {
		return false
	}
	if pred(t) {
		return true
	}
	if !canContainTypeVar(t.Kind()) {
		return false
	}
	if seen[t] {
		return false
	}
	seen[t] = true
	return typ.Visit(t, typ.Visitor[bool]{
		Optional: func(o *typ.Optional) bool {
			return walkTypeMemo(o.Inner, depth+1, seen, pred)
		},
		Union: func(u *typ.Union) bool {
			for _, m := range u.Members {
				if walkTypeMemo(m, depth+1, seen, pred) {
					return true
				}
			}
			return false
		},
		Intersection: func(in *typ.Intersection) bool {
			for _, m := range in.Members {
				if walkTypeMemo(m, depth+1, seen, pred) {
					return true
				}
			}
			return false
		},
		Tuple: func(tup *typ.Tuple) bool {
			for _, e := range tup.Elements {
				if walkTypeMemo(e, depth+1, seen, pred) {
					return true
				}
			}
			return false
		},
		Array: func(a *typ.Array) bool {
			return walkTypeMemo(a.Element, depth+1, seen, pred)
		},
		Map: func(m *typ.Map) bool {
			return walkTypeMemo(m.Key, depth+1, seen, pred) || walkTypeMemo(m.Value, depth+1, seen, pred)
		},
		Function: func(fn *typ.Function) bool {
			for _, p := range fn.Params {
				if walkTypeMemo(p.Type, depth+1, seen, pred) {
					return true
				}
			}
			for _, r := range fn.Returns {
				if walkTypeMemo(r, depth+1, seen, pred) {
					return true
				}
			}
			return walkTypeMemo(fn.Variadic, depth+1, seen, pred)
		},
		Record: func(r *typ.Record) bool {
			for _, f := range r.Fields {
				if walkTypeMemo(f.Type, depth+1, seen, pred) {
					return true
				}
			}
			return false
		},
		Alias: func(a *typ.Alias) bool {
			return walkTypeMemo(a.Target, depth+1, seen, pred)
		},
		Default: func(t typ.Type) bool {
			return false
		},
	})
}

// Bounds tracks constraints on a type variable.
type Bounds struct {
	Lower []typ.Type
	Upper []typ.Type
}

// NewBounds creates empty bounds.
func NewBounds() *Bounds {
	return &Bounds{}
}

// AddLower adds a lower bound.
func (b *Bounds) AddLower(t typ.Type) {
	if t == nil {
		return
	}

	b.Lower = append(b.Lower, t)
}

// AddUpper adds an upper bound.
func (b *Bounds) AddUpper(t typ.Type) {
	if t == nil {
		return
	}

	b.Upper = append(b.Upper, t)
}

// Solve finds a type that satisfies all bounds.
func (b *Bounds) Solve() (typ.Type, error) {
	lower := b.joinLower()
	upper := b.meetUpper()

	if len(b.Lower) == 0 && len(b.Upper) == 0 {
		return nil, nil
	}

	lower = subtype.Widen(lower)

	if !typ.IsNever(lower) && !typ.IsAny(upper) {
		if !containsTypeVar(lower) && !containsTypeVar(upper) {
			if !subtype.IsSubtype(lower, upper) {
				return nil, &BoundsError{Lower: lower, Upper: upper}
			}
		}
	}

	if len(b.Lower) > 0 && !containsTypeVar(lower) {
		return lower, nil
	}

	if len(b.Upper) > 0 {
		return upper, nil
	}

	if len(b.Lower) > 0 {
		return lower, nil
	}

	return nil, nil
}

func (b *Bounds) joinLower() typ.Type {
	if len(b.Lower) == 0 {
		return typ.Never
	}

	if len(b.Lower) == 1 {
		return b.Lower[0]
	}

	return subtype.NormalizeUnion(b.Lower...)
}

func (b *Bounds) meetUpper() typ.Type {
	if len(b.Upper) == 0 {
		return typ.Any
	}

	if len(b.Upper) == 1 {
		return b.Upper[0]
	}

	return subtype.NormalizeIntersection(b.Upper...)
}

// BoundsError represents unsatisfiable bounds.
type BoundsError struct {
	Lower typ.Type
	Upper typ.Type
}

func (e *BoundsError) Error() string {
	return "unsatisfiable bounds: " + e.Lower.String() + " is not subtype of " + e.Upper.String()
}

// UnsatisfiableError represents constraint set with concrete type violations.
type UnsatisfiableError struct{}

func (e *UnsatisfiableError) Error() string {
	return "unsatisfiable constraints: concrete subtype violation"
}

func containsTypeVar(t typ.Type) bool {
	seen := make(map[typ.Type]bool)
	return walkTypeMemo(t, 0, seen, func(inner typ.Type) bool {
		return inner.Kind() == kind.TypeVar
	})
}

// InferSubstitution maps type variable IDs to their solved types.
type InferSubstitution map[int]typ.Type

// Apply applies this substitution to a type.
func (s InferSubstitution) Apply(t typ.Type) typ.Type {
	visited := make(map[int]bool)
	memo := make(map[typ.Type]typ.Type)

	return applyInferSubst(t, s, visited, memo, 0)
}

func applyInferSubst(t typ.Type, s InferSubstitution, visited map[int]bool, memo map[typ.Type]typ.Type, depth int) typ.Type {
	if stopDepth(t, depth) {
		return t
	}

	if result, ok := memo[t]; ok {
		return result
	}

	if _, ok := t.(*typ.TypeVar); ok {
		current := t

		for {
			tv, ok := current.(*typ.TypeVar)
			if !ok {
				break
			}

			if visited[tv.ID] {
				return current
			}

			solved, ok := s[tv.ID]
			if !ok {
				return current
			}

			visited[tv.ID] = true

			if _, isSolvedVar := solved.(*typ.TypeVar); isSolvedVar {
				current = solved
				continue
			}

			result := applyInferSubst(solved, s, visited, memo, depth+1)
			curr := t

			for {
				tv, ok := curr.(*typ.TypeVar)
				if !ok {
					break
				}

				delete(visited, tv.ID)

				next, ok := s[tv.ID]
				if !ok {
					break
				}

				if _, isVar := next.(*typ.TypeVar); !isVar {
					break
				}

				curr = next
			}

			return result
		}

		return current
	}

	// Sentinel: mark t as in-progress so recursive references return t as-is.
	memo[t] = t
	result := typ.Visit(t, typ.Visitor[typ.Type]{
		Optional: func(o *typ.Optional) typ.Type {
			inner := applyInferSubst(o.Inner, s, visited, memo, depth+1)
			if inner == o.Inner {
				return t
			}

			return typ.NewOptional(inner)
		},
		Union: func(u *typ.Union) typ.Type {
			changed := false

			members := make([]typ.Type, len(u.Members))
			for i, m := range u.Members {
				members[i] = applyInferSubst(m, s, visited, memo, depth+1)
				if members[i] != m {
					changed = true
				}
			}

			if !changed {
				return t
			}

			return typ.NewUnion(members...)
		},
		Intersection: func(in *typ.Intersection) typ.Type {
			changed := false

			members := make([]typ.Type, len(in.Members))
			for i, m := range in.Members {
				members[i] = applyInferSubst(m, s, visited, memo, depth+1)
				if members[i] != m {
					changed = true
				}
			}

			if !changed {
				return t
			}

			return typ.NewIntersection(members...)
		},
		Tuple: func(tup *typ.Tuple) typ.Type {
			changed := false

			elems := make([]typ.Type, len(tup.Elements))
			for i, e := range tup.Elements {
				elems[i] = applyInferSubst(e, s, visited, memo, depth+1)
				if elems[i] != e {
					changed = true
				}
			}

			if !changed {
				return t
			}

			return typ.NewTuple(elems...)
		},
		Function: func(fn *typ.Function) typ.Type {
			changed := false
			params := make([]typ.Param, len(fn.Params))

			for i, p := range fn.Params {
				pType := applyInferSubst(p.Type, s, visited, memo, depth+1)
				params[i] = typ.Param{Name: p.Name, Type: pType, Optional: p.Optional}

				if pType != p.Type {
					changed = true
				}
			}

			returns := make([]typ.Type, len(fn.Returns))

			for i, r := range fn.Returns {
				returns[i] = applyInferSubst(r, s, visited, memo, depth+1)
				if returns[i] != r {
					changed = true
				}
			}

			var variadic typ.Type
			if fn.Variadic != nil {
				variadic = applyInferSubst(fn.Variadic, s, visited, memo, depth+1)
				if variadic != fn.Variadic {
					changed = true
				}
			}

			if !changed {
				return t
			}

			fb := typ.Func()

			for _, p := range params {
				if p.Optional {
					fb.OptParam(p.Name, p.Type)
				} else {
					fb.Param(p.Name, p.Type)
				}
			}

			if variadic != nil {
				fb.Variadic(variadic)
			}

			fb.Returns(returns...)

			return fb.Build()
		},
		Array: func(a *typ.Array) typ.Type {
			elem := applyInferSubst(a.Element, s, visited, memo, depth+1)
			if elem == a.Element {
				return t
			}

			return typ.NewArray(elem)
		},
		Map: func(m *typ.Map) typ.Type {
			key := applyInferSubst(m.Key, s, visited, memo, depth+1)
			value := applyInferSubst(m.Value, s, visited, memo, depth+1)

			if key == m.Key && value == m.Value {
				return t
			}

			return typ.NewMap(key, value)
		},
		Record: func(r *typ.Record) typ.Type {
			changed := false
			fields := make([]typ.Field, len(r.Fields))

			for i, f := range r.Fields {
				fType := applyInferSubst(f.Type, s, visited, memo, depth+1)
				fields[i] = typ.Field{Name: f.Name, Type: fType, Optional: f.Optional, Readonly: f.Readonly}

				if fType != f.Type {
					changed = true
				}
			}

			if !changed {
				return t
			}

			rb := typ.NewRecord()

			for _, f := range fields {
				if f.Readonly {
					rb.ReadonlyField(f.Name, f.Type)
				} else if f.Optional {
					rb.OptField(f.Name, f.Type)
				} else {
					rb.Field(f.Name, f.Type)
				}
			}

			rec := rb.Build()
			rec.Metatable = r.Metatable

			return rec
		},
		Alias: func(a *typ.Alias) typ.Type {
			target := applyInferSubst(a.Target, s, visited, memo, depth+1)
			if target == a.Target {
				return t
			}

			return typ.NewAlias(a.Name, target)
		},
		Default: func(t typ.Type) typ.Type {
			return t
		},
	})
	memo[t] = result
	return result
}

// Match walks pattern and concrete types in parallel, collecting constraints.
func Match(pattern, concrete typ.Type, cs *InferSet) {
	matchDepth(pattern, concrete, cs, subtype.Covariant, 0)
}

// MatchContra matches with contravariant orientation (for parameter positions).
func MatchContra(pattern, concrete typ.Type, cs *InferSet) {
	matchDepth(pattern, concrete, cs, subtype.Contravariant, 0)
}

// MatchCo matches with covariant orientation (for return positions).
func MatchCo(pattern, concrete typ.Type, cs *InferSet) {
	matchDepth(pattern, concrete, cs, subtype.Covariant, 0)
}

func matchDepth(pattern, concrete typ.Type, cs *InferSet, variance subtype.Variance, depth int) {
	if stopDepthPattern(pattern, concrete, depth) {
		return
	}

	pattern = unwrap.Alias(pattern)
	concrete = unwrap.Alias(concrete)
	concrete = subtype.WidenForInference(concrete)

	if pattern.Kind() == kind.TypeVar {
		v := pattern.(*typ.TypeVar)

		switch variance {
		case subtype.Covariant:
			cs.AddSubtype(v, concrete)
		case subtype.Contravariant:
			cs.AddSubtype(concrete, v)
		case subtype.Invariant, subtype.Bivariant:
			cs.AddSubtype(v, concrete)
			cs.AddSubtype(concrete, v)
		}

		return
	}

	if concrete.Kind() == kind.TypeVar {
		return
	}

	if isTopOrBottom(pattern) || isTopOrBottom(concrete) {
		return
	}

	typ.Visit(pattern, typ.Visitor[struct{}]{
		Optional: func(p *typ.Optional) struct{} {
			if c, ok := concrete.(*typ.Optional); ok {
				matchDepth(p.Inner, c.Inner, cs, variance, depth+1)
			} else {
				matchDepth(p.Inner, concrete, cs, variance, depth+1)
			}
			return struct{}{}
		},
		Union: func(p *typ.Union) struct{} {
			concreteMembers := []typ.Type{concrete}
			if c, ok := concrete.(*typ.Union); ok {
				concreteMembers = c.Members
			}
			matchUnionMembers(p.Members, concreteMembers, cs, variance, depth+1)
			return struct{}{}
		},
		Array: func(p *typ.Array) struct{} {
			if c, ok := concrete.(*typ.Array); ok {
				v := subtype.CombineVariance(variance, subtype.Invariant)
				matchDepth(p.Element, c.Element, cs, v, depth+1)
			}
			return struct{}{}
		},
		Map: func(p *typ.Map) struct{} {
			if c, ok := concrete.(*typ.Map); ok {
				keyVar := subtype.CombineVariance(variance, subtype.Invariant)
				valVar := subtype.CombineVariance(variance, subtype.Invariant)

				matchDepth(p.Key, c.Key, cs, keyVar, depth+1)
				matchDepth(p.Value, c.Value, cs, valVar, depth+1)
			} else if c, ok := concrete.(*typ.Record); ok {
				keyVar := subtype.CombineVariance(variance, subtype.Invariant)
				valVar := subtype.CombineVariance(variance, subtype.Invariant)

				for _, f := range c.Fields {
					keyType := typ.LiteralString(f.Name)
					matchDepth(p.Key, keyType, cs, keyVar, depth+1)
					matchDepth(p.Value, f.Type, cs, valVar, depth+1)
				}
			} else if c, ok := concrete.(*typ.Intersection); ok {
				// Match map pattern against intersection by matching with first map member
				for _, m := range c.Members {
					if mapType, ok := m.(*typ.Map); ok {
						keyVar := subtype.CombineVariance(variance, subtype.Invariant)
						valVar := subtype.CombineVariance(variance, subtype.Invariant)

						matchDepth(p.Key, mapType.Key, cs, keyVar, depth+1)
						matchDepth(p.Value, mapType.Value, cs, valVar, depth+1)

						break
					}
				}
			}
			return struct{}{}
		},
		Tuple: func(p *typ.Tuple) struct{} {
			if c, ok := concrete.(*typ.Tuple); ok {
				n := len(p.Elements)
				if len(c.Elements) < n {
					n = len(c.Elements)
				}

				for i := 0; i < n; i++ {
					matchDepth(p.Elements[i], c.Elements[i], cs, variance, depth+1)
				}
			}
			return struct{}{}
		},
		Function: func(p *typ.Function) struct{} {
			if c, ok := concrete.(*typ.Function); ok {
				paramVar := subtype.CombineVariance(variance, subtype.Contravariant)
				retVar := subtype.CombineVariance(variance, subtype.Covariant)

				n := len(p.Params)
				if len(c.Params) < n {
					n = len(c.Params)
				}

				for i := 0; i < n; i++ {
					matchDepth(p.Params[i].Type, c.Params[i].Type, cs, paramVar, depth+1)
				}

				if p.Variadic != nil && c.Variadic != nil {
					matchDepth(p.Variadic, c.Variadic, cs, paramVar, depth+1)
				}

				n = len(p.Returns)
				if len(c.Returns) < n {
					n = len(c.Returns)
				}

				for i := 0; i < n; i++ {
					matchDepth(p.Returns[i], c.Returns[i], cs, retVar, depth+1)
				}
			}
			return struct{}{}
		},
		Record: func(p *typ.Record) struct{} {
			if c, ok := concrete.(*typ.Record); ok {
				for _, f := range p.Fields {
					if cf := c.GetField(f.Name); cf != nil {
						matchDepth(f.Type, cf.Type, cs, variance, depth+1)
					}
				}
			} else if c, ok := concrete.(*typ.Intersection); ok {
				// Match record pattern against intersection by matching with first record member
				for _, m := range c.Members {
					if rec, ok := m.(*typ.Record); ok {
						for _, f := range p.Fields {
							if cf := rec.GetField(f.Name); cf != nil {
								matchDepth(f.Type, cf.Type, cs, variance, depth+1)
							}
						}

						break // Only match against first record member
					}
				}
			}
			return struct{}{}
		},
		Instantiated: func(p *typ.Instantiated) struct{} {
			if c, ok := concrete.(*typ.Instantiated); ok {
				if p.Generic.Name == c.Generic.Name {
					v := subtype.CombineVariance(variance, subtype.Invariant)

					n := len(p.TypeArgs)
					if len(c.TypeArgs) < n {
						n = len(c.TypeArgs)
					}

					for i := 0; i < n; i++ {
						matchDepth(p.TypeArgs[i], c.TypeArgs[i], cs, v, depth+1)
					}
				}
			}
			return struct{}{}
		},
		Default: func(t typ.Type) struct{} {
			return struct{}{}
		},
	})
}

func matchUnionMembers(patternMembers, concreteMembers []typ.Type, cs *InferSet, variance subtype.Variance, depth int) {
	if len(patternMembers) == 0 || len(concreteMembers) == 0 {
		return
	}

	order := make([]int, len(patternMembers))
	for i := range patternMembers {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool {
		left := unionMemberSpecificity(patternMembers[order[i]])
		right := unionMemberSpecificity(patternMembers[order[j]])
		if left != right {
			return left > right
		}
		return order[i] < order[j]
	})

	used := make([]bool, len(concreteMembers))
	for _, patternIdx := range order {
		patternMember := patternMembers[patternIdx]

		bestConcrete := -1
		bestScore := -1
		for concreteIdx, concreteMember := range concreteMembers {
			if used[concreteIdx] || !typesOverlapForInference(patternMember, concreteMember) {
				continue
			}
			score := unionPairScore(patternMember, concreteMember)
			if score > bestScore {
				bestScore = score
				bestConcrete = concreteIdx
			}
		}

		if bestConcrete < 0 {
			continue
		}

		used[bestConcrete] = true
		matchDepth(patternMember, concreteMembers[bestConcrete], cs, variance, depth)
	}
}

func unionMemberSpecificity(t typ.Type) int {
	if t == nil {
		return 0
	}

	score := 0
	if !containsTypeVar(t) {
		score += 4
	}

	switch unwrap.Alias(t).Kind() {
	case kind.TypeVar:
		// Leave fully-generic members as the last resort.
	case kind.Literal:
		score += 4
	default:
		score += 2
	}

	return score
}

func unionPairScore(pattern, concrete typ.Type) int {
	score := unionMemberSpecificity(pattern)

	patternKind := unwrap.Alias(pattern).Kind()
	concreteKind := unwrap.Alias(concrete).Kind()
	if patternKind == concreteKind {
		score += 4
	}
	if subtype.IsSubtype(concrete, pattern) || subtype.IsSubtype(pattern, concrete) {
		score += 2
	}

	return score
}

func typesOverlapForInference(a, b typ.Type) bool {
	intersection := subtype.NormalizeIntersection(a, b)
	return intersection != nil && !typ.IsNever(intersection)
}

func isTopOrBottom(t typ.Type) bool {
	if t == nil {
		return true
	}
	return t.Kind().IsTopOrBottom()
}
