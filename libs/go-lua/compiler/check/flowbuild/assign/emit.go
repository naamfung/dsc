// Package assign implements assignment constraint extraction for the flow system.
// It processes assignment statements in the CFG and emits type constraints that
// the flow solver uses to track variable types through the program.
//
// # EXTRACTION PIPELINE
//
// The package extracts three types of assignment information:
//
//  1. Variable Assignments: Local declarations and reassignments that establish
//     or update the type of a symbol at a CFG point.
//
//  2. Field Assignments: Table field writes (t.foo = v) that contribute to
//     record type inference for table values.
//
//  3. Function Definitions: Named function definitions (function M.foo()) that
//     add fields to module tables.
//
// # SPEC NARROWING
//
// The package implements spec-based type narrowing where contract specifications
// on function parameters constrain the types of expressions passed to methods.
// For example, if a function is annotated with @spec, the spec types are used
// to narrow parameter types at call sites.
//
// # INFERRED TYPES
//
// For unannotated variables, types are inferred from their initialization
// expressions using the synthesis engine. These inferred types are tracked
// separately from annotated types to support different narrowing behaviors.
package assign

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	cfganalysis "github.com/wippyai/go-lua/compiler/cfg/analysis"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/cond"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/constprop"
	fbcore "github.com/wippyai/go-lua/compiler/check/flowbuild/core"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/decl"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/guard"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/keyscoll"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/mutator"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/path"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/predicate"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/resolve"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/tblutil"
	checkscope "github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// ExtractAssignments extracts assignment info from graph.
func ExtractAssignments(fc *fbcore.FlowContext, inputs *flow.Inputs, keysCollector KeysCollectorFunc) {
	if fc == nil || fc.Graph == nil {
		return
	}
	if inputs == nil {
		return
	}
	derived := fc.Derived
	if derived == nil {
		derived = &fbcore.Derived{}
	}
	synth := derived.Synth
	if synth == nil {
		synth = func(ast.Expr, cfg.Point) typ.Type {
			return typ.Unknown
		}
	}
	symResolver := derived.SymResolver
	if symResolver == nil {
		symResolver = func(cfg.Point, cfg.SymbolID) (typ.Type, bool) {
			return nil, false
		}
	}
	// Worklist fixpoint for spec narrowing:
	// Collects spec-narrowed types from contract specs and propagates through method calls.
	// Uses expandValues with SpecTypes overlay for method call synthesis.
	specNarrowed := CollectSpecNarrowedTypes(fc.Graph, fc.Scopes, synth, symResolver, fc.API, fc.ModuleBindings)
	preflowBranchSolution := buildPreflowBranchSolution(fc, inputs)
	inferredTypes := collectInferredTypes(fc.Graph, fc.Scopes, synth, fc.API, symResolver, specNarrowed, inputs.AnnotatedVars, inputs, fc.ModuleBindings, fc.CallCtx, fc.TypeOps, preflowBranchSolution, fc.Services)
	// Promote inferred parameter types into DeclaredTypes for unannotated params.
	// This enables bidirectional inference at call sites (e.g., custom assert helpers).
	if inputs.DeclaredTypes != nil {
		for _, sym := range fc.Graph.ParamSymbols() {
			if sym == 0 {
				continue
			}
			if inputs.AnnotatedVars != nil && inputs.AnnotatedVars[sym] {
				continue
			}
			inferred := inferredTypes[sym]
			if typ.IsAbsentOrUnknown(inferred) {
				continue
			}
			current := inputs.DeclaredTypes[sym]
			if current == nil || current.Kind().IsPlaceholder() {
				inputs.DeclaredTypes[sym] = inferred
			}
		}
	}

	bindings := fc.Graph.Bindings()

	// Precompute loop variable types for all for loops to improve RHS synthesis.
	// This includes both numeric for loops (integer index) and generic for loops (iterator variables).
	loopVarTypes := make(map[cfg.SymbolID]typ.Type)
	fc.Graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if info == nil || len(info.Targets) == 0 {
			return
		}
		// Handle numeric for loops
		if info.NumericFor != nil {
			target, ok := info.FirstTarget()
			if !ok {
				return
			}
			if target.Kind != cfg.TargetIdent || target.Name == "" || target.Symbol == 0 {
				return
			}
			loopVarTypes[target.Symbol] = typ.Integer
			return
		}
		// Handle generic for loops
		if len(info.IterExprs) > 0 {
			var varTypes []typ.Type
			if fc.API != nil {
				varTypes = fc.API.InferIterVarsWithSpecTypes(info.IterExprs, len(info.Targets), p, nil)
			}
			info.EachTargetSource(func(i int, target cfg.AssignTarget, _ ast.Expr) {
				if target.Kind != cfg.TargetIdent || target.Name == "" || target.Symbol == 0 {
					return
				}
				if i < len(varTypes) && varTypes[i] != nil {
					loopVarTypes[target.Symbol] = varTypes[i]
				}
			})
		}
	})
	var overlayTypes api.SpecTypes
	overlayTypes = mergeSpecTypesInto(overlayTypes, inputs.DeclaredTypes)
	overlayTypes = mergeSpecTypesInto(overlayTypes, inferredTypes)
	overlayTypes = mergeSpecTypesInto(overlayTypes, specNarrowed)
	overlayTypes = mergeSpecTypesInto(overlayTypes, loopVarTypes)
	// Precompute truthy guards: map from CFG point to paths that are narrowed (non-nil) at that point.
	// Used during table literal synthesis to unwrap optional types.
	truthyGuards := guard.CollectTruthyGuards(fc.Graph, bindings)
	typeGuards := guard.CollectTypeGuards(fc.Graph, bindings)

	baseSynth := synthWithOverlayAndPreflow(overlayTypes, bindings, inputs, fc.CallCtx, fc.TypeOps, preflowBranchSolution, synth)
	idom, _ := cfganalysis.ComputeDominators(fc.Graph.CFG())
	structuredWrites := indexStructuredWrites(fc.Graph)
	var wrappedSynth func(ast.Expr, cfg.Point) typ.Type
	wrappedSynth = func(expr ast.Expr, p cfg.Point) typ.Type {
		if table, ok := expr.(*ast.TableExpr); ok && !tblutil.TableHasFunctionField(table) {
			if t := tblutil.SynthTableLiteralWithWrapper(table, p, wrappedSynth); t != nil {
				return t
			}
		}
		// Check if this is an attribute access where the full path has a truthy guard.
		if attr, ok := expr.(*ast.AttrGetExpr); ok {
			t := synth(expr, p)
			if t != nil && bindings != nil {
				if pathKey, ok := guard.TruthyKeyFromExpr(attr, bindings); ok && pathKey.Field != "" {
					if guards, ok := typeGuards[p]; ok {
						if tk, ok := guards[pathKey]; ok && !tk.IsZero() {
							if narrowed := narrow.ByTypeKey(t, tk, nil); narrowed != nil {
								t = narrowed
							}
						}
					}
					if guards, ok := truthyGuards[p]; ok {
						if guards[pathKey] {
							if opt, ok := t.(*typ.Optional); ok {
								return opt.Inner
							}
						}
					}
				}
			}
			return t
		}
		return baseSynth(expr, p)
	}

	// Build a resolver that includes spec-narrowed types
	resolverWithSpec := func(p cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
		if t, ok := loopVarTypes[sym]; ok {
			return t, true
		}
		if t, ok := specNarrowed[sym]; ok {
			return t, true
		}
		return symResolver(p, sym)
	}

	fc.Graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		sc := fc.Scopes[p]

		// Handle numeric for loops
		if info.NumericFor != nil {
			target, ok := info.FirstTarget()
			if !ok {
				return
			}
			if target.Kind != cfg.TargetIdent || target.Name == "" {
				return
			}
			inputs.Assignments = append(inputs.Assignments, flow.UnifiedAssignment{
				Point:      p,
				TargetPath: constraint.Path{Root: resolve.RootName(fc.Graph, target.Symbol, target.Name), Symbol: target.Symbol},
				Type:       typ.Integer,
			})
			return
		}

		// Handle generic for loops
		if len(info.IterExprs) > 0 && len(info.Targets) > 0 {
			var varTypes []typ.Type
			if fc.API != nil {
				varTypes = fc.API.InferIterVarsWithSpecTypes(info.IterExprs, len(info.Targets), p, overlayTypes)
			}
			// Build const resolver for iterator source extraction
			constResolver := predicate.BuildConstResolver(inputs, p)
			iterSource := resolve.ExtractIteratorSource(info.IterExprs, p, wrappedSynth, resolverWithSpec, constResolver, bindings)
			info.EachTargetSource(func(i int, target cfg.AssignTarget, _ ast.Expr) {
				if target.Kind != cfg.TargetIdent || target.Name == "" {
					return
				}
				sym := target.Symbol
				varType := typ.Unknown
				if i < len(varTypes) && varTypes[i] != nil {
					varType = varTypes[i]
				}
				assignment := flow.UnifiedAssignment{
					Point:      p,
					TargetPath: constraint.Path{Root: resolve.RootName(fc.Graph, sym, target.Name), Symbol: sym},
					Type:       resolve.Ref(varType, sc),
				}
				if iterSource != nil {
					assignment.IterSource = &flow.IteratorSource{
						Path:     iterSource.Path,
						Kind:     iterSource.Kind,
						VarIndex: i,
					}
				}
				inputs.Assignments = append(inputs.Assignments, assignment)
			})
			return
		}

		// Build const resolver for this point
		constResolver := predicate.BuildConstResolver(inputs, p)

		var (
			values         []typ.Type
			valuesComputed bool
		)
		ensureValues := func() {
			if valuesComputed {
				return
			}
			// Use pre-assignment symbol overlays for assignment targets so RHS
			// synthesis follows Lua evaluation order (`x = f(x, ...)`).
			rhsOverlay := rhsSpecTypesAtAssignPoint(fc.Graph, info, p, overlayTypes, resolverWithSpec)
			rhsOverlay = enrichStructuredOverlayAtPoint(fc.Graph, idom, structuredWrites, p, rhsOverlay, resolverWithSpec, wrappedSynth)
			values = expandedAssignValues(fc.API, info, p, rhsOverlay)
			valuesComputed = true
		}

		for i, target := range info.Targets {
			source := info.SourceAt(i)

			// Handle identifier targets
			if target.Kind == cfg.TargetIdent {
				name := target.Name
				if name == "" {
					continue
				}

				sym := target.Symbol

				// Determine assigned type using identity-based resolver.
				// For annotated locals, keep the declared type (RHS should not override).
				assignedType := typ.Unknown
				if info.IsLocal {
					if inputs != nil && inputs.AnnotatedVars != nil && inputs.AnnotatedVars[sym] {
						if dt, ok := inputs.DeclaredTypes[sym]; ok && dt != nil {
							assignedType = dt
						}
					} else {
						if t, ok := resolverWithSpec(p, sym); ok && t != nil {
							// Keep previously resolved assignment types only when
							// they carry concrete information. Top-like placeholders
							// (any/unknown/soft) must not block RHS-derived types.
							if !isTopLikeResolvedAssignType(t) {
								assignedType = t
							}
						}
					}
				}
				// Fall back to expression synthesis if no declared/known type
				if typ.IsAbsentOrUnknown(assignedType) {
					ensureValues()
					if value := assignValueAt(values, i); value != nil {
						assignedType = value
						assignedType = preferPreciseDirectSourceType(assignedType, source, p, sc, wrappedSynth, len(info.Targets) == 1)
					} else if wrappedSynth != nil && source != nil {
						assignedType = wrappedSynth(source, p)
					}
				}
				// Override with expanded values if source call has a spec-narrowed receiver.
				// This handles cases like ch:receive() where ch was narrowed by spec.
				// Propagate the result to specNarrowed for subsequent method calls (e.g., msg:from()).
				if call, _ := info.CallForTarget(i); call != nil {
					if call.Receiver != nil {
						if recvSym := call.ReceiverSymbol; recvSym != 0 {
							ensureValues()
							if value := assignValueAt(values, i); value != nil {
								if _, narrowed := specNarrowed[recvSym]; narrowed {
									assignedType = value
									// Propagate to specNarrowed for method calls on this variable
									if !typ.IsAbsentOrUnknown(assignedType) {
										specNarrowed[sym] = assignedType
									}
								}
							}
						}
					}
				}
				if assignedType == nil {
					assignedType = typ.Unknown
				}

				// Use pre-collected spec-narrowed type if available (via SymbolID)
				if narrowed, ok := specNarrowed[sym]; ok {
					assignedType = narrowed
				}

				// Build source path with const resolution and bindings.
				// For dynamic map index reads (t[k]) where k is non-const and
				// sourcePath cannot be represented statically, attach MapElementSource
				// so solve-time propagation can derive the value type from map flow facts.
				var sourcePath constraint.Path
				var mapElementSource *flow.MapElementSource
				if source != nil {
					if sp := path.FromExprWithBindings(source, constResolver, bindings); !sp.IsEmpty() {
						sourcePath = constraint.Path{
							Root:     resolve.RootNameFromBindings(bindings, sp.Symbol, sp.Root),
							Symbol:   sp.Symbol,
							Segments: sp.Segments,
						}
					} else if attr, ok := source.(*ast.AttrGetExpr); ok {
						if _, isStatic := staticSegmentForAttrKey(attr.Key, constResolver); !isStatic {
							if mp := path.FromExprWithBindings(attr.Object, constResolver, bindings); !mp.IsEmpty() && mp.Symbol != 0 {
								mp = constraint.Path{
									Root:     resolve.RootNameFromBindings(bindings, mp.Symbol, mp.Root),
									Symbol:   mp.Symbol,
									Segments: mp.Segments,
								}
								var keySym cfg.SymbolID
								var keyVar string
								if keyIdent, ok := attr.Key.(*ast.IdentExpr); ok && bindings != nil {
									keySym, _ = bindings.SymbolOf(keyIdent)
									keyVar = resolve.RootNameFromBindings(bindings, keySym, keyIdent.Value)
								}
								mapElementSource = &flow.MapElementSource{
									MapPath:   mp,
									KeySymbol: keySym,
									KeyVar:    keyVar,
								}
							}
						}
					}
				}

				// Extract predicate link if RHS is a predicate call.
				if callInfo, retIndex := info.CallForTarget(i); callInfo != nil {
					if link := cond.ExtractPredicateLinkFromCallInfo(callInfo, retIndex, p, sc, inputs, derived.TypeKeyRes, wrappedSynth, derived.RefinementBySym, symResolver, fc.Graph, fc.ModuleBindings); link != nil {
						if retIndex == 1 && callInfo.IsTypeCheck && callInfo.Method == "is" && callInfo.Receiver != nil && derived.TypeKeyRes != nil {
							if typeKey, ok := derived.TypeKeyRes(callInfo.TypeCheckName, sc); ok && !typeKey.IsZero() {
								valuePath := constraint.Path{}
								valIndex := i - retIndex
								if valIndex >= 0 && valIndex < len(info.Targets) {
									valTarget := info.Targets[valIndex]
									if valTarget.Kind == cfg.TargetIdent && valTarget.Name != "" && valTarget.Symbol != 0 {
										valuePath = constraint.Path{
											Root:   resolve.RootName(fc.Graph, valTarget.Symbol, valTarget.Name),
											Symbol: valTarget.Symbol,
										}
									}
								}
								if !valuePath.IsEmpty() {
									link.OnFalsy = constraint.And(link.OnFalsy, constraint.FromConstraints(constraint.HasType{Path: valuePath, Type: typeKey}))
									link.OnTruthy = constraint.And(link.OnTruthy, constraint.FromConstraints(constraint.NotHasType{Path: valuePath, Type: typeKey}))
									link.OnTruthy = constraint.And(link.OnTruthy, constraint.FromConstraints(constraint.IsNil{Path: valuePath}))
									var checkType typ.Type
									if sc != nil {
										if resolved, ok := sc.LookupType(callInfo.TypeCheckName); ok && resolved != nil {
											checkType = resolve.Ref(resolved, sc)
										}
									}
									if checkType != nil {
										baseType := unwrap.Alias(checkType)
										if baseType != nil && !baseType.Kind().IsPlaceholder() && !unwrap.IsOptionalLike(baseType) {
											link.OnFalsy = constraint.And(link.OnFalsy, constraint.FromConstraints(constraint.NotNil{Path: valuePath}))
										}
									}
								}
							}
						}
						varKey := predicate.LinkKey(name, p)
						inputs.PredicateLinks[varKey] = *link
					}
				}

				// Detect keys collector calls: local keys = sorted_keys(table)
				if call, retIndex := info.CallForTarget(i); call != nil {
					var tableSym cfg.SymbolID
					calleeSymbols := callsite.CallableCalleeSymbolCandidates(call, fc.Graph, bindings, fc.ModuleBindings)

					// Try local function analysis first
					if keysCollector != nil {
						tableSym = keysCollector(call, p, retIndex)
					}

					// Fallback: resolve function literal via module bindings.
					if tableSym == 0 && fc.ModuleBindings != nil {
						for _, calleeSym := range calleeSymbols {
							fn, ok := fc.ModuleBindings.FuncLitBySymbol(calleeSym)
							if !ok || fn == nil {
								continue
							}
							if info := keyscoll.DetectKeysCollector(fn); info != nil && info.ReturnIndex == retIndex {
								tableSym = callsite.SymbolOrCreateFieldFromExpr(callsite.RuntimeArgAt(call, info.ParamIndex), bindings)
								break
							}
						}
					}

					// Fallback: check function refinement for KeyOf-based keys collector.
					if tableSym == 0 && derived.RefinementBySym != nil {
						for _, calleeSym := range calleeSymbols {
							eff := derived.RefinementBySym(calleeSym)
							if eff == nil {
								continue
							}
							if paramIdx, keyReturnIdx, ok := eff.KeysCollectorInfo(); ok && retIndex == keyReturnIdx {
								tableSym = callsite.SymbolOrCreateFieldFromExpr(callsite.RuntimeArgAt(call, paramIdx), bindings)
								break
							}
						}
					}

					if tableSym != 0 {
						if inputs.KeysProvenance == nil {
							inputs.KeysProvenance = make(map[cfg.SymbolID]cfg.SymbolID)
						}
						inputs.KeysProvenance[sym] = tableSym
					}
				}

				// Check for container element return effects (e.g., channel:receive())
				var containerElemSrc *flow.ContainerElementSource
				if call, retIndex := info.CallForTarget(i); call != nil {
					assignmentTypesResolver := resolve.BuildAssignmentTypeResolver(inputs)
					if elemInfo := mutator.ContainerElementReturnFromCall(call, p, wrappedSynth, resolverWithSpec, assignmentTypesResolver, fc.Graph, bindings, fc.ModuleBindings); elemInfo != nil {
						// Check if this return index matches
						if elemInfo.ReturnIndex == retIndex {
							// For method calls, index 0 is self (receiver)
							if callsite.IsMethodCallInfo(call) && elemInfo.SourceRef.Index == 0 {
								if recvPath := path.FromExprWithBindings(call.Receiver, constResolver, bindings); !recvPath.IsEmpty() && recvPath.Symbol != 0 {
									containerElemSrc = &flow.ContainerElementSource{
										ContainerPath: constraint.Path{
											Root:     resolve.RootNameFromBindings(bindings, recvPath.Symbol, recvPath.Root),
											Symbol:   recvPath.Symbol,
											Segments: recvPath.Segments,
										},
										ReturnIndex: elemInfo.ReturnIndex,
									}
								}
							}
						}
					}
				}

				inputs.Assignments = append(inputs.Assignments, flow.UnifiedAssignment{
					Point:                  p,
					TargetPath:             constraint.Path{Root: resolve.RootName(fc.Graph, sym, name), Symbol: sym},
					SourcePath:             sourcePath,
					Type:                   resolve.Ref(assignedType, sc),
					ContainerElementSource: containerElemSrc,
					MapElementSource:       mapElementSource,
				})

				// Emit per-field assignments for table literals to enable flow narrowing
				if source != nil {
					if tbl, ok := source.(*ast.TableExpr); ok && !tblutil.TableHasFunctionField(tbl) {
						EmitTableLiteralFieldAssignments(tbl, sym, resolve.RootName(fc.Graph, sym, name), p, bindings, constResolver, wrappedSynth, sc, inputs)
					}
				}
				continue
			}

			// Handle field targets: t.a or t["a"] where "a" is a valid identifier
			if target.Kind == cfg.TargetField && target.BaseName != "" && len(target.FieldPath) > 0 {
				sym := target.BaseSymbol

				// Determine assigned type
				assignedType := typ.Unknown
				// First check expanded values for multi-return assignments
				ensureValues()
				if value := assignValueAt(values, i); !typ.IsAbsentOrUnknown(value) {
					assignedType = value
				} else if source != nil {
					if tbl, ok := source.(*ast.TableExpr); ok && wrappedSynth != nil && !tblutil.TableHasFunctionField(tbl) {
						assignedType = wrappedSynth(source, p)
					} else if wrappedSynth != nil {
						assignedType = wrappedSynth(source, p)
					}
				}
				if assignedType == nil {
					assignedType = typ.Unknown
				}

				// Build segments from field path
				segments := make([]constraint.Segment, len(target.FieldPath))
				for j, field := range target.FieldPath {
					segments[j] = constraint.Segment{Kind: constraint.SegmentField, Name: field}
				}

				// Create assignment for the field path: root.field1.field2... = assignedType
				sourcePath := constraint.Path{}
				if source != nil {
					if sp := path.FromExprWithBindings(source, constResolver, bindings); !sp.IsEmpty() {
						sourcePath = sp
					}
				}
				inputs.Assignments = append(inputs.Assignments, flow.UnifiedAssignment{
					Point: p,
					TargetPath: constraint.Path{
						Root:     resolve.RootName(fc.Graph, sym, target.BaseName),
						Symbol:   sym,
						Segments: segments,
					},
					SourcePath: sourcePath,
					Type:       resolve.Ref(assignedType, sc),
				})
				continue
			}

			// Handle index targets with string literal or const keys: t["key"] = v
			if target.Kind == cfg.TargetIndex {
				var basePath constraint.Path
				if target.BaseName != "" {
					sym := target.BaseSymbol
					basePath = constraint.Path{
						Root:   resolve.RootNameFromBindings(bindings, sym, target.BaseName),
						Symbol: sym,
					}
				} else if target.Base != nil {
					if bp := path.FromExprWithBindings(target.Base, constResolver, bindings); !bp.IsEmpty() && bp.Symbol != 0 {
						basePath = constraint.Path{
							Root:     resolve.RootNameFromBindings(bindings, bp.Symbol, bp.Root),
							Symbol:   bp.Symbol,
							Segments: bp.Segments,
						}
					}
				}
				// Determine assigned type
				assignedType := typ.Unknown
				// First check expanded values for multi-return assignments
				ensureValues()
				if value := assignValueAt(values, i); !typ.IsAbsentOrUnknown(value) {
					assignedType = value
				} else if source != nil {
					if tbl, ok := source.(*ast.TableExpr); ok && wrappedSynth != nil && !tblutil.TableHasFunctionField(tbl) {
						assignedType = wrappedSynth(source, p)
					} else if wrappedSynth != nil {
						assignedType = wrappedSynth(source, p)
					}
				}
				if assignedType == nil {
					assignedType = typ.Unknown
				}

				// Determine static key segment from the key expression.
				var keySeg constraint.Segment
				var keyType typ.Type
				switch k := target.Key.(type) {
				case *ast.StringExpr:
					if seg, ok := path.StaticKeySegment(k); ok {
						keySeg = seg
					}
				case *ast.IdentExpr:
					// Variable key - try const resolution
					if constResolver != nil {
						if val := constResolver(k.Value); val != nil {
							switch val.Kind {
							case flow.ConstString:
								if seg, ok := path.StaticKeySegment(&ast.StringExpr{Value: val.Str}); ok {
									keySeg = seg
								}
							case flow.ConstInt:
								keyType = typ.Integer
							case flow.ConstFloat:
								keyType = typ.Number
							}
						}
					}
				case *ast.NumberExpr:
					// Number literal key - try const resolution
					if val := constprop.ConstValueFromExpr(target.Key); val != nil {
						switch val.Kind {
						case flow.ConstInt:
							keyType = typ.Integer
						case flow.ConstFloat:
							keyType = typ.Number
						}
					}
				}

				if basePath.IsEmpty() {
					if lifted, ok := buildLiftedDynamicIndexerAssignment(
						target,
						source,
						assignedType,
						p,
						sc,
						fc.Graph,
						bindings,
						constResolver,
						wrappedSynth,
						resolverWithSpec,
						truthyGuards,
						typeGuards,
					); ok {
						inputs.IndexerAssignments = append(inputs.IndexerAssignments, lifted)
					}
					continue
				}

				// For non-const keys, emit an IndexerAssignment to widen the table
				if keySeg.Name == "" {
					// Extract key variable name and symbol using bindings.
					var keyVar string
					var keySym cfg.SymbolID
					if keyIdent, ok := target.Key.(*ast.IdentExpr); ok && bindings != nil {
						keySym, _ = bindings.SymbolOf(keyIdent)
						keyVar = resolve.RootNameFromBindings(bindings, keySym, keyIdent.Value)
					}
					// Prefer symbol-based key typing at solve-time so branch narrowing
					// (for example `if suite then suites[suite] = ...`) is preserved.
					if keyType == nil && target.Key != nil && wrappedSynth != nil {
						keyType = wrappedSynth(target.Key, p)
					}
					keyType = canonicalDynamicKeyType(keyType)
					// Apply truthy guards to narrow optional fields in table literals.
					valType := assignedType
					if source != nil && bindings != nil && truthyGuards != nil {
						if tbl, ok := source.(*ast.TableExpr); ok {
							valType = guard.NarrowTableFieldsByGuard(valType, tbl, p, bindings, truthyGuards, typeGuards)
						}
					}
					valuePath := constraint.Path{}
					if source != nil {
						if sp := path.FromExprWithBindings(source, constResolver, bindings); !sp.IsEmpty() {
							valuePath = constraint.Path{
								Root:     resolve.RootNameFromBindings(bindings, sp.Symbol, sp.Root),
								Symbol:   sp.Symbol,
								Segments: sp.Segments,
							}
						}
					}
					resolved := resolve.Ref(valType, sc)
					inputs.IndexerAssignments = append(inputs.IndexerAssignments, flow.IndexerAssignment{
						Point:     p,
						Root:      basePath.Root,
						Symbol:    basePath.Symbol,
						Segments:  basePath.Segments,
						KeyVar:    keyVar,
						KeySymbol: keySym,
						KeyType:   keyType,
						ValuePath: valuePath,
						ValType:   resolved,
					})
					continue
				}

				// Create assignment for the field path: root.fieldName = assignedType
				sourcePath := constraint.Path{}
				if source != nil {
					if sp := path.FromExprWithBindings(source, constResolver, bindings); !sp.IsEmpty() {
						sourcePath = sp
					}
				}
				inputs.Assignments = append(inputs.Assignments, flow.UnifiedAssignment{
					Point: p,
					TargetPath: constraint.Path{
						Root:     basePath.Root,
						Symbol:   basePath.Symbol,
						Segments: append(append([]constraint.Segment{}, basePath.Segments...), keySeg),
					},
					SourcePath: sourcePath,
					Type:       resolve.Ref(assignedType, sc),
				})
				continue
			}
		}

		// Handle sibling assignments from expanding trailing calls.
		if sourceCall, start := info.ExpandingSourceCall(); sourceCall != nil {
			count := len(info.Targets) - start
			symbols := make([]cfg.SymbolID, count)
			names := make([]string, count)
			types := make([]typ.Type, count)
			ensureValues()
			for i := 0; i < count; i++ {
				target, ok := info.TargetAt(start + i)
				if !ok {
					continue
				}
				if target.Kind == cfg.TargetIdent && target.Name != "" {
					names[i] = target.Name
					symbols[i] = target.Symbol
				}
				if value := assignValueAt(values, start+i); value != nil {
					types[i] = value
				}
			}
			correlations, coCorrelations, guardedCorrelations := extractCallCorrelations(sourceCall, wrappedSynth, p, resolverWithSpec, fc.Graph, bindings, fc.ModuleBindings)
			for _, corr := range guardedCorrelations {
				decl.AddTypeKey(inputs, corr.TargetType)
			}
			sibling := &flow.SiblingAssignment{
				Symbols:             symbols,
				Names:               names,
				Types:               types,
				Correlations:        correlations,
				CoCorrelations:      coCorrelations,
				GuardedCorrelations: guardedCorrelations,
			}
			for i, sym := range symbols {
				if sym != 0 && names[i] != "" {
					ver := fc.Graph.VisibleVersion(p, sym)
					if ver.ID == 0 {
						continue
					}
					key := flow.SiblingKey{Symbol: sym, VersionID: ver.ID}
					inputs.SiblingAssignments[key] = sibling
				}
			}
		}
	})
}

type attrChainStep struct {
	KeyExpr ast.Expr
	Seg     constraint.Segment
	Static  bool
}

func buildLiftedDynamicIndexerAssignment(
	target cfg.AssignTarget,
	source ast.Expr,
	assignedType typ.Type,
	p cfg.Point,
	sc *checkscope.State,
	graph *cfg.Graph,
	bindings *bind.BindingTable,
	constResolver func(string) *flow.ConstValue,
	synth func(ast.Expr, cfg.Point) typ.Type,
	symResolver func(cfg.Point, cfg.SymbolID) (typ.Type, bool),
	truthyGuards map[cfg.Point]map[guard.TruthyPathKey]bool,
	typeGuards map[cfg.Point]map[guard.TruthyPathKey]narrow.TypeKey,
) (flow.IndexerAssignment, bool) {
	if target.Expr == nil {
		return flow.IndexerAssignment{}, false
	}

	rootExpr, steps, ok := flattenAttrChain(target.Expr, constResolver)
	if !ok || rootExpr == nil || len(steps) == 0 {
		return flow.IndexerAssignment{}, false
	}

	rootPath := path.FromExprWithBindings(rootExpr, constResolver, bindings)
	if rootPath.IsEmpty() || rootPath.Symbol == 0 {
		return flow.IndexerAssignment{}, false
	}
	rootPath = constraint.Path{
		Root:     resolve.RootNameFromBindings(bindings, rootPath.Symbol, rootPath.Root),
		Symbol:   rootPath.Symbol,
		Segments: rootPath.Segments,
	}

	firstDynamic := -1
	for i, step := range steps {
		if step.Static {
			if firstDynamic == -1 {
				rootPath = rootPath.Append(step.Seg)
			}
			continue
		}
		if firstDynamic == -1 {
			firstDynamic = i
		}
	}
	if firstDynamic == -1 {
		return flow.IndexerAssignment{}, false
	}

	outer := steps[firstDynamic]
	keyVar, keySym, keyType := keyInfoForStep(outer, graph, bindings, synth, symResolver, p, true)

	valType := assignedType
	if source != nil && bindings != nil && truthyGuards != nil {
		if tbl, ok := source.(*ast.TableExpr); ok {
			valType = guard.NarrowTableFieldsByGuard(valType, tbl, p, bindings, truthyGuards, typeGuards)
		}
	}
	valType = resolve.Ref(valType, sc)
	if valType == nil {
		valType = typ.Unknown
	}

	for i := len(steps) - 1; i > firstDynamic; i-- {
		valType = wrapStepValue(steps[i], valType, graph, bindings, synth, symResolver, p)
	}

	valuePath := constraint.Path{}
	if source != nil {
		if sp := path.FromExprWithBindings(source, constResolver, bindings); !sp.IsEmpty() {
			valuePath = constraint.Path{
				Root:     resolve.RootNameFromBindings(bindings, sp.Symbol, sp.Root),
				Symbol:   sp.Symbol,
				Segments: sp.Segments,
			}
		}
	}

	return flow.IndexerAssignment{
		Point:     p,
		Root:      rootPath.Root,
		Symbol:    rootPath.Symbol,
		Segments:  rootPath.Segments,
		KeyVar:    keyVar,
		KeySymbol: keySym,
		KeyType:   keyType,
		ValuePath: valuePath,
		ValType:   valType,
	}, true
}

func flattenAttrChain(expr ast.Expr, constResolver func(string) *flow.ConstValue) (ast.Expr, []attrChainStep, bool) {
	if expr == nil {
		return nil, nil, false
	}
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok {
		return expr, nil, true
	}

	root, steps, ok := flattenAttrChain(attr.Object, constResolver)
	if !ok || root == nil {
		return nil, nil, false
	}

	step := attrChainStep{KeyExpr: attr.Key}
	if seg, ok := staticSegmentForAttrKey(attr.Key, constResolver); ok {
		step.Static = true
		step.Seg = seg
	}

	steps = append(steps, step)
	return root, steps, true
}

func staticSegmentForAttrKey(key ast.Expr, constResolver func(string) *flow.ConstValue) (constraint.Segment, bool) {
	switch k := key.(type) {
	case *ast.StringExpr, *ast.NumberExpr:
		return path.StaticKeySegment(k)
	case *ast.IdentExpr:
		if constResolver == nil {
			return constraint.Segment{}, false
		}
		val := constResolver(k.Value)
		if val == nil {
			return constraint.Segment{}, false
		}
		switch val.Kind {
		case flow.ConstString:
			return path.StaticKeySegment(&ast.StringExpr{Value: val.Str})
		case flow.ConstInt:
			return constraint.Segment{Kind: constraint.SegmentIndexInt, Index: int(val.Int)}, true
		}
	}
	return constraint.Segment{}, false
}

func keyInfoForStep(
	step attrChainStep,
	graph *cfg.Graph,
	bindings *bind.BindingTable,
	synth func(ast.Expr, cfg.Point) typ.Type,
	symResolver func(cfg.Point, cfg.SymbolID) (typ.Type, bool),
	p cfg.Point,
	_ bool,
) (string, cfg.SymbolID, typ.Type) {
	var keyVar string
	var keySym cfg.SymbolID
	if keyIdent, ok := step.KeyExpr.(*ast.IdentExpr); ok && bindings != nil {
		keySym, _ = bindings.SymbolOf(keyIdent)
		keyVar = resolve.RootNameFromBindings(bindings, keySym, keyIdent.Value)
	}

	keyType := inferDynamicKeyType(step, synth, p)
	if typ.IsAbsentOrUnknown(keyType) && keySym != 0 && symResolver != nil {
		if resolved, ok := symResolver(p, keySym); ok && !typ.IsAbsentOrUnknown(resolved) {
			keyType = resolved
		}
	}
	if typ.IsAbsentOrUnknown(keyType) && keySym != 0 {
		if resolved := inferSymbolTypeFromVisibleDef(graph, keySym, p, synth); !typ.IsAbsentOrUnknown(resolved) {
			keyType = resolved
		}
	}
	keyType = canonicalDynamicKeyType(keyType)
	return keyVar, keySym, keyType
}

func inferDynamicKeyType(step attrChainStep, synth func(ast.Expr, cfg.Point) typ.Type, p cfg.Point) typ.Type {
	if step.Static {
		switch step.Seg.Kind {
		case constraint.SegmentIndexInt:
			return typ.Integer
		case constraint.SegmentField, constraint.SegmentIndexString:
			return typ.String
		}
	}

	if step.KeyExpr != nil {
		if val := constprop.ConstValueFromExpr(step.KeyExpr); val != nil {
			switch val.Kind {
			case flow.ConstInt:
				return typ.Integer
			case flow.ConstFloat:
				return typ.Number
			case flow.ConstString:
				return typ.String
			}
		}
		if synth != nil {
			if t := synth(step.KeyExpr, p); !typ.IsAbsentOrUnknown(t) {
				return t
			}
		}
	}

	return typ.Unknown
}

func wrapStepValue(
	step attrChainStep,
	value typ.Type,
	graph *cfg.Graph,
	bindings *bind.BindingTable,
	synth func(ast.Expr, cfg.Point) typ.Type,
	symResolver func(cfg.Point, cfg.SymbolID) (typ.Type, bool),
	p cfg.Point,
) typ.Type {
	if value == nil {
		value = typ.Unknown
	}
	if step.Static {
		switch step.Seg.Kind {
		case constraint.SegmentField:
			return typ.NewRecord().SetOpen(true).Field(step.Seg.Name, value).Build()
		case constraint.SegmentIndexInt:
			return typ.NewMap(typ.Integer, value)
		case constraint.SegmentIndexString:
			return typ.NewMap(typ.String, value)
		}
	}

	_, _, keyType := keyInfoForStep(step, graph, bindings, synth, symResolver, p, false)
	return typ.NewMap(keyType, value)
}

func inferSymbolTypeFromVisibleDef(
	graph *cfg.Graph,
	sym cfg.SymbolID,
	at cfg.Point,
	synth func(ast.Expr, cfg.Point) typ.Type,
) typ.Type {
	if graph == nil || sym == 0 || synth == nil {
		return nil
	}
	ver := graph.VisibleVersion(at, sym)
	if ver.Symbol == 0 || ver.ID == 0 {
		return nil
	}

	var inferred typ.Type
	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if inferred != nil || p > at || info == nil {
			return
		}
		if pv := graph.VisibleVersion(p, sym); pv.Symbol != ver.Symbol || pv.ID != ver.ID {
			return
		}
		info.EachTargetSource(func(i int, target cfg.AssignTarget, source ast.Expr) {
			if inferred != nil {
				return
			}
			if target.Kind != cfg.TargetIdent || target.Symbol != sym || source == nil {
				return
			}
			if t := synth(source, p); !typ.IsAbsentOrUnknown(t) {
				inferred = t
			}
			_ = i
		})
	})
	return inferred
}

// ExtractFuncDefAssignments extracts function definitions as assignments.
// Handles:
// - Local/global function definitions: local function foo() ... end
// - Table field definitions: function M.add() ... end
// - Method definitions: function M:add() ... end
func ExtractFuncDefAssignments(fc *fbcore.FlowContext, inputs *flow.Inputs) {
	fc.Graph.EachFuncDef(func(p cfg.Point, info *cfg.FuncDefInfo) {
		sc := fc.Scopes[p]

		// Handle local/global function definitions
		if info.TargetKind == cfg.FuncDefGlobal {
			if info.Symbol == 0 || info.Name == "" {
				return
			}
			// Skip if already in DeclaredTypes (has explicit return types)
			if _, exists := inputs.DeclaredTypes[info.Symbol]; exists {
				return
			}
			// Synthesize the function type with inferred returns
			var fnType typ.Type
			if fc.API != nil && info.FuncExpr != nil {
				fnType = fc.API.TypeOf(info.FuncExpr, p)
			}
			if fnType == nil {
				fnType = typ.Unknown
			}
			// Create assignment for the function variable
			inputs.Assignments = append(inputs.Assignments, flow.UnifiedAssignment{
				Point: p,
				TargetPath: constraint.Path{
					Root:   resolve.RootName(fc.Graph, info.Symbol, info.Name),
					Symbol: info.Symbol,
				},
				Type: resolve.Ref(fnType, sc),
			})
			return
		}

		// Handle field and method definitions on receivers
		if info.TargetKind != cfg.FuncDefField && info.TargetKind != cfg.FuncDefMethod {
			return
		}

		if info.ReceiverName == "" {
			return
		}
		sym := info.ReceiverSymbol
		// ReceiverSymbol should be populated by the binder. Fallback to bindings lookup
		// if not set (for receivers that are simple identifiers).
		if sym == 0 {
			if bindings := fc.Graph.Bindings(); bindings != nil {
				if recvIdent, ok := info.Receiver.(*ast.IdentExpr); ok {
					sym, _ = bindings.SymbolOf(recvIdent)
				}
			}
		}
		if sym == 0 {
			return
		}
		root := resolve.RootNameFromBindings(fc.Graph.Bindings(), sym, info.ReceiverName)

		// Synthesize the function type
		var fnType typ.Type
		if fc.API != nil && info.FuncExpr != nil {
			fnType = fc.API.TypeOf(info.FuncExpr, p)
		}
		if fnType == nil {
			fnType = typ.Unknown
		}

		// Create sub-path assignment: M.add = function
		inputs.Assignments = append(inputs.Assignments, flow.UnifiedAssignment{
			Point: p,
			TargetPath: constraint.Path{
				Root:     root,
				Symbol:   sym,
				Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: info.Name}},
			},
			Type: resolve.Ref(fnType, sc),
		})
	})
}

func isTopLikeResolvedAssignType(t typ.Type) bool {
	if t == nil {
		return true
	}
	t = typ.PruneSoftUnionMembers(t)
	if typ.IsAny(t) || typ.IsUnknown(t) || typ.IsSoft(t, typ.SoftAnnotationPolicy) {
		return true
	}

	switch v := unwrap.Alias(t).(type) {
	case *typ.Optional:
		return isTopLikeResolvedAssignType(v.Inner)
	case *typ.Union:
		if len(v.Members) == 0 {
			return true
		}
		for _, m := range v.Members {
			if m == nil || m.Kind() == kind.Nil {
				continue
			}
			if !isTopLikeResolvedAssignType(m) {
				return false
			}
		}
		return true
	}

	return false
}

// extractCallCorrelations extracts ErrorReturn and CorrelatedReturn correlations from the callee's spec.
// Callee type resolution is delegated to resolve.CalleeType to keep call semantics
// canonical across flowbuild passes.
func extractCallCorrelations(
	callInfo *cfg.CallInfo,
	synth func(ast.Expr, cfg.Point) typ.Type,
	p cfg.Point,
	symResolver func(cfg.Point, cfg.SymbolID) (typ.Type, bool),
	graph *cfg.Graph,
	bindings *bind.BindingTable,
	moduleBindings *bind.BindingTable,
) ([]flow.ReturnCorrelation, []flow.ReturnCorrelation, []flow.GuardedTypeCorrelation) {
	if callInfo == nil {
		return nil, nil, nil
	}
	fnType := resolve.CalleeType(callInfo, p, synth, symResolver, nil, graph, bindings, moduleBindings)
	inv, co := correlationsFromFunctionType(fnType)
	guarded := guardedTypeCorrelationsFromCall(fnType, callInfo, synth, p)
	return inv, co, guarded
}

// correlationsFromFunctionType extracts ErrorReturn and CorrelatedReturn labels from a function's spec effects.
// Returns (inverse correlations, co-correlations).
func correlationsFromFunctionType(fnType typ.Type) ([]flow.ReturnCorrelation, []flow.ReturnCorrelation) {
	if fnType == nil {
		return nil, nil
	}
	spec := contract.ExtractSpec(fnType)
	var inverse []flow.ReturnCorrelation
	var coCorr []flow.ReturnCorrelation
	if spec != nil {
		for _, label := range spec.Effects.Labels {
			if er, ok := label.(effect.ErrorReturn); ok {
				inverse = append(inverse, flow.ReturnCorrelation{
					ValueIndex: er.ValueIndex,
					ErrorIndex: er.ErrorIndex,
				})
			}
			if cr, ok := label.(effect.CorrelatedReturn); ok {
				// Expand pairwise: each pair of indices forms a co-correlation
				for i := 0; i < len(cr.Indices); i++ {
					for j := i + 1; j < len(cr.Indices); j++ {
						coCorr = append(coCorr, flow.ReturnCorrelation{
							ValueIndex: cr.Indices[i],
							ErrorIndex: cr.Indices[j],
						})
					}
				}
			}
		}
	}
	if len(inverse) == 0 && len(coCorr) == 0 {
		convInv, convCo := InferErrorReturnConvention(fnType)
		if len(convInv) > 0 {
			inverse = append(inverse, convInv...)
		}
		if len(convCo) > 0 {
			coCorr = append(coCorr, convCo...)
		}
	}
	return inverse, coCorr
}

func guardedTypeCorrelationsFromCall(
	fnType typ.Type,
	callInfo *cfg.CallInfo,
	synth func(ast.Expr, cfg.Point) typ.Type,
	p cfg.Point,
) []flow.GuardedTypeCorrelation {
	if fnType == nil || callInfo == nil || synth == nil {
		return nil
	}
	fn := unwrap.Function(fnType)
	if fn == nil {
		return nil
	}
	guardIdx, ok := firstBooleanReturnIndex(fn)
	if !ok {
		return nil
	}
	spec := contract.ExtractSpec(fnType)
	if spec == nil {
		return nil
	}

	var out []flow.GuardedTypeCorrelation
	for _, label := range spec.Effects.Labels {
		ret, ok := label.(effect.Return)
		if !ok || ret.Transform == nil || ret.ReturnIndex < 0 {
			continue
		}
		cb, ok := ret.Transform.(effect.CallbackReturn)
		if !ok {
			continue
		}
		argIdx, ok := effect.ResolveParamIndex(cb.CallbackParam, len(callInfo.Args))
		if !ok || argIdx < 0 || argIdx >= len(callInfo.Args) {
			continue
		}
		arg := callInfo.Args[argIdx]
		if arg == nil {
			continue
		}
		targetType := firstCallableReturnType(synth(arg, p))
		if targetType == nil || typ.IsAny(targetType) || typ.IsUnknown(targetType) {
			continue
		}
		out = append(out, flow.GuardedTypeCorrelation{
			GuardIndex:    guardIdx,
			TargetIndex:   ret.ReturnIndex,
			GuardOnTruthy: true,
			TargetType:    targetType,
		})
	}
	return out
}

func firstBooleanReturnIndex(fn *typ.Function) (int, bool) {
	if fn == nil {
		return 0, false
	}
	for i, ret := range fn.Returns {
		if isBooleanType(ret) {
			return i, true
		}
	}
	return 0, false
}

func isBooleanType(t typ.Type) bool {
	t = unwrap.Alias(t)
	if t == nil {
		return false
	}
	switch v := t.(type) {
	case *typ.Literal:
		return v.Base == kind.Boolean
	case *typ.Optional:
		return isBooleanType(v.Inner)
	case *typ.Union:
		seen := false
		for _, m := range v.Members {
			if m == nil || m.Kind() == kind.Nil {
				continue
			}
			if !isBooleanType(m) {
				return false
			}
			seen = true
		}
		return seen
	default:
		return t.Kind() == kind.Boolean
	}
}

func firstCallableReturnType(t typ.Type) typ.Type {
	t = unwrap.Alias(t)
	if t == nil {
		return nil
	}
	switch v := t.(type) {
	case *typ.Function:
		if len(v.Returns) == 0 || v.Returns[0] == nil {
			return nil
		}
		return v.Returns[0]
	case *typ.Optional:
		return firstCallableReturnType(v.Inner)
	case *typ.Union:
		var retTypes []typ.Type
		for _, m := range v.Members {
			if rt := firstCallableReturnType(m); rt != nil {
				retTypes = append(retTypes, rt)
			}
		}
		if len(retTypes) == 0 {
			return nil
		}
		return typ.NewUnion(retTypes...)
	default:
		if typ.IsAny(t) {
			return typ.Any
		}
		if typ.IsUnknown(t) {
			return typ.Unknown
		}
		return nil
	}
}
