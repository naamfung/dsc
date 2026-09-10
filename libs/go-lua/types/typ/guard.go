package typ

import "github.com/wippyai/go-lua/internal"

// DefaultRecursionDepth is the canonical recursion depth for typ operations.
const DefaultRecursionDepth = internal.MaxMediumDepth

// DeepRecursionDepth is used for deep structural expansions.
const DeepRecursionDepth = internal.MaxDeepDepth

// NewGuard returns a recursion guard using the canonical default depth.
func NewGuard() internal.RecursionGuard {
	return internal.NewRecursionGuard(DefaultRecursionDepth)
}

// NewDeepGuard returns a recursion guard using the canonical deep depth.
func NewDeepGuard() internal.RecursionGuard {
	return internal.NewRecursionGuard(DeepRecursionDepth)
}

// GuardForDepth returns a recursion guard for a specific depth.
// If maxDepth is non-positive, the default depth is used.
func GuardForDepth(maxDepth int) internal.RecursionGuard {
	if maxDepth <= 0 {
		maxDepth = DefaultRecursionDepth
	}
	return internal.NewRecursionGuard(maxDepth)
}

// DepthExceeded reports whether depth exceeds the default recursion limit.
func DepthExceeded(depth int) bool {
	return depth > DefaultRecursionDepth
}
