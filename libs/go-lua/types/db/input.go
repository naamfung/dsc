package db

import (
	"sort"
	"sync"
)

// Input represents a tracked input value keyed by K.
//
// Input is the fundamental source of truth in the incremental computation model.
// Unlike memoized queries that derive their values, inputs are set directly and
// form the leaves of the dependency graph.
//
// Each key-value pair carries its own revision number for fine-grained dependency
// tracking. When an input value changes, only queries that depend on that specific
// key need recomputation.
//
// Thread safety: All methods are safe for concurrent use via internal locking.
//
// Example usage:
//
//	sourceInputs := db.NewInput[string, string](database)
//	sourceInputs.Set("main.lua", sourceCode)
//
//	// Queries that call sourceInputs.Get() will track dependency on that key
//	result := someQuery.Get(ctx, "main.lua")
type Input[K comparable, V any] struct {
	db     *DB
	mu     sync.RWMutex
	values map[K]inputEntry[V]
}

type inputEntry[V any] struct {
	value    V
	revision Revision
}

// NewInput creates a new input table bound to a DB.
func NewInput[K comparable, V any](db *DB) *Input[K, V] {
	return &Input[K, V]{
		db:     db,
		values: make(map[K]inputEntry[V]),
	}
}

// Set updates the input value and bumps its revision.
//
// This operation also bumps the database's global revision, which triggers
// revalidation of any queries that may depend on this input.
func (i *Input[K, V]) Set(key K, value V) {
	if i == nil {
		return
	}

	i.mu.Lock()
	defer i.mu.Unlock()
	rev := i.db.Bump()
	i.values[key] = inputEntry[V]{value: value, revision: rev}
}

// Get returns the input value for key.
//
// When called within a query context (ctx is non-nil), this records a dependency
// on the input at the current revision. Future calls to the containing query
// will revalidate by checking if this input's revision has increased.
//
// Returns the value and true if found, or zero value and false if not.
func (i *Input[K, V]) Get(ctx *QueryContext, key K) (V, bool) {
	if i == nil {
		var zero V
		return zero, false
	}

	i.mu.RLock()
	entry, ok := i.values[key]
	i.mu.RUnlock()

	if ctx != nil && ctx.hasActiveFrame() {
		last := entry.revision

		ctx.recordDep(dep{
			kind:   depKindInput,
			source: i,
			key:    key,
			last:   last,
		})
	}

	return entry.value, ok
}

func (i *Input[K, V]) revision(key K) Revision {
	if i == nil {
		return 0
	}

	i.mu.RLock()
	defer i.mu.RUnlock()

	entry, ok := i.values[key]
	if !ok {
		return 0
	}

	return entry.revision
}

func (i *Input[K, V]) revisionAny(key any) Revision {
	return i.revision(key.(K))
}

// Range iterates over all stored values in a deterministic order.
//
// For ordered key types (string, numeric), iteration occurs in sorted order.
// For other key types, iteration order is unspecified but consistent within
// a single Range call.
//
// The callback returns false to stop iteration early.
func (i *Input[K, V]) Range(fn func(K, V) bool) {
	if i == nil || fn == nil {
		return
	}

	i.mu.RLock()
	defer i.mu.RUnlock()

	keys := make([]K, 0, len(i.values))
	for k := range i.values {
		keys = append(keys, k)
	}

	if less, ok := orderedLess[K](); ok {
		sort.Slice(keys, func(a, b int) bool {
			return less(keys[a], keys[b])
		})
	}

	for _, k := range keys {
		entry := i.values[k]
		if !fn(k, entry.value) {
			return
		}
	}
}

func orderedLess[K comparable]() (func(a, b K) bool, bool) {
	var zero K
	switch any(zero).(type) {
	case string:
		return func(a, b K) bool { return any(a).(string) < any(b).(string) }, true
	case int:
		return func(a, b K) bool { return any(a).(int) < any(b).(int) }, true
	case int8:
		return func(a, b K) bool { return any(a).(int8) < any(b).(int8) }, true
	case int16:
		return func(a, b K) bool { return any(a).(int16) < any(b).(int16) }, true
	case int32:
		return func(a, b K) bool { return any(a).(int32) < any(b).(int32) }, true
	case int64:
		return func(a, b K) bool { return any(a).(int64) < any(b).(int64) }, true
	case uint:
		return func(a, b K) bool { return any(a).(uint) < any(b).(uint) }, true
	case uint8:
		return func(a, b K) bool { return any(a).(uint8) < any(b).(uint8) }, true
	case uint16:
		return func(a, b K) bool { return any(a).(uint16) < any(b).(uint16) }, true
	case uint32:
		return func(a, b K) bool { return any(a).(uint32) < any(b).(uint32) }, true
	case uint64:
		return func(a, b K) bool { return any(a).(uint64) < any(b).(uint64) }, true
	case float32:
		return func(a, b K) bool { return any(a).(float32) < any(b).(float32) }, true
	case float64:
		return func(a, b K) bool { return any(a).(float64) < any(b).(float64) }, true
	default:
		return nil, false
	}
}
