package flow

import "github.com/wippyai/go-lua/types/cfg"

type predecessorReadOnly interface {
	PredecessorsReadOnly(p cfg.Point) []cfg.Point
}

type successorReadOnly interface {
	SuccessorsReadOnly(p cfg.Point) []cfg.Point
}

func graphPredecessors(g cfg.Graph, p cfg.Point) []cfg.Point {
	if ro, ok := g.(predecessorReadOnly); ok {
		return ro.PredecessorsReadOnly(p)
	}
	return g.Predecessors(p)
}

func graphSuccessors(g cfg.Graph, p cfg.Point) []cfg.Point {
	if ro, ok := g.(successorReadOnly); ok {
		return ro.SuccessorsReadOnly(p)
	}
	return g.Successors(p)
}
