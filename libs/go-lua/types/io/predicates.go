package io

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

const (
	exprTagNone uint8 = iota
	exprTagVar
	exprTagConst
	exprTagBinOp
	exprTagLen
	exprTagParam
	exprTagRet
	exprTagParamLen
	exprTagRetLen
	exprTagMin
	exprTagMax
)

// --- encode ---

func (w *typeWriter) writeConstraint(c constraint.Constraint) {
	if c == nil {
		w.writeByte(byte(constraint.KindInvalid))
		return
	}

	w.writeByte(byte(c.Kind()))
	handled := constraint.VisitConstraint(c, constraint.ConstraintVisitor[bool]{
		Truthy: func(v constraint.Truthy) bool {
			w.writePath(v.Path)
			return true
		},
		Falsy: func(v constraint.Falsy) bool {
			w.writePath(v.Path)
			return true
		},
		IsNil: func(v constraint.IsNil) bool {
			w.writePath(v.Path)
			return true
		},
		NotNil: func(v constraint.NotNil) bool {
			w.writePath(v.Path)
			return true
		},
		HasType: func(v constraint.HasType) bool {
			w.writePath(v.Path)
			w.writeTypeKey(v.Type)
			return true
		},
		NotHasType: func(v constraint.NotHasType) bool {
			w.writePath(v.Path)
			w.writeTypeKey(v.Type)
			return true
		},
		FieldEquals: func(v constraint.FieldEquals) bool {
			w.writePath(v.Target)
			w.writeString(v.Field)
			w.writeBool(v.Value != nil)
			if v.Value != nil {
				w.writeLiteral(v.Value)
			}
			return true
		},
		IndexEquals: func(v constraint.IndexEquals) bool {
			w.writePath(v.Target)
			w.writeBool(v.Key != nil)
			if v.Key != nil {
				w.writeType(v.Key)
			}
			w.writeBool(v.Value != nil)
			if v.Value != nil {
				w.writeLiteral(v.Value)
			}
			return true
		},
		EqPath: func(v constraint.EqPath) bool {
			w.writePath(v.Left)
			w.writePath(v.Right)
			return true
		},
		NotEqPath: func(v constraint.NotEqPath) bool {
			w.writePath(v.Left)
			w.writePath(v.Right)
			return true
		},
		FieldEqualsPath: func(v constraint.FieldEqualsPath) bool {
			w.writePath(v.Target)
			w.writeString(v.Field)
			w.writePath(v.Value)
			return true
		},
		IndexEqualsPath: func(v constraint.IndexEqualsPath) bool {
			w.writePath(v.Target)
			w.writeBool(v.Key != nil)
			if v.Key != nil {
				w.writeType(v.Key)
			}
			w.writePath(v.Value)
			return true
		},
		KeyOf: func(v constraint.KeyOf) bool {
			w.writePath(v.Table)
			w.writePath(v.Key)
			return true
		},
		HasField: func(v constraint.HasField) bool {
			w.writePath(v.Path)
			w.writeString(v.Field)
			return true
		},
		FieldNotEquals: func(v constraint.FieldNotEquals) bool {
			w.writePath(v.Target)
			w.writeString(v.Field)
			w.writeBool(v.Value != nil)
			if v.Value != nil {
				w.writeLiteral(v.Value)
			}
			return true
		},
		IndexNotEquals: func(v constraint.IndexNotEquals) bool {
			w.writePath(v.Target)
			w.writeBool(v.Key != nil)
			if v.Key != nil {
				w.writeType(v.Key)
			}
			w.writeBool(v.Value != nil)
			if v.Value != nil {
				w.writeLiteral(v.Value)
			}
			return true
		},
		FieldNotEqualsPath: func(v constraint.FieldNotEqualsPath) bool {
			w.writePath(v.Target)
			w.writeString(v.Field)
			w.writePath(v.Value)
			return true
		},
		IndexNotEqualsPath: func(v constraint.IndexNotEqualsPath) bool {
			w.writePath(v.Target)
			w.writeBool(v.Key != nil)
			if v.Key != nil {
				w.writeType(v.Key)
			}
			w.writePath(v.Value)
			return true
		},
		Default: func(constraint.Constraint) bool {
			return false
		},
	})
	if !handled {
		w.writeByte(byte(constraint.KindInvalid))
	}
}

func (w *typeWriter) writeConjunction(conj []constraint.Constraint) {
	w.writeUint32(uint32(len(conj)))

	for _, c := range conj {
		w.writeConstraint(c)
	}
}

func (w *typeWriter) writeCondition(cond constraint.Condition) {
	if cond.IsFalse() {
		w.writeUint32(0)
		return
	}

	w.writeUint32(uint32(len(cond.Disjuncts)))
	for _, d := range cond.Disjuncts {
		w.writeConjunction(d)
	}
}

func (w *typeWriter) writeFunctionRefinement(eff *constraint.FunctionRefinement) {
	if eff == nil {
		w.writeBool(false)
		return
	}

	w.writeBool(true)
	w.writeCondition(eff.OnReturn)
	w.writeCondition(eff.OnTrue)
	w.writeCondition(eff.OnFalse)
}

func (w *typeWriter) writeExpr(e constraint.Expr) {
	if e == nil {
		w.writeByte(exprTagNone)
		return
	}

	constraint.VisitExpr(e, constraint.ExprVisitor[struct{}]{
		Var: func(v constraint.Var) struct{} {
			w.writeByte(exprTagVar)
			w.writeString(v.Name)
			return struct{}{}
		},
		Const: func(v constraint.Const) struct{} {
			w.writeByte(exprTagConst)
			w.writeUint64(uint64(v.Value))
			return struct{}{}
		},
		BinOp: func(v constraint.BinOp) struct{} {
			w.writeByte(exprTagBinOp)
			w.writeByte(byte(v.Op))
			w.writeExpr(v.Left)
			w.writeExpr(v.Right)
			return struct{}{}
		},
		Len: func(v constraint.Len) struct{} {
			w.writeByte(exprTagLen)
			w.writeString(v.Of)
			return struct{}{}
		},
		Param: func(v constraint.Param) struct{} {
			w.writeByte(exprTagParam)
			w.writeUint32(uint32(int32(v.Index)))
			return struct{}{}
		},
		Ret: func(v constraint.Ret) struct{} {
			w.writeByte(exprTagRet)
			w.writeUint32(uint32(int32(v.Index)))
			return struct{}{}
		},
		ParamLen: func(v constraint.ParamLen) struct{} {
			w.writeByte(exprTagParamLen)
			w.writeUint32(uint32(int32(v.Index)))
			return struct{}{}
		},
		RetLen: func(v constraint.RetLen) struct{} {
			w.writeByte(exprTagRetLen)
			w.writeUint32(uint32(int32(v.Index)))
			return struct{}{}
		},
		Min: func(v constraint.Min) struct{} {
			w.writeByte(exprTagMin)
			w.writeExpr(v.Left)
			w.writeExpr(v.Right)
			return struct{}{}
		},
		Max: func(v constraint.Max) struct{} {
			w.writeByte(exprTagMax)
			w.writeExpr(v.Left)
			w.writeExpr(v.Right)
			return struct{}{}
		},
		Default: func(constraint.Expr) struct{} {
			w.writeByte(exprTagNone)
			return struct{}{}
		},
	})
}

func (w *typeWriter) writeExprCompares(list []constraint.ExprCompare) {
	w.writeUint32(uint32(len(list)))

	for _, c := range list {
		w.writeByte(byte(c.Rel))
		w.writeExpr(c.Left)
		w.writeExpr(c.Right)
	}
}

// --- decode ---

func (r *typeReader) readConstraint() constraint.Constraint {
	kind := constraint.Kind(r.readByte())
	switch kind {
	case constraint.KindTruthy:
		return constraint.Truthy{Path: r.readPath()}
	case constraint.KindFalsy:
		return constraint.Falsy{Path: r.readPath()}
	case constraint.KindIsNil:
		return constraint.IsNil{Path: r.readPath()}
	case constraint.KindNotNil:
		return constraint.NotNil{Path: r.readPath()}
	case constraint.KindHasType:
		return constraint.HasType{Path: r.readPath(), Type: r.readTypeKey()}
	case constraint.KindNotHasType:
		return constraint.NotHasType{Path: r.readPath(), Type: r.readTypeKey()}
	case constraint.KindFieldEquals:
		target := r.readPath()
		field := r.readString()

		var value *typ.Literal
		if r.readBool() {
			value = r.readLiteral()
		}

		return constraint.FieldEquals{Target: target, Field: field, Value: value}
	case constraint.KindIndexEquals:
		target := r.readPath()

		var key typ.Type
		if r.readBool() {
			key = r.readType()
		}

		var value *typ.Literal
		if r.readBool() {
			value = r.readLiteral()
		}

		return constraint.IndexEquals{Target: target, Key: key, Value: value}
	case constraint.KindEqPath:
		return constraint.EqPath{Left: r.readPath(), Right: r.readPath()}
	case constraint.KindNotEqPath:
		return constraint.NotEqPath{Left: r.readPath(), Right: r.readPath()}
	case constraint.KindFieldEqualsPath:
		target := r.readPath()
		field := r.readString()
		value := r.readPath()

		return constraint.FieldEqualsPath{Target: target, Field: field, Value: value}
	case constraint.KindIndexEqualsPath:
		target := r.readPath()

		var key typ.Type
		if r.readBool() {
			key = r.readType()
		}

		value := r.readPath()

		return constraint.IndexEqualsPath{Target: target, Key: key, Value: value}
	case constraint.KindKeyOf:
		return constraint.KeyOf{Table: r.readPath(), Key: r.readPath()}
	case constraint.KindHasField:
		return constraint.HasField{Path: r.readPath(), Field: r.readString()}
	case constraint.KindFieldNotEquals:
		target := r.readPath()
		field := r.readString()

		var value *typ.Literal
		if r.readBool() {
			value = r.readLiteral()
		}

		return constraint.FieldNotEquals{Target: target, Field: field, Value: value}
	case constraint.KindIndexNotEquals:
		target := r.readPath()

		var key typ.Type
		if r.readBool() {
			key = r.readType()
		}

		var value *typ.Literal
		if r.readBool() {
			value = r.readLiteral()
		}

		return constraint.IndexNotEquals{Target: target, Key: key, Value: value}
	case constraint.KindFieldNotEqualsPath:
		target := r.readPath()
		field := r.readString()
		value := r.readPath()

		return constraint.FieldNotEqualsPath{Target: target, Field: field, Value: value}
	case constraint.KindIndexNotEqualsPath:
		target := r.readPath()

		var key typ.Type
		if r.readBool() {
			key = r.readType()
		}

		value := r.readPath()

		return constraint.IndexNotEqualsPath{Target: target, Key: key, Value: value}
	case constraint.KindInvalid:
		return nil
	}

	return nil
}

func (r *typeReader) readConjunction() []constraint.Constraint {
	count := r.readUint32()
	if !r.checkSliceLen(count) {
		return nil
	}

	list := make([]constraint.Constraint, 0, count)

	for i := uint32(0); i < count; i++ {
		if c := r.readConstraint(); c != nil {
			list = append(list, c)
		}
	}

	return list
}

func (r *typeReader) readCondition() constraint.Condition {
	count := r.readUint32()
	if count == 0 {
		return constraint.Condition{}
	}
	if !r.checkSliceLen(count) {
		return constraint.Condition{}
	}
	disjuncts := make([][]constraint.Constraint, int(count))
	for i := 0; i < int(count); i++ {
		disjuncts[i] = r.readConjunction()
	}
	return constraint.Condition{Disjuncts: disjuncts}
}

func (r *typeReader) readFunctionRefinement() *constraint.FunctionRefinement {
	if !r.readBool() {
		return nil
	}

	return &constraint.FunctionRefinement{
		OnReturn: r.readCondition(),
		OnTrue:   r.readCondition(),
		OnFalse:  r.readCondition(),
	}
}

func (r *typeReader) readExpr() constraint.Expr {
	tag := r.readByte()
	switch tag {
	case exprTagNone:
		return nil
	case exprTagVar:
		return constraint.Var{Name: r.readString()}
	case exprTagConst:
		return constraint.Const{Value: int64(r.readUint64())}
	case exprTagBinOp:
		op := constraint.Op(r.readByte())
		left := r.readExpr()
		right := r.readExpr()

		return constraint.BinOp{Op: op, Left: left, Right: right}
	case exprTagLen:
		return constraint.Len{Of: r.readString()}
	case exprTagParam:
		return constraint.Param{Index: int(int32(r.readUint32()))}
	case exprTagRet:
		return constraint.Ret{Index: int(int32(r.readUint32()))}
	case exprTagParamLen:
		return constraint.ParamLen{Index: int(int32(r.readUint32()))}
	case exprTagRetLen:
		return constraint.RetLen{Index: int(int32(r.readUint32()))}
	case exprTagMin:
		return constraint.Min{Left: r.readExpr(), Right: r.readExpr()}
	case exprTagMax:
		return constraint.Max{Left: r.readExpr(), Right: r.readExpr()}
	default:
		return nil
	}
}

func (r *typeReader) readExprCompares() []constraint.ExprCompare {
	count := r.readUint32()
	if !r.checkSliceLen(count) {
		return nil
	}

	comps := make([]constraint.ExprCompare, 0, count)

	for i := uint32(0); i < count; i++ {
		rel := constraint.ExprRel(r.readByte())
		left := r.readExpr()
		right := r.readExpr()
		comps = append(comps, constraint.ExprCompare{Rel: rel, Left: left, Right: right})
	}

	return comps
}
