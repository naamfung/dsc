package internal

import "testing"

func TestRecursionGuardDepth(t *testing.T) {
	t.Parallel()

	guard := NewRecursionGuard(1)

	next, ok := guard.Enter(nil)
	if !ok {
		t.Fatalf("expected first enter to succeed")
	}

	next, ok = next.Enter(nil)
	if !ok {
		t.Fatalf("expected second enter to succeed")
	}

	if _, ok = next.Enter(nil); ok {
		t.Fatalf("expected third enter to fail at max depth")
	}
}
