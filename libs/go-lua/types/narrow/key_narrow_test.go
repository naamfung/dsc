package narrow_test

import (
	"testing"

	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestByTypeKey_Builtin(t *testing.T) {
	base := typ.NewUnion(typ.String, typ.Number, typ.Nil)
	key := narrow.BuiltinTypeKey("number")

	result := narrow.ByTypeKey(base, key, nil)
	if result == nil {
		t.Fatal("expected narrowed type")
	}
	if result.Kind() != typ.Number.Kind() {
		t.Fatalf("expected number, got %v", result)
	}
}

func TestByTypeKey_ZeroKey(t *testing.T) {
	base := typ.String
	key := narrow.TypeKey{}

	result := narrow.ByTypeKey(base, key, nil)
	if result != base {
		t.Fatal("zero key should return original type")
	}
}

func TestExcludeByTypeKey_Builtin(t *testing.T) {
	base := typ.NewUnion(typ.String, typ.Number)
	key := narrow.BuiltinTypeKey("string")

	result := narrow.ExcludeByTypeKey(base, key, nil)
	if result == nil {
		t.Fatal("expected narrowed type")
	}
	if result.Kind() != typ.Number.Kind() {
		t.Fatalf("expected number, got %v", result)
	}
}

func TestByTypeKey_Nil(t *testing.T) {
	key := narrow.BuiltinTypeKey("string")
	result := narrow.ByTypeKey(nil, key, nil)
	if result != nil {
		t.Error("nil input should return nil")
	}
}

func TestByTypeKey_UnknownBuiltin(t *testing.T) {
	base := typ.String
	key := narrow.BuiltinTypeKey("nonexistent")
	result := narrow.ByTypeKey(base, key, nil)
	if result != base {
		t.Error("unknown builtin kind should return original")
	}
}

func TestByTypeKey_HashWithoutResolver(t *testing.T) {
	base := typ.String
	key := narrow.HashTypeKey(12345)
	result := narrow.ByTypeKey(base, key, nil)
	if result != base {
		t.Error("hash key without resolver should return original")
	}
}

func TestByTypeKey_HashWithNilResolution(t *testing.T) {
	base := typ.String
	key := narrow.HashTypeKey(12345)
	resolver := func(narrow.TypeKey) typ.Type { return nil }
	result := narrow.ByTypeKey(base, key, resolver)
	if result != base {
		t.Error("hash key with nil resolution should return original")
	}
}

func TestByTypeKey_HashWithResolver(t *testing.T) {
	base := typ.NewUnion(typ.String, typ.Number)
	key := narrow.HashTypeKey(typ.String.Hash())
	resolver := func(k narrow.TypeKey) typ.Type {
		if k.Hash == typ.String.Hash() {
			return typ.String
		}
		return nil
	}
	result := narrow.ByTypeKey(base, key, resolver)
	if !typ.TypeEquals(result, typ.String) {
		t.Errorf("expected string, got %v", result)
	}
}

func TestExcludeByTypeKey_Nil(t *testing.T) {
	key := narrow.BuiltinTypeKey("string")
	result := narrow.ExcludeByTypeKey(nil, key, nil)
	if result != nil {
		t.Error("nil input should return nil")
	}
}

func TestExcludeByTypeKey_ZeroKey(t *testing.T) {
	base := typ.String
	key := narrow.TypeKey{}
	result := narrow.ExcludeByTypeKey(base, key, nil)
	if result != base {
		t.Error("zero key should return original")
	}
}

func TestExcludeByTypeKey_UnknownBuiltin(t *testing.T) {
	base := typ.String
	key := narrow.BuiltinTypeKey("nonexistent")
	result := narrow.ExcludeByTypeKey(base, key, nil)
	if result != base {
		t.Error("unknown builtin kind should return original")
	}
}

func TestExcludeByTypeKey_BuiltinResultsNever(t *testing.T) {
	base := typ.String
	key := narrow.BuiltinTypeKey("string")
	result := narrow.ExcludeByTypeKey(base, key, nil)
	if result != base {
		t.Error("exclusion resulting in never should return original")
	}
}

func TestExcludeByTypeKey_HashWithoutResolver(t *testing.T) {
	base := typ.String
	key := narrow.HashTypeKey(12345)
	result := narrow.ExcludeByTypeKey(base, key, nil)
	if result != base {
		t.Error("hash key without resolver should return original")
	}
}

func TestExcludeByTypeKey_HashWithNilResolution(t *testing.T) {
	base := typ.String
	key := narrow.HashTypeKey(12345)
	resolver := func(narrow.TypeKey) typ.Type { return nil }
	result := narrow.ExcludeByTypeKey(base, key, resolver)
	if result != base {
		t.Error("hash key with nil resolution should return original")
	}
}

func TestExcludeByTypeKey_HashWithResolver(t *testing.T) {
	base := typ.NewUnion(typ.String, typ.Number)
	key := narrow.HashTypeKey(typ.String.Hash())
	resolver := func(k narrow.TypeKey) typ.Type {
		if k.Hash == typ.String.Hash() {
			return typ.String
		}
		return nil
	}
	result := narrow.ExcludeByTypeKey(base, key, resolver)
	if !typ.TypeEquals(result, typ.Number) {
		t.Errorf("expected number, got %v", result)
	}
}

func TestExcludeByTypeKey_HashResultsNever(t *testing.T) {
	base := typ.String
	key := narrow.HashTypeKey(typ.String.Hash())
	resolver := func(k narrow.TypeKey) typ.Type {
		if k.Hash == typ.String.Hash() {
			return typ.String
		}
		return nil
	}
	result := narrow.ExcludeByTypeKey(base, key, resolver)
	if result != base {
		t.Error("exclusion resulting in never should return original")
	}
}

func TestBuiltinTypeKey_Empty(t *testing.T) {
	key := narrow.BuiltinTypeKey("")
	if !key.IsZero() {
		t.Error("empty name should create zero key")
	}
}

func TestHashTypeKey_Zero(t *testing.T) {
	key := narrow.HashTypeKey(0)
	if !key.IsZero() {
		t.Error("zero hash should create zero key")
	}
}

func TestTypeKey_Hash64(t *testing.T) {
	key1 := narrow.BuiltinTypeKey("string")
	key2 := narrow.BuiltinTypeKey("number")
	key3 := narrow.HashTypeKey(12345)

	if key1.Hash64() == key2.Hash64() {
		t.Error("different builtin keys should have different hashes")
	}
	if key1.Hash64() == key3.Hash64() {
		t.Error("builtin and hash keys should have different hashes")
	}
	if key1.Hash64() == 0 {
		t.Error("builtin key should have non-zero hash")
	}
}

func TestTypeKey_Hash64_Zero(t *testing.T) {
	key := narrow.TypeKey{}
	if key.Hash64() != 0 {
		t.Error("zero key should have zero hash")
	}
}

func TestTypeKey_Equal(t *testing.T) {
	key1 := narrow.BuiltinTypeKey("string")
	key2 := narrow.BuiltinTypeKey("string")
	key3 := narrow.BuiltinTypeKey("number")
	key4 := narrow.HashTypeKey(12345)

	if !key1.Equal(key2) {
		t.Error("same builtin keys should be equal")
	}
	if key1.Equal(key3) {
		t.Error("different builtin keys should not be equal")
	}
	if key1.Equal(key4) {
		t.Error("builtin and hash keys should not be equal")
	}
}
