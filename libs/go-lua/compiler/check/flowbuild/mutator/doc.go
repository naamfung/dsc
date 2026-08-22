// Package mutator extracts container mutation operations from call sites.
//
// This package identifies calls that mutate container types (arrays, maps, channels)
// and produces flow inputs that track element type constraints. It enables the type
// checker to infer container element types from usage patterns.
//
// # Container Mutators
//
// A container mutator is a function that adds elements to a container:
//
//	table.insert(arr, value)  -- adds value to arr
//	channel:send(msg)         -- sends msg through channel
//	map[key] = value          -- assigns value at key
//
// The package detects these patterns by matching call signatures against known
// mutator specs (from type annotations or builtins) and extracting the target
// container and value expressions.
//
// # Flow Integration
//
// Extracted mutations become [flow.ContainerMutatorAssignment] records:
//
//   - Target: The container being mutated (path + symbol)
//   - Value: The element being added (expression + synthesized type)
//   - Point: The CFG location of the mutation
//
// These feed into the flow solver to constrain container element types.
//
// # Table Mutation
//
// Table-specific mutations (field assignments, index assignments) are also
// tracked here to support incremental table type construction where fields
// are assigned after initial table creation.
package mutator
