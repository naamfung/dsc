package constraint

import (
	"sync"
)

// Interner provides constraint interning to reduce allocations for common patterns.
// Thread-safe for concurrent use.
//
// Interning is beneficial when the same constraints are created repeatedly,
// such as Truthy/Falsy/IsNil/NotNil on frequently-accessed paths. Interned
// constraints share storage, reducing GC pressure in hot paths.
//
// Usage:
//
//	interner := NewInterner()
//	c1 := interner.Truthy(path)
//	c2 := interner.Truthy(path)  // returns same instance as c1
type Interner struct {
	mu sync.RWMutex

	truthy map[PathKey]Truthy
	falsy  map[PathKey]Falsy
	isNil  map[PathKey]IsNil
	notNil map[PathKey]NotNil
}

// NewInterner creates a new constraint interner.
func NewInterner() *Interner {
	return &Interner{
		truthy: make(map[PathKey]Truthy),
		falsy:  make(map[PathKey]Falsy),
		isNil:  make(map[PathKey]IsNil),
		notNil: make(map[PathKey]NotNil),
	}
}

// Truthy returns an interned Truthy constraint for the given path.
func (i *Interner) Truthy(p Path) Truthy {
	if p.IsEmpty() {
		return Truthy{}
	}
	key := p.Key()

	i.mu.RLock()
	if c, ok := i.truthy[key]; ok {
		i.mu.RUnlock()
		return c
	}
	i.mu.RUnlock()

	i.mu.Lock()
	defer i.mu.Unlock()

	// Double-check after acquiring write lock
	if c, ok := i.truthy[key]; ok {
		return c
	}

	c := Truthy{Path: p}
	i.truthy[key] = c
	return c
}

// Falsy returns an interned Falsy constraint for the given path.
func (i *Interner) Falsy(p Path) Falsy {
	if p.IsEmpty() {
		return Falsy{}
	}
	key := p.Key()

	i.mu.RLock()
	if c, ok := i.falsy[key]; ok {
		i.mu.RUnlock()
		return c
	}
	i.mu.RUnlock()

	i.mu.Lock()
	defer i.mu.Unlock()

	if c, ok := i.falsy[key]; ok {
		return c
	}

	c := Falsy{Path: p}
	i.falsy[key] = c
	return c
}

// IsNil returns an interned IsNil constraint for the given path.
func (i *Interner) IsNil(p Path) IsNil {
	if p.IsEmpty() {
		return IsNil{}
	}
	key := p.Key()

	i.mu.RLock()
	if c, ok := i.isNil[key]; ok {
		i.mu.RUnlock()
		return c
	}
	i.mu.RUnlock()

	i.mu.Lock()
	defer i.mu.Unlock()

	if c, ok := i.isNil[key]; ok {
		return c
	}

	c := IsNil{Path: p}
	i.isNil[key] = c
	return c
}

// NotNil returns an interned NotNil constraint for the given path.
func (i *Interner) NotNil(p Path) NotNil {
	if p.IsEmpty() {
		return NotNil{}
	}
	key := p.Key()

	i.mu.RLock()
	if c, ok := i.notNil[key]; ok {
		i.mu.RUnlock()
		return c
	}
	i.mu.RUnlock()

	i.mu.Lock()
	defer i.mu.Unlock()

	if c, ok := i.notNil[key]; ok {
		return c
	}

	c := NotNil{Path: p}
	i.notNil[key] = c
	return c
}

// Size returns the total number of interned constraints.
func (i *Interner) Size() int {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return len(i.truthy) + len(i.falsy) + len(i.isNil) + len(i.notNil)
}

// Clear removes all interned constraints.
func (i *Interner) Clear() {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.truthy = make(map[PathKey]Truthy)
	i.falsy = make(map[PathKey]Falsy)
	i.isNil = make(map[PathKey]IsNil)
	i.notNil = make(map[PathKey]NotNil)
}
