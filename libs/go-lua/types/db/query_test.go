package db

import "testing"

func TestInputRevisionBumps(t *testing.T) {
	db := New()
	input := NewInput[string, int](db)

	input.Set("a", 1)

	rev1 := input.revision("a")
	if rev1 == 0 {
		t.Fatal("expected non-zero revision after Set")
	}

	input.Set("a", 2)

	rev2 := input.revision("a")
	if rev2 <= rev1 {
		t.Fatalf("expected revision to increase, got %d then %d", rev1, rev2)
	}
}

func TestQueryCachesUntilInputChanges(t *testing.T) {
	db := New()
	ctx := NewQueryContext(db)
	input := NewInput[string, int](db)
	input.Set("x", 1)

	calls := 0
	q := NewQuery("double", func(ctx *QueryContext, key string) int {
		calls++
		v, _ := input.Get(ctx, key)

		return v * 2
	}, func(a, b int) bool { return a == b })

	if got := q.Get(ctx, "x"); got != 2 {
		t.Fatalf("got %d, want 2", got)
	}

	if got := q.Get(ctx, "x"); got != 2 {
		t.Fatalf("got %d, want 2", got)
	}

	if calls != 1 {
		t.Fatalf("expected 1 compute call, got %d", calls)
	}

	input.Set("x", 2)

	if got := q.Get(ctx, "x"); got != 4 {
		t.Fatalf("got %d, want 4", got)
	}

	if calls != 2 {
		t.Fatalf("expected 2 compute calls after input change, got %d", calls)
	}
}

func TestQueryTracksQueryDependencies(t *testing.T) {
	db := New()
	ctx := NewQueryContext(db)
	input := NewInput[string, int](db)
	input.Set("n", 3)

	baseCalls := 0
	base := NewQuery("base", func(ctx *QueryContext, key string) int {
		baseCalls++
		v, _ := input.Get(ctx, key)

		return v
	}, func(a, b int) bool { return a == b })

	derivedCalls := 0
	derived := NewQuery("derived", func(ctx *QueryContext, key string) int {
		derivedCalls++
		return base.Get(ctx, key) + 1
	}, func(a, b int) bool { return a == b })

	if got := derived.Get(ctx, "n"); got != 4 {
		t.Fatalf("got %d, want 4", got)
	}

	if got := derived.Get(ctx, "n"); got != 4 {
		t.Fatalf("got %d, want 4", got)
	}

	if baseCalls != 1 || derivedCalls != 1 {
		t.Fatalf("expected 1 call each, got base=%d derived=%d", baseCalls, derivedCalls)
	}

	input.Set("n", 5)

	if got := derived.Get(ctx, "n"); got != 6 {
		t.Fatalf("got %d, want 6", got)
	}

	if baseCalls != 2 || derivedCalls != 2 {
		t.Fatalf("expected recompute after input change, got base=%d derived=%d", baseCalls, derivedCalls)
	}
}

func TestQueryCycleUsesWiden(t *testing.T) {
	db := New()
	ctx := NewQueryContext(db)

	var q *Query[int, int]
	q = NewQueryWithWiden("cycle", func(ctx *QueryContext, key int) int {
		return q.Get(ctx, key) + 1
	}, func(a, b int) bool { return a == b }, func(prev, _ int) int {
		return prev
	})

	if got := q.Get(ctx, 1); got != 1 {
		t.Fatalf("expected widened result 1, got %d", got)
	}
}
