package effect

import "testing"

func TestEmpty(t *testing.T) {
	if !Empty.Pure() {
		t.Error("Empty should be pure")
	}

	if !Empty.IsClosed() {
		t.Error("Empty should be closed")
	}

	if Empty.String() != "{}" {
		t.Errorf("Empty.String() = %q, want {}", Empty.String())
	}
}

func TestUnknown(t *testing.T) {
	if Unknown.Pure() {
		t.Error("Unknown should not be pure")
	}

	if !Unknown.IsOpen() {
		t.Error("Unknown should be open")
	}

	if !Unknown.IsUnknown() {
		t.Error("Unknown should be unknown")
	}
}

func TestRowWith(t *testing.T) {
	r := Empty.With(Throw{}, IO{})
	if !r.HasThrow() {
		t.Error("Should have throw")
	}

	if !r.HasIO() {
		t.Error("Should have io")
	}

	if r.Pure() {
		t.Error("Should not be pure")
	}
}

func TestRowWithDuplicate(t *testing.T) {
	r := Empty.With(Throw{}, Throw{})
	count := 0

	for _, l := range r.Labels {
		if _, ok := l.(Throw); ok {
			count++
		}
	}

	if count != 1 {
		t.Errorf("Should have exactly 1 throw, got %d", count)
	}
}

func TestRowString(t *testing.T) {
	tests := []struct {
		row  Row
		want string
	}{
		{Empty, "{}"},
		{Throws(), "{throw}"},
		{Row{Labels: []Label{Throw{}, IO{}}}, "{throw, io}"},
		{Open("ρ", Throw{}), "{throw | ρ}"},
		{Open("ρ"), "{ρ}"},
	}

	for _, tt := range tests {
		if got := tt.row.String(); got != tt.want {
			t.Errorf("Row.String() = %q, want %q", got, tt.want)
		}
	}
}

func TestUnion(t *testing.T) {
	t.Run("closed rows", func(t *testing.T) {
		r1 := Row{Labels: []Label{Throw{}}}
		r2 := Row{Labels: []Label{IO{}}}
		u := Union(r1, r2)

		if !u.HasThrow() || !u.HasIO() {
			t.Error("Union should have both throw and io")
		}

		if !u.IsClosed() {
			t.Error("Union of closed rows should be closed")
		}
	})

	t.Run("open row", func(t *testing.T) {
		r1 := Open("ρ", Throw{})
		r2 := Row{Labels: []Label{IO{}}}
		u := Union(r1, r2)

		if !u.HasThrow() || !u.HasIO() {
			t.Error("Union should have both effects")
		}

		if !u.IsOpen() {
			t.Error("Union with open row should be open")
		}
	})

	t.Run("unknown absorbs", func(t *testing.T) {
		r := Row{Labels: []Label{Throw{}}}
		u := Union(r, Unknown)

		if !u.IsUnknown() {
			t.Error("Union with Unknown should be Unknown")
		}
	})

	t.Run("deduplication", func(t *testing.T) {
		r1 := Row{Labels: []Label{Throw{}}}
		r2 := Row{Labels: []Label{Throw{}, IO{}}}
		u := Union(r1, r2)

		count := 0

		for _, l := range u.Labels {
			if _, ok := l.(Throw); ok {
				count++
			}
		}

		if count != 1 {
			t.Errorf("Union should deduplicate, got %d throws", count)
		}
	})
}

func TestMutateEffect(t *testing.T) {
	m := Mutates(0, ElementUnion{Source: ParamRef{Index: 1}})

	if !m.HasMutate() {
		t.Error("Should have mutation")
	}

	got := m.GetMutate(0)
	if got == nil {
		t.Error("Should find mutation for param 0")
	}

	if m.GetMutate(1) != nil {
		t.Error("Should not find mutation for param 1")
	}
}

func TestReturnEffect(t *testing.T) {
	r := Returns(0, ElementOf{Source: ParamRef{Index: 0}})

	got := r.GetReturn(0)
	if got == nil {
		t.Error("Should find return for index 0")
	}

	if r.GetReturn(1) != nil {
		t.Error("Should not find return for index 1")
	}
}

func TestRowEquals(t *testing.T) {
	tests := []struct {
		r1, r2 Row
		want   bool
	}{
		{Empty, Empty, true},
		{Empty, Throws(), false},
		{Throws(), Throws(), true},
		{Row{Labels: []Label{Throw{}, IO{}}}, Row{Labels: []Label{IO{}, Throw{}}}, true},
		{Open("ρ"), Open("ρ"), true},
		{Open("ρ"), Open("σ"), false},
		{Throws(), Open("ρ", Throw{}), false},
	}

	for _, tt := range tests {
		got := tt.r1.Equals(tt.r2)
		if got != tt.want {
			t.Errorf("%v.Equals(%v) = %v, want %v", tt.r1, tt.r2, got, tt.want)
		}
	}
}

func TestReads(t *testing.T) {
	r := Empty
	if !r.Pure() {
		t.Error("Reads should be pure (empty)")
	}

	if r.String() != "{}" {
		t.Errorf("Empty.String() = %q, want {}", r.String())
	}
}

func TestWithIO(t *testing.T) {
	r := WithIO()
	if !r.HasIO() {
		t.Error("WithIO should have IO effect")
	}

	if r.Pure() {
		t.Error("WithIO should not be pure")
	}
}

func TestMayDiverge(t *testing.T) {
	r := MayDiverge()
	if !r.HasDiverge() {
		t.Error("MayDiverge should have Diverge effect")
	}

	if r.Pure() {
		t.Error("MayDiverge should not be pure")
	}
}

func TestRowWithout(t *testing.T) {
	r := Row{Labels: []Label{Throw{}, IO{}, Diverge{}}}
	filtered := r.Without(func(l Label) bool {
		_, ok := l.(IO)
		return ok
	})

	if filtered.HasIO() {
		t.Error("Without should remove IO")
	}

	if !filtered.HasThrow() {
		t.Error("Without should keep Throw")
	}

	if !filtered.HasDiverge() {
		t.Error("Without should keep Diverge")
	}
}

func TestVarString(t *testing.T) {
	v := &Var{Name: "rho"}
	if got := v.String(); got != "rho" {
		t.Errorf("Var.String() = %q, want 'rho'", got)
	}

	var nilVar *Var
	if got := nilVar.String(); got != "" {
		t.Errorf("nil Var.String() = %q, want ''", got)
	}
}

func TestRowStringOpenNoLabels(t *testing.T) {
	r := Row{Tail: &Var{Name: "rho"}}
	if got := r.String(); got != "{rho}" {
		t.Errorf("Open row no labels.String() = %q, want '{rho}'", got)
	}
}

func TestIteratorEffects(t *testing.T) {
	r := Row{Labels: []Label{Iterator{Source: ParamRef{Index: 0}, Kind: IterateIndexed}}}

	if !r.HasIterator() {
		t.Error("Should have iterator")
	}

	iter := r.GetIterator()
	if iter == nil {
		t.Error("Should find iterator")
	}

	if !r.IsIndexedIterator() {
		t.Error("Should be indexed iterator")
	}

	if r.IsKeyedIterator() {
		t.Error("Should not be keyed iterator")
	}

	// Keyed iterator
	r2 := Row{Labels: []Label{Iterator{Source: ParamRef{Index: 0}, Kind: IterateKeyed}}}
	if !r2.IsKeyedIterator() {
		t.Error("Should be keyed iterator")
	}

	if r2.IsIndexedIterator() {
		t.Error("Should not be indexed iterator")
	}

	// No iterator
	r3 := Empty
	if r3.IsIndexedIterator() || r3.IsKeyedIterator() {
		t.Error("Empty row should not be any iterator")
	}
}

func TestTableMutatorEffects(t *testing.T) {
	r := Row{Labels: []Label{TableMutator{Target: ParamRef{Index: 0}, Value: ParamRef{Index: 1}}}}

	if !r.HasTableMutator() {
		t.Error("Should have table mutator")
	}

	mut := r.GetTableMutator()
	if mut == nil {
		t.Error("Should find table mutator")
	}

	// No mutator
	if Empty.GetTableMutator() != nil {
		t.Error("Empty row should not have table mutator")
	}
}

func TestReturnLengthEffect(t *testing.T) {
	r := Row{Labels: []Label{ReturnLength{ReturnIndex: 0, Length: nil}}}

	rl := r.GetReturnLength(0)
	if rl == nil {
		t.Error("Should find return length")
	}

	if r.GetReturnLength(1) != nil {
		t.Error("Should not find return length for wrong index")
	}
}

func TestErrorReturnEffect(t *testing.T) {
	r := Row{Labels: []Label{ErrorReturn{ValueIndex: 0, ErrorIndex: 1}}}

	er := r.GetErrorReturn(0)
	if er == nil {
		t.Error("Should find error return")
	}

	if r.GetErrorReturn(1) != nil {
		t.Error("Should not find error return for wrong value index")
	}
}

func TestBorrowStoreEffects(t *testing.T) {
	// BorrowsOnly
	r := BorrowsOnly()
	if !r.HasBorrow() {
		t.Error("BorrowsOnly should have borrow")
	}

	if !r.BorrowsAllParams() {
		t.Error("BorrowsOnly should borrow all params")
	}

	// Borrow param
	r2 := Row{Labels: []Label{Borrow{Param: ParamRef{Index: 0}}}}
	if !r2.HasBorrow() {
		t.Error("Borrow should have borrow")
	}

	b := r2.GetBorrow(0)
	if b == nil {
		t.Error("Should find borrow for param 0")
	}

	if r2.GetBorrow(1) != nil {
		t.Error("Should not find borrow for param 1")
	}

	// StoresParam
	r3 := StoresParam(0, 1)
	if !r3.HasStore() {
		t.Error("StoresParam should have store")
	}

	s := r3.GetStore(0)
	if s == nil {
		t.Error("Should find store for param 0")
	}

	if r3.GetStore(1) != nil {
		t.Error("Should not find store for param 1")
	}

	// OnlyBorrows
	if !r2.OnlyBorrows() {
		t.Error("BorrowsParam should only borrow")
	}

	if r3.OnlyBorrows() {
		t.Error("StoresParam should not only borrow")
	}

	if Empty.OnlyBorrows() {
		t.Error("Empty should not only borrow")
	}

	// Mutate with borrow
	r4 := Row{Labels: []Label{Borrow{Param: ParamRef{Index: 0}}, Mutate{Target: ParamRef{Index: 0}}}}
	if r4.OnlyBorrows() {
		t.Error("Borrow+Mutate should not be only borrows")
	}
}

func TestNewRow(t *testing.T) {
	r := Row{Labels: []Label{Throw{}, IO{}}}
	if !r.HasThrow() || !r.HasIO() {
		t.Error("Row should have the given labels")
	}
}

func TestGetCorrelatedReturn(t *testing.T) {
	r := Row{Labels: []Label{CorrelatedReturn{Indices: []int{0, 1, 2}}}}

	cr := r.GetCorrelatedReturn(1)
	if cr == nil {
		t.Error("Should find correlated return for index 1")
	}

	if r.GetCorrelatedReturn(5) != nil {
		t.Error("Should not find correlated return for index 5")
	}

	if Empty.GetCorrelatedReturn(0) != nil {
		t.Error("Empty row should not have correlated return")
	}
}

func TestSemanticEffectConstructors(t *testing.T) {
	t.Run("WithModuleLoad", func(t *testing.T) {
		r := WithModuleLoad()
		if !r.HasModuleLoad() {
			t.Error("WithModuleLoad should have module load effect")
		}
	})

	t.Run("WithVariadicTransform", func(t *testing.T) {
		r := WithVariadicTransform()
		if !r.HasVariadicTransform() {
			t.Error("WithVariadicTransform should have variadic transform effect")
		}
	})

	t.Run("WithTypePredicate", func(t *testing.T) {
		r := WithTypePredicate()
		if !r.HasTypePredicate() {
			t.Error("WithTypePredicate should have type predicate effect")
		}
	})

	t.Run("WithTypeValueMethod", func(t *testing.T) {
		r := WithTypeValueMethod()
		if !r.HasTypeValueMethod() {
			t.Error("WithTypeValueMethod should have type value method effect")
		}
	})

	t.Run("WithCallableType", func(t *testing.T) {
		r := WithCallableType()
		if !r.HasCallableType() {
			t.Error("WithCallableType should have callable type effect")
		}
	})
}

func TestRowEqualsNonRow(t *testing.T) {
	r := Empty
	if r.Equals("not a row") {
		t.Error("Row should not equal non-Row type")
	}
}
