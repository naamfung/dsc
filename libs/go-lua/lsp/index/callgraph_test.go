package index

import (
	"testing"

	"github.com/wippyai/go-lua/types/diag"
)

func TestCallGraph_AddCall(t *testing.T) {
	cg := NewCallGraph()

	callerSpan := diag.Span{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 10}
	calleeSpan := diag.Span{StartLine: 5, StartCol: 1, EndLine: 5, EndCol: 10}
	callSpan := diag.Span{StartLine: 2, StartCol: 5, EndLine: 2, EndCol: 15}

	cg.AddCall("main.lua", "caller", callerSpan, "main.lua", "callee", calleeSpan, callSpan)

	// Check callers of "callee"
	callers := cg.CallersOf("main.lua", "callee")
	if len(callers) != 1 {
		t.Fatalf("Expected 1 caller, got %d", len(callers))
	}
	if callers[0].CallerName != "caller" {
		t.Errorf("Expected caller name 'caller', got %q", callers[0].CallerName)
	}

	// Check callees from "caller"
	callees := cg.CalleesOf("main.lua", "caller")
	if len(callees) != 1 {
		t.Fatalf("Expected 1 callee, got %d", len(callees))
	}
	if callees[0].CalleeName != "callee" {
		t.Errorf("Expected callee name 'callee', got %q", callees[0].CalleeName)
	}
}

func TestCallGraph_MultipleCallers(t *testing.T) {
	cg := NewCallGraph()

	// "target" is called by "func1", "func2", and "func3"
	cg.AddCall("a.lua", "func1", diag.Span{StartLine: 1}, "target.lua", "target", diag.Span{StartLine: 10}, diag.Span{StartLine: 2})
	cg.AddCall("b.lua", "func2", diag.Span{StartLine: 1}, "target.lua", "target", diag.Span{StartLine: 10}, diag.Span{StartLine: 3})
	cg.AddCall("c.lua", "func3", diag.Span{StartLine: 1}, "target.lua", "target", diag.Span{StartLine: 10}, diag.Span{StartLine: 4})

	callers := cg.CallersOf("target.lua", "target")
	if len(callers) != 3 {
		t.Errorf("Expected 3 callers, got %d", len(callers))
	}
}

func TestCallGraph_MultipleCallees(t *testing.T) {
	cg := NewCallGraph()

	// "caller" calls "target1", "target2", "target3"
	cg.AddCall("main.lua", "caller", diag.Span{StartLine: 1}, "main.lua", "target1", diag.Span{StartLine: 10}, diag.Span{StartLine: 2})
	cg.AddCall("main.lua", "caller", diag.Span{StartLine: 1}, "main.lua", "target2", diag.Span{StartLine: 20}, diag.Span{StartLine: 3})
	cg.AddCall("main.lua", "caller", diag.Span{StartLine: 1}, "main.lua", "target3", diag.Span{StartLine: 30}, diag.Span{StartLine: 4})

	callees := cg.CalleesOf("main.lua", "caller")
	if len(callees) != 3 {
		t.Errorf("Expected 3 callees, got %d", len(callees))
	}
}

func TestCallGraph_CrossFileCall(t *testing.T) {
	cg := NewCallGraph()

	// "main.lua:main" calls "utils.lua:helper"
	cg.AddCall("main.lua", "main", diag.Span{StartLine: 1}, "utils.lua", "helper", diag.Span{StartLine: 5}, diag.Span{StartLine: 3})

	// CallersOf filters by callee file
	callers := cg.CallersOf("utils.lua", "helper")
	if len(callers) != 1 {
		t.Errorf("Expected 1 caller for utils.lua:helper, got %d", len(callers))
	}
}

func TestCallGraph_InvalidateFile(t *testing.T) {
	cg := NewCallGraph()

	cg.AddCall("main.lua", "caller", diag.Span{StartLine: 1}, "target.lua", "callee", diag.Span{StartLine: 5}, diag.Span{StartLine: 2})
	cg.AddCall("other.lua", "other", diag.Span{StartLine: 1}, "target.lua", "callee", diag.Span{StartLine: 5}, diag.Span{StartLine: 2})

	// Before invalidation
	callers := cg.CallersOf("target.lua", "callee")
	if len(callers) != 2 {
		t.Errorf("Expected 2 callers before invalidation, got %d", len(callers))
	}

	// Invalidate main.lua
	cg.InvalidateFile("main.lua")

	// After invalidation - only other.lua remains
	callers = cg.CallersOf("target.lua", "callee")
	if len(callers) != 1 {
		t.Errorf("Expected 1 caller after invalidation, got %d", len(callers))
	}
}

func TestCallGraph_RecursiveCall(t *testing.T) {
	cg := NewCallGraph()

	// Recursive function
	cg.AddCall("main.lua", "factorial", diag.Span{StartLine: 1}, "main.lua", "factorial", diag.Span{StartLine: 1}, diag.Span{StartLine: 3})

	callers := cg.CallersOf("main.lua", "factorial")
	if len(callers) != 1 {
		t.Errorf("Expected 1 caller (self), got %d", len(callers))
	}
	if callers[0].CallerName != "factorial" {
		t.Errorf("Expected self-call, got caller %q", callers[0].CallerName)
	}
}

func TestCallGraph_CallAtPosition(t *testing.T) {
	cg := NewCallGraph()

	callSpan := diag.Span{StartLine: 5, StartCol: 10, EndLine: 5, EndCol: 20}
	cg.AddCall("main.lua", "caller", diag.Span{StartLine: 1}, "main.lua", "callee", diag.Span{StartLine: 10}, callSpan)

	// Find call at position
	call := cg.CallAt("main.lua", 5, 15)
	if call == nil {
		t.Fatal("Expected to find call at position")
	}
	if call.CalleeName != "callee" {
		t.Errorf("Expected callee 'callee', got %q", call.CalleeName)
	}

	// No call at different position
	call = cg.CallAt("main.lua", 1, 1)
	if call != nil {
		t.Error("Expected no call at position (1,1)")
	}
}

func TestCallGraph_Clear(t *testing.T) {
	cg := NewCallGraph()

	cg.AddCall("main.lua", "caller", diag.Span{StartLine: 1}, "main.lua", "callee", diag.Span{StartLine: 5}, diag.Span{StartLine: 2})

	cg.Clear()

	callers := cg.CallersOf("main.lua", "callee")
	if len(callers) != 0 {
		t.Errorf("Expected 0 callers after clear, got %d", len(callers))
	}
}

func TestCallGraph_AllEdges(t *testing.T) {
	cg := NewCallGraph()

	cg.AddCall("a.lua", "func1", diag.Span{StartLine: 1}, "target.lua", "target", diag.Span{StartLine: 10}, diag.Span{StartLine: 2})
	cg.AddCall("b.lua", "func2", diag.Span{StartLine: 1}, "target.lua", "target", diag.Span{StartLine: 10}, diag.Span{StartLine: 3})
	cg.AddCall("a.lua", "func1", diag.Span{StartLine: 1}, "other.lua", "other", diag.Span{StartLine: 20}, diag.Span{StartLine: 4})

	edges := cg.AllEdges()
	if len(edges) != 3 {
		t.Errorf("Expected 3 edges, got %d", len(edges))
	}
}

func TestCallGraph_AllEdges_Empty(t *testing.T) {
	cg := NewCallGraph()
	edges := cg.AllEdges()
	if len(edges) != 0 {
		t.Errorf("Expected 0 edges from empty graph, got %d", len(edges))
	}
}

func TestCallGraph_CallCount(t *testing.T) {
	cg := NewCallGraph()

	cg.AddCall("main.lua", "caller", diag.Span{StartLine: 1}, "main.lua", "callee1", diag.Span{StartLine: 10}, diag.Span{StartLine: 2})
	cg.AddCall("main.lua", "caller", diag.Span{StartLine: 1}, "main.lua", "callee2", diag.Span{StartLine: 20}, diag.Span{StartLine: 3})
	cg.AddCall("main.lua", "caller", diag.Span{StartLine: 1}, "main.lua", "callee3", diag.Span{StartLine: 30}, diag.Span{StartLine: 4})
	cg.AddCall("main.lua", "other", diag.Span{StartLine: 5}, "main.lua", "callee1", diag.Span{StartLine: 10}, diag.Span{StartLine: 6})

	count := cg.CallCount("main.lua", "caller")
	if count != 3 {
		t.Errorf("Expected call count 3, got %d", count)
	}

	count = cg.CallCount("main.lua", "other")
	if count != 1 {
		t.Errorf("Expected call count 1, got %d", count)
	}

	count = cg.CallCount("main.lua", "nonexistent")
	if count != 0 {
		t.Errorf("Expected call count 0, got %d", count)
	}
}

func TestCallGraph_CallerCount(t *testing.T) {
	cg := NewCallGraph()

	cg.AddCall("a.lua", "func1", diag.Span{StartLine: 1}, "target.lua", "target", diag.Span{StartLine: 10}, diag.Span{StartLine: 2})
	cg.AddCall("b.lua", "func2", diag.Span{StartLine: 1}, "target.lua", "target", diag.Span{StartLine: 10}, diag.Span{StartLine: 3})
	cg.AddCall("c.lua", "func3", diag.Span{StartLine: 1}, "target.lua", "target", diag.Span{StartLine: 10}, diag.Span{StartLine: 4})
	cg.AddCall("d.lua", "func4", diag.Span{StartLine: 1}, "other.lua", "other", diag.Span{StartLine: 20}, diag.Span{StartLine: 5})

	count := cg.CallerCount("target")
	if count != 3 {
		t.Errorf("Expected caller count 3, got %d", count)
	}

	count = cg.CallerCount("other")
	if count != 1 {
		t.Errorf("Expected caller count 1, got %d", count)
	}

	count = cg.CallerCount("nonexistent")
	if count != 0 {
		t.Errorf("Expected caller count 0, got %d", count)
	}
}

func TestCallGraph_InvalidateFile_NoEdges(t *testing.T) {
	cg := NewCallGraph()

	// Invalidate non-existent file should not panic
	cg.InvalidateFile("nonexistent.lua")

	// Add edge then invalidate different file
	cg.AddCall("a.lua", "func", diag.Span{StartLine: 1}, "a.lua", "target", diag.Span{StartLine: 10}, diag.Span{StartLine: 2})
	cg.InvalidateFile("b.lua")

	edges := cg.AllEdges()
	if len(edges) != 1 {
		t.Errorf("Expected 1 edge after invalidating different file, got %d", len(edges))
	}
}
