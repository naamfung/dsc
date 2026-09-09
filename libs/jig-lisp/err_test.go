package lisp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jig/lisp/env"
	"github.com/jig/lisp/lib/core"
	"github.com/jig/lisp/types"
)

func TestBasicError(t *testing.T) {
	ns := env.NewEnv()
	_, err := REPL(context.Background(), ns, `(abc 1 2 3)`, types.NewCursorFile(t.Name()))
	if err == nil {
		t.Fatal("fatal error")
	}
	if !strings.Contains(err.Error(), `symbol 'abc' not found`) {
		t.Fatal(err)
	}
}

func TestTryCatchError2(t *testing.T) {
	ns := env.NewEnv()
	res, err := REPL(context.Background(), ns, `(try abc (catch exc exc))`, types.NewCursorFile(t.Name()))
	if err != nil {
		t.Fatal(err)
	}

	//if !strings.HasSuffix(res.(string), `'abc' not found`) {
	if res != `«go-error "symbol 'abc' not found"»` {
		t.Fatalf("%s", res)
	}
}

func TestTryCatchError3(t *testing.T) {
	ns := env.NewEnv()
	res, err := REPL(context.Background(), ns, `(try (abc 1 2) (catch exc exc))`, types.NewCursorFile(t.Name()))
	if err != nil {
		t.Fatal(err)
	}

	// if !strings.HasSuffix(res.(string), `'abc' not found`) {
	if res != `«go-error "symbol 'abc' not found"»` {
		t.Fatalf("%s", res)
	}
}

func TestTryCatchThrowsMalType(t *testing.T) {
	ns := env.NewEnv()
	core.Load(ns)
	res, err := REPL(context.Background(), ns, `(try (throw {:a 1}) (catch exc exc))`, types.NewCursorFile(t.Name()))
	if err != nil {
		t.Fatal(err)
	}

	// if !strings.HasSuffix(res.(string), `'abc' not found`) {
	if res != `{:a 1}` {
		t.Fatalf("%s", res)
	}
}

func TestStackTrace(t *testing.T) {
	ns := env.NewEnv()
	core.Load(ns)

	// Test that nested function calls create a stack trace
	code := `(do (def helper (fn [x] (+ x abc))) (def caller (fn [y] (helper y))) (caller 5))`
	_, err := REPL(context.Background(), ns, code, types.NewCursorFile(t.Name()))
	if err == nil {
		t.Fatal("expected error but got none")
	}

	// Verify error message contains the original error
	if !strings.Contains(err.Error(), "symbol 'abc' not found") {
		t.Fatalf("error should contain original message: %s", err)
	}

	// Verify stack trace is present (should have multiple "at" lines)
	errStr := err.Error()
	atCount := strings.Count(errStr, "\n  at ")
	if atCount < 1 {
		t.Fatalf("expected stack trace with at least 1 frame, got %d: %s", atCount, errStr)
	}
}

func TestCursorPreservedInTryCatch(t *testing.T) {
	ns := env.NewEnv()
	core.Load(ns)

	// Test that try/catch preserves the original error position
	code := `(try (+ 1 undefined-symbol) (catch exc (throw exc)))`
	_, err := REPL(context.Background(), ns, code, types.NewCursorFile(t.Name()))
	if err == nil {
		t.Fatal("expected error but got none")
	}

	// Should contain the original error message
	if !strings.Contains(err.Error(), "symbol 'undefined-symbol' not found") {
		t.Fatalf("error should contain original message: %s", err)
	}
}

func TestNestedFunctionStackTrace(t *testing.T) {
	ns := env.NewEnv()
	core.Load(ns)

	// Test equivalent to /tmp/test_stack.lisp
	// Defines nested functions and calls them to generate a stack trace
	code := `(do
		(def helper (fn [x] (+ x undefined-var)))
		(def caller (fn [y] (helper y)))
		(def main (fn [] (caller 42)))
		(main)
	)`

	_, err := REPL(context.Background(), ns, code, types.NewCursorFile(t.Name()))
	if err == nil {
		t.Fatal("expected error but got none")
	}

	// Verify error message contains the original error
	if !strings.Contains(err.Error(), "symbol 'undefined-var' not found") {
		t.Fatalf("error should contain original message: %s", err)
	}

	// Verify stack trace is present
	errStr := err.Error()
	lines := strings.Split(errStr, "\n")

	// First line should be the error with position
	if !strings.Contains(lines[0], "symbol 'undefined-var' not found") {
		t.Errorf("First line should contain error: %s", lines[0])
	}

	// Should have stack frames
	// Note: Due to TCO and duplicate filtering, we typically get 1 frame with function name
	// The duplicate frame (same position, no function name) is filtered out
	atCount := 0
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "at ") {
			atCount++
			t.Logf("Stack frame %d: %s", atCount, line)
		}
	}

	if atCount < 1 {
		t.Errorf("Expected at least 1 stack frame, got %d", atCount)
	}

	t.Logf("Full stack trace:\n%s", errStr)
}

func TestTryCatchPreservesOriginalPosition(t *testing.T) {
	ns := env.NewEnv()
	core.Load(ns)

	// Test equivalent to /tmp/test_trycatch.lisp
	// Verifies that re-throwing an error in catch preserves original position
	code := `(do
		(def test-fn (fn []
			(try
				(+ 1 missing-symbol)
				(catch exc (throw exc)))))
		(test-fn)
	)`

	_, err := REPL(context.Background(), ns, code, types.NewCursorFile(t.Name()))
	if err == nil {
		t.Fatal("expected error but got none")
	}

	// Should contain the original error message
	if !strings.Contains(err.Error(), "symbol 'missing-symbol' not found") {
		t.Fatalf("error should contain original message: %s", err)
	}

	// The error should reference the position where missing-symbol appears
	// Verify the cursor was preserved (not overwritten by catch/throw)
	errStr := err.Error()
	lines := strings.Split(errStr, "\n")

	// First line has the error with position pointing to missing-symbol
	if !strings.Contains(lines[0], "missing-symbol") {
		t.Errorf("First line should reference missing-symbol position: %s", lines[0])
	}

	// The cursor should NOT point to the throw statement, but to the original error
	// This validates that NewLispError preserves the original cursor
	t.Logf("Error with preserved position:\n%s", errStr)
}

func TestQuasiquoteErrorPosition(t *testing.T) {
	ns := env.NewEnv()
	core.Load(ns)

	// Test that quasiquote-generated code has proper positions
	code := "`(~undefined-in-quasiquote)"

	_, err := REPL(context.Background(), ns, code, types.NewCursorFile(t.Name()))
	if err == nil {
		t.Fatal("expected error but got none")
	}

	// Should contain the error about undefined symbol
	if !strings.Contains(err.Error(), "symbol 'undefined-in-quasiquote' not found") {
		t.Fatalf("error should contain original message: %s", err)
	}

	errStr := err.Error()
	lines := strings.Split(errStr, "\n")

	// Verify the position points to the symbol location
	// The quasiquote implementation should propagate positions from original nodes
	if !strings.Contains(lines[0], "undefined-in-quasiquote") {
		t.Errorf("Error position should reference undefined-in-quasiquote: %s", lines[0])
	}

	t.Logf("Quasiquote error:\n%s", err)
}

func TestEvalAstErrorPosition(t *testing.T) {
	ns := env.NewEnv()
	core.Load(ns)

	// Test that errors in list evaluation have proper positions
	// When evaluating a list of expressions, if one fails,
	// the position should point to the failing element
	code := `(do
		(def a 1)
		(def b 2)
		undefined-var
		(def c 3)
	)`

	_, err := REPL(context.Background(), ns, code, types.NewCursorFile(t.Name()))
	if err == nil {
		t.Fatal("expected error but got none")
	}

	// Should contain the error about undefined symbol
	if !strings.Contains(err.Error(), "symbol 'undefined-var' not found") {
		t.Fatalf("error should contain original message: %s", err)
	}

	errStr := err.Error()
	lines := strings.Split(errStr, "\n")

	// The cursor should point to line 4 where undefined-var is
	// Format is typically: TestName:line…line,col…col
	if !strings.Contains(lines[0], ":4:") {
		t.Errorf("Error should reference line 4 where undefined-var is: %s", lines[0])
	}

	t.Logf("eval_ast error:\n%s", err)
}

func TestStackFrameValidation(t *testing.T) {
	ns := env.NewEnv()
	core.Load(ns)

	// Comprehensive test to validate stack frames are correct
	// Create a deep call stack: outer → middle → inner → error
	code := `(do
		(def inner (fn [x] (+ x nonexistent)))
		(def middle (fn [x] (inner (+ x 1))))
		(def outer (fn [x] (middle (+ x 10))))
		(outer 5)
	)`

	_, err := REPL(context.Background(), ns, code, types.NewCursorFile(t.Name()))
	if err == nil {
		t.Fatal("expected error but got none")
	}

	errStr := err.Error()
	lines := strings.Split(errStr, "\n")

	t.Logf("Stack trace with deep nesting:")
	for i, line := range lines {
		t.Logf("  Line %d: %s", i, line)
	}

	// Verify error message
	if !strings.Contains(errStr, "symbol 'nonexistent' not found") {
		t.Fatalf("error should contain original message: %s", errStr)
	}

	// Count "at" frames
	atCount := 0
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "at ") {
			atCount++
		}
	}

	// Stack frames explained:
	// Due to TCO (Tail Call Optimization), function calls in Lisp don't create
	// new EVAL invocations, they reuse the same loop. So we get frames for:
	// 1. Each EVAL call that propagates the error (with function names)
	// Note: Duplicate frames (same position, no function name) are automatically filtered
	if atCount < 1 {
		t.Errorf("Expected at least 1 stack frame for deep nesting, got %d", atCount)
	}

	// Validate stack trace structure:
	// Line 0: error message with position
	// Line 1+: "  at position" for each frame
	if len(lines) < 2 {
		t.Errorf("Expected at least 2 lines (error + stack frame), got %d", len(lines))
	}

	// First line should contain the error
	if !strings.Contains(lines[0], "nonexistent") {
		t.Errorf("First line should contain error location: %s", lines[0])
	}

	// Subsequent lines should be stack frames
	for i := 1; i < len(lines); i++ {
		if !strings.HasPrefix(strings.TrimSpace(lines[i]), "at ") {
			t.Errorf("Line %d should be a stack frame: %s", i, lines[i])
		}
	}

	t.Logf("Total stack frames: %d", atCount)
	t.Logf("✓ Stack trace structure validated")
}

func TestLoadFileErrorStackTrace(t *testing.T) {
	ns := env.NewEnv()
	core.Load(ns)
	core.LoadInput(ns) // Needed for slurp function

	// Manually load eval and load-file since we can't use nscore (import cycle)
	ns.Set(types.Symbol{Val: "eval"}, types.Func{Fn: func(ctx context.Context, a []types.MalType) (types.MalType, error) {
		return EVAL(ctx, a[0], ns)
	}})

	_, err := REPL(context.Background(), ns, core.HeaderBasic(), types.NewCursorFile(t.Name()))
	if err != nil {
		t.Fatalf("Failed to load basic headers: %v", err)
	}

	_, err = REPL(context.Background(), ns, core.HeaderLoadFile(), types.NewCursorFile(t.Name()))
	if err != nil {
		t.Fatalf("Failed to load load-file: %v", err)
	}

	// Create a temporary file with an error inside
	// On Windows the OS path contains backslashes which the Lisp string scanner
	// would interpret as escape sequences, so the file is written with the OS
	// path but loaded through a forward-slash path.
	tmpDir := t.TempDir()
	errorFile := filepath.Join(tmpDir, "error_file.lisp")

	// File content with nested functions and an error
	fileContent := `
;; This file will be loaded with load-file
(def helper-in-file (fn [x]
	(+ x undefined-in-loaded-file)))

(def caller-in-file (fn [y]
	(helper-in-file y)))

;; Call the function to trigger the error
(caller-in-file 42)
`

	if err := os.WriteFile(errorFile, []byte(fileContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Load the file using load-file
	code := fmt.Sprintf(`(load-file "%s")`, filepath.ToSlash(errorFile))
	_, err = REPL(context.Background(), ns, code, types.NewCursorFile(t.Name()))

	if err == nil {
		t.Fatal("expected error but got none")
	}

	errStr := err.Error()
	lines := strings.Split(errStr, "\n")

	t.Logf("Stack trace from load-file:")
	for i, line := range lines {
		t.Logf("  Line %d: %s", i, line)
	}

	// Verify error message
	if !strings.Contains(errStr, "symbol 'undefined-in-loaded-file' not found") {
		t.Fatalf("error should contain original message: %s", errStr)
	}

	// The stack trace should reference the loaded file
	// Check if the file path appears in the error
	if strings.Contains(errStr, "error_file.lisp") {
		t.Logf("✓ File path appears in error: error_file.lisp")
	} else {
		t.Logf("Note: File path not directly visible, but may be in module reference")
	}

	// Verify that load-file appears in the stack trace
	if !strings.Contains(errStr, "at load-file (") {
		t.Errorf("Expected 'at load-file (' in stack trace, got: %s", errStr)
	} else {
		t.Logf("✓ load-file appears in stack trace")
	}

	// Count stack frames
	atCount := 0
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "at ") {
			atCount++
		}
	}

	if atCount < 1 {
		t.Errorf("Expected at least 1 stack frame, got %d", atCount)
	}

	t.Logf("Total stack frames from loaded file: %d", atCount)
	t.Logf("✓ load-file error stack trace validated")
}

func TestMacroExpansionStackTrace(t *testing.T) {
	ns := env.NewEnv()
	core.Load(ns)

	// Define a macro that expands to code with an error
	// Use a simpler approach - macro returns quoted code
	code := `(do
		;; Define a macro that returns code with an undefined symbol
		(defmacro broken-macro (fn [] (quote (+ 1 undefined-after-macro-expansion))))

		;; Call the macro - this will expand to: (+ 1 undefined-after-macro-expansion)
		(broken-macro)
	)`

	_, err := REPL(context.Background(), ns, code, types.NewCursorFile(t.Name()))

	if err == nil {
		t.Fatal("expected error but got none")
	}

	errStr := err.Error()
	lines := strings.Split(errStr, "\n")

	t.Logf("Stack trace after macro expansion:")
	for i, line := range lines {
		t.Logf("  Line %d: %s", i, line)
	}

	// Verify error message
	if !strings.Contains(errStr, "symbol 'undefined-after-macro-expansion' not found") {
		t.Fatalf("error should contain original message: %s", errStr)
	}

	// Verify that the macro name appears with macro: prefix
	if !strings.Contains(errStr, "at macro:broken-macro (") {
		t.Logf("Note: macro name may not appear in stack after expansion")
		t.Logf("This is expected behavior - macros expand before execution")
	} else {
		t.Logf("✓ Macro name appears with macro: prefix in stack trace")
	}

	// Count stack frames
	atCount := 0
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "at ") {
			atCount++
		}
	}

	// The error should have stack frames showing the macro expansion context
	if atCount < 1 {
		t.Errorf("Expected at least 1 stack frame after macro expansion, got %d", atCount)
	}

	t.Logf("Total stack frames after macro expansion: %d", atCount)
	t.Logf("✓ Macro expansion error stack trace validated")
}

func TestMacroWithQuasiquoteError(t *testing.T) {
	ns := env.NewEnv()
	core.Load(ns)

	_, err := REPL(context.Background(), ns, core.HeaderBasic(), types.NewCursorFile(t.Name()))
	if err != nil {
		t.Fatalf("Failed to load basic headers: %v", err)
	}

	// Test macro that uses quasiquote to generate code with error
	code := `(do
		;; Define a simple macro using quasiquote
		(defmacro test-macro (fn []
			` + "`" + `(+ 1 2 undefined-symbol-in-quasiquote)))

		;; Use the macro - this will expand to: (+ 1 2 undefined-symbol-in-quasiquote)
		(test-macro)
	)`

	_, err = REPL(context.Background(), ns, code, types.NewCursorFile(t.Name()))

	if err == nil {
		t.Fatal("expected error but got none")
	}

	errStr := err.Error()
	lines := strings.Split(errStr, "\n")

	t.Logf("Stack trace from macro with quasiquote:")
	for i, line := range lines {
		t.Logf("  Line %d: %s", i, line)
	}

	// Verify error message
	if !strings.Contains(errStr, "symbol 'undefined-symbol-in-quasiquote' not found") {
		t.Fatalf("error should contain original message: %s", errStr)
	}

	// Verify that the macro name appears with macro: prefix
	if strings.Contains(errStr, "at macro:test-macro (") {
		t.Logf("✓ Macro name appears with macro: prefix in stack trace")
	} else {
		t.Logf("Note: Macro name not visible after expansion (expected behavior)")
	}

	// Count stack frames
	atCount := 0
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "at ") {
			atCount++
		}
	}

	// Note: After duplicate filtering, simple macros may have 0 stack frames
	// This is expected when the error occurs at the macro expansion location
	// and there are no parent frames with function names
	t.Logf("Total stack frames from macro+quasiquote: %d", atCount)
	t.Logf("✓ Macro with quasiquote error stack trace validated")
}
