// Effect label codecs for serialization and deserialization.
//
// This file registers all built-in effect label codecs with the effect system.
// Each codec handles encoding and decoding of a specific effect label type
// to/from the manifest format.
//
// Codecs are registered at package initialization via [Register] and are
// looked up by key string during manifest reading. Each codec implements:
//   - Key(): Returns the unique string identifier for this effect type
//   - Encode(): Serializes the label to the manifest writer
//   - Decode(): Deserializes the label from the manifest reader
//
// The registered effect types cover:
//   - Control flow: Throw, Diverge
//   - I/O: IO, ModuleLoad, Send
//   - Mutation: Mutate, TableMutator, LengthChange, Store, Freeze
//   - Borrowing: Borrow, BorrowAll
//   - Returns: Return, ErrorReturn, ReturnLength, CorrelatedReturn
//   - Type system: TypePredicate, TypeValueMethod, CallableType
//   - Data flow: PassThrough, FlowInto, VariadicTransform
//   - Iteration: Iterator
package effect

import (
	"fmt"

	"github.com/wippyai/go-lua/types/constraint"
)

func init() {
	Register(throwCodec{})
	Register(ioCodec{})
	Register(divergeCodec{})
	Register(mutateCodec{})
	Register(returnCodec{})
	Register(errorReturnCodec{})
	Register(returnLengthCodec{})
	Register(iteratorCodec{})
	Register(tableMutatorCodec{})
	Register(lengthChangeCodec{})
	Register(borrowCodec{})
	Register(storeCodec{})
	Register(borrowAllCodec{})
	Register(passThroughCodec{})
	Register(flowIntoCodec{})
	Register(sendCodec{})
	Register(freezeCodec{})
	Register(correlatedReturnCodec{})
	Register(moduleLoadCodec{})
	Register(variadicTransformCodec{})
	Register(typePredicateCodec{})
	Register(typeValueMethodCodec{})
	Register(callableTypeCodec{})
}

// throwCodec handles Throw effect serialization.
type throwCodec struct{}

func (throwCodec) Key() string { return KeyThrow }

func (throwCodec) Encode(l Label, w Writer) error {
	return nil
}

func (throwCodec) Decode(r Reader) (Label, error) {
	return Throw{}, nil
}

// ioCodec handles IO effect serialization.
type ioCodec struct{}

func (ioCodec) Key() string { return KeyIO }

func (ioCodec) Encode(l Label, w Writer) error {
	return nil
}

func (ioCodec) Decode(r Reader) (Label, error) {
	return IO{}, nil
}

// divergeCodec handles Diverge effect serialization.
type divergeCodec struct{}

func (divergeCodec) Key() string { return KeyDiverge }

func (divergeCodec) Encode(l Label, w Writer) error {
	return nil
}

func (divergeCodec) Decode(r Reader) (Label, error) {
	return Diverge{}, nil
}

// mutateCodec handles Mutate effect serialization.
type mutateCodec struct{}

func (mutateCodec) Key() string { return KeyMutate }

func (mutateCodec) Encode(l Label, w Writer) error {
	m := l.(Mutate)
	if err := w.WriteInt32(int32(m.Target.Index)); err != nil {
		return err
	}

	if err := writeTransform(w, m.Transform); err != nil {
		return err
	}

	return writeExpr(w, m.LengthDelta)
}

func (mutateCodec) Decode(r Reader) (Label, error) {
	idx, err := r.ReadInt32()
	if err != nil {
		return nil, err
	}

	transform, err := readTransform(r)
	if err != nil {
		return nil, err
	}

	delta, err := readExpr(r)
	if err != nil {
		return nil, err
	}

	return Mutate{
		Target:      ParamRef{Index: int(idx)},
		Transform:   transform,
		LengthDelta: delta,
	}, nil
}

// returnCodec handles Return effect serialization.
type returnCodec struct{}

func (returnCodec) Key() string { return KeyReturn }

func (returnCodec) Encode(l Label, w Writer) error {
	ret := l.(Return)
	if err := w.WriteInt32(int32(ret.ReturnIndex)); err != nil {
		return err
	}

	return writeReturnType(w, ret.Transform)
}

func (returnCodec) Decode(r Reader) (Label, error) {
	idx, err := r.ReadInt32()
	if err != nil {
		return nil, err
	}

	transform, err := readReturnType(r)
	if err != nil {
		return nil, err
	}

	return Return{ReturnIndex: int(idx), Transform: transform}, nil
}

// errorReturnCodec handles ErrorReturn effect serialization.
type errorReturnCodec struct{}

func (errorReturnCodec) Key() string { return KeyErrorReturn }

func (errorReturnCodec) Encode(l Label, w Writer) error {
	e := l.(ErrorReturn)
	if err := w.WriteInt32(int32(e.ValueIndex)); err != nil {
		return err
	}
	return w.WriteInt32(int32(e.ErrorIndex))
}

func (errorReturnCodec) Decode(r Reader) (Label, error) {
	valIdx, err := r.ReadInt32()
	if err != nil {
		return nil, err
	}
	errIdx, err := r.ReadInt32()
	if err != nil {
		return nil, err
	}
	return ErrorReturn{ValueIndex: int(valIdx), ErrorIndex: int(errIdx)}, nil
}

// returnLengthCodec handles ReturnLength effect serialization.
type returnLengthCodec struct{}

func (returnLengthCodec) Key() string { return KeyReturnLength }

func (returnLengthCodec) Encode(l Label, w Writer) error {
	rl := l.(ReturnLength)
	if err := w.WriteInt32(int32(rl.ReturnIndex)); err != nil {
		return err
	}

	return writeExpr(w, rl.Length)
}

func (returnLengthCodec) Decode(r Reader) (Label, error) {
	idx, err := r.ReadInt32()
	if err != nil {
		return nil, err
	}

	length, err := readExpr(r)
	if err != nil {
		return nil, err
	}

	return ReturnLength{ReturnIndex: int(idx), Length: length}, nil
}

// iteratorCodec handles Iterator effect serialization.
type iteratorCodec struct{}

func (iteratorCodec) Key() string { return KeyIterator }

func (iteratorCodec) Encode(l Label, w Writer) error {
	it := l.(Iterator)
	if err := w.WriteInt32(int32(it.Source.Index)); err != nil {
		return err
	}

	return w.WriteByte(byte(it.Kind))
}

func (iteratorCodec) Decode(r Reader) (Label, error) {
	idx, err := r.ReadInt32()
	if err != nil {
		return nil, err
	}

	kind, err := r.ReadByte()
	if err != nil {
		return nil, err
	}

	return Iterator{Source: ParamRef{Index: int(idx)}, Kind: IteratorKind(kind)}, nil
}

// tableMutatorCodec handles TableMutator effect serialization.
type tableMutatorCodec struct{}

func (tableMutatorCodec) Key() string { return KeyTableMutator }

func (tableMutatorCodec) Encode(l Label, w Writer) error {
	tm := l.(TableMutator)
	if err := w.WriteInt32(int32(tm.Target.Index)); err != nil {
		return err
	}

	return w.WriteInt32(int32(tm.Value.Index))
}

func (tableMutatorCodec) Decode(r Reader) (Label, error) {
	target, err := r.ReadInt32()
	if err != nil {
		return nil, err
	}

	value, err := r.ReadInt32()
	if err != nil {
		return nil, err
	}

	return TableMutator{
		Target: ParamRef{Index: int(target)},
		Value:  ParamRef{Index: int(value)},
	}, nil
}

// lengthChangeCodec handles LengthChange effect serialization.
type lengthChangeCodec struct{}

func (lengthChangeCodec) Key() string { return KeyLengthChange }

func (lengthChangeCodec) Encode(l Label, w Writer) error {
	lc := l.(LengthChange)
	if err := w.WriteInt32(int32(lc.Target.Index)); err != nil {
		return err
	}

	return w.WriteInt32(int32(lc.Delta))
}

func (lengthChangeCodec) Decode(r Reader) (Label, error) {
	target, err := r.ReadInt32()
	if err != nil {
		return nil, err
	}

	delta, err := r.ReadInt32()
	if err != nil {
		return nil, err
	}

	return LengthChange{Target: ParamRef{Index: int(target)}, Delta: int(delta)}, nil
}

// Transform encoding tags
const (
	transformUnchanged    = 0
	transformElementUnion = 1
	transformToArray      = 2
)

func writeTransform(w Writer, t TypeTransform) error {
	if t == nil {
		return w.WriteByte(transformUnchanged)
	}

	return VisitTransform(t, TypeTransformVisitor[error]{
		Unchanged: func(Unchanged) error {
			return w.WriteByte(transformUnchanged)
		},
		ElementUnion: func(v ElementUnion) error {
			if err := w.WriteByte(transformElementUnion); err != nil {
				return err
			}
			return w.WriteInt32(int32(v.Source.Index))
		},
		ToArray: func(v ToArray) error {
			if err := w.WriteByte(transformToArray); err != nil {
				return err
			}
			return w.WriteInt32(int32(v.Element.Index))
		},
		Default: func(TypeTransform) error {
			return w.WriteByte(transformUnchanged)
		},
	})
}

func readTransform(r Reader) (TypeTransform, error) {
	tag, err := r.ReadByte()
	if err != nil {
		return nil, err
	}

	switch tag {
	case transformUnchanged:
		return Unchanged{}, nil
	case transformElementUnion:
		idx, err := r.ReadInt32()
		if err != nil {
			return nil, err
		}

		return ElementUnion{Source: ParamRef{Index: int(idx)}}, nil
	case transformToArray:
		idx, err := r.ReadInt32()
		if err != nil {
			return nil, err
		}

		return ToArray{Element: ParamRef{Index: int(idx)}}, nil
	default:
		return Unchanged{}, nil
	}
}

// ReturnType encoding tags
const (
	returnTypeNil               = 0
	returnTypeElementOf         = 1
	returnTypeOptionalElementOf = 2
	returnTypeCallbackReturn    = 3
	returnTypeArrayOfCallback   = 4
	returnTypeSameAs            = 5
	returnTypeDeepElementOf     = 6
	returnTypeStringUnpackValue = 7
	returnTypeSelectCaseOfParam = 8
	returnTypeSelectResultCases = 9
)

func writeReturnType(w Writer, rt ReturnType) error {
	if rt == nil {
		return w.WriteByte(returnTypeNil)
	}

	return VisitReturnType(rt, ReturnTypeVisitor[error]{
		ElementOf: func(v ElementOf) error {
			if err := w.WriteByte(returnTypeElementOf); err != nil {
				return err
			}
			return w.WriteInt32(int32(v.Source.Index))
		},
		OptionalElementOf: func(v OptionalElementOf) error {
			if err := w.WriteByte(returnTypeOptionalElementOf); err != nil {
				return err
			}
			return w.WriteInt32(int32(v.Source.Index))
		},
		CallbackReturn: func(v CallbackReturn) error {
			if err := w.WriteByte(returnTypeCallbackReturn); err != nil {
				return err
			}
			return w.WriteInt32(int32(v.CallbackParam.Index))
		},
		ArrayOfCallbackReturn: func(v ArrayOfCallbackReturn) error {
			if err := w.WriteByte(returnTypeArrayOfCallback); err != nil {
				return err
			}
			return w.WriteInt32(int32(v.CallbackParam.Index))
		},
		SameAs: func(v SameAs) error {
			if err := w.WriteByte(returnTypeSameAs); err != nil {
				return err
			}
			return w.WriteInt32(int32(v.Source.Index))
		},
		DeepElementOf: func(v DeepElementOf) error {
			if err := w.WriteByte(returnTypeDeepElementOf); err != nil {
				return err
			}
			return w.WriteInt32(int32(v.Source.Index))
		},
		StringUnpackValue: func(v StringUnpackValue) error {
			if err := w.WriteByte(returnTypeStringUnpackValue); err != nil {
				return err
			}
			return w.WriteInt32(int32(v.Format.Index))
		},
		SelectCaseOfParam: func(v SelectCaseOfParam) error {
			if err := w.WriteByte(returnTypeSelectCaseOfParam); err != nil {
				return err
			}
			return w.WriteInt32(int32(v.Source.Index))
		},
		SelectResultOfCases: func(v SelectResultOfCases) error {
			if err := w.WriteByte(returnTypeSelectResultCases); err != nil {
				return err
			}
			if err := w.WriteInt32(int32(v.Cases.Index)); err != nil {
				return err
			}
			return w.WriteInt32(int32(v.Default.Index))
		},
		Default: func(ReturnType) error {
			return w.WriteByte(returnTypeNil)
		},
	})
}

func readReturnType(r Reader) (ReturnType, error) {
	tag, err := r.ReadByte()
	if err != nil {
		return nil, err
	}

	switch tag {
	case returnTypeNil:
		return nil, nil
	case returnTypeElementOf:
		idx, err := r.ReadInt32()
		if err != nil {
			return nil, err
		}

		return ElementOf{Source: ParamRef{Index: int(idx)}}, nil
	case returnTypeOptionalElementOf:
		idx, err := r.ReadInt32()
		if err != nil {
			return nil, err
		}

		return OptionalElementOf{Source: ParamRef{Index: int(idx)}}, nil
	case returnTypeCallbackReturn:
		idx, err := r.ReadInt32()
		if err != nil {
			return nil, err
		}

		return CallbackReturn{CallbackParam: ParamRef{Index: int(idx)}}, nil
	case returnTypeArrayOfCallback:
		idx, err := r.ReadInt32()
		if err != nil {
			return nil, err
		}

		return ArrayOfCallbackReturn{CallbackParam: ParamRef{Index: int(idx)}}, nil
	case returnTypeSameAs:
		idx, err := r.ReadInt32()
		if err != nil {
			return nil, err
		}

		return SameAs{Source: ParamRef{Index: int(idx)}}, nil
	case returnTypeDeepElementOf:
		idx, err := r.ReadInt32()
		if err != nil {
			return nil, err
		}

		return DeepElementOf{Source: ParamRef{Index: int(idx)}}, nil
	case returnTypeStringUnpackValue:
		idx, err := r.ReadInt32()
		if err != nil {
			return nil, err
		}

		return StringUnpackValue{Format: ParamRef{Index: int(idx)}}, nil
	case returnTypeSelectCaseOfParam:
		idx, err := r.ReadInt32()
		if err != nil {
			return nil, err
		}

		return SelectCaseOfParam{Source: ParamRef{Index: int(idx)}}, nil
	case returnTypeSelectResultCases:
		casesIdx, err := r.ReadInt32()
		if err != nil {
			return nil, err
		}

		defaultIdx, err := r.ReadInt32()
		if err != nil {
			return nil, err
		}

		return SelectResultOfCases{
			Cases:   ParamRef{Index: int(casesIdx)},
			Default: ParamRef{Index: int(defaultIdx)},
		}, nil
	default:
		return nil, fmt.Errorf("unknown return type tag: %d", tag)
	}
}

// Expr encoding tags
const (
	exprNil      = 0
	exprVar      = 1
	exprConst    = 2
	exprBinOp    = 3
	exprLen      = 4
	exprParam    = 5
	exprRet      = 6
	exprParamLen = 7
	exprRetLen   = 8
	exprMin      = 9
	exprMax      = 10
)

func writeExpr(w Writer, e constraint.Expr) error {
	if e == nil {
		return w.WriteByte(exprNil)
	}

	return constraint.VisitExpr(e, constraint.ExprVisitor[error]{
		Var: func(v constraint.Var) error {
			if err := w.WriteByte(exprVar); err != nil {
				return err
			}
			return w.WriteString(v.Name)
		},
		Const: func(v constraint.Const) error {
			if err := w.WriteByte(exprConst); err != nil {
				return err
			}
			return w.WriteInt32(int32(v.Value))
		},
		BinOp: func(v constraint.BinOp) error {
			if err := w.WriteByte(exprBinOp); err != nil {
				return err
			}
			if err := w.WriteByte(byte(v.Op)); err != nil {
				return err
			}
			if err := writeExpr(w, v.Left); err != nil {
				return err
			}
			return writeExpr(w, v.Right)
		},
		Len: func(v constraint.Len) error {
			if err := w.WriteByte(exprLen); err != nil {
				return err
			}
			return w.WriteString(v.Of)
		},
		Param: func(v constraint.Param) error {
			if err := w.WriteByte(exprParam); err != nil {
				return err
			}
			return w.WriteInt32(int32(v.Index))
		},
		Ret: func(v constraint.Ret) error {
			if err := w.WriteByte(exprRet); err != nil {
				return err
			}
			return w.WriteInt32(int32(v.Index))
		},
		ParamLen: func(v constraint.ParamLen) error {
			if err := w.WriteByte(exprParamLen); err != nil {
				return err
			}
			return w.WriteInt32(int32(v.Index))
		},
		RetLen: func(v constraint.RetLen) error {
			if err := w.WriteByte(exprRetLen); err != nil {
				return err
			}
			return w.WriteInt32(int32(v.Index))
		},
		Min: func(v constraint.Min) error {
			if err := w.WriteByte(exprMin); err != nil {
				return err
			}
			if err := writeExpr(w, v.Left); err != nil {
				return err
			}
			return writeExpr(w, v.Right)
		},
		Max: func(v constraint.Max) error {
			if err := w.WriteByte(exprMax); err != nil {
				return err
			}
			if err := writeExpr(w, v.Left); err != nil {
				return err
			}
			return writeExpr(w, v.Right)
		},
		Default: func(constraint.Expr) error {
			return w.WriteByte(exprNil)
		},
	})
}

func readExpr(r Reader) (constraint.Expr, error) {
	tag, err := r.ReadByte()
	if err != nil {
		return nil, err
	}

	switch tag {
	case exprNil:
		return nil, nil
	case exprVar:
		name, err := r.ReadString()
		if err != nil {
			return nil, err
		}

		return constraint.Var{Name: name}, nil
	case exprConst:
		val, err := r.ReadInt32()
		if err != nil {
			return nil, err
		}

		return constraint.Const{Value: int64(val)}, nil
	case exprBinOp:
		op, err := r.ReadByte()
		if err != nil {
			return nil, err
		}

		left, err := readExpr(r)
		if err != nil {
			return nil, err
		}

		right, err := readExpr(r)
		if err != nil {
			return nil, err
		}

		return constraint.BinOp{Op: constraint.Op(op), Left: left, Right: right}, nil
	case exprLen:
		of, err := r.ReadString()
		if err != nil {
			return nil, err
		}

		return constraint.Len{Of: of}, nil
	case exprParam:
		idx, err := r.ReadInt32()
		if err != nil {
			return nil, err
		}

		return constraint.Param{Index: int(idx)}, nil
	case exprRet:
		idx, err := r.ReadInt32()
		if err != nil {
			return nil, err
		}

		return constraint.Ret{Index: int(idx)}, nil
	case exprParamLen:
		idx, err := r.ReadInt32()
		if err != nil {
			return nil, err
		}

		return constraint.ParamLen{Index: int(idx)}, nil
	case exprRetLen:
		idx, err := r.ReadInt32()
		if err != nil {
			return nil, err
		}

		return constraint.RetLen{Index: int(idx)}, nil
	case exprMin:
		left, err := readExpr(r)
		if err != nil {
			return nil, err
		}

		right, err := readExpr(r)
		if err != nil {
			return nil, err
		}

		return constraint.Min{Left: left, Right: right}, nil
	case exprMax:
		left, err := readExpr(r)
		if err != nil {
			return nil, err
		}

		right, err := readExpr(r)
		if err != nil {
			return nil, err
		}

		return constraint.Max{Left: left, Right: right}, nil
	default:
		return nil, fmt.Errorf("unknown expr tag: %d", tag)
	}
}

// borrowCodec handles Borrow effect serialization.
type borrowCodec struct{}

func (borrowCodec) Key() string { return KeyBorrow }

func (borrowCodec) Encode(l Label, w Writer) error {
	b := l.(Borrow)
	return w.WriteInt32(int32(b.Param.Index))
}

func (borrowCodec) Decode(r Reader) (Label, error) {
	idx, err := r.ReadInt32()
	if err != nil {
		return nil, err
	}

	return Borrow{Param: ParamRef{Index: int(idx)}}, nil
}

// storeCodec handles Store effect serialization.
type storeCodec struct{}

func (storeCodec) Key() string { return KeyStore }

func (storeCodec) Encode(l Label, w Writer) error {
	s := l.(Store)
	if err := w.WriteInt32(int32(s.Param.Index)); err != nil {
		return err
	}

	return w.WriteInt32(int32(s.Into.Index))
}

func (storeCodec) Decode(r Reader) (Label, error) {
	param, err := r.ReadInt32()
	if err != nil {
		return nil, err
	}

	into, err := r.ReadInt32()
	if err != nil {
		return nil, err
	}

	return Store{
		Param: ParamRef{Index: int(param)},
		Into:  ParamRef{Index: int(into)},
	}, nil
}

// borrowAllCodec handles BorrowAll effect serialization.
type borrowAllCodec struct{}

func (borrowAllCodec) Key() string { return KeyBorrowAll }

func (borrowAllCodec) Encode(l Label, w Writer) error {
	return nil
}

func (borrowAllCodec) Decode(r Reader) (Label, error) {
	return BorrowAll{}, nil
}

// passThroughCodec handles PassThrough effect serialization.
type passThroughCodec struct{}

func (passThroughCodec) Key() string { return KeyPassThrough }

func (passThroughCodec) Encode(l Label, w Writer) error {
	p := l.(PassThrough)
	if err := w.WriteInt32(int32(p.ParamIndex)); err != nil {
		return err
	}

	return w.WriteInt32(int32(p.ReturnIndex))
}

func (passThroughCodec) Decode(r Reader) (Label, error) {
	param, err := r.ReadInt32()
	if err != nil {
		return nil, err
	}

	ret, err := r.ReadInt32()
	if err != nil {
		return nil, err
	}

	return PassThrough{ParamIndex: int(param), ReturnIndex: int(ret)}, nil
}

// flowIntoCodec handles FlowInto effect serialization.
type flowIntoCodec struct{}

func (flowIntoCodec) Key() string { return KeyFlowInto }

func (flowIntoCodec) Encode(l Label, w Writer) error {
	f := l.(FlowInto)
	if err := w.WriteInt32(int32(f.ParamIndex)); err != nil {
		return err
	}

	if err := w.WriteInt32(int32(f.ReturnIndex)); err != nil {
		return err
	}

	return w.WriteString(f.Path)
}

func (flowIntoCodec) Decode(r Reader) (Label, error) {
	param, err := r.ReadInt32()
	if err != nil {
		return nil, err
	}

	ret, err := r.ReadInt32()
	if err != nil {
		return nil, err
	}

	path, err := r.ReadString()
	if err != nil {
		return nil, err
	}

	return FlowInto{ParamIndex: int(param), ReturnIndex: int(ret), Path: path}, nil
}

// sendCodec handles Send effect serialization.
type sendCodec struct{}

func (sendCodec) Key() string { return KeySend }

func (sendCodec) Encode(l Label, w Writer) error {
	s := l.(Send)
	return w.WriteInt32(int32(s.FromParam))
}

func (sendCodec) Decode(r Reader) (Label, error) {
	from, err := r.ReadInt32()
	if err != nil {
		return nil, err
	}

	return Send{FromParam: int(from)}, nil
}

// freezeCodec handles Freeze effect serialization.
type freezeCodec struct{}

func (freezeCodec) Key() string { return KeyFreeze }

func (freezeCodec) Encode(l Label, w Writer) error {
	f := l.(Freeze)
	return w.WriteInt32(int32(f.Param.Index))
}

func (freezeCodec) Decode(r Reader) (Label, error) {
	idx, err := r.ReadInt32()
	if err != nil {
		return nil, err
	}

	return Freeze{Param: ParamRef{Index: int(idx)}}, nil
}

// correlatedReturnCodec handles CorrelatedReturn effect serialization.
type correlatedReturnCodec struct{}

func (correlatedReturnCodec) Key() string { return KeyCorrelatedReturn }

func (correlatedReturnCodec) Encode(l Label, w Writer) error {
	c := l.(CorrelatedReturn)
	if err := w.WriteInt32(int32(len(c.Indices))); err != nil {
		return err
	}
	for _, idx := range c.Indices {
		if err := w.WriteInt32(int32(idx)); err != nil {
			return err
		}
	}
	return nil
}

func (correlatedReturnCodec) Decode(r Reader) (Label, error) {
	count, err := r.ReadInt32()
	if err != nil {
		return nil, err
	}
	indices := make([]int, count)
	for i := int32(0); i < count; i++ {
		idx, err := r.ReadInt32()
		if err != nil {
			return nil, err
		}
		indices[i] = int(idx)
	}
	return CorrelatedReturn{Indices: indices}, nil
}

// moduleLoadCodec handles ModuleLoad effect serialization.
type moduleLoadCodec struct{}

func (moduleLoadCodec) Key() string                    { return KeyModuleLoad }
func (moduleLoadCodec) Encode(l Label, w Writer) error { return nil }
func (moduleLoadCodec) Decode(r Reader) (Label, error) { return ModuleLoad{}, nil }

// variadicTransformCodec handles VariadicTransform effect serialization.
type variadicTransformCodec struct{}

func (variadicTransformCodec) Key() string                    { return KeyVariadicTransform }
func (variadicTransformCodec) Encode(l Label, w Writer) error { return nil }
func (variadicTransformCodec) Decode(r Reader) (Label, error) { return VariadicTransform{}, nil }

// typePredicateCodec handles TypePredicate effect serialization.
type typePredicateCodec struct{}

func (typePredicateCodec) Key() string                    { return KeyTypePredicate }
func (typePredicateCodec) Encode(l Label, w Writer) error { return nil }
func (typePredicateCodec) Decode(r Reader) (Label, error) { return TypePredicate{}, nil }

// typeValueMethodCodec handles TypeValueMethod effect serialization.
type typeValueMethodCodec struct{}

func (typeValueMethodCodec) Key() string                    { return KeyTypeValueMethod }
func (typeValueMethodCodec) Encode(l Label, w Writer) error { return nil }
func (typeValueMethodCodec) Decode(r Reader) (Label, error) { return TypeValueMethod{}, nil }

// callableTypeCodec handles CallableType effect serialization.
type callableTypeCodec struct{}

func (callableTypeCodec) Key() string                    { return KeyCallableType }
func (callableTypeCodec) Encode(l Label, w Writer) error { return nil }
func (callableTypeCodec) Decode(r Reader) (Label, error) { return CallableType{}, nil }
