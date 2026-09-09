// Package db provides a Salsa-style incremental computation database for
// efficient re-analysis when source files change.
//
// The db package enables incremental type checking by tracking dependencies
// between computations and only recomputing what changed. This is essential
// for responsive IDE tooling where most edits affect only a small portion
// of the program.
//
// # Core Concepts
//
// Revision: A monotonically increasing version number bumped whenever inputs
// change. Used to track staleness of cached computations.
//
// Input: A memoized query that caches values by key with VerifiedAt tracking.
// When an input is accessed, its VerifiedAt is compared to the current revision
// to determine if recomputation is needed.
//
// Interning: Structural deduplication ensuring that types with identical
// structure share the same pointer. This enables cheap equality checks and
// stable memoization keys.
//
// Manifest: Cross-module type information loaded from dependencies. Manifests
// are connected/disconnected from the database to enable module-level caching.
//
// # Thread Safety
//
// The database is thread-safe for concurrent access. Multiple goroutines can
// query and update the database simultaneously without external synchronization.
package db

import (
	"sync"
	"sync/atomic"

	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// Revision is a monotonically increasing version number.
// Used to track when inputs changed and when queries were verified.
type Revision uint64

// DB is the central database for incremental computation.
// Thread-safe for concurrent access.
type DB struct {
	revision  uint64
	interner  *Interner
	manifests *Input[string, *io.Manifest]
}

// New creates a new database starting at revision 1.
func New() *DB {
	db := &DB{
		revision: 1,
		interner: newInterner(),
	}
	db.manifests = NewInput[string, *io.Manifest](db)

	return db
}

// Revision returns the current revision number.
func (db *DB) Revision() Revision {
	return Revision(atomic.LoadUint64(&db.revision))
}

// Bump increments the revision and returns the new value.
// Call this when inputs change.
func (db *DB) Bump() Revision {
	return Revision(atomic.AddUint64(&db.revision, 1))
}

// Intern returns the canonical instance of a value.
// Values with the same structural hash and equality share the same pointer.
func (db *DB) Intern(key uint64, factory func() any, equal func(existing, candidate any) bool) any {
	return db.interner.Intern(key, factory, equal)
}

// InternType returns a canonical instance of a type for stable memoization keys.
func (db *DB) InternType(t typ.Type) typ.Type {
	if db == nil || t == nil {
		return t
	}

	if fn, ok := t.(*typ.Function); ok {
		if fn.Effects != nil || fn.Spec != nil || fn.Refinement != nil {
			return t
		}
	}

	value := db.Intern(t.Hash(), func() any { return t }, func(existing, candidate any) bool {
		return typ.TypeEquals(existing.(typ.Type), candidate.(typ.Type))
	})

	return value.(typ.Type)
}

// Connect adds a manifest to the database.
// Bumps revision to invalidate dependent queries.
func (db *DB) Connect(path string, manifest *io.Manifest) {
	if db == nil {
		return
	}

	if current, ok := db.manifests.Get(nil, path); ok && current == manifest {
		return
	}

	db.manifests.Set(path, manifest)
}

// Disconnect removes a manifest from the database.
// Bumps revision to invalidate dependent queries.
func (db *DB) Disconnect(path string) {
	if db == nil {
		return
	}

	if current, ok := db.manifests.Get(nil, path); ok && current == nil {
		return
	}

	db.manifests.Set(path, nil)
}

// Manifest returns the manifest at the given path, or nil.
func (db *DB) Manifest(path string) *io.Manifest {
	v, _ := db.manifests.Get(nil, path)
	return v
}

// Manifests iterates over all connected manifests.
func (db *DB) Manifests(fn func(path string, manifest *io.Manifest) bool) {
	db.manifests.Range(func(path string, manifest *io.Manifest) bool {
		return fn(path, manifest)
	})
}

// Compile-time check that DB implements io.ManifestQuerier.
var _ io.ManifestQuerier = (*DB)(nil)

// Imports implements io.ManifestQuerier.
// Returns a copy of the manifest map for read-only access.
func (db *DB) Imports() map[string]*io.Manifest {
	if db == nil {
		return nil
	}
	result := make(map[string]*io.Manifest)
	db.Manifests(func(path string, m *io.Manifest) bool {
		result[path] = m
		return true
	})
	return result
}

// Interner provides structural deduplication.
type Interner struct {
	mu    sync.RWMutex
	cache map[uint64][]any
}

const maxBucketSize = 8

func newInterner() *Interner {
	return &Interner{
		cache: make(map[uint64][]any),
	}
}

// Intern returns canonical instance for given hash.
func (i *Interner) Intern(key uint64, factory func() any, equal func(existing, candidate any) bool) any {
	i.mu.RLock()
	bucket := i.cache[key]

	if len(bucket) > 0 {
		i.mu.RUnlock()

		candidate := factory()

		i.mu.Lock()
		defer i.mu.Unlock()

		bucket = i.cache[key]

		for _, existing := range bucket {
			if equal == nil || equal(existing, candidate) {
				return existing
			}
		}

		if len(bucket) < maxBucketSize {
			i.cache[key] = append(bucket, candidate)
		}

		return candidate
	}
	i.mu.RUnlock()

	candidate := factory()

	i.mu.Lock()
	defer i.mu.Unlock()

	// Double-check after acquiring write lock
	bucket = i.cache[key]

	if len(bucket) > 0 {
		for _, existing := range bucket {
			if equal == nil || equal(existing, candidate) {
				return existing
			}
		}

		if len(bucket) < maxBucketSize {
			i.cache[key] = append(bucket, candidate)
		}

		return candidate
	}

	i.cache[key] = []any{candidate}

	return candidate
}

// MemoEntry stores a memoized query result with version tracking.
type MemoEntry struct {
	Value      any
	VerifiedAt Revision
	UpdatedAt  Revision
	Deps       []dep
}
