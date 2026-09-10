package cfg

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg/extraction"
	"github.com/wippyai/go-lua/compiler/pathseg"
	basecfg "github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

// ParamDefs processes function parameters.
func (b *Builder) ParamDefs(fn *ast.FunctionExpr) {
	if fn == nil || fn.ParList == nil {
		return
	}

	paramSyms := b.Bindings.ParamSymbols(fn)
	if len(paramSyms) == 0 && len(fn.ParList.Names) == 0 {
		return
	}
	if len(paramSyms) == 0 {
		// Degrade gracefully when bindings are unavailable instead of dropping
		// parameter declarations entirely.
		paramSyms = make([]basecfg.SymbolID, len(fn.ParList.Names))
	}

	slotCount := len(paramSyms)
	if len(fn.ParList.Names) > slotCount {
		slotCount = len(fn.ParList.Names)
	}

	hasImplicitSelf := false
	if len(paramSyms) == len(fn.ParList.Names)+1 && len(paramSyms) > 0 {
		first := paramSyms[0]
		if first != 0 && b.Bindings.Name(first) == "self" && (len(fn.ParList.Names) == 0 || fn.ParList.Names[0] != "self") {
			hasImplicitSelf = true
		}
	}

	b.ParamNames = make([]string, 0, slotCount)
	b.ParamSymbols = make([]basecfg.SymbolID, 0, slotCount)
	b.ParamDeclPoints = make([]basecfg.Point, 0, slotCount)

	for i := 0; i < slotCount; i++ {
		var sym basecfg.SymbolID
		if i < len(paramSyms) {
			sym = paramSyms[i]
		}
		var name string
		var typeAnnotation ast.TypeExpr
		if hasImplicitSelf && i == 0 {
			name = "self"
		} else {
			parIdx := i
			if hasImplicitSelf {
				parIdx--
			}
			if parIdx >= 0 && parIdx < len(fn.ParList.Names) {
				name = fn.ParList.Names[parIdx]
			}
			if parIdx >= 0 && fn.ParList.Types != nil && parIdx < len(fn.ParList.Types) {
				typeAnnotation = fn.ParList.Types[parIdx]
			}
		}
		if name == "" && sym != 0 {
			name = b.Bindings.Name(sym)
		}
		if name == "" {
			continue
		}

		p := b.Cfg.AddNode(basecfg.NodeAssign, sym, "")
		b.AddLinearEdge(p)

		if sym != 0 {
			b.ScopeTracker.RegisterSymbol(sym, name, basecfg.SymbolParam, p)
		}

		b.ScopeTracker.SnapshotVisibility(p)

		b.ParamNames = append(b.ParamNames, name)
		b.ParamSymbols = append(b.ParamSymbols, sym)
		b.ParamDeclPoints = append(b.ParamDeclPoints, p)

		info := &AssignInfo{IsLocal: true}
		info.Targets = info.singleTarget[:]
		info.Targets[0] = AssignTarget{Kind: TargetIdent, Name: name, Symbol: sym}
		info.TypeAnnotations = info.singleTypeAnnotation[:]
		info.TypeAnnotations[0] = typeAnnotation
		b.Info[p] = info
	}
}

// Stmts processes a list of statements.
func (b *Builder) Stmts(stmts []ast.Stmt) {
	for _, stmt := range stmts {
		b.Stmt(stmt)
	}
}

// Stmt processes a single statement.
func (b *Builder) Stmt(stmt ast.Stmt) {
	if stmt == nil {
		return
	}

	switch s := stmt.(type) {
	case *ast.LocalAssignStmt:
		b.LocalAssign(s)
	case *ast.AssignStmt:
		b.Assign(s)
	case *ast.ReturnStmt:
		b.ReturnStmt(s)
	case *ast.IfStmt:
		b.IfStmt(s)
	case *ast.WhileStmt:
		b.WhileStmt(s)
	case *ast.RepeatStmt:
		b.RepeatStmt(s)
	case *ast.NumberForStmt:
		b.NumberFor(s)
	case *ast.GenericForStmt:
		b.GenericFor(s)
	case *ast.FuncCallStmt:
		b.CallStmt(s)
	case *ast.DoBlockStmt:
		b.ScopedBlock(s.Stmts)
	case *ast.FuncDefStmt:
		b.FuncDef(s)
	case *ast.TypeDefStmt:
		b.TypeDef(s)
	case *ast.BreakStmt:
		b.BreakStmt(s)
	case *ast.LabelStmt:
		b.LabelStmt(s)
	case *ast.GotoStmt:
		b.GotoStmt(s)
	}
}

// LocalAssign processes a local assignment statement.
func (b *Builder) LocalAssign(s *ast.LocalAssignStmt) {
	p := b.Cfg.AddNode(basecfg.NodeAssign, 0, "")
	b.AddLinearEdge(p)

	sourceNames := extractIdentNames(s.Exprs)

	info := &AssignInfo{
		IsLocal:     true,
		Stmt:        s,
		Sources:     s.Exprs,
		SourceNames: sourceNames,
		SourceCalls: ExtractSourceCalls(s.Exprs),
	}
	if len(s.Types) > 0 {
		if len(s.Names) == 1 {
			info.TypeAnnotations = info.singleTypeAnnotation[:]
		} else {
			info.TypeAnnotations = make([]ast.TypeExpr, len(s.Names))
		}
	}
	annotations := info.TypeAnnotations
	switch len(s.Names) {
	case 1:
		info.Targets = info.singleTarget[:]
	case 2, 3, 4, 5, 6, 7, 8:
		info.Targets = make([]AssignTarget, len(s.Names))
	default:
		if len(s.Names) > 0 {
			info.Targets = make([]AssignTarget, len(s.Names))
		}
	}

	for i, name := range s.Names {
		sym := b.localAssignSymbolAt(s, i)

		if sym != 0 {
			b.ScopeTracker.RegisterSymbol(sym, name, basecfg.SymbolLocal, p)
		}

		if sym != 0 && i < len(s.Exprs) {
			if fnExpr, ok := s.Exprs[i].(*ast.FunctionExpr); ok && fnExpr != nil && b.Bindings != nil {
				b.Bindings.SetFuncLitSymbol(fnExpr, sym)
			}
		}

		info.Targets[i] = AssignTarget{Kind: TargetIdent, Name: name, Symbol: sym}

		if annotations != nil && i < len(s.Types) {
			annotations[i] = s.Types[i]
		}
	}

	b.ScopeTracker.SnapshotVisibility(p)

	for i, expr := range s.Exprs {
		var baseSym basecfg.SymbolID
		if i < len(info.Targets) {
			baseSym = info.Targets[i].Symbol
		}

		b.scanExprForFuncsWithSymbol(p, expr, baseSym)
	}

	b.resolveSourceSymbols(info, s.Exprs)
	b.resolveCallInfos(info.SourceCalls)
	b.Info[p] = info
}

func (b *Builder) localAssignSymbolAt(s *ast.LocalAssignStmt, i int) basecfg.SymbolID {
	if b.Bindings == nil {
		return 0
	}
	sym, _ := b.Bindings.LocalSymbolAt(s, i)

	return sym
}

// Assign processes a regular assignment statement.
func (b *Builder) Assign(s *ast.AssignStmt) {
	p := b.Cfg.AddNode(basecfg.NodeAssign, 0, "")
	b.AddLinearEdge(p)

	info := &AssignInfo{
		IsLocal:     false,
		Stmt:        s,
		Sources:     s.Rhs,
		SourceCalls: ExtractSourceCalls(s.Rhs),
	}
	switch len(s.Lhs) {
	case 1:
		info.Targets = info.singleTarget[:]
	case 2, 3, 4, 5, 6, 7, 8:
		info.Targets = make([]AssignTarget, len(s.Lhs))
	default:
		if len(s.Lhs) > 0 {
			info.Targets = make([]AssignTarget, len(s.Lhs))
		}
	}

	for i, lhs := range s.Lhs {
		target := ExtractAssignTarget(lhs)
		switch target.Kind {
		case TargetIdent:
			if ident, ok := lhs.(*ast.IdentExpr); ok {
				target.Symbol, _ = b.symbolFromIdent(ident)
				if target.Symbol != 0 && i < len(s.Rhs) {
					if fnExpr, ok := s.Rhs[i].(*ast.FunctionExpr); ok && fnExpr != nil && b.Bindings != nil {
						b.Bindings.SetFuncLitSymbol(fnExpr, target.Symbol)
					}
				}
			}
		case TargetField:
			baseSym := b.resolveFieldBaseSymbol(lhs)
			target.BaseSymbol = baseSym

			if baseSym != 0 && len(target.FieldPath) > 0 {
				if fieldSegments, ok := fieldSegmentsFromNames(target.FieldPath); ok {
					target.Symbol = b.getOrCreateFieldPathSymbol(baseSym, fieldSegments)
				}

				if i < len(s.Rhs) {
					if fnExpr, ok := s.Rhs[i].(*ast.FunctionExpr); ok && fnExpr != nil && b.Bindings != nil {
						b.Bindings.SetFuncLitSymbol(fnExpr, target.Symbol)
					}
				}
			}
		case TargetIndex:
			if baseIdent, ok := target.Base.(*ast.IdentExpr); ok && baseIdent != nil {
				target.BaseName = baseIdent.Value
				target.BaseSymbol, _ = b.symbolFromIdent(baseIdent)
			}

			if target.BaseSymbol != 0 && target.Key != nil {
				if keySegment, ok := pathseg.StaticAttrKeySegment(target.Key); ok {
					target.Symbol = b.getOrCreateFieldPathSymbol(target.BaseSymbol, []constraint.Segment{keySegment})
				}

				if target.Symbol != 0 && i < len(s.Rhs) {
					if fnExpr, ok := s.Rhs[i].(*ast.FunctionExpr); ok && fnExpr != nil && b.Bindings != nil {
						b.Bindings.SetFuncLitSymbol(fnExpr, target.Symbol)
					}
				}
			}
		}

		info.Targets[i] = target
	}

	b.ScopeTracker.SnapshotVisibility(p)
	info.SourceNames = b.ProcessExprs(p, s.Rhs)

	b.resolveSourceSymbols(info, s.Rhs)
	b.resolveCallInfos(info.SourceCalls)
	b.Info[p] = info
}

// ReturnStmt processes a return statement.
func (b *Builder) ReturnStmt(s *ast.ReturnStmt) {
	p := b.Cfg.AddNode(basecfg.NodeReturn, 0, "")
	b.AddLinearEdge(p)
	b.Cfg.AddEdge(p, b.Cfg.Exit(), false)
	b.Current = p
	b.CurrentLive = false

	b.ScopeTracker.SnapshotVisibility(p)

	info := &ReturnInfo{
		Stmt:        s,
		Exprs:       s.Exprs,
		Names:       b.ProcessExprs(p, s.Exprs),
		SourceCalls: ExtractSourceCalls(s.Exprs),
	}

	b.resolveCallInfos(info.SourceCalls)

	if len(s.Exprs) > 0 {
		var symbols []basecfg.SymbolID

		for i, expr := range s.Exprs {
			if sym, ok := b.symbolFromExpr(expr); ok {
				if symbols == nil {
					symbols = make([]basecfg.SymbolID, len(s.Exprs))
				}
				symbols[i] = sym
			}
		}
		info.Symbols = symbols
	}

	b.Info[p] = info
}

// BreakStmt processes a break statement.
func (b *Builder) BreakStmt(_ *ast.BreakStmt) {
	p := b.Cfg.AddNode(basecfg.NodeJoin, 0, "")
	if len(b.LoopExits) == 0 {
		b.AddLinearEdge(p)

		return
	}

	if b.CurrentLive {
		b.Cfg.AddEdge(b.Current, p, false)
	}

	b.ScopeTracker.SnapshotVisibility(p)
	exit := b.LoopExits[len(b.LoopExits)-1]
	b.Cfg.AddEdge(p, exit, false)
	b.Current = p
	b.CurrentLive = false
}

// IfStmt processes an if statement.
func (b *Builder) IfStmt(s *ast.IfStmt) {
	entryLive := b.CurrentLive
	thenStart := b.Cfg.AddNode(basecfg.NodeScopeEnter, 0, "")
	elseStart := b.Cfg.AddNode(basecfg.NodeScopeEnter, 0, "")
	condEntry := b.AddConditionEdges(s.Condition, thenStart, elseStart)
	b.AddLinearEdge(condEntry)

	// Then branch
	b.Current = thenStart
	b.CurrentLive = entryLive
	b.ScopeTracker.EnterScope()
	b.ScopeTracker.SnapshotVisibility(thenStart)
	b.Stmts(s.Then)
	thenExit := b.Cfg.AddNode(basecfg.NodeScopeExit, 0, "")

	if condNode := b.Cfg.Node(condEntry); condNode != nil {
		b.Cfg.Nodes[thenExit].CondVar = condNode.CondVar
		b.Cfg.Nodes[thenExit].CondCheck = condNode.CondCheck
	}

	thenLive := b.CurrentLive

	if entryLive && thenLive {
		b.Cfg.AddEdge(b.Current, thenExit, false)
	}

	b.ScopeTracker.ExitScope()
	b.ScopeTracker.SnapshotVisibility(thenExit)
	thenEnd := thenExit

	// Else branch
	b.Current = elseStart
	b.CurrentLive = entryLive
	b.ScopeTracker.EnterScope()
	b.ScopeTracker.SnapshotVisibility(elseStart)

	if len(s.Else) > 0 {
		b.Stmts(s.Else)
	}

	elseExit := b.Cfg.AddNode(basecfg.NodeScopeExit, 0, "")

	if condNode := b.Cfg.Node(condEntry); condNode != nil {
		b.Cfg.Nodes[elseExit].CondVar = condNode.CondVar
		b.Cfg.Nodes[elseExit].CondCheck = condNode.CondCheck
	}

	elseLive := b.CurrentLive

	if entryLive && elseLive {
		b.Cfg.AddEdge(b.Current, elseExit, false)
	}

	b.ScopeTracker.ExitScope()
	b.ScopeTracker.SnapshotVisibility(elseExit)
	elseEnd := elseExit

	// Join point
	join := b.Cfg.AddNode(basecfg.NodeJoin, 0, "")
	if entryLive && thenLive {
		b.Cfg.AddEdge(thenEnd, join, true)
	}

	if entryLive && elseLive {
		b.Cfg.AddEdge(elseEnd, join, false)
	}

	b.Current = join
	b.CurrentLive = entryLive && (thenLive || elseLive)
	b.ScopeTracker.SnapshotVisibility(join)
}

// WhileStmt processes a while statement.
func (b *Builder) WhileStmt(s *ast.WhileStmt) {
	entryLive := b.CurrentLive
	preheader := b.Current
	bodyStart := b.Cfg.AddNode(basecfg.NodeScopeEnter, 0, "")
	loopExit := b.Cfg.AddNode(basecfg.NodeScopeExit, 0, "")
	b.ScopeTracker.SnapshotVisibility(loopExit)
	join := b.Cfg.AddNode(basecfg.NodeJoin, 0, "")
	condEntry := b.AddConditionEdges(s.Condition, bodyStart, join)

	if loopVarIdents := extraction.AssignedOuterIdentsInBlock(s.Stmts); len(loopVarIdents) > 0 {
		b.Cfg.Nodes[condEntry].LoopVars = b.resolveIdentsToSymbols(loopVarIdents)
	}

	b.Cfg.Nodes[condEntry].LoopPreheader = preheader
	b.Cfg.Nodes[condEntry].LoopPreheaderSet = true
	b.AddLinearEdge(condEntry)

	b.Current = bodyStart
	b.CurrentLive = entryLive
	b.ScopeTracker.EnterScope()
	b.ScopeTracker.SnapshotVisibility(bodyStart)
	b.LoopExits = append(b.LoopExits, loopExit)
	b.Stmts(s.Stmts)
	b.LoopExits = b.LoopExits[:len(b.LoopExits)-1]
	bodyExit := b.Cfg.AddNode(basecfg.NodeScopeExit, 0, "")

	if entryLive && b.CurrentLive {
		b.Cfg.AddEdge(b.Current, bodyExit, false)
		b.Cfg.AddEdge(bodyExit, condEntry, false)
	}

	b.ScopeTracker.ExitScope()
	b.ScopeTracker.SnapshotVisibility(bodyExit)
	b.Cfg.AddEdge(loopExit, join, false)

	b.Current = join
	b.CurrentLive = entryLive
	b.ScopeTracker.SnapshotVisibility(join)
}

// RepeatStmt processes a repeat statement.
func (b *Builder) RepeatStmt(s *ast.RepeatStmt) {
	entryLive := b.CurrentLive
	scopeEnter := b.Cfg.AddNode(basecfg.NodeScopeEnter, 0, "")
	bodyStart := b.Cfg.AddNode(basecfg.NodeJoin, 0, "")
	b.ScopeTracker.SnapshotVisibility(bodyStart)
	b.AddLinearEdge(scopeEnter)
	b.Cfg.AddEdge(scopeEnter, bodyStart, false)
	b.CurrentLive = entryLive

	if loopVarIdents := extraction.AssignedOuterIdentsInBlock(s.Stmts); len(loopVarIdents) > 0 {
		b.Cfg.Nodes[bodyStart].LoopVars = b.resolveIdentsToSymbols(loopVarIdents)
	}

	b.Cfg.Nodes[bodyStart].LoopPreheader = scopeEnter
	b.Cfg.Nodes[bodyStart].LoopPreheaderSet = true

	b.ScopeTracker.EnterScope()
	b.ScopeTracker.SnapshotVisibility(scopeEnter)

	loopExit := b.Cfg.AddNode(basecfg.NodeScopeExit, 0, "")
	b.LoopExits = append(b.LoopExits, loopExit)
	b.Current = bodyStart
	b.Stmts(s.Stmts)
	b.LoopExits = b.LoopExits[:len(b.LoopExits)-1]

	join := b.Cfg.AddNode(basecfg.NodeJoin, 0, "")
	preheader := b.Current
	condEntry := b.AddConditionEdges(s.Condition, loopExit, bodyStart)
	b.Cfg.Nodes[condEntry].LoopPreheader = preheader
	b.Cfg.Nodes[condEntry].LoopPreheaderSet = true

	if entryLive && b.CurrentLive {
		b.AddLinearEdge(condEntry)
	} else {
		b.Current = condEntry
		b.CurrentLive = false
	}

	b.ScopeTracker.ExitScope()
	b.ScopeTracker.SnapshotVisibility(loopExit)
	b.Cfg.AddEdge(loopExit, join, false)

	b.Current = join
	b.CurrentLive = entryLive && b.CurrentLive
	b.ScopeTracker.SnapshotVisibility(join)
}

// NumberFor processes a numeric for statement.
func (b *Builder) NumberFor(s *ast.NumberForStmt) {
	entryLive := b.CurrentLive
	loopEnter := b.Cfg.AddNode(basecfg.NodeScopeEnter, 0, "")
	b.AddLinearEdge(loopEnter)

	b.ScopeTracker.EnterScope()
	b.ScopeTracker.SnapshotVisibility(loopEnter)

	sym, _ := b.Bindings.NumForSymbol(s)

	assign := b.Cfg.AddNode(basecfg.NodeAssign, sym, "")
	b.AddLinearEdge(assign)

	if sym != 0 {
		b.ScopeTracker.RegisterSymbol(sym, s.Name, basecfg.SymbolLocal, assign)
	}

	b.ScopeTracker.SnapshotVisibility(assign)

	b.Info[assign] = &AssignInfo{
		IsLocal: true,
		Stmt:    s,
		Targets: []AssignTarget{{Kind: TargetIdent, Name: s.Name, Symbol: sym}},
		NumericFor: &NumericForInfo{
			VarName: s.Name,
			Init:    s.Init,
			Limit:   s.Limit,
			Step:    s.Step,
		},
	}

	b.scanExprForFuncsWithContext(assign, s.Init, nil, "")
	b.scanExprForFuncsWithContext(assign, s.Limit, nil, "")
	b.scanExprForFuncsWithContext(assign, s.Step, nil, "")

	branch := b.Cfg.AddBranch(sym, basecfg.CondCheck{Kind: basecfg.CheckLimit})
	b.ScopeTracker.SnapshotVisibility(branch)
	b.Cfg.Nodes[branch].LoopLocals = []basecfg.SymbolID{sym}

	if loopVarIdents := extraction.AssignedOuterIdentsInBlock(s.Stmts); len(loopVarIdents) > 0 {
		b.Cfg.Nodes[branch].LoopVars = b.resolveIdentsToSymbols(loopVarIdents)
	}

	b.Cfg.Nodes[branch].LoopPreheader = assign
	b.Cfg.Nodes[branch].LoopPreheaderSet = true
	b.Cfg.AddEdge(assign, branch, false)

	b.Info[branch] = &BranchInfo{
		CondVar:    s.Name,
		CondSymbol: sym,
		CondCheck:  basecfg.CondCheck{Kind: basecfg.CheckLimit},
	}

	bodyStart := b.Cfg.AddNode(basecfg.NodeScopeEnter, 0, "")
	b.Cfg.Nodes[bodyStart].LoopLocals = []basecfg.SymbolID{sym}
	b.Cfg.AddEdge(branch, bodyStart, true)
	b.Current = bodyStart
	b.CurrentLive = entryLive
	b.ScopeTracker.SnapshotVisibility(bodyStart)
	loopExit := b.Cfg.AddNode(basecfg.NodeScopeExit, 0, "")
	join := b.Cfg.AddNode(basecfg.NodeJoin, 0, "")
	b.LoopExits = append(b.LoopExits, loopExit)
	b.Stmts(s.Stmts)
	b.LoopExits = b.LoopExits[:len(b.LoopExits)-1]
	bodyExit := b.Cfg.AddNode(basecfg.NodeScopeExit, 0, "")

	if entryLive && b.CurrentLive {
		b.Cfg.AddEdge(b.Current, bodyExit, false)
		b.Cfg.AddEdge(bodyExit, branch, false)
	}

	b.ScopeTracker.SnapshotVisibility(bodyExit)

	b.Cfg.AddEdge(branch, loopExit, false)
	b.ScopeTracker.ExitScope()
	b.ScopeTracker.SnapshotVisibility(loopExit)
	b.Cfg.AddEdge(loopExit, join, false)

	b.Current = join
	b.CurrentLive = entryLive
	b.ScopeTracker.SnapshotVisibility(join)
}

// GenericFor processes a generic for statement.
func (b *Builder) GenericFor(s *ast.GenericForStmt) {
	if len(s.Names) == 0 {
		return
	}

	entryLive := b.CurrentLive
	loopEnter := b.Cfg.AddNode(basecfg.NodeScopeEnter, 0, "")
	b.AddLinearEdge(loopEnter)

	b.ScopeTracker.EnterScope()
	b.ScopeTracker.SnapshotVisibility(loopEnter)

	forSyms := b.Bindings.GenericForSymbols(s)

	assign := b.Cfg.AddNode(basecfg.NodeAssign, 0, "")
	b.AddLinearEdge(assign)

	targets := make([]AssignTarget, 0, len(s.Names))

	var firstSym basecfg.SymbolID

	for i, name := range s.Names {
		var sym basecfg.SymbolID

		if i < len(forSyms) {
			sym = forSyms[i]
		}

		if i == 0 {
			firstSym = sym
			b.Cfg.Nodes[assign].Target = sym
		}

		if sym != 0 {
			b.ScopeTracker.RegisterSymbol(sym, name, basecfg.SymbolLocal, assign)
		}
		targets = append(targets, AssignTarget{Kind: TargetIdent, Name: name, Symbol: sym})
	}

	b.ScopeTracker.SnapshotVisibility(assign)

	b.Info[assign] = &AssignInfo{
		IsLocal:   true,
		Stmt:      s,
		Targets:   targets,
		IterExprs: s.Exprs,
	}

	for _, expr := range s.Exprs {
		b.scanExprForFuncsWithContext(assign, expr, nil, "")
	}

	branch := b.Cfg.AddBranch(firstSym, basecfg.CondCheck{Kind: basecfg.CheckNotNil})

	b.ScopeTracker.SnapshotVisibility(branch)

	loopLocalSyms := make([]basecfg.SymbolID, 0, len(targets))

	for _, t := range targets {
		if t.Symbol != 0 {
			loopLocalSyms = append(loopLocalSyms, t.Symbol)
		}
	}

	b.Cfg.Nodes[branch].LoopLocals = loopLocalSyms

	if loopVarIdents := extraction.AssignedOuterIdentsInBlock(s.Stmts); len(loopVarIdents) > 0 {
		b.Cfg.Nodes[branch].LoopVars = b.resolveIdentsToSymbols(loopVarIdents)
	}

	b.Cfg.Nodes[branch].LoopPreheader = assign
	b.Cfg.Nodes[branch].LoopPreheaderSet = true
	b.Cfg.AddEdge(b.Current, branch, false)

	b.Info[branch] = &BranchInfo{
		CondVar:    s.Names[0],
		CondSymbol: firstSym,
		CondCheck:  basecfg.CondCheck{Kind: basecfg.CheckNotNil},
	}

	bodyStart := b.Cfg.AddNode(basecfg.NodeScopeEnter, 0, "")
	b.Cfg.Nodes[bodyStart].LoopLocals = loopLocalSyms
	b.Cfg.AddEdge(branch, bodyStart, true)
	b.Current = bodyStart
	b.CurrentLive = entryLive
	b.ScopeTracker.SnapshotVisibility(bodyStart)
	loopExit := b.Cfg.AddNode(basecfg.NodeScopeExit, 0, "")
	join := b.Cfg.AddNode(basecfg.NodeJoin, 0, "")
	b.LoopExits = append(b.LoopExits, loopExit)
	b.Stmts(s.Stmts)
	b.LoopExits = b.LoopExits[:len(b.LoopExits)-1]
	bodyExit := b.Cfg.AddNode(basecfg.NodeScopeExit, 0, "")

	if entryLive && b.CurrentLive {
		b.Cfg.AddEdge(b.Current, bodyExit, false)
		b.Cfg.AddEdge(bodyExit, branch, false)
	}

	b.ScopeTracker.SnapshotVisibility(bodyExit)

	b.Cfg.AddEdge(branch, loopExit, false)
	b.ScopeTracker.ExitScope()
	b.ScopeTracker.SnapshotVisibility(loopExit)
	b.Cfg.AddEdge(loopExit, join, false)

	b.Current = join
	b.CurrentLive = entryLive
	b.ScopeTracker.SnapshotVisibility(join)
}

// CallStmt processes a function call statement.
func (b *Builder) CallStmt(s *ast.FuncCallStmt) {
	callee := extraction.ExtractCalleeName(s.Expr)
	p := b.Cfg.AddNode(basecfg.NodeCall, 0, callee)
	b.AddLinearEdge(p)

	if callee == "error" {
		b.CurrentLive = false
	}

	b.ScopeTracker.SnapshotVisibility(p)

	if call, ok := s.Expr.(*ast.FuncCallExpr); ok {
		b.scanExprForFuncsWithContext(p, call.Func, nil, "")
		b.scanExprForFuncsWithContext(p, call.Receiver, nil, "")
		b.scanExprsForFuncs(p, call.Args)
		info := BuildCallInfo(call, true)
		b.resolveCallInfoSymbols(info)
		b.Info[p] = info
	}
}

// FuncDef processes a function definition statement.
func (b *Builder) FuncDef(s *ast.FuncDefStmt) {
	p := b.Cfg.AddNode(basecfg.NodeAssign, 0, "")
	b.AddLinearEdge(p)
	b.ScopeTracker.SnapshotVisibility(p)

	info := &FuncDefInfo{
		FuncExpr: s.Func,
	}

	if s.Name == nil {
		b.Info[p] = info

		return
	}

	if s.Name.Method != "" {
		info.TargetKind = FuncDefMethod
		info.Name = s.Name.Method
		info.Receiver = s.Name.Receiver
		info.IsMethod = true

		if recvIdent, ok := s.Name.Receiver.(*ast.IdentExpr); ok && recvIdent != nil {
			info.ReceiverName = recvIdent.Value
			info.ReceiverSymbol, _ = b.symbolFromIdent(recvIdent)
		}

		receiverPath := b.pathFromExpr(s.Name.Receiver)
		if !receiverPath.IsEmpty() {
			info.TargetPath = receiverPath.Append(constraint.Segment{Kind: constraint.SegmentField, Name: info.Name})
			info.Symbol = b.getOrCreateFieldPathSymbol(receiverPath.Symbol, info.TargetPath.Segments)
		}
	} else if s.Name.Func != nil {
		switch fn := s.Name.Func.(type) {
		case *ast.IdentExpr:
			info.TargetKind = FuncDefGlobal
			info.Name = fn.Value
			info.Symbol, _ = b.symbolFromIdent(fn)

			if info.Symbol != 0 {
				b.Cfg.Nodes[p].Target = info.Symbol
			}
			info.TargetPath = constraint.Path{Root: fn.Value, Symbol: info.Symbol}
		case *ast.AttrGetExpr:
			info.TargetKind = FuncDefField
			info.Receiver = fn.Object

			if key, ok := fn.Key.(*ast.StringExpr); ok {
				info.Name = key.Value
			} else if key, ok := fn.Key.(*ast.IdentExpr); ok {
				info.Name = key.Value
			}

			if recvIdent, ok := fn.Object.(*ast.IdentExpr); ok && recvIdent != nil {
				info.ReceiverName = recvIdent.Value
				info.ReceiverSymbol, _ = b.symbolFromIdent(recvIdent)
			}

			info.TargetPath = b.pathFromExpr(fn)
			if !info.TargetPath.IsEmpty() && info.TargetPath.Symbol != 0 {
				info.Symbol = b.getOrCreateFieldPathSymbol(info.TargetPath.Symbol, info.TargetPath.Segments)
			}
		}
	}

	var nestedSym basecfg.SymbolID

	if info.Symbol != 0 {
		nestedSym = info.Symbol

		if s.Func != nil && b.Bindings != nil {
			b.Bindings.SetFuncLitSymbol(s.Func, info.Symbol)
		}
	} else if s.Func != nil && b.Bindings != nil {
		nestedSym = b.Bindings.GetOrCreateFuncLitSymbol(s.Func)
	}

	b.Info[p] = info
	if s.Func != nil {
		b.Nested = append(b.Nested, NestedFunc{Point: p, Func: s.Func, Symbol: nestedSym})
	}
}

// TypeDef processes a type definition statement.
func (b *Builder) TypeDef(s *ast.TypeDefStmt) {
	p := b.Cfg.AddNode(basecfg.NodeTypeDef, 0, "")
	b.AddLinearEdge(p)
	b.ScopeTracker.SnapshotVisibility(p)

	params := make([]TypeParamInfo, 0, len(s.TypeParams))
	for _, tp := range s.TypeParams {
		params = append(params, TypeParamInfo{
			Name:       tp.Name,
			Constraint: tp.Constraint,
		})
	}

	b.Info[p] = &TypeDefInfo{
		Name:       s.Name,
		TypeParams: params,
		TypeExpr:   s.Type,
	}
}

// LabelStmt processes a label statement.
func (b *Builder) LabelStmt(s *ast.LabelStmt) {
	p := b.Cfg.AddNode(basecfg.NodeJoin, 0, "")
	if b.CurrentLive {
		b.Cfg.AddEdge(b.Current, p, false)
	}

	if pending := b.Pending[s.Name]; len(pending) > 0 {
		for _, from := range pending {
			b.Cfg.AddEdge(from, p, false)
		}

		delete(b.Pending, s.Name)
	}

	b.ScopeTracker.SnapshotVisibility(p)
	b.Labels[s.Name] = p
	b.Current = p
	b.CurrentLive = true
}

// GotoStmt processes a goto statement.
func (b *Builder) GotoStmt(s *ast.GotoStmt) {
	p := b.Cfg.AddNode(basecfg.NodeJoin, 0, "")
	if b.CurrentLive {
		b.Cfg.AddEdge(b.Current, p, false)
	}

	b.ScopeTracker.SnapshotVisibility(p)

	if target, ok := b.Labels[s.Label]; ok {
		b.Cfg.AddEdge(p, target, false)
	} else {
		b.Pending[s.Label] = append(b.Pending[s.Label], p)
	}

	b.Current = p
	b.CurrentLive = false
}

// ScopedBlock processes a do block.
func (b *Builder) ScopedBlock(stmts []ast.Stmt) {
	entryLive := b.CurrentLive
	enter := b.Cfg.AddNode(basecfg.NodeScopeEnter, 0, "")
	b.AddLinearEdge(enter)

	b.ScopeTracker.EnterScope()
	b.ScopeTracker.SnapshotVisibility(enter)

	b.Stmts(stmts)
	exit := b.Cfg.AddNode(basecfg.NodeScopeExit, 0, "")
	blockLive := b.CurrentLive

	if blockLive {
		b.Cfg.AddEdge(b.Current, exit, false)
	}

	b.ScopeTracker.ExitScope()
	b.ScopeTracker.SnapshotVisibility(exit)

	b.Current = exit
	b.CurrentLive = entryLive && blockLive
}
