package api

import "github.com/wippyai/go-lua/types/db"

// GraphProviderKey is the typed attachment key for GraphProvider.
var GraphProviderKey = db.NewAttachmentKey[GraphProvider]("check.GraphProvider")

// AttachGraphs attaches a graph provider to the query context for lookup.
func AttachGraphs(ctx *db.QueryContext, graphs GraphProvider) {
	if ctx == nil || graphs == nil {
		return
	}
	db.Attach(ctx, GraphProviderKey, graphs)
}

// GraphsFrom retrieves the graph provider from a db.QueryContext.
func GraphsFrom(ctx *db.QueryContext) GraphProvider {
	graphs, _ := db.Attached(ctx, GraphProviderKey)
	return graphs
}
