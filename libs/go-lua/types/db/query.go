package db

import (
	"sync"

	"github.com/wippyai/go-lua/internal"
)

// QueryContext carries query dependency tracking state.
//
// A QueryContext tracks which inputs and queries are accessed during computation,
// enabling the incremental computation system to determine which cached values
// need revalidation when inputs change.
//
// Lifecycle:
//   - Create a new QueryContext for each top-level analysis run
//   - Pass the same context through all query calls in that run
//   - Discard after the run completes
//
// Thread safety: QueryContext is not safe for concurrent use. Use a single
// context per goroutine, or serialize access externally.
//
// The context also carries attachments (via Attach/Attached) for passing
// additional state through the query graph without modifying function signatures.
type QueryContext struct {
	db          *DB
	tracker     *tracker
	attachments map[string]any
	validation  *QueryContext
}

// NewQueryContext creates a query context for a DB.
func NewQueryContext(db *DB) *QueryContext {
	return &QueryContext{
		db: db,
		tracker: &tracker{
			inProgress: make(map[cycleKey]bool),
			cycle:      make(map[cycleKey]bool),
		},
	}
}

// DB returns the backing database for this context.
func (c *QueryContext) DB() *DB {
	if c == nil {
		return nil
	}

	return c.db
}

func (c *QueryContext) hasActiveFrame() bool {
	return c != nil && c.tracker != nil && len(c.tracker.stack) > 0
}

func (c *QueryContext) validationContext() *QueryContext {
	if c == nil {
		return nil
	}

	if c.tracker == nil {
		return NewQueryContext(c.db)
	}

	if c.validation == nil {
		c.validation = &QueryContext{
			db: c.db,
			tracker: &tracker{
				inProgress: c.tracker.inProgress,
				cycle:      c.tracker.cycle,
			},
			attachments: c.attachments,
		}
		return c.validation
	}

	validationTracker := c.validation.tracker
	validationTracker.inProgress = c.tracker.inProgress
	validationTracker.cycle = c.tracker.cycle
	if len(validationTracker.stack) > 0 {
		validationTracker.stack = validationTracker.stack[:0]
	}
	c.validation.attachments = c.attachments

	return c.validation
}

type tracker struct {
	stack      []frame
	inProgress map[cycleKey]bool // tracks queries currently being computed to detect cycles
	cycle      map[cycleKey]bool // tracks queries that detected a cycle
}

type frame struct {
	deps []dep
}

type dep struct {
	kind   depKind
	source any
	key    any
	last   Revision
}

type depKind uint8

const (
	depKindCustom depKind = iota
	depKindQuery
	depKindInput
)

type queryDepRevalidator interface {
	revalidateAny(*QueryContext, any) Revision
}

type inputDepRevisioner interface {
	revisionAny(any) Revision
}

// cycleKey uniquely identifies a query invocation for cycle detection.
type cycleKey struct {
	query any
	key   any
}

func (t *tracker) push() {
	t.stack = append(t.stack, frame{})
}

func (t *tracker) pop() []dep {
	if len(t.stack) == 0 {
		return nil
	}

	top := t.stack[len(t.stack)-1]
	t.stack = t.stack[:len(t.stack)-1]

	return top.deps
}

func (c *QueryContext) recordDep(d dep) {
	if !c.hasActiveFrame() {
		return
	}

	top := &c.tracker.stack[len(c.tracker.stack)-1]
	top.deps = append(top.deps, d)
}

const defaultMaxIter = 64

// Query represents a memoized query with dependency tracking.
//
// Query is the core abstraction for incremental computation. Each Query:
//   - Computes a value V from a key K using the provided compute function
//   - Caches results and tracks which inputs/queries were accessed
//   - Revalidates cached values when dependencies change
//   - Handles recursive/cyclic queries via fixpoint iteration
//
// Memoization strategy:
//   - First call: compute and cache result with dependency list
//   - Subsequent calls at same revision: return cached value
//   - After revision bump: check if dependencies changed, recompute if needed
//
// Cycle handling:
//   - Detects recursive calls to the same (query, key) pair
//   - Uses seed value for initial approximation
//   - Iterates to fixpoint using equal function for convergence check
//   - Applies widen function (if provided) to ensure termination
//
// Thread safety: Query is safe for concurrent use via internal locking.
type Query[K comparable, V any] struct {
	name        string
	compute     func(*QueryContext, K) V
	equal       func(a, b V) bool
	widen       func(prev, next V) V
	alwaysWiden bool
	seed        func() V
	onMaxIter   func(iter int, last V, key K) (V, bool)
	maxIter     int
	mu          sync.RWMutex
	cache       map[K]*MemoEntry
}

// NewQuery constructs a memoized query.
//
// Parameters:
//   - name: identifier for debugging and cycle detection
//   - compute: function to compute V from K (may call other queries)
//   - equal: equality function for result comparison and convergence
//
// If equal is nil, defaults to internal.Equaler interface check or pointer equality.
func NewQuery[K comparable, V any](
	name string,
	compute func(*QueryContext, K) V,
	equal func(a, b V) bool,
) *Query[K, V] {
	return NewQueryWithWiden(name, compute, equal, nil)
}

// NewQueryWithWiden constructs a query with a widening function for cycles.
//
// The widen function is called during fixpoint iteration when a cycle is detected.
// It should return a value that is "wider" than both inputs, ensuring eventual
// convergence. Common widening strategies include unioning type sets or taking
// upper bounds in lattices.
//
// Without widening, cycles may iterate indefinitely. With widening, the sequence
// prev, widen(prev, next), widen(..., next), ... is guaranteed to stabilize.
func NewQueryWithWiden[K comparable, V any](
	name string,
	compute func(*QueryContext, K) V,
	equal func(a, b V) bool,
	widen func(prev, next V) V,
) *Query[K, V] {
	var zero V

	return &Query[K, V]{
		name:    name,
		compute: compute,
		equal:   equal,
		widen:   widen,
		seed:    func() V { return zero },
		maxIter: defaultMaxIter,
		cache:   make(map[K]*MemoEntry),
	}
}

// SetMaxIter controls how many fixpoint iterations to attempt before triggering onMaxIter.
func (q *Query[K, V]) SetMaxIter(limit int) {
	if q == nil {
		return
	}

	q.maxIter = limit
}

// SetAlwaysWiden enables widening on every update, not just during cycles.
// Use for monotone analyses where results should only grow.
func (q *Query[K, V]) SetAlwaysWiden(enabled bool) {
	if q == nil {
		return
	}
	q.alwaysWiden = enabled
}

// SetOnMaxIter registers a handler for non-converging cycles.
func (q *Query[K, V]) SetOnMaxIter(fn func(iter int, last V, key K) (V, bool)) {
	if q == nil {
		return
	}

	q.onMaxIter = fn
}

// SetOnMaxIterReturnLast avoids panics by returning the last value after max iterations.
func (q *Query[K, V]) SetOnMaxIterReturnLast() {
	q.SetOnMaxIter(func(_ int, last V, _ K) (V, bool) { return last, true })
}

// Get returns the query result, recomputing if dependencies changed.
//
// Algorithm:
//  1. If cached and verified at current revision, return cached value
//  2. If cached but not verified, check if dependencies changed
//  3. If dependencies unchanged, mark verified and return cached value
//  4. Otherwise, recompute, update cache, and return new value
//
// Cycle handling: If this (query, key) is already being computed (recursive call),
// returns the current approximation (seed value on first iteration, previous
// result on subsequent iterations). After the outermost call completes, iterates
// to fixpoint if a cycle was detected.
//
// If ctx is nil, computes directly without memoization or dependency tracking.
func (q *Query[K, V]) Get(ctx *QueryContext, key K) V {
	if q == nil {
		var zero V
		return zero
	}

	if ctx == nil || ctx.tracker == nil {
		return q.compute(ctx, key)
	}

	// Cycle detection: if in progress, return current approximation.
	ck := cycleKey{query: q, key: key}
	if ctx.tracker.inProgress[ck] {
		ctx.tracker.cycle[ck] = true

		if value, ok := q.getCachedValue(key); ok {
			q.recordQueryDep(ctx, key, q.updatedAt(key))
			return value
		}

		if q.seed != nil {
			q.recordQueryDep(ctx, key, q.updatedAt(key))
			return q.seed()
		}

		var zero V

		return zero
	}

	// Read cache atomically - copy values needed for validation
	equalFn := q.equal

	var cachedValue V

	var cachedDeps []dep

	var cachedUpdatedAt Revision

	var cachedVerifiedAt Revision
	hasEntry := q.readCacheEntry(key, &cachedValue, &cachedDeps, &cachedUpdatedAt, &cachedVerifiedAt)

	if equalFn == nil {
		equalFn = anyEqual[V]
	}

	// Fast path: already verified at this revision.
	if hasEntry && cachedVerifiedAt == ctx.db.Revision() {
		q.recordQueryDep(ctx, key, cachedUpdatedAt)
		return cachedValue
	}

	// Use copied values for validation check.
	if hasEntry && !depsChanged(ctx.validationContext(), cachedDeps) {
		q.markVerified(key, ctx.db.Revision())
		q.recordQueryDep(ctx, key, cachedUpdatedAt)

		return cachedValue
	}

	var value V

	var updatedAt Revision

	var seen []V

	iterations := 0

	for {
		// Mark as in progress before computing.
		ctx.tracker.inProgress[ck] = true
		ctx.tracker.push()

		var deps []dep

		func() {
			defer func() {
				if r := recover(); r != nil {
					ctx.tracker.pop()
					delete(ctx.tracker.inProgress, ck)
					delete(ctx.tracker.cycle, ck)
					panic(r)
				}
			}()

			value = q.compute(ctx, key)
			deps = ctx.tracker.pop()
		}()
		delete(ctx.tracker.inProgress, ck)

		cycled := ctx.tracker.cycle[ck]
		value, updatedAt = q.updateCacheEntry(key, value, deps, ctx.db.Revision(), equalFn, cycled && q.widen != nil)

		q.recordQueryDep(ctx, key, updatedAt)
		delete(ctx.tracker.cycle, ck)

		if !cycled {
			return value
		}

		// If we detected a cycle, iterate to a fixpoint.
		if len(seen) > 0 && equalFn(seen[len(seen)-1], value) {
			return value
		}

		for i := range len(seen) - 1 {
			if equalFn(seen[i], value) {
				return value
			}
		}

		seen = append(seen, value)

		iterations++
		if q.maxIter > 0 && iterations >= q.maxIter {
			if q.onMaxIter != nil {
				if result, ok := q.onMaxIter(iterations, value, key); ok {
					return result
				}
			}

			panic("db.Query.Get exceeded max fixpoint iterations for " + q.name)
		}
	}
}

func (q *Query[K, V]) recordQueryDep(ctx *QueryContext, key K, last Revision) {
	if !ctx.hasActiveFrame() {
		return
	}

	ctx.recordDep(q.queryDep(key, last))
}

func (q *Query[K, V]) queryDep(key K, last Revision) dep {
	return dep{
		kind:   depKindQuery,
		source: q,
		key:    key,
		last:   last,
	}
}

func (q *Query[K, V]) revalidateAny(ctx *QueryContext, key any) Revision {
	return q.revalidate(ctx, key.(K))
}

func (q *Query[K, V]) revalidate(ctx *QueryContext, key K) Revision {
	_ = q.Get(ctx, key)
	return q.updatedAt(key)
}

func (q *Query[K, V]) updatedAt(key K) Revision {
	if q == nil {
		return 0
	}

	q.mu.RLock()
	defer q.mu.RUnlock()

	entry := q.cache[key]
	if entry == nil {
		return 0
	}

	return entry.UpdatedAt
}

// getCachedValue returns the cached value if present.
func (q *Query[K, V]) getCachedValue(key K) (V, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	entry := q.cache[key]
	if entry != nil && entry.Value != nil {
		return entry.Value.(V), true
	}

	var zero V

	return zero, false
}

// readCacheEntry copies cache entry fields under lock.
func (q *Query[K, V]) readCacheEntry(key K, value *V, deps *[]dep, updatedAt, verifiedAt *Revision) bool {
	q.mu.RLock()
	defer q.mu.RUnlock()

	entry := q.cache[key]
	if entry == nil {
		return false
	}

	if entry.Value != nil {
		*value = entry.Value.(V)
	}

	*deps = entry.Deps

	*updatedAt = entry.UpdatedAt
	*verifiedAt = entry.VerifiedAt

	return true
}

// markVerified updates the VerifiedAt timestamp under lock.
func (q *Query[K, V]) markVerified(key K, rev Revision) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if e := q.cache[key]; e != nil {
		e.VerifiedAt = rev
	}
}

// updateCacheEntry updates cache entry under lock and returns the potentially widened value and updatedAt.
func (q *Query[K, V]) updateCacheEntry(
	key K,
	value V,
	deps []dep,
	rev Revision,
	equalFn func(V, V) bool,
	applyWiden bool,
) (V, Revision) {
	q.mu.Lock()
	defer q.mu.Unlock()

	entry := q.cache[key]
	if entry == nil {
		entry = &MemoEntry{}
		q.cache[key] = entry
	}

	// Apply widening if needed
	if (applyWiden || q.alwaysWiden) && entry.Value != nil {
		prevVal := entry.Value.(V)
		value = q.widen(prevVal, value)
	}

	entry.VerifiedAt = rev
	entry.Deps = deps

	if entry.UpdatedAt == 0 {
		entry.Value = value
		entry.UpdatedAt = rev
	} else {
		existing := entry.Value.(V)
		if !equalFn(existing, value) {
			entry.Value = value
			entry.UpdatedAt = rev
		}
	}

	return value, entry.UpdatedAt
}

func depsChanged(ctx *QueryContext, deps []dep) bool {
	for _, d := range deps {
		if d.changedAt(ctx) {
			return true
		}
	}

	return false
}

func (d dep) changedAt(ctx *QueryContext) bool {
	if ctx == nil {
		return false
	}

	if ctx.db != nil && ctx.db.Revision() <= d.last {
		return false
	}

	switch d.kind {
	case depKindQuery:
		query, ok := d.source.(queryDepRevalidator)
		if !ok || query == nil {
			return false
		}
		return query.revalidateAny(ctx, d.key) > d.last
	case depKindInput:
		input, ok := d.source.(inputDepRevisioner)
		if !ok || input == nil {
			return false
		}
		return input.revisionAny(d.key) > d.last
	default:
		changed, ok := d.source.(func(*QueryContext) bool)
		return ok && changed != nil && changed(ctx)
	}
}

// anyEqual compares two values using internal.Equaler if available.
// Falls back to pointer equality for non-comparable types.
func anyEqual[V any](a, b V) bool {
	// Try internal.Equaler interface first
	if ea, ok := any(a).(internal.Equaler); ok {
		return ea.Equals(b)
	}
	// Try comparable equality via any comparison
	return any(a) == any(b)
}

// Clear removes all entries from the query cache.
//
// Use for batch operations where memoization between files isn't needed,
// or when the database is reset. Does not affect other queries in the database.
func (q *Query[K, V]) Clear() {
	if q == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	clear(q.cache)
}
