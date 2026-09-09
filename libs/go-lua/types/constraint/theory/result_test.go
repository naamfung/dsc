package theory

import "testing"

func TestResultString(t *testing.T) {
	tests := []struct {
		r    Result
		want string
	}{
		{Valid, "valid"},
		{Invalid, "invalid"},
		{Unknown, "unknown"},
		{Result(99), "?"},
	}

	for _, tt := range tests {
		if got := tt.r.String(); got != tt.want {
			t.Errorf("Result(%d).String() = %q, want %q", tt.r, got, tt.want)
		}
	}
}

func TestResultCombine(t *testing.T) {
	tests := []struct {
		a, b Result
		want Result
	}{
		// Invalid dominates
		{Invalid, Valid, Invalid},
		{Valid, Invalid, Invalid},
		{Invalid, Unknown, Invalid},
		{Unknown, Invalid, Invalid},
		{Invalid, Invalid, Invalid},

		// Valid requires both
		{Valid, Valid, Valid},

		// Otherwise Unknown
		{Valid, Unknown, Unknown},
		{Unknown, Valid, Unknown},
		{Unknown, Unknown, Unknown},
	}

	for _, tt := range tests {
		got := tt.a.Combine(tt.b)
		if got != tt.want {
			t.Errorf("%s.Combine(%s) = %s, want %s", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestResultCombineCommutative(t *testing.T) {
	results := []Result{Valid, Invalid, Unknown}
	for _, a := range results {
		for _, b := range results {
			ab := a.Combine(b)
			ba := b.Combine(a)

			if ab != ba {
				t.Errorf("Combine not commutative: %s.Combine(%s)=%s, %s.Combine(%s)=%s",
					a, b, ab, b, a, ba)
			}
		}
	}
}
