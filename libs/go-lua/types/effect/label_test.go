package effect

import (
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
)

func TestParamRef_String(t *testing.T) {
	tests := []struct {
		ref  ParamRef
		want string
	}{
		{ParamRef{Index: 0}, "param[0]"},
		{ParamRef{Index: 1}, "param[1]"},
		{ParamRef{Index: -1}, "param[last]"},
	}

	for _, tt := range tests {
		got := tt.ref.String()
		if got != tt.want {
			t.Errorf("ParamRef{%d}.String() = %q, want %q", tt.ref.Index, got, tt.want)
		}
	}
}

func TestMutate_String(t *testing.T) {
	m := Mutate{
		Target:    ParamRef{Index: 0},
		Transform: Unchanged{},
	}

	got := m.String()
	if got != "mutate(param[0], unchanged)" {
		t.Errorf("Mutate.String() = %q", got)
	}

	mWithDelta := Mutate{
		Target:      ParamRef{Index: 0},
		Transform:   ElementUnion{Source: ParamRef{Index: 1}},
		LengthDelta: constraint.C(1),
	}

	got = mWithDelta.String()
	if got != "mutate(param[0], union_elem(param[1]), delta=1)" {
		t.Errorf("Mutate with delta.String() = %q", got)
	}
}

func TestMutate_Equals(t *testing.T) {
	m1 := Mutate{Target: ParamRef{Index: 0}, Transform: Unchanged{}}
	m2 := Mutate{Target: ParamRef{Index: 0}, Transform: Unchanged{}}
	m3 := Mutate{Target: ParamRef{Index: 1}, Transform: Unchanged{}}

	if !m1.Equals(m2) {
		t.Error("same Mutates should be equal")
	}

	if m1.Equals(m3) {
		t.Error("different target Mutates should not be equal")
	}

	if m1.Equals(Throw{}) {
		t.Error("Mutate should not equal Throw")
	}

	// Different transforms should not be equal
	m4 := Mutate{Target: ParamRef{Index: 0}, Transform: ElementUnion{Source: ParamRef{Index: 1}}}
	if m1.Equals(m4) {
		t.Error("different transform Mutates should not be equal")
	}

	// Different LengthDelta should not be equal
	m5 := Mutate{Target: ParamRef{Index: 0}, Transform: Unchanged{}, LengthDelta: constraint.C(1)}
	m6 := Mutate{Target: ParamRef{Index: 0}, Transform: Unchanged{}, LengthDelta: constraint.C(2)}

	if m5.Equals(m6) {
		t.Error("different LengthDelta Mutates should not be equal")
	}

	m7 := Mutate{Target: ParamRef{Index: 0}, Transform: Unchanged{}, LengthDelta: constraint.C(1)}
	if !m5.Equals(m7) {
		t.Error("same LengthDelta Mutates should be equal")
	}
}

func TestElementUnion_String(t *testing.T) {
	e := ElementUnion{Source: ParamRef{Index: 1}}
	if got := e.String(); got != "union_elem(param[1])" {
		t.Errorf("ElementUnion.String() = %q", got)
	}
}

func TestToArray_String(t *testing.T) {
	ta := ToArray{Element: ParamRef{Index: 0}}
	if got := ta.String(); got != "to_array(param[0])" {
		t.Errorf("ToArray.String() = %q", got)
	}
}

func TestUnchanged_String(t *testing.T) {
	u := Unchanged{}
	if got := u.String(); got != "unchanged" {
		t.Errorf("Unchanged.String() = %q", got)
	}
}

func TestReturn_String(t *testing.T) {
	r := Return{
		ReturnIndex: 0,
		Transform:   ElementOf{Source: ParamRef{Index: 0}},
	}
	if got := r.String(); got != "ret[0].type = elem(param[0])" {
		t.Errorf("Return.String() = %q", got)
	}
}

func TestReturn_Equals(t *testing.T) {
	r1 := Return{ReturnIndex: 0, Transform: ElementOf{Source: ParamRef{Index: 0}}}
	r2 := Return{ReturnIndex: 0, Transform: ElementOf{Source: ParamRef{Index: 0}}}
	r3 := Return{ReturnIndex: 1, Transform: ElementOf{Source: ParamRef{Index: 0}}}

	if !r1.Equals(r2) {
		t.Error("same Returns should be equal")
	}

	if r1.Equals(r3) {
		t.Error("different index Returns should not be equal")
	}

	if r1.Equals(Throw{}) {
		t.Error("Return should not equal Throw")
	}

	// Different transforms should not be equal
	r4 := Return{ReturnIndex: 0, Transform: SameAs{Source: ParamRef{Index: 0}}}
	if r1.Equals(r4) {
		t.Error("different transform Returns should not be equal")
	}

	r5 := Return{ReturnIndex: 0, Transform: ElementOf{Source: ParamRef{Index: 1}}}
	if r1.Equals(r5) {
		t.Error("different source in transform Returns should not be equal")
	}
}

func TestErrorReturn_String(t *testing.T) {
	er := ErrorReturn{ValueIndex: 0, ErrorIndex: 1}
	if got := er.String(); got != "errret(val[0], err[1])" {
		t.Errorf("ErrorReturn.String() = %q", got)
	}
}

func TestErrorReturn_Equals(t *testing.T) {
	e1 := ErrorReturn{ValueIndex: 0, ErrorIndex: 1}
	e2 := ErrorReturn{ValueIndex: 0, ErrorIndex: 1}
	e3 := ErrorReturn{ValueIndex: 1, ErrorIndex: 1}
	e4 := ErrorReturn{ValueIndex: 0, ErrorIndex: 2}

	if !e1.Equals(e2) {
		t.Error("same ErrorReturn should be equal")
	}
	if e1.Equals(e3) {
		t.Error("different value index should not be equal")
	}
	if e1.Equals(e4) {
		t.Error("different error index should not be equal")
	}
	if e1.Equals(Return{}) {
		t.Error("ErrorReturn should not equal Return")
	}
}

func TestReturnLength_String(t *testing.T) {
	rl := ReturnLength{
		ReturnIndex: 0,
		Length:      constraint.PL(0),
	}

	got := rl.String()
	if got != "ret[0].len = len(param[0])" {
		t.Errorf("ReturnLength.String() = %q", got)
	}
}

func TestReturnLength_Equals(t *testing.T) {
	rl1 := ReturnLength{ReturnIndex: 0, Length: constraint.PL(0)}
	rl2 := ReturnLength{ReturnIndex: 0, Length: constraint.PL(0)}
	rl3 := ReturnLength{ReturnIndex: 1, Length: constraint.PL(0)}

	if !rl1.Equals(rl2) {
		t.Error("same ReturnLengths should be equal")
	}

	if rl1.Equals(rl3) {
		t.Error("different index ReturnLengths should not be equal")
	}

	// Different lengths should not be equal
	rl4 := ReturnLength{ReturnIndex: 0, Length: constraint.PL(1)}
	if rl1.Equals(rl4) {
		t.Error("different Length ReturnLengths should not be equal")
	}

	rl5 := ReturnLength{ReturnIndex: 0, Length: constraint.C(5)}
	rl6 := ReturnLength{ReturnIndex: 0, Length: constraint.C(5)}

	if !rl5.Equals(rl6) {
		t.Error("same constant Length ReturnLengths should be equal")
	}
}

func TestElementOf_String(t *testing.T) {
	e := ElementOf{Source: ParamRef{Index: 0}}
	if got := e.String(); got != "elem(param[0])" {
		t.Errorf("ElementOf.String() = %q", got)
	}
}

func TestOptionalElementOf_String(t *testing.T) {
	oe := OptionalElementOf{Source: ParamRef{Index: 0}}
	if got := oe.String(); got != "elem(param[0]) | nil" {
		t.Errorf("OptionalElementOf.String() = %q", got)
	}
}

func TestCallbackReturn_String(t *testing.T) {
	cr := CallbackReturn{CallbackParam: ParamRef{Index: 1}}
	if got := cr.String(); got != "callback_ret(param[1])" {
		t.Errorf("CallbackReturn.String() = %q", got)
	}
}

func TestArrayOfCallbackReturn_String(t *testing.T) {
	acr := ArrayOfCallbackReturn{CallbackParam: ParamRef{Index: 1}}
	if got := acr.String(); got != "array(callback_ret(param[1]))" {
		t.Errorf("ArrayOfCallbackReturn.String() = %q", got)
	}
}

func TestSameAs_String(t *testing.T) {
	sa := SameAs{Source: ParamRef{Index: 0}}
	if got := sa.String(); got != "same(param[0])" {
		t.Errorf("SameAs.String() = %q", got)
	}
}

func TestDeepElementOf_String(t *testing.T) {
	de := DeepElementOf{Source: ParamRef{Index: 0}}
	if got := de.String(); got != "deep_elem(param[0])" {
		t.Errorf("DeepElementOf.String() = %q", got)
	}
}

func TestStringUnpackValue_String(t *testing.T) {
	u := StringUnpackValue{Format: ParamRef{Index: 0}}
	if got := u.String(); got != "string_unpack(param[0])" {
		t.Errorf("StringUnpackValue.String() = %q", got)
	}
}

func TestThrow(t *testing.T) {
	th := Throw{}
	if got := th.String(); got != "throw" {
		t.Errorf("Throw.String() = %q", got)
	}

	if !th.Equals(Throw{}) {
		t.Error("Throw should equal Throw")
	}

	if th.Equals(IO{}) {
		t.Error("Throw should not equal IO")
	}
}

func TestDiverge(t *testing.T) {
	d := Diverge{}
	if got := d.String(); got != "diverge" {
		t.Errorf("Diverge.String() = %q", got)
	}

	if !d.Equals(Diverge{}) {
		t.Error("Diverge should equal Diverge")
	}

	if d.Equals(Throw{}) {
		t.Error("Diverge should not equal Throw")
	}
}

func TestIO(t *testing.T) {
	io := IO{}
	if got := io.String(); got != "io" {
		t.Errorf("IO.String() = %q", got)
	}

	if !io.Equals(IO{}) {
		t.Error("IO should equal IO")
	}

	if io.Equals(Throw{}) {
		t.Error("IO should not equal Throw")
	}
}

func TestLengthChange_String(t *testing.T) {
	lc := LengthChange{Target: ParamRef{Index: 0}, Delta: 1}
	if got := lc.String(); got != "len(param[0]) += 1" {
		t.Errorf("LengthChange positive.String() = %q", got)
	}

	lcNeg := LengthChange{Target: ParamRef{Index: 0}, Delta: -1}
	if got := lcNeg.String(); got != "len(param[0]) -= 1" {
		t.Errorf("LengthChange negative.String() = %q", got)
	}
}

func TestLengthChange_Equals(t *testing.T) {
	lc1 := LengthChange{Target: ParamRef{Index: 0}, Delta: 1}
	lc2 := LengthChange{Target: ParamRef{Index: 0}, Delta: 1}
	lc3 := LengthChange{Target: ParamRef{Index: 1}, Delta: 1}

	if !lc1.Equals(lc2) {
		t.Error("same LengthChanges should be equal")
	}

	if lc1.Equals(lc3) {
		t.Error("different target LengthChanges should not be equal")
	}

	// Different deltas should not be equal
	lc4 := LengthChange{Target: ParamRef{Index: 0}, Delta: 2}
	if lc1.Equals(lc4) {
		t.Error("different Delta LengthChanges should not be equal")
	}

	lc5 := LengthChange{Target: ParamRef{Index: 0}, Delta: -1}
	if lc1.Equals(lc5) {
		t.Error("positive and negative Delta should not be equal")
	}
}

func TestLabelInterface(t *testing.T) {
	labels := []Label{
		Mutate{},
		Return{},
		ErrorReturn{},
		ReturnLength{},
		Throw{},
		Diverge{},
		IO{},
		LengthChange{},
	}

	for _, l := range labels {
		_ = l.String()
		_ = l.Equals(l)
	}
}

func TestTransformInterface(t *testing.T) {
	transforms := []TypeTransform{
		ElementUnion{},
		ToArray{},
		Unchanged{},
	}

	for _, tr := range transforms {
		_ = tr.String()
	}
}

func TestReturnTypeInterface(t *testing.T) {
	returnTypes := []ReturnType{
		ElementOf{},
		OptionalElementOf{},
		CallbackReturn{},
		ArrayOfCallbackReturn{},
		SameAs{},
		DeepElementOf{},
		StringUnpackValue{},
	}

	for _, rt := range returnTypes {
		_ = rt.String()
	}
}

func TestIterator(t *testing.T) {
	iter := Iterator{Source: ParamRef{Index: 0}, Kind: IterateIndexed}
	if got := iter.String(); got != "iterator(param[0], indexed)" {
		t.Errorf("Iterator indexed.String() = %q", got)
	}

	iterKeyed := Iterator{Source: ParamRef{Index: 0}, Kind: IterateKeyed}
	if got := iterKeyed.String(); got != "iterator(param[0], keyed)" {
		t.Errorf("Iterator keyed.String() = %q", got)
	}

	// Equals
	if !iter.Equals(Iterator{Source: ParamRef{Index: 0}, Kind: IterateIndexed}) {
		t.Error("same Iterator should be equal")
	}

	if iter.Equals(Iterator{Source: ParamRef{Index: 1}, Kind: IterateIndexed}) {
		t.Error("different source should not be equal")
	}

	if iter.Equals(Iterator{Source: ParamRef{Index: 0}, Kind: IterateKeyed}) {
		t.Error("different kind should not be equal")
	}

	if iter.Equals(Throw{}) {
		t.Error("Iterator should not equal Throw")
	}
}

func TestTableMutator(t *testing.T) {
	tm := TableMutator{Target: ParamRef{Index: 0}, Value: ParamRef{Index: 1}}
	if got := tm.String(); got != "table_mutator(param[0], param[1])" {
		t.Errorf("TableMutator.String() = %q", got)
	}

	// Equals
	if !tm.Equals(TableMutator{Target: ParamRef{Index: 0}, Value: ParamRef{Index: 1}}) {
		t.Error("same TableMutator should be equal")
	}

	if tm.Equals(TableMutator{Target: ParamRef{Index: 1}, Value: ParamRef{Index: 1}}) {
		t.Error("different target should not be equal")
	}

	if tm.Equals(TableMutator{Target: ParamRef{Index: 0}, Value: ParamRef{Index: 2}}) {
		t.Error("different value should not be equal")
	}

	if tm.Equals(Throw{}) {
		t.Error("TableMutator should not equal Throw")
	}
}

func TestBorrow(t *testing.T) {
	b := Borrow{Param: ParamRef{Index: 0}}
	if got := b.String(); got != "borrow(param[0])" {
		t.Errorf("Borrow.String() = %q", got)
	}

	// Equals
	if !b.Equals(Borrow{Param: ParamRef{Index: 0}}) {
		t.Error("same Borrow should be equal")
	}

	if b.Equals(Borrow{Param: ParamRef{Index: 1}}) {
		t.Error("different param Borrow should not be equal")
	}

	if b.Equals(Throw{}) {
		t.Error("Borrow should not equal Throw")
	}
}

func TestStore(t *testing.T) {
	s := Store{Param: ParamRef{Index: 0}, Into: ParamRef{Index: 1}}
	if got := s.String(); got != "store(param[0] into param[1])" {
		t.Errorf("Store with into.String() = %q", got)
	}

	sUnknown := Store{Param: ParamRef{Index: 0}, Into: ParamRef{Index: -1}}
	if got := sUnknown.String(); got != "store(param[0])" {
		t.Errorf("Store unknown into.String() = %q", got)
	}

	// Equals
	if !s.Equals(Store{Param: ParamRef{Index: 0}, Into: ParamRef{Index: 1}}) {
		t.Error("same Store should be equal")
	}

	if s.Equals(Store{Param: ParamRef{Index: 1}, Into: ParamRef{Index: 1}}) {
		t.Error("different param Store should not be equal")
	}

	if s.Equals(Throw{}) {
		t.Error("Store should not equal Throw")
	}

	// Different Into should not be equal
	s2 := Store{Param: ParamRef{Index: 0}, Into: ParamRef{Index: 2}}
	if s.Equals(s2) {
		t.Error("different Into Store should not be equal")
	}
	// Same param but different Into (one known, one unknown)
	if s.Equals(sUnknown) {
		t.Error("different Into (known vs unknown) Store should not be equal")
	}
}

func TestBorrowAll(t *testing.T) {
	ba := BorrowAll{}
	if got := ba.String(); got != "borrow_all" {
		t.Errorf("BorrowAll.String() = %q", got)
	}

	if !ba.Equals(BorrowAll{}) {
		t.Error("BorrowAll should equal BorrowAll")
	}

	if ba.Equals(Throw{}) {
		t.Error("BorrowAll should not equal Throw")
	}
}

func TestAllLabelsImplementInterface(t *testing.T) {
	labels := []Label{
		Mutate{},
		Return{},
		ErrorReturn{},
		ReturnLength{},
		Throw{},
		Diverge{},
		IO{},
		LengthChange{},
		Iterator{},
		TableMutator{},
		Borrow{},
		Store{},
		BorrowAll{},
	}

	for _, l := range labels {
		_ = l.String()
		_ = l.Equals(l)
	}
}

func TestMarkerMethods(t *testing.T) {
	// Test label() marker methods
	Mutate{}.label()
	Return{}.label()
	ErrorReturn{}.label()
	ReturnLength{}.label()
	Throw{}.label()
	Diverge{}.label()
	IO{}.label()
	LengthChange{}.label()
	Iterator{}.label()
	TableMutator{}.label()
	Borrow{}.label()
	Store{}.label()
	BorrowAll{}.label()

	// Test transform() marker methods
	ElementUnion{}.transform()
	ToArray{}.transform()
	Unchanged{}.transform()

	// Test returnType() marker methods
	ElementOf{}.returnType()
	OptionalElementOf{}.returnType()
	CallbackReturn{}.returnType()
	ArrayOfCallbackReturn{}.returnType()
	SameAs{}.returnType()
	DeepElementOf{}.returnType()
	StringUnpackValue{}.returnType()
}

func TestReturnLengthEqualsNonMatch(t *testing.T) {
	rl := ReturnLength{ReturnIndex: 0}
	if rl.Equals(Throw{}) {
		t.Error("ReturnLength should not equal Throw")
	}
}

func TestSend(t *testing.T) {
	s := Send{FromParam: 2}
	if got := s.String(); got != "send(params[2:])" {
		t.Errorf("Send.String() = %q", got)
	}

	if !s.Equals(Send{FromParam: 2}) {
		t.Error("same Send should be equal")
	}

	if s.Equals(Send{FromParam: 3}) {
		t.Error("different FromParam should not be equal")
	}

	if s.Equals(Throw{}) {
		t.Error("Send should not equal Throw")
	}
}

func TestFreeze(t *testing.T) {
	f := Freeze{Param: ParamRef{Index: 0}}
	if got := f.String(); got != "freeze(param[0])" {
		t.Errorf("Freeze.String() = %q", got)
	}

	if !f.Equals(Freeze{Param: ParamRef{Index: 0}}) {
		t.Error("same Freeze should be equal")
	}

	if f.Equals(Freeze{Param: ParamRef{Index: 1}}) {
		t.Error("different Param should not be equal")
	}

	if f.Equals(Throw{}) {
		t.Error("Freeze should not equal Throw")
	}
}

func TestCorrelatedReturn(t *testing.T) {
	cr := CorrelatedReturn{Indices: []int{0, 1, 2}}
	if got := cr.String(); got != "correlated_return([0 1 2])" {
		t.Errorf("CorrelatedReturn.String() = %q", got)
	}

	if !cr.Equals(CorrelatedReturn{Indices: []int{0, 1, 2}}) {
		t.Error("same CorrelatedReturn should be equal")
	}

	if cr.Equals(CorrelatedReturn{Indices: []int{0, 1}}) {
		t.Error("different length should not be equal")
	}

	if cr.Equals(CorrelatedReturn{Indices: []int{0, 1, 3}}) {
		t.Error("different indices should not be equal")
	}

	if cr.Equals(Throw{}) {
		t.Error("CorrelatedReturn should not equal Throw")
	}
}

func TestModuleLoad(t *testing.T) {
	ml := ModuleLoad{}
	if got := ml.String(); got != "module_load" {
		t.Errorf("ModuleLoad.String() = %q", got)
	}

	if !ml.Equals(ModuleLoad{}) {
		t.Error("ModuleLoad should equal ModuleLoad")
	}

	if ml.Equals(Throw{}) {
		t.Error("ModuleLoad should not equal Throw")
	}
}

func TestVariadicTransform(t *testing.T) {
	vt := VariadicTransform{}
	if got := vt.String(); got != "variadic_transform" {
		t.Errorf("VariadicTransform.String() = %q", got)
	}

	if !vt.Equals(VariadicTransform{}) {
		t.Error("VariadicTransform should equal VariadicTransform")
	}

	if vt.Equals(Throw{}) {
		t.Error("VariadicTransform should not equal Throw")
	}
}

func TestTypePredicate(t *testing.T) {
	tp := TypePredicate{}
	if got := tp.String(); got != "type_predicate" {
		t.Errorf("TypePredicate.String() = %q", got)
	}

	if !tp.Equals(TypePredicate{}) {
		t.Error("TypePredicate should equal TypePredicate")
	}

	if tp.Equals(Throw{}) {
		t.Error("TypePredicate should not equal Throw")
	}
}

func TestTypeValueMethod(t *testing.T) {
	tvm := TypeValueMethod{}
	if got := tvm.String(); got != "type_value_method" {
		t.Errorf("TypeValueMethod.String() = %q", got)
	}

	if !tvm.Equals(TypeValueMethod{}) {
		t.Error("TypeValueMethod should equal TypeValueMethod")
	}

	if tvm.Equals(Throw{}) {
		t.Error("TypeValueMethod should not equal Throw")
	}
}

func TestCallableType(t *testing.T) {
	ct := CallableType{}
	if got := ct.String(); got != "callable_type" {
		t.Errorf("CallableType.String() = %q", got)
	}

	if !ct.Equals(CallableType{}) {
		t.Error("CallableType should equal CallableType")
	}

	if ct.Equals(Throw{}) {
		t.Error("CallableType should not equal Throw")
	}
}

func TestContainerElementUnion_String(t *testing.T) {
	c := ContainerElementUnion{Container: ParamRef{Index: 0}, Value: ParamRef{Index: 1}}
	if got := c.String(); got != "union_elem(param[0], param[1])" {
		t.Errorf("ContainerElementUnion.String() = %q", got)
	}
}

func TestSelectCaseOfParam_String(t *testing.T) {
	s := SelectCaseOfParam{Source: ParamRef{Index: 0}}
	if got := s.String(); got != "select_case(param[0])" {
		t.Errorf("SelectCaseOfParam.String() = %q", got)
	}
}

func TestSelectResultOfCases_String(t *testing.T) {
	s := SelectResultOfCases{Cases: ParamRef{Index: 0}, Default: ParamRef{Index: 1}}
	if got := s.String(); got != "select_result(param[0], param[1])" {
		t.Errorf("SelectResultOfCases.String() = %q", got)
	}
}
