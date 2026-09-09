package typ

// deduplicateTypes removes duplicate types using hash-based bucketing
// with structural equality checks to handle hash collisions.
func deduplicateTypes(types []Type) []Type {
	if len(types) == 0 {
		return nil
	}

	seen := make(map[uint64][]Type)

	for _, t := range types {
		h := t.Hash()
		bucket := seen[h]
		duplicate := false

		for _, existing := range bucket {
			if TypeEquals(existing, t) {
				duplicate = true
				break
			}
		}

		if !duplicate {
			seen[h] = append(bucket, t)
		}
	}

	result := make([]Type, 0, len(types))
	for _, bucket := range seen {
		result = append(result, bucket...)
	}

	return result
}
