// Package nestedinfer processes nested function definitions during type analysis.
//
// Nested functions (closures) require special handling because they:
//   - Capture variables from enclosing scopes
//   - May be called before their definition is reached
//   - Can form mutual recursion with siblings
//
// The [Processor] gathers nested function definitions from a parent graph,
// groups them by scope, and analyzes each group with the appropriate parent
// context. This includes:
//   - Computing enriched parent scopes with sibling function types
//   - Propagating captured field assignments back to parent scopes
//   - Recursively processing nested functions within nested functions
//
// The processor integrates with the fixpoint loop by storing interprocedural
// facts (literal signatures, captured assignments) that may affect other
// functions in subsequent iterations.
package nestedinfer

import (
	"sort"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/assign"
	"github.com/wippyai/go-lua/compiler/check/infer/captured"
	"github.com/wippyai/go-lua/compiler/check/nested"
	"github.com/wippyai/go-lua/compiler/check/returns"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/siblings"
	phasecore "github.com/wippyai/go-lua/compiler/check/synth/phase/core"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// CheckFunc analyzes a nested function with a given parent scope.
type CheckFunc func(fn *ast.FunctionExpr, parent *scope.State)

// ResultFunc returns the analysis result for a function literal.
type ResultFunc func(fn *ast.FunctionExpr) *api.FuncResultView

// Config holds dependencies for nested processing.
type Config struct {
	Stdlib        *scope.State
	Store         api.NestedStore
	Graphs        api.GraphProvider
	Check         CheckFunc
	ResultForFunc ResultFunc
	RootResult    *api.FuncResultView
}

// Processor analyzes nested functions for a parent graph.
type Processor struct {
	stdlib        *scope.State
	store         api.NestedStore
	graphs        api.GraphProvider
	check         CheckFunc
	resultForFunc ResultFunc
	rootResult    *api.FuncResultView
}

// New creates a nested processor.
func New(cfg Config) *Processor {
	return &Processor{
		stdlib:        cfg.Stdlib,
		store:         cfg.Store,
		graphs:        cfg.Graphs,
		check:         cfg.Check,
		resultForFunc: cfg.ResultForFunc,
		rootResult:    cfg.RootResult,
	}
}

// ProcessNestedFunctions analyzes all nested function definitions within a parent graph.
func (p *Processor) ProcessNestedFunctions(graph *cfg.Graph, parentResult *api.FuncResultView) {
	if parentResult == nil {
		return
	}

	scopes := parentResult.Scopes
	if scopes == nil {
		return
	}

	// Gather nested function definitions.
	gathered := nested.GatherChildren(graph, scopes, p.stdlib)
	if len(gathered) == 0 {
		return
	}

	// Find the parent function for this graph.
	parentFunc := (*ast.FunctionExpr)(nil)
	if p.store != nil {
		parentFunc = p.store.FuncForGraph(graph)
	}

	// Group by scope and build FuncInfo entries.
	groups := p.groupNestedByScope(gathered)

	// Process each scope group.
	for _, group := range groups {
		p.processNestedGroup(graph, scopes, group, parentResult, parentFunc)
	}
}

// nestedGroup holds a group of functions sharing the same parent scope.
type nestedGroup struct {
	Hash     uint64
	Funcs    []*nested.FuncInfo
	MinPoint cfg.Point
}

// groupNestedByScope groups nested function children by their defining scope hash.
func (p *Processor) groupNestedByScope(gathered []nested.Child) []*nestedGroup {
	scopeGroups := make(map[uint64][]*nested.FuncInfo)

	for i := range gathered {
		child := &gathered[i]
		scopeHash := child.DefScope.GroupHash()

		info := &nested.FuncInfo{Child: *child}

		scopeGroups[scopeHash] = append(scopeGroups[scopeHash], info)
	}

	// Collect groups in deterministic order.
	groups := make([]*nestedGroup, 0, len(scopeGroups))
	for scopeHash, funcs := range scopeGroups {
		if len(funcs) == 0 {
			continue
		}
		sort.SliceStable(funcs, func(i, j int) bool {
			return funcs[i].NF.Point < funcs[j].NF.Point
		})
		groups = append(groups, &nestedGroup{
			Hash:     scopeHash,
			Funcs:    funcs,
			MinPoint: funcs[0].NF.Point,
		})
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].MinPoint != groups[j].MinPoint {
			return groups[i].MinPoint < groups[j].MinPoint
		}
		return groups[i].Hash < groups[j].Hash
	})

	return groups
}

// processNestedGroup processes all functions in a scope group.
func (p *Processor) processNestedGroup(
	graph *cfg.Graph,
	scopes map[cfg.Point]*scope.State,
	group *nestedGroup,
	parentResult *api.FuncResultView,
	parentFunc *ast.FunctionExpr,
) {
	// Build sibling types for this group.
	siblingTypes := p.buildSiblingTypesForGroup(graph, scopes, group.Hash, group.Funcs, parentResult)
	if siblingTypes == nil {
		siblingTypes = make(map[cfg.SymbolID]typ.Type)
	}

	// Process each function in the group.
	for _, info := range group.Funcs {
		p.processNestedFunction(graph, scopes, info, siblingTypes, parentResult, parentFunc)
	}
}

// processNestedFunction analyzes a single nested function.
func (p *Processor) processNestedFunction(
	graph *cfg.Graph,
	scopes map[cfg.Point]*scope.State,
	info *nested.FuncInfo,
	siblingTypes map[cfg.SymbolID]typ.Type,
	parentResult *api.FuncResultView,
	parentFunc *ast.FunctionExpr,
) {
	baseParentScope := scopes[info.NF.Point]
	if baseParentScope == nil {
		baseParentScope = p.stdlib
	}

	parentScope := baseParentScope
	var nestedGraph *cfg.Graph
	if p.graphs != nil {
		nestedGraph = p.graphs.GetOrBuildCFG(info.NF.Func)
	}

	var capturedTypes map[cfg.SymbolID]typ.Type
	if nestedGraph != nil && parentResult != nil {
		capturedTypes = captured.FromParentFacts(parentResult.Facts, nestedGraph, info.NF.Point, nestedGraph.Bindings())
	}
	if nestedGraph != nil && parentResult != nil && parentResult.NarrowSynth != nil {
		bindings := nestedGraph.Bindings()
		if bindings != nil {
			capturedSyms := bindings.CapturedSymbols(info.NF.Func)
			if len(capturedSyms) > 0 {
				capturedSet := make(map[cfg.SymbolID]bool, len(capturedSyms))
				for _, sym := range capturedSyms {
					if sym != 0 {
						capturedSet[sym] = true
					}
				}
				if len(capturedSet) > 0 {
					fields := assign.CollectFieldAssignments(parentResult.Graph, parentResult.NarrowSynth.TypeOf, capturedSet)
					if len(fields) > 0 {
						if capturedTypes == nil {
							capturedTypes = make(map[cfg.SymbolID]typ.Type, len(fields))
						}
						for _, sym := range cfg.SortedSymbolIDs(fields) {
							fieldMap := fields[sym]
							if sym == 0 {
								continue
							}
							base := capturedTypes[sym]
							capturedTypes[sym] = returns.MergeFieldsIntoType(base, fieldMap)
						}
					}
				}
			}
		}
	}

	// For method definitions, bind self to the receiver type.
	if info.FuncDef != nil && info.FuncDef.IsMethod {
		if recvIdent, ok := info.FuncDef.Receiver.(*ast.IdentExpr); ok {
			if bindings := graph.Bindings(); bindings != nil {
				if sym, ok := bindings.SymbolOf(recvIdent); ok {
					selfType := p.resolveSelfTypeForMethod(info, sym, graph, parentResult, p.rootResult)
					if selfType != nil {
						selfType = nested.NormalizeMethodSelfType(selfType)
						parentScope = parentScope.WithSelf(selfType).WithLocalName("self")
					}
				}
			}
		}
	}

	// For methods with self parameter, derive self-type from the owning object.
	if info.FuncDef == nil || !info.FuncDef.IsMethod {
		fn := info.NF.Func
		if phasecore.HasUnannotatedSelfParam(fn, graph.Bindings()) {
			selfType, tblSym := p.resolveSelfTypeForImplicitSelf(info, siblingTypes, graph, parentResult, capturedTypes)
			if selfType != nil && tblSym != 0 && p.store != nil {
				selfType = nested.EnrichSelfTypeWithConstructorFields(selfType, tblSym, &nestedStoreAdapter{store: p.store})
			}
			if selfType != nil {
				parentScope = parentScope.WithSelf(selfType).WithLocalName("self")
			}
		}
	}

	if nestedGraph != nil && len(capturedTypes) > 0 && p.store != nil {
		p.persistCapturedTypesForNestedGraph(nestedGraph, parentScope, capturedTypes)
	}

	// Check the function.
	if p.check != nil {
		p.check(info.NF.Func, parentScope)
	}

	// Get the result for constructor detection and sibling updates.
	result := (*api.FuncResultView)(nil)
	if p.resultForFunc != nil {
		result = p.resultForFunc(info.NF.Func)
	}
	if result == nil {
		return
	}

	// Detect constructor pattern and store instance fields.
	if result.Graph != nil && p.store != nil {
		classSym, selfSym := nested.DetectConstructorPattern(result.Graph, graph, info.NF.Func, info.FuncDef)
		if classSym != 0 && selfSym != 0 {
			var synthFn func(ast.Expr, cfg.Point) typ.Type
			if result.NarrowSynth != nil {
				synthFn = result.NarrowSynth.TypeOf
			}
			fields := nested.CollectConstructorFields(result.Graph, selfSym, synthFn)
			if len(fields) > 0 {
				p.store.StoreConstructorFields(classSym, fields)
			}
		}
	}

	// Update sibling types with the fully-inferred function type.
	if info.IsLocal && info.FuncSym != 0 && result.NarrowSynth != nil {
		if inferredType := result.NarrowSynth.FunctionType(info.NF.Func, parentScope); inferredType != nil {
			siblingTypes[info.FuncSym] = returns.MergeFunctionFactType(siblingTypes[info.FuncSym], inferredType)
		}
	}
}

// resolveSelfTypeForMethod resolves the self-type for a method definition (T:method).
func (p *Processor) resolveSelfTypeForMethod(
	info *nested.FuncInfo,
	sym cfg.SymbolID,
	graph *cfg.Graph,
	parentResult *api.FuncResultView,
	rootResult *api.FuncResultView,
) typ.Type {
	var selfType typ.Type

	// Prefer the explicit type-space binding for `T` in `function T:m(...)`.
	// The receiver value `T` is the class table; the instance/self contract
	// lives in the type namespace binding with the same name.
	if info != nil && info.FuncDef != nil && info.FuncDef.ReceiverName != "" && info.DefScope != nil {
		if named, ok := info.DefScope.LookupType(info.FuncDef.ReceiverName); ok && named != nil {
			selfType = named
		}
	}

	// First try root result facts.
	if selfType == nil && rootResult != nil && rootResult.Facts != nil {
		tv := rootResult.Facts.EffectiveTypeAt(info.NF.Point, sym)
		if tv.Type != nil && tv.State == flow.StateResolved {
			selfType = tv.Type
		}
	}

	// Fall back to parent result facts.
	if selfType == nil && parentResult != nil && parentResult.Facts != nil {
		tv := parentResult.Facts.EffectiveTypeAt(info.NF.Point, sym)
		if tv.Type != nil && tv.State == flow.StateResolved {
			selfType = tv.Type
		}
	}

	// Enrich self-type with constructor instance fields.
	if selfType != nil && p.store != nil {
		selfType = nested.EnrichSelfTypeWithConstructorFields(selfType, sym, &nestedStoreAdapter{store: p.store})
	}

	return selfType
}

func (p *Processor) persistCapturedTypesForNestedGraph(
	nestedGraph *cfg.Graph,
	parentScope *scope.State,
	capturedTypes map[cfg.SymbolID]typ.Type,
) {
	if p.store == nil || nestedGraph == nil || parentScope == nil || len(capturedTypes) == 0 {
		return
	}
	if p.store.GraphParentHashOf(nestedGraph.ID()) == 0 {
		if setter, ok := p.store.(interface {
			SetGraphParentHash(graphID, parentHash uint64)
		}); ok {
			setter.SetGraphParentHash(nestedGraph.ID(), parentScope.Hash())
		}
	}
	key, ok := p.store.GraphKeyFor(nestedGraph, parentScope)
	if !ok {
		return
	}
	nextCaptured := make(api.CapturedTypes, len(capturedTypes))
	for _, sym := range cfg.SortedSymbolIDs(capturedTypes) {
		t := capturedTypes[sym]
		if sym == 0 || t == nil {
			continue
		}
		nextCaptured[sym] = t
	}
	if len(nextCaptured) == 0 {
		return
	}
	p.store.UpdateInterprocFactsNext(key, func(facts *api.Facts) {
		facts.CapturedTypes = returns.WidenCapturedTypes(facts.CapturedTypes, nextCaptured)
	})
}

// resolveSelfTypeForImplicitSelf resolves the self-type for methods with implicit self parameter.
func (p *Processor) resolveSelfTypeForImplicitSelf(
	info *nested.FuncInfo,
	siblingTypes map[cfg.SymbolID]typ.Type,
	graph *cfg.Graph,
	parentResult *api.FuncResultView,
	capturedTypes map[cfg.SymbolID]typ.Type,
) (typ.Type, cfg.SymbolID) {
	fn := info.NF.Func
	var selfType typ.Type
	var tblSym cfg.SymbolID
	var tbl *ast.TableExpr

	// Pattern 1: Table literal methods {m = function(self)...}
	if tbl, tblSym = nested.FindTableLiteralOwner(graph, fn); tbl != nil && tblSym != 0 {
		selfType = siblingTypes[tblSym]
		// Use table literal type when available.
		if selfType == nil && parentResult != nil && parentResult.NarrowSynth != nil {
			selfType = parentResult.NarrowSynth.TypeOf(tbl, info.NF.Point)
		}
		// Use FlowSolution.TypeAt to get field-merged type.
		if selfType == nil && parentResult != nil && parentResult.FlowSolution != nil {
			path := constraint.Path{Symbol: tblSym}
			selfType = parentResult.FlowSolution.TypeAt(info.NF.Point, path)
		}
		// Fall back to Facts.EffectiveTypeAt.
		if selfType == nil && parentResult != nil && parentResult.Facts != nil {
			tv := parentResult.Facts.EffectiveTypeAt(info.NF.Point, tblSym)
			if tv.Type != nil && tv.State == flow.StateResolved {
				selfType = tv.Type
			}
		}
		if rec, ok := selfType.(*typ.Record); ok {
			selfType = nested.EnrichTableTypeWithFuncTypes(rec, tbl, graph, siblingTypes)
		}
	}

	// Pattern 2: Field assignment methods obj.m = function(self)...
	if selfType == nil {
		baseSym, baseTbl, baseTblPoint := nested.FindFieldAssignmentBase(graph, fn, info.NF.Point)
		if baseSym != 0 {
			tblSym = baseSym
			selfType = siblingTypes[baseSym]
			// Use captured types from the parent scope (flow-derived).
			if selfType == nil && len(capturedTypes) > 0 {
				if t := capturedTypes[baseSym]; t != nil {
					selfType = t
				}
			}
			// Use table literal type when available.
			if selfType == nil && baseTbl != nil && parentResult != nil && parentResult.NarrowSynth != nil && baseTblPoint != 0 {
				selfType = parentResult.NarrowSynth.TypeOf(baseTbl, baseTblPoint)
			}
			// Use FlowSolution.TypeAt to get field-merged type.
			if selfType == nil && parentResult != nil && parentResult.FlowSolution != nil {
				path := constraint.Path{Symbol: baseSym}
				selfType = parentResult.FlowSolution.TypeAt(info.NF.Point, path)
			}
			// Fall back to Facts.EffectiveTypeAt.
			if selfType == nil && parentResult != nil && parentResult.Facts != nil {
				tv := parentResult.Facts.EffectiveTypeAt(info.NF.Point, baseSym)
				if tv.Type != nil && tv.State == flow.StateResolved {
					selfType = tv.Type
				}
			}
			if rec, ok := selfType.(*typ.Record); ok && baseTbl != nil {
				selfType = nested.EnrichTableTypeWithFuncTypes(rec, baseTbl, graph, siblingTypes)
			}
		}
	}

	return selfType, tblSym
}

// nestedStoreAdapter implements nested.Store for the enrich functions.
type nestedStoreAdapter struct {
	store api.NestedStore
}

func (s *nestedStoreAdapter) LookupConstructorFields(classSym cfg.SymbolID) map[string]typ.Type {
	if s.store == nil {
		return nil
	}
	return s.store.LookupConstructorFields(classSym)
}

// buildSiblingTypesForGroup computes sibling function types for a scope group.
func (p *Processor) buildSiblingTypesForGroup(
	graph *cfg.Graph,
	scopes map[cfg.Point]*scope.State,
	groupHash uint64,
	funcs []*nested.FuncInfo,
	parentResult *api.FuncResultView,
) map[cfg.SymbolID]typ.Type {
	if p.store == nil || graph == nil || len(funcs) == 0 {
		return nil
	}

	entries := make([]siblings.FuncEntry, len(funcs))
	for i, info := range funcs {
		entries[i] = siblings.FuncEntry{
			Func:    info.NF.Func,
			Point:   info.NF.Point,
			Symbol:  info.FuncSym,
			IsLocal: info.IsLocal,
		}
	}

	bindings := graph.Bindings()

	buildCfg := siblings.BuildConfig{
		Funcs:     entries,
		GroupHash: groupHash,
	}

	// Use canonical local function types (signatures + param hints + return summaries).
	var parentScope *scope.State
	if len(funcs) > 0 {
		parentScope = funcs[0].DefScope
	}
	buildCfg.FuncTypes = p.store.GetLocalFuncTypesSnapshot(graph, parentScope)

	buildCfg.Services = siblings.BuildServicesFuncs{
		CapturedSymbolsFn: func(fn *ast.FunctionExpr) []cfg.SymbolID {
			if bindings == nil {
				return nil
			}
			return bindings.CapturedSymbols(fn)
		},
		TypeAtPointFn: func(point cfg.Point, sym cfg.SymbolID) typ.Type {
			if parentResult == nil || parentResult.Facts == nil {
				return nil
			}
			ref := parentResult.Facts.RefinedAt(point, sym)
			decl := parentResult.Facts.DeclaredAt(point, sym)

			var chosen typ.Type
			if ref.Type != nil && !typ.IsSoft(ref.Type, typ.SoftAnnotationPolicy) {
				chosen = ref.Type
			} else if decl.Type != nil && !typ.IsSoft(decl.Type, typ.SoftAnnotationPolicy) {
				chosen = decl.Type
			} else if ref.State == flow.StateResolved && ref.Type != nil {
				chosen = ref.Type
			} else if decl.State == flow.StateResolved && decl.Type != nil {
				chosen = decl.Type
			} else {
				return nil
			}

			return chosen
		},
		EnrichRecordFn: func(rec *typ.Record, sym cfg.SymbolID) typ.Type {
			if tbl, _ := nested.FindTableLiteralForSymbol(graph, sym); tbl != nil {
				return nested.EnrichTableTypeWithFuncTypes(rec, tbl, graph, buildCfg.FuncTypes)
			}
			return nil
		},
	}

	return siblings.Build(buildCfg)
}
