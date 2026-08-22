package cfg

// SymbolKind classifies how a symbol was declared.
//
// The declaration kind affects type checking behavior:
//   - Param: Initialized by caller, type from function signature
//   - Local: Initialized at declaration point
//   - Global: Initialized from global environment
//   - Upvalue: Captured from enclosing scope, may have multiple definitions
type SymbolKind int

const (
	// SymbolUnknown indicates the symbol kind is not known.
	SymbolUnknown SymbolKind = iota
	// SymbolParam indicates a function parameter.
	SymbolParam
	// SymbolLocal indicates a local variable.
	SymbolLocal
	// SymbolGlobal indicates a global variable.
	SymbolGlobal
	// SymbolUpvalue indicates an upvalue (captured variable).
	SymbolUpvalue
)

// SSAVersioned provides SSA versioning data for a CFG.
//
// This interface abstracts over SSA construction, allowing the type checker
// to query which version of a variable is visible at each program point
// without knowing the construction details.
//
// The versioning information enables flow-sensitive type checking: different
// assignments can have different types, and the checker uses the version to
// look up the correct type.
type SSAVersioned interface {
	// VisibleVersion returns the SSA version of a symbol visible at a point.
	// Returns a zero Version if the symbol is not defined on all paths to this point.
	VisibleVersion(p Point, sym SymbolID) Version

	// AllVisibleVersions returns all symbol versions visible at a point.
	// The returned map should not be modified by callers.
	AllVisibleVersions(p Point) map[SymbolID]Version

	// PhiNodes returns all phi nodes in the graph.
	// Used to determine type at join points by unioning operand types.
	PhiNodes() []PhiNode
}

// SymbolScopeProvider provides symbol visibility information at CFG points.
//
// This interface enables the type checker to resolve variable names to their
// SymbolIDs based on lexical scoping rules at each program point.
type SymbolScopeProvider interface {
	// SymbolAt returns the SymbolID for a variable name at a specific CFG point.
	// Returns (0, false) if the name is not visible at that point.
	SymbolAt(p Point, name string) (SymbolID, bool)

	// AllSymbolsAt returns all visible symbols at a CFG point.
	// Returns a map of variable names to their SymbolIDs.
	AllSymbolsAt(p Point) map[string]SymbolID

	// DeclarationPoint returns the CFG point where a symbol was declared.
	// Returns (0, false) if the symbol is unknown.
	DeclarationPoint(sym SymbolID) (Point, bool)

	// NameOf returns the variable name for a symbol (for display purposes).
	// Returns empty string if the symbol is unknown.
	NameOf(sym SymbolID) string

	// SymbolKind returns the kind of a symbol (Param, Local, or Global).
	// Returns (SymbolUnknown, false) if the symbol is not known.
	SymbolKind(sym SymbolID) (SymbolKind, bool)
}

// ParamProvider provides function parameter information.
//
// This interface enables the type checker to access function parameter
// metadata for signature checking and type initialization.
type ParamProvider interface {
	// ParamNames returns the function parameter names in order.
	// Returns nil for block graphs or functions with no parameters.
	ParamNames() []string

	// ParamSymbols returns the function parameter symbol IDs in order.
	// Returns nil for block graphs or functions with no parameters.
	ParamSymbols() []SymbolID

	// ParamDeclPoints returns the CFG points where parameters are declared.
	// Returns nil for block graphs or functions with no parameters.
	ParamDeclPoints() []Point
}

// VersionedGraph combines a CFG graph with SSA versioning and symbol scope info.
//
// This is the full interface required by the type checker for flow-sensitive
// analysis. It combines:
//   - Graph: control flow structure for traversal
//   - SSAVersioned: version information for flow-sensitive types
//   - SymbolScopeProvider: name resolution at each point
//   - ParamProvider: function signature information
type VersionedGraph interface {
	Graph
	SSAVersioned
	SymbolScopeProvider
	ParamProvider
}
