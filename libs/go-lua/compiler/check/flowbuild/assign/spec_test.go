package assign

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/typ"
)

func buildEmptyGraph() *cfg.Graph {
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{}}
	return cfg.Build(fn)
}

func TestCollectSpecNarrowedTypes_NilGraph(t *testing.T) {
	result := CollectSpecNarrowedTypes(nil, nil, nil, nil, nil, nil)
	if result == nil {
		t.Error("expected non-nil result for nil graph")
	}
	if len(result) != 0 {
		t.Errorf("expected empty result for nil graph, got %d entries", len(result))
	}
}

func TestCollectSpecNarrowedTypes_EmptyGraph(t *testing.T) {
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{}}
	graph := cfg.Build(fn)
	scopes := make(map[cfg.Point]*scope.State)
	result := CollectSpecNarrowedTypes(graph, scopes, nil, nil, nil, nil)
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestBuildReceiverDependencies_NilGraph(t *testing.T) {
	result := BuildReceiverDependencies(nil)
	if result == nil {
		t.Error("expected non-nil result for nil graph")
	}
	if len(result) != 0 {
		t.Errorf("expected empty result for nil graph, got %d entries", len(result))
	}
}

func TestBuildReceiverDependencies_EmptyGraph(t *testing.T) {
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{}}
	graph := cfg.Build(fn)
	result := BuildReceiverDependencies(graph)
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestNarrowReturnTypeBySpec_NilCallInfo(t *testing.T) {
	result := NarrowReturnTypeBySpec(nil, nil, nil, 0, nil, nil, nil, nil)
	if result != nil {
		t.Errorf("expected nil for nil callInfo, got %v", result)
	}
}

func TestNarrowReturnTypeBySpec_NilSynth(t *testing.T) {
	callInfo := &cfg.CallInfo{}
	result := NarrowReturnTypeBySpec(callInfo, nil, nil, 0, nil, nil, nil, nil)
	if result != nil {
		t.Errorf("expected nil for nil synth, got %v", result)
	}
}

func TestNarrowReturnTypeBySpec_WithSynth(t *testing.T) {
	callInfo := &cfg.CallInfo{
		Callee: &ast.IdentExpr{Value: "fn"},
	}
	synth := func(expr ast.Expr, p cfg.Point) typ.Type {
		return typ.String
	}
	result := NarrowReturnTypeBySpec(callInfo, nil, synth, 0, nil, nil, nil, nil)
	// String type has no spec, so result should be nil
	if result != nil {
		t.Errorf("expected nil for type without spec, got %v", result)
	}
}

func TestNarrowReturnTypeBySpec_WithSymResolver(t *testing.T) {
	callInfo := &cfg.CallInfo{
		CalleeSymbol: 1,
	}
	synth := func(expr ast.Expr, p cfg.Point) typ.Type {
		return nil
	}
	symResolver := func(p cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
		return typ.Integer, true
	}
	result := NarrowReturnTypeBySpec(callInfo, nil, synth, 0, symResolver, nil, nil, nil)
	// Integer type has no spec, so result should be nil
	if result != nil {
		t.Errorf("expected nil for type without spec, got %v", result)
	}
}

func TestNarrowReturnTypeBySpec_PassesPointToSymResolver(t *testing.T) {
	callInfo := &cfg.CallInfo{
		CalleeSymbol: 1,
	}
	synth := func(expr ast.Expr, p cfg.Point) typ.Type {
		return nil
	}
	wantPoint := cfg.Point(42)
	seenPoint := cfg.Point(0)
	symResolver := func(p cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
		seenPoint = p
		return typ.Integer, true
	}
	_ = NarrowReturnTypeBySpec(callInfo, nil, synth, wantPoint, symResolver, nil, nil, nil)
	if seenPoint != wantPoint {
		t.Fatalf("symResolver point = %d, want %d", seenPoint, wantPoint)
	}
}

func TestNarrowReturnTypeBySpec_UsesCalleeNameCandidateSymbols(t *testing.T) {
	spec := contract.NewSpec().
		WithReturnCase(
			constraint.FromConstraints(constraint.FieldEquals{
				Target: constraint.Path{Root: "$1"},
				Field:  "message",
				Value:  typ.LiteralBool(true),
			}),
			typ.String,
		).
		WithDefaultReturn(typ.Any)
	fnType := typ.Func().
		Param("topic", typ.String).
		OptParam("opts", typ.NewRecord().OptField("message", typ.Boolean).Build()).
		Returns(typ.Any).
		Spec(spec).
		Build()

	callInfo := &cfg.CallInfo{
		CalleeName: "listen",
		Args: []ast.Expr{
			&ast.StringExpr{Value: "increment"},
			&ast.TableExpr{
				Fields: []*ast.Field{{
					Key:   &ast.IdentExpr{Value: "message"},
					Value: &ast.TrueExpr{},
				}},
			},
		},
	}

	const listenSym cfg.SymbolID = 77
	bindings := bind.NewBindingTable()
	bindings.SetName(listenSym, "listen")

	synth := func(ast.Expr, cfg.Point) typ.Type { return nil }
	symResolver := func(_ cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
		if sym == listenSym {
			return fnType, true
		}
		return nil, false
	}

	result := NarrowReturnTypeBySpec(callInfo, nil, synth, 0, symResolver, nil, bindings, nil)
	if !typ.TypeEquals(result, typ.String) {
		t.Fatalf("expected spec-narrowed string via callee-name candidate, got %v", result)
	}
}

func TestNarrowReturnTypeBySpec_PrefersResolvableBindingCandidateOverRaw(t *testing.T) {
	spec := contract.NewSpec().
		WithReturnCase(
			constraint.FromConstraints(constraint.FieldEquals{
				Target: constraint.Path{Root: "$1"},
				Field:  "message",
				Value:  typ.LiteralBool(true),
			}),
			typ.String,
		).
		WithDefaultReturn(typ.Any)
	fnType := typ.Func().
		Param("topic", typ.String).
		OptParam("opts", typ.NewRecord().OptField("message", typ.Boolean).Build()).
		Returns(typ.Any).
		Spec(spec).
		Build()

	callee := &ast.IdentExpr{Value: "listen"}
	const (
		rawSym    cfg.SymbolID = 900
		listenSym cfg.SymbolID = 901
	)
	bindings := bind.NewBindingTable()
	bindings.Bind(callee, listenSym)

	callInfo := &cfg.CallInfo{
		Callee:       callee,
		CalleeSymbol: rawSym,
		CalleeName:   "listen",
		Args: []ast.Expr{
			&ast.StringExpr{Value: "increment"},
			&ast.TableExpr{
				Fields: []*ast.Field{{
					Key:   &ast.IdentExpr{Value: "message"},
					Value: &ast.TrueExpr{},
				}},
			},
		},
	}

	synth := func(ast.Expr, cfg.Point) typ.Type { return nil }
	symResolver := func(_ cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
		if sym == listenSym {
			return fnType, true
		}
		return nil, false
	}

	result := NarrowReturnTypeBySpec(callInfo, nil, synth, 0, symResolver, nil, bindings, nil)
	if !typ.TypeEquals(result, typ.String) {
		t.Fatalf("expected spec-narrowed string via binding candidate, got %v", result)
	}
}

func TestCollectSpecNarrowedTypes_MultiReturnTrailingTarget(t *testing.T) {
	code := `
		local obj = make()
		local ok, msg = obj:receive()
		local from = msg:from()
	`
	chunk, err := parse.ParseString(code, "spec_multi_return.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{Stmts: chunk}, "make")
	exit := graph.Exit()
	symObj, ok := graph.SymbolAt(exit, "obj")
	if !ok || symObj == 0 {
		t.Fatal("expected symbol for obj")
	}
	symMsg, ok := graph.SymbolAt(exit, "msg")
	if !ok || symMsg == 0 {
		t.Fatal("expected symbol for msg")
	}
	symFrom, ok := graph.SymbolAt(exit, "from")
	if !ok || symFrom == 0 {
		t.Fatal("expected symbol for from")
	}

	msgType := typ.NewRecord().
		Field("id", typ.String).
		Build()
	objType := typ.NewInterface("Obj", []typ.Method{
		{
			Name: "receive",
			Type: typ.Func().
				Param("self", typ.Self).
				Returns(typ.Boolean, msgType).
				Build(),
		},
	})
	makeType := typ.Func().
		Returns(objType).
		Spec(contract.NewSpec().WithReturnCase(constraint.TrueCondition(), objType)).
		Build()

	synth := func(expr ast.Expr, _ cfg.Point) typ.Type {
		if ident, ok := expr.(*ast.IdentExpr); ok && ident.Value == "make" {
			return makeType
		}
		return typ.Unknown
	}
	synthAPI := &specTestSynthAPI{
		graph:   graph,
		msgType: msgType,
	}

	result := CollectSpecNarrowedTypes(graph, map[cfg.Point]*scope.State{}, synth, nil, synthAPI, nil)
	if !typ.TypeEquals(result[symObj], objType) {
		t.Fatalf("expected obj type to be inferred, got %v", result[symObj])
	}
	if !typ.TypeEquals(result[symMsg], msgType) {
		t.Fatalf("expected trailing target msg type to be inferred, got %v", result[symMsg])
	}
	if !typ.TypeEquals(result[symFrom], typ.String) {
		t.Fatalf("expected dependent from type to be inferred, got %v", result[symFrom])
	}
}

func TestCollectSpecNarrowedTypes_ReprocessesPointWhenNewReceiverBecomesKnown(t *testing.T) {
	code := `
		local peer
		local root = make()
		local a, b = root:a(), peer:b()
		peer = a:peer()
	`
	chunk, err := parse.ParseString(code, "spec_reprocess_point.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	graph := cfg.Build(&ast.FunctionExpr{Stmts: chunk}, "make")
	exit := graph.Exit()

	symRoot, ok := graph.SymbolAt(exit, "root")
	if !ok || symRoot == 0 {
		t.Fatal("expected symbol for root")
	}
	symA, ok := graph.SymbolAt(exit, "a")
	if !ok || symA == 0 {
		t.Fatal("expected symbol for a")
	}
	symB, ok := graph.SymbolAt(exit, "b")
	if !ok || symB == 0 {
		t.Fatal("expected symbol for b")
	}
	symPeer, ok := graph.SymbolAt(exit, "peer")
	if !ok || symPeer == 0 {
		t.Fatal("expected symbol for peer")
	}

	typeRoot := typ.NewRecord().Field("kind", typ.LiteralString("root")).Build()
	typeA := typ.NewRecord().Field("kind", typ.LiteralString("a")).Build()
	typePeer := typ.NewRecord().Field("kind", typ.LiteralString("peer")).Build()
	typeB := typ.NewRecord().Field("kind", typ.LiteralString("b")).Build()

	makeType := typ.Func().
		Returns(typeRoot).
		Spec(contract.NewSpec().WithReturnCase(constraint.TrueCondition(), typeRoot)).
		Build()

	synth := func(expr ast.Expr, _ cfg.Point) typ.Type {
		if ident, ok := expr.(*ast.IdentExpr); ok && ident.Value == "make" {
			return makeType
		}
		return typ.Unknown
	}

	synthAPI := &reprocessSpecSynthAPI{
		graph:    graph,
		rootSym:  symRoot,
		aSym:     symA,
		peerSym:  symPeer,
		typeA:    typeA,
		typePeer: typePeer,
		typeB:    typeB,
	}

	result := CollectSpecNarrowedTypes(graph, map[cfg.Point]*scope.State{}, synth, nil, synthAPI, nil)
	if !typ.TypeEquals(result[symRoot], typeRoot) {
		t.Fatalf("expected root type to be inferred, got %v", result[symRoot])
	}
	if !typ.TypeEquals(result[symA], typeA) {
		t.Fatalf("expected a type to be inferred, got %v", result[symA])
	}
	if !typ.TypeEquals(result[symPeer], typePeer) {
		t.Fatalf("expected peer type to be inferred, got %v", result[symPeer])
	}
	if !typ.TypeEquals(result[symB], typeB) {
		t.Fatalf("expected b type to be inferred after point reprocessing, got %v", result[symB])
	}
}

type specTestSynthAPI struct {
	graph   *cfg.Graph
	msgType typ.Type
}

func (s *specTestSynthAPI) TypeOf(_ ast.Expr, _ cfg.Point) typ.Type {
	return typ.Unknown
}

func (s *specTestSynthAPI) ExpandValues(_ []ast.Expr, needed int, _ cfg.Point) []typ.Type {
	return make([]typ.Type, needed)
}

func (s *specTestSynthAPI) InferIterVars(_ []ast.Expr, count int, _ cfg.Point) []typ.Type {
	return make([]typ.Type, count)
}

func (s *specTestSynthAPI) ExpandValuesWithSpecTypes(_ []ast.Expr, needed int, p cfg.Point, specTypes api.SpecTypes) []typ.Type {
	values := make([]typ.Type, needed)
	if s == nil || s.graph == nil {
		return values
	}
	info := s.graph.Assign(p)
	if info == nil {
		return values
	}
	for i := 0; i < needed; i++ {
		call, retIdx := info.CallForTarget(i)
		if call == nil {
			continue
		}
		switch call.Method {
		case "receive":
			if call.ReceiverSymbol == 0 {
				continue
			}
			if _, ok := specTypes[call.ReceiverSymbol]; !ok {
				continue
			}
			switch retIdx {
			case 0:
				values[i] = typ.Boolean
			case 1:
				values[i] = s.msgType
			}
		case "from":
			if call.ReceiverSymbol == 0 {
				continue
			}
			if _, ok := specTypes[call.ReceiverSymbol]; !ok {
				continue
			}
			if retIdx == 0 {
				values[i] = typ.String
			}
		}
	}
	return values
}

func (s *specTestSynthAPI) InferIterVarsWithSpecTypes(_ []ast.Expr, count int, _ cfg.Point, _ api.SpecTypes) []typ.Type {
	return make([]typ.Type, count)
}

type reprocessSpecSynthAPI struct {
	graph                  *cfg.Graph
	rootSym, aSym, peerSym cfg.SymbolID
	typeA, typePeer, typeB typ.Type
}

func (s *reprocessSpecSynthAPI) TypeOf(_ ast.Expr, _ cfg.Point) typ.Type {
	return typ.Unknown
}

func (s *reprocessSpecSynthAPI) ExpandValues(_ []ast.Expr, needed int, _ cfg.Point) []typ.Type {
	return make([]typ.Type, needed)
}

func (s *reprocessSpecSynthAPI) InferIterVars(_ []ast.Expr, count int, _ cfg.Point) []typ.Type {
	return make([]typ.Type, count)
}

func (s *reprocessSpecSynthAPI) ExpandValuesWithSpecTypes(_ []ast.Expr, needed int, p cfg.Point, specTypes api.SpecTypes) []typ.Type {
	values := make([]typ.Type, needed)
	if s == nil || s.graph == nil {
		return values
	}
	info := s.graph.Assign(p)
	if info == nil {
		return values
	}
	for i := 0; i < needed; i++ {
		call, retIdx := info.CallForTarget(i)
		if call == nil || retIdx != 0 {
			continue
		}
		switch call.Method {
		case "a":
			if _, ok := specTypes[s.rootSym]; ok {
				values[i] = s.typeA
			}
		case "peer":
			if _, ok := specTypes[s.aSym]; ok {
				values[i] = s.typePeer
			}
		case "b":
			if _, ok := specTypes[s.peerSym]; ok {
				values[i] = s.typeB
			}
		}
	}
	return values
}

func (s *reprocessSpecSynthAPI) InferIterVarsWithSpecTypes(_ []ast.Expr, count int, _ cfg.Point, _ api.SpecTypes) []typ.Type {
	return make([]typ.Type, count)
}
