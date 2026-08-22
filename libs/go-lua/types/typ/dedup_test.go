package typ

import "testing"

func TestDeduplicateTypes_Empty(t *testing.T) {
	result := deduplicateTypes(nil)
	if result != nil {
		t.Error("deduplicateTypes(nil) should return nil")
	}

	result = deduplicateTypes([]Type{})
	if result != nil {
		t.Error("deduplicateTypes([]) should return nil")
	}
}

func TestDeduplicateTypes_NoDuplicates(t *testing.T) {
	types := []Type{String, Number, Boolean}
	result := deduplicateTypes(types)
	if len(result) != 3 {
		t.Errorf("len = %d, want 3", len(result))
	}
}

func TestDeduplicateTypes_WithDuplicates(t *testing.T) {
	types := []Type{String, Number, String, Number, Boolean}
	result := deduplicateTypes(types)
	if len(result) != 3 {
		t.Errorf("len = %d, want 3", len(result))
	}
}

func TestDeduplicateTypes_AllSame(t *testing.T) {
	types := []Type{String, String, String}
	result := deduplicateTypes(types)
	if len(result) != 1 {
		t.Errorf("len = %d, want 1", len(result))
	}
}
