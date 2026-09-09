package cfg

import (
	"strconv"
	"sync/atomic"
)

// SymbolID uniquely identifies a variable declaration across the program.
//
// SymbolID provides declaration-level identity, distinguishing between
// variables that happen to have the same name but are different bindings:
//
//	local x = 1        -- SymbolID 100
//	if cond then
//	    local x = 2    -- SymbolID 101 (different binding)
//	    print(x)       -- refers to SymbolID 101
//	end
//	print(x)           -- refers to SymbolID 100
//
// The combination of SymbolID and Version forms a complete SSA identity,
// where SymbolID identifies which variable and Version identifies which
// definition of that variable.
//
// SymbolID 0 is reserved for unresolved or unknown references.
type SymbolID uint64

var symbolCounter uint64

// NextSymbolID generates a unique symbol ID.
// Thread-safe via atomic operations.
func NextSymbolID() SymbolID {
	return SymbolID(atomic.AddUint64(&symbolCounter, 1))
}

// ReserveSymbolIDs reserves a contiguous block of symbol IDs and returns the
// first ID in the block. Returns 0 when n <= 0.
func ReserveSymbolIDs(n int) SymbolID {
	if n <= 0 {
		return 0
	}
	end := atomic.AddUint64(&symbolCounter, uint64(n))
	start := end - uint64(n) + 1
	return SymbolID(start)
}

// Version represents a stable SSA version of a variable.
//
// In SSA (Static Single Assignment) form, each assignment creates a new version
// of a variable. The Version type captures both the variable identity and which
// assignment created this particular value:
//
//	local x = 1     -- x@1
//	x = x + 1       -- x@2 (new version)
//	if cond then
//	    x = 10      -- x@3 in then-branch
//	else
//	    x = 20      -- x@4 in else-branch
//	end
//	-- x@5 = phi(x@3, x@4) at join point
//
// Version components:
//   - Root: The variable name for display purposes
//   - Symbol: The SymbolID of the declaration (distinguishes shadowed names)
//   - ID: The version number (0 = undefined, 1+ = specific assignment)
type Version struct {
	Root   string   // Variable name (for display and lookup)
	Symbol SymbolID // Declaration identity (distinguishes same-named variables in different scopes)
	ID     int      // Version number within the function (0 = undefined/uninitialized)
}

// IsZero returns true if this is an undefined/uninitialized version.
// Zero versions indicate the variable has not been assigned on this path.
func (v Version) IsZero() bool {
	return v.ID == 0
}

// Key returns a unique string key for this version, suitable for use as a map key.
// Format: "name#symbol@version" or "name@version" if symbol is zero.
// Note: This is an SSA-level key, not a PathKey. For PathKey-based lookups,
// use pathkey.Resolver.KeyAtVersion().
func (v Version) Key() string {
	if v.Symbol != 0 {
		return v.Root + "#" + strconv.FormatUint(uint64(v.Symbol), 10) + "@" + strconv.Itoa(v.ID)
	}
	return v.Root + "@" + strconv.Itoa(v.ID)
}

// String returns a human-readable representation of the version.
func (v Version) String() string {
	return v.Root + "@" + strconv.Itoa(v.ID)
}

// PhiOperand represents one incoming value for a phi node.
//
// At a join point where multiple control flow paths merge, a phi node needs
// to know which version of a variable comes from each predecessor. Each
// PhiOperand pairs a predecessor point with the version visible at that point.
type PhiOperand struct {
	From    Point   // Predecessor point where this version comes from
	Version Version // The SSA version from that predecessor
}

// PhiNode represents a phi function at a control flow join point.
//
// Phi nodes are the mechanism for merging variable versions after control
// flow divergence. When an if/else or loop creates multiple definitions of
// a variable, a phi node at the join point unifies them into a single version.
//
// Example:
//
//	if cond then
//	    x = "hello"  -- x@1
//	else
//	    x = "world"  -- x@2
//	end
//	-- phi node: x@3 = phi(x@1 from then-branch, x@2 from else-branch)
//	print(x)  -- uses x@3
//
// For type checking, the phi node's type is typically the union of all
// operand types.
type PhiNode struct {
	Point    Point        // CFG point where the phi is located
	Target   Version      // The new version created by this phi
	Operands []PhiOperand // Incoming versions from each predecessor
}
