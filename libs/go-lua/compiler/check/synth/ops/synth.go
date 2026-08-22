// Package ops provides type synthesis for expressions and function calls.
//
// For function call synthesis, use the two-phase approach:
//
//	infer := ops.InferCall(ctx, def)    // Phase 1: resolve callee, infer type args
//	// ... optionally re-synthesize args using infer.ExpectedArgs ...
//	infer = ops.ReInfer(ctx, def, infer) // Re-infer with updated args
//	result := ops.FinishCall(ctx, def, infer) // Phase 2: check args, compute return
//
// For simple cases, CallWithGenericInference wraps the full flow.
package ops

import (
	"github.com/wippyai/go-lua/types/typ"
)

// FieldDef describes a field in a table constructor.
type FieldDef struct {
	Name     string
	Type     typ.Type
	Optional bool
}

// tableConstructor synthesizes type for table constructor {}.
func tableConstructor(fields []FieldDef, array []typ.Type) typ.Type {
	// Empty table
	if len(fields) == 0 && len(array) == 0 {
		return typ.NewRecord().Build()
	}

	// Pure array
	if len(fields) == 0 {
		return synthesizeArray(array)
	}

	// Record with named fields
	rec := typ.NewRecord()

	for _, f := range fields {
		ft := f.Type
		if ft == nil {
			ft = typ.Unknown
		}
		if f.Optional {
			rec = rec.OptField(f.Name, ft)
		} else {
			rec = rec.Field(f.Name, ft)
		}
	}

	return rec.Build()
}

// synthesizeArray creates array type from elements.
func synthesizeArray(elements []typ.Type) typ.Type {
	if len(elements) == 0 {
		return typ.NewArray(typ.Never)
	}

	// Union of all element types
	elemType := elements[0]
	if elemType == nil {
		elemType = typ.Unknown
	}
	for i := 1; i < len(elements); i++ {
		next := elements[i]
		if next == nil {
			next = typ.Unknown
		}
		elemType = typ.NewUnion(elemType, next)
	}

	return typ.NewArray(elemType)
}
