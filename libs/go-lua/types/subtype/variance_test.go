package subtype

// Tests for variance utilities - covariance, contravariance, invariance, bivariance.

import "testing"

func TestVariance_String(t *testing.T) {
	tests := []struct {
		v    Variance
		want string
	}{
		{Invariant, "invariant"},
		{Covariant, "covariant"},
		{Contravariant, "contravariant"},
		{Bivariant, "bivariant"},
		{Variance(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.v.String(); got != tt.want {
			t.Errorf("%d.String() = %s, want %s", tt.v, got, tt.want)
		}
	}
}

func TestFlipVariance(t *testing.T) {
	tests := []struct {
		v    Variance
		want Variance
	}{
		{Covariant, Contravariant},
		{Contravariant, Covariant},
		{Invariant, Invariant},
		{Bivariant, Bivariant},
	}
	for _, tt := range tests {
		if got := FlipVariance(tt.v); got != tt.want {
			t.Errorf("FlipVariance(%s) = %s, want %s", tt.v, got, tt.want)
		}
	}
}

func TestFlipVariance_Idempotent(t *testing.T) {
	for _, v := range []Variance{Covariant, Contravariant} {
		if FlipVariance(FlipVariance(v)) != v {
			t.Errorf("FlipVariance(FlipVariance(%s)) != %s", v, v)
		}
	}
}

func TestCombineVariance(t *testing.T) {
	tests := []struct {
		outer, inner Variance
		want         Variance
	}{
		// Invariant dominates
		{Invariant, Covariant, Invariant},
		{Invariant, Contravariant, Invariant},
		{Invariant, Bivariant, Invariant},
		{Covariant, Invariant, Invariant},
		{Contravariant, Invariant, Invariant},
		{Invariant, Invariant, Invariant},

		// Bivariant propagates
		{Bivariant, Covariant, Bivariant},
		{Bivariant, Contravariant, Bivariant},
		{Covariant, Bivariant, Bivariant},
		{Contravariant, Bivariant, Bivariant},
		{Bivariant, Bivariant, Bivariant},

		// Same -> Covariant
		{Covariant, Covariant, Covariant},
		{Contravariant, Contravariant, Covariant},

		// Different -> Contravariant
		{Covariant, Contravariant, Contravariant},
		{Contravariant, Covariant, Contravariant},
	}
	for _, tt := range tests {
		got := CombineVariance(tt.outer, tt.inner)
		if got != tt.want {
			t.Errorf("CombineVariance(%s, %s) = %s, want %s", tt.outer, tt.inner, got, tt.want)
		}
	}
}

func TestCombineVariancePositions(t *testing.T) {
	tests := []struct {
		v1, v2 Variance
		want   Variance
	}{
		// Same -> Same
		{Covariant, Covariant, Covariant},
		{Contravariant, Contravariant, Contravariant},
		{Invariant, Invariant, Invariant},
		{Bivariant, Bivariant, Bivariant},

		// Bivariant yields to other
		{Bivariant, Covariant, Covariant},
		{Bivariant, Contravariant, Contravariant},
		{Bivariant, Invariant, Invariant},
		{Covariant, Bivariant, Covariant},
		{Contravariant, Bivariant, Contravariant},
		{Invariant, Bivariant, Invariant},

		// Different (non-bivariant) -> Invariant
		{Covariant, Contravariant, Invariant},
		{Contravariant, Covariant, Invariant},
		{Covariant, Invariant, Invariant},
		{Contravariant, Invariant, Invariant},
	}
	for _, tt := range tests {
		got := CombineVariancePositions(tt.v1, tt.v2)
		if got != tt.want {
			t.Errorf("CombineVariancePositions(%s, %s) = %s, want %s", tt.v1, tt.v2, got, tt.want)
		}
	}
}

func TestCombineVariancePositions_Commutative(t *testing.T) {
	variances := []Variance{Invariant, Covariant, Contravariant, Bivariant}
	for _, v1 := range variances {
		for _, v2 := range variances {
			r1 := CombineVariancePositions(v1, v2)
			r2 := CombineVariancePositions(v2, v1)

			if r1 != r2 {
				t.Errorf("CombineVariancePositions not commutative: (%s, %s) = %s but (%s, %s) = %s",
					v1, v2, r1, v2, v1, r2)
			}
		}
	}
}
