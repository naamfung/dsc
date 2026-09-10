package index

// Query retrieves a cached value or computes and stores it.
// If the key exists and is valid, returns the cached value.
// Otherwise, calls compute(), stores the result, and returns it.
func (db *DB) Query(k Key, compute func() any) any {
	if v, ok := db.Get(k); ok {
		return v
	}
	v := compute()
	db.Set(k, v, nil)
	return v
}

// QueryWithDeps retrieves or computes a value with explicit dependencies.
func (db *DB) QueryWithDeps(k Key, deps []Key, compute func() any) any {
	if v, ok := db.Get(k); ok {
		return v
	}
	v := compute()
	db.Set(k, v, deps)
	return v
}

// Has returns true if the key exists and is valid.
func (db *DB) Has(k Key) bool {
	_, ok := db.Get(k)
	return ok
}

// Delete removes a specific key from the cache.
func (db *DB) Delete(k Key) {
	db.mu.Lock()
	defer db.mu.Unlock()
	delete(db.store, k)
}

// Keys returns all keys currently in the cache.
func (db *DB) Keys() []Key {
	db.mu.RLock()
	defer db.mu.RUnlock()
	keys := make([]Key, 0, len(db.store))
	for k := range db.store {
		keys = append(keys, k)
	}
	return keys
}

// KeysForFile returns all keys for a specific file.
func (db *DB) KeysForFile(file string) []Key {
	db.mu.RLock()
	defer db.mu.RUnlock()
	var keys []Key
	for k := range db.store {
		if k.File == file {
			keys = append(keys, k)
		}
	}
	return keys
}
