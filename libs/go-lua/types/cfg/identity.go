package cfg

// BoundIdent represents an identifier that has been resolved to a symbol.
//
// A BoundIdent is created when a name reference in the AST is resolved to
// its declaration. It captures both the SymbolID (for lookup) and the Point
// (for flow-sensitive analysis).
//
// By using BoundIdent instead of raw strings, the type system enforces that
// variable identity comes from proper binding resolution, not string comparison.
// Two variables with the same name in different scopes have different BoundIdents.
type BoundIdent struct {
	name   string
	Symbol SymbolID
	Point  Point
}

// NewBoundIdent creates a BoundIdent from a name, symbol, and point.
// Returns zero value if name is empty or symbol is zero.
func NewBoundIdent(name string, symbol SymbolID, point Point) BoundIdent {
	if name == "" || symbol == 0 {
		return BoundIdent{}
	}
	return BoundIdent{name: name, Symbol: symbol, Point: point}
}

// IsValid returns true if this represents a valid binding.
func (b BoundIdent) IsValid() bool {
	return b.name != "" && b.Symbol != 0
}

// Name returns the identifier name for display purposes only.
//
// This method is for error messages and debugging output. For identity
// comparisons, use the Symbol field, which distinguishes between different
// variables that happen to have the same name.
func (b BoundIdent) Name() string {
	return b.name
}
