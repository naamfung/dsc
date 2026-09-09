package internal

// FNV-1a constants.
const (
	FnvOffset64 = 14695981039346656037
	FnvPrime64  = 1099511628211
)

// MixHash combines two hashes using FNV-1a style mixing.
func MixHash(h, v uint64) uint64 {
	h ^= v
	h *= FnvPrime64

	return h
}

// HashCombine is an alias for MixHash.
func HashCombine(a, b uint64) uint64 {
	return MixHash(a, b)
}

// FnvString hashes a string using FNV-1a.
func FnvString(s string) uint64 {
	var h uint64 = FnvOffset64
	for i := range len(s) {
		h ^= uint64(s[i])
		h *= FnvPrime64
	}

	return h
}

// Equaler is implemented by types that support typed equality comparison.
// Used by typ.Function to compare Effects, Spec, and Refinement fields
// without importing circular dependencies.
type Equaler interface {
	Equals(other any) bool
}
