package cfg

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	basecfg "github.com/wippyai/go-lua/types/cfg"
)

// TestNewBuilder verifies Builder initialization.
func TestNewBuilder(t *testing.T) {
	t.Parallel()

	b := NewBuilder()

	if b.Cfg == nil {
		t.Error("Cfg should not be nil")
	}
	if b.Info == nil {
		t.Error("Info should not be nil")
	}
	if !b.CurrentLive {
		t.Error("CurrentLive should be true initially")
	}
	if b.Labels == nil {
		t.Error("Labels should not be nil")
	}
	if b.Pending == nil {
		t.Error("Pending should not be nil")
	}
	if b.NextVersionID == nil {
		t.Error("NextVersionID should not be nil")
	}
	if b.VisibleVersion == nil {
		t.Error("VisibleVersion should not be nil")
	}
	if b.ScopeTracker == nil {
		t.Error("ScopeTracker should not be nil")
	}
}

// TestBuilder_ParamDefs tests parameter processing.
func TestBuilder_ParamDefs(t *testing.T) {
	t.Parallel()

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"a", "b", "c"},
		},
	}

	// Use binder to create bindings
	bindings := bind.Bind(fn, nil)

	b := NewBuilder()
	b.Bindings = bindings
	b.Current = b.Cfg.Entry()

	b.ParamDefs(fn)

	// Count assign nodes for parameters
	assignCount := 0
	for _, info := range b.Info {
		if a, ok := info.(*AssignInfo); ok && a.IsLocal {
			assignCount++
		}
	}

	if assignCount != 3 {
		t.Errorf("Expected 3 parameter assign nodes, got %d", assignCount)
	}
}

// TestBuilder_ParamDefs_Nil tests nil handling.
func TestBuilder_ParamDefs_Nil(t *testing.T) {
	t.Parallel()

	b := NewBuilder()
	b.Bindings = bind.NewBindingTable()
	b.Current = b.Cfg.Entry()

	b.ParamDefs(nil)
	b.ParamDefs(&ast.FunctionExpr{})
	b.ParamDefs(&ast.FunctionExpr{ParList: &ast.ParList{}})

	if len(b.Info) != 0 {
		t.Error("No info should be created for empty params")
	}
}

// TestBuilder_LocalAssign tests local variable declarations.
func TestBuilder_LocalAssign(t *testing.T) {
	t.Parallel()

	stmt := &ast.LocalAssignStmt{
		Names: []string{"x", "y"},
		Exprs: []ast.Expr{
			&ast.NumberExpr{Value: "1"},
			&ast.NumberExpr{Value: "2"},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	bindings := bind.Bind(fn, nil)

	b := NewBuilder()
	b.Bindings = bindings
	b.Current = b.Cfg.Entry()

	b.LocalAssign(stmt)

	if len(b.Info) != 1 {
		t.Fatalf("Expected 1 info entry, got %d", len(b.Info))
	}

	var found *AssignInfo

	for _, info := range b.Info {
		if a, ok := info.(*AssignInfo); ok {
			found = a

			break
		}
	}

	if found == nil {
		t.Fatal("AssignInfo not found")
	}
	if !found.IsLocal {
		t.Error("Should be local assignment")
	}
	if len(found.Targets) != 2 {
		t.Errorf("Expected 2 targets, got %d", len(found.Targets))
	}
	if found.Targets[0].Name != "x" || found.Targets[1].Name != "y" {
		t.Error("Target names should be x and y")
	}
}

// TestBuilder_Assign tests non-local assignments.
func TestBuilder_Assign(t *testing.T) {
	t.Parallel()

	stmt := &ast.AssignStmt{
		Lhs: []ast.Expr{&ast.IdentExpr{Value: "x"}},
		Rhs: []ast.Expr{&ast.NumberExpr{Value: "42"}},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	bindings := bind.Bind(fn, nil)

	b := NewBuilder()
	b.Bindings = bindings
	b.Current = b.Cfg.Entry()

	b.Assign(stmt)

	var found *AssignInfo

	for _, info := range b.Info {
		if a, ok := info.(*AssignInfo); ok {
			found = a

			break
		}
	}

	if found == nil {
		t.Fatal("AssignInfo not found")
	}

	if found.IsLocal {
		t.Error("Should not be local assignment")
	}

	if found.Targets[0].Kind != TargetIdent {
		t.Error("Target should be TargetIdent")
	}
}

// TestBuilder_AssignGlobalBaseSymbols ensures base symbols are declared for field/index assignments.
func TestBuilder_AssignGlobalBaseSymbols(t *testing.T) {
	t.Parallel()

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts: []ast.Stmt{
			&ast.AssignStmt{
				Lhs: []ast.Expr{
					&ast.AttrGetExpr{
						Object: &ast.IdentExpr{Value: "t"},
						Key:    &ast.StringExpr{Value: "x"},
					},
				},
				Rhs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
			},
			&ast.AssignStmt{
				Lhs: []ast.Expr{
					&ast.AttrGetExpr{
						Object: &ast.IdentExpr{Value: "t"},
						Key:    &ast.NumberExpr{Value: "1"},
					},
				},
				Rhs: []ast.Expr{&ast.NumberExpr{Value: "2"}},
			},
		},
	}

	g := Build(fn)
	if g == nil {
		t.Fatal("Build should return graph")
	}

	var fieldSym basecfg.SymbolID
	var fieldPoint Point
	g.EachAssign(func(p Point, info *AssignInfo) {
		if len(info.Targets) == 0 {
			return
		}
		tgt := info.Targets[0]
		if tgt.Kind == TargetField || tgt.Kind == TargetIndex {
			if tgt.BaseSymbol == 0 {
				t.Error("BaseSymbol should be resolved for assignment target")
			}
			if fieldSym == 0 {
				fieldSym = tgt.BaseSymbol
				fieldPoint = p
			}
		}
	})

	if fieldSym == 0 {
		t.Fatal("Expected base symbol for t")
	}
	if sym, ok := g.SymbolAt(fieldPoint, "t"); !ok || sym == 0 {
		t.Error("t should be visible at assignment point")
	}
	// Globals are registered at CFG entry point
	declPoint, ok := g.DeclarationPoint(fieldSym)
	if !ok {
		t.Error("t should have a declaration point")
	}
	entry := g.Entry()
	if declPoint != entry {
		t.Errorf("Global t declaration point should be entry (%d), got %d", entry, declPoint)
	}
}

// TestBuilder_ReturnStmt tests return statement processing.
func TestBuilder_ReturnStmt(t *testing.T) {
	t.Parallel()

	stmt := &ast.ReturnStmt{
		Exprs: []ast.Expr{
			&ast.NumberExpr{Value: "1"},
			&ast.IdentExpr{Value: "x"},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	bindings := bind.Bind(fn, nil)

	b := NewBuilder()
	b.Bindings = bindings
	b.Current = b.Cfg.Entry()

	b.ReturnStmt(stmt)

	var found *ReturnInfo

	for _, info := range b.Info {
		if r, ok := info.(*ReturnInfo); ok {
			found = r

			break
		}
	}

	if found == nil {
		t.Fatal("ReturnInfo not found")
	}
	if len(found.Exprs) != 2 {
		t.Error("Should have 2 expressions")
	}
	if found.Names[1] != "x" {
		t.Errorf("Names[1] should be 'x', got %q", found.Names[1])
	}

	// CurrentLive should be false after return
	if b.CurrentLive {
		t.Error("CurrentLive should be false after return")
	}
}

// TestBuilder_CallAndReturnSymbols ensures call/return symbols are resolved at build time.
// CalleeSymbol is set for simple variable calls to enable symbol-only effect resolution.
func TestBuilder_CallAndReturnSymbols(t *testing.T) {
	t.Parallel()

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"x"},
				Exprs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
			},
			&ast.FuncCallStmt{
				Expr: &ast.FuncCallExpr{
					Func: &ast.IdentExpr{Value: "print"},
					Args: []ast.Expr{&ast.IdentExpr{Value: "x"}},
				},
			},
			&ast.ReturnStmt{
				Exprs: []ast.Expr{&ast.IdentExpr{Value: "x"}},
			},
		},
	}

	g := Build(fn, "print")
	if g == nil {
		t.Fatal("Build should return graph")
	}

	var callInfo *CallInfo

	g.EachCall(func(_ Point, info *CallInfo) {
		callInfo = info
	})

	if callInfo == nil {
		t.Fatal("CallInfo not found")
	}
	// CalleeSymbol is set for simple variable calls (enables effect resolution)
	if callInfo.CalleeSymbol == 0 {
		t.Error("CalleeSymbol should be set for simple variable call")
	}
	if len(callInfo.ArgSymbols) != 1 || callInfo.ArgSymbols[0] == 0 {
		t.Error("ArgSymbols should resolve x")
	}

	var retInfo *ReturnInfo

	g.EachReturn(func(_ Point, info *ReturnInfo) {
		retInfo = info
	})

	if retInfo == nil {
		t.Fatal("ReturnInfo not found")
	}
	if len(retInfo.Symbols) != 1 || retInfo.Symbols[0] == 0 {
		t.Error("Return symbol should be resolved for x")
	}
}

// TestBuilder_BreakStmt tests break statement.
func TestBuilder_BreakStmt(t *testing.T) {
	t.Parallel()

	b := NewBuilder()
	b.Bindings = bind.NewBindingTable()
	b.Current = b.Cfg.Entry()

	// Without loop context
	b.BreakStmt(&ast.BreakStmt{})
	if !b.CurrentLive {
		t.Error("Break outside loop should keep CurrentLive true")
	}

	// With loop context
	b2 := NewBuilder()
	b2.Bindings = bind.NewBindingTable()
	b2.Current = b2.Cfg.Entry()
	loopExit := b2.Cfg.AddNode(basecfg.NodeJoin, 0, "")
	b2.LoopExits = append(b2.LoopExits, loopExit)

	b2.BreakStmt(&ast.BreakStmt{})
	if b2.CurrentLive {
		t.Error("Break in loop should set CurrentLive false")
	}
}

// TestBuilder_IfStmt tests if statement CFG construction.
func TestBuilder_IfStmt(t *testing.T) {
	t.Parallel()

	stmt := &ast.IfStmt{
		Condition: &ast.IdentExpr{Value: "cond"},
		Then: []ast.Stmt{
			&ast.LocalAssignStmt{Names: []string{"x"}, Exprs: []ast.Expr{&ast.NumberExpr{Value: "1"}}},
		},
		Else: []ast.Stmt{
			&ast.LocalAssignStmt{Names: []string{"y"}, Exprs: []ast.Expr{&ast.NumberExpr{Value: "2"}}},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	bindings := bind.Bind(fn, nil)

	b := NewBuilder()
	b.Bindings = bindings
	b.Current = b.Cfg.Entry()
	b.ScopeTracker.SnapshotVisibility(b.Current)

	b.IfStmt(stmt)

	// Count node types
	branchCount := 0
	joinCount := 0
	scopeEnterCount := 0
	scopeExitCount := 0

	for _, n := range b.Cfg.Nodes {
		switch n.Kind {
		case basecfg.NodeBranch:
			branchCount++
		case basecfg.NodeJoin:
			joinCount++
		case basecfg.NodeScopeEnter:
			scopeEnterCount++
		case basecfg.NodeScopeExit:
			scopeExitCount++
		}
	}

	if branchCount == 0 {
		t.Error("If statement should create branch node")
	}
	if joinCount == 0 {
		t.Error("If statement should create join node")
	}
	if scopeEnterCount < 2 {
		t.Errorf("Expected at least 2 scope enters, got %d", scopeEnterCount)
	}
}

// TestBuilder_WhileStmt tests while loop CFG construction.
func TestBuilder_WhileStmt(t *testing.T) {
	t.Parallel()

	stmt := &ast.WhileStmt{
		Condition: &ast.IdentExpr{Value: "cond"},
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{Names: []string{"x"}, Exprs: []ast.Expr{&ast.NumberExpr{Value: "1"}}},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	bindings := bind.Bind(fn, nil)

	b := NewBuilder()
	b.Bindings = bindings
	b.Current = b.Cfg.Entry()
	b.ScopeTracker.SnapshotVisibility(b.Current)

	b.WhileStmt(stmt)

	// Find branch node
	var branchPoint basecfg.Point

	for index, node := range b.Cfg.Nodes {
		if node.Kind == basecfg.NodeBranch {
			branchPoint = basecfg.Point(index)

			break
		}
	}

	if branchPoint == 0 {
		t.Fatal("Branch node not found")
	}

	// Should have back edge
	preds := b.Cfg.Predecessors(branchPoint)
	if len(preds) < 2 {
		t.Error("While loop branch should have multiple predecessors (back edge)")
	}
}

// TestBuilder_RepeatStmt tests repeat-until loop CFG construction.
func TestBuilder_RepeatStmt(t *testing.T) {
	t.Parallel()

	stmt := &ast.RepeatStmt{
		Condition: &ast.IdentExpr{Value: "done"},
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{Names: []string{"x"}, Exprs: []ast.Expr{&ast.NumberExpr{Value: "1"}}},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	bindings := bind.Bind(fn, nil)

	b := NewBuilder()
	b.Bindings = bindings
	b.Current = b.Cfg.Entry()
	b.ScopeTracker.SnapshotVisibility(b.Current)

	b.RepeatStmt(stmt)

	// Count join nodes
	joinCount := 0
	for _, n := range b.Cfg.Nodes {
		if n.Kind == basecfg.NodeJoin {
			joinCount++
		}
	}

	if joinCount < 2 {
		t.Errorf("Repeat loop should have at least 2 join points, got %d", joinCount)
	}
}

// TestBuilder_NumberFor tests numeric for loop.
func TestBuilder_NumberFor(t *testing.T) {
	t.Parallel()

	stmt := &ast.NumberForStmt{
		Name:  "i",
		Init:  &ast.NumberExpr{Value: "1"},
		Limit: &ast.NumberExpr{Value: "10"},
		Step:  &ast.NumberExpr{Value: "1"},
		Stmts: []ast.Stmt{},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	bindings := bind.Bind(fn, nil)

	b := NewBuilder()
	b.Bindings = bindings
	b.Current = b.Cfg.Entry()
	b.ScopeTracker.SnapshotVisibility(b.Current)

	b.NumberFor(stmt)

	// Find loop variable assignment
	var forInfo *NumericForInfo

	for _, info := range b.Info {
		if assignInfo, ok := info.(*AssignInfo); ok && assignInfo.NumericFor != nil {
			forInfo = assignInfo.NumericFor

			break
		}
	}

	if forInfo == nil {
		t.Fatal("NumericForInfo not found")
	}
	if forInfo.VarName != "i" {
		t.Errorf("VarName should be 'i', got %q", forInfo.VarName)
	}
}

// TestBuilder_GenericFor tests generic for loop.
func TestBuilder_GenericFor(t *testing.T) {
	t.Parallel()

	stmt := &ast.GenericForStmt{
		Names: []string{"k", "v"},
		Exprs: []ast.Expr{
			&ast.FuncCallExpr{
				Func: &ast.IdentExpr{Value: "pairs"},
				Args: []ast.Expr{&ast.IdentExpr{Value: "t"}},
			},
		},
		Stmts: []ast.Stmt{},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	bindings := bind.Bind(fn, nil)

	b := NewBuilder()
	b.Bindings = bindings
	b.Current = b.Cfg.Entry()
	b.ScopeTracker.SnapshotVisibility(b.Current)

	b.GenericFor(stmt)

	// Find loop variable assignment
	var found bool
	for _, info := range b.Info {
		if a, ok := info.(*AssignInfo); ok {
			if len(a.Targets) == 2 && a.Targets[0].Name == "k" && a.Targets[1].Name == "v" {
				found = true

				break
			}
		}
	}

	if !found {
		t.Error("Generic for variables k, v not found")
	}
}

// TestBuilder_GenericFor_Empty tests empty names handling.
func TestBuilder_GenericFor_Empty(t *testing.T) {
	t.Parallel()

	b := NewBuilder()
	b.Bindings = bind.NewBindingTable()
	b.Current = b.Cfg.Entry()

	b.GenericFor(&ast.GenericForStmt{
		Names: []string{},
		Exprs: []ast.Expr{},
		Stmts: []ast.Stmt{},
	})

	if len(b.Info) != 0 {
		t.Error("Empty generic for should not create nodes")
	}
}

// TestBuilder_CallStmt tests function call statement.
func TestBuilder_CallStmt(t *testing.T) {
	t.Parallel()

	stmt := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{
			Func: &ast.IdentExpr{Value: "print"},
			Args: []ast.Expr{&ast.StringExpr{Value: "hello"}},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	bindings := bind.Bind(fn, nil)

	b := NewBuilder()
	b.Bindings = bindings
	b.Current = b.Cfg.Entry()

	b.CallStmt(stmt)

	var found *CallInfo
	for _, info := range b.Info {
		if c, ok := info.(*CallInfo); ok {
			found = c

			break
		}
	}

	if found == nil {
		t.Fatal("CallInfo not found")
	}
	if found.CalleeName != "print" {
		t.Errorf("CalleeName should be 'print', got %q", found.CalleeName)
	}
	if !found.IsStmt {
		t.Error("IsStmt should be true")
	}
}

// TestBuilder_CallStmt_Error tests that error() terminates flow.
func TestBuilder_CallStmt_Error(t *testing.T) {
	t.Parallel()

	stmt := &ast.FuncCallStmt{
		Expr: &ast.FuncCallExpr{
			Func: &ast.IdentExpr{Value: "error"},
			Args: []ast.Expr{&ast.StringExpr{Value: "oops"}},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	bindings := bind.Bind(fn, nil)

	b := NewBuilder()
	b.Bindings = bindings
	b.Current = b.Cfg.Entry()

	b.CallStmt(stmt)

	if b.CurrentLive {
		t.Error("error() should set CurrentLive to false")
	}
}

// TestBuilder_FuncDef tests function definition.
func TestBuilder_FuncDef(t *testing.T) {
	t.Parallel()

	stmt := &ast.FuncDefStmt{
		Name: &ast.FuncName{
			Func: &ast.IdentExpr{Value: "myFunc"},
		},
		Func: &ast.FunctionExpr{
			ParList: &ast.ParList{Names: []string{"a"}},
			Stmts:   []ast.Stmt{},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	bindings := bind.Bind(fn, nil)

	b := NewBuilder()
	b.Bindings = bindings
	b.Current = b.Cfg.Entry()

	b.FuncDef(stmt)

	var found *FuncDefInfo
	for _, info := range b.Info {
		if f, ok := info.(*FuncDefInfo); ok {
			found = f

			break
		}
	}

	if found == nil {
		t.Fatal("FuncDefInfo not found")
	}

	if found.Name != "myFunc" {
		t.Errorf("Name should be 'myFunc', got %q", found.Name)
	}
	if found.TargetKind != FuncDefGlobal {
		t.Error("Should be global function")
	}

	if len(b.Nested) != 1 {
		t.Errorf("Expected 1 nested function, got %d", len(b.Nested))
	}
}

// TestBuilder_FuncDef_Method tests method definition.
func TestBuilder_FuncDef_Method(t *testing.T) {
	t.Parallel()

	stmt := &ast.FuncDefStmt{
		Name: &ast.FuncName{
			Receiver: &ast.IdentExpr{Value: "MyClass"},
			Method:   "doSomething",
		},
		Func: &ast.FunctionExpr{
			ParList: &ast.ParList{},
			Stmts:   []ast.Stmt{},
		},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{stmt}}
	bindings := bind.Bind(fn, nil)

	b := NewBuilder()
	b.Bindings = bindings
	b.Current = b.Cfg.Entry()

	b.FuncDef(stmt)

	var found *FuncDefInfo

	for _, info := range b.Info {
		if f, ok := info.(*FuncDefInfo); ok {
			found = f

			break
		}
	}

	if found == nil {
		t.Fatal("FuncDefInfo not found")
	}
	if found.Name != "doSomething" {
		t.Errorf("Name should be 'doSomething', got %q", found.Name)
	}
	if found.TargetKind != FuncDefMethod {
		t.Error("Should be method")
	}
	if !found.IsMethod {
		t.Error("IsMethod should be true")
	}
}

// TestBuilder_TypeDef tests type definition.
func TestBuilder_TypeDef(t *testing.T) {
	t.Parallel()

	b := NewBuilder()
	b.Bindings = bind.NewBindingTable()
	b.Current = b.Cfg.Entry()

	b.TypeDef(&ast.TypeDefStmt{
		Name: "MyType",
		TypeParams: []ast.TypeParamExpr{
			{Name: "T"},
		},
	})

	var found *TypeDefInfo

	for _, info := range b.Info {
		if td, ok := info.(*TypeDefInfo); ok {
			found = td

			break
		}
	}

	if found == nil {
		t.Fatal("TypeDefInfo not found")
	}
	if found.Name != "MyType" {
		t.Errorf("Name should be 'MyType', got %q", found.Name)
	}
	if len(found.TypeParams) != 1 {
		t.Errorf("Expected 1 type param, got %d", len(found.TypeParams))
	}
}

// TestBuilder_LabelStmt tests label statement.
func TestBuilder_LabelStmt(t *testing.T) {
	t.Parallel()

	b := NewBuilder()
	b.Bindings = bind.NewBindingTable()
	b.Current = b.Cfg.Entry()

	b.LabelStmt(&ast.LabelStmt{Name: "myLabel"})

	if _, ok := b.Labels["myLabel"]; !ok {
		t.Error("Label should be registered")
	}
}

// TestBuilder_GotoStmt tests goto statement.
func TestBuilder_GotoStmt(t *testing.T) {
	t.Parallel()

	b := NewBuilder()
	b.Bindings = bind.NewBindingTable()
	b.Current = b.Cfg.Entry()

	// Forward goto (label not yet defined)
	b.GotoStmt(&ast.GotoStmt{Label: "forward"})

	if len(b.Pending["forward"]) != 1 {
		t.Error("Forward goto should be pending")
	}
	if b.CurrentLive {
		t.Error("Goto should set CurrentLive false")
	}
}

// TestBuilder_ScopedBlock tests do-end block.
func TestBuilder_ScopedBlock(t *testing.T) {
	t.Parallel()

	stmts := []ast.Stmt{
		&ast.LocalAssignStmt{Names: []string{"x"}, Exprs: []ast.Expr{&ast.NumberExpr{Value: "1"}}},
	}
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{&ast.DoBlockStmt{Stmts: stmts}}}
	bindings := bind.Bind(fn, nil)

	b := NewBuilder()
	b.Bindings = bindings
	b.Current = b.Cfg.Entry()
	b.ScopeTracker.SnapshotVisibility(b.Current)

	b.ScopedBlock(stmts)

	enterCount := 0
	exitCount := 0
	for _, n := range b.Cfg.Nodes {
		if n.Kind == basecfg.NodeScopeEnter {
			enterCount++
		}
		if n.Kind == basecfg.NodeScopeExit {
			exitCount++
		}
	}

	if enterCount == 0 {
		t.Error("Should have scope enter")
	}
	if exitCount == 0 {
		t.Error("Should have scope exit")
	}
}

// TestBuilder_Stmts tests statement list processing.
func TestBuilder_Stmts(t *testing.T) {
	t.Parallel()

	stmts := []ast.Stmt{
		&ast.LocalAssignStmt{Names: []string{"x"}, Exprs: []ast.Expr{&ast.NumberExpr{Value: "1"}}},
		&ast.LocalAssignStmt{Names: []string{"y"}, Exprs: []ast.Expr{&ast.NumberExpr{Value: "2"}}},
	}
	fn := &ast.FunctionExpr{Stmts: stmts}
	bindings := bind.Bind(fn, nil)

	b := NewBuilder()
	b.Bindings = bindings
	b.Current = b.Cfg.Entry()

	b.Stmts(stmts)

	if len(b.Info) != 2 {
		t.Errorf("Expected 2 info entries, got %d", len(b.Info))
	}
}

// TestBuilder_Stmt_Nil tests nil statement handling.
func TestBuilder_Stmt_Nil(t *testing.T) {
	t.Parallel()

	b := NewBuilder()
	b.Bindings = bind.NewBindingTable()
	b.Current = b.Cfg.Entry()

	b.Stmt(nil)

	if len(b.Info) != 0 {
		t.Error("Nil statement should not create info")
	}
}

// TestSnapshotVisibility_AllPointsCovered verifies that SnapshotVisibility is called
// for every CFG point, ensuring globals are visible at all points.
func TestSnapshotVisibility_AllPointsCovered(t *testing.T) {
	t.Parallel()

	// Build a function with various statement types
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"x"}},
		Stmts: []ast.Stmt{
			// Local declaration
			&ast.LocalAssignStmt{
				Names: []string{"a"},
				Exprs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
			},
			// Function call
			&ast.FuncCallStmt{
				Expr: &ast.FuncCallExpr{
					Func: &ast.IdentExpr{Value: "print"},
					Args: []ast.Expr{&ast.IdentExpr{Value: "a"}},
				},
			},
			// If statement
			&ast.IfStmt{
				Condition: &ast.IdentExpr{Value: "x"},
				Then: []ast.Stmt{
					&ast.ReturnStmt{Exprs: []ast.Expr{&ast.NumberExpr{Value: "1"}}},
				},
			},
			// While loop
			&ast.WhileStmt{
				Condition: &ast.TrueExpr{},
				Stmts: []ast.Stmt{
					&ast.BreakStmt{},
				},
			},
			// Repeat loop
			&ast.RepeatStmt{
				Condition: &ast.TrueExpr{},
				Stmts: []ast.Stmt{
					&ast.LocalAssignStmt{Names: []string{"b"}, Exprs: []ast.Expr{&ast.NumberExpr{Value: "2"}}},
				},
			},
			// Numeric for
			&ast.NumberForStmt{
				Name:  "i",
				Init:  &ast.NumberExpr{Value: "1"},
				Limit: &ast.NumberExpr{Value: "10"},
				Stmts: []ast.Stmt{},
			},
			// Generic for
			&ast.GenericForStmt{
				Names: []string{"k", "v"},
				Exprs: []ast.Expr{&ast.IdentExpr{Value: "pairs"}, &ast.IdentExpr{Value: "t"}},
				Stmts: []ast.Stmt{},
			},
			// Goto/Label
			&ast.GotoStmt{Label: "done"},
			&ast.LabelStmt{Name: "done"},
			// Return
			&ast.ReturnStmt{Exprs: []ast.Expr{&ast.IdentExpr{Value: "a"}}},
		},
	}

	// Build with a global seeded
	g := Build(fn, "io")

	// Verify globals are visible at all points
	var missingPoints []basecfg.Point
	for p := basecfg.Point(0); p < basecfg.Point(g.Size()); p++ {
		node := g.Node(p)
		if node == nil {
			continue
		}
		sym, ok := g.SymbolAt(p, "io")
		if !ok || sym == 0 {
			missingPoints = append(missingPoints, p)
		}
	}

	if len(missingPoints) > 0 {
		t.Errorf("SnapshotVisibility missing at %d points: %v", len(missingPoints), missingPoints)
		for _, p := range missingPoints {
			node := g.Node(p)
			if node != nil {
				t.Logf("  Point %d: Kind=%d", p, node.Kind)
			}
		}
	}
}

// TestBuilder_ScopedBlock_CurrentLive tests that ScopedBlock properly updates CurrentLive.
func TestBuilder_ScopedBlock_CurrentLive(t *testing.T) {
	t.Parallel()

	t.Run("block ending with return sets CurrentLive false", func(t *testing.T) {
		t.Parallel()

		stmts := []ast.Stmt{
			&ast.ReturnStmt{Exprs: []ast.Expr{&ast.NumberExpr{Value: "1"}}},
		}
		fn := &ast.FunctionExpr{Stmts: []ast.Stmt{&ast.DoBlockStmt{Stmts: stmts}}}
		bindings := bind.Bind(fn, nil)

		b := NewBuilder()
		b.Bindings = bindings
		b.Current = b.Cfg.Entry()
		b.ScopeTracker.SnapshotVisibility(b.Current)

		b.ScopedBlock(stmts)

		if b.CurrentLive {
			t.Error("CurrentLive should be false after block with return")
		}
	})

	t.Run("block ending with break in loop sets CurrentLive false", func(t *testing.T) {
		t.Parallel()

		b := NewBuilder()
		b.Bindings = bind.NewBindingTable()
		b.Current = b.Cfg.Entry()
		b.ScopeTracker.SnapshotVisibility(b.Current)

		loopExit := b.Cfg.AddNode(basecfg.NodeJoin, 0, "")
		b.LoopExits = append(b.LoopExits, loopExit)

		b.ScopedBlock([]ast.Stmt{
			&ast.BreakStmt{},
		})

		if b.CurrentLive {
			t.Error("CurrentLive should be false after block with break")
		}
	})

	t.Run("block with normal statements keeps CurrentLive true", func(t *testing.T) {
		t.Parallel()

		stmts := []ast.Stmt{
			&ast.LocalAssignStmt{Names: []string{"x"}, Exprs: []ast.Expr{&ast.NumberExpr{Value: "1"}}},
		}
		fn := &ast.FunctionExpr{Stmts: []ast.Stmt{&ast.DoBlockStmt{Stmts: stmts}}}
		bindings := bind.Bind(fn, nil)

		b := NewBuilder()
		b.Bindings = bindings
		b.Current = b.Cfg.Entry()
		b.ScopeTracker.SnapshotVisibility(b.Current)

		b.ScopedBlock(stmts)

		if !b.CurrentLive {
			t.Error("CurrentLive should be true after block with normal statements")
		}
	})

	t.Run("nested scoped blocks propagate dead code", func(t *testing.T) {
		t.Parallel()

		innerBlock := &ast.DoBlockStmt{
			Stmts: []ast.Stmt{
				&ast.ReturnStmt{Exprs: []ast.Expr{&ast.NumberExpr{Value: "1"}}},
			},
		}
		outerStmts := []ast.Stmt{innerBlock}
		fn := &ast.FunctionExpr{Stmts: []ast.Stmt{&ast.DoBlockStmt{Stmts: outerStmts}}}
		bindings := bind.Bind(fn, nil)

		b := NewBuilder()
		b.Bindings = bindings
		b.Current = b.Cfg.Entry()
		b.ScopeTracker.SnapshotVisibility(b.Current)

		b.ScopedBlock(outerStmts)

		if b.CurrentLive {
			t.Error("CurrentLive should be false after nested block with return")
		}
	})

	t.Run("statement after scoped block with return is dead code", func(t *testing.T) {
		t.Parallel()

		fn := &ast.FunctionExpr{
			ParList: &ast.ParList{},
			Stmts: []ast.Stmt{
				&ast.DoBlockStmt{
					Stmts: []ast.Stmt{
						&ast.ReturnStmt{Exprs: []ast.Expr{&ast.NumberExpr{Value: "1"}}},
					},
				},
				&ast.LocalAssignStmt{
					Names: []string{"x"},
					Exprs: []ast.Expr{&ast.NumberExpr{Value: "2"}},
				},
			},
		}

		g := Build(fn)
		if g == nil {
			t.Fatal("Build returned nil")
		}
		// The assignment after the do block should have no predecessors
		// from the do block's scope exit (it's dead code)
		var assignPoint basecfg.Point
		g.EachAssign(func(p basecfg.Point, info *AssignInfo) {
			if len(info.Targets) == 1 && info.Targets[0].Name == "x" {
				assignPoint = p
			}
		})

		if assignPoint != 0 {
			preds := g.Predecessors(assignPoint)
			// The assignment should have no edge from the do block's exit
			// because that path is dead (return already executed)
			for _, pred := range preds {
				node := g.Node(pred)
				if node != nil && node.Kind == basecfg.NodeScopeExit {
					t.Error("dead code should not have edge from scope exit")
				}
			}
		}
	})
}

// TestPhiOperandVersions_HaveSymbol verifies that phi operand versions have Symbol set.
func TestPhiOperandVersions_HaveSymbol(t *testing.T) {
	t.Parallel()

	// Pattern: local v = nil; if cond then v = x end; if v then use(v) end
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: []string{"cond", "x"}},
		Stmts: []ast.Stmt{
			// local v = nil
			&ast.LocalAssignStmt{
				Names: []string{"v"},
				Exprs: []ast.Expr{&ast.NilExpr{}},
			},
			// if cond then v = x end
			&ast.IfStmt{
				Condition: &ast.IdentExpr{Value: "cond"},
				Then: []ast.Stmt{
					&ast.AssignStmt{
						Lhs: []ast.Expr{&ast.IdentExpr{Value: "v"}},
						Rhs: []ast.Expr{&ast.IdentExpr{Value: "x"}},
					},
				},
			},
			// if v then use(v) end
			&ast.IfStmt{
				Condition: &ast.IdentExpr{Value: "v"},
				Then: []ast.Stmt{
					&ast.FuncCallStmt{
						Expr: &ast.FuncCallExpr{
							Func: &ast.IdentExpr{Value: "print"},
							Args: []ast.Expr{&ast.IdentExpr{Value: "v"}},
						},
					},
				},
			},
		},
	}

	g := Build(fn, "print")
	if g == nil {
		t.Fatal("Build returned nil")
	}

	// Check phi nodes
	phis := g.PhiNodes()
	if len(phis) == 0 {
		t.Fatal("Expected at least one phi node for v at join point")
	}

	// Find phi for v
	var vPhi *basecfg.PhiNode

	for i := range phis {
		if phis[i].Target.Root == "v" {
			vPhi = &phis[i]

			break
		}
	}

	if vPhi == nil {
		t.Fatal("No phi node found for v")
	}

	t.Logf("Phi target: %s (Symbol=%d, ID=%d)", vPhi.Target.Key(), vPhi.Target.Symbol, vPhi.Target.ID)

	// Check that target has Symbol set
	if vPhi.Target.Symbol == 0 {
		t.Errorf("Phi target should have Symbol set, got 0")
	}

	// Check all operands have Symbol set
	for i, op := range vPhi.Operands {
		t.Logf("Operand[%d]: from=%d key=%s (Symbol=%d, ID=%d)",
			i, op.From, op.Version.Key(), op.Version.Symbol, op.Version.ID)
		if op.Version.Symbol == 0 {
			t.Errorf("Operand[%d] should have Symbol set, got 0", i)
		}
		// Symbol should match target symbol
		if op.Version.Symbol != vPhi.Target.Symbol {
			t.Errorf("Operand[%d] Symbol=%d should match target Symbol=%d", i, op.Version.Symbol, vPhi.Target.Symbol)
		}
	}
}

// TestReturnStmt_SourceCalls verifies that calls inside return expressions
// produce SourceCalls with resolved callee symbols.
func TestReturnStmt_SourceCalls(t *testing.T) {
	t.Parallel()

	fooIdent := &ast.IdentExpr{Value: "foo"}
	callExpr := &ast.FuncCallExpr{
		Func: fooIdent,
		Args: []ast.Expr{&ast.NumberExpr{Value: "1"}},
	}

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts: []ast.Stmt{
			&ast.ReturnStmt{
				Exprs: []ast.Expr{callExpr},
			},
		},
	}

	g := Build(fn, "foo")
	if g == nil {
		t.Fatal("Build should return graph")
	}

	var found *ReturnInfo
	g.EachReturn(func(_ Point, info *ReturnInfo) {
		if info != nil && len(info.Exprs) > 0 {
			found = info
		}
	})
	if found == nil {
		t.Fatal("ReturnInfo not found")
	}
	if len(found.SourceCalls) == 0 {
		t.Fatal("SourceCalls should be populated for return with call expression")
	}
	if found.SourceCalls[0] == nil {
		t.Fatal("SourceCalls[0] should be non-nil for call expression")
	}
	if found.SourceCalls[0].CalleeName != "foo" {
		t.Errorf("CalleeName should be 'foo', got %q", found.SourceCalls[0].CalleeName)
	}
}

// TestReturnStmt_SourceCalls_WithBindings verifies that return source calls are collected.
// CalleeSymbol is set for simple variable calls to enable symbol-only effect resolution.
func TestReturnStmt_SourceCalls_WithBindings(t *testing.T) {
	t.Parallel()

	fooIdent := &ast.IdentExpr{Value: "foo"}
	callExpr := &ast.FuncCallExpr{
		Func: fooIdent,
		Args: []ast.Expr{&ast.NumberExpr{Value: "1"}},
	}

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts: []ast.Stmt{
			// local foo = function() end
			&ast.LocalAssignStmt{
				Names: []string{"foo"},
				Exprs: []ast.Expr{&ast.FunctionExpr{ParList: &ast.ParList{}}},
			},
			&ast.ReturnStmt{
				Exprs: []ast.Expr{callExpr},
			},
		},
	}

	bindings := bind.Bind(fn, nil)
	g := BuildWithBindings(fn, bindings)

	if g == nil {
		t.Fatal("Build should return graph")
	}

	var found *ReturnInfo
	g.EachReturn(func(_ Point, info *ReturnInfo) {
		if info != nil && len(info.SourceCalls) > 0 && info.SourceCalls[0] != nil {
			found = info
		}
	})
	if found == nil {
		t.Fatal("ReturnInfo with SourceCalls not found")
	}
	// CalleeSymbol is set for simple variable calls (enables effect resolution)
	if found.SourceCalls[0].CalleeSymbol == 0 {
		t.Error("CalleeSymbol should be set for simple variable call")
	}
	// CalleeName should still be set
	if found.SourceCalls[0].CalleeName != "foo" {
		t.Errorf("CalleeName = %q, want %q", found.SourceCalls[0].CalleeName, "foo")
	}
}

// TestReturnStmt_SourceCalls_ForwardRef verifies that return source calls are collected.
// Note: CalleeSymbol = 0 for simple variable calls since variables can be reassigned.
func TestReturnStmt_SourceCalls_ForwardRef(t *testing.T) {
	t.Parallel()

	barIdent := &ast.IdentExpr{Value: "bar"}
	callExpr := &ast.FuncCallExpr{
		Func: barIdent,
		Args: []ast.Expr{&ast.NumberExpr{Value: "1"}},
	}

	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
		Stmts: []ast.Stmt{
			// local function foo(n) return bar(n) end
			&ast.LocalAssignStmt{
				Names: []string{"foo"},
				Exprs: []ast.Expr{
					&ast.FunctionExpr{
						ParList: &ast.ParList{Names: []string{"n"}},
						Stmts: []ast.Stmt{
							&ast.ReturnStmt{
								Exprs: []ast.Expr{callExpr},
							},
						},
					},
				},
			},
			// local function bar(n) return n end
			&ast.LocalAssignStmt{
				Names: []string{"bar"},
				Exprs: []ast.Expr{
					&ast.FunctionExpr{
						ParList: &ast.ParList{Names: []string{"n"}},
						Stmts: []ast.Stmt{
							&ast.ReturnStmt{
								Exprs: []ast.Expr{&ast.IdentExpr{Value: "n"}},
							},
						},
					},
				},
			},
		},
	}

	bindings := bind.Bind(fn, nil)

	// Build foo's inner CFG using module bindings
	fooExpr := fn.Stmts[0].(*ast.LocalAssignStmt).Exprs[0].(*ast.FunctionExpr)
	fooGraph := BuildWithBindings(fooExpr, bindings)
	if fooGraph == nil {
		t.Fatal("fooGraph should not be nil")
	}

	var found *ReturnInfo
	fooGraph.EachReturn(func(_ Point, info *ReturnInfo) {
		if info != nil && len(info.SourceCalls) > 0 && info.SourceCalls[0] != nil {
			found = info
		}
	})
	if found == nil {
		t.Fatal("ReturnInfo with SourceCalls not found in foo's graph")
	}
	// CalleeSymbol is set for simple variable calls (enables effect resolution)
	if found.SourceCalls[0].CalleeSymbol == 0 {
		t.Error("CalleeSymbol should be set for simple variable call")
	}
	// CalleeName should still be set
	if found.SourceCalls[0].CalleeName != "bar" {
		t.Errorf("CalleeName = %q, want %q", found.SourceCalls[0].CalleeName, "bar")
	}

	// Verify bar has a bound symbol and CalleeSymbol matches
	barSym, ok := bindings.SymbolOf(barIdent)
	if !ok || barSym == 0 {
		t.Fatal("bar should have a bound symbol")
	}
	if found.SourceCalls[0].CalleeSymbol != barSym {
		t.Errorf("CalleeSymbol = %d, want %d (barSym)", found.SourceCalls[0].CalleeSymbol, barSym)
	}
}
