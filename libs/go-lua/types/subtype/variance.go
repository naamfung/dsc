package subtype

// Variance describes how a type parameter position relates to subtyping.
//
// In generic type contexts, variance determines how type parameters relate
// when the containing type is subtyped:
//
//   - Invariant: The type parameter must be exactly equal. Used for mutable
//     references where both reading and writing occur.
//
//   - Covariant: A subtype can be used where a supertype is expected. Used
//     for read-only/producer positions like function return types.
//
//   - Contravariant: A supertype can be used where a subtype is expected.
//     Used for write-only/consumer positions like function parameters.
//
//   - Bivariant: Either subtype or supertype is acceptable. Rarely used,
//     typically indicates the type parameter is unused or phantom.
type Variance int

const (
	Invariant     Variance = iota // must be exactly equal
	Covariant                     // can use subtype (read/output)
	Contravariant                 // can use supertype (write/input)
	Bivariant                     // can use either (unused parameter)
)

func (v Variance) String() string {
	switch v {
	case Invariant:
		return "invariant"
	case Covariant:
		return "covariant"
	case Contravariant:
		return "contravariant"
	case Bivariant:
		return "bivariant"
	}

	return "unknown"
}

// FlipVariance inverts variance for contravariant positions.
//
// When entering a contravariant position (like a function parameter type),
// the variance of nested positions flips: covariant becomes contravariant
// and vice versa.
func FlipVariance(v Variance) Variance {
	switch v {
	case Covariant:
		return Contravariant
	case Contravariant:
		return Covariant
	default:
		return v
	}
}

// CombineVariance combines two variances when composing type constructors.
//
// For example, a covariant position inside a contravariant position becomes
// contravariant overall. Invariant absorbs all variances, and bivariant is
// absorbed by all.
func CombineVariance(outer, inner Variance) Variance {
	if outer == Invariant || inner == Invariant {
		return Invariant
	}

	if outer == Bivariant || inner == Bivariant {
		return Bivariant
	}

	if outer == inner {
		return Covariant
	}

	return Contravariant
}

// CombineVariancePositions combines variances from different uses of a type parameter.
//
// When a type parameter appears in multiple positions with different variances,
// this function determines the combined variance. Equal variances stay the same,
// bivariant is absorbed by the other, and different non-bivariant variances
// become invariant.
func CombineVariancePositions(v1, v2 Variance) Variance {
	if v1 == v2 {
		return v1
	}

	if v1 == Bivariant {
		return v2
	}

	if v2 == Bivariant {
		return v1
	}

	return Invariant
}
