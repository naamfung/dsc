// Package paramhints infers parameter types from call site arguments.
//
// This package analyzes function call sites to infer parameter types for
// functions without explicit type annotations. When a function is called
// with known-type arguments, those types hint at the parameter types.
//
// # Hint Collection
//
// For each call site:
//
//	foo(123, "bar")  -- hints: param1=number, param2=string
//
// The package collects argument types and associates them with parameter
// positions. Multiple call sites contribute hints that are joined.
//
// # Hint Merging
//
// When multiple calls provide conflicting hints:
//
//	foo(1)      -- hints: param1=number
//	foo("a")    -- hints: param1=string
//
// The hints are joined to produce: param1 = number | string
//
// # Integration
//
// Parameter hints feed into function signature inference, providing
// types for parameters that lack explicit annotations.
package paramhints
