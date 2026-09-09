// Package keyscoll collects table keys from expressions for type inference.
//
// This package extracts key information from table constructors and index
// expressions to support record type inference and map key type analysis.
//
// # Key Collection
//
// For table constructors:
//
//	{ name = "foo", count = 1 }  -- keys: ["name", "count"]
//	{ [1] = "a", [2] = "b" }     -- integer keys
//
// The package identifies whether a table has string keys (record-like),
// integer keys (array-like), or mixed keys.
//
// # Usage
//
// Key collection supports:
//   - Distinguishing arrays from records
//   - Validating field access against known keys
//   - Inferring map key types from index patterns
package keyscoll
