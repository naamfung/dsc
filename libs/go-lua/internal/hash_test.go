package internal

import "testing"

func TestFnvString(t *testing.T) {
	t.Parallel()

	// Empty string should return offset
	if FnvString("") != FnvOffset64 {
		t.Error("empty string should return FnvOffset64")
	}

	// Same string should produce same hash
	hash1 := FnvString("hello")
	hash2 := FnvString("hello")

	if hash1 != hash2 {
		t.Error("same string should produce same hash")
	}

	// Different strings should produce different hashes
	hash3 := FnvString("world")
	if hash1 == hash3 {
		t.Error("different strings should produce different hashes")
	}
}

func TestFnvStringUniqueness(t *testing.T) {
	t.Parallel()

	strings := []string{
		"", "a", "b", "ab", "ba",
		"hello", "world", "foo", "bar",
		"number", "string", "boolean",
	}

	hashes := make(map[uint64]string)

	for _, str := range strings {
		hashValue := FnvString(str)
		if existing, ok := hashes[hashValue]; ok {
			t.Errorf("hash collision: %q and %q", existing, str)
		}

		hashes[hashValue] = str
	}
}

func TestMixHash(t *testing.T) {
	t.Parallel()

	// Basic mixing
	hashResult := MixHash(1, 2)
	if hashResult == 1 || hashResult == 2 {
		t.Error("MixHash should transform input")
	}

	// Deterministic
	hash1 := MixHash(100, 200)
	hash2 := MixHash(100, 200)

	if hash1 != hash2 {
		t.Error("MixHash should be deterministic")
	}

	// Different inputs produce different outputs
	if MixHash(1, 2) == MixHash(1, 3) {
		t.Error("different inputs should produce different outputs")
	}
}

func TestHashCombine(t *testing.T) {
	t.Parallel()

	// HashCombine is alias for MixHash
	if HashCombine(1, 2) != MixHash(1, 2) {
		t.Error("HashCombine should equal MixHash")
	}
}

func TestHashCombineChaining(t *testing.T) {
	t.Parallel()

	// Chaining multiple values
	hash1 := HashCombine(HashCombine(1, 2), 3)
	hash2 := HashCombine(1, HashCombine(2, 3))

	// Not associative (expected behavior)
	if hash1 == hash2 {
		t.Log("HashCombine is associative (unexpected but not wrong)")
	}
}
