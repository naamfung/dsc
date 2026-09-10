package db

// AttachmentKey is a type-safe key for context attachments.
//
// Attachments allow passing additional state through the query context without
// modifying the QueryContext type. Each AttachmentKey[T] creates a unique slot
// identified by its name string.
//
// Usage pattern:
//
//	// Define a key at package level
//	var MyDataKey = db.NewAttachmentKey[*MyData]("mypackage.MyData")
//
//	// Attach data to context
//	db.Attach(ctx, MyDataKey, myData)
//
//	// Retrieve data from context
//	if data, ok := db.Attached(ctx, MyDataKey); ok {
//	    // use data
//	}
//
// The generic type T ensures type safety at compile time, while the name
// string enables runtime lookup in the attachments map.
type AttachmentKey[T any] struct {
	name string
}

// NewAttachmentKey creates a new typed attachment key.
// The name is used for debugging and should be unique per attachment type.
func NewAttachmentKey[T any](name string) AttachmentKey[T] {
	return AttachmentKey[T]{name: name}
}

// Name returns the key's name for debugging.
func (k AttachmentKey[T]) Name() string {
	return k.name
}

// Attach stores a value in the context using a typed key.
//
// The value is stored in the context's attachment map, keyed by the
// AttachmentKey's name. Calling Attach with the same key replaces any
// previously attached value.
func Attach[T any](ctx *QueryContext, key AttachmentKey[T], val T) {
	if ctx == nil {
		return
	}
	if ctx.attachments == nil {
		ctx.attachments = make(map[string]any)
	}
	ctx.attachments[key.name] = val
}

// Attached retrieves a value from the context using a typed key.
//
// Returns the attached value and true if found, or the zero value and false
// if not found or if the value's type doesn't match T.
func Attached[T any](ctx *QueryContext, key AttachmentKey[T]) (T, bool) {
	var zero T
	if ctx == nil || ctx.attachments == nil {
		return zero, false
	}
	v, ok := ctx.attachments[key.name]
	if !ok {
		return zero, false
	}
	typed, ok := v.(T)
	if !ok {
		return zero, false
	}
	return typed, true
}
