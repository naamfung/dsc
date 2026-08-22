package cond

import "github.com/wippyai/go-lua/compiler/cfg"

// EdgeKey identifies a CFG edge by source and target point.
type EdgeKey struct {
	From, To cfg.Point
}
