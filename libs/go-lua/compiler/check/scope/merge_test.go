package scope

import (
	"testing"
)

func TestMergeScopeExit(t *testing.T) {
	t.Run("nil parent returns nil", func(t *testing.T) {
		child := New()
		result := MergeScopeExit(nil, child)
		if result != nil {
			t.Fatal("expected nil result")
		}
	})

	t.Run("nil child returns parent", func(t *testing.T) {
		parent := New()
		result := MergeScopeExit(parent, nil)
		if result != parent {
			t.Fatal("expected parent to be returned")
		}
	})

	t.Run("both nil returns nil", func(t *testing.T) {
		result := MergeScopeExit(nil, nil)
		if result != nil {
			t.Fatal("expected nil result")
		}
	})

	t.Run("non-local mutations propagate to parent", func(t *testing.T) {
		parent := New()
		child := New().WithMutatedNames([]string{"globalVar"})
		result := MergeScopeExit(parent, child)
		if result == nil {
			t.Fatal("expected non-nil result")
		}
		if !result.IsMutated("globalVar") {
			t.Fatal("expected globalVar to be marked mutated")
		}
	})

	t.Run("local mutations do not propagate", func(t *testing.T) {
		parent := New()
		child := New().WithLocalName("localVar")
		child = child.WithMutatedNames([]string{"localVar"})
		result := MergeScopeExit(parent, child)
		if result.IsMutated("localVar") {
			t.Fatal("expected localVar mutation to not propagate")
		}
	})

	t.Run("empty child returns parent unchanged", func(t *testing.T) {
		parent := New()
		child := New()
		result := MergeScopeExit(parent, child)
		if result != parent {
			t.Fatal("expected parent to be returned unchanged")
		}
	})
}
