// Package index provides per-function indexing for type checking results.
// Designed for LSP integration where incremental updates are common.
//
// The index uses a key-based lookup with file, function, and kind dimensions.
// Entries are invalidated when their file is modified or global version changes.
package index
