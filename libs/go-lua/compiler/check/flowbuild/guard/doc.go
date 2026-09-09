// Package guard extracts type guard conditions from control flow branches.
//
// This package analyzes conditional expressions to identify type guards that
// narrow variable types along true/false branches. Guards are the foundation
// of flow-sensitive type narrowing.
//
// # Guard Extraction
//
// Guards are extracted from conditions like:
//
//	if x ~= nil then      -- x is non-nil in true branch
//	if type(x) == "string" then  -- x is string in true branch
//	if x:is(SomeClass) then      -- x is SomeClass in true branch
//
// The package produces [flow.Guard] records containing:
//   - Path: The variable or field being guarded
//   - Predicate: The type constraint (excludes nil, includes string, etc.)
//   - Branch: Which branch (true/false) the guard applies to
//
// # Negation Handling
//
// Guards automatically handle negation and branch inversion:
//
//	if not (x == nil) then  -- equivalent to x ~= nil
//	if x == nil then return end
//	-- x is non-nil after the if
//
// # Integration
//
// Guards feed into the flow solver which propagates narrowed types through
// the CFG, enabling precise type tracking at each program point.
package guard
