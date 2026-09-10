package assign

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/core"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/keyscoll"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/resolve"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

type preciseSourceSynthStub struct {
	preciseType typ.Type
}

func (s *preciseSourceSynthStub) TypeOf(expr ast.Expr, _ cfg.Point) typ.Type {
	switch expr.(type) {
	case *ast.LogicalOpExpr:
		return s.preciseType
	default:
		return typ.Unknown
	}
}

func (s *preciseSourceSynthStub) ExpandValues([]ast.Expr, int, cfg.Point) []typ.Type { return nil }

func (s *preciseSourceSynthStub) InferIterVars([]ast.Expr, int, cfg.Point) []typ.Type { return nil }

func (s *preciseSourceSynthStub) ExpandValuesWithSpecTypes([]ast.Expr, int, cfg.Point, api.SpecTypes) []typ.Type {
	return []typ.Type{typ.Any}
}

func (s *preciseSourceSynthStub) InferIterVarsWithSpecTypes([]ast.Expr, int, cfg.Point, api.SpecTypes) []typ.Type {
	return nil
}

func TestExtractAssignments_NilConfig(t *testing.T) {
	inputs := &flow.Inputs{
		Assignments:        []flow.UnifiedAssignment{},
		IndexerAssignments: []flow.IndexerAssignment{},
		SiblingAssignments: make(map[flow.SiblingKey]*flow.SiblingAssignment),
		PredicateLinks:     make(map[string]flow.PredicateLink),
	}
	fc := &core.FlowContext{}
	ExtractAssignments(fc, inputs, nil)
	if len(inputs.Assignments) != 0 {
		t.Errorf("expected no assignments with nil graph, got %d", len(inputs.Assignments))
	}
}

func TestExtractIterSource_NilArgs(t *testing.T) {
	result := resolve.ExtractIteratorSource(nil, 0, nil, nil, nil, nil)
	if result != nil {
		t.Errorf("expected nil for nil iterExprs, got %v", result)
	}
}

func TestExtractIterSource_EmptyExprs(t *testing.T) {
	result := resolve.ExtractIteratorSource([]ast.Expr{}, 0, nil, nil, nil, nil)
	if result != nil {
		t.Errorf("expected nil for empty iterExprs, got %v", result)
	}
}

func TestExtractIterSource_WithSynth(t *testing.T) {
	synth := func(expr ast.Expr, p cfg.Point) typ.Type {
		return typ.String
	}
	symResolver := func(p cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
		return nil, false
	}
	constResolver := func(name string) *flow.ConstValue {
		return nil
	}
	result := resolve.ExtractIteratorSource(
		[]ast.Expr{&ast.IdentExpr{Value: "test"}},
		0,
		synth,
		symResolver,
		constResolver,
		nil,
	)
	_ = result
}

func TestExtractFuncDefAssignments_NilConfig(t *testing.T) {
	inputs := &flow.Inputs{
		Assignments: []flow.UnifiedAssignment{},
	}
	fc := &core.FlowContext{}
	ExtractFuncDefAssignments(fc, inputs)
	if len(inputs.Assignments) != 0 {
		t.Errorf("expected no assignments with nil graph, got %d", len(inputs.Assignments))
	}
}

func TestExtractCallCorrelations_PassesPointToSymResolver(t *testing.T) {
	callInfo := &cfg.CallInfo{
		CalleeSymbol: 7,
	}
	wantPoint := cfg.Point(99)
	seenPoint := cfg.Point(0)
	symResolver := func(p cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
		seenPoint = p
		return typ.Integer, true
	}
	_, _, _ = extractCallCorrelations(callInfo, nil, wantPoint, symResolver, nil, nil, nil)
	if seenPoint != wantPoint {
		t.Fatalf("symResolver point = %d, want %d", seenPoint, wantPoint)
	}
}

func TestExtractCallCorrelations_MethodUsesCanonicalCalleeResolution(t *testing.T) {
	receiverSym := cfg.SymbolID(42)
	callInfo := &cfg.CallInfo{
		Method:         "receive",
		Receiver:       &ast.IdentExpr{Value: "ch"},
		ReceiverSymbol: receiverSym,
	}
	receiverType := typ.NewInterface("Channel", []typ.Method{
		{
			Name: "receive",
			Type: typ.Func().
				Param("self", typ.Self).
				Returns(typ.String, typ.NewOptional(typ.LuaError)).
				Build(),
		},
	})

	symResolver := func(_ cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
		if sym == receiverSym {
			return receiverType, true
		}
		return nil, false
	}

	inverse, co, _ := extractCallCorrelations(callInfo, nil, 1, symResolver, nil, nil, nil)
	if len(co) != 0 {
		t.Fatalf("expected no co-correlations, got %v", co)
	}
	if len(inverse) != 1 {
		t.Fatalf("expected one inverse correlation, got %v", inverse)
	}
	if inverse[0] != (flow.ReturnCorrelation{ValueIndex: 0, ErrorIndex: 1}) {
		t.Fatalf("unexpected inverse correlation: %+v", inverse[0])
	}
}

func TestIterSourceInfo_ZeroValue(t *testing.T) {
	var info resolve.IteratorSourceInfo
	if info.Path.Root != "" {
		t.Errorf("expected empty Path.Root for zero value")
	}
}

func TestExtractIterSource_WithBindings(t *testing.T) {
	bindings := &bind.BindingTable{}
	synth := func(expr ast.Expr, p cfg.Point) typ.Type {
		return typ.Unknown
	}
	symResolver := func(p cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
		return nil, false
	}
	constResolver := func(name string) *flow.ConstValue {
		return nil
	}
	result := resolve.ExtractIteratorSource(
		[]ast.Expr{&ast.IdentExpr{Value: "iter"}},
		1,
		synth,
		symResolver,
		constResolver,
		bindings,
	)
	_ = result
}

func TestExtractAssignments_ContainerElementSourceFromTrailingCall(t *testing.T) {
	code := `
		local ch = new_channel()
		local lead, ok, msg = "x", ch:receive()
	`
	chunk, err := parse.ParseString(code, "emit_container_trailing.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{Stmts: chunk}, "new_channel")
	exit := graph.Exit()
	chSym, ok := graph.SymbolAt(exit, "ch")
	if !ok || chSym == 0 {
		t.Fatal("expected symbol for ch")
	}
	msgSym, ok := graph.SymbolAt(exit, "msg")
	if !ok || msgSym == 0 {
		t.Fatal("expected symbol for msg")
	}

	elemType := typ.NewRecord().Field("id", typ.String).Build()
	receiveSpec := contract.NewSpec().WithEffects(effect.Return{
		ReturnIndex: 1,
		Transform:   effect.ElementOf{Source: effect.ParamRef{Index: 0}},
	})
	channelType := typ.NewInterface("Channel", []typ.Method{
		{
			Name: "receive",
			Type: typ.Func().
				Param("self", typ.Self).
				Returns(typ.Boolean, elemType).
				Spec(receiveSpec).
				Build(),
		},
	})
	symResolver := func(_ cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
		if sym == chSym {
			return channelType, true
		}
		return nil, false
	}

	inputs := &flow.Inputs{
		DeclaredTypes:      make(map[cfg.SymbolID]typ.Type),
		PredicateLinks:     make(map[string]flow.PredicateLink),
		SiblingAssignments: make(map[flow.SiblingKey]*flow.SiblingAssignment),
	}
	ExtractAssignments(&core.FlowContext{
		Graph: graph,
		Derived: &core.Derived{
			Synth: func(ast.Expr, cfg.Point) typ.Type {
				return typ.Unknown
			},
			SymResolver: symResolver,
		},
	}, inputs, nil)

	var msgAssign *flow.UnifiedAssignment
	for i := range inputs.Assignments {
		assign := &inputs.Assignments[i]
		if assign.TargetPath.Symbol == msgSym {
			msgAssign = assign
			break
		}
	}
	if msgAssign == nil {
		t.Fatal("expected assignment for msg")
	}
	if msgAssign.ContainerElementSource == nil {
		t.Fatal("expected container element source for trailing target from receive()")
	}
	if msgAssign.ContainerElementSource.ContainerPath.Symbol != chSym {
		t.Fatalf("container source symbol = %d, want %d", msgAssign.ContainerElementSource.ContainerPath.Symbol, chSym)
	}
	if msgAssign.ContainerElementSource.ReturnIndex != 1 {
		t.Fatalf("container return index = %d, want 1", msgAssign.ContainerElementSource.ReturnIndex)
	}
}

func TestExtractAssignments_KeysCollectorEffectFallbackIgnoresNonCollectorEffects(t *testing.T) {
	code := `
		local function passthrough(a, b)
			return b
		end
		local t1 = {}
		local t2 = {}
		local keys = passthrough(t1, t2)
	`
	chunk, err := parse.ParseString(code, "emit_keys_provenance_noncollector.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{Stmts: chunk}, "emit_keys_provenance_noncollector")
	exit := graph.Exit()
	keysSym, ok := graph.SymbolAt(exit, "keys")
	if !ok || keysSym == 0 {
		t.Fatal("expected symbol for keys")
	}

	inputs := &flow.Inputs{
		DeclaredTypes:      make(map[cfg.SymbolID]typ.Type),
		PredicateLinks:     make(map[string]flow.PredicateLink),
		SiblingAssignments: make(map[flow.SiblingKey]*flow.SiblingAssignment),
	}
	ExtractAssignments(&core.FlowContext{
		Graph: graph,
		Derived: &core.Derived{
			Synth: func(ast.Expr, cfg.Point) typ.Type {
				return typ.Unknown
			},
			RefinementBySym: func(cfg.SymbolID) *constraint.FunctionRefinement {
				// Non-collector effect (no KeyOf constraint).
				return &constraint.FunctionRefinement{}
			},
		},
	}, inputs, nil)

	if src, ok := inputs.KeysProvenance[keysSym]; ok && src != 0 {
		t.Fatalf("unexpected keys provenance for non-collector effect: keys sym %d -> %d", keysSym, src)
	}
}

func TestExtractAssignments_PrefersPreciseDirectTypeOverExpandedAnyForLogicalOr(t *testing.T) {
	code := `
		local left = nil
		local right = nil
		local ctx = left or right
	`
	chunk, err := parse.ParseString(code, "emit_precise_or.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{Stmts: chunk}, "emit_precise_or")
	exit := graph.Exit()
	ctxSym, ok := graph.SymbolAt(exit, "ctx")
	if !ok || ctxSym == 0 {
		t.Fatal("expected symbol for ctx")
	}

	contextAlias := typ.NewAlias("Context", typ.NewMap(typ.String, typ.Any))
	synthAPI := &preciseSourceSynthStub{preciseType: contextAlias}
	inputs := &flow.Inputs{
		DeclaredTypes:      make(map[cfg.SymbolID]typ.Type),
		PredicateLinks:     make(map[string]flow.PredicateLink),
		SiblingAssignments: make(map[flow.SiblingKey]*flow.SiblingAssignment),
	}
	ExtractAssignments(&core.FlowContext{
		Graph: graph,
		API:   synthAPI,
		Derived: &core.Derived{
			Synth: synthAPI.TypeOf,
			SymResolver: func(cfg.Point, cfg.SymbolID) (typ.Type, bool) {
				return nil, false
			},
		},
	}, inputs, nil)

	for _, assign := range inputs.Assignments {
		if assign.TargetPath.Symbol != ctxSym {
			continue
		}
		if !typ.TypeEquals(assign.Type, contextAlias) {
			t.Fatalf("ctx assignment type = %v, want %v", assign.Type, contextAlias)
		}
		return
	}

	t.Fatal("expected assignment for ctx")
}

func TestExtractAssignments_KeysCollectorEffectFallbackRespectsReturnIndex(t *testing.T) {
	code := `
		local function two_returns(tbl)
			return 0, 0
		end
		local t = {}
		local first, second = two_returns(t)
	`
	chunk, err := parse.ParseString(code, "emit_keys_provenance_return_index.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{Stmts: chunk}, "emit_keys_provenance_return_index")
	exit := graph.Exit()
	tSym, ok := graph.SymbolAt(exit, "t")
	if !ok || tSym == 0 {
		t.Fatal("expected symbol for t")
	}
	firstSym, ok := graph.SymbolAt(exit, "first")
	if !ok || firstSym == 0 {
		t.Fatal("expected symbol for first")
	}
	secondSym, ok := graph.SymbolAt(exit, "second")
	if !ok || secondSym == 0 {
		t.Fatal("expected symbol for second")
	}

	inputs := &flow.Inputs{
		DeclaredTypes:      make(map[cfg.SymbolID]typ.Type),
		PredicateLinks:     make(map[string]flow.PredicateLink),
		SiblingAssignments: make(map[flow.SiblingKey]*flow.SiblingAssignment),
	}
	ExtractAssignments(&core.FlowContext{
		Graph: graph,
		Derived: &core.Derived{
			Synth: func(ast.Expr, cfg.Point) typ.Type {
				return typ.Unknown
			},
			RefinementBySym: func(cfg.SymbolID) *constraint.FunctionRefinement {
				return &constraint.FunctionRefinement{
					OnReturn: constraint.FromConstraints(constraint.KeyOf{
						Table: constraint.ParamPath(0),
						Key:   constraint.RetPath(1),
					}),
				}
			},
		},
	}, inputs, nil)

	if src, ok := inputs.KeysProvenance[firstSym]; ok && src != 0 {
		t.Fatalf("unexpected keys provenance for first return target: %d -> %d", firstSym, src)
	}
	src, ok := inputs.KeysProvenance[secondSym]
	if !ok || src != tSym {
		t.Fatalf("expected keys provenance for second target %d -> %d, got %d (present=%v)", secondSym, tSym, src, ok)
	}
}

func TestExtractAssignments_KeysCollectorEffectFallback_TriesAllNameCandidates(t *testing.T) {
	code := `
		local t = {}
		local keys = collect_keys(t)
	`
	chunk, err := parse.ParseString(code, "emit_keys_provenance_name_candidates.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{Stmts: chunk}, "emit_keys_provenance_name_candidates")
	exit := graph.Exit()
	tSym, ok := graph.SymbolAt(exit, "t")
	if !ok || tSym == 0 {
		t.Fatal("expected symbol for t")
	}
	keysSym, ok := graph.SymbolAt(exit, "keys")
	if !ok || keysSym == 0 {
		t.Fatal("expected symbol for keys")
	}

	moduleBindings := bind.NewBindingTable()
	const mismatchSym cfg.SymbolID = 101
	const matchSym cfg.SymbolID = 102
	moduleBindings.SetName(mismatchSym, "collect_keys")
	moduleBindings.SetName(matchSym, "collect_keys")

	inputs := &flow.Inputs{
		DeclaredTypes:      make(map[cfg.SymbolID]typ.Type),
		PredicateLinks:     make(map[string]flow.PredicateLink),
		SiblingAssignments: make(map[flow.SiblingKey]*flow.SiblingAssignment),
	}
	ExtractAssignments(&core.FlowContext{
		Graph:          graph,
		ModuleBindings: moduleBindings,
		Derived: &core.Derived{
			Synth: func(ast.Expr, cfg.Point) typ.Type {
				return typ.Unknown
			},
			RefinementBySym: func(sym cfg.SymbolID) *constraint.FunctionRefinement {
				switch sym {
				case mismatchSym:
					return &constraint.FunctionRefinement{
						OnReturn: constraint.FromConstraints(constraint.KeyOf{
							Table: constraint.ParamPath(0),
							Key:   constraint.RetPath(1),
						}),
					}
				case matchSym:
					return &constraint.FunctionRefinement{
						OnReturn: constraint.FromConstraints(constraint.KeyOf{
							Table: constraint.ParamPath(0),
							Key:   constraint.RetPath(0),
						}),
					}
				default:
					return nil
				}
			},
		},
	}, inputs, nil)

	src, ok := inputs.KeysProvenance[keysSym]
	if !ok || src != tSym {
		t.Fatalf("expected keys provenance for target %d -> %d, got %d (present=%v)", keysSym, tSym, src, ok)
	}
}

func TestExtractAssignments_KeysCollector_WithFilterBranch(t *testing.T) {
	code := `
		local function sorted_keys(t)
			local keys = {}
			for k in pairs(t) do
				table.insert(keys, k)
			end
			table.sort(keys)
			return keys
		end

		local function filter_tests(entries, patterns)
			if not patterns or #patterns == 0 then
				return entries
			end
			local filtered = {}
			for _, entry in ipairs(entries) do
				for _, pattern in ipairs(patterns) do
					if entry.id:find(pattern, 1, true) then
						table.insert(filtered, entry)
						break
					end
				end
			end
			return filtered
		end

		local entries = {}
		local args = {}
		if args and #args > 0 then
			entries = filter_tests(entries, args)
		end

		local suites = {}
		local suite_names = sorted_keys(suites)
	`
	chunk, err := parse.ParseString(code, "emit_keys_provenance_filter_branch.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{Stmts: chunk}, "emit_keys_provenance_filter_branch")
	exit := graph.Exit()
	suitesSym, ok := graph.SymbolAt(exit, "suites")
	if !ok || suitesSym == 0 {
		t.Fatal("expected symbol for suites")
	}
	suiteNamesSym, ok := graph.SymbolAt(exit, "suite_names")
	if !ok || suiteNamesSym == 0 {
		t.Fatal("expected symbol for suite_names")
	}

	inputs := &flow.Inputs{
		DeclaredTypes:      make(map[cfg.SymbolID]typ.Type),
		PredicateLinks:     make(map[string]flow.PredicateLink),
		SiblingAssignments: make(map[flow.SiblingKey]*flow.SiblingAssignment),
	}
	ExtractAssignments(&core.FlowContext{
		Graph: graph,
		Derived: &core.Derived{
			Synth: func(ast.Expr, cfg.Point) typ.Type {
				return typ.Unknown
			},
		},
	}, inputs, keyscoll.BuildKeysCollectorDetector(graph, nil))

	src, ok := inputs.KeysProvenance[suiteNamesSym]
	if !ok || src != suitesSym {
		t.Fatalf("expected keys provenance for suite_names %d -> suites %d, got %d (present=%v)", suiteNamesSym, suitesSym, src, ok)
	}
}

func TestExtractAssignments_IndexAssign_NonIdentifierStringKey_UsesIndexStringSegment(t *testing.T) {
	code := `
		local t = {}
		local src = "v"
		t["x-y"] = src
	`
	chunk, err := parse.ParseString(code, "emit_index_string_key.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{Stmts: chunk}, "emit_index_string_key")
	exit := graph.Exit()
	tSym, ok := graph.SymbolAt(exit, "t")
	if !ok || tSym == 0 {
		t.Fatal("expected symbol for t")
	}

	inputs := &flow.Inputs{
		DeclaredTypes:      make(map[cfg.SymbolID]typ.Type),
		PredicateLinks:     make(map[string]flow.PredicateLink),
		SiblingAssignments: make(map[flow.SiblingKey]*flow.SiblingAssignment),
	}
	ExtractAssignments(&core.FlowContext{
		Graph: graph,
		Derived: &core.Derived{
			Synth: func(ast.Expr, cfg.Point) typ.Type {
				return typ.Unknown
			},
		},
	}, inputs, nil)

	var got *flow.UnifiedAssignment
	for i := range inputs.Assignments {
		assign := &inputs.Assignments[i]
		if assign.TargetPath.Symbol != tSym || len(assign.TargetPath.Segments) != 1 {
			continue
		}
		seg := assign.TargetPath.Segments[0]
		if seg.Name == "x-y" {
			got = assign
			break
		}
	}
	if got == nil {
		t.Fatal("expected assignment for t[\"x-y\"]")
	}
	seg := got.TargetPath.Segments[0]
	if seg.Kind != constraint.SegmentIndexString {
		t.Fatalf("expected SegmentIndexString, got %v", seg.Kind)
	}
}

func TestExtractAssignments_NestedDynamicIndex_LiftsToRootIndexer(t *testing.T) {
	code := `
		local subscribers = {}
		local cid = "c1"
		local sub_pid = "p1"

		if not subscribers[cid] then
			subscribers[cid] = {}
		end
		subscribers[cid][sub_pid] = true
	`

	chunk, err := parse.ParseString(code, "emit_nested_dynamic_index.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{Stmts: chunk}, "emit_nested_dynamic_index")
	exit := graph.Exit()
	subscribersSym, ok := graph.SymbolAt(exit, "subscribers")
	if !ok || subscribersSym == 0 {
		t.Fatal("expected symbol for subscribers")
	}
	cidSym, ok := graph.SymbolAt(exit, "cid")
	if !ok || cidSym == 0 {
		t.Fatal("expected symbol for cid")
	}

	inputs := &flow.Inputs{
		DeclaredTypes:      make(map[cfg.SymbolID]typ.Type),
		PredicateLinks:     make(map[string]flow.PredicateLink),
		SiblingAssignments: make(map[flow.SiblingKey]*flow.SiblingAssignment),
	}
	ExtractAssignments(&core.FlowContext{
		Graph: graph,
		Derived: &core.Derived{
			Synth: func(ast.Expr, cfg.Point) typ.Type {
				return typ.Unknown
			},
		},
	}, inputs, nil)

	var lifted *flow.IndexerAssignment
	for i := range inputs.IndexerAssignments {
		assign := &inputs.IndexerAssignments[i]
		if assign.Symbol != subscribersSym || assign.KeySymbol != cidSym || len(assign.Segments) != 0 {
			continue
		}
		if _, ok := assign.ValType.(*typ.Map); ok {
			lifted = assign
			break
		}
	}
	if lifted == nil {
		t.Fatalf("expected lifted indexer assignment for nested dynamic write, got %#v", inputs.IndexerAssignments)
	}
}

func TestCorrelationsFromFunctionType_NoImplicitErrorConvention(t *testing.T) {
	fnType := typ.Func().
		Returns(typ.NewOptional(typ.String), typ.NewOptional(typ.Number)).
		Build()
	inverse, co := correlationsFromFunctionType(fnType)
	if len(inverse) != 0 || len(co) != 0 {
		t.Fatalf("expected no implicit correlations without explicit spec effects, got inverse=%v co=%v", inverse, co)
	}
}

func TestCorrelationsFromFunctionType_ExplicitErrorReturn(t *testing.T) {
	fnType := typ.Func().
		Returns(typ.String, typ.NewOptional(typ.LuaError)).
		Spec(contract.NewSpec().WithEffects(effect.ErrorReturn{
			ValueIndex: 0,
			ErrorIndex: 1,
		})).
		Build()
	inverse, co := correlationsFromFunctionType(fnType)
	if len(co) != 0 {
		t.Fatalf("expected no co-correlations, got %v", co)
	}
	if len(inverse) != 1 {
		t.Fatalf("expected one explicit error correlation, got %v", inverse)
	}
	if inverse[0] != (flow.ReturnCorrelation{ValueIndex: 0, ErrorIndex: 1}) {
		t.Fatalf("unexpected correlation: %+v", inverse[0])
	}
}

func TestCorrelationsFromFunctionType_ImplicitLuaErrorConvention(t *testing.T) {
	fnType := typ.Func().
		Returns(typ.NewOptional(typ.String), typ.NewOptional(typ.LuaError)).
		Build()
	inverse, co := correlationsFromFunctionType(fnType)
	if len(co) != 0 {
		t.Fatalf("expected no co-correlations, got %v", co)
	}
	if len(inverse) != 1 {
		t.Fatalf("expected one convention-based correlation, got %v", inverse)
	}
	if inverse[0] != (flow.ReturnCorrelation{ValueIndex: 0, ErrorIndex: 1}) {
		t.Fatalf("unexpected convention correlation: %+v", inverse[0])
	}
}

func TestCorrelationsFromFunctionType_ImplicitStringErrorConvention(t *testing.T) {
	fnType := typ.Func().
		Returns(typ.NewOptional(typ.String), typ.NewOptional(typ.String)).
		Build()
	inverse, co := correlationsFromFunctionType(fnType)
	if len(co) != 0 {
		t.Fatalf("expected no co-correlations, got %v", co)
	}
	if len(inverse) != 1 {
		t.Fatalf("expected one convention-based correlation, got %v", inverse)
	}
	if inverse[0] != (flow.ReturnCorrelation{ValueIndex: 0, ErrorIndex: 1}) {
		t.Fatalf("unexpected convention correlation: %+v", inverse[0])
	}
}

func TestCorrelationsFromFunctionType_ImplicitStructuredErrorConvention(t *testing.T) {
	errorType := typ.NewRecord().
		Field("status_code", typ.Number).
		Field("message", typ.String).
		Build()
	fnType := typ.Func().
		Returns(typ.NewOptional(typ.String), typ.NewOptional(errorType)).
		Build()
	inverse, co := correlationsFromFunctionType(fnType)
	if len(co) != 0 {
		t.Fatalf("expected no co-correlations, got %v", co)
	}
	if len(inverse) != 1 {
		t.Fatalf("expected one convention-based correlation, got %v", inverse)
	}
	if inverse[0] != (flow.ReturnCorrelation{ValueIndex: 0, ErrorIndex: 1}) {
		t.Fatalf("unexpected convention correlation: %+v", inverse[0])
	}
}

func TestCorrelationsFromFunctionType_ImplicitUnionErrorConvention(t *testing.T) {
	errorType := typ.NewRecord().
		Field("message", typ.String).
		Build()
	fnType := typ.Func().
		Returns(typ.NewOptional(typ.String), typ.NewOptional(typ.NewUnion(typ.String, typ.LuaError, errorType))).
		Build()
	inverse, co := correlationsFromFunctionType(fnType)
	if len(co) != 0 {
		t.Fatalf("expected no co-correlations, got %v", co)
	}
	if len(inverse) != 1 {
		t.Fatalf("expected one convention-based correlation, got %v", inverse)
	}
	if inverse[0] != (flow.ReturnCorrelation{ValueIndex: 0, ErrorIndex: 1}) {
		t.Fatalf("unexpected convention correlation: %+v", inverse[0])
	}
}

func TestCorrelationsFromFunctionType_NoImplicitStructuredErrorWithoutMessage(t *testing.T) {
	auxType := typ.NewRecord().
		Field("status_code", typ.Number).
		Build()
	fnType := typ.Func().
		Returns(typ.NewOptional(typ.String), typ.NewOptional(auxType)).
		Build()
	inverse, co := correlationsFromFunctionType(fnType)
	if len(inverse) != 0 || len(co) != 0 {
		t.Fatalf("expected no implicit correlations without message-like error slot, got inverse=%v co=%v", inverse, co)
	}
}

func TestGuardedTypeCorrelationsFromCall_CallbackReturnOnTruthy(t *testing.T) {
	fnType := typ.Func().
		Param("f", typ.Any).
		Returns(typ.Boolean, typ.Any).
		Spec(contract.NewSpec().WithEffects(effect.Return{
			ReturnIndex: 1,
			Transform:   effect.CallbackReturn{CallbackParam: effect.ParamRef{Index: 0}},
		})).
		Build()

	callInfo := &cfg.CallInfo{
		Args: []ast.Expr{&ast.FunctionExpr{}},
	}
	synth := func(ast.Expr, cfg.Point) typ.Type {
		return typ.Func().Returns(typ.String).Build()
	}

	got := guardedTypeCorrelationsFromCall(fnType, callInfo, synth, 1)
	if len(got) != 1 {
		t.Fatalf("expected one guarded correlation, got %v", got)
	}
	if got[0].GuardIndex != 0 || got[0].TargetIndex != 1 || !got[0].GuardOnTruthy {
		t.Fatalf("unexpected guarded correlation shape: %+v", got[0])
	}
	if !typ.TypeEquals(got[0].TargetType, typ.String) {
		t.Fatalf("expected guarded target type string, got %v", got[0].TargetType)
	}
}
