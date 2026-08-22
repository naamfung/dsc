// Package extract handles expression extraction from CFG for synthesis.
//
// This package walks the CFG to extract expressions that need type
// synthesis, organizing them for efficient processing by the synthesizer.
//
// # Expression Extraction
//
// The package extracts from CFG nodes:
//   - Assignment sources and targets
//   - Call expressions and arguments
//   - Return expressions
//   - Condition expressions
//
// # Synthesis Coordination
//
// Extracted expressions are processed in dependency order:
//   - Subexpressions before containing expressions
//   - Definitions before uses
//   - Independent expressions in parallel where possible
//
// # Table Handling
//
// Table constructor expressions receive special handling:
//   - Field expressions are extracted for individual synthesis
//   - Expected types propagate to fields
//   - Structural typing is applied
//
// # Iterator Support
//
// For-in loop iterators are extracted with their iteration context:
//
//	for k, v in pairs(t) do ... end
//
// The package extracts the iterator call and binds result types to
// loop variables.
//
// # Integration
//
// This package bridges the CFG representation with the expression-level
// type synthesis performed by the synth engine.
package extract
