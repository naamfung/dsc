package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestTypeFacts_Interface(t *testing.T) {
	var _ TypeFacts = (*Solution)(nil)
}

func TestSolution_DeclaredAt_Nil(t *testing.T) {
	var s *Solution
	tv := s.DeclaredAt(0, 0)
	if tv.Type != typ.Unknown {
		t.Error("nil Solution.DeclaredAt should return Unknown")
	}
	if tv.State != StateUnknown {
		t.Error("nil Solution.DeclaredAt should return StateUnknown")
	}
}

func TestSolution_RefinedAt_Nil(t *testing.T) {
	var s *Solution
	tv := s.RefinedAt(0, 0)
	if tv.Type != nil {
		t.Error("nil Solution.RefinedAt should return nil type")
	}
	if tv.State != StateUnknown {
		t.Error("nil Solution.RefinedAt should return StateUnknown")
	}
}

func TestSolution_EffectiveTypeAt_Nil(t *testing.T) {
	var s *Solution
	tv := s.EffectiveTypeAt(0, 0)
	if tv.Type != typ.Unknown {
		t.Error("nil Solution.EffectiveTypeAt should return Unknown")
	}
}

func TestSolution_IsAnnotated_Nil(t *testing.T) {
	var s *Solution
	if s.IsAnnotated(0) {
		t.Error("nil Solution.IsAnnotated should return false")
	}
}
