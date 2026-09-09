package typ

import "testing"

func TestIsUnknown(t *testing.T) {
	tests := []struct {
		name string
		in   Type
		want bool
	}{
		{name: "nil", in: nil, want: false},
		{name: "unknown", in: Unknown, want: true},
		{name: "any", in: Any, want: false},
		{name: "nil type", in: Nil, want: false},
		{name: "string", in: String, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsUnknown(tt.in); got != tt.want {
				t.Fatalf("IsUnknown(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestIsAny(t *testing.T) {
	tests := []struct {
		name string
		in   Type
		want bool
	}{
		{name: "nil", in: nil, want: false},
		{name: "any", in: Any, want: true},
		{name: "unknown", in: Unknown, want: false},
		{name: "number", in: Number, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsAny(tt.in); got != tt.want {
				t.Fatalf("IsAny(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestIsNever(t *testing.T) {
	tests := []struct {
		name string
		in   Type
		want bool
	}{
		{name: "nil", in: nil, want: false},
		{name: "never", in: Never, want: true},
		{name: "unknown", in: Unknown, want: false},
		{name: "string", in: String, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNever(tt.in); got != tt.want {
				t.Fatalf("IsNever(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestIsAbsentOrUnknown(t *testing.T) {
	tests := []struct {
		name string
		in   Type
		want bool
	}{
		{name: "nil", in: nil, want: true},
		{name: "unknown", in: Unknown, want: true},
		{name: "nil type", in: Nil, want: false},
		{name: "any", in: Any, want: false},
		{name: "number", in: Number, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsAbsentOrUnknown(tt.in); got != tt.want {
				t.Fatalf("IsAbsentOrUnknown(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestHasKnownType(t *testing.T) {
	tests := []struct {
		name string
		in   []Type
		want bool
	}{
		{name: "nil slice", in: nil, want: false},
		{name: "unknown only", in: []Type{Unknown, nil}, want: false},
		{name: "includes nil type", in: []Type{Unknown, Nil}, want: true},
		{name: "includes concrete", in: []Type{Unknown, String}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasKnownType(tt.in); got != tt.want {
				t.Fatalf("HasKnownType(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestIsUnknownOrNil(t *testing.T) {
	tests := []struct {
		name string
		in   Type
		want bool
	}{
		{name: "nil", in: nil, want: true},
		{name: "unknown", in: Unknown, want: true},
		{name: "nil type", in: Nil, want: true},
		{name: "any", in: Any, want: false},
		{name: "number", in: Number, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsUnknownOrNil(tt.in); got != tt.want {
				t.Fatalf("IsUnknownOrNil(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestIsUnknownOnlyOrEmpty(t *testing.T) {
	tests := []struct {
		name string
		in   []Type
		want bool
	}{
		{name: "nil slice", in: nil, want: true},
		{name: "unknown only", in: []Type{Unknown, nil}, want: true},
		{name: "has concrete", in: []Type{Unknown, Number}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsUnknownOnlyOrEmpty(tt.in); got != tt.want {
				t.Fatalf("IsUnknownOnlyOrEmpty(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
