package typ

import "testing"

func TestAnnotated_String(t *testing.T) {
	tests := []struct {
		name        string
		inner       Type
		annotations []Annotation
		want        string
	}{
		{
			name:        "number with min",
			inner:       Number,
			annotations: []Annotation{{Name: "min", Arg: float64(0)}},
			want:        "number @min(0)",
		},
		{
			name:        "number with min and max",
			inner:       Number,
			annotations: []Annotation{{Name: "min", Arg: float64(0)}, {Name: "max", Arg: float64(100)}},
			want:        "number @min(0) @max(100)",
		},
		{
			name:        "string with pattern",
			inner:       String,
			annotations: []Annotation{{Name: "pattern", Arg: "^.+@.+$"}},
			want:        `string @pattern("^.+@.+$")`,
		},
		{
			name:        "integer annotation no args",
			inner:       Number,
			annotations: []Annotation{{Name: "integer"}},
			want:        "number @integer",
		},
		{
			name:        "int arg",
			inner:       Number,
			annotations: []Annotation{{Name: "min", Arg: int(5)}},
			want:        "number @min(5)",
		},
		{
			name:        "int64 arg",
			inner:       Number,
			annotations: []Annotation{{Name: "min", Arg: int64(10)}},
			want:        "number @min(10)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ann := NewAnnotated(tt.inner, tt.annotations)
			if ann.String() != tt.want {
				t.Errorf("got %q, want %q", ann.String(), tt.want)
			}
		})
	}
}

func TestAnnotated_Kind(t *testing.T) {
	ann := NewAnnotated(Number, []Annotation{{Name: "min", Arg: float64(0)}})
	if ann.Kind() != Number.Kind() {
		t.Errorf("annotated should preserve inner kind")
	}
}

func TestAnnotated_Hash(t *testing.T) {
	ann1 := NewAnnotated(Number, []Annotation{{Name: "min", Arg: float64(0)}})
	ann2 := NewAnnotated(Number, []Annotation{{Name: "min", Arg: float64(0)}})
	ann3 := NewAnnotated(Number, []Annotation{{Name: "max", Arg: float64(100)}})

	if ann1.Hash() != ann2.Hash() {
		t.Error("same annotations should have same hash")
	}
	if ann1.Hash() == ann3.Hash() {
		t.Error("different annotations should have different hash")
	}
}

func TestAnnotated_Equals(t *testing.T) {
	ann1 := NewAnnotated(Number, []Annotation{{Name: "min", Arg: float64(0)}})
	ann2 := NewAnnotated(Number, []Annotation{{Name: "min", Arg: float64(0)}})
	ann3 := NewAnnotated(Number, []Annotation{{Name: "max", Arg: float64(100)}})
	ann4 := NewAnnotated(String, []Annotation{{Name: "min", Arg: float64(0)}})

	if !ann1.Equals(ann2) {
		t.Error("same annotated types should be equal")
	}
	if ann1.Equals(ann3) {
		t.Error("different annotations should not be equal")
	}
	if ann1.Equals(ann4) {
		t.Error("different inner types should not be equal")
	}
	if ann1.Equals(Number) {
		t.Error("annotated should not equal non-annotated")
	}
}

func TestNewAnnotated_EmptyReturnsInner(t *testing.T) {
	result := NewAnnotated(Number, nil)
	if result != Number {
		t.Error("empty annotations should return inner type")
	}

	result = NewAnnotated(Number, []Annotation{})
	if result != Number {
		t.Error("empty annotations slice should return inner type")
	}
}

func TestNewAnnotated_NilInner(t *testing.T) {
	result := NewAnnotated(nil, []Annotation{{Name: "min", Arg: float64(0)}})
	if a, ok := result.(*Annotated); !ok || a.Inner != Unknown {
		t.Error("nil inner should become Unknown")
	}
}

func TestUnwrapAnnotated(t *testing.T) {
	ann := NewAnnotated(Number, []Annotation{{Name: "min", Arg: float64(0)}})
	if UnwrapAnnotated(ann) != Number {
		t.Error("UnwrapAnnotated should return inner type")
	}
	if UnwrapAnnotated(Number) != Number {
		t.Error("UnwrapAnnotated on non-annotated should return same type")
	}
}

func TestGetAnnotations(t *testing.T) {
	annotations := []Annotation{{Name: "min", Arg: float64(0)}}
	ann := NewAnnotated(Number, annotations)

	got := GetAnnotations(ann)
	if len(got) != 1 || got[0].Name != "min" {
		t.Error("GetAnnotations should return annotations")
	}

	if GetAnnotations(Number) != nil {
		t.Error("GetAnnotations on non-annotated should return nil")
	}
}
