// Package union provides union type analysis operations.
//
// These operations extract and merge information from union types,
// commonly used in contextual typing and function signature merging.
package union

import (
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// FieldTypes collects field types from all union members that have the field.
func FieldTypes(expected typ.Type, fieldName string) typ.Type {
	union, ok := unwrap.Alias(expected).(*typ.Union)
	if !ok {
		return nil
	}

	var fieldTypes []typ.Type
	for _, member := range union.Members {
		record := unwrap.Record(member)
		if record == nil {
			continue
		}
		if field := record.GetField(fieldName); field != nil {
			fieldTypes = append(fieldTypes, field.Type)
		}
	}

	if len(fieldTypes) == 0 {
		return nil
	}
	return typ.NewUnion(fieldTypes...)
}

// FunctionTypes extracts function types from a union for contextual typing.
func FunctionTypes(expected typ.Type) []*typ.Function {
	union, ok := unwrap.Alias(expected).(*typ.Union)
	if !ok {
		if fn, ok := unwrap.Alias(expected).(*typ.Function); ok {
			return []*typ.Function{fn}
		}
		return nil
	}

	var fns []*typ.Function
	for _, member := range union.Members {
		if fn, ok := unwrap.Alias(member).(*typ.Function); ok {
			fns = append(fns, fn)
		}
	}
	return fns
}

// MergeFunctions creates a merged function signature from multiple functions.
func MergeFunctions(fns []*typ.Function) *typ.Function {
	if len(fns) == 0 {
		return nil
	}
	if len(fns) == 1 {
		return fns[0]
	}

	maxParams := 0
	for _, fn := range fns {
		if len(fn.Params) > maxParams {
			maxParams = len(fn.Params)
		}
	}

	builder := typ.Func()

	for i := 0; i < maxParams; i++ {
		var paramTypes []typ.Type
		var paramName string
		isOptional := false

		for _, fn := range fns {
			if i < len(fn.Params) {
				paramTypes = append(paramTypes, fn.Params[i].Type)
				if paramName == "" {
					paramName = fn.Params[i].Name
				}
				if fn.Params[i].Optional {
					isOptional = true
				}
			} else {
				isOptional = true
			}
		}

		paramType := typ.NewUnion(paramTypes...)
		if isOptional {
			builder = builder.OptParam(paramName, paramType)
		} else {
			builder = builder.Param(paramName, paramType)
		}
	}

	maxReturns := 0
	for _, fn := range fns {
		if len(fn.Returns) > maxReturns {
			maxReturns = len(fn.Returns)
		}
	}

	if maxReturns > 0 {
		returns := make([]typ.Type, maxReturns)
		for i := 0; i < maxReturns; i++ {
			var returnTypes []typ.Type
			for _, fn := range fns {
				if i < len(fn.Returns) {
					returnTypes = append(returnTypes, fn.Returns[i])
				}
			}
			if len(returnTypes) > 0 {
				returns[i] = typ.NewUnion(returnTypes...)
			}
		}
		builder = builder.Returns(returns...)
	}

	return builder.Build()
}

// DistributeExpected distributes an expected return type over union members.
func DistributeExpected(expected typ.Type) []typ.Type {
	union, ok := unwrap.Alias(expected).(*typ.Union)
	if !ok {
		return []typ.Type{expected}
	}
	return union.Members
}
