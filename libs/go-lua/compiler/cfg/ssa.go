package cfg

import (
	"slices"
	"unsafe"

	"github.com/wippyai/go-lua/compiler/cfg/analysis"
	basecfg "github.com/wippyai/go-lua/types/cfg"
)

// mapPtr returns a uintptr that uniquely identifies a map instance.
func mapPtr(m map[string]basecfg.SymbolID) uintptr {
	return *(*uintptr)(unsafe.Pointer(&m))
}

// ComputeSSAVersions computes SSA versions for all variables and inserts phi nodes.
// Uses Cytron et al. dominance-frontier phi placement + dominator-tree rename.
func (b *Builder) ComputeSSAVersions() {
	assignedSyms := b.collectAssignedSymbols()
	if len(assignedSyms) == 0 {
		return
	}

	defPoints := b.collectDefPoints(assignedSyms)
	domInfo := analysis.ComputeDomInfoDense(b.Cfg)
	phiSites := b.placePhis(assignedSyms, defPoints, domInfo.DominanceFrontier)
	b.renameSSA(assignedSyms, defPoints, phiSites, domInfo.DominatorTree)
}

// collectAssignedSymbols returns the set of SymbolIDs that need SSA versions.
func (b *Builder) collectAssignedSymbols() map[basecfg.SymbolID]string {
	syms := make(map[basecfg.SymbolID]string)

	for _, info := range b.Info {
		switch v := info.(type) {
		case *AssignInfo:
			for _, target := range v.Targets {
				sym, name := assignmentTargetSymbol(target)
				if sym != 0 {
					syms[sym] = name
				}
			}
		case *FuncDefInfo:
			if v.Symbol != 0 && v.TargetKind == FuncDefGlobal {
				syms[v.Symbol] = v.Name
			}
		}
	}

	if b.ScopeTracker != nil {
		for sym, declPoint := range b.ScopeTracker.declPoints {
			if declPoint == 0 {
				if name := b.ScopeTracker.symbolNames[sym]; name != "" {
					syms[sym] = name
				}
			}
		}
	}

	if b.Cfg != nil && b.ScopeTracker != nil {
		entry := b.Cfg.Entry()
		if vis := b.ScopeTracker.visibility[entry]; vis != nil {
			for name, sym := range vis {
				if _, exists := syms[sym]; !exists {
					syms[sym] = name
				}
			}
			for name, sym := range b.ScopeTracker.globals {
				if _, inLocal := vis[name]; inLocal {
					continue
				}
				if _, exists := syms[sym]; !exists {
					syms[sym] = name
				}
			}
		}
	}

	return syms
}

// collectDefPoints returns the set of CFG points where each symbol is defined.
func (b *Builder) collectDefPoints(assignedSyms map[basecfg.SymbolID]string) map[basecfg.SymbolID][]basecfg.Point {
	if len(assignedSyms) == 0 {
		return map[basecfg.SymbolID][]basecfg.Point{}
	}

	defCounts := make(map[basecfg.SymbolID]int, len(assignedSyms))
	entry := b.Cfg.Entry()

	if b.ScopeTracker != nil {
		for sym := range assignedSyms {
			if declPoint, ok := b.ScopeTracker.declPoints[sym]; ok && declPoint == 0 {
				defCounts[sym]++
			}
		}
	}

	// Count defs first to avoid repeated slice growth while appending.
	for pi := 0; pi < b.Cfg.Size(); pi++ {
		p := basecfg.Point(pi)
		info := b.Info[p]
		if info == nil {
			continue
		}
		switch v := info.(type) {
		case *AssignInfo:
			for _, target := range v.Targets {
				sym, _ := assignmentTargetSymbol(target)
				if sym == 0 {
					continue
				}
				if _, ok := assignedSyms[sym]; ok {
					defCounts[sym]++
				}
			}
		case *FuncDefInfo:
			if v.Symbol != 0 && v.TargetKind == FuncDefGlobal {
				if _, ok := assignedSyms[v.Symbol]; ok {
					defCounts[v.Symbol]++
				}
			}
		}
	}

	defPoints := make(map[basecfg.SymbolID][]basecfg.Point, len(defCounts))
	for sym, count := range defCounts {
		if count > 0 {
			defPoints[sym] = make([]basecfg.Point, 0, count)
		}
	}

	if b.ScopeTracker != nil {
		for sym := range assignedSyms {
			if declPoint, ok := b.ScopeTracker.declPoints[sym]; ok && declPoint == 0 {
				defPoints[sym] = append(defPoints[sym], entry)
			}
		}
	}

	for pi := 0; pi < b.Cfg.Size(); pi++ {
		p := basecfg.Point(pi)
		info := b.Info[p]
		if info == nil {
			continue
		}
		switch v := info.(type) {
		case *AssignInfo:
			for _, target := range v.Targets {
				sym, _ := assignmentTargetSymbol(target)
				if sym == 0 {
					continue
				}
				if _, ok := assignedSyms[sym]; ok {
					defPoints[sym] = append(defPoints[sym], p)
				}
			}
		case *FuncDefInfo:
			if v.Symbol != 0 && v.TargetKind == FuncDefGlobal {
				if _, ok := assignedSyms[v.Symbol]; ok {
					defPoints[v.Symbol] = append(defPoints[v.Symbol], p)
				}
			}
		}
	}

	return defPoints
}

// placePhis uses the iterated dominance frontier to determine where phi nodes are needed.
func (b *Builder) placePhis(
	assignedSyms map[basecfg.SymbolID]string,
	defPoints map[basecfg.SymbolID][]basecfg.Point,
	df [][]basecfg.Point,
) map[basecfg.Point][]basecfg.SymbolID {
	phiSites := make(map[basecfg.Point][]basecfg.SymbolID)
	if b.ScopeTracker == nil {
		return phiSites
	}

	sortedSyms := make([]basecfg.SymbolID, 0, len(assignedSyms))

	for sym := range assignedSyms {
		sortedSyms = append(sortedSyms, sym)
	}

	slices.Sort(sortedSyms)
	cfgSize := b.Cfg.Size()
	visibility := b.ScopeTracker.visibility
	globals := b.ScopeTracker.globals
	hasAlreadyMarks := make([]int, cfgSize)
	everOnWorklistMarks := make([]int, cfgSize)
	markEpoch := 0

	for _, sym := range sortedSyms {
		name := assignedSyms[sym]
		defs := defPoints[sym]

		if len(defs) == 0 {
			continue
		}

		markEpoch++
		worklist := make([]basecfg.Point, 0, len(defs))
		for _, d := range defs {
			di := int(d)
			if di < 0 || di >= cfgSize {
				continue
			}
			if everOnWorklistMarks[di] == markEpoch {
				continue
			}
			everOnWorklistMarks[di] = markEpoch
			worklist = append(worklist, d)
		}

		for len(worklist) > 0 {
			d := worklist[len(worklist)-1]
			worklist = worklist[:len(worklist)-1]

			dIdx := int(d)
			if dIdx < 0 || dIdx >= len(df) {
				continue
			}
			for _, y := range df[dIdx] {
				yi := int(y)
				if yi < 0 || yi >= cfgSize {
					continue
				}
				if hasAlreadyMarks[yi] == markEpoch {
					continue
				}

				vis := visibility[y]
				if vis == nil {
					continue
				}
				resolvedSym, ok := vis[name]
				if !ok {
					resolvedSym, ok = globals[name]
				}
				if !ok || resolvedSym != sym {
					continue
				}

				hasAlreadyMarks[yi] = markEpoch

				phiSites[y] = append(phiSites[y], sym)

				if everOnWorklistMarks[yi] != markEpoch {
					everOnWorklistMarks[yi] = markEpoch
					worklist = append(worklist, y)
				}
			}
		}
	}

	return phiSites
}

// renameSSA performs dominator-tree preorder traversal to assign SSA version numbers.
func (b *Builder) renameSSA(
	assignedSyms map[basecfg.SymbolID]string,
	defPoints map[basecfg.SymbolID][]basecfg.Point,
	phiSites map[basecfg.Point][]basecfg.SymbolID,
	domTree [][]basecfg.Point,
) {
	entry := b.Cfg.Entry()

	sortedAssignedSyms := make([]basecfg.SymbolID, 0, len(assignedSyms))
	for sym := range assignedSyms {
		sortedAssignedSyms = append(sortedAssignedSyms, sym)
	}
	slices.Sort(sortedAssignedSyms)

	var symIndexMap map[basecfg.SymbolID]int
	var symIndexDense []int
	if len(sortedAssignedSyms) > 0 {
		maxSym := int(sortedAssignedSyms[len(sortedAssignedSyms)-1])
		// Prefer dense lookup only when symbol IDs are reasonably compact.
		if maxSym > 0 && maxSym <= 16384 && maxSym <= len(sortedAssignedSyms)*8 {
			symIndexDense = make([]int, maxSym+1)
		} else {
			symIndexMap = make(map[basecfg.SymbolID]int, len(sortedAssignedSyms))
		}
	} else {
		symIndexMap = make(map[basecfg.SymbolID]int)
	}
	symByIndex := make([]basecfg.SymbolID, len(sortedAssignedSyms))
	rootByIndex := make([]string, len(sortedAssignedSyms))
	nextVersionID := make([]int, len(sortedAssignedSyms))
	touchedVersionID := make([]bool, len(sortedAssignedSyms))
	stacks := make([][]Version, len(sortedAssignedSyms))

	for i, sym := range sortedAssignedSyms {
		if symIndexDense != nil {
			// Store index+1 so zero remains the "not present" sentinel.
			symIndexDense[int(sym)] = i + 1
		} else {
			symIndexMap[sym] = i
		}
		symByIndex[i] = sym
		rootByIndex[i] = assignedSyms[sym]
		nextVersionID[i] = b.NextVersionID[sym]

		// Keep initial stack capacity modest; growth is cheap and many symbols
		// only need a small number of active versions.
		capacity := len(defPoints[sym]) + 1
		if capacity < 2 {
			capacity = 2
		}
		if capacity > 3 {
			capacity = 3
		}
		stacks[i] = make([]Version, 0, capacity)
	}

	lookupSymIndex := func(sym basecfg.SymbolID) (int, bool) {
		if symIndexDense != nil {
			symInt := int(sym)
			if symInt < 0 || symInt >= len(symIndexDense) {
				return 0, false
			}
			idx := symIndexDense[symInt]
			if idx == 0 {
				return 0, false
			}
			return idx - 1, true
		}
		idx, ok := symIndexMap[sym]
		return idx, ok
	}

	type phiEntry struct {
		symIdx int
		phi    PhiInfo
	}

	phiByPoint := make(map[basecfg.Point][]phiEntry, len(phiSites))
	for p, ordered := range phiSites {
		operandCap := len(b.Cfg.PredecessorsReadOnly(p))
		entries := make([]phiEntry, 0, len(ordered))
		for _, sym := range ordered {
			symIdx, ok := lookupSymIndex(sym)
			if !ok {
				continue
			}
			entries = append(entries, phiEntry{
				symIdx: symIdx,
				phi: PhiInfo{
					Point:    p,
					Target:   Version{Root: rootByIndex[symIdx], Symbol: sym},
					Operands: make([]PhiOperand, 0, operandCap),
				},
			})
		}
		if len(entries) > 0 {
			phiByPoint[p] = entries
		}
	}

	var seededGlobals []int
	if b.ScopeTracker != nil {
		seededGlobals = make([]int, 0, len(sortedAssignedSyms))
		for _, sym := range sortedAssignedSyms {
			if declPoint, ok := b.ScopeTracker.declPoints[sym]; ok && declPoint == 0 {
				if symIdx, found := lookupSymIndex(sym); found {
					seededGlobals = append(seededGlobals, symIdx)
				}
			}
		}
	}

	newVersion := func(symIdx int) Version {
		nextVersionID[symIdx]++
		touchedVersionID[symIdx] = true

		return Version{
			Root:   rootByIndex[symIdx],
			Symbol: symByIndex[symIdx],
			ID:     nextVersionID[symIdx],
		}
	}

	// pushedEntry stores a symbol index and its push count for the small array optimization.
	type pushedEntry struct {
		symIdx int
		count  int
	}

	// Cache visible assigned symbols by visibility map pointer to avoid per-point scans.
	visibility := b.ScopeTracker.visibility
	globals := b.ScopeTracker.globals
	type globalAssignedSym struct {
		name   string
		symIdx int
	}
	globalAssigned := make([]globalAssignedSym, 0, len(globals))
	for name, resolvedSym := range globals {
		if symIdx, ok := lookupSymIndex(resolvedSym); ok && rootByIndex[symIdx] == name {
			globalAssigned = append(globalAssigned, globalAssignedSym{name: name, symIdx: symIdx})
		}
	}

	// Count visibility-map pointer reuse only for small graphs.
	// On larger graphs this pre-pass can allocate substantially more than it saves.
	var visRefCount map[uintptr]int
	var visAssignedCache map[uintptr][]int
	if len(visibility) <= 512 {
		visRefCount = make(map[uintptr]int, len(visibility))
		for _, visLocal := range visibility {
			if visLocal != nil {
				visRefCount[mapPtr(visLocal)]++
			}
		}
		cacheCap := len(visRefCount)
		if cacheCap > 128 {
			cacheCap = 128
		}
		visAssignedCache = make(map[uintptr][]int, cacheCap)
	}
	visibleVersionByPoint := b.VisibleVersionByPoint
	cfgSize := b.Cfg.Size()
	if len(visibleVersionByPoint) < cfgSize {
		visibleVersionByPoint = make([]map[basecfg.SymbolID]Version, cfgSize)
	} else {
		visibleVersionByPoint = visibleVersionByPoint[:cfgSize]
	}

	// Track parent's VisibleVersion for sharing when unchanged.
	type renameState struct {
		parentVersions map[basecfg.SymbolID]Version
		parentVisPtr   uintptr // pointer to parent's visibility map for identity check
	}

	var rename func(p basecfg.Point, state renameState)
	rename = func(p basecfg.Point, state renameState) {
		// Use small fixed array for common case (most points have 0-4 pushes).
		var pushedArr [8]pushedEntry

		pushedLen := 0

		var pushedOverflow []pushedEntry // only allocated if overflow

		addPush := func(symIdx int) {
			// Check array first.
			for i := range pushedLen {
				if pushedArr[i].symIdx == symIdx {
					pushedArr[i].count++

					return
				}
			}
			// Add to array if space.
			if pushedLen < len(pushedArr) {
				pushedArr[pushedLen] = pushedEntry{symIdx: symIdx, count: 1}
				pushedLen++

				return
			}
			// Overflow to a compact slice. Overflow is rare in practice.
			for i := range pushedOverflow {
				if pushedOverflow[i].symIdx == symIdx {
					pushedOverflow[i].count++
					return
				}
			}
			pushedOverflow = append(pushedOverflow, pushedEntry{symIdx: symIdx, count: 1})
		}

		if phiEntries := phiByPoint[p]; len(phiEntries) > 0 {
			for i := range phiEntries {
				entry := &phiEntries[i]
				ver := newVersion(entry.symIdx)
				entry.phi.Target = ver
				stacks[entry.symIdx] = append(stacks[entry.symIdx], ver)
				addPush(entry.symIdx)
			}
		}

		if p == entry {
			for _, symIdx := range seededGlobals {
				ver := newVersion(symIdx)
				stacks[symIdx] = append(stacks[symIdx], ver)
				addPush(symIdx)
			}
		}

		if info := b.Info[p]; info != nil {
			switch v := info.(type) {
			case *AssignInfo:
				if len(v.TargetVersions) < len(v.Targets) {
					if len(v.Targets) == 1 {
						v.TargetVersions = v.singleTargetVersion[:]
					} else {
						targetVersions := make([]Version, len(v.Targets))
						copy(targetVersions, v.TargetVersions)
						v.TargetVersions = targetVersions
					}
				}
				for i, target := range v.Targets {
					sym, _ := assignmentTargetSymbol(target)
					if sym == 0 {
						continue
					}
					if symIdx, ok := lookupSymIndex(sym); ok {
						ver := newVersion(symIdx)
						stacks[symIdx] = append(stacks[symIdx], ver)
						addPush(symIdx)
						v.TargetVersions[i] = ver
					}
				}
			case *FuncDefInfo:
				if v.Symbol != 0 && v.TargetKind == FuncDefGlobal {
					if symIdx, ok := lookupSymIndex(v.Symbol); ok {
						ver := newVersion(symIdx)
						stacks[symIdx] = append(stacks[symIdx], ver)
						addPush(symIdx)
					}
				}
			}
		}

		var (
			currentVersions map[basecfg.SymbolID]Version
			currentVisPtr   uintptr
		)

		pIdx := int(p)
		if visLocal := visibility[p]; visLocal != nil {
			currentVisPtr = mapPtr(visLocal)
			noPushes := pushedLen == 0 && len(pushedOverflow) == 0
			sameVisibilityAsParent := state.parentVersions != nil && currentVisPtr == state.parentVisPtr

			// Reuse parent's VisibleVersion if visibility unchanged and no pushes.
			if noPushes && sameVisibilityAsParent {
				visibleVersionByPoint[pIdx] = state.parentVersions
				currentVersions = state.parentVersions
			} else if sameVisibilityAsParent {
				hasVisiblePush := false
				isVisiblePushedSym := func(symIdx int) bool {
					name := rootByIndex[symIdx]
					sym := symByIndex[symIdx]
					if resolvedSym, ok := visLocal[name]; ok {
						return resolvedSym == sym
					}
					if resolvedSym, ok := globals[name]; ok {
						return resolvedSym == sym
					}
					return false
				}
				for i := range pushedLen {
					if isVisiblePushedSym(pushedArr[i].symIdx) {
						hasVisiblePush = true
						break
					}
				}
				if !hasVisiblePush {
					for i := range pushedOverflow {
						if isVisiblePushedSym(pushedOverflow[i].symIdx) {
							hasVisiblePush = true
							break
						}
					}
				}
				if !hasVisiblePush {
					visibleVersionByPoint[pIdx] = state.parentVersions
					currentVersions = state.parentVersions
				} else {
					// Visibility unchanged: copy parent once and patch only locally updated symbols.
					currentVersions = make(map[basecfg.SymbolID]Version, len(state.parentVersions)+pushedLen+len(pushedOverflow))
					for sym, ver := range state.parentVersions {
						currentVersions[sym] = ver
					}
					for i := range pushedLen {
						symIdx := pushedArr[i].symIdx
						if stack := stacks[symIdx]; len(stack) > 0 {
							currentVersions[symByIndex[symIdx]] = stack[len(stack)-1]
						}
					}
					for i := range pushedOverflow {
						symIdx := pushedOverflow[i].symIdx
						if stack := stacks[symIdx]; len(stack) > 0 {
							currentVersions[symByIndex[symIdx]] = stack[len(stack)-1]
						}
					}
					visibleVersionByPoint[pIdx] = currentVersions
				}
			} else if visAssignedCache != nil && visRefCount[currentVisPtr] > 1 {
				visibleAssigned, cached := visAssignedCache[currentVisPtr]
				if !cached {
					visibleAssigned = make([]int, 0, len(visLocal)+len(globalAssigned))
					for name, resolvedSym := range visLocal {
						if symIdx, ok := lookupSymIndex(resolvedSym); ok && rootByIndex[symIdx] == name {
							visibleAssigned = append(visibleAssigned, symIdx)
						}
					}
					for _, globalInfo := range globalAssigned {
						if _, shadowed := visLocal[globalInfo.name]; shadowed {
							continue
						}
						visibleAssigned = append(visibleAssigned, globalInfo.symIdx)
					}
					visAssignedCache[currentVisPtr] = visibleAssigned
				}
				activeCount := 0
				for _, symIdx := range visibleAssigned {
					if stack := stacks[symIdx]; len(stack) > 0 {
						activeCount++
					}
				}
				if activeCount > 0 {
					currentVersions = make(map[basecfg.SymbolID]Version, activeCount)
					visibleVersionByPoint[pIdx] = currentVersions
					for _, symIdx := range visibleAssigned {
						if stack := stacks[symIdx]; len(stack) > 0 {
							currentVersions[symByIndex[symIdx]] = stack[len(stack)-1]
						}
					}
				}
			} else {
				estimatedCap := len(visLocal) + len(globalAssigned)
				for name, resolvedSym := range visLocal {
					symIdx, ok := lookupSymIndex(resolvedSym)
					if !ok || rootByIndex[symIdx] != name {
						continue
					}
					if stack := stacks[symIdx]; len(stack) > 0 {
						if currentVersions == nil {
							currentVersions = make(map[basecfg.SymbolID]Version, estimatedCap)
							visibleVersionByPoint[pIdx] = currentVersions
						}
						currentVersions[symByIndex[symIdx]] = stack[len(stack)-1]
					}
				}
				for _, globalInfo := range globalAssigned {
					if _, shadowed := visLocal[globalInfo.name]; shadowed {
						continue
					}
					symIdx := globalInfo.symIdx
					if stack := stacks[symIdx]; len(stack) > 0 {
						if currentVersions == nil {
							currentVersions = make(map[basecfg.SymbolID]Version, estimatedCap)
							visibleVersionByPoint[pIdx] = currentVersions
						}
						currentVersions[symByIndex[symIdx]] = stack[len(stack)-1]
					}
				}
			}
		}

		emitPhiOperands := func(succ basecfg.Point) {
			if succPhiEntries := phiByPoint[succ]; len(succPhiEntries) > 0 {
				for i := range succPhiEntries {
					entry := &succPhiEntries[i]
					if stack := stacks[entry.symIdx]; len(stack) > 0 {
						entry.phi.Operands = append(entry.phi.Operands, PhiOperand{
							From:    p,
							Version: stack[len(stack)-1],
						})
					}
				}
			}
		}

		if b.Cfg.IsBranch(p) {
			for _, succ := range b.Cfg.SuccessorsReadOnly(p) {
				emitPhiOperands(succ)
			}
		} else if succ := b.Cfg.Successor(p); succ != p {
			emitPhiOperands(succ)
		}

		childState := renameState{
			parentVersions: currentVersions,
			parentVisPtr:   currentVisPtr,
		}
		if pIdx >= 0 && pIdx < len(domTree) {
			for _, child := range domTree[pIdx] {
				rename(child, childState)
			}
		}

		for i := range pushedLen {
			symIdx := pushedArr[i].symIdx
			count := pushedArr[i].count
			stacks[symIdx] = stacks[symIdx][:len(stacks[symIdx])-count]
		}
		for i := range pushedOverflow {
			symIdx := pushedOverflow[i].symIdx
			count := pushedOverflow[i].count
			stacks[symIdx] = stacks[symIdx][:len(stacks[symIdx])-count]
		}
	}

	rename(entry, renameState{})

	for symIdx, sym := range symByIndex {
		if touchedVersionID[symIdx] {
			b.NextVersionID[sym] = nextVersionID[symIdx]
		}
	}
	b.VisibleVersionByPoint = visibleVersionByPoint
	phiCap := 0
	for _, phiEntries := range phiByPoint {
		phiCap += len(phiEntries)
	}
	if phiCap > 0 {
		if cap(b.PhiNodes) < phiCap {
			b.PhiNodes = make([]PhiInfo, 0, phiCap)
		} else {
			b.PhiNodes = b.PhiNodes[:0]
		}
	} else {
		b.PhiNodes = b.PhiNodes[:0]
	}

	for _, p := range b.Cfg.RPOReadOnly() {
		if phiEntries := phiByPoint[p]; len(phiEntries) > 0 {
			for i := range phiEntries {
				entry := &phiEntries[i]
				if len(entry.phi.Operands) > 0 {
					b.PhiNodes = append(b.PhiNodes, entry.phi)
				}
			}
		}
	}
}

func assignmentTargetSymbol(target AssignTarget) (basecfg.SymbolID, string) {
	switch target.Kind {
	case TargetIdent:
		if target.Symbol != 0 {
			return target.Symbol, target.Name
		}
	case TargetField, TargetIndex:
		if target.BaseSymbol != 0 {
			return target.BaseSymbol, target.BaseName
		}
	}
	return 0, ""
}
