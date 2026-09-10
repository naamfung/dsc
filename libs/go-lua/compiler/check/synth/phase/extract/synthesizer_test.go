package extract

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

type mockTypeQuerier struct{}

func (m mockTypeQuerier) Field(_ *db.QueryContext, t typ.Type, name string) (typ.Type, bool) {
	if rec, ok := t.(*typ.Record); ok {
		if f := rec.GetField(name); f != nil {
			return f.Type, true
		}
	}
	return nil, false
}

func (m mockTypeQuerier) Index(_ *db.QueryContext, t typ.Type, key typ.Type) (typ.Type, bool) {
	if arr, ok := t.(*typ.Array); ok {
		return arr.Element, true
	}
	return nil, false
}

func (m mockTypeQuerier) Method(_ *db.QueryContext, t typ.Type, name string) (typ.Type, bool) {
	return nil, false
}

func (m mockTypeQuerier) BinaryOp(_ *db.QueryContext, left typ.Type, op string, right typ.Type) typ.Type {
	return typ.Number
}

func (m mockTypeQuerier) UnaryOp(_ *db.QueryContext, op string, operand typ.Type) typ.Type {
	if op == "#" {
		return typ.Integer
	}
	return typ.Number
}

func (m mockTypeQuerier) IsSubtype(_ *db.QueryContext, sub, super typ.Type) bool {
	return false
}

func (m mockTypeQuerier) ExpandInstantiated(_ *db.QueryContext, t typ.Type) typ.Type {
	return t
}

func (m mockTypeQuerier) Widen(_ *db.QueryContext, t typ.Type) typ.Type {
	return t
}

func (m mockTypeQuerier) WidenForInference(_ *db.QueryContext, t typ.Type) typ.Type {
	return t
}

type mockGraph struct {
	symbols map[string]cfg.SymbolID
}

func (m mockGraph) ID() uint64                                                  { return 0 }
func (m mockGraph) Entry() cfg.Point                                            { return 0 }
func (m mockGraph) Exit() cfg.Point                                             { return 0 }
func (m mockGraph) Node(p cfg.Point) *cfg.Node                                  { return nil }
func (m mockGraph) RPO() []cfg.Point                                            { return nil }
func (m mockGraph) Predecessors(p cfg.Point) []cfg.Point                        { return nil }
func (m mockGraph) Successor(p cfg.Point) cfg.Point                             { return 0 }
func (m mockGraph) Successors(p cfg.Point) []cfg.Point                          { return nil }
func (m mockGraph) Edges() []cfg.Edge                                           { return nil }
func (m mockGraph) Size() int                                                   { return 0 }
func (m mockGraph) EdgeCond(from, to cfg.Point) (bool, bool)                    { return false, false }
func (m mockGraph) IsJoin(p cfg.Point) bool                                     { return false }
func (m mockGraph) IsBranch(p cfg.Point) bool                                   { return false }
func (m mockGraph) PhiNodes() []cfg.PhiNode                                     { return nil }
func (m mockGraph) VisibleVersion(p cfg.Point, sym cfg.SymbolID) cfg.Version    { return cfg.Version{} }
func (m mockGraph) AllVisibleVersions(p cfg.Point) map[cfg.SymbolID]cfg.Version { return nil }
func (m mockGraph) SymbolAt(p cfg.Point, name string) (cfg.SymbolID, bool) {
	if m.symbols == nil {
		return 0, false
	}
	sym, ok := m.symbols[name]
	return sym, ok
}
func (m mockGraph) AllSymbolsAt(p cfg.Point) map[string]cfg.SymbolID { return m.symbols }
func (m mockGraph) DefVersionAt(p cfg.Point, sym cfg.SymbolID) (cfg.Version, bool) {
	return cfg.Version{}, false
}
func (m mockGraph) DeclarationPoint(sym cfg.SymbolID) (cfg.Point, bool) { return 0, false }
func (m mockGraph) ParamNames() []string                                { return nil }
func (m mockGraph) ParamSymbols() []cfg.SymbolID                        { return nil }
func (m mockGraph) ParamDeclPoints() []cfg.Point                        { return nil }
func (m mockGraph) NameOf(sym cfg.SymbolID) string {
	for name, s := range m.symbols {
		if s == sym {
			return name
		}
	}
	return ""
}
func (m mockGraph) SymbolKind(sym cfg.SymbolID) (cfg.SymbolKind, bool) {
	return cfg.SymbolUnknown, false
}

func newTestSynthesizer() *Synthesizer {
	deps := &Deps{
		Ctx:      db.NewQueryContext(db.New()),
		Types:    mockTypeQuerier{},
		Scopes:   make(api.ScopeMap),
		PreCache: make(api.Cache),
	}
	return NewSynthesizer(deps, api.PhaseTypeResolution)
}

func newTestSynthesizerWithSymbol(name string, t typ.Type) (*Synthesizer, *ast.IdentExpr) {
	ident := &ast.IdentExpr{Value: name}
	const sym = cfg.SymbolID(1)

	bindings := bind.NewBindingTable()
	bindings.Bind(ident, sym)

	graph := mockGraph{symbols: map[string]cfg.SymbolID{name: sym}}
	declared := flow.DeclaredTypes{sym: t}
	checkCtx := api.NewDeclaredEnv(api.DeclaredEnvConfig{
		Graph:         graph,
		Bindings:      bindings,
		DeclaredTypes: declared,
	})

	deps := &Deps{
		Ctx:      db.NewQueryContext(db.New()),
		Types:    mockTypeQuerier{},
		Scopes:   make(api.ScopeMap),
		CheckCtx: checkCtx,
		PreCache: make(api.Cache),
	}
	return NewSynthesizer(deps, api.PhaseTypeResolution), ident
}

func TestSynthesizer_TypeOf_Nil(t *testing.T) {
	s := newTestSynthesizer()
	result := s.TypeOf(nil, 0)
	if result != typ.Nil {
		t.Fatalf("got %v, want nil", result)
	}
}

func TestSynthesizer_TypeOf_NilExpr(t *testing.T) {
	s := newTestSynthesizer()
	result := s.TypeOf(&ast.NilExpr{}, 0)
	if result != typ.Nil {
		t.Fatalf("got %v, want nil", result)
	}
}

func TestSynthesizer_TypeOf_True(t *testing.T) {
	s := newTestSynthesizer()
	result := s.TypeOf(&ast.TrueExpr{}, 0)
	if result != typ.True {
		t.Fatalf("got %v, want true", result)
	}
}

func TestSynthesizer_TypeOf_False(t *testing.T) {
	s := newTestSynthesizer()
	result := s.TypeOf(&ast.FalseExpr{}, 0)
	if result != typ.False {
		t.Fatalf("got %v, want false", result)
	}
}

func TestSynthesizer_TypeOf_Integer(t *testing.T) {
	s := newTestSynthesizer()
	result := s.TypeOf(&ast.NumberExpr{Value: "42"}, 0)
	lit, ok := result.(*typ.Literal)
	if !ok {
		t.Fatalf("got %T, want literal", result)
	}
	if lit.Value != int64(42) {
		t.Fatalf("got %v, want 42", lit.Value)
	}
}

func TestSynthesizer_TypeOf_Number(t *testing.T) {
	s := newTestSynthesizer()
	result := s.TypeOf(&ast.NumberExpr{Value: "3.14"}, 0)
	lit, ok := result.(*typ.Literal)
	if !ok {
		t.Fatalf("got %T, want literal", result)
	}
	if lit.Value != 3.14 {
		t.Fatalf("got %v, want 3.14", lit.Value)
	}
}

func TestSynthesizer_TypeOf_String(t *testing.T) {
	s := newTestSynthesizer()
	result := s.TypeOf(&ast.StringExpr{Value: "hello"}, 0)
	lit, ok := result.(*typ.Literal)
	if !ok {
		t.Fatalf("got %T, want literal", result)
	}
	if lit.Value != "hello" {
		t.Fatalf("got %v, want hello", lit.Value)
	}
}

func TestSynthesizer_TypeOf_Ident(t *testing.T) {
	s, ident := newTestSynthesizerWithSymbol("x", typ.Integer)
	result := s.TypeOf(ident, 0)
	if result != typ.Integer {
		t.Fatalf("got %v, want integer", result)
	}
}

func TestSynthesizer_TypeOf_IdentFallsBackToGraphSymbolAt(t *testing.T) {
	s, _ := newTestSynthesizerWithSymbol("x", typ.Integer)
	result := s.TypeOf(&ast.IdentExpr{Value: "x"}, 0)
	if result != typ.Integer {
		t.Fatalf("got %v, want integer via SymbolAt fallback", result)
	}
}

func TestSynthesizer_TypeOf_UnknownIdent(t *testing.T) {
	s := newTestSynthesizer()
	result := s.TypeOf(&ast.IdentExpr{Value: "unknown"}, 0)
	if result != typ.Unknown {
		t.Fatalf("got %v, want unknown", result)
	}
}

func TestSynthesizer_TypeOf_RelationalOp(t *testing.T) {
	s := newTestSynthesizer()
	result := s.TypeOf(&ast.RelationalOpExpr{
		Operator: "==",
		Lhs:      &ast.NumberExpr{Value: "1"},
		Rhs:      &ast.NumberExpr{Value: "2"},
	}, 0)
	if result != typ.Boolean {
		t.Fatalf("got %v, want boolean", result)
	}
}

func TestSynthesizer_TypeOf_StringConcat(t *testing.T) {
	s := newTestSynthesizer()
	result := s.TypeOf(&ast.StringConcatOpExpr{
		Lhs: &ast.StringExpr{Value: "a"},
		Rhs: &ast.StringExpr{Value: "b"},
	}, 0)
	if result != typ.String {
		t.Fatalf("got %v, want string", result)
	}
}

func TestSynthesizer_TypeOf_UnaryNot(t *testing.T) {
	s := newTestSynthesizer()
	result := s.TypeOf(&ast.UnaryNotOpExpr{
		Expr: &ast.TrueExpr{},
	}, 0)
	if result != typ.Boolean {
		t.Fatalf("got %v, want boolean", result)
	}
}

func TestSynthesizer_TypeOf_UnaryLen(t *testing.T) {
	s := newTestSynthesizer()
	result := s.TypeOf(&ast.UnaryLenOpExpr{
		Expr: &ast.StringExpr{Value: "test"},
	}, 0)
	if result != typ.Integer {
		t.Fatalf("got %v, want integer", result)
	}
}

func TestSynthesizer_TypeOf_UnaryBNot(t *testing.T) {
	s := newTestSynthesizer()
	result := s.TypeOf(&ast.UnaryBNotOpExpr{
		Expr: &ast.NumberExpr{Value: "1"},
	}, 0)
	if result != typ.Integer {
		t.Fatalf("got %v, want integer", result)
	}
}

func TestSynthesizer_MultiTypeOf_SingleExpr(t *testing.T) {
	s := newTestSynthesizer()
	result := s.MultiTypeOf(&ast.NumberExpr{Value: "42"}, 0)
	if len(result) != 1 {
		t.Fatalf("got %d types, want 1", len(result))
	}
}

func TestSynthesizer_ExpandValues(t *testing.T) {
	s := newTestSynthesizer()
	exprs := []ast.Expr{
		&ast.NumberExpr{Value: "1"},
		&ast.StringExpr{Value: "hello"},
	}
	result := s.ExpandValues(exprs, 3, 0)
	if len(result) != 3 {
		t.Fatalf("got %d types, want 3", len(result))
	}
	if result[2] != typ.Nil {
		t.Fatalf("expected nil padding, got %v", result[2])
	}
}

func TestSynthesizer_Cache(t *testing.T) {
	s := newTestSynthesizer()
	expr := &ast.NumberExpr{Value: "42"}

	t1 := s.TypeOf(expr, 0)
	t2 := s.TypeOf(expr, 0)

	if t1 != t2 {
		t.Fatal("caching not working")
	}
}

func TestSynthesizer_FunctionType(t *testing.T) {
	s := newTestSynthesizer()
	sc := scope.New()

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"a"},
			Types: []ast.TypeExpr{
				&ast.PrimitiveTypeExpr{Name: "string"},
			},
		},
		ReturnTypes: []ast.TypeExpr{
			&ast.PrimitiveTypeExpr{Name: "number"},
		},
	}

	result := s.FunctionType(fn, sc)
	if result == nil {
		t.Fatal("expected function type")
	}
	if len(result.Params) != 1 {
		t.Fatalf("got %d params, want 1", len(result.Params))
	}
}

func TestSynthesizer_Phase_Declared(t *testing.T) {
	deps := &Deps{
		Ctx:      db.NewQueryContext(db.New()),
		Types:    mockTypeQuerier{},
		Scopes:   make(api.ScopeMap),
		PreCache: make(api.Cache),
	}
	s := NewSynthesizer(deps, api.PhaseTypeResolution)

	if s.IsNarrowing() {
		t.Fatal("expected IsNarrowing()=false for PhaseTypeResolution")
	}
	if s.Phase() != api.PhaseTypeResolution {
		t.Fatalf("got phase %d, want PhaseTypeResolution", s.Phase())
	}
}

func TestSynthesizer_Phase_Narrowed(t *testing.T) {
	deps := &Deps{
		Ctx:      db.NewQueryContext(db.New()),
		Types:    mockTypeQuerier{},
		Scopes:   make(api.ScopeMap),
		PreCache: make(api.Cache),
	}
	s := NewSynthesizer(deps, api.PhaseNarrowing)

	if !s.IsNarrowing() {
		t.Fatal("expected IsNarrowing()=true for PhaseNarrowing")
	}
	if s.Phase() != api.PhaseNarrowing {
		t.Fatalf("got phase %d, want PhaseNarrowing", s.Phase())
	}
}
