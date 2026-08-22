package effect

// Union combines two effect rows via sequential composition.
//
// When a function f calls function g, the combined effect of f is the union
// of f's own effects with g's effects. This models effect propagation through
// call chains.
//
// Union rules by row shape:
//
//   - Closed + Closed: Returns a closed row with all labels from both.
//     Union({a, b}, {c}) = {a, b, c}
//
//   - Open + Closed (or vice versa): Returns an open row with combined labels
//     and the open row's tail variable.
//     Union({a | e}, {b}) = {a, b | e}
//
//   - Open + Open: Returns an open row with combined labels and a merged tail.
//     If the tails have the same name, that tail is used; otherwise, a new
//     tail with name "e1∪e2" is created.
//     Union({a | e}, {b | e}) = {a, b | e}
//     Union({a | e1}, {b | e2}) = {a, b | e1∪e2}
//
//   - Unknown involved: Returns Unknown (the unknown effect subsumes all).
//     Union({a}, {?}) = {?}
//
// Labels are deduplicated using semantic equality (Equals method), not pointer
// equality, so structurally identical labels appear only once in the result.
func Union(r1, r2 Row) Row {
	// Unknown subsumes everything
	if r1.IsUnknown() || r2.IsUnknown() {
		return Unknown
	}

	// Combine labels using semantic equality (Equals), not string comparison
	labels := append([]Label{}, r1.Labels...)

	for _, l := range r2.Labels {
		if !containsLabelEquals(labels, l) {
			labels = append(labels, l)
		}
	}

	// Handle tail variables with proper unification
	var tail *Var

	switch {
	case r1.Tail != nil && r2.Tail != nil:
		if r1.Tail.Name == r2.Tail.Name {
			// Same tail variable
			tail = r1.Tail
		} else {
			// Different tails: create merged tail preserving both
			tail = &Var{Name: r1.Tail.Name + "∪" + r2.Tail.Name}
		}
	case r1.Tail != nil:
		tail = r1.Tail
	case r2.Tail != nil:
		tail = r2.Tail
	}

	return Row{Labels: labels, Tail: tail}
}

// containsLabelEquals checks if labels contains l using semantic equality.
func containsLabelEquals(labels []Label, l Label) bool {
	for _, existing := range labels {
		if existing.Equals(l) {
			return true
		}
	}

	return false
}

// Intersect returns the intersection of two effect rows.
//
// The result contains only labels present in both input rows. This is useful
// for computing the guaranteed effects when analyzing multiple code paths.
//
// Intersection rules:
//
//   - If either row is pure (empty), returns Empty (no guaranteed effects).
//   - Labels are matched using semantic equality (Equals method).
//   - The tail is kept only if both rows have the same tail variable.
//
// Example:
//
//	Intersect({throw, io}, {throw, diverge}) = {throw}
//	Intersect({throw | e}, {io | e}) = {| e} (labels differ, tail matches)
func Intersect(r1, r2 Row) Row {
	// Empty intersected with anything is empty
	if r1.Pure() || r2.Pure() {
		return Empty
	}

	var labels []Label

	for _, l := range r1.Labels {
		for _, l2 := range r2.Labels {
			if l.Equals(l2) {
				labels = append(labels, l)
				break
			}
		}
	}

	// Tail: keep only if both have same tail
	var tail *Var
	if r1.Tail != nil && r2.Tail != nil && r1.Tail.Name == r2.Tail.Name {
		tail = r1.Tail
	}

	return Row{Labels: labels, Tail: tail}
}

// Subset returns true if r1's effects are a subset of r2's effects.
//
// This is the effect compatibility check: a function with effects r1 can be
// used where effects r2 are expected if Subset(r1, r2) is true.
//
// Subset rules:
//
//   - Empty is a subset of everything (pure functions are universally compatible).
//   - Everything is a subset of Unknown (unknown accepts any effect).
//   - Unknown is only a subset of Unknown itself.
//   - For closed rows: every label in r1 must have a matching label in r2.
//   - For open r2: labels not in r2's explicit set may be covered by its tail.
//
// Example:
//
//	Subset({}, {throw}) = true      // pure can be used where throw is expected
//	Subset({throw}, {throw, io}) = true  // fewer effects is compatible
//	Subset({throw, io}, {throw}) = false // more effects is not compatible
//	Subset({throw}, {?}) = true     // anything is subset of unknown
func Subset(r1, r2 Row) bool {
	// Empty is subset of everything
	if r1.Pure() {
		return true
	}
	// Unknown is subset of unknown only
	if r2.IsUnknown() {
		return true
	}

	if r1.IsUnknown() {
		return false
	}

	// Check all r1 labels are in r2
	for _, l := range r1.Labels {
		found := false

		for _, l2 := range r2.Labels {
			if l.Equals(l2) {
				found = true
				break
			}
		}

		if !found && !r2.IsOpen() {
			return false
		}
	}

	return true
}

// Open creates an open effect row with the specified tail variable.
//
// Open rows are used for effect-polymorphic functions. The tail variable name
// is typically a lowercase letter like "e" by convention.
//
// Example:
//
//	// A higher-order function that propagates callback effects
//	row := effect.Open("e", effect.Throw{})  // {throw | e}
func Open(name string, labels ...Label) Row {
	return Row{Labels: labels, Tail: &Var{Name: name}}
}
