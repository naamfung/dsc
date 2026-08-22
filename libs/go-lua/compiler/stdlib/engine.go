package stdlib

import (
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

// EngineConfig returns configuration for the type query engine.
// This enables method resolution on primitive types (e.g., "hello":upper()).
func EngineConfig() core.StdlibConfig {
	return core.StdlibConfig{
		MethodProviders: map[kind.Kind]*typ.Record{
			kind.String: stringMethods,
		},
	}
}
