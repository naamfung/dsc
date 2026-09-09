package constraint

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

// BenchmarkSolverApply measures constraint solving performance.
func BenchmarkSolverApply(b *testing.B) {
	path1 := Path{Root: "x", Symbol: cfg.SymbolID(1)}
	path2 := Path{Root: "y", Symbol: cfg.SymbolID(2)}
	path3 := Path{Root: "z", Symbol: cfg.SymbolID(3)}

	constraints := []Constraint{
		Truthy{Path: path1},
		NotNil{Path: path2},
		HasType{Path: path3, Type: narrow.BuiltinTypeKey("string")},
	}

	base := map[PathKey]typ.Type{
		path1.Key(): typ.NewOptional(typ.String),
		path2.Key(): typ.NewOptional(typ.Number),
		path3.Key(): typ.NewUnion(typ.String, typ.Number, typ.Nil),
	}

	solver := Solver{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		solver.Apply(constraints, base)
	}
}

// BenchmarkSolverApplyLarge measures solver with many constraints below work-skip threshold.
func BenchmarkSolverApplyLarge(b *testing.B) {
	const numPaths = 50
	constraints := make([]Constraint, 0, numPaths*2)
	base := make(map[PathKey]typ.Type, numPaths)

	for i := 1; i <= numPaths; i++ {
		p := Path{Root: "v", Symbol: cfg.SymbolID(i)}
		base[p.Key()] = typ.NewOptional(typ.String)
		constraints = append(constraints, Truthy{Path: p})
		constraints = append(constraints, NotNil{Path: p})
	}

	solver := Solver{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		solver.Apply(constraints, base)
	}
}

// BenchmarkSolverApplyHuge measures solver with constraints above work-skip threshold.
func BenchmarkSolverApplyHuge(b *testing.B) {
	const numPaths = 150
	constraints := make([]Constraint, 0, numPaths*2)
	base := make(map[PathKey]typ.Type, numPaths)

	for i := 1; i <= numPaths; i++ {
		p := Path{Root: "v", Symbol: cfg.SymbolID(i)}
		base[p.Key()] = typ.NewOptional(typ.String)
		constraints = append(constraints, Truthy{Path: p})
		constraints = append(constraints, NotNil{Path: p})
	}

	solver := Solver{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		solver.Apply(constraints, base)
	}
}

// BenchmarkConditionAnd measures DNF conjunction performance.
func BenchmarkConditionAnd(b *testing.B) {
	path1 := Path{Root: "x", Symbol: cfg.SymbolID(1)}
	path2 := Path{Root: "y", Symbol: cfg.SymbolID(2)}

	cond1 := FromConstraints(Truthy{Path: path1}, NotNil{Path: path2})
	cond2 := FromConstraints(HasType{Path: path1, Type: narrow.BuiltinTypeKey("string")})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		And(cond1, cond2)
	}
}

// BenchmarkConditionOr measures DNF disjunction performance.
func BenchmarkConditionOr(b *testing.B) {
	path1 := Path{Root: "x", Symbol: cfg.SymbolID(1)}
	path2 := Path{Root: "y", Symbol: cfg.SymbolID(2)}

	cond1 := FromConstraints(Truthy{Path: path1})
	cond2 := FromConstraints(NotNil{Path: path2})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Or(cond1, cond2)
	}
}

// BenchmarkConditionAndLarge measures DNF with many disjuncts.
func BenchmarkConditionAndLarge(b *testing.B) {
	// Create conditions with multiple disjuncts
	var disjuncts1, disjuncts2 [][]Constraint
	for i := 1; i <= 8; i++ {
		p := Path{Root: "x", Symbol: cfg.SymbolID(i)}
		disjuncts1 = append(disjuncts1, []Constraint{Truthy{Path: p}})
		disjuncts2 = append(disjuncts2, []Constraint{NotNil{Path: p}})
	}

	cond1 := FromDisjuncts(disjuncts1)
	cond2 := FromDisjuncts(disjuncts2)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		And(cond1, cond2)
	}
}

// BenchmarkConditionNot measures DNF negation performance.
func BenchmarkConditionNot(b *testing.B) {
	path1 := Path{Root: "x", Symbol: cfg.SymbolID(1)}
	path2 := Path{Root: "y", Symbol: cfg.SymbolID(2)}

	cond := FromConstraints(Truthy{Path: path1}, NotNil{Path: path2})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Not(cond)
	}
}

// BenchmarkPathKey measures path key generation.
func BenchmarkPathKey(b *testing.B) {
	path := Path{
		Root:   "obj",
		Symbol: cfg.SymbolID(42),
		Segments: []Segment{
			{Kind: SegmentField, Name: "field"},
			{Kind: SegmentIndexInt, Index: 0},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = path.Key()
	}
}

// BenchmarkPathHash measures path hashing.
func BenchmarkPathHash(b *testing.B) {
	path := Path{
		Root:   "obj",
		Symbol: cfg.SymbolID(42),
		Segments: []Segment{
			{Kind: SegmentField, Name: "field"},
			{Kind: SegmentIndexString, Name: "key"},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = path.Hash()
	}
}

// BenchmarkPathEqual measures path equality comparison.
func BenchmarkPathEqual(b *testing.B) {
	path1 := Path{
		Root:   "obj",
		Symbol: cfg.SymbolID(42),
		Segments: []Segment{
			{Kind: SegmentField, Name: "field"},
			{Kind: SegmentIndexInt, Index: 5},
		},
	}
	path2 := Path{
		Root:   "obj",
		Symbol: cfg.SymbolID(42),
		Segments: []Segment{
			{Kind: SegmentField, Name: "field"},
			{Kind: SegmentIndexInt, Index: 5},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = path1.Equal(path2)
	}
}

// BenchmarkConstraintHash measures constraint hashing.
func BenchmarkConstraintHash(b *testing.B) {
	path := Path{Root: "x", Symbol: cfg.SymbolID(1)}
	c := FieldEquals{
		Target: path,
		Field:  "kind",
		Value:  typ.LiteralString("error"),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.Hash()
	}
}

// BenchmarkConjunctionContains measures binary search in sorted conjunction.
func BenchmarkConjunctionContains(b *testing.B) {
	var constraints []Constraint
	for i := 1; i <= 20; i++ {
		p := Path{Root: "v", Symbol: cfg.SymbolID(i)}
		constraints = append(constraints, Truthy{Path: p})
	}
	conj := canonicalizeConjunction(constraints)

	target := Truthy{Path: Path{Root: "v", Symbol: cfg.SymbolID(15)}}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ConjunctionContains(conj, target)
	}
}

// BenchmarkInferSetSolve measures generic type inference.
func BenchmarkInferSetSolve(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cs := NewInferSet()
		tv1 := typ.NewTypeVar(1)
		tv2 := typ.NewTypeVar(2)

		cs.AddSubtype(tv1, typ.String)
		cs.AddSubtype(typ.Number, tv1)
		cs.AddSubtype(tv2, tv1)
		cs.AddSubtype(typ.Boolean, tv2)

		_, _ = cs.Solve()
	}
}

// BenchmarkInferSetSolveCyclic measures inference with cyclic dependencies.
func BenchmarkInferSetSolveCyclic(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cs := NewInferSet()
		tv1 := typ.NewTypeVar(1)
		tv2 := typ.NewTypeVar(2)
		tv3 := typ.NewTypeVar(3)

		// Create cycle: tv1 <: tv2 <: tv3 <: tv1
		cs.AddSubtype(tv1, tv2)
		cs.AddSubtype(tv2, tv3)
		cs.AddSubtype(tv3, tv1)

		// Add concrete bounds
		cs.AddSubtype(typ.String, tv1)
		cs.AddSubtype(tv3, typ.Any)

		_, _ = cs.Solve()
	}
}

// BenchmarkSubstituteCondition measures placeholder substitution.
func BenchmarkSubstituteCondition(b *testing.B) {
	placeholder0 := Path{Root: "$0"}
	placeholder1 := Path{Root: "$1"}

	cond := FromConstraints(
		Truthy{Path: placeholder0},
		NotNil{Path: placeholder1},
		FieldEquals{Target: placeholder0, Field: "ok", Value: typ.LiteralBool(true)},
	)

	args := []Path{
		{Root: "result", Symbol: cfg.SymbolID(10)},
		{Root: "err", Symbol: cfg.SymbolID(11)},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cond.Substitute(args)
	}
}

// BenchmarkConditionHash measures condition hashing for memoization.
func BenchmarkConditionHash(b *testing.B) {
	path1 := Path{Root: "x", Symbol: cfg.SymbolID(1)}
	path2 := Path{Root: "y", Symbol: cfg.SymbolID(2)}

	cond := FromConstraints(
		Truthy{Path: path1},
		NotNil{Path: path2},
		HasType{Path: path1, Type: narrow.BuiltinTypeKey("table")},
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cond.Hash()
	}
}
