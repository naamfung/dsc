package assign

import (
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

// canonicalDynamicKeyType normalizes inferred key types for dynamic table access.
//
// Dynamic keys must exclude falsy members (nil/false) and preserve uncertainty
// as unknown rather than forcing a concrete string key type.
func canonicalDynamicKeyType(keyType typ.Type) typ.Type {
	keyType = narrow.ToTruthy(keyType)
	if typ.IsAbsentOrUnknown(keyType) {
		return typ.Unknown
	}
	return subtype.Widen(keyType)
}
