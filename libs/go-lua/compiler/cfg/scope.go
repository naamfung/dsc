package cfg

import (
	"sort"

	basecfg "github.com/wippyai/go-lua/types/cfg"
)

// SymbolID is an alias for basecfg.SymbolID.
type SymbolID = basecfg.SymbolID

// SymbolKind is an alias for basecfg.SymbolKind.
type SymbolKind = basecfg.SymbolKind

// Symbol kind constants.
const (
	SymbolUnknown = basecfg.SymbolUnknown
	SymbolLocal   = basecfg.SymbolLocal
	SymbolGlobal  = basecfg.SymbolGlobal
	SymbolParam   = basecfg.SymbolParam
	SymbolUpvalue = basecfg.SymbolUpvalue
)

// ScopeTracker tracks symbol visibility during CFG construction.
// It does NOT create symbols - it only registers symbols created by the binder.
// Uses copy-on-write: maps are shared until modification.
type ScopeTracker struct {
	stack       []map[string]basecfg.SymbolID
	shared      []bool                      // shared[i] is true if stack[i] is shared and needs copy before write
	globals     map[string]basecfg.SymbolID // lazy globals overlay, avoids copying all frames
	visibility  map[basecfg.Point]map[string]basecfg.SymbolID
	declPoints  map[basecfg.SymbolID]basecfg.Point
	symbolNames map[basecfg.SymbolID]string
	symbolKinds map[basecfg.SymbolID]basecfg.SymbolKind
}

// NewScopeTracker creates a new scope tracker.
func NewScopeTracker() *ScopeTracker {
	return NewScopeTrackerWithCapacity(0)
}

// NewScopeTrackerWithCapacity creates a new scope tracker with visibility capacity hint.
func NewScopeTrackerWithCapacity(pointCap int) *ScopeTracker {
	if pointCap < 0 {
		pointCap = 0
	}

	visCap := pointCap
	switch {
	case visCap > 128:
		visCap = 128
	case visCap > 64:
		visCap = 64
	case visCap > 32:
		visCap = 32
	}

	symbolCap := 0
	if visCap > 0 {
		symbolCap = visCap
		if symbolCap > 64 {
			symbolCap = 64
		}
	}

	return &ScopeTracker{
		stack:       []map[string]basecfg.SymbolID{make(map[string]basecfg.SymbolID)},
		shared:      []bool{false},
		globals:     make(map[string]basecfg.SymbolID),
		visibility:  make(map[basecfg.Point]map[string]basecfg.SymbolID, visCap),
		declPoints:  make(map[basecfg.SymbolID]basecfg.Point, symbolCap),
		symbolNames: make(map[basecfg.SymbolID]string, symbolCap),
		symbolKinds: make(map[basecfg.SymbolID]basecfg.SymbolKind, symbolCap),
	}
}

// EnterScope pushes a new scope frame onto the stack.
// Uses copy-on-write: shares parent's map until modification.
func (t *ScopeTracker) EnterScope() {
	cur := t.current()
	t.markShared(len(t.stack) - 1)
	t.stack = append(t.stack, cur)
	t.shared = append(t.shared, true)
}

// ExitScope pops the current scope frame from the stack.
func (t *ScopeTracker) ExitScope() {
	if len(t.stack) > 1 {
		t.stack = t.stack[:len(t.stack)-1]
		t.shared = t.shared[:len(t.shared)-1]
	}
}

// markShared marks a stack level as shared.
func (t *ScopeTracker) markShared(level int) {
	if level >= 0 && level < len(t.shared) {
		t.shared[level] = true
	}
}

// ensureWritable ensures the current map can be modified.
// If the map is shared, it creates a copy.
func (t *ScopeTracker) ensureWritable() {
	if len(t.stack) == 0 {
		return
	}

	idx := len(t.stack) - 1

	if t.shared[idx] {
		t.stack[idx] = copyMap(t.stack[idx])
		t.shared[idx] = false
	}
}

// RegisterSymbol registers an externally-created symbol in the current scope.
func (t *ScopeTracker) RegisterSymbol(
	sym basecfg.SymbolID, name string, kind basecfg.SymbolKind, declPoint basecfg.Point,
) {
	if sym == 0 || name == "" || len(t.stack) == 0 {
		return
	}

	t.ensureWritable()
	t.stack[len(t.stack)-1][name] = sym
	t.declPoints[sym] = declPoint
	t.symbolNames[sym] = name
	t.symbolKinds[sym] = kind
}

// RegisterGlobal registers a symbol as a global.
// Uses lazy overlay - globals are stored separately and merged on lookup.
func (t *ScopeTracker) RegisterGlobal(sym basecfg.SymbolID, name string, declPoint basecfg.Point) {
	if sym == 0 || name == "" {
		return
	}

	t.globals[name] = sym
	t.declPoints[sym] = declPoint
	t.symbolNames[sym] = name
	t.symbolKinds[sym] = basecfg.SymbolGlobal
}

// SnapshotVisibility records the current visibility state at a CFG point.
// Uses copy-on-write: the next modification will copy the map.
func (t *ScopeTracker) SnapshotVisibility(point basecfg.Point) {
	cur := t.current()
	t.visibility[point] = cur
	t.markShared(len(t.stack) - 1)
}

// VisibleAt returns the visibility snapshot at a CFG point.
func (t *ScopeTracker) VisibleAt(point basecfg.Point) *SymbolMap {
	if m := t.visibility[point]; m != nil {
		return &SymbolMap{m: m, globals: t.globals}
	}

	return nil
}

// SymbolAt returns the symbol for a name at a specific CFG point.
func (t *ScopeTracker) SymbolAt(point basecfg.Point, name string) (basecfg.SymbolID, bool) {
	if vis := t.visibility[point]; vis != nil {
		if sym, ok := vis[name]; ok {
			return sym, true
		}
	}

	if sym, ok := t.globals[name]; ok {
		return sym, true
	}

	return 0, false
}

// DeclarationPoint returns the CFG point where a symbol was declared.
func (t *ScopeTracker) DeclarationPoint(sym basecfg.SymbolID) (basecfg.Point, bool) {
	p, ok := t.declPoints[sym]

	return p, ok
}

// CurrentDepth returns the current scope nesting depth.
func (t *ScopeTracker) CurrentDepth() int {
	return len(t.stack) - 1
}

// SymbolKind returns the kind of a symbol.
func (t *ScopeTracker) SymbolKind(sym basecfg.SymbolID) (basecfg.SymbolKind, bool) {
	kind, ok := t.symbolKinds[sym]

	return kind, ok
}

// Lookup returns the symbol for a name in the current scope.
func (t *ScopeTracker) Lookup(name string) (basecfg.SymbolID, bool) {
	cur := t.current()

	if sym, ok := cur[name]; ok {
		return sym, true
	}

	if sym, ok := t.globals[name]; ok {
		return sym, true
	}

	return 0, false
}

func (t *ScopeTracker) current() map[string]basecfg.SymbolID {
	if len(t.stack) == 0 {
		return make(map[string]basecfg.SymbolID)
	}

	return t.stack[len(t.stack)-1]
}

func copyMap(m map[string]basecfg.SymbolID) map[string]basecfg.SymbolID {
	if m == nil {
		return make(map[string]basecfg.SymbolID)
	}

	cp := make(map[string]basecfg.SymbolID, len(m))

	for k, v := range m {
		cp[k] = v
	}

	return cp
}

// SymbolMap wraps a map for compatibility with existing code.
type SymbolMap struct {
	m       map[string]basecfg.SymbolID
	globals map[string]basecfg.SymbolID // lazy globals overlay
}

// Get returns the symbol for a name.
func (s *SymbolMap) Get(name string) (basecfg.SymbolID, bool) {
	if sym, ok := s.m[name]; ok {
		return sym, true
	}

	if s.globals != nil {
		if sym, ok := s.globals[name]; ok {
			return sym, true
		}
	}

	return 0, false
}

// Range iterates over all symbols.
func (s *SymbolMap) Range(fn func(name string, sym basecfg.SymbolID) bool) {
	localNames := make([]string, 0, len(s.m))
	for name := range s.m {
		localNames = append(localNames, name)
	}
	sort.Strings(localNames)
	for _, name := range localNames {
		sym := s.m[name]
		if !fn(name, sym) {
			return
		}
	}

	if s.globals != nil {
		globalNames := make([]string, 0, len(s.globals))
		for name := range s.globals {
			if _, inLocal := s.m[name]; inLocal {
				continue // local shadows global
			}
			globalNames = append(globalNames, name)
		}
		sort.Strings(globalNames)
		for _, name := range globalNames {
			sym := s.globals[name]
			if _, inLocal := s.m[name]; inLocal {
				continue // local shadows global
			}

			if !fn(name, sym) {
				return
			}
		}
	}
}

// ToMap returns the underlying map (without globals for pointer identity).
func (s *SymbolMap) ToMap() map[string]basecfg.SymbolID {
	return s.m
}

// Size returns the number of symbols in the map.
func (s *SymbolMap) Size() int {
	if s.globals == nil {
		return len(s.m)
	}
	// Count unique names
	count := len(s.m)

	for name := range s.globals {
		if _, inLocal := s.m[name]; !inLocal {
			count++
		}
	}

	return count
}
