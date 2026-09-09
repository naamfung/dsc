package typ

import "testing"

func TestSplitNilableFieldType(t *testing.T) {
	t.Run("optional", func(t *testing.T) {
		inner, optional := SplitNilableFieldType(NewOptional(String))
		if !optional {
			t.Fatal("expected optional")
		}
		if !TypeEquals(inner, String) {
			t.Fatalf("inner = %v, want string", inner)
		}
	})

	t.Run("union with nil", func(t *testing.T) {
		inner, optional := SplitNilableFieldType(NewUnion(String, Boolean, Nil))
		if !optional {
			t.Fatal("expected optional")
		}
		want := NewUnion(String, Boolean)
		if !TypeEquals(inner, want) {
			t.Fatalf("inner = %v, want %v", inner, want)
		}
	})

	t.Run("alias to optional", func(t *testing.T) {
		maybeString := NewAlias("MaybeString", NewOptional(String))
		inner, optional := SplitNilableFieldType(maybeString)
		if !optional {
			t.Fatal("expected optional")
		}
		if !TypeEquals(inner, String) {
			t.Fatalf("inner = %v, want string", inner)
		}
	})

	t.Run("non optional alias preserved", func(t *testing.T) {
		name := NewAlias("Name", String)
		inner, optional := SplitNilableFieldType(name)
		if optional {
			t.Fatal("did not expect optional")
		}
		if inner != name {
			t.Fatalf("inner = %v, want original alias", inner)
		}
	})
}
