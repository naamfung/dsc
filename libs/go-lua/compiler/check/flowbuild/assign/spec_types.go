package assign

import (
	"github.com/wippyai/go-lua/compiler/check/api"
)

func mergeSpecTypesInto(out, src api.SpecTypes) api.SpecTypes {
	if len(src) == 0 {
		return out
	}

	if out == nil {
		out = make(api.SpecTypes, len(src))
	}

	for k, v := range src {
		out[k] = v
	}

	return out
}

// MergeSpecTypes merges base and override, with override taking precedence.
// This is used to build per-operation overlay types during extraction.
func MergeSpecTypes(base, override api.SpecTypes) api.SpecTypes {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}

	out := mergeSpecTypesInto(nil, base)
	return mergeSpecTypesInto(out, override)
}
