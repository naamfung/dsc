package effect

// LabelVisitor dispatches on label variants.
// Nil handlers fall back to Default when provided; otherwise return zero.
type LabelVisitor[R any] struct {
	Mutate            func(Mutate) R
	Return            func(Return) R
	ErrorReturn       func(ErrorReturn) R
	ReturnLength      func(ReturnLength) R
	Throw             func(Throw) R
	Diverge           func(Diverge) R
	IO                func(IO) R
	LengthChange      func(LengthChange) R
	Iterator          func(Iterator) R
	TableMutator      func(TableMutator) R
	Borrow            func(Borrow) R
	Store             func(Store) R
	BorrowAll         func(BorrowAll) R
	PassThrough       func(PassThrough) R
	FlowInto          func(FlowInto) R
	Send              func(Send) R
	CorrelatedReturn  func(CorrelatedReturn) R
	Freeze            func(Freeze) R
	ModuleLoad        func(ModuleLoad) R
	VariadicTransform func(VariadicTransform) R
	TypePredicate     func(TypePredicate) R
	TypeValueMethod   func(TypeValueMethod) R
	CallableType      func(CallableType) R
	Default           func(Label) R
}

// VisitLabel applies the first matching handler in v to l.
func VisitLabel[R any](l Label, v LabelVisitor[R]) R {
	switch ll := l.(type) {
	case Mutate:
		if v.Mutate != nil {
			return v.Mutate(ll)
		}
	case *Mutate:
		if v.Mutate != nil {
			return v.Mutate(*ll)
		}
	case Return:
		if v.Return != nil {
			return v.Return(ll)
		}
	case *Return:
		if v.Return != nil {
			return v.Return(*ll)
		}
	case ErrorReturn:
		if v.ErrorReturn != nil {
			return v.ErrorReturn(ll)
		}
	case *ErrorReturn:
		if v.ErrorReturn != nil {
			return v.ErrorReturn(*ll)
		}
	case ReturnLength:
		if v.ReturnLength != nil {
			return v.ReturnLength(ll)
		}
	case *ReturnLength:
		if v.ReturnLength != nil {
			return v.ReturnLength(*ll)
		}
	case Throw:
		if v.Throw != nil {
			return v.Throw(ll)
		}
	case *Throw:
		if v.Throw != nil {
			return v.Throw(*ll)
		}
	case Diverge:
		if v.Diverge != nil {
			return v.Diverge(ll)
		}
	case *Diverge:
		if v.Diverge != nil {
			return v.Diverge(*ll)
		}
	case IO:
		if v.IO != nil {
			return v.IO(ll)
		}
	case *IO:
		if v.IO != nil {
			return v.IO(*ll)
		}
	case LengthChange:
		if v.LengthChange != nil {
			return v.LengthChange(ll)
		}
	case *LengthChange:
		if v.LengthChange != nil {
			return v.LengthChange(*ll)
		}
	case Iterator:
		if v.Iterator != nil {
			return v.Iterator(ll)
		}
	case *Iterator:
		if v.Iterator != nil {
			return v.Iterator(*ll)
		}
	case TableMutator:
		if v.TableMutator != nil {
			return v.TableMutator(ll)
		}
	case *TableMutator:
		if v.TableMutator != nil {
			return v.TableMutator(*ll)
		}
	case Borrow:
		if v.Borrow != nil {
			return v.Borrow(ll)
		}
	case *Borrow:
		if v.Borrow != nil {
			return v.Borrow(*ll)
		}
	case Store:
		if v.Store != nil {
			return v.Store(ll)
		}
	case *Store:
		if v.Store != nil {
			return v.Store(*ll)
		}
	case BorrowAll:
		if v.BorrowAll != nil {
			return v.BorrowAll(ll)
		}
	case *BorrowAll:
		if v.BorrowAll != nil {
			return v.BorrowAll(*ll)
		}
	case PassThrough:
		if v.PassThrough != nil {
			return v.PassThrough(ll)
		}
	case *PassThrough:
		if v.PassThrough != nil {
			return v.PassThrough(*ll)
		}
	case FlowInto:
		if v.FlowInto != nil {
			return v.FlowInto(ll)
		}
	case *FlowInto:
		if v.FlowInto != nil {
			return v.FlowInto(*ll)
		}
	case Send:
		if v.Send != nil {
			return v.Send(ll)
		}
	case *Send:
		if v.Send != nil {
			return v.Send(*ll)
		}
	case CorrelatedReturn:
		if v.CorrelatedReturn != nil {
			return v.CorrelatedReturn(ll)
		}
	case *CorrelatedReturn:
		if v.CorrelatedReturn != nil {
			return v.CorrelatedReturn(*ll)
		}
	case Freeze:
		if v.Freeze != nil {
			return v.Freeze(ll)
		}
	case *Freeze:
		if v.Freeze != nil {
			return v.Freeze(*ll)
		}
	case ModuleLoad:
		if v.ModuleLoad != nil {
			return v.ModuleLoad(ll)
		}
	case *ModuleLoad:
		if v.ModuleLoad != nil {
			return v.ModuleLoad(*ll)
		}
	case VariadicTransform:
		if v.VariadicTransform != nil {
			return v.VariadicTransform(ll)
		}
	case *VariadicTransform:
		if v.VariadicTransform != nil {
			return v.VariadicTransform(*ll)
		}
	case TypePredicate:
		if v.TypePredicate != nil {
			return v.TypePredicate(ll)
		}
	case *TypePredicate:
		if v.TypePredicate != nil {
			return v.TypePredicate(*ll)
		}
	case TypeValueMethod:
		if v.TypeValueMethod != nil {
			return v.TypeValueMethod(ll)
		}
	case *TypeValueMethod:
		if v.TypeValueMethod != nil {
			return v.TypeValueMethod(*ll)
		}
	case CallableType:
		if v.CallableType != nil {
			return v.CallableType(ll)
		}
	case *CallableType:
		if v.CallableType != nil {
			return v.CallableType(*ll)
		}
	}
	if v.Default != nil {
		return v.Default(l)
	}
	var zero R
	return zero
}

// TypeTransformVisitor dispatches on transform variants.
// Nil handlers fall back to Default when provided; otherwise return zero.
type TypeTransformVisitor[R any] struct {
	Unchanged             func(Unchanged) R
	ElementUnion          func(ElementUnion) R
	ContainerElementUnion func(ContainerElementUnion) R
	ToArray               func(ToArray) R
	Default               func(TypeTransform) R
}

// VisitTransform applies the first matching handler in v to t.
func VisitTransform[R any](t TypeTransform, v TypeTransformVisitor[R]) R {
	switch tt := t.(type) {
	case Unchanged:
		if v.Unchanged != nil {
			return v.Unchanged(tt)
		}
	case *Unchanged:
		if v.Unchanged != nil {
			return v.Unchanged(*tt)
		}
	case ElementUnion:
		if v.ElementUnion != nil {
			return v.ElementUnion(tt)
		}
	case *ElementUnion:
		if v.ElementUnion != nil {
			return v.ElementUnion(*tt)
		}
	case ContainerElementUnion:
		if v.ContainerElementUnion != nil {
			return v.ContainerElementUnion(tt)
		}
	case *ContainerElementUnion:
		if v.ContainerElementUnion != nil {
			return v.ContainerElementUnion(*tt)
		}
	case ToArray:
		if v.ToArray != nil {
			return v.ToArray(tt)
		}
	case *ToArray:
		if v.ToArray != nil {
			return v.ToArray(*tt)
		}
	}
	if v.Default != nil {
		return v.Default(t)
	}
	var zero R
	return zero
}

// ReturnTypeVisitor dispatches on return type variants.
// Nil handlers fall back to Default when provided; otherwise return zero.
type ReturnTypeVisitor[R any] struct {
	ElementOf             func(ElementOf) R
	OptionalElementOf     func(OptionalElementOf) R
	CallbackReturn        func(CallbackReturn) R
	ArrayOfCallbackReturn func(ArrayOfCallbackReturn) R
	SameAs                func(SameAs) R
	DeepElementOf         func(DeepElementOf) R
	StringUnpackValue     func(StringUnpackValue) R
	SelectCaseOfParam     func(SelectCaseOfParam) R
	SelectResultOfCases   func(SelectResultOfCases) R
	Default               func(ReturnType) R
}

// VisitReturnType applies the first matching handler in v to t.
func VisitReturnType[R any](t ReturnType, v ReturnTypeVisitor[R]) R {
	switch tt := t.(type) {
	case ElementOf:
		if v.ElementOf != nil {
			return v.ElementOf(tt)
		}
	case *ElementOf:
		if v.ElementOf != nil {
			return v.ElementOf(*tt)
		}
	case OptionalElementOf:
		if v.OptionalElementOf != nil {
			return v.OptionalElementOf(tt)
		}
	case *OptionalElementOf:
		if v.OptionalElementOf != nil {
			return v.OptionalElementOf(*tt)
		}
	case CallbackReturn:
		if v.CallbackReturn != nil {
			return v.CallbackReturn(tt)
		}
	case *CallbackReturn:
		if v.CallbackReturn != nil {
			return v.CallbackReturn(*tt)
		}
	case ArrayOfCallbackReturn:
		if v.ArrayOfCallbackReturn != nil {
			return v.ArrayOfCallbackReturn(tt)
		}
	case *ArrayOfCallbackReturn:
		if v.ArrayOfCallbackReturn != nil {
			return v.ArrayOfCallbackReturn(*tt)
		}
	case SameAs:
		if v.SameAs != nil {
			return v.SameAs(tt)
		}
	case *SameAs:
		if v.SameAs != nil {
			return v.SameAs(*tt)
		}
	case DeepElementOf:
		if v.DeepElementOf != nil {
			return v.DeepElementOf(tt)
		}
	case *DeepElementOf:
		if v.DeepElementOf != nil {
			return v.DeepElementOf(*tt)
		}
	case StringUnpackValue:
		if v.StringUnpackValue != nil {
			return v.StringUnpackValue(tt)
		}
	case *StringUnpackValue:
		if v.StringUnpackValue != nil {
			return v.StringUnpackValue(*tt)
		}
	case SelectCaseOfParam:
		if v.SelectCaseOfParam != nil {
			return v.SelectCaseOfParam(tt)
		}
	case *SelectCaseOfParam:
		if v.SelectCaseOfParam != nil {
			return v.SelectCaseOfParam(*tt)
		}
	case SelectResultOfCases:
		if v.SelectResultOfCases != nil {
			return v.SelectResultOfCases(tt)
		}
	case *SelectResultOfCases:
		if v.SelectResultOfCases != nil {
			return v.SelectResultOfCases(*tt)
		}
	}
	if v.Default != nil {
		return v.Default(t)
	}
	var zero R
	return zero
}
