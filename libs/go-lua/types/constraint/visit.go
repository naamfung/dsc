package constraint

// ConstraintVisitor dispatches on constraint variants.
// Nil handlers fall back to Default when provided; otherwise return zero.
type ConstraintVisitor[R any] struct {
	Truthy             func(Truthy) R
	Falsy              func(Falsy) R
	IsNil              func(IsNil) R
	NotNil             func(NotNil) R
	HasType            func(HasType) R
	NotHasType         func(NotHasType) R
	HasField           func(HasField) R
	FieldEquals        func(FieldEquals) R
	FieldNotEquals     func(FieldNotEquals) R
	IndexEquals        func(IndexEquals) R
	IndexNotEquals     func(IndexNotEquals) R
	EqPath             func(EqPath) R
	NotEqPath          func(NotEqPath) R
	FieldEqualsPath    func(FieldEqualsPath) R
	FieldNotEqualsPath func(FieldNotEqualsPath) R
	IndexEqualsPath    func(IndexEqualsPath) R
	IndexNotEqualsPath func(IndexNotEqualsPath) R
	KeyOf              func(KeyOf) R
	Default            func(Constraint) R
}

// VisitConstraint applies the first matching handler in v to c.
func VisitConstraint[R any](c Constraint, v ConstraintVisitor[R]) R {
	switch cc := c.(type) {
	case Truthy:
		if v.Truthy != nil {
			return v.Truthy(cc)
		}
	case *Truthy:
		if v.Truthy != nil {
			return v.Truthy(*cc)
		}
	case Falsy:
		if v.Falsy != nil {
			return v.Falsy(cc)
		}
	case *Falsy:
		if v.Falsy != nil {
			return v.Falsy(*cc)
		}
	case IsNil:
		if v.IsNil != nil {
			return v.IsNil(cc)
		}
	case *IsNil:
		if v.IsNil != nil {
			return v.IsNil(*cc)
		}
	case NotNil:
		if v.NotNil != nil {
			return v.NotNil(cc)
		}
	case *NotNil:
		if v.NotNil != nil {
			return v.NotNil(*cc)
		}
	case HasType:
		if v.HasType != nil {
			return v.HasType(cc)
		}
	case *HasType:
		if v.HasType != nil {
			return v.HasType(*cc)
		}
	case NotHasType:
		if v.NotHasType != nil {
			return v.NotHasType(cc)
		}
	case *NotHasType:
		if v.NotHasType != nil {
			return v.NotHasType(*cc)
		}
	case HasField:
		if v.HasField != nil {
			return v.HasField(cc)
		}
	case *HasField:
		if v.HasField != nil {
			return v.HasField(*cc)
		}
	case FieldEquals:
		if v.FieldEquals != nil {
			return v.FieldEquals(cc)
		}
	case *FieldEquals:
		if v.FieldEquals != nil {
			return v.FieldEquals(*cc)
		}
	case FieldNotEquals:
		if v.FieldNotEquals != nil {
			return v.FieldNotEquals(cc)
		}
	case *FieldNotEquals:
		if v.FieldNotEquals != nil {
			return v.FieldNotEquals(*cc)
		}
	case IndexEquals:
		if v.IndexEquals != nil {
			return v.IndexEquals(cc)
		}
	case *IndexEquals:
		if v.IndexEquals != nil {
			return v.IndexEquals(*cc)
		}
	case IndexNotEquals:
		if v.IndexNotEquals != nil {
			return v.IndexNotEquals(cc)
		}
	case *IndexNotEquals:
		if v.IndexNotEquals != nil {
			return v.IndexNotEquals(*cc)
		}
	case EqPath:
		if v.EqPath != nil {
			return v.EqPath(cc)
		}
	case *EqPath:
		if v.EqPath != nil {
			return v.EqPath(*cc)
		}
	case NotEqPath:
		if v.NotEqPath != nil {
			return v.NotEqPath(cc)
		}
	case *NotEqPath:
		if v.NotEqPath != nil {
			return v.NotEqPath(*cc)
		}
	case FieldEqualsPath:
		if v.FieldEqualsPath != nil {
			return v.FieldEqualsPath(cc)
		}
	case *FieldEqualsPath:
		if v.FieldEqualsPath != nil {
			return v.FieldEqualsPath(*cc)
		}
	case FieldNotEqualsPath:
		if v.FieldNotEqualsPath != nil {
			return v.FieldNotEqualsPath(cc)
		}
	case *FieldNotEqualsPath:
		if v.FieldNotEqualsPath != nil {
			return v.FieldNotEqualsPath(*cc)
		}
	case IndexEqualsPath:
		if v.IndexEqualsPath != nil {
			return v.IndexEqualsPath(cc)
		}
	case *IndexEqualsPath:
		if v.IndexEqualsPath != nil {
			return v.IndexEqualsPath(*cc)
		}
	case IndexNotEqualsPath:
		if v.IndexNotEqualsPath != nil {
			return v.IndexNotEqualsPath(cc)
		}
	case *IndexNotEqualsPath:
		if v.IndexNotEqualsPath != nil {
			return v.IndexNotEqualsPath(*cc)
		}
	case KeyOf:
		if v.KeyOf != nil {
			return v.KeyOf(cc)
		}
	case *KeyOf:
		if v.KeyOf != nil {
			return v.KeyOf(*cc)
		}
	}
	if v.Default != nil {
		return v.Default(c)
	}
	var zero R
	return zero
}

// NumericConstraintVisitor dispatches on numeric constraint variants.
// Nil handlers fall back to Default when provided; otherwise return zero.
type NumericConstraintVisitor[R any] struct {
	Le      func(Le) R
	Lt      func(Lt) R
	Ge      func(Ge) R
	Gt      func(Gt) R
	Eq      func(Eq) R
	EqConst func(EqConst) R
	LeConst func(LeConst) R
	GeConst func(GeConst) R
	ModEq   func(ModEq) R
	LeLenOf func(LeLenOf) R
	Default func(NumericConstraint) R
}

// VisitNumericConstraint applies the first matching handler in v to c.
func VisitNumericConstraint[R any](c NumericConstraint, v NumericConstraintVisitor[R]) R {
	switch cc := c.(type) {
	case Le:
		if v.Le != nil {
			return v.Le(cc)
		}
	case *Le:
		if v.Le != nil {
			return v.Le(*cc)
		}
	case Lt:
		if v.Lt != nil {
			return v.Lt(cc)
		}
	case *Lt:
		if v.Lt != nil {
			return v.Lt(*cc)
		}
	case Ge:
		if v.Ge != nil {
			return v.Ge(cc)
		}
	case *Ge:
		if v.Ge != nil {
			return v.Ge(*cc)
		}
	case Gt:
		if v.Gt != nil {
			return v.Gt(cc)
		}
	case *Gt:
		if v.Gt != nil {
			return v.Gt(*cc)
		}
	case Eq:
		if v.Eq != nil {
			return v.Eq(cc)
		}
	case *Eq:
		if v.Eq != nil {
			return v.Eq(*cc)
		}
	case EqConst:
		if v.EqConst != nil {
			return v.EqConst(cc)
		}
	case *EqConst:
		if v.EqConst != nil {
			return v.EqConst(*cc)
		}
	case LeConst:
		if v.LeConst != nil {
			return v.LeConst(cc)
		}
	case *LeConst:
		if v.LeConst != nil {
			return v.LeConst(*cc)
		}
	case GeConst:
		if v.GeConst != nil {
			return v.GeConst(cc)
		}
	case *GeConst:
		if v.GeConst != nil {
			return v.GeConst(*cc)
		}
	case ModEq:
		if v.ModEq != nil {
			return v.ModEq(cc)
		}
	case *ModEq:
		if v.ModEq != nil {
			return v.ModEq(*cc)
		}
	case LeLenOf:
		if v.LeLenOf != nil {
			return v.LeLenOf(cc)
		}
	case *LeLenOf:
		if v.LeLenOf != nil {
			return v.LeLenOf(*cc)
		}
	}
	if v.Default != nil {
		return v.Default(c)
	}
	var zero R
	return zero
}

// ExprVisitor dispatches on expression variants.
// Nil handlers fall back to Default when provided; otherwise return zero.
type ExprVisitor[R any] struct {
	Var      func(Var) R
	Const    func(Const) R
	BinOp    func(BinOp) R
	Len      func(Len) R
	Param    func(Param) R
	Ret      func(Ret) R
	ParamLen func(ParamLen) R
	RetLen   func(RetLen) R
	Min      func(Min) R
	Max      func(Max) R
	Default  func(Expr) R
}

// VisitExpr applies the first matching handler in v to e.
func VisitExpr[R any](e Expr, v ExprVisitor[R]) R {
	switch ee := e.(type) {
	case Var:
		if v.Var != nil {
			return v.Var(ee)
		}
	case *Var:
		if v.Var != nil {
			return v.Var(*ee)
		}
	case Const:
		if v.Const != nil {
			return v.Const(ee)
		}
	case *Const:
		if v.Const != nil {
			return v.Const(*ee)
		}
	case BinOp:
		if v.BinOp != nil {
			return v.BinOp(ee)
		}
	case *BinOp:
		if v.BinOp != nil {
			return v.BinOp(*ee)
		}
	case Len:
		if v.Len != nil {
			return v.Len(ee)
		}
	case *Len:
		if v.Len != nil {
			return v.Len(*ee)
		}
	case Param:
		if v.Param != nil {
			return v.Param(ee)
		}
	case *Param:
		if v.Param != nil {
			return v.Param(*ee)
		}
	case Ret:
		if v.Ret != nil {
			return v.Ret(ee)
		}
	case *Ret:
		if v.Ret != nil {
			return v.Ret(*ee)
		}
	case ParamLen:
		if v.ParamLen != nil {
			return v.ParamLen(ee)
		}
	case *ParamLen:
		if v.ParamLen != nil {
			return v.ParamLen(*ee)
		}
	case RetLen:
		if v.RetLen != nil {
			return v.RetLen(ee)
		}
	case *RetLen:
		if v.RetLen != nil {
			return v.RetLen(*ee)
		}
	case Min:
		if v.Min != nil {
			return v.Min(ee)
		}
	case *Min:
		if v.Min != nil {
			return v.Min(*ee)
		}
	case Max:
		if v.Max != nil {
			return v.Max(ee)
		}
	case *Max:
		if v.Max != nil {
			return v.Max(*ee)
		}
	}
	if v.Default != nil {
		return v.Default(e)
	}
	var zero R
	return zero
}
