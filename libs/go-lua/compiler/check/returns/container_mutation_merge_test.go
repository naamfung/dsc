package returns

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

func TestMergeContainerMutationSlices_DedupAndSorted(t *testing.T) {
	existing := []api.ContainerMutation{
		{
			Segments:  []constraint.Segment{{Kind: constraint.SegmentField, Name: "b"}},
			ValueType: typ.Number,
		},
	}
	next := []api.ContainerMutation{
		{
			Segments:  []constraint.Segment{{Kind: constraint.SegmentField, Name: "a"}},
			ValueType: typ.String,
		},
		{
			Segments:  []constraint.Segment{{Kind: constraint.SegmentField, Name: "b"}},
			ValueType: typ.Integer,
		},
	}

	got := MergeContainerMutationSlices(existing, next, func(prev *api.ContainerMutation, n api.ContainerMutation) api.ContainerMutation {
		if prev != nil {
			n.ValueType = typ.JoinPreferNonSoft(prev.ValueType, n.ValueType)
		}
		return n
	})

	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if k := api.ContainerMutationKey(got[0]); k != ".a" {
		t.Fatalf("first key = %q, want .a", k)
	}
	if k := api.ContainerMutationKey(got[1]); k != ".b" {
		t.Fatalf("second key = %q, want .b", k)
	}
	if !typ.TypeEquals(got[1].ValueType, typ.Number) {
		t.Fatalf(".b merged type = %v, want number", got[1].ValueType)
	}
}

func TestMergeCapturedContainerMutationMaps_MergeBySymbol(t *testing.T) {
	existing := map[cfg.SymbolID][]api.ContainerMutation{
		1: {
			{
				Segments:  []constraint.Segment{{Kind: constraint.SegmentField, Name: "x"}},
				ValueType: typ.String,
			},
		},
	}
	next := map[cfg.SymbolID][]api.ContainerMutation{
		1: {
			{
				Segments:  []constraint.Segment{{Kind: constraint.SegmentField, Name: "x"}},
				ValueType: typ.String,
			},
		},
		2: {
			{
				Segments:  []constraint.Segment{{Kind: constraint.SegmentField, Name: "y"}},
				ValueType: typ.Boolean,
			},
		},
	}

	got := MergeCapturedContainerMutationMaps(existing, next, nil)
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2 symbols", len(got))
	}
	if len(got[1]) != 1 || len(got[2]) != 1 {
		t.Fatalf("unexpected per-symbol merge sizes: sym1=%d sym2=%d", len(got[1]), len(got[2]))
	}
	if key := api.ContainerMutationKey(got[2][0]); key != ".y" {
		t.Fatalf("sym2 key = %q, want .y", key)
	}
}
