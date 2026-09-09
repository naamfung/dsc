package constraint

import (
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/cfg"
)

// SegmentKind describes how a path accesses a nested value.
type SegmentKind uint8

const (
	SegmentField SegmentKind = iota
	SegmentIndexString
	SegmentIndexInt
)

// Segment identifies a single field or index access step on a path.
// For field access (.name), Kind is SegmentField and Name holds the field name.
// For string index (["key"]), Kind is SegmentIndexString and Name holds the key.
// For integer index ([1]), Kind is SegmentIndexInt and Index holds the value.
type Segment struct {
	Kind  SegmentKind
	Name  string
	Index int
}

// Path identifies a runtime value through its access path (variable.field.index).
//
// Paths are the fundamental identity mechanism for type narrowing. They track
// values across control flow using SSA-style symbol IDs, enabling precise
// narrowing even when the same variable name refers to different bindings.
//
// Symbol provides SSA identity; Root is optional for display when Symbol is set.
// When Symbol is non-zero, it is the primary identity for the path root.
// Placeholder paths ($0, $1, etc.) use Root only with Symbol=0 and are
// substituted with concrete paths when applying function refinements at call sites.
//
// Examples:
//   - {Root: "x", Symbol: 5}: Variable x with symbol ID 5
//   - {Root: "x", Symbol: 5, Segments: [{Field, "name", 0}]}: x.name
//   - {Root: "$0", Symbol: 0}: Placeholder for first function parameter
type Path struct {
	Root     string       // Variable name (optional for symbol paths, required for placeholders)
	Symbol   cfg.SymbolID // SymbolID for identity (0 if unresolved/placeholder)
	Segments []Segment
	Version  int // SSA version ID (0 = unspecified, non-zero binds path to a specific version)
}

// PathKey is a stable string key for map usage.
type PathKey string

// NewPath creates a path with the given symbol and display name.
// This is the primary constructor for resolved paths.
//
// Example:
//
//	path := NewPath(sym, "x")           // x
//	path := NewPath(sym, "x").Field("y") // x.y
func NewPath(sym cfg.SymbolID, name string) Path {
	return Path{Root: name, Symbol: sym}
}

// NewPlaceholder creates a placeholder path for function refinement parameters.
// Index 0 creates $0, index 1 creates $1, etc.
//
// Example:
//
//	p0 := NewPlaceholder(0) // $0 (first parameter)
//	p1 := NewPlaceholder(1) // $1 (second parameter)
func NewPlaceholder(index int) Path {
	if index < 0 {
		return Path{}
	}
	return Path{Root: "$" + strconv.Itoa(index)}
}

// Field returns a new path with a field access segment appended.
//
// Example:
//
//	path.Field("name")  // path.name
//	path.Field("a").Field("b") // path.a.b
func (p Path) Field(name string) Path {
	return p.Append(Segment{Kind: SegmentField, Name: name})
}

// IndexStr returns a new path with a string index segment appended.
//
// Example:
//
//	path.IndexStr("key")  // path["key"]
func (p Path) IndexStr(key string) Path {
	return p.Append(Segment{Kind: SegmentIndexString, Name: key})
}

// IndexInt returns a new path with an integer index segment appended.
//
// Example:
//
//	path.IndexInt(0)  // path[0]
//	path.IndexInt(1)  // path[1]
func (p Path) IndexInt(index int) Path {
	return p.Append(Segment{Kind: SegmentIndexInt, Index: index})
}

// Parent returns the path without its last segment.
// Returns an empty path if there are no segments.
//
// Example:
//
//	path.Field("a").Field("b").Parent() // path.a
func (p Path) Parent() Path {
	if len(p.Segments) == 0 {
		return Path{}
	}
	// Copy segments to avoid slice aliasing
	parentSegs := make([]Segment, len(p.Segments)-1)
	copy(parentSegs, p.Segments[:len(p.Segments)-1])
	return Path{
		Root:     p.Root,
		Symbol:   p.Symbol,
		Version:  p.Version,
		Segments: parentSegs,
	}
}

// LastSegment returns the final segment of the path, if any.
// Returns (segment, true) if the path has segments, (zero, false) otherwise.
func (p Path) LastSegment() (Segment, bool) {
	if len(p.Segments) == 0 {
		return Segment{}, false
	}
	return p.Segments[len(p.Segments)-1], true
}

// IsFieldAccess returns true if the last segment is a field access.
func (p Path) IsFieldAccess() bool {
	if len(p.Segments) == 0 {
		return false
	}
	return p.Segments[len(p.Segments)-1].Kind == SegmentField
}

// FieldName returns the field name if the last segment is a field access.
// Returns empty string if not a field access.
func (p Path) FieldName() string {
	if seg, ok := p.LastSegment(); ok && seg.Kind == SegmentField {
		return seg.Name
	}
	return ""
}

// FormatSegments converts path segments to a canonical suffix string.
// This is the single canonical implementation for segment serialization.
// Format: .field for SegmentField, ["key"] for SegmentIndexString, [123] for SegmentIndexInt.
func FormatSegments(segs []Segment) string {
	if len(segs) == 0 {
		return ""
	}
	var b strings.Builder
	for _, seg := range segs {
		switch seg.Kind {
		case SegmentField:
			b.WriteByte('.')
			b.WriteString(seg.Name)
		case SegmentIndexString:
			writeQuotedPathIndex(&b, seg.Name)
		case SegmentIndexInt:
			b.WriteByte('[')
			b.WriteString(strconv.Itoa(seg.Index))
			b.WriteByte(']')
		}
	}
	return b.String()
}

func writeQuotedPathIndex(b *strings.Builder, key string) {
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

// IsEmpty returns true if the path has no identity (no Root and no Symbol).
func (p Path) IsEmpty() bool { return p.Root == "" && p.Symbol == 0 }

// HasSymbol returns true if this path has a resolved SymbolID.
func (p Path) HasSymbol() bool { return p.Symbol != 0 }

// DisplayRoot returns the display name for the path root.
// For symbol-rooted paths, uses the provided nameResolver to get the name.
// For placeholder paths (Symbol=0), returns the Root field directly.
func (p Path) DisplayRoot(nameResolver func(cfg.SymbolID) string) string {
	if p.Symbol != 0 && nameResolver != nil {
		if name := nameResolver(p.Symbol); name != "" {
			return name
		}
	}
	return p.Root
}

// Equal returns true if two paths have the same identity.
// Symbol-based identity takes precedence when available.
func (p Path) Equal(other Path) bool {
	// Symbol-based identity is primary when available
	if p.Symbol != 0 || other.Symbol != 0 {
		// If either has a symbol, both must have the same symbol
		if p.Symbol != other.Symbol {
			return false
		}
		if p.Version != other.Version {
			return false
		}
	} else {
		// Neither has symbol (placeholder paths) - use Root for identity
		if p.Root != other.Root {
			return false
		}
	}

	if len(p.Segments) != len(other.Segments) {
		return false
	}

	for i := range p.Segments {
		a := p.Segments[i]
		b := other.Segments[i]

		if a.Kind != b.Kind || a.Name != b.Name || a.Index != b.Index {
			return false
		}
	}

	return true
}

// Append returns a new path with the given segment appended.
// Returns an empty path if the receiver is empty.
func (p Path) Append(seg Segment) Path {
	if p.IsEmpty() {
		return Path{}
	}

	next := Path{Root: p.Root, Symbol: p.Symbol, Version: p.Version}
	if len(p.Segments) > 0 {
		next.Segments = append(next.Segments, p.Segments...)
	}

	next.Segments = append(next.Segments, seg)

	return next
}

// String returns a human-readable representation of the path.
// Format: root.field[index] where root is either the variable name or $symN for symbol-only paths.
func (p Path) String() string {
	if p.Root == "" && p.Symbol == 0 {
		return ""
	}

	var b strings.Builder

	if p.Root != "" {
		b.WriteString(p.Root)
	} else {
		// Symbol-only path: use symbolic display
		b.WriteString("$sym")
		b.WriteString(strconv.FormatUint(uint64(p.Symbol), 10))
	}

	for _, seg := range p.Segments {
		switch seg.Kind {
		case SegmentField:
			b.WriteByte('.')
			b.WriteString(seg.Name)
		case SegmentIndexString:
			b.WriteByte('[')
			b.WriteString(seg.Name)
			b.WriteByte(']')
		case SegmentIndexInt:
			b.WriteByte('[')
			b.WriteString(strconv.Itoa(seg.Index))
			b.WriteByte(']')
		}
	}

	return b.String()
}

// Key returns a stable key representation of the path.
// Format: sym<SymbolID>@<VersionID><segments> for versioned symbol paths,
// sym<SymbolID><segments> for unversioned symbol paths, <Root><segments> for placeholders.
// For versioned flow lookups, prefer pathkey.Resolver.KeyAt.
func (p Path) Key() PathKey {
	if p.IsEmpty() {
		return ""
	}
	var b strings.Builder
	if p.Symbol != 0 {
		b.WriteString("sym")
		b.WriteString(strconv.FormatUint(uint64(p.Symbol), 10))
		if p.Version != 0 {
			b.WriteByte('@')
			b.WriteString(strconv.Itoa(p.Version))
		}
	} else {
		b.WriteString(p.Root)
	}
	b.WriteString(FormatSegments(p.Segments))
	return PathKey(b.String())
}

// Hash returns a 64-bit hash of the path for use in hash-based collections.
// Symbol-based identity is used when available, otherwise Root is hashed.
func (p Path) Hash() uint64 {
	if p.Root == "" && p.Symbol == 0 {
		return 0
	}

	var h uint64
	if p.Symbol != 0 {
		// Use Symbol as primary identity for hashing
		h = internal.HashCombine(0, uint64(p.Symbol))
		h = internal.HashCombine(h, uint64(p.Version))
	} else {
		h = internal.FnvString(p.Root)
	}

	for _, seg := range p.Segments {
		h = internal.HashCombine(h, uint64(seg.Kind))

		switch seg.Kind {
		case SegmentField, SegmentIndexString:
			h = internal.HashCombine(h, internal.FnvString(seg.Name))
		case SegmentIndexInt:
			h = internal.HashCombine(h, uint64(seg.Index))
		}
	}

	return h
}

// Less provides a stable ordering for canonicalization.
// Compares by Symbol first when both are set, otherwise by Root.
func (p Path) Less(other Path) bool {
	// Compare by Symbol when both have it
	if p.Symbol != 0 && other.Symbol != 0 {
		if p.Symbol != other.Symbol {
			return p.Symbol < other.Symbol
		}
		if p.Version != other.Version {
			return p.Version < other.Version
		}
	} else if p.Root != other.Root {
		return p.Root < other.Root
	}

	if len(p.Segments) != len(other.Segments) {
		return len(p.Segments) < len(other.Segments)
	}

	for i := range p.Segments {
		a := p.Segments[i]
		b := other.Segments[i]

		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}

		if a.Name != b.Name {
			return a.Name < b.Name
		}

		if a.Index != b.Index {
			return a.Index < b.Index
		}
	}

	return false
}

// IsPlaceholder returns true if this path is a placeholder (used in function refinements).
// Placeholders have Symbol == 0 and Root matching $0, $1, etc.
func (p Path) IsPlaceholder() bool {
	return p.Symbol == 0 && p.PlaceholderIndex() >= 0
}

// ValidateSymbol checks the symbol-first identity invariant.
// For resolved paths (non-empty, non-placeholder), Symbol must be non-zero.
// Returns empty string if valid, error message if invalid.
// Used for debug-mode assertions.
func (p Path) ValidateSymbol() string {
	if p.IsEmpty() {
		return "" // empty paths are valid
	}
	if p.IsPlaceholder() {
		return "" // placeholders are valid with Symbol=0
	}
	if p.Symbol == 0 && p.Root != "" {
		return "resolved path missing Symbol: " + p.Root
	}
	return ""
}

// PlaceholderIndex returns the parameter index if this path's root is a placeholder ($0, $1, etc).
// Returns -1 if not a placeholder.
func (p Path) PlaceholderIndex() int {
	return PlaceholderIndexFromString(p.Root)
}

// Substitute replaces placeholder roots with actual argument paths.
// Only paths with Symbol == 0 and Root matching $0, $1, etc. are substituted.
// Returns (result, true) on success, (empty, false) if placeholder out of range or arg path empty.
func (p Path) Substitute(args []Path) (Path, bool) {
	if p.IsEmpty() {
		return Path{}, false
	}

	if !p.IsPlaceholder() {
		return p, true
	}

	idx := p.PlaceholderIndex()

	if idx >= len(args) {
		return Path{}, false
	}

	argPath := args[idx]
	if argPath.IsEmpty() {
		return Path{}, false
	}

	result := Path{Root: argPath.Root, Symbol: argPath.Symbol, Version: argPath.Version}
	result.Segments = append(result.Segments, argPath.Segments...)
	result.Segments = append(result.Segments, p.Segments...)

	return result, true
}
