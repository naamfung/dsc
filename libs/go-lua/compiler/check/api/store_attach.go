package api

import "github.com/wippyai/go-lua/types/db"

// StoreKey is the typed attachment key for StoreView.
var StoreKey = db.NewAttachmentKey[StoreView]("check.StoreView")

// AttachStore attaches a store to the query context for lookup.
func AttachStore(ctx *db.QueryContext, store StoreView) {
	if ctx == nil || store == nil {
		return
	}
	db.Attach(ctx, StoreKey, store)
}

// StoreFrom retrieves the StoreView from a db.QueryContext.
func StoreFrom(ctx *db.QueryContext) StoreView {
	store, _ := db.Attached(ctx, StoreKey)
	return store
}
