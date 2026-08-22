// Package pathkey provides canonical path key generation for flow analysis.
//
// In SSA form, each variable has multiple versions at different program points.
// This package maps constraint paths (symbol + field segments) to canonical
// versioned keys that uniquely identify the variable incarnation.
//
// Key format: sym<SymbolID>@<VersionID><segments>
// Example: sym42@3.field[0] refers to symbol 42, version 3, field "field", index 0.
package pathkey

import (
	"strings"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

// VersionedGraph provides SSA version lookup for symbols at CFG points.
//
// In SSA form, each variable has multiple versions corresponding to different
// assignments. VisibleVersion returns the version of a symbol that is "live"
// at a particular program point - the most recent assignment that dominates
// the point.
type VersionedGraph interface {
	// VisibleVersion returns the SSA version of sym that is visible at point p.
	// Returns a zero Version if no version is visible (symbol not in scope).
	VisibleVersion(p cfg.Point, sym cfg.SymbolID) cfg.Version
}

// Resolver provides canonical path key generation from constraint paths.
//
// All path-to-key conversions in the flow solver must go through the resolver
// to ensure consistent SSA versioning. The resolver encapsulates the logic for:
//
//   - Querying the CFG for visible SSA versions
//   - Building canonical key strings in the "sym<ID>@<Ver><segments>" format
//   - Handling placeholder paths (used in function refinements)
//   - Validating that paths have resolvable versions
//
// Using the resolver ensures that the same path at the same point always
// produces the same key, enabling correct value lookup and constraint matching.
type Resolver struct {
	graph  VersionedGraph
	root   map[versionedRootKey]constraint.PathKey
	single map[singleSegmentKey]constraint.PathKey
}

type versionedRootKey struct {
	sym     cfg.SymbolID
	version int
}

type singleSegmentKey struct {
	root versionedRootKey
	seg  constraint.Segment
}

// NewResolver creates a resolver bound to a versioned graph.
//
// The graph must provide SSA version lookup for all symbols that will be
// resolved. Typically this is a cfg.VersionedGraph from CFG construction.
func NewResolver(g VersionedGraph) *Resolver {
	return &Resolver{
		graph:  g,
		root:   make(map[versionedRootKey]constraint.PathKey),
		single: make(map[singleSegmentKey]constraint.PathKey),
	}
}

// KeyAt returns the canonical key for a path at a CFG point.
// This is the ONLY method that should be used for path→key conversion.
//
// Canonical key format: sym<SymbolID>@<VersionID><segments>
// Example: sym42@3.field[0]
//
// Rules:
//   - Symbol=0 paths (placeholders) use Root as-is
//   - Requires valid SSA version for non-placeholder paths
//   - Returns empty string if path is empty or version unavailable
func (r *Resolver) KeyAt(p cfg.Point, path constraint.Path) constraint.PathKey {
	if path.IsEmpty() {
		return ""
	}

	// Placeholders ($0, $1) use Root directly
	if path.IsPlaceholder() {
		return buildPlaceholderKey(path.Root, path.Segments)
	}

	// Non-placeholder paths require Symbol
	if path.Symbol == 0 {
		return ""
	}

	ver := r.versionAt(p, path)
	if ver.IsZero() {
		return ""
	}

	return r.buildKey(path.Symbol, ver.ID, path.Segments)
}

// KeyAtVersion returns the canonical key using an explicit version ID.
// Use when version is already known (e.g., from phi operands).
func (r *Resolver) KeyAtVersion(sym cfg.SymbolID, versionID int, segments []constraint.Segment) constraint.PathKey {
	if sym == 0 {
		return ""
	}
	return r.buildKey(sym, versionID, segments)
}

// buildKey constructs the canonical key string from components.
//
// The format is: sym<SymbolID>@<VersionID><segments>
// Example: sym42@3.field[0]
//
// This format ensures keys are:
//   - Unique per (symbol, version, path suffix) tuple
//   - Parseable back to components via ParseKey
//   - Sortable in a meaningful order (by symbol, then version)
func (r *Resolver) buildKey(sym cfg.SymbolID, versionID int, segments []constraint.Segment) constraint.PathKey {
	rootKey := r.rootKey(sym, versionID)
	if len(segments) == 0 {
		return rootKey
	}
	if len(segments) == 1 {
		cacheKey := singleSegmentKey{
			root: versionedRootKey{sym: sym, version: versionID},
			seg:  segments[0],
		}
		if cached, ok := r.single[cacheKey]; ok {
			return cached
		}
		b := strings.Builder{}
		b.Grow(len(rootKey) + segmentStringLen(segments[0]))
		b.WriteString(string(rootKey))
		appendSegments(&b, segments)
		key := constraint.PathKey(b.String())
		r.single[cacheKey] = key
		return key
	}
	var b strings.Builder
	b.Grow(len(rootKey) + segmentsStringLen(segments))
	b.WriteString(string(rootKey))
	appendSegments(&b, segments)
	return constraint.PathKey(b.String())
}

func (r *Resolver) rootKey(sym cfg.SymbolID, versionID int) constraint.PathKey {
	cacheKey := versionedRootKey{sym: sym, version: versionID}
	if cached, ok := r.root[cacheKey]; ok {
		return cached
	}
	var b strings.Builder
	b.Grow(16)
	b.WriteString("sym")
	writeUint(&b, uint64(sym))
	b.WriteByte('@')
	writeInt(&b, versionID)
	key := constraint.PathKey(b.String())
	r.root[cacheKey] = key
	return key
}

func buildPlaceholderKey(root string, segments []constraint.Segment) constraint.PathKey {
	if len(segments) == 0 {
		return constraint.PathKey(root)
	}
	var b strings.Builder
	b.WriteString(root)
	appendSegments(&b, segments)
	return constraint.PathKey(b.String())
}

func appendSegments(b *strings.Builder, segments []constraint.Segment) {
	for _, seg := range segments {
		switch seg.Kind {
		case constraint.SegmentField:
			b.WriteByte('.')
			b.WriteString(seg.Name)
		case constraint.SegmentIndexString:
			writeQuotedSegmentIndex(b, seg.Name)
		case constraint.SegmentIndexInt:
			b.WriteByte('[')
			writeInt(b, seg.Index)
			b.WriteByte(']')
		}
	}
}

func segmentsStringLen(segments []constraint.Segment) int {
	total := 0
	for _, seg := range segments {
		total += segmentStringLen(seg)
	}
	return total
}

func segmentStringLen(seg constraint.Segment) int {
	switch seg.Kind {
	case constraint.SegmentField:
		return 1 + len(seg.Name)
	case constraint.SegmentIndexString:
		escaped := 0
		for i := 0; i < len(seg.Name); i++ {
			switch seg.Name[i] {
			case '\\', '"':
				escaped++
			}
		}
		return 4 + len(seg.Name) + escaped
	case constraint.SegmentIndexInt:
		n := seg.Index
		if n == 0 {
			return 3
		}
		digits := 0
		if n < 0 {
			digits++
			n = -n
		}
		for n > 0 {
			digits++
			n /= 10
		}
		return digits + 2
	default:
		return 0
	}
}

func writeQuotedSegmentIndex(b *strings.Builder, key string) {
	b.WriteString("[\"")
	for i := 0; i < len(key); i++ {
		switch key[i] {
		case '\\', '"':
			b.WriteByte('\\')
			b.WriteByte(key[i])
		default:
			b.WriteByte(key[i])
		}
	}
	b.WriteString("\"]")
}

func writeUint(b *strings.Builder, value uint64) {
	writeUnsignedDecimal(b, value)
}

func writeInt(b *strings.Builder, value int) {
	if value < 0 {
		b.WriteByte('-')
		writeUnsignedDecimal(b, uint64(-value))
		return
	}
	writeUnsignedDecimal(b, uint64(value))
}

// ParseKey extracts components from a canonical key string.
//
// This is the inverse of buildKey. Given "sym42@3.field", returns:
//   - SymbolID: 42
//   - VersionID: 3
//   - Suffix: ".field"
//   - OK: true
//
// For keys without a version (missing @), versionID is 0.
// For keys that don't start with "sym", returns (0, 0, "", false).
//
// The suffix includes any field or index path after the version number.
func ParseKey(key constraint.PathKey) (cfg.SymbolID, int, string, bool) {
	return parseKey(key, true)
}

// ParseKeyUnchecked extracts key components without suffix grammar validation.
//
// This is intended for trusted internal keys produced by this package. It skips
// ParseSuffix validation to reduce parse overhead in hot paths.
func ParseKeyUnchecked(key constraint.PathKey) (cfg.SymbolID, int, string, bool) {
	return parseKey(key, false)
}

func parseKey(key constraint.PathKey, validateSuffix bool) (cfg.SymbolID, int, string, bool) {
	s := string(key)
	sym, i, ok := parseLeadingSymbol(s)
	if !ok {
		return 0, 0, "", false
	}
	versionID := 0
	suffixStart := i
	if i < len(s) && s[i] == '@' {
		ver, next, ok := parseLeadingVersionAfterAt(s, i+1)
		if !ok {
			return 0, 0, "", false
		}
		versionID = ver
		suffixStart = next
	}

	suffix := s[suffixStart:]
	if suffix != "" {
		switch suffix[0] {
		case '.', '[':
		default:
			return 0, 0, "", false
		}
	}
	if validateSuffix && suffix != "" && ParseSuffix(suffix) == nil {
		return 0, 0, "", false
	}

	return sym, versionID, suffix, true
}

// KeySymbol extracts the symbol ID from a canonical key.
//
// This is a convenience wrapper around ParseKey for when only the symbol
// is needed. Returns 0 if the key is not a valid canonical key.
func KeySymbol(key constraint.PathKey) cfg.SymbolID {
	sym, _, _, ok := ParseKey(key)
	if !ok {
		return 0
	}
	return sym
}

// KeySymbolUnchecked extracts a symbol ID from a key without suffix validation.
//
// This is intended for trusted internal keys. It is cheaper than KeySymbol and
// returns 0 when the key does not begin with a canonical symbol root.
func KeySymbolUnchecked(key constraint.PathKey) cfg.SymbolID {
	s, _, ok := parseLeadingSymbol(string(key))
	if !ok {
		return 0
	}
	return s
}

// KeysShareSymbol returns true if both keys reference the same symbol.
//
// This is used to check if two keys are versions of the same variable,
// regardless of version number or path suffix. Useful for phi node handling
// and constraint propagation across versions.
func KeysShareSymbol(a, b constraint.PathKey) bool {
	symA, _, _, okA := ParseKey(a)
	symB, _, _, okB := ParseKey(b)
	return okA && okB && symA == symB
}

func parseLeadingSymbol(s string) (cfg.SymbolID, int, bool) {
	if len(s) < 4 || s[0] != 's' || s[1] != 'y' || s[2] != 'm' {
		return 0, 0, false
	}
	value, end, ok := parseNonNegativeUintComponent(s, 3)
	if !ok {
		return 0, 0, false
	}
	return cfg.SymbolID(value), end, true
}

func parseLeadingVersionAfterAt(s string, start int) (int, int, bool) {
	value, end, ok := parseNonNegativeUintComponent(s, start)
	if !ok || value > uint64(maxInt) {
		return 0, 0, false
	}
	return int(value), end, true
}

func parseNonNegativeUintComponent(s string, start int) (uint64, int, bool) {
	if start >= len(s) {
		return 0, 0, false
	}
	i := start
	var value uint64
	for i < len(s) {
		ch := s[i]
		switch ch {
		case '@', '.', '[':
			if i == start {
				return 0, 0, false
			}
			return value, i, true
		}
		if ch < '0' || ch > '9' {
			return 0, 0, false
		}
		digit := uint64(ch - '0')
		if value > (maxUint64-digit)/10 {
			return 0, 0, false
		}
		value = value*10 + digit
		i++
	}
	if i == start {
		return 0, 0, false
	}
	return value, i, true
}

func writeUnsignedDecimal(b *strings.Builder, value uint64) {
	if value == 0 {
		b.WriteByte('0')
		return
	}
	var digits [20]byte
	i := len(digits)
	for value > 0 {
		i--
		digits[i] = byte('0' + value%10)
		value /= 10
	}
	_, _ = b.Write(digits[i:])
}

const (
	maxUint64 = ^uint64(0)
	maxInt    = int(^uint(0) >> 1)
)

// versionAt returns the visible SSA version for a path at point p.
//
// Queries the versioned graph to find which version of the path's symbol
// is live at the given point. Returns a zero version if the graph is nil
// or the symbol is not in scope at the point.
func (r *Resolver) versionAt(p cfg.Point, path constraint.Path) cfg.Version {
	if path.Symbol == 0 {
		return cfg.Version{}
	}
	if path.Version != 0 {
		return cfg.Version{Root: path.Root, Symbol: path.Symbol, ID: path.Version}
	}
	if r.graph == nil {
		return cfg.Version{}
	}
	return r.graph.VisibleVersion(p, path.Symbol)
}

// VersionAtSym returns the visible SSA version for a symbol at point p.
//
// This is a convenience method when you have a SymbolID rather than a
// full Path. Equivalent to calling VersionAt with Path{Symbol: sym}.
func (r *Resolver) VersionAtSym(p cfg.Point, sym cfg.SymbolID) cfg.Version {
	if r.graph == nil || sym == 0 {
		return cfg.Version{}
	}
	return r.graph.VisibleVersion(p, sym)
}

// HasVersion returns true if a valid SSA version exists for the path at point p.
//
// A path has a version if:
//   - It is not empty
//   - It is not a placeholder ($0, $1, etc.)
//   - It has a Symbol
//   - The symbol has a non-zero version at point p
//
// This is used to check if a path can be resolved before attempting resolution.
func (r *Resolver) HasVersion(p cfg.Point, path constraint.Path) bool {
	if path.IsEmpty() || path.IsPlaceholder() {
		return false
	}
	if path.Symbol == 0 {
		return false
	}
	return !r.versionAt(p, path).IsZero()
}
