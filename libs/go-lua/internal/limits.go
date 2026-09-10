package internal

// Depth limits for recursive type operations.
const (
	// MaxShallowDepth for simple type checks (32).
	MaxShallowDepth = 32

	// MaxMediumDepth for type operations (64).
	MaxMediumDepth = 64

	// MaxDeepDepth for environment and control flow (256).
	MaxDeepDepth = 256

	// MaxHashDepth for structural hashing.
	MaxHashDepth = 50

	// MaxDistributionProduct caps the cartesian product size when distributing
	// intersection over unions to prevent exponential blowup.
	MaxDistributionProduct = 256
)
