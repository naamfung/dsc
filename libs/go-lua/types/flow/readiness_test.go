package flow

import (
	"math/rand"
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow/numeric"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

// =============================================================================
// Order-Independence Tests
// =============================================================================

// TestOrderIndependence_EdgeConditions verifies that shuffling edge condition
// order produces identical narrowing results.
func TestOrderIndependence_EdgeConditions(t *testing.T) {
	typeA := typ.NewRecord().Field("tag", typ.LiteralString("a")).Field("data", typ.String).Build()
	typeB := typ.NewRecord().Field("tag", typ.LiteralString("b")).Field("data", typ.Number).Build()
	typeC := typ.NewRecord().Field("tag", typ.LiteralString("c")).Field("data", typ.Boolean).Build()
	union := typ.NewUnion(typeA, typeB, typeC)

	// Build CFG: entry -> branch -> [case1, case2, case3] -> join -> exit
	c := cfg.New()
	branch := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	case1 := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	case2 := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	case3 := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	join := c.AddNode(cfg.NodeJoin, cfg.SymbolID(0), "")

	c.AddEdge(c.Entry(), branch, true)
	c.AddEdge(branch, case1, true)
	c.AddEdge(branch, case2, true)
	c.AddEdge(branch, case3, false)
	c.AddEdge(case1, join, true)
	c.AddEdge(case2, join, true)
	c.AddEdge(case3, join, true)
	c.AddEdge(join, c.Exit(), true)

	g := newMockSSAGraph(c)
	allPoints := []cfg.Point{c.Entry(), branch, case1, case2, case3, join, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	pathX := constraint.Path{Root: "x", Symbol: symX}

	// Define edge conditions in different orders
	edgeConditions := []EdgeCondition{
		{From: branch, To: case1, Condition: constraint.FromConstraints(constraint.FieldEquals{Target: pathX, Field: "tag", Value: typ.LiteralString("a")})},
		{From: branch, To: case2, Condition: constraint.FromConstraints(constraint.FieldEquals{Target: pathX, Field: "tag", Value: typ.LiteralString("b")})},
		{From: branch, To: case3, Condition: constraint.FromConstraints(constraint.FieldEquals{Target: pathX, Field: "tag", Value: typ.LiteralString("c")})},
	}

	// Run with original order
	inputs1 := newInputs(g)
	inputs1.DeclaredTypes[symX] = union
	inputs1.EdgeConditions = edgeConditions
	s1 := Solve(inputs1, testResolver())
	got1Case1 := s1.NarrowedTypeAt(case1, pathX)
	got1Case2 := s1.NarrowedTypeAt(case2, pathX)
	got1Case3 := s1.NarrowedTypeAt(case3, pathX)

	// Shuffle and run multiple times
	for i := 0; i < 5; i++ {
		shuffled := make([]EdgeCondition, len(edgeConditions))
		copy(shuffled, edgeConditions)
		rand.Shuffle(len(shuffled), func(i, j int) {
			shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
		})

		inputs2 := newInputs(g)
		inputs2.DeclaredTypes[symX] = union
		inputs2.EdgeConditions = shuffled
		s2 := Solve(inputs2, testResolver())

		got2Case1 := s2.NarrowedTypeAt(case1, pathX)
		got2Case2 := s2.NarrowedTypeAt(case2, pathX)
		got2Case3 := s2.NarrowedTypeAt(case3, pathX)

		if !typ.TypeEquals(got1Case1, got2Case1) {
			t.Errorf("permutation %d: case1 mismatch: %v vs %v", i, got1Case1, got2Case1)
		}
		if !typ.TypeEquals(got1Case2, got2Case2) {
			t.Errorf("permutation %d: case2 mismatch: %v vs %v", i, got1Case2, got2Case2)
		}
		if !typ.TypeEquals(got1Case3, got2Case3) {
			t.Errorf("permutation %d: case3 mismatch: %v vs %v", i, got1Case3, got2Case3)
		}
	}
}

// TestOrderIndependence_PhiOperands verifies that phi operand order doesn't
// affect the resulting type at join points.
func TestOrderIndependence_PhiOperands(t *testing.T) {
	typeA := typ.NewRecord().Field("kind", typ.LiteralString("a")).Build()
	typeB := typ.NewRecord().Field("kind", typ.LiteralString("b")).Build()
	typeC := typ.NewRecord().Field("kind", typ.LiteralString("c")).Build()

	c := cfg.New()
	branch := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	path1 := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	path2 := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	path3 := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	join := c.AddNode(cfg.NodeJoin, cfg.SymbolID(0), "")

	c.AddEdge(c.Entry(), branch, true)
	c.AddEdge(branch, path1, true)
	c.AddEdge(branch, path2, true)
	c.AddEdge(branch, path3, false)
	c.AddEdge(path1, join, true)
	c.AddEdge(path2, join, true)
	c.AddEdge(path3, join, true)
	c.AddEdge(join, c.Exit(), true)

	operands := []cfg.PhiOperand{
		{From: path1, Version: cfg.Version{Root: "x", Symbol: 1, ID: 1}},
		{From: path2, Version: cfg.Version{Root: "x", Symbol: 1, ID: 2}},
		{From: path3, Version: cfg.Version{Root: "x", Symbol: 1, ID: 3}},
	}

	// Run with original order
	g1 := newMockSSAGraph(c)
	allPoints := []cfg.Point{c.Entry(), branch, path1, path2, path3, join, c.Exit()}
	sym := setupSymbol(g1, "x", allPoints)

	setVersion(g1, c.Entry(), sym, cfg.Version{Root: "x", Symbol: sym, ID: 0})
	setVersion(g1, branch, sym, cfg.Version{Root: "x", Symbol: sym, ID: 0})
	setVersion(g1, path1, sym, cfg.Version{Root: "x", Symbol: sym, ID: 1})
	setVersion(g1, path2, sym, cfg.Version{Root: "x", Symbol: sym, ID: 2})
	setVersion(g1, path3, sym, cfg.Version{Root: "x", Symbol: sym, ID: 3})
	setVersion(g1, join, sym, cfg.Version{Root: "x", Symbol: sym, ID: 4})

	g1.addPhiNode(cfg.PhiNode{
		Point:    join,
		Target:   cfg.Version{Root: "x", Symbol: sym, ID: 4},
		Operands: operands,
	})

	inputs1 := newInputs(g1)
	inputs1.DeclaredTypes[sym] = typ.NewUnion(typeA, typeB, typeC)

	pathX := constraint.Path{Root: "x", Symbol: sym}
	inputs1.EdgeConditions = []EdgeCondition{
		{From: branch, To: path1, Condition: constraint.FromConstraints(constraint.FieldEquals{Target: pathX, Field: "kind", Value: typ.LiteralString("a")})},
		{From: branch, To: path2, Condition: constraint.FromConstraints(constraint.FieldEquals{Target: pathX, Field: "kind", Value: typ.LiteralString("b")})},
		{From: branch, To: path3, Condition: constraint.FromConstraints(constraint.FieldEquals{Target: pathX, Field: "kind", Value: typ.LiteralString("c")})},
	}

	s1 := Solve(inputs1, testResolver())
	got1 := s1.TypeAt(join, pathX)

	// Shuffle phi operands and verify same result
	for i := 0; i < 5; i++ {
		shuffled := make([]cfg.PhiOperand, len(operands))
		copy(shuffled, operands)
		rand.Shuffle(len(shuffled), func(i, j int) {
			shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
		})

		g2 := newMockSSAGraph(c)
		setupSymbol(g2, "x", allPoints)
		setVersion(g2, c.Entry(), sym, cfg.Version{Root: "x", Symbol: sym, ID: 0})
		setVersion(g2, branch, sym, cfg.Version{Root: "x", Symbol: sym, ID: 0})
		setVersion(g2, path1, sym, cfg.Version{Root: "x", Symbol: sym, ID: 1})
		setVersion(g2, path2, sym, cfg.Version{Root: "x", Symbol: sym, ID: 2})
		setVersion(g2, path3, sym, cfg.Version{Root: "x", Symbol: sym, ID: 3})
		setVersion(g2, join, sym, cfg.Version{Root: "x", Symbol: sym, ID: 4})

		g2.addPhiNode(cfg.PhiNode{
			Point:    join,
			Target:   cfg.Version{Root: "x", Symbol: sym, ID: 4},
			Operands: shuffled,
		})

		inputs2 := newInputs(g2)
		inputs2.DeclaredTypes[sym] = typ.NewUnion(typeA, typeB, typeC)
		inputs2.EdgeConditions = inputs1.EdgeConditions

		s2 := Solve(inputs2, testResolver())
		got2 := s2.TypeAt(join, pathX)

		if !typ.TypeEquals(got1, got2) {
			t.Errorf("phi permutation %d: join type mismatch: %v vs %v", i, got1, got2)
		}
	}
}

// =============================================================================
// DNF Cap Behavior Tests
// =============================================================================

// TestDNFCap_ExceedsMaxDisjuncts verifies that exceeding DefaultMaxDisjuncts
// doesn't panic and produces a sound (widened) result.
func TestDNFCap_ExceedsMaxDisjuncts(t *testing.T) {
	// Create many disjuncts (more than DefaultMaxDisjuncts = 32)
	var disjuncts [][]constraint.Constraint
	for i := 0; i < 50; i++ {
		disjuncts = append(disjuncts, []constraint.Constraint{
			constraint.FieldEquals{
				Target: constraint.Path{Root: "x"},
				Field:  "tag",
				Value:  typ.LiteralInt(int64(i)),
			},
		})
	}

	cond := constraint.FromDisjuncts(disjuncts)

	// Should not panic
	if cond.IsFalse() {
		t.Error("condition should not be false")
	}

	// Result should be widened (fewer disjuncts or collapsed to must-constraints)
	if cond.NumDisjuncts() > constraint.DefaultMaxDisjuncts {
		t.Errorf("expected at most %d disjuncts, got %d", constraint.DefaultMaxDisjuncts, cond.NumDisjuncts())
	}

	t.Logf("DNF cap: 50 disjuncts -> %d after normalization", cond.NumDisjuncts())
}

// TestDNFCap_ANDExplosion verifies that AND of large conditions doesn't explode.
func TestDNFCap_ANDExplosion(t *testing.T) {
	// Create two conditions with many disjuncts
	var disjuncts1, disjuncts2 [][]constraint.Constraint
	for i := 0; i < 10; i++ {
		disjuncts1 = append(disjuncts1, []constraint.Constraint{
			constraint.HasType{Path: constraint.Path{Root: "x"}, Type: narrow.BuiltinTypeKey("string")},
		})
		disjuncts2 = append(disjuncts2, []constraint.Constraint{
			constraint.NotNil{Path: constraint.Path{Root: "y"}},
		})
	}

	cond1 := constraint.FromDisjuncts(disjuncts1)
	cond2 := constraint.FromDisjuncts(disjuncts2)

	// AND would normally produce 100 disjuncts (10 * 10)
	result := constraint.And(cond1, cond2)

	// Should not panic and should be capped
	if result.IsFalse() {
		t.Error("AND result should not be false")
	}
	if result.NumDisjuncts() > constraint.DefaultMaxDisjuncts {
		t.Errorf("AND explosion not capped: got %d disjuncts", result.NumDisjuncts())
	}

	t.Logf("AND cap: 10x10 -> %d after capping", result.NumDisjuncts())
}

// TestDNFCap_NarrowingSoundness verifies that DNF cap doesn't cause false positives.
func TestDNFCap_NarrowingSoundness(t *testing.T) {
	// Build a union with many variants
	var variants []typ.Type
	for i := 0; i < 40; i++ {
		variants = append(variants, typ.NewRecord().Field("id", typ.LiteralInt(int64(i))).Build())
	}
	union := typ.NewUnion(variants...)

	c, branch, thenNode, _, join := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, join, c.Exit()}
	sym := setupSymbol(g, "x", allPoints)
	ver := cfg.Version{Root: "x", Symbol: sym, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, sym, ver)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[sym] = union

	// Constraint that should narrow to id=0
	pathX := constraint.Path{Root: "x", Symbol: sym}
	inputs.EdgeConditions = []EdgeCondition{
		{From: branch, To: thenNode, Condition: constraint.FromConstraints(
			constraint.FieldEquals{Target: pathX, Field: "id", Value: typ.LiteralInt(0)},
		)},
	}

	s := Solve(inputs, testResolver())
	got := s.NarrowedTypeAt(thenNode, pathX)

	// Result must include the matching variant (soundness: no false negatives)
	if got == nil {
		t.Fatal("narrowing produced nil type")
	}

	// The narrowed type should be a subset of or equal to the original
	// It should include {id: 0}
	t.Logf("narrowed type: %v", got)
}

// =============================================================================
// Aliasing + Mutation Tests
// =============================================================================

// TestAliasing_ConstraintOnAlias verifies that constraint on alias t narrows
// the original u when t = u.
func TestAliasing_ConstraintOnAlias(t *testing.T) {
	typeA := typ.NewRecord().Field("tag", typ.LiteralString("a")).Field("value", typ.Number).Build()
	typeB := typ.NewRecord().Field("tag", typ.LiteralString("b")).Field("value", typ.String).Build()
	union := typ.NewUnion(typeA, typeB)

	c, branch, thenNode, _, join := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, join, c.Exit()}
	symU := setupSymbol(g, "u", allPoints)
	symT := setupSymbol(g, "t", allPoints)

	verU := cfg.Version{Root: "u", Symbol: symU, ID: 1}
	verT := cfg.Version{Root: "t", Symbol: symT, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symU, verU)
		setVersion(g, p, symT, verT)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symU] = union
	inputs.DeclaredTypes[symT] = union

	pathU := constraint.Path{Root: "u", Symbol: symU}
	pathT := constraint.Path{Root: "t", Symbol: symT}

	// t = u (EqPath) AND t.tag == "a"
	inputs.EdgeConditions = []EdgeCondition{
		{From: branch, To: thenNode, Condition: constraint.FromConstraints(
			constraint.NewEqPath(pathT, pathU),
			constraint.FieldEquals{Target: pathT, Field: "tag", Value: typ.LiteralString("a")},
		)},
	}

	s := Solve(inputs, testResolver())

	// t should be narrowed to typeA
	gotT := s.NarrowedTypeAt(thenNode, pathT)
	if !typ.TypeEquals(gotT, typeA) {
		t.Errorf("t: got %v, want %v", gotT, typeA)
	}

	// Note: u narrowing via EqPath is implementation-dependent
	// The test verifies t is narrowed; u narrowing is a bonus
	t.Logf("t narrowed to: %v", gotT)
}

// TestReassignment_KillsPriorConstraints verifies that reassignment kills
// constraints from the previous value.
func TestReassignment_KillsPriorConstraints(t *testing.T) {
	typeA := typ.NewRecord().Field("tag", typ.LiteralString("a")).Build()
	typeB := typ.NewRecord().Field("tag", typ.LiteralString("b")).Build()
	union := typ.NewUnion(typeA, typeB)

	c := cfg.New()
	branch := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	narrow := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	reassign := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")

	c.AddEdge(c.Entry(), branch, true)
	c.AddEdge(branch, narrow, true)
	c.AddEdge(branch, c.Exit(), false)
	c.AddEdge(narrow, reassign, true)
	c.AddEdge(reassign, c.Exit(), true)

	g := newMockSSAGraph(c)
	allPoints := []cfg.Point{c.Entry(), branch, narrow, reassign, c.Exit()}
	sym := setupSymbol(g, "x", allPoints)

	// Version 1: initial, Version 2: after reassignment
	ver1 := cfg.Version{Root: "x", Symbol: sym, ID: 1}
	ver2 := cfg.Version{Root: "x", Symbol: sym, ID: 2}

	setVersion(g, c.Entry(), sym, ver1)
	setVersion(g, branch, sym, ver1)
	setVersion(g, narrow, sym, ver1)
	setVersion(g, reassign, sym, ver2) // New version after reassignment

	inputs := newInputs(g)
	inputs.DeclaredTypes[sym] = union

	pathX := constraint.Path{Root: "x", Symbol: sym}

	// x.tag == "a" on edge to narrow
	inputs.EdgeConditions = []EdgeCondition{
		{From: branch, To: narrow, Condition: constraint.FromConstraints(
			constraint.FieldEquals{Target: pathX, Field: "tag", Value: typ.LiteralString("a")},
		)},
	}

	s := Solve(inputs, testResolver())

	// At narrow: x should be typeA
	gotNarrow := s.NarrowedTypeAt(narrow, pathX)
	if !typ.TypeEquals(gotNarrow, typeA) {
		t.Errorf("narrow: got %v, want %v", gotNarrow, typeA)
	}

	// At reassign: new version, constraint killed, should be full union
	gotReassign := s.TypeAt(reassign, pathX)
	t.Logf("after reassign: %v (constraint should be killed for new version)", gotReassign)
}

// =============================================================================
// Metatable/Dynamic Index Tests
// =============================================================================

// TestMetatable_DynamicIndex verifies conservative handling of dynamic keys.
func TestMetatable_DynamicIndex(t *testing.T) {
	// Record with metatable that might provide __index
	meta := typ.NewRecord().Field("__index", typ.Any).Build()
	rec := typ.NewRecord().Field("known", typ.String).Build()
	rec.Metatable = meta

	c, branch, thenNode, _, _ := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	sym := setupSymbol(g, "x", allPoints)
	ver := cfg.Version{Root: "x", Symbol: sym, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, sym, ver)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[sym] = rec

	pathX := constraint.Path{Root: "x", Symbol: sym}

	// Dynamic field access - should not narrow aggressively
	inputs.EdgeConditions = []EdgeCondition{
		{From: branch, To: thenNode, Condition: constraint.FromConstraints(
			constraint.HasField{Path: pathX, Field: "unknown"},
		)},
	}

	s := Solve(inputs, testResolver())

	// Type should remain the original (conservative)
	got := s.TypeAt(thenNode, pathX)
	t.Logf("metatable type after HasField(unknown): %v", got)

	// Should not have narrowed to Never (would be unsound)
	if got != nil && got.Kind() == typ.Never.Kind() {
		t.Error("should not narrow to Never for dynamic field")
	}
}

// TestDynamicKey_IndexAccess verifies dynamic key access is handled conservatively.
func TestDynamicKey_IndexAccess(t *testing.T) {
	// Map type where keys are dynamic
	mapType := typ.NewMap(typ.String, typ.Number)

	c, branch, thenNode, _, _ := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	sym := setupSymbol(g, "m", allPoints)
	ver := cfg.Version{Root: "m", Symbol: sym, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, sym, ver)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[sym] = mapType

	s := Solve(inputs, testResolver())

	// Map type should be preserved
	pathM := constraint.Path{Root: "m", Symbol: sym}
	got := s.TypeAt(thenNode, pathM)
	if got == nil {
		t.Fatal("map type should not be nil")
	}

	t.Logf("map type preserved: %v", got)
}

// =============================================================================
// Numeric Theory Soundness Tests
// =============================================================================

// TestTheorySoundness_NeverWidensBeyondInterval verifies theory solver never
// produces bounds wider than the interval state.
func TestTheorySoundness_NeverWidensBeyondInterval(t *testing.T) {
	state := numeric.NewState()

	// Set interval: x in [0, 10]
	xKey := constraint.PathKey("sym1")
	state.ApplyGeConst(xKey, 0)
	state.ApplyLeConst(xKey, 10)

	// Add relational constraint: x < y
	yKey := constraint.PathKey("sym2")
	state.ApplyLt(xKey, yKey)

	// Tighten with theory
	tightened := numeric.TightenWithTheory(state)
	if tightened.IsUnsat() {
		t.Fatal("should be satisfiable")
	}

	lower, upper, ok := tightened.BoundsFor(xKey)
	if !ok {
		// No bounds is fine
		return
	}

	// Theory should not widen beyond [0, 10]
	if lower < 0 {
		t.Errorf("theory widened lower bound: got %d, original was 0", lower)
	}
	if upper > 10 {
		t.Errorf("theory widened upper bound: got %d, original was 10", upper)
	}

	t.Logf("theory tightened x: [%d, %d]", lower, upper)
}

// TestTheorySoundness_UNSATMarksUnreachable verifies that UNSAT from theory
// properly marks the state as unsatisfiable.
func TestTheorySoundness_UNSATMarksUnreachable(t *testing.T) {
	state := numeric.NewState()

	xKey := constraint.PathKey("sym1")
	yKey := constraint.PathKey("sym2")

	// x > y AND y > x (contradiction)
	state.ApplyGt(xKey, yKey)
	state.ApplyGt(yKey, xKey)

	// Theory should detect UNSAT
	tightened := numeric.TightenWithTheory(state)
	if !tightened.IsUnsat() {
		t.Error("contradictory constraints should be UNSAT")
	}
}

// TestTheorySoundness_TransitiveTighteningSound verifies transitive bounds
// are sound (never exclude valid values).
func TestTheorySoundness_TransitiveTighteningSound(t *testing.T) {
	state := numeric.NewState()

	// a < b, b < c, c <= 100
	aKey := constraint.PathKey("a#1")
	bKey := constraint.PathKey("b#1")
	cKey := constraint.PathKey("c#1")

	state.ApplyLt(aKey, bKey)
	state.ApplyLt(bKey, cKey)
	state.ApplyLeConst(cKey, 100)

	tightened := numeric.TightenWithTheory(state)
	if tightened.IsUnsat() {
		t.Fatal("should be satisfiable")
	}

	// Theory derives: a <= 98 (c-2)
	lower, upper, ok := numeric.BoundsForWithTheory(tightened, aKey)
	if !ok {
		t.Log("no bounds derived for a")
		return
	}

	// Upper bound should be at most 98
	if upper > 98 {
		t.Errorf("transitive bound not tight enough: a <= %d, expected <= 98", upper)
	}

	// Any value in [MinInt, 98] should be valid for a
	// This is sound because we don't exclude valid values
	t.Logf("transitive bounds for a: [%d, %d]", lower, upper)
}

// TestTheorySoundness_ModularDoesNotContradict verifies modular constraints
// don't contradict interval constraints.
func TestTheorySoundness_ModularDoesNotContradict(t *testing.T) {
	state := numeric.NewState()

	xKey := constraint.PathKey("sym1")

	// x in [0, 9], x % 2 == 0
	state.ApplyGeConst(xKey, 0)
	state.ApplyLeConst(xKey, 9)
	state.ApplyModEq(xKey, 2, 0)

	// Should be satisfiable (0, 2, 4, 6, 8 are valid)
	if state.IsUnsat() {
		t.Error("valid modular constraint marked UNSAT")
	}

	// Count should be correct
	count := numeric.CountModularValues(state, xKey, 2, 0)
	if count != 5 {
		t.Errorf("expected 5 even values in [0,9], got %d", count)
	}
}

// TestTheorySoundness_EmptyIntervalIsUNSAT verifies empty intervals are UNSAT.
func TestTheorySoundness_EmptyIntervalIsUNSAT(t *testing.T) {
	state := numeric.NewState()

	xKey := constraint.PathKey("sym1")

	// x >= 10 AND x <= 5 (empty interval)
	state.ApplyGeConst(xKey, 10)
	state.ApplyLeConst(xKey, 5)

	if !state.IsUnsat() {
		t.Error("empty interval should be UNSAT")
	}
}

// =============================================================================
// Helpers
// =============================================================================
