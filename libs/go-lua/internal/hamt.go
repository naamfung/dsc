// Package internal provides pure data structures for the type system.
//
// HAMT (Hash Array Mapped Trie):
//   - Get: O(log32 n) ≈ O(1) for practical sizes
//   - Set: O(log32 n) with path copying
//   - Delete: O(log32 n) with path copying
//   - Memory: Unchanged subtrees are shared between versions.
package internal

import (
	"fmt"
	"hash/maphash"
	"math/bits"
)

const (
	// Bits per level (32-way branching).
	bitsPerLevel = 5
	// Children per node.
	width = 1 << bitsPerLevel // 32
	// Mask for extracting bits.
	mask = width - 1
)

// HAMT is an immutable hash array mapped trie.
// Zero value is an empty map.
type HAMT[K comparable, V any] struct {
	root   *node[K, V]
	size   int
	hasher maphash.Hash
	seed   maphash.Seed
}

// node is either a branch (children) or leaf (entries).
type node[K comparable, V any] struct {
	// bitmap indicates which children are present (for branch nodes)
	bitmap uint32
	// entries holds key-value pairs (for leaf nodes or collisions)
	entries []entry[K, V]
	// children holds child nodes (for branch nodes)
	children []*node[K, V]
}

type entry[K comparable, V any] struct {
	key   K
	value V
	hash  uint64
}

// sharedSeed is used across all maps to enable structural sharing between maps
// created from the same source.
var sharedSeed = maphash.MakeSeed()

// New creates an empty HAMT.
func New[K comparable, V any]() *HAMT[K, V] {
	return &HAMT[K, V]{
		root:   nil,
		size:   0,
		hasher: maphash.Hash{},
		seed:   sharedSeed,
	}
}

// FromMap creates a HAMT from a Go map.
func FromMap[K comparable, V any](m map[K]V) *HAMT[K, V] {
	result := New[K, V]()
	for k, v := range m {
		result = result.Set(k, v)
	}

	return result
}

// ToMap converts HAMT to a Go map.
func (m *HAMT[K, V]) ToMap() map[K]V {
	result := make(map[K]V, m.size)
	m.Range(func(k K, v V) bool {
		result[k] = v

		return true
	})

	return result
}

// Size returns the number of entries.
func (m *HAMT[K, V]) Size() int {
	if m == nil {
		return 0
	}

	return m.size
}

// IsEmpty returns true if the map has no entries.
func (m *HAMT[K, V]) IsEmpty() bool {
	return m == nil || m.size == 0
}

// Get retrieves a value by key.
func (m *HAMT[K, V]) Get(key K) (V, bool) {
	var zero V
	if m == nil || m.root == nil {
		return zero, false
	}

	hash := m.hashKey(key)

	return m.root.get(key, hash, 0)
}

// Set returns a new map with the key-value pair added or updated.
func (m *HAMT[K, V]) Set(key K, value V) *HAMT[K, V] {
	// Handle nil map
	if m == nil {
		m = New[K, V]()
	}

	hash := m.hashKey(key)

	var newRoot *node[K, V]

	var added bool

	if m.root == nil {
		newRoot = &node[K, V]{
			bitmap:   0,
			entries:  nil,
			children: nil,
		}
		newRoot, added = newRoot.set(key, value, hash, 0)
	} else {
		newRoot, added = m.root.set(key, value, hash, 0)
	}

	newSize := m.size
	if added {
		newSize++
	}

	return &HAMT[K, V]{
		root:   newRoot,
		size:   newSize,
		hasher: maphash.Hash{},
		seed:   m.seed,
	}
}

// Delete returns a new map with the key removed.
func (m *HAMT[K, V]) Delete(key K) *HAMT[K, V] {
	if m == nil || m.root == nil {
		return m
	}

	hash := m.hashKey(key)
	newRoot, deleted := m.root.delete(key, hash, 0)

	if !deleted {
		return m
	}

	return &HAMT[K, V]{
		root:   newRoot,
		size:   m.size - 1,
		hasher: maphash.Hash{},
		seed:   m.seed,
	}
}

// Range iterates over all entries. Stops if fn returns false.
func (m *HAMT[K, V]) Range(fn func(K, V) bool) {
	if m == nil || m.root == nil {
		return
	}

	m.root.iterate(fn)
}

// hashKey computes hash for a key using FNV-1a for speed.
func (m *HAMT[K, V]) hashKey(key K) uint64 {
	// FNV-1a constants
	const (
		offset64 = 14695981039346656037
		prime64  = 1099511628211
	)

	switch k := any(key).(type) {
	case string:
		// Fast FNV-1a for strings
		hashValue := uint64(offset64)
		for i := range len(k) {
			hashValue ^= uint64(k[i])
			hashValue *= prime64
		}

		return hashValue
	case int:
		// Safe conversion: int to uint64 wraps for negative numbers, which is acceptable for hashing
		return uint64(k)*prime64 ^ offset64 //nolint:gosec // hash function intentionally uses wrapping arithmetic
	case int64:
		// Safe conversion: int64 to uint64 wraps for negative numbers, which is acceptable for hashing
		return uint64(k)*prime64 ^ offset64 //nolint:gosec // hash function intentionally uses wrapping arithmetic
	case uint64:
		return k*prime64 ^ offset64
	default:
		// Fallback for other comparable types - use fmt to serialize value
		m.hasher.Reset()
		m.hasher.SetSeed(m.seed)
		_, _ = m.hasher.WriteString(fmt.Sprintf("%T:%v", key, key))

		return m.hasher.Sum64()
	}
}

// node methods

func (n *node[K, V]) get(key K, hash uint64, shift uint) (V, bool) {
	var zero V

	// Check leaf entries first
	for _, e := range n.entries {
		if e.key == key {
			return e.value, true
		}
	}

	// Check children
	if len(n.children) == 0 {
		return zero, false
	}

	idx := (hash >> shift) & mask
	bit := uint32(1) << idx

	if n.bitmap&bit == 0 {
		return zero, false
	}

	// Count bits to find child index
	childIdx := popcount(n.bitmap & (bit - 1))

	return n.children[childIdx].get(key, hash, shift+bitsPerLevel)
}

// maxHashShift is the maximum shift value for hash-based trie traversal.
const maxHashShift = 64

func (n *node[K, V]) set(key K, value V, hash uint64, shift uint) (*node[K, V], bool) {
	// At max depth or collision, store in entries
	if shift >= maxHashShift {
		return n.setEntry(key, value, hash)
	}

	// Check if key exists in this node's entries first (leaf nodes have entries)
	for i, e := range n.entries {
		if e.key == key {
			// Update existing entry - COW entries slice
			newEntries := make([]entry[K, V], len(n.entries))
			copy(newEntries, n.entries)
			newEntries[i] = entry[K, V]{key: key, value: value, hash: hash}

			return &node[K, V]{
				bitmap:   n.bitmap,
				entries:  newEntries,
				children: n.children,
			}, false
		}
	}

	idx := (hash >> shift) & mask
	bit := uint32(1) << idx

	if n.bitmap&bit == 0 {
		// No child at this position, add one
		childIdx := popcount(n.bitmap & (bit - 1))
		child := &node[K, V]{
			bitmap:   0,
			entries:  []entry[K, V]{{key: key, value: value, hash: hash}},
			children: nil,
		}

		return &node[K, V]{
			bitmap:   n.bitmap | bit,
			entries:  n.entries,
			children: insertChild(n.children, childIdx, child),
		}, true
	}

	// Child exists, recurse
	childIdx := popcount(n.bitmap & (bit - 1))
	newChild, added := n.children[childIdx].set(key, value, hash, shift+bitsPerLevel)

	// Only copy children slice, reuse entries
	newChildren := make([]*node[K, V], len(n.children))
	copy(newChildren, n.children)
	newChildren[childIdx] = newChild

	return &node[K, V]{
		bitmap:   n.bitmap,
		entries:  n.entries,
		children: newChildren,
	}, added
}

func (n *node[K, V]) setEntry(key K, value V, hash uint64) (*node[K, V], bool) {
	// Check if key exists
	for i, e := range n.entries {
		if e.key == key {
			// Update existing - COW the entries slice
			newEntries := make([]entry[K, V], len(n.entries))
			copy(newEntries, n.entries)
			newEntries[i] = entry[K, V]{key: key, value: value, hash: hash}

			return &node[K, V]{
				bitmap:   n.bitmap,
				entries:  newEntries,
				children: n.children,
			}, false
		}
	}

	// Add new entry - create new slice with extra capacity
	newEntries := make([]entry[K, V], len(n.entries)+1)
	copy(newEntries, n.entries)
	newEntries[len(n.entries)] = entry[K, V]{key: key, value: value, hash: hash}

	return &node[K, V]{
		bitmap:   n.bitmap,
		entries:  newEntries,
		children: n.children,
	}, true
}

func (n *node[K, V]) delete(key K, hash uint64, shift uint) (*node[K, V], bool) {
	// Check entries
	for i, e := range n.entries {
		if e.key == key {
			// COW entries slice
			newEntries := make([]entry[K, V], len(n.entries)-1)
			copy(newEntries[:i], n.entries[:i])
			copy(newEntries[i:], n.entries[i+1:])

			return &node[K, V]{
				bitmap:   n.bitmap,
				entries:  newEntries,
				children: n.children,
			}, true
		}
	}

	if len(n.children) == 0 {
		return n, false
	}

	idx := (hash >> shift) & mask
	bit := uint32(1) << idx

	if n.bitmap&bit == 0 {
		return n, false
	}

	childIdx := popcount(n.bitmap & (bit - 1))
	newChild, deleted := n.children[childIdx].delete(key, hash, shift+bitsPerLevel)

	if !deleted {
		return n, false
	}

	newNode := n.clone()
	newNode.children = make([]*node[K, V], len(n.children))
	copy(newNode.children, n.children)

	// Remove child if empty
	if newChild.isEmpty() {
		newNode.children = append(newNode.children[:childIdx], newNode.children[childIdx+1:]...)
		newNode.bitmap &^= bit
	} else {
		newNode.children[childIdx] = newChild
	}

	return newNode, true
}

func (n *node[K, V]) clone() *node[K, V] {
	return &node[K, V]{
		bitmap:   n.bitmap,
		entries:  n.entries,  // COW: copy on write when modified
		children: n.children, // COW: copy on write when modified
	}
}

func (n *node[K, V]) isEmpty() bool {
	return len(n.entries) == 0 && len(n.children) == 0
}

func (n *node[K, V]) iterate(fn func(K, V) bool) bool {
	for _, e := range n.entries {
		if !fn(e.key, e.value) {
			return false
		}
	}

	for _, child := range n.children {
		if !child.iterate(fn) {
			return false
		}
	}

	return true
}

// popcount counts set bits using hardware instruction.
func popcount(x uint32) int {
	return bits.OnesCount32(x)
}

// insertChild inserts a child at the given index.
func insertChild[K comparable, V any](children []*node[K, V], idx int, child *node[K, V]) []*node[K, V] {
	result := make([]*node[K, V], len(children)+1)
	copy(result, children[:idx])
	result[idx] = child
	copy(result[idx+1:], children[idx:])

	return result
}
