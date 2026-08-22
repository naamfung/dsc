package scope

// MergeScopeExit merges child scope metadata back into parent on scope exit.
//
// When a block or function scope exits, mutations performed in the child scope
// to non-local variables must be reflected in the parent scope. This function
// propagates mutation markers for variables that were modified in the child
// but declared in an ancestor scope.
//
// Only mutation metadata is propagated; value types are stored externally in
// flow.DeclaredTypes and merged separately. Local declarations in the child
// are not propagated since they go out of scope.
//
// This is essential for accurate escape analysis and mutation tracking across
// nested function boundaries.
func MergeScopeExit(parent *State, child *State) *State {
	if parent == nil || child == nil {
		return parent
	}

	out := parent

	var mutated []string
	child.RangeMutations(func(name string) bool {
		if !child.IsLocal(name) {
			mutated = append(mutated, name)
		}
		return true
	})
	if len(mutated) > 0 {
		out = out.WithMutatedNames(mutated)
	}

	return out
}
