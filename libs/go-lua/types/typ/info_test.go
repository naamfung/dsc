package typ

import "testing"

func TestEffectInfo_InterfaceExists(t *testing.T) {
	var _ EffectInfo = nil
}

func TestSpecInfo_InterfaceExists(t *testing.T) {
	var _ SpecInfo = nil
}

func TestRefinementInfo_InterfaceExists(t *testing.T) {
	var _ RefinementInfo = nil
}
