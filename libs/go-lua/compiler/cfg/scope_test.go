package cfg

import (
	"testing"

	basecfg "github.com/wippyai/go-lua/types/cfg"
)

func TestScopeTracker_BasicRegistration(t *testing.T) {
	tracker := NewScopeTracker()

	// Create and register x at point 1
	sym := basecfg.NextSymbolID()
	tracker.RegisterSymbol(sym, "x", basecfg.SymbolLocal, basecfg.Point(1))

	// Lookup should find it
	found, ok := tracker.Lookup("x")
	if !ok {
		t.Error("Lookup should find registered variable")
	}
	if found != sym {
		t.Errorf("Lookup returned wrong symbol: got %d, want %d", found, sym)
	}

	// Unknown variable should not be found
	_, ok = tracker.Lookup("y")
	if ok {
		t.Error("Lookup should not find unregistered variable")
	}
}

func TestScopeTracker_NestedScopes(t *testing.T) {
	tracker := NewScopeTracker()

	// Outer scope: register x
	outerX := basecfg.NextSymbolID()
	tracker.RegisterSymbol(outerX, "x", basecfg.SymbolLocal, basecfg.Point(1))

	// Enter inner scope
	tracker.EnterScope()

	// Lookup x should find outer x
	found, ok := tracker.Lookup("x")
	if !ok || found != outerX {
		t.Error("Inner scope should see outer x")
	}

	// Register y in inner scope
	innerY := basecfg.NextSymbolID()
	tracker.RegisterSymbol(innerY, "y", basecfg.SymbolLocal, basecfg.Point(2))

	// y should be visible
	found, ok = tracker.Lookup("y")
	if !ok || found != innerY {
		t.Error("Inner y should be visible")
	}

	// Exit inner scope
	tracker.ExitScope()

	// y should no longer be visible
	_, ok = tracker.Lookup("y")
	if ok {
		t.Error("y should not be visible after exiting inner scope")
	}

	// x should still be visible
	found, ok = tracker.Lookup("x")
	if !ok || found != outerX {
		t.Error("Outer x should still be visible")
	}
}

func TestScopeTracker_Shadowing(t *testing.T) {
	tracker := NewScopeTracker()

	// Outer scope: register x
	outerX := basecfg.NextSymbolID()
	tracker.RegisterSymbol(outerX, "x", basecfg.SymbolLocal, basecfg.Point(1))
	tracker.SnapshotVisibility(basecfg.Point(1))

	// Enter inner scope
	tracker.EnterScope()

	// Register x again (shadowing)
	innerX := basecfg.NextSymbolID()
	tracker.RegisterSymbol(innerX, "x", basecfg.SymbolLocal, basecfg.Point(2))
	tracker.SnapshotVisibility(basecfg.Point(2))

	// The two x's should have different SymbolIDs
	if outerX == innerX {
		t.Error("Shadowed x should have different basecfg.SymbolID")
	}

	// Lookup should find inner x
	found, ok := tracker.Lookup("x")
	if !ok || found != innerX {
		t.Errorf("Lookup should find inner x, got %d want %d", found, innerX)
	}

	// Exit inner scope
	tracker.ExitScope()
	tracker.SnapshotVisibility(basecfg.Point(3))

	// Lookup should find outer x again
	found, ok = tracker.Lookup("x")
	if !ok || found != outerX {
		t.Errorf("After exit, lookup should find outer x, got %d want %d", found, outerX)
	}
}

func TestScopeTracker_VisibilitySnapshots(t *testing.T) {
	tracker := NewScopeTracker()

	// Point 1: register x
	xSym := basecfg.NextSymbolID()
	tracker.RegisterSymbol(xSym, "x", basecfg.SymbolLocal, basecfg.Point(1))
	tracker.SnapshotVisibility(basecfg.Point(1))

	// Point 2: enter scope, register y
	tracker.EnterScope()
	ySym := basecfg.NextSymbolID()
	tracker.RegisterSymbol(ySym, "y", basecfg.SymbolLocal, basecfg.Point(2))
	tracker.SnapshotVisibility(basecfg.Point(2))

	// Point 3: exit scope
	tracker.ExitScope()
	tracker.SnapshotVisibility(basecfg.Point(3))

	// Check visibility at point 1
	vis1 := tracker.VisibleAt(basecfg.Point(1))
	if vis1 == nil {
		t.Error("Point 1: visibility should not be nil")
	} else {
		if sym, ok := vis1.Get("x"); !ok || sym != xSym {
			t.Error("Point 1: x should be visible")
		}
		if _, ok := vis1.Get("y"); ok {
			t.Error("Point 1: y should not be visible yet")
		}
	}

	// Check visibility at point 2
	vis2 := tracker.VisibleAt(basecfg.Point(2))
	if vis2 == nil {
		t.Error("Point 2: visibility should not be nil")
	} else {
		if sym, ok := vis2.Get("x"); !ok || sym != xSym {
			t.Error("Point 2: x should be visible")
		}
		if sym, ok := vis2.Get("y"); !ok || sym != ySym {
			t.Error("Point 2: y should be visible")
		}
	}

	// Check visibility at point 3
	vis3 := tracker.VisibleAt(basecfg.Point(3))
	if vis3 == nil {
		t.Error("Point 3: visibility should not be nil")
	} else {
		if sym, ok := vis3.Get("x"); !ok || sym != xSym {
			t.Error("Point 3: x should be visible")
		}
		if _, ok := vis3.Get("y"); ok {
			t.Error("Point 3: y should not be visible after scope exit")
		}
	}
}

func TestScopeTracker_SymbolAt(t *testing.T) {
	tracker := NewScopeTracker()

	xSym := basecfg.NextSymbolID()
	tracker.RegisterSymbol(xSym, "x", basecfg.SymbolLocal, basecfg.Point(1))
	tracker.SnapshotVisibility(basecfg.Point(1))

	tracker.EnterScope()
	ySym := basecfg.NextSymbolID()
	tracker.RegisterSymbol(ySym, "y", basecfg.SymbolLocal, basecfg.Point(2))
	tracker.SnapshotVisibility(basecfg.Point(2))

	tracker.ExitScope()
	tracker.SnapshotVisibility(basecfg.Point(3))

	// Test SymbolAt
	sym, ok := tracker.SymbolAt(basecfg.Point(1), "x")
	if !ok || sym != xSym {
		t.Error("SymbolAt(1, x) should return xSym")
	}

	sym, ok = tracker.SymbolAt(basecfg.Point(2), "y")
	if !ok || sym != ySym {
		t.Error("SymbolAt(2, y) should return ySym")
	}

	_, ok = tracker.SymbolAt(basecfg.Point(3), "y")
	if ok {
		t.Error("SymbolAt(3, y) should return false")
	}
}

func TestScopeTracker_DeclarationPoint(t *testing.T) {
	tracker := NewScopeTracker()

	xSym := basecfg.NextSymbolID()
	tracker.RegisterSymbol(xSym, "x", basecfg.SymbolLocal, basecfg.Point(5))
	ySym := basecfg.NextSymbolID()
	tracker.RegisterSymbol(ySym, "y", basecfg.SymbolLocal, basecfg.Point(10))

	point, ok := tracker.DeclarationPoint(xSym)
	if !ok || point != basecfg.Point(5) {
		t.Errorf("DeclarationPoint(xSym) should return 5, got %d", point)
	}

	point, ok = tracker.DeclarationPoint(ySym)
	if !ok || point != basecfg.Point(10) {
		t.Errorf("DeclarationPoint(ySym) should return 10, got %d", point)
	}

	_, ok = tracker.DeclarationPoint(basecfg.SymbolID(9999))
	if ok {
		t.Error("DeclarationPoint for unknown symbol should return false")
	}
}

func TestScopeTracker_MultipleScopes(t *testing.T) {
	tracker := NewScopeTracker()

	// Simulate: local x = 1; do local y = 2 end; do local z = 3 end
	xSym := basecfg.NextSymbolID()
	tracker.RegisterSymbol(xSym, "x", basecfg.SymbolLocal, basecfg.Point(1))
	tracker.SnapshotVisibility(basecfg.Point(1))

	// First do block
	tracker.EnterScope()
	ySym := basecfg.NextSymbolID()
	tracker.RegisterSymbol(ySym, "y", basecfg.SymbolLocal, basecfg.Point(2))
	tracker.SnapshotVisibility(basecfg.Point(2))
	tracker.ExitScope()
	tracker.SnapshotVisibility(basecfg.Point(3))

	// Second do block
	tracker.EnterScope()
	zSym := basecfg.NextSymbolID()
	tracker.RegisterSymbol(zSym, "z", basecfg.SymbolLocal, basecfg.Point(4))
	tracker.SnapshotVisibility(basecfg.Point(4))
	tracker.ExitScope()
	tracker.SnapshotVisibility(basecfg.Point(5))

	// Verify all symbols are unique
	if xSym == ySym || xSym == zSym || ySym == zSym {
		t.Error("All symbols should be unique")
	}

	// At point 3: only x visible
	vis3 := tracker.VisibleAt(basecfg.Point(3))
	if vis3 == nil {
		t.Error("Point 3: visibility should not be nil")
	} else {
		if vis3.Size() != 1 {
			t.Error("Point 3: only x should be visible")
		}
		if sym, ok := vis3.Get("x"); !ok || sym != xSym {
			t.Error("Point 3: only x should be visible")
		}
	}

	// At point 4: x and z visible (not y)
	vis4 := tracker.VisibleAt(basecfg.Point(4))
	if vis4 == nil {
		t.Error("Point 4: visibility should not be nil")
	} else {
		if sym, ok := vis4.Get("x"); !ok || sym != xSym {
			t.Error("Point 4: x should be visible")
		}
		if sym, ok := vis4.Get("z"); !ok || sym != zSym {
			t.Error("Point 4: z should be visible")
		}
		if _, ok := vis4.Get("y"); ok {
			t.Error("Point 4: y should not be visible")
		}
	}
}

func TestScopeTracker_DeeplyNested(t *testing.T) {
	tracker := NewScopeTracker()

	// local a; do local b; do local c end end
	aSym := basecfg.NextSymbolID()
	tracker.RegisterSymbol(aSym, "a", basecfg.SymbolLocal, basecfg.Point(1))
	if tracker.CurrentDepth() != 0 {
		t.Error("Initial depth should be 0")
	}

	tracker.EnterScope()
	bSym := basecfg.NextSymbolID()
	tracker.RegisterSymbol(bSym, "b", basecfg.SymbolLocal, basecfg.Point(2))
	if tracker.CurrentDepth() != 1 {
		t.Error("After first EnterScope, depth should be 1")
	}

	tracker.EnterScope()
	cSym := basecfg.NextSymbolID()
	tracker.RegisterSymbol(cSym, "c", basecfg.SymbolLocal, basecfg.Point(3))
	tracker.SnapshotVisibility(basecfg.Point(3))
	if tracker.CurrentDepth() != 2 {
		t.Error("After second EnterScope, depth should be 2")
	}

	// At innermost point, all should be visible
	vis := tracker.VisibleAt(basecfg.Point(3))
	if vis == nil {
		t.Error("visibility should not be nil")
	} else {
		symA, okA := vis.Get("a")
		symB, okB := vis.Get("b")
		symC, okC := vis.Get("c")
		if !okA || !okB || !okC || symA != aSym || symB != bSym || symC != cSym {
			t.Error("All variables should be visible at innermost point")
		}
	}

	tracker.ExitScope()
	tracker.ExitScope()
	if tracker.CurrentDepth() != 0 {
		t.Error("After two ExitScope, depth should be 0")
	}
}

func TestScopeTracker_RegisterGlobal(t *testing.T) {
	tracker := NewScopeTracker()

	// Register globals
	regSym := basecfg.NextSymbolID()
	tracker.RegisterGlobal(regSym, "registry", 0)
	procSym := basecfg.NextSymbolID()
	tracker.RegisterGlobal(procSym, "process", 0)
	printSym := basecfg.NextSymbolID()
	tracker.RegisterGlobal(printSym, "print", 0)

	// All globals should be found via Lookup
	for name, expectedSym := range map[string]basecfg.SymbolID{"registry": regSym, "process": procSym, "print": printSym} {
		sym, ok := tracker.Lookup(name)
		if !ok {
			t.Errorf("Global %q should be found", name)
		}
		if sym != expectedSym {
			t.Errorf("Global %q has wrong symbol: got %d, want %d", name, sym, expectedSym)
		}
	}

	// Declaration point for globals should be 0 (pre-declared)
	for _, sym := range []basecfg.SymbolID{regSym, procSym, printSym} {
		declPoint, ok := tracker.DeclarationPoint(sym)
		if !ok {
			t.Error("DeclarationPoint should find global symbol")
		}
		if declPoint != 0 {
			t.Errorf("Global declaration point should be 0, got %d", declPoint)
		}
	}

	// Kind should be basecfg.SymbolGlobal
	for _, sym := range []basecfg.SymbolID{regSym, procSym, printSym} {
		kind, ok := tracker.SymbolKind(sym)
		if !ok {
			t.Error("basecfg.SymbolKind should find global symbol")
		}
		if kind != basecfg.SymbolGlobal {
			t.Errorf("Global kind should be basecfg.SymbolGlobal, got %v", kind)
		}
	}
}

func TestScopeTracker_GlobalShadowing(t *testing.T) {
	tracker := NewScopeTracker()

	// Register global x
	globalX := basecfg.NextSymbolID()
	tracker.RegisterGlobal(globalX, "x", 0)
	tracker.SnapshotVisibility(basecfg.Point(0))

	// Enter inner scope and register local x
	tracker.EnterScope()
	localX := basecfg.NextSymbolID()
	tracker.RegisterSymbol(localX, "x", basecfg.SymbolLocal, basecfg.Point(1))
	tracker.SnapshotVisibility(basecfg.Point(1))

	// Local x should shadow global x
	if localX == globalX {
		t.Error("Local x should have different basecfg.SymbolID than global x")
	}

	foundX, ok := tracker.Lookup("x")
	if !ok {
		t.Error("x should be found")
	}
	if foundX != localX {
		t.Error("Lookup should find local x, not global x")
	}

	// Exit inner scope
	tracker.ExitScope()
	tracker.SnapshotVisibility(basecfg.Point(2))

	// Now global x should be visible again
	foundX, ok = tracker.Lookup("x")
	if !ok {
		t.Error("x should be found after scope exit")
	}
	if foundX != globalX {
		t.Error("After scope exit, global x should be visible")
	}

	// Verify snapshots
	sym0, ok := tracker.SymbolAt(basecfg.Point(0), "x")
	if !ok || sym0 != globalX {
		t.Error("At point 0, global x should be visible")
	}

	sym1, ok := tracker.SymbolAt(basecfg.Point(1), "x")
	if !ok || sym1 != localX {
		t.Error("At point 1, local x should be visible")
	}

	sym2, ok := tracker.SymbolAt(basecfg.Point(2), "x")
	if !ok || sym2 != globalX {
		t.Error("At point 2, global x should be visible again")
	}
}

func TestScopeTracker_SymbolKind(t *testing.T) {
	tracker := NewScopeTracker()

	// Register symbols with different kinds
	paramSym := basecfg.NextSymbolID()
	tracker.RegisterSymbol(paramSym, "param", basecfg.SymbolParam, basecfg.Point(1))

	localSym := basecfg.NextSymbolID()
	tracker.RegisterSymbol(localSym, "local", basecfg.SymbolLocal, basecfg.Point(2))

	globalSym := basecfg.NextSymbolID()
	tracker.RegisterGlobal(globalSym, "global", 0)

	// Verify kinds
	kind, ok := tracker.SymbolKind(paramSym)
	if !ok || kind != basecfg.SymbolParam {
		t.Error("param should have basecfg.SymbolParam kind")
	}

	kind, ok = tracker.SymbolKind(localSym)
	if !ok || kind != basecfg.SymbolLocal {
		t.Error("local should have basecfg.SymbolLocal kind")
	}

	kind, ok = tracker.SymbolKind(globalSym)
	if !ok || kind != basecfg.SymbolGlobal {
		t.Error("global should have basecfg.SymbolGlobal kind")
	}
}
