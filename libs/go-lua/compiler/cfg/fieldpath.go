package cfg

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow/pathkey"
)

func fieldSegmentsFromNames(fields []string) ([]constraint.Segment, bool) {
	if len(fields) == 0 {
		return nil, false
	}

	segments := make([]constraint.Segment, 0, len(fields))

	for _, field := range fields {
		if field == "" || !pathkey.IsIdentName(field) {
			return nil, false
		}

		segments = append(segments, constraint.Segment{Kind: constraint.SegmentField, Name: field})
	}

	return segments, true
}
