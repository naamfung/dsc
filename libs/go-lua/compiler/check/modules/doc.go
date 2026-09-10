// Package modules handles module export type computation.
//
// This package computes the exported type of a Lua module by analyzing
// its return statements and the types of exported values.
//
// # Export Detection
//
// Lua modules export values via return statements:
//
//	local M = {}
//	function M.foo() ... end
//	return M
//
// The package traces the returned value to compute its type, including
// all fields and methods.
//
// # Type Enrichment
//
// Exported types are enriched with:
//   - Function refinement annotations (terminates, type guards)
//   - Refined field types from flow analysis
//   - Method signatures from implementation analysis
//
// # Integration
//
// Module export types are stored in the type database for use by
// importers, enabling cross-module type checking.
package modules
