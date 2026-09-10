package constraint

import (
	"testing"
)

func TestVar(t *testing.T) {
	x := V("x")
	if x.String() != "x" {
		t.Errorf("expected x, got %s", x.String())
	}

	if vars := x.Variables(); len(vars) != 1 || vars[0] != "x" {
		t.Errorf("expected [x], got %v", vars)
	}
}

func TestConst(t *testing.T) {
	c := C(42)
	if c.String() != "42" {
		t.Errorf("expected 42, got %s", c.String())
	}

	val, ok := c.Eval(nil)
	if !ok || val != 42 {
		t.Errorf("expected (42, true), got (%d, %v)", val, ok)
	}
}

func TestBinOp(t *testing.T) {
	tests := []struct {
		expr     Expr
		env      map[string]int64
		expected int64
	}{
		{Add(C(2), C(3)), nil, 5},
		{Sub(C(10), C(3)), nil, 7},
		{Mul(C(4), C(5)), nil, 20},
		{Div(C(15), C(3)), nil, 5},
		{Mod(C(17), C(5)), nil, 2},
		{Add(V("x"), C(1)), map[string]int64{"x": 5}, 6},
		{Sub(V("x"), V("y")), map[string]int64{"x": 10, "y": 3}, 7},
	}

	for _, tt := range tests {
		val, ok := tt.expr.Eval(tt.env)
		if !ok {
			t.Errorf("%s: expected ok, got not ok", tt.expr)
			continue
		}

		if val != tt.expected {
			t.Errorf("%s: expected %d, got %d", tt.expr, tt.expected, val)
		}
	}
}

func TestLen(t *testing.T) {
	l := L("arr")
	if l.String() != "len(arr)" {
		t.Errorf("expected len(arr), got %s", l.String())
	}

	env := map[string]int64{"arr.len": 5}

	val, ok := l.Eval(env)
	if !ok || val != 5 {
		t.Errorf("expected (5, true), got (%d, %v)", val, ok)
	}

	// Unknown length
	_, ok = l.Eval(nil)
	if ok {
		t.Error("expected not ok for unknown length")
	}
}

func TestSubstitute(t *testing.T) {
	// x + y with x -> 5
	expr := Add(V("x"), V("y"))
	subst := map[string]Expr{"x": C(5)}
	result := expr.Substitute(subst)

	// Should be 5 + y
	env := map[string]int64{"y": 3}

	val, ok := result.Eval(env)
	if !ok || val != 8 {
		t.Errorf("expected (8, true), got (%d, %v)", val, ok)
	}
}

func TestSimplify(t *testing.T) {
	tests := []struct {
		expr     Expr
		expected string
	}{
		{Add(C(2), C(3)), "5"},
		{Add(V("x"), C(0)), "x"},
		{Add(C(0), V("x")), "x"},
		{Sub(V("x"), C(0)), "x"},
		{Mul(V("x"), C(1)), "x"},
		{Mul(C(1), V("x")), "x"},
		{Mul(V("x"), C(0)), "0"},
		{Mul(C(0), V("x")), "0"},
	}

	for _, tt := range tests {
		result := Simplify(tt.expr)
		if result.String() != tt.expected {
			t.Errorf("Simplify(%s): expected %s, got %s", tt.expr, tt.expected, result)
		}
	}
}

func TestVariables(t *testing.T) {
	expr := Add(Mul(V("x"), V("y")), Sub(V("z"), V("x")))
	vars := expr.Variables()

	varSet := make(map[string]bool)
	for _, v := range vars {
		varSet[v] = true
	}

	if !varSet["x"] || !varSet["y"] || !varSet["z"] {
		t.Errorf("expected {x, y, z}, got %v", vars)
	}
}

func TestParam(t *testing.T) {
	tests := []struct {
		name     string
		param    Param
		expected string
		varName  string
	}{
		{"param 0", P(0), "param[0]", "param[0]"},
		{"param 1", P(1), "param[1]", "param[1]"},
		{"param 5", P(5), "param[5]", "param[5]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.param.String(); got != tt.expected {
				t.Errorf("String() = %s, want %s", got, tt.expected)
			}

			vars := tt.param.Variables()
			if len(vars) != 1 || vars[0] != tt.varName {
				t.Errorf("Variables() = %v, want [%s]", vars, tt.varName)
			}
		})
	}
}

func TestParamRetEval(t *testing.T) {
	tests := []struct {
		name     string
		expr     Expr
		env      map[string]int64
		expected int64
		ok       bool
	}{
		{"param[0] found", P(0), map[string]int64{"param[0]": 42}, 42, true},
		{"param[1] found", P(1), map[string]int64{"param[1]": 100}, 100, true},
		{"param[0] not found", P(0), nil, 0, false},
		{"param[0] not found in env", P(0), map[string]int64{"x": 5}, 0, false},
		{"ret[0] found", R(0), map[string]int64{"ret[0]": 99}, 99, true},
		{"ret[1] found", R(1), map[string]int64{"ret[1]": 55}, 55, true},
		{"ret[0] not found", R(0), nil, 0, false},
		{"ret[0] not found in env", R(0), map[string]int64{"x": 5}, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, ok := tt.expr.Eval(tt.env)
			if ok != tt.ok {
				t.Errorf("Eval() ok = %v, want %v", ok, tt.ok)
			}
			if ok && val != tt.expected {
				t.Errorf("Eval() val = %d, want %d", val, tt.expected)
			}
		})
	}
}

func TestParamSubstitute(t *testing.T) {
	tests := []struct {
		name     string
		param    Param
		subst    map[string]Expr
		expected Expr
	}{
		{"substitute param[0]", P(0), map[string]Expr{"param[0]": C(42)}, C(42)},
		{"no substitution", P(0), map[string]Expr{"x": C(10)}, P(0)},
		{"substitute param[1]", P(1), map[string]Expr{"param[1]": V("x")}, V("x")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.param.Substitute(tt.subst)
			if result.String() != tt.expected.String() {
				t.Errorf("Substitute() = %s, want %s", result, tt.expected)
			}
		})
	}
}

func TestRet(t *testing.T) {
	tests := []struct {
		name     string
		ret      Ret
		expected string
		varName  string
	}{
		{"ret 0", R(0), "ret[0]", "ret[0]"},
		{"ret 1", R(1), "ret[1]", "ret[1]"},
		{"ret 2", R(2), "ret[2]", "ret[2]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ret.String(); got != tt.expected {
				t.Errorf("String() = %s, want %s", got, tt.expected)
			}

			vars := tt.ret.Variables()
			if len(vars) != 1 || vars[0] != tt.varName {
				t.Errorf("Variables() = %v, want [%s]", vars, tt.varName)
			}
		})
	}
}

func TestRetSubstitute(t *testing.T) {
	tests := []struct {
		name     string
		ret      Ret
		subst    map[string]Expr
		expected Expr
	}{
		{"substitute ret[0]", R(0), map[string]Expr{"ret[0]": C(33)}, C(33)},
		{"no substitution", R(0), map[string]Expr{"x": C(10)}, R(0)},
		{"substitute ret[1]", R(1), map[string]Expr{"ret[1]": V("y")}, V("y")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.ret.Substitute(tt.subst)
			if result.String() != tt.expected.String() {
				t.Errorf("Substitute() = %s, want %s", result, tt.expected)
			}
		})
	}
}

func TestParamLen(t *testing.T) {
	tests := []struct {
		name     string
		plen     ParamLen
		expected string
		varName  string
	}{
		{"param[0] len", PL(0), "len(param[0])", "param[0].len"},
		{"param[1] len", PL(1), "len(param[1])", "param[1].len"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.plen.String(); got != tt.expected {
				t.Errorf("String() = %s, want %s", got, tt.expected)
			}

			vars := tt.plen.Variables()
			if len(vars) != 1 || vars[0] != tt.varName {
				t.Errorf("Variables() = %v, want [%s]", vars, tt.varName)
			}
		})
	}
}

func TestParamLenEval(t *testing.T) {
	tests := []struct {
		name     string
		plen     ParamLen
		env      map[string]int64
		expected int64
		ok       bool
	}{
		{"param[0].len found", PL(0), map[string]int64{"param[0].len": 10}, 10, true},
		{"param[1].len found", PL(1), map[string]int64{"param[1].len": 25}, 25, true},
		{"param[0].len not found", PL(0), nil, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, ok := tt.plen.Eval(tt.env)
			if ok != tt.ok {
				t.Errorf("Eval() ok = %v, want %v", ok, tt.ok)
			}

			if ok && val != tt.expected {
				t.Errorf("Eval() val = %d, want %d", val, tt.expected)
			}
		})
	}
}

func TestParamLenSubstitute(t *testing.T) {
	tests := []struct {
		name     string
		plen     ParamLen
		subst    map[string]Expr
		expected Expr
	}{
		{"substitute param[0].len", PL(0), map[string]Expr{"param[0].len": C(10)}, C(10)},
		{"no substitution", PL(0), map[string]Expr{"x": C(5)}, PL(0)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.plen.Substitute(tt.subst)
			if result.String() != tt.expected.String() {
				t.Errorf("Substitute() = %s, want %s", result, tt.expected)
			}
		})
	}
}

func TestRetLen(t *testing.T) {
	tests := []struct {
		name     string
		rlen     RetLen
		expected string
		varName  string
	}{
		{"ret[0] len", RL(0), "len(ret[0])", "ret[0].len"},
		{"ret[1] len", RL(1), "len(ret[1])", "ret[1].len"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.rlen.String(); got != tt.expected {
				t.Errorf("String() = %s, want %s", got, tt.expected)
			}

			vars := tt.rlen.Variables()
			if len(vars) != 1 || vars[0] != tt.varName {
				t.Errorf("Variables() = %v, want [%s]", vars, tt.varName)
			}
		})
	}
}

func TestRetLenEval(t *testing.T) {
	tests := []struct {
		name     string
		rlen     RetLen
		env      map[string]int64
		expected int64
		ok       bool
	}{
		{"ret[0].len found", RL(0), map[string]int64{"ret[0].len": 15}, 15, true},
		{"ret[1].len found", RL(1), map[string]int64{"ret[1].len": 30}, 30, true},
		{"ret[0].len not found", RL(0), nil, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, ok := tt.rlen.Eval(tt.env)
			if ok != tt.ok {
				t.Errorf("Eval() ok = %v, want %v", ok, tt.ok)
			}

			if ok && val != tt.expected {
				t.Errorf("Eval() val = %d, want %d", val, tt.expected)
			}
		})
	}
}

func TestRetLenSubstitute(t *testing.T) {
	tests := []struct {
		name     string
		rlen     RetLen
		subst    map[string]Expr
		expected Expr
	}{
		{"substitute ret[0].len", RL(0), map[string]Expr{"ret[0].len": C(20)}, C(20)},
		{"no substitution", RL(0), map[string]Expr{"x": C(5)}, RL(0)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.rlen.Substitute(tt.subst)
			if result.String() != tt.expected.String() {
				t.Errorf("Substitute() = %s, want %s", result, tt.expected)
			}
		})
	}
}

func TestMin(t *testing.T) {
	tests := []struct {
		name     string
		expr     Min
		expected string
	}{
		{"min constants", MinExpr(C(5), C(10)), "min(5, 10)"},
		{"min vars", MinExpr(V("x"), V("y")), "min(x, y)"},
		{"min mixed", MinExpr(L("a"), C(5)), "min(len(a), 5)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.expr.String(); got != tt.expected {
				t.Errorf("String() = %s, want %s", got, tt.expected)
			}
		})
	}
}

func TestMinMaxEval(t *testing.T) {
	tests := []struct {
		name     string
		expr     Expr
		env      map[string]int64
		expected int64
		ok       bool
	}{
		{"min(5, 10)", MinExpr(C(5), C(10)), nil, 5, true},
		{"min(10, 5)", MinExpr(C(10), C(5)), nil, 5, true},
		{"min(x, y) with x=3, y=7", MinExpr(V("x"), V("y")), map[string]int64{"x": 3, "y": 7}, 3, true},
		{"min(x, y) with x=8, y=2", MinExpr(V("x"), V("y")), map[string]int64{"x": 8, "y": 2}, 2, true},
		{"min(x, 5) with x unknown", MinExpr(V("x"), C(5)), nil, 0, false},
		{"min(5, y) with y unknown", MinExpr(C(5), V("y")), nil, 0, false},
		{"min(x, y) with x=5, y=5", MinExpr(V("x"), V("y")), map[string]int64{"x": 5, "y": 5}, 5, true},
		{"max(5, 10)", MaxExpr(C(5), C(10)), nil, 10, true},
		{"max(10, 5)", MaxExpr(C(10), C(5)), nil, 10, true},
		{"max(x, y) with x=3, y=7", MaxExpr(V("x"), V("y")), map[string]int64{"x": 3, "y": 7}, 7, true},
		{"max(x, y) with x=8, y=2", MaxExpr(V("x"), V("y")), map[string]int64{"x": 8, "y": 2}, 8, true},
		{"max(x, 5) with x unknown", MaxExpr(V("x"), C(5)), nil, 0, false},
		{"max(5, y) with y unknown", MaxExpr(C(5), V("y")), nil, 0, false},
		{"max(x, y) with x=5, y=5", MaxExpr(V("x"), V("y")), map[string]int64{"x": 5, "y": 5}, 5, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, ok := tt.expr.Eval(tt.env)
			if ok != tt.ok {
				t.Errorf("Eval() ok = %v, want %v", ok, tt.ok)
			}
			if ok && val != tt.expected {
				t.Errorf("Eval() val = %d, want %d", val, tt.expected)
			}
		})
	}
}

func TestMinSubstitute(t *testing.T) {
	tests := []struct {
		name     string
		expr     Min
		subst    map[string]Expr
		expected string
	}{
		{"substitute left", MinExpr(V("x"), C(5)), map[string]Expr{"x": C(3)}, "min(3, 5)"},
		{"substitute both", MinExpr(V("x"), V("y")), map[string]Expr{"x": C(3), "y": C(7)}, "min(3, 7)"},
		{"no substitution", MinExpr(V("x"), V("y")), nil, "min(x, y)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.expr.Substitute(tt.subst)
			if got := result.String(); got != tt.expected {
				t.Errorf("Substitute() = %s, want %s", got, tt.expected)
			}
		})
	}
}

func TestMinMaxVariables(t *testing.T) {
	tests := []struct {
		name     string
		expr     Expr
		expected []string
	}{
		{"min no vars", MinExpr(C(5), C(10)), nil},
		{"min one var", MinExpr(V("x"), C(5)), []string{"x"}},
		{"min two vars", MinExpr(V("x"), V("y")), []string{"x", "y"}},
		{"min with len", MinExpr(L("arr"), V("x")), []string{"arr.len", "x"}},
		{"max no vars", MaxExpr(C(5), C(10)), nil},
		{"max one var", MaxExpr(V("x"), C(5)), []string{"x"}},
		{"max two vars", MaxExpr(V("x"), V("y")), []string{"x", "y"}},
		{"max with len", MaxExpr(L("arr"), V("x")), []string{"arr.len", "x"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vars := tt.expr.Variables()
			varSet := make(map[string]bool)

			for _, v := range vars {
				varSet[v] = true
			}

			for _, want := range tt.expected {
				if !varSet[want] {
					t.Errorf("Variables() missing %s, got %v", want, vars)
				}
			}

			if len(tt.expected) == 0 && len(vars) != 0 {
				t.Errorf("Variables() = %v, want empty", vars)
			}
		})
	}
}

func TestMax(t *testing.T) {
	tests := []struct {
		name     string
		expr     Max
		expected string
	}{
		{"max constants", MaxExpr(C(5), C(10)), "max(5, 10)"},
		{"max vars", MaxExpr(V("x"), V("y")), "max(x, y)"},
		{"max mixed", MaxExpr(L("a"), C(5)), "max(len(a), 5)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.expr.String(); got != tt.expected {
				t.Errorf("String() = %s, want %s", got, tt.expected)
			}
		})
	}
}

func TestMaxSubstitute(t *testing.T) {
	tests := []struct {
		name     string
		expr     Max
		subst    map[string]Expr
		expected string
	}{
		{"substitute left", MaxExpr(V("x"), C(5)), map[string]Expr{"x": C(3)}, "max(3, 5)"},
		{"substitute both", MaxExpr(V("x"), V("y")), map[string]Expr{"x": C(3), "y": C(7)}, "max(3, 7)"},
		{"no substitution", MaxExpr(V("x"), V("y")), nil, "max(x, y)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.expr.Substitute(tt.subst)
			if got := result.String(); got != tt.expected {
				t.Errorf("Substitute() = %s, want %s", got, tt.expected)
			}
		})
	}
}

func TestBinOpDivisionByZero(t *testing.T) {
	tests := []struct {
		name string
		expr Expr
	}{
		{"div by zero", Div(C(10), C(0))},
		{"mod by zero", Mod(C(10), C(0))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := tt.expr.Eval(nil)
			if ok {
				t.Errorf("Eval() should fail for division by zero")
			}
		})
	}
}

func TestBinOpUnknownOp(t *testing.T) {
	expr := BinOp{Op: Op(999), Left: C(5), Right: C(3)}

	_, ok := expr.Eval(nil)
	if ok {
		t.Errorf("Eval() should fail for unknown operator")
	}
}

func TestOpString(t *testing.T) {
	tests := []struct {
		op       Op
		expected string
	}{
		{OpAdd, "+"},
		{OpSub, "-"},
		{OpMul, "*"},
		{OpDiv, "/"},
		{OpMod, "%"},
		{Op(999), "?"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.op.String(); got != tt.expected {
				t.Errorf("String() = %s, want %s", got, tt.expected)
			}
		})
	}
}

func TestLenSubstitute(t *testing.T) {
	tests := []struct {
		name     string
		len      Len
		subst    map[string]Expr
		expected Expr
	}{
		{"substitute arr.len", L("arr"), map[string]Expr{"arr.len": C(10)}, C(10)},
		{"no substitution", L("arr"), map[string]Expr{"x": C(5)}, L("arr")},
		{"substitute with var", L("arr"), map[string]Expr{"arr.len": V("n")}, V("n")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.len.Substitute(tt.subst)
			if result.String() != tt.expected.String() {
				t.Errorf("Substitute() = %s, want %s", result, tt.expected)
			}
		})
	}
}

func TestCollectVars(t *testing.T) {
	tests := []struct {
		name     string
		exprs    []Expr
		expected []string
	}{
		{"single var", []Expr{V("x")}, []string{"x"}},
		{"multiple vars", []Expr{V("x"), V("y"), V("z")}, []string{"x", "y", "z"}},
		{"duplicates", []Expr{V("x"), V("y"), V("x")}, []string{"x", "y"}},
		{"constants", []Expr{C(5), C(10)}, nil},
		{"mixed", []Expr{V("x"), C(5), L("arr")}, []string{"x", "arr.len"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vars := collectVars(tt.exprs...)
			varSet := make(map[string]bool)

			for _, v := range vars {
				varSet[v] = true
			}

			for _, want := range tt.expected {
				if !varSet[want] {
					t.Errorf("collectVars() missing %s, got %v", want, vars)
				}
			}

			if len(tt.expected) == 0 && len(vars) != 0 {
				t.Errorf("collectVars() = %v, want empty", vars)
			}
		})
	}
}

func TestSimplifyMinMax(t *testing.T) {
	tests := []struct {
		name     string
		expr     Expr
		expected string
	}{
		{"min(5, 10)", MinExpr(C(5), C(10)), "5"},
		{"min(10, 5)", MinExpr(C(10), C(5)), "5"},
		{"max(5, 10)", MaxExpr(C(5), C(10)), "10"},
		{"max(10, 5)", MaxExpr(C(10), C(5)), "10"},
		{"min(x, y)", MinExpr(V("x"), V("y")), "min(x, y)"},
		{"max(x, y)", MaxExpr(V("x"), V("y")), "max(x, y)"},
		{"nested min", MinExpr(MinExpr(C(3), C(5)), C(7)), "3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Simplify(tt.expr)
			if got := result.String(); got != tt.expected {
				t.Errorf("Simplify() = %s, want %s", got, tt.expected)
			}
		})
	}
}

func TestSimplifyAllOps(t *testing.T) {
	tests := []struct {
		name     string
		expr     Expr
		expected string
	}{
		{"div 15/3", Div(C(15), C(3)), "5"},
		{"mod 17%5", Mod(C(17), C(5)), "2"},
		{"complex", Add(Mul(C(2), C(3)), Sub(C(10), C(2))), "14"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Simplify(tt.expr)
			if got := result.String(); got != tt.expected {
				t.Errorf("Simplify() = %s, want %s", got, tt.expected)
			}
		})
	}
}

func TestBinOpEvalUnknownVar(t *testing.T) {
	tests := []struct {
		name string
		expr Expr
	}{
		{"left unknown", Add(V("x"), C(5))},
		{"right unknown", Add(C(5), V("y"))},
		{"both unknown", Add(V("x"), V("y"))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := tt.expr.Eval(nil)
			if ok {
				t.Errorf("Eval() should fail when variables are unknown")
			}
		})
	}
}
