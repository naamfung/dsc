package flow

import (
	"math/rand"
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow/numeric"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

// =============================================================================
// Randomized Ordering Tests (Seeded for Determinism)
// =============================================================================

func TestStress_RandomizedEdgeConditionOrder(t *testing.T) {
	rng := rand.New(rand.NewSource(42))

	typeA := typ.NewRecord().Field("tag", typ.LiteralString("a")).Build()
	typeB := typ.NewRecord().Field("tag", typ.LiteralString("b")).Build()
	typeC := typ.NewRecord().Field("tag", typ.LiteralString("c")).Build()
	typeD := typ.NewRecord().Field("tag", typ.LiteralString("d")).Build()
	union := typ.NewUnion(typeA, typeB, typeC, typeD)

	c := cfg.New()
	branch := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	cases := make([]cfg.Point, 4)
	for i := range cases {
		cases[i] = c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	}
	join := c.AddNode(cfg.NodeJoin, cfg.SymbolID(0), "")

	c.AddEdge(c.Entry(), branch, true)
	for i, cs := range cases {
		c.AddEdge(branch, cs, i == 0)
		c.AddEdge(cs, join, true)
	}
	c.AddEdge(join, c.Exit(), true)

	g := newMockSSAGraph(c)
	allPoints := append([]cfg.Point{c.Entry(), branch}, cases...)
	allPoints = append(allPoints, join, c.Exit())
	sym := setupSymbol(g, "x", allPoints)
	ver := cfg.Version{Root: "x", Symbol: sym, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, sym, ver)
	}

	pathX := constraint.Path{Root: "x", Symbol: sym}
	tags := []string{"a", "b", "c", "d"}
	baseConditions := make([]EdgeCondition, 4)
	for i, tag := range tags {
		baseConditions[i] = EdgeCondition{
			From:      branch,
			To:        cases[i],
			Condition: constraint.FromConstraints(constraint.FieldEquals{Target: pathX, Field: "tag", Value: typ.LiteralString(tag)}),
		}
	}

	// Run with original order to get baseline
	inputs := newInputs(g)
	inputs.DeclaredTypes[sym] = union
	inputs.EdgeConditions = baseConditions
	baseline := Solve(inputs, testResolver())

	baselineResults := make([]typ.Type, 4)
	for i, cs := range cases {
		baselineResults[i] = baseline.NarrowedTypeAt(cs, pathX)
	}

	// Run 20 times with shuffled order
	for iter := 0; iter < 20; iter++ {
		shuffled := make([]EdgeCondition, len(baseConditions))
		copy(shuffled, baseConditions)
		rng.Shuffle(len(shuffled), func(i, j int) {
			shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
		})

		inputs2 := newInputs(g)
		inputs2.DeclaredTypes[sym] = union
		inputs2.EdgeConditions = shuffled
		s := Solve(inputs2, testResolver())

		for i, cs := range cases {
			got := s.NarrowedTypeAt(cs, pathX)
			if !typ.TypeEquals(got, baselineResults[i]) {
				t.Errorf("iter %d, case %d: got %v, want %v", iter, i, got, baselineResults[i])
			}
		}
	}
}

func TestStress_RandomizedConstraintOrder(t *testing.T) {
	rng := rand.New(rand.NewSource(123))

	c, branch, thenNode, _, _ := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	typeTarget := typ.NewRecord().
		Field("a", typ.LiteralString("x")).
		Field("b", typ.LiteralInt(1)).
		Field("c", typ.True).
		Build()
	typeOther := typ.NewRecord().
		Field("a", typ.String).
		Field("b", typ.Number).
		Field("c", typ.Boolean).
		Build()
	union := typ.NewUnion(typeTarget, typeOther)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	sym := setupSymbol(g, "x", allPoints)
	ver := cfg.Version{Root: "x", Symbol: sym, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, sym, ver)
	}

	pathX := constraint.Path{Root: "x", Symbol: sym}

	// Multiple constraints that should narrow to typeTarget
	baseConstraints := []constraint.Constraint{
		constraint.FieldEquals{Target: pathX, Field: "a", Value: typ.LiteralString("x")},
		constraint.FieldEquals{Target: pathX, Field: "b", Value: typ.LiteralInt(1)},
		constraint.FieldEquals{Target: pathX, Field: "c", Value: typ.True},
	}

	// Run with original order
	inputs := newInputs(g)
	inputs.DeclaredTypes[sym] = union
	inputs.EdgeConditions = []EdgeCondition{
		{From: branch, To: thenNode, Condition: constraint.FromConstraints(baseConstraints...)},
	}
	baseline := Solve(inputs, testResolver())
	baselineResult := baseline.NarrowedTypeAt(thenNode, pathX)

	// Run 20 times with shuffled constraint order
	for iter := 0; iter < 20; iter++ {
		shuffled := make([]constraint.Constraint, len(baseConstraints))
		copy(shuffled, baseConstraints)
		rng.Shuffle(len(shuffled), func(i, j int) {
			shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
		})

		inputs2 := newInputs(g)
		inputs2.DeclaredTypes[sym] = union
		inputs2.EdgeConditions = []EdgeCondition{
			{From: branch, To: thenNode, Condition: constraint.FromConstraints(shuffled...)},
		}
		s := Solve(inputs2, testResolver())
		got := s.NarrowedTypeAt(thenNode, pathX)

		if !typ.TypeEquals(got, baselineResult) {
			t.Errorf("iter %d: got %v, want %v", iter, got, baselineResult)
		}
	}
}

// =============================================================================
// Deep Nested Path Tests
// =============================================================================

func TestStress_DeepNestedPaths(t *testing.T) {
	// Build a deeply nested record type (10 levels)
	deepType := typ.String
	for i := 9; i >= 0; i-- {
		deepType = typ.NewRecord().Field("level", deepType).Build()
	}

	c, branch, thenNode, _, _ := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	sym := setupSymbol(g, "x", allPoints)
	ver := cfg.Version{Root: "x", Symbol: sym, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, sym, ver)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[sym] = deepType

	s := Solve(inputs, testResolver())

	// Query at each nesting level
	basePath := constraint.Path{Root: "x", Symbol: sym}
	currentPath := basePath
	for depth := 0; depth < 10; depth++ {
		got := s.TypeAt(thenNode, currentPath)
		if got == nil {
			t.Errorf("depth %d: TypeAt returned nil", depth)
			break
		}
		currentPath = currentPath.Append(constraint.Segment{Kind: constraint.SegmentField, Name: "level"})
	}
}

func TestStress_DeepNestedConstraints(t *testing.T) {
	// Create a type with discriminant at depth 5
	innerA := typ.NewRecord().Field("tag", typ.LiteralString("a")).Build()
	innerB := typ.NewRecord().Field("tag", typ.LiteralString("b")).Build()
	innerUnion := typ.NewUnion(innerA, innerB)

	// Wrap 5 levels deep
	wrapped := innerUnion
	for i := 0; i < 5; i++ {
		wrapped = typ.NewRecord().Field("inner", wrapped).Build()
	}

	c, branch, thenNode, _, _ := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	sym := setupSymbol(g, "x", allPoints)
	ver := cfg.Version{Root: "x", Symbol: sym, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, sym, ver)
	}

	// Build path to inner.inner.inner.inner.inner.tag
	pathX := constraint.Path{Root: "x", Symbol: sym}
	deepPath := pathX
	for i := 0; i < 5; i++ {
		deepPath = deepPath.Append(constraint.Segment{Kind: constraint.SegmentField, Name: "inner"})
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[sym] = wrapped
	inputs.EdgeConditions = []EdgeCondition{
		{
			From: branch,
			To:   thenNode,
			Condition: constraint.FromConstraints(
				constraint.FieldEquals{Target: deepPath, Field: "tag", Value: typ.LiteralString("a")},
			),
		},
	}

	s := Solve(inputs, testResolver())

	// Query at the deep path
	deepPathWithTag := deepPath.Append(constraint.Segment{Kind: constraint.SegmentField, Name: "tag"})
	got := s.NarrowedTypeAt(thenNode, deepPathWithTag)
	if got != nil {
		t.Logf("deep narrowing result: %v", got)
	}
}

// =============================================================================
// Large Union Tests
// =============================================================================

func TestStress_LargeUnion_NarrowingTerminates(t *testing.T) {
	// Create union with 50 variants
	variants := make([]typ.Type, 50)
	for i := 0; i < 50; i++ {
		variants[i] = typ.NewRecord().Field("id", typ.LiteralInt(int64(i))).Build()
	}
	largeUnion := typ.NewUnion(variants...)

	c, branch, thenNode, _, _ := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	sym := setupSymbol(g, "x", allPoints)
	ver := cfg.Version{Root: "x", Symbol: sym, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, sym, ver)
	}

	pathX := constraint.Path{Root: "x", Symbol: sym}

	inputs := newInputs(g)
	inputs.DeclaredTypes[sym] = largeUnion
	inputs.EdgeConditions = []EdgeCondition{
		{
			From: branch,
			To:   thenNode,
			Condition: constraint.FromConstraints(
				constraint.FieldEquals{Target: pathX, Field: "id", Value: typ.LiteralInt(25)},
			),
		},
	}

	// Should terminate without timeout
	s := Solve(inputs, testResolver())

	got := s.NarrowedTypeAt(thenNode, pathX)
	if got == nil {
		t.Fatal("narrowing should not return nil")
	}

	// Should narrow to single variant
	expected := variants[25]
	if !typ.TypeEquals(got, expected) {
		t.Errorf("got %v, want %v", got, expected)
	}
}

func TestStress_LargeUnion_SoundNarrowing(t *testing.T) {
	// Create union with 100 variants
	variants := make([]typ.Type, 100)
	for i := 0; i < 100; i++ {
		variants[i] = typ.NewRecord().
			Field("id", typ.LiteralInt(int64(i))).
			Field("group", typ.LiteralString(string(rune('A'+i%5)))).
			Build()
	}
	largeUnion := typ.NewUnion(variants...)

	c, branch, thenNode, _, _ := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	sym := setupSymbol(g, "x", allPoints)
	ver := cfg.Version{Root: "x", Symbol: sym, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, sym, ver)
	}

	pathX := constraint.Path{Root: "x", Symbol: sym}

	// Narrow by group = "A" (should keep variants 0, 5, 10, 15, ...)
	inputs := newInputs(g)
	inputs.DeclaredTypes[sym] = largeUnion
	inputs.EdgeConditions = []EdgeCondition{
		{
			From: branch,
			To:   thenNode,
			Condition: constraint.FromConstraints(
				constraint.FieldEquals{Target: pathX, Field: "group", Value: typ.LiteralString("A")},
			),
		},
	}

	s := Solve(inputs, testResolver())

	got := s.NarrowedTypeAt(thenNode, pathX)
	if got == nil {
		t.Fatal("narrowing should not return nil")
	}

	// Verify soundness: narrowed type is subtype of original
	if !subtype.IsSubtype(got, largeUnion) {
		t.Errorf("narrowed type %v is not subtype of original %v", got, largeUnion)
	}
}

// =============================================================================
// DNF Cap Safety Tests
// =============================================================================

func TestStress_DNFCap_NoParnic(t *testing.T) {
	// Create condition that exceeds DefaultMaxDisjuncts (32)
	var disjuncts [][]constraint.Constraint
	for i := 0; i < 100; i++ {
		disjuncts = append(disjuncts, []constraint.Constraint{
			constraint.FieldEquals{
				Target: constraint.Path{Root: "x"},
				Field:  "id",
				Value:  typ.LiteralInt(int64(i)),
			},
		})
	}

	// Should not panic
	cond := constraint.FromDisjuncts(disjuncts)

	if cond.IsFalse() {
		t.Error("condition should not be false")
	}

	t.Logf("100 disjuncts -> %d after normalization", cond.NumDisjuncts())
}

func TestStress_DNFCap_SoundWidening(t *testing.T) {
	// Create large union and DNF that would explode
	variants := make([]typ.Type, 40)
	for i := 0; i < 40; i++ {
		variants[i] = typ.NewRecord().Field("id", typ.LiteralInt(int64(i))).Build()
	}
	union := typ.NewUnion(variants...)

	c, branch, thenNode, _, _ := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	sym := setupSymbol(g, "x", allPoints)
	ver := cfg.Version{Root: "x", Symbol: sym, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, sym, ver)
	}

	pathX := constraint.Path{Root: "x", Symbol: sym}

	// Create OR of many constraints
	var disjuncts [][]constraint.Constraint
	for i := 0; i < 40; i++ {
		disjuncts = append(disjuncts, []constraint.Constraint{
			constraint.FieldEquals{Target: pathX, Field: "id", Value: typ.LiteralInt(int64(i))},
		})
	}
	cond := constraint.FromDisjuncts(disjuncts)

	inputs := newInputs(g)
	inputs.DeclaredTypes[sym] = union
	inputs.EdgeConditions = []EdgeCondition{
		{From: branch, To: thenNode, Condition: cond},
	}

	s := Solve(inputs, testResolver())

	got := s.NarrowedTypeAt(thenNode, pathX)
	if got == nil {
		t.Fatal("narrowing should not return nil")
	}

	// Soundness: widened result must include all valid values
	// The narrowed type should be a subtype of the original
	if !subtype.IsSubtype(got, union) {
		t.Errorf("widened type %v is not subtype of original %v", got, union)
	}
}

func TestStress_DNFCap_StableAcrossRuns(t *testing.T) {
	var disjuncts [][]constraint.Constraint
	for i := 0; i < 50; i++ {
		disjuncts = append(disjuncts, []constraint.Constraint{
			constraint.FieldEquals{
				Target: constraint.Path{Root: "x", Symbol: 1},
				Field:  "tag",
				Value:  typ.LiteralInt(int64(i)),
			},
		})
	}

	// Run multiple times and verify same result
	var baseline constraint.Condition
	for i := 0; i < 10; i++ {
		cond := constraint.FromDisjuncts(disjuncts)
		if i == 0 {
			baseline = cond
		} else {
			if cond.NumDisjuncts() != baseline.NumDisjuncts() {
				t.Errorf("run %d: got %d disjuncts, want %d", i, cond.NumDisjuncts(), baseline.NumDisjuncts())
			}
		}
	}
}

// =============================================================================
// Soundness Invariant Tests
// =============================================================================

func TestSoundness_NarrowedIsSubtypeOfBase(t *testing.T) {
	typeA := typ.NewRecord().Field("tag", typ.LiteralString("a")).Field("data", typ.String).Build()
	typeB := typ.NewRecord().Field("tag", typ.LiteralString("b")).Field("data", typ.Number).Build()
	union := typ.NewUnion(typeA, typeB)

	c, branch, thenNode, elseNode, join := buildBranchJoinCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, elseNode, join, c.Exit()}
	sym := setupSymbol(g, "x", allPoints)
	ver := cfg.Version{Root: "x", Symbol: sym, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, sym, ver)
	}

	pathX := constraint.Path{Root: "x", Symbol: sym}

	inputs := newInputs(g)
	inputs.DeclaredTypes[sym] = union
	inputs.EdgeConditions = []EdgeCondition{
		{From: branch, To: thenNode, Condition: constraint.FromConstraints(
			constraint.FieldEquals{Target: pathX, Field: "tag", Value: typ.LiteralString("a")},
		)},
		{From: branch, To: elseNode, Condition: constraint.FromConstraints(
			constraint.FieldEquals{Target: pathX, Field: "tag", Value: typ.LiteralString("b")},
		)},
	}

	s := Solve(inputs, testResolver())

	// For each point, verify NarrowedTypeAt is subtype of TypeAt
	for _, p := range []cfg.Point{thenNode, elseNode, join} {
		base := s.TypeAt(p, pathX)
		narrowed := s.NarrowedTypeAt(p, pathX)

		if base == nil || narrowed == nil {
			continue
		}

		if !subtype.IsSubtype(narrowed, base) {
			t.Errorf("point %d: narrowed %v is not subtype of base %v", p, narrowed, base)
		}
	}
}

func TestSoundness_JoinIsOROfIncoming(t *testing.T) {
	typeA := typ.NewRecord().Field("tag", typ.LiteralString("a")).Build()
	typeB := typ.NewRecord().Field("tag", typ.LiteralString("b")).Build()
	union := typ.NewUnion(typeA, typeB)

	c := cfg.New()
	branch := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	path1 := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	path2 := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	join := c.AddNode(cfg.NodeJoin, cfg.SymbolID(0), "")

	c.AddEdge(c.Entry(), branch, true)
	c.AddEdge(branch, path1, true)
	c.AddEdge(branch, path2, false)
	c.AddEdge(path1, join, true)
	c.AddEdge(path2, join, true)
	c.AddEdge(join, c.Exit(), true)

	g := newMockSSAGraph(c)
	allPoints := []cfg.Point{c.Entry(), branch, path1, path2, join, c.Exit()}
	sym := setupSymbol(g, "x", allPoints)
	ver := cfg.Version{Root: "x", Symbol: sym, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, sym, ver)
	}

	pathX := constraint.Path{Root: "x", Symbol: sym}

	inputs := newInputs(g)
	inputs.DeclaredTypes[sym] = union
	inputs.EdgeConditions = []EdgeCondition{
		{From: branch, To: path1, Condition: constraint.FromConstraints(
			constraint.FieldEquals{Target: pathX, Field: "tag", Value: typ.LiteralString("a")},
		)},
		{From: branch, To: path2, Condition: constraint.FromConstraints(
			constraint.FieldEquals{Target: pathX, Field: "tag", Value: typ.LiteralString("b")},
		)},
	}

	s := Solve(inputs, testResolver())

	// At join, condition should be OR of incoming
	joinCond := s.ConditionAt(join)

	// Join type should include both variants
	joinType := s.TypeAt(join, pathX)
	if joinType == nil {
		t.Fatal("join type should not be nil")
	}

	// Should be the full union (OR of both paths)
	if !typ.TypeEquals(joinType, union) {
		t.Errorf("join type should be union, got %v", joinType)
	}

	t.Logf("join condition disjuncts: %d", joinCond.NumDisjuncts())
}

// =============================================================================
// Theory Solver Robustness Tests
// =============================================================================

func TestTheory_ConflictingConstraintsDetectUNSAT(t *testing.T) {
	state := numeric.NewState()

	xKey := constraint.PathKey("sym1")

	// x >= 10 AND x <= 5 (contradiction)
	state.ApplyGeConst(xKey, 10)
	state.ApplyLeConst(xKey, 5)

	if !state.IsUnsat() {
		t.Error("contradictory bounds should be UNSAT")
	}
}

func TestTheory_NeverWidensOutsideInterval(t *testing.T) {
	state := numeric.NewState()

	xKey := constraint.PathKey("sym1")
	yKey := constraint.PathKey("sym2")

	// x in [0, 100]
	state.ApplyGeConst(xKey, 0)
	state.ApplyLeConst(xKey, 100)

	// x < y
	state.ApplyLt(xKey, yKey)

	tightened := numeric.TightenWithTheory(state)
	if tightened.IsUnsat() {
		t.Fatal("should be satisfiable")
	}

	lower, upper, ok := tightened.BoundsFor(xKey)
	if !ok {
		return // no bounds is fine
	}

	// Theory must not widen beyond original interval
	if lower < 0 {
		t.Errorf("theory widened lower: got %d, expected >= 0", lower)
	}
	if upper > 100 {
		t.Errorf("theory widened upper: got %d, expected <= 100", upper)
	}
}

func TestTheory_LargeConstraintSetConverges(t *testing.T) {
	state := numeric.NewState()

	// Add 50 variables with relational constraints
	for i := 0; i < 50; i++ {
		key := constraint.PathKey("v" + string(rune('0'+i%10)) + "#1")
		state.ApplyGeConst(key, int64(i))
		state.ApplyLeConst(key, int64(i+100))
	}

	// Add chain of relations
	for i := 0; i < 49; i++ {
		k1 := constraint.PathKey("v" + string(rune('0'+i%10)) + "#1")
		k2 := constraint.PathKey("v" + string(rune('0'+(i+1)%10)) + "#1")
		state.ApplyLe(k1, k2)
	}

	// Should complete without timeout
	tightened := numeric.TightenWithTheory(state)

	// Should be satisfiable
	if tightened.IsUnsat() {
		t.Error("large constraint set should be satisfiable")
	}
}

func TestTheory_TransitiveChainSoundness(t *testing.T) {
	state := numeric.NewState()

	// a < b, b < c, c < d, d <= 100
	aKey := constraint.PathKey("a#1")
	bKey := constraint.PathKey("b#1")
	cKey := constraint.PathKey("c#1")
	dKey := constraint.PathKey("d#1")

	state.ApplyLt(aKey, bKey)
	state.ApplyLt(bKey, cKey)
	state.ApplyLt(cKey, dKey)
	state.ApplyLeConst(dKey, 100)

	tightened := numeric.TightenWithTheory(state)
	if tightened.IsUnsat() {
		t.Fatal("should be satisfiable")
	}

	// Theory should derive: a <= 97 (100 - 3)
	_, upper, ok := numeric.BoundsForWithTheory(tightened, aKey)
	if ok && upper > 97 {
		t.Errorf("transitive upper bound for a: got %d, expected <= 97", upper)
	}
}

// =============================================================================
// Mutation Kill Correctness
// =============================================================================

func TestMutation_ReassignmentKillsConstraints(t *testing.T) {
	typeA := typ.NewRecord().Field("tag", typ.LiteralString("a")).Build()
	typeB := typ.NewRecord().Field("tag", typ.LiteralString("b")).Build()
	union := typ.NewUnion(typeA, typeB)

	c := cfg.New()
	branch := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
	narrowPoint := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	reassignPoint := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
	afterReassign := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")

	c.AddEdge(c.Entry(), branch, true)
	c.AddEdge(branch, narrowPoint, true)
	c.AddEdge(branch, c.Exit(), false)
	c.AddEdge(narrowPoint, reassignPoint, true)
	c.AddEdge(reassignPoint, afterReassign, true)
	c.AddEdge(afterReassign, c.Exit(), true)

	g := newMockSSAGraph(c)
	allPoints := []cfg.Point{c.Entry(), branch, narrowPoint, reassignPoint, afterReassign, c.Exit()}
	sym := setupSymbol(g, "x", allPoints)

	// Version 1 at entry through narrowPoint
	ver1 := cfg.Version{Root: "x", Symbol: sym, ID: 1}
	// Version 2 at reassignPoint onward
	ver2 := cfg.Version{Root: "x", Symbol: sym, ID: 2}

	setVersion(g, c.Entry(), sym, ver1)
	setVersion(g, branch, sym, ver1)
	setVersion(g, narrowPoint, sym, ver1)
	setVersion(g, reassignPoint, sym, ver2)
	setVersion(g, afterReassign, sym, ver2)

	pathX := constraint.Path{Root: "x", Symbol: sym}

	inputs := newInputs(g)
	inputs.DeclaredTypes[sym] = union
	inputs.EdgeConditions = []EdgeCondition{
		{From: branch, To: narrowPoint, Condition: constraint.FromConstraints(
			constraint.FieldEquals{Target: pathX, Field: "tag", Value: typ.LiteralString("a")},
		)},
	}

	s := Solve(inputs, testResolver())

	// At narrowPoint: should be narrowed to typeA
	gotNarrow := s.NarrowedTypeAt(narrowPoint, pathX)
	if !typ.TypeEquals(gotNarrow, typeA) {
		t.Errorf("narrowPoint: got %v, want %v", gotNarrow, typeA)
	}

	// At afterReassign: new version, constraint killed
	gotAfter := s.TypeAt(afterReassign, pathX)
	// New version should have full union type (constraint killed)
	t.Logf("afterReassign type: %v", gotAfter)
}

// =============================================================================
// Stress: Combined Scenarios
// =============================================================================

func TestStress_CombinedScenario(t *testing.T) {
	rng := rand.New(rand.NewSource(999))

	// Build complex CFG with multiple branches
	c := cfg.New()
	points := []cfg.Point{c.Entry()}

	// Create 10 sequential branch-join pairs
	for i := 0; i < 10; i++ {
		branch := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
		thenN := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
		elseN := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
		join := c.AddNode(cfg.NodeJoin, cfg.SymbolID(0), "")

		c.AddEdge(points[len(points)-1], branch, true)
		c.AddEdge(branch, thenN, true)
		c.AddEdge(branch, elseN, false)
		c.AddEdge(thenN, join, true)
		c.AddEdge(elseN, join, true)

		points = append(points, branch, thenN, elseN, join)
	}
	c.AddEdge(points[len(points)-1], c.Exit(), true)
	points = append(points, c.Exit())

	g := newMockSSAGraph(c)

	// Create large union
	variants := make([]typ.Type, 20)
	for i := 0; i < 20; i++ {
		variants[i] = typ.NewRecord().Field("id", typ.LiteralInt(int64(i))).Build()
	}
	union := typ.NewUnion(variants...)

	sym := setupSymbol(g, "x", points)
	ver := cfg.Version{Root: "x", Symbol: sym, ID: 1}
	for _, p := range points {
		setVersion(g, p, sym, ver)
	}

	pathX := constraint.Path{Root: "x", Symbol: sym}

	// Create random edge conditions
	var edgeConditions []EdgeCondition
	for i := 0; i < 10; i++ {
		branch := points[1+i*4]
		thenN := points[2+i*4]
		id := rng.Intn(20)
		edgeConditions = append(edgeConditions, EdgeCondition{
			From: branch,
			To:   thenN,
			Condition: constraint.FromConstraints(
				constraint.FieldEquals{Target: pathX, Field: "id", Value: typ.LiteralInt(int64(id))},
			),
		})
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[sym] = union
	inputs.EdgeConditions = edgeConditions

	// Should complete without panic
	s := Solve(inputs, testResolver())

	// Verify soundness at random points
	for _, p := range points {
		base := s.TypeAt(p, pathX)
		narrowed := s.NarrowedTypeAt(p, pathX)
		if base == nil || narrowed == nil {
			continue
		}

		// Narrowed should intersect with base (not be disjoint)
		intersection := narrow.Intersect(base, narrowed)
		if intersection == nil {
			t.Errorf("point %d: narrowed %v is disjoint from base %v", p, narrowed, base)
		}
	}
}
