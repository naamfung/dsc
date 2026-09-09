package index

import "sync"

// Key identifies a cached computation result.
type Key struct {
	File string // Source file path
	Func string // Function name (empty for file-level)
	Kind string // Cache kind: "check", "infer", "effects", etc.
}

// Entry stores a cached value with version and dependencies.
type Entry struct {
	Value   any   // The cached result
	Version int64 // Version when entry was created
	Deps    []Key // Dependencies for invalidation
}

// DB is a thread-safe cache database for type checking results.
type DB struct {
	mu      sync.RWMutex
	store   map[Key]Entry
	version int64
}

// NewDB creates a new empty cache database.
func NewDB() *DB {
	return &DB{
		store: make(map[Key]Entry),
	}
}

// Get retrieves a cached entry if it exists and is still valid.
// Entries created before a Clear() call are considered stale.
func (db *DB) Get(k Key) (any, bool) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	e, ok := db.store[k]
	if !ok || e.Version < db.version {
		return nil, false
	}
	return e.Value, true
}

// Set stores a value in the cache with optional dependencies.
func (db *DB) Set(k Key, v any, deps []Key) {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.store[k] = Entry{Value: v, Version: db.version, Deps: deps}
}

// InvalidateFile removes all cache entries for a specific file.
func (db *DB) InvalidateFile(file string) {
	db.mu.Lock()
	defer db.mu.Unlock()
	for k := range db.store {
		if k.File == file {
			delete(db.store, k)
		}
	}
}

// InvalidateWithDependents removes an entry and all entries that depend on it.
func (db *DB) InvalidateWithDependents(k Key) {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.invalidateCascade(k, make(map[Key]bool))
}

// InvalidateFileWithDependents removes all entries for a file and cascades to dependents.
func (db *DB) InvalidateFileWithDependents(file string) {
	db.mu.Lock()
	defer db.mu.Unlock()

	visited := make(map[Key]bool)
	for k := range db.store {
		if k.File == file {
			db.invalidateCascade(k, visited)
		}
	}
}

// invalidateCascade recursively removes entries and their dependents.
func (db *DB) invalidateCascade(k Key, visited map[Key]bool) {
	if visited[k] {
		return
	}
	visited[k] = true

	// Find all entries that depend on k
	for key, entry := range db.store {
		for _, dep := range entry.Deps {
			if dep == k {
				db.invalidateCascade(key, visited)
				break
			}
		}
	}
	delete(db.store, k)
}

// InvalidateFunc removes cache entries for a specific function.
func (db *DB) InvalidateFunc(file, funcName string) {
	db.mu.Lock()
	defer db.mu.Unlock()
	for k := range db.store {
		if k.File == file && k.Func == funcName {
			delete(db.store, k)
		}
	}
}

// Clear removes all entries and bumps the version.
func (db *DB) Clear() {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.store = make(map[Key]Entry)
	db.version++
}

// Version returns the current cache version.
func (db *DB) Version() int64 {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.version
}

// Size returns the number of cached entries.
func (db *DB) Size() int {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return len(db.store)
}
