package internal

import "testing"

func TestDepthLimitsOrdering(t *testing.T) {
	t.Parallel()

	// Shallow < Medium < Deep
	if MaxShallowDepth >= MaxMediumDepth {
		t.Error("MaxShallowDepth should be less than MaxMediumDepth")
	}

	if MaxMediumDepth >= MaxDeepDepth {
		t.Error("MaxMediumDepth should be less than MaxDeepDepth")
	}
}

func TestDepthLimitsPositive(t *testing.T) {
	t.Parallel()

	limits := []struct {
		name  string
		value int
	}{
		{"MaxShallowDepth", MaxShallowDepth},
		{"MaxMediumDepth", MaxMediumDepth},
		{"MaxDeepDepth", MaxDeepDepth},
		{"MaxHashDepth", MaxHashDepth},
		{"MaxDistributionProduct", MaxDistributionProduct},
	}

	for _, limit := range limits {
		if limit.value <= 0 {
			t.Errorf("%s should be positive, got %d", limit.name, limit.value)
		}
	}
}
