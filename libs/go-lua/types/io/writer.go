package io

import (
	"io"
	"math"
	"sort"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

type typeWriter struct {
	w   io.Writer
	err error

	recursiveIDs    map[*typ.Recursive]uint64
	nextRecursiveID uint64
}

func (w *typeWriter) writeByte(b byte) {
	if w.err != nil {
		return
	}

	_, w.err = w.w.Write([]byte{b})
}

func (w *typeWriter) writeUint32(v uint32) {
	if w.err != nil {
		return
	}

	w.err = binaryWrite(w.w, v)
}

func (w *typeWriter) writeUint64(v uint64) {
	if w.err != nil {
		return
	}

	w.err = binaryWrite64(w.w, v)
}

func (w *typeWriter) writeString(s string) {
	if w.err != nil {
		return
	}

	data := []byte(s)
	w.writeUint32(uint32(len(data)))

	if len(data) > 0 {
		_, w.err = w.w.Write(data)
	}
}

func (w *typeWriter) writeBool(b bool) {
	if b {
		w.writeByte(1)
	} else {
		w.writeByte(0)
	}
}

func (w *typeWriter) writeType(t typ.Type) {
	if w.err != nil {
		return
	}

	if t == nil {
		w.writeByte(kindToByte(kind.Nil))
		return
	}

	if ann, ok := t.(*typ.Annotated); ok {
		w.writeByte(kindToByte(kind.Refined))
		w.writeAnnotated(ann)
		return
	}

	w.writeByte(kindToByte(t.Kind()))

	switch t.Kind() {
	case kind.Nil, kind.Boolean, kind.Number, kind.Integer, kind.String,
		kind.Any, kind.Unknown, kind.Never, kind.Self:
		// Singletons - kind is enough
	case kind.Optional, kind.Union, kind.Intersection, kind.Tuple, kind.Function,
		kind.Array, kind.Map, kind.Record, kind.Alias, kind.Generic, kind.Instantiated,
		kind.Platform, kind.Literal, kind.Ref, kind.Meta, kind.TypeParam, kind.TypeVar,
		kind.Sum, kind.Interface, kind.FieldAccess, kind.IndexAccess, kind.Recursive:
		w.writeTypeData(t)
	default:
		w.err = ErrUnknownType
	}
}

func (w *typeWriter) writeTypeData(t typ.Type) {
	typ.Visit(t, typ.Visitor[struct{}]{
		Optional: func(v *typ.Optional) struct{} {
			w.writeType(v.Inner)
			return struct{}{}
		},
		Union: func(v *typ.Union) struct{} {
			w.writeUint32(uint32(len(v.Members)))
			for _, m := range v.Members {
				w.writeType(m)
			}
			return struct{}{}
		},
		Intersection: func(v *typ.Intersection) struct{} {
			w.writeUint32(uint32(len(v.Members)))
			for _, m := range v.Members {
				w.writeType(m)
			}
			return struct{}{}
		},
		Tuple: func(v *typ.Tuple) struct{} {
			w.writeUint32(uint32(len(v.Elements)))
			for _, e := range v.Elements {
				w.writeType(e)
			}
			return struct{}{}
		},
		Function: func(v *typ.Function) struct{} {
			w.writeUint32(uint32(len(v.TypeParams)))
			for _, tp := range v.TypeParams {
				if tp == nil {
					w.err = ErrInvalidType
					return struct{}{}
				}
				w.writeTypeParam(tp)
			}
			w.writeUint32(uint32(len(v.Params)))
			for _, p := range v.Params {
				w.writeString(p.Name)
				if p.Type == nil {
					w.err = ErrInvalidType
					return struct{}{}
				}
				w.writeType(p.Type)
				w.writeBool(p.Optional)
			}

			w.writeBool(v.Variadic != nil)
			if v.Variadic != nil {
				w.writeType(v.Variadic)
			}

			w.writeUint32(uint32(len(v.Returns)))
			for _, r := range v.Returns {
				if r == nil {
					w.err = ErrInvalidType
					return struct{}{}
				}
				w.writeType(r)
			}

			if eff, ok := v.Effects.(effect.Row); ok {
				w.writeEffectRow(eff)
			} else {
				w.writeEffectRow(effect.Empty)
			}

			if spec, ok := v.Spec.(*contract.Spec); ok && spec != nil {
				w.writeBool(true)
				w.writeContractSpec(spec)
			} else {
				w.writeBool(false)
			}

			if eff, ok := v.Refinement.(*constraint.FunctionRefinement); ok {
				w.writeFunctionRefinement(eff)
			} else {
				w.writeFunctionRefinement(nil)
			}
			return struct{}{}
		},
		Array: func(v *typ.Array) struct{} {
			if v.Element == nil {
				w.err = ErrInvalidType
				return struct{}{}
			}
			w.writeType(v.Element)
			return struct{}{}
		},
		Map: func(v *typ.Map) struct{} {
			if v.Key == nil || v.Value == nil {
				w.err = ErrInvalidType
				return struct{}{}
			}
			w.writeType(v.Key)
			w.writeType(v.Value)
			return struct{}{}
		},
		Record: func(v *typ.Record) struct{} {
			w.writeUint32(uint32(len(v.Fields)))
			for _, f := range v.Fields {
				w.writeString(f.Name)
				if f.Type == nil {
					w.err = ErrInvalidType
					return struct{}{}
				}
				w.writeType(f.Type)
				w.writeBool(f.Optional)
				w.writeBool(f.Readonly)
			}

			w.writeBool(v.Metatable != nil)
			if v.Metatable != nil {
				w.writeType(v.Metatable)
			}
			w.writeBool(v.Open)
			hasMap := v.HasMapComponent()
			w.writeBool(hasMap)
			if hasMap {
				if v.MapKey == nil || v.MapValue == nil {
					w.err = ErrInvalidType
					return struct{}{}
				}
				w.writeType(v.MapKey)
				w.writeType(v.MapValue)
			}
			return struct{}{}
		},
		Generic: func(v *typ.Generic) struct{} {
			w.writeString(v.Name)
			w.writeUint32(uint32(len(v.TypeParams)))
			for _, p := range v.TypeParams {
				w.writeTypeParam(p)
			}
			w.writeBool(v.Body != nil)
			if v.Body != nil {
				w.writeType(v.Body)
			}
			return struct{}{}
		},
		Instantiated: func(v *typ.Instantiated) struct{} {
			w.writeBool(v.Generic != nil)
			if v.Generic != nil {
				w.writeType(v.Generic)
			}
			w.writeUint32(uint32(len(v.TypeArgs)))
			for _, a := range v.TypeArgs {
				w.writeType(a)
			}
			return struct{}{}
		},
		TypeParam: func(v *typ.TypeParam) struct{} {
			w.writeString(v.Name)
			w.writeBool(v.Constraint != nil)
			if v.Constraint != nil {
				w.writeType(v.Constraint)
			}
			return struct{}{}
		},
		TypeVar: func(v *typ.TypeVar) struct{} {
			w.writeUint32(uint32(v.ID))
			return struct{}{}
		},
		Literal: func(v *typ.Literal) struct{} {
			w.writeLiteral(v)
			return struct{}{}
		},
		Ref: func(v *typ.Ref) struct{} {
			w.writeString(v.Module)
			w.writeString(v.Name)
			return struct{}{}
		},
		Alias: func(v *typ.Alias) struct{} {
			w.writeString(v.Name)
			w.writeType(v.Target)
			return struct{}{}
		},
		Meta: func(v *typ.Meta) struct{} {
			w.writeType(v.Of)
			return struct{}{}
		},
		Platform: func(v *typ.Platform) struct{} {
			w.writeString(v.Name)
			return struct{}{}
		},
		FieldAccess: func(v *typ.FieldAccess) struct{} {
			if v.Base == nil {
				w.err = ErrInvalidType
				return struct{}{}
			}
			w.writeType(v.Base)
			w.writeString(v.Field)
			return struct{}{}
		},
		IndexAccess: func(v *typ.IndexAccess) struct{} {
			if v.Base == nil || v.Index == nil {
				w.err = ErrInvalidType
				return struct{}{}
			}
			w.writeType(v.Base)
			w.writeType(v.Index)
			return struct{}{}
		},
		Sum: func(v *typ.Sum) struct{} {
			w.writeString(v.Name)
			w.writeUint32(uint32(len(v.Variants)))
			for _, variant := range v.Variants {
				w.writeString(variant.Tag)
				w.writeUint32(uint32(len(variant.Types)))
				for _, vt := range variant.Types {
					if vt == nil {
						w.err = ErrInvalidType
						return struct{}{}
					}
					w.writeType(vt)
				}
			}
			return struct{}{}
		},
		Interface: func(v *typ.Interface) struct{} {
			w.writeString(v.Name)
			w.writeUint32(uint32(len(v.Methods)))
			for _, method := range v.Methods {
				w.writeString(method.Name)
				if method.Type == nil {
					w.err = ErrInvalidType
					return struct{}{}
				}
				w.writeType(method.Type)
			}
			return struct{}{}
		},
		Recursive: func(v *typ.Recursive) struct{} {
			if v == nil {
				w.err = ErrInvalidType
				return struct{}{}
			}
			id, isNew := w.recursiveID(v)
			w.writeUint64(id)
			w.writeString(v.Name)
			if !isNew {
				w.writeBool(false)
				return struct{}{}
			}
			w.writeBool(true)
			if v.Body == nil {
				w.err = ErrInvalidType
				return struct{}{}
			}
			w.writeType(v.Body)
			return struct{}{}
		},
		Default: func(typ.Type) struct{} {
			w.err = ErrUnknownType
			return struct{}{}
		},
	})
}

func (w *typeWriter) recursiveID(rec *typ.Recursive) (uint64, bool) {
	if w.recursiveIDs == nil {
		w.recursiveIDs = make(map[*typ.Recursive]uint64)
		w.nextRecursiveID = 1
	}
	if id, ok := w.recursiveIDs[rec]; ok {
		return id, false
	}
	id := w.nextRecursiveID
	w.nextRecursiveID++
	w.recursiveIDs[rec] = id
	return id, true
}

func (w *typeWriter) writeAnnotated(ann *typ.Annotated) {
	if ann == nil {
		w.err = ErrInvalidType
		return
	}
	if ann.Inner == nil {
		w.err = ErrInvalidType
		return
	}
	w.writeType(ann.Inner)
	w.writeUint32(uint32(len(ann.Annotations)))
	for _, a := range ann.Annotations {
		w.writeString(a.Name)
		w.writeAnnotationArg(a.Arg)
		if w.err != nil {
			return
		}
	}
}

func (w *typeWriter) writeAnnotationArg(arg any) {
	switch v := arg.(type) {
	case nil:
		w.writeByte(annotationArgNil)
	case string:
		w.writeByte(annotationArgString)
		w.writeString(v)
	case int:
		w.writeByte(annotationArgInt)
		w.writeUint64(uint64(int64(v)))
	case int64:
		w.writeByte(annotationArgInt)
		w.writeUint64(uint64(v))
	case float32:
		w.writeByte(annotationArgFloat)
		w.writeUint64(math.Float64bits(float64(v)))
	case float64:
		w.writeByte(annotationArgFloat)
		w.writeUint64(math.Float64bits(v))
	case bool:
		w.writeByte(annotationArgBool)
		w.writeBool(v)
	default:
		w.err = ErrInvalidType
	}
}

func (w *typeWriter) writeTypeParam(p *typ.TypeParam) {
	w.writeString(p.Name)
	w.writeBool(p.Constraint != nil)

	if p.Constraint != nil {
		w.writeType(p.Constraint)
	}
}

func (w *typeWriter) writeLiteral(lit *typ.Literal) {
	w.writeByte(kindToByte(lit.Base))

	switch lit.Base {
	case kind.Boolean:
		if v, ok := lit.Value.(bool); ok {
			w.writeBool(v)
		}
	case kind.Integer:
		if v, ok := lit.Value.(int64); ok {
			w.writeUint64(uint64(v))
		}
	case kind.Number:
		if v, ok := lit.Value.(float64); ok {
			w.writeUint64(math.Float64bits(v))
		}
	case kind.String:
		if v, ok := lit.Value.(string); ok {
			w.writeString(v)
		}
	case kind.Nil, kind.Any, kind.Unknown, kind.Never, kind.Optional, kind.Union,
		kind.Intersection, kind.Tuple, kind.Function, kind.Array, kind.Map, kind.Record,
		kind.Sum, kind.Interface, kind.Alias, kind.Generic, kind.Instantiated, kind.Platform,
		kind.Literal, kind.Self, kind.Ref, kind.Meta, kind.TypeParam, kind.TypeVar,
		kind.Refined, kind.FieldAccess, kind.IndexAccess, kind.Recursive:
		// Not valid literal base kinds
	}
}

func (w *typeWriter) writeEffectRow(row effect.Row) {
	w.writeUint32(uint32(len(row.Labels)))

	for _, label := range row.Labels {
		w.writeEffectLabel(label)
	}

	w.writeBool(row.Tail != nil)

	if row.Tail != nil {
		w.writeString(row.Tail.Name)
	}
}

func (w *typeWriter) writeEffectLabel(l effect.Label) {
	codec, ok := effect.CodecFor(l)
	if !ok {
		w.writeString("")
		return
	}

	w.writeString(codec.Key())

	if err := codec.Encode(l, &writerAdapter{w}); err != nil && w.err == nil {
		w.err = err
	}
}

type writerAdapter struct{ w *typeWriter }

func (a *writerAdapter) WriteByte(b byte) error {
	a.w.writeByte(b)
	return a.w.err
}

func (a *writerAdapter) WriteInt32(v int32) error {
	a.w.writeUint32(uint32(v))
	return a.w.err
}

func (a *writerAdapter) WriteString(s string) error {
	a.w.writeString(s)
	return a.w.err
}

func (a *writerAdapter) WriteType(t any) error {
	if ty, ok := t.(typ.Type); ok {
		a.w.writeType(ty)
	}

	return a.w.err
}

func (w *typeWriter) writePath(p constraint.Path) {
	w.writeString(p.Root)
	w.writeUint32(uint32(len(p.Segments)))

	for _, seg := range p.Segments {
		w.writeByte(byte(seg.Kind))

		switch seg.Kind {
		case constraint.SegmentField, constraint.SegmentIndexString:
			w.writeString(seg.Name)
		case constraint.SegmentIndexInt:
			w.writeUint32(uint32(int32(seg.Index)))
		}
	}
}

func (w *typeWriter) writeTypeKey(k narrow.TypeKey) {
	w.writeByte(byte(k.Kind))

	switch k.Kind {
	case narrow.TypeKeyBuiltin:
		w.writeString(k.Name)
	case narrow.TypeKeyHash:
		w.writeUint64(k.Hash)
	case narrow.TypeKeyInvalid:
		// Nothing to write
	}
}

func (w *typeWriter) writeCallbackSpec(spec *contract.CallbackSpec) {
	if spec == nil {
		w.writeBool(false)
		return
	}

	w.writeBool(true)
	w.writeUint32(uint32(int32(spec.InputSource.Index)))
	w.writeBool(spec.ReturnsBoolean)
	w.writeUint32(uint32(spec.Cardinality))
	w.writeBool(spec.Pure)

	w.writeUint32(uint32(len(spec.EnvOverlay)))

	for _, name := range sortedKeys(spec.EnvOverlay) {
		w.writeString(name)
		w.writeType(spec.EnvOverlay[name])
	}
}

func (w *typeWriter) writeContractSpec(spec *contract.Spec) {
	if spec == nil {
		w.writeCondition(constraint.TrueCondition())
		w.writeCondition(constraint.TrueCondition())
		w.writeExprCompares(nil)
		w.writeExprCompares(nil)
		w.writeEffectRow(effect.Empty)
		w.writeUint32(0)
		w.writeBool(false)

		return
	}

	w.writeCondition(spec.Requires)
	w.writeCondition(spec.Ensures)
	w.writeExprCompares(spec.ExprRequires)
	w.writeExprCompares(spec.ExprEnsures)
	w.writeEffectRow(spec.Effects)

	keys := make([]int, 0, len(spec.Callbacks))
	for k := range spec.Callbacks {
		keys = append(keys, k)
	}

	sort.Ints(keys)
	w.writeUint32(uint32(len(keys)))

	for _, k := range keys {
		w.writeUint32(uint32(k))
		w.writeCallbackSpec(spec.Callbacks[k])
	}

	w.writeReturnSpec(spec.Return)
}

func (w *typeWriter) writeReturnSpec(rs *contract.ReturnSpec) {
	if rs == nil {
		w.writeBool(false)
		return
	}

	w.writeBool(true)
	w.writeUint32(uint32(len(rs.Cases)))

	for _, c := range rs.Cases {
		w.writeCondition(c.When)
		w.writeType(c.Type)
	}

	w.writeBool(rs.Default != nil)

	if rs.Default != nil {
		w.writeType(rs.Default)
	}
}
