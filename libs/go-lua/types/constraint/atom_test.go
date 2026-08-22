package constraint

import (
	"testing"

	"github.com/wippyai/go-lua/types/narrow"
)

func TestTermHash(t *testing.T) {
	tests := []struct {
		name   string
		term1  Term
		term2  Term
		expect bool
	}{
		{"same var", TermVar("x"), TermVar("x"), true},
		{"diff var", TermVar("x"), TermVar("y"), false},
		{"same len", TermLen("arr"), TermLen("arr"), true},
		{"diff len", TermLen("arr"), TermLen("brr"), false},
		{"var vs len", TermVar("x"), TermLen("x"), false},
		{"same const", TermConst(42), TermConst(42), true},
		{"diff const", TermConst(42), TermConst(43), false},
		{"nil vs nil", TermNil(), TermNil(), true},
		{"nil vs const0", TermNil(), TermConst(0), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h1 := tc.term1.Hash()
			h2 := tc.term2.Hash()
			if (h1 == h2) != tc.expect {
				t.Errorf("Hash match = %v, want %v", h1 == h2, tc.expect)
			}
		})
	}
}

func TestTermEqual(t *testing.T) {
	tests := []struct {
		name   string
		term1  Term
		term2  Term
		expect bool
	}{
		{"same var", TermVar("x"), TermVar("x"), true},
		{"diff var", TermVar("x"), TermVar("y"), false},
		{"same len", TermLen("arr"), TermLen("arr"), true},
		{"var vs len same path", TermVar("x"), TermLen("x"), false},
		{"same const", TermConst(100), TermConst(100), true},
		{"diff const", TermConst(1), TermConst(2), false},
		{"nil vs nil", TermNil(), TermNil(), true},
		{"var vs nil", TermVar("x"), TermNil(), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.term1.Equal(tc.term2); got != tc.expect {
				t.Errorf("Equal = %v, want %v", got, tc.expect)
			}
		})
	}
}

func TestTermPaths(t *testing.T) {
	tests := []struct {
		name   string
		term   Term
		expect int
	}{
		{"var has path", TermVar("x"), 1},
		{"len has path", TermLen("arr"), 1},
		{"const no path", TermConst(42), 0},
		{"nil no path", TermNil(), 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			paths := tc.term.Paths()
			if len(paths) != tc.expect {
				t.Errorf("len(Paths) = %d, want %d", len(paths), tc.expect)
			}
		})
	}
}

func TestAtomHash(t *testing.T) {
	x := TermVar("x")
	y := TermVar("y")
	c10 := TermConst(10)

	tests := []struct {
		name   string
		atom1  Atom
		atom2  Atom
		expect bool
	}{
		{"same eq", AtomEq(x, y), AtomEq(x, y), true},
		{"diff eq", AtomEq(x, y), AtomEq(y, x), false},
		{"eq vs ne", AtomEq(x, y), AtomNe(x, y), false},
		{"same lt", AtomLt(x, y), AtomLt(x, y), true},
		{"same le const", AtomLe(x, c10), AtomLe(x, c10), true},
		{"same truthy", AtomTruthy(x), AtomTruthy(x), true},
		{"truthy vs falsy", AtomTruthy(x), AtomFalsy(x), false},
		{"same modeq", AtomModEq(x, 5, 2), AtomModEq(x, 5, 2), true},
		{"diff modeq mod", AtomModEq(x, 5, 2), AtomModEq(x, 6, 2), false},
		{"diff modeq rem", AtomModEq(x, 5, 2), AtomModEq(x, 5, 3), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h1 := tc.atom1.Hash()
			h2 := tc.atom2.Hash()
			if (h1 == h2) != tc.expect {
				t.Errorf("Hash match = %v, want %v", h1 == h2, tc.expect)
			}
		})
	}
}

func TestAtomEqual(t *testing.T) {
	x := TermVar("x")
	y := TermVar("y")
	c10 := TermConst(10)
	tk := narrow.BuiltinTypeKey("number")

	tests := []struct {
		name   string
		atom1  Atom
		atom2  Atom
		expect bool
	}{
		{"same eq", AtomEq(x, y), AtomEq(x, y), true},
		{"diff eq terms", AtomEq(x, y), AtomEq(y, x), false},
		{"eq vs ne", AtomEq(x, y), AtomNe(x, y), false},
		{"same hastype", AtomHasType(x, tk), AtomHasType(x, tk), true},
		{"hastype vs nothastype", AtomHasType(x, tk), AtomNotHasType(x, tk), false},
		{"same le const", AtomLe(x, c10), AtomLe(x, c10), true},
		{"diff le const", AtomLe(x, c10), AtomLe(x, TermConst(20)), false},
		{"same truthy", AtomTruthy(x), AtomTruthy(x), true},
		{"diff truthy term", AtomTruthy(x), AtomTruthy(y), false},
		{"same modeq", AtomModEq(x, 5, 2), AtomModEq(x, 5, 2), true},
		{"diff modeq", AtomModEq(x, 5, 2), AtomModEq(x, 5, 3), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.atom1.Equal(tc.atom2); got != tc.expect {
				t.Errorf("Equal = %v, want %v", got, tc.expect)
			}
		})
	}
}

func TestAtomPaths(t *testing.T) {
	x := TermVar("x")
	y := TermVar("y")
	c10 := TermConst(10)

	tests := []struct {
		name   string
		atom   Atom
		expect int
	}{
		{"eq two vars", AtomEq(x, y), 2},
		{"eq var and const", AtomEq(x, c10), 1},
		{"eq nil", AtomEq(x, TermNil()), 1},
		{"truthy", AtomTruthy(x), 1},
		{"modeq", AtomModEq(x, 5, 2), 1},
		{"le with len", AtomLe(x, TermLen("arr")), 2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			paths := tc.atom.Paths()
			if len(paths) != tc.expect {
				t.Errorf("len(Paths) = %d, want %d", len(paths), tc.expect)
			}
		})
	}
}

func TestTypeConstraintToAtom(t *testing.T) {
	path := Path{Root: "x"}

	tests := []struct {
		name       string
		constraint Constraint
		expectOk   bool
		expectKind AtomKind
	}{
		{"Truthy", Truthy{Path: path}, true, AtomKindTruthy},
		{"Falsy", Falsy{Path: path}, true, AtomKindFalsy},
		{"IsNil", IsNil{Path: path}, true, AtomKindEq},
		{"NotNil", NotNil{Path: path}, true, AtomKindNe},
		{"HasType", HasType{Path: path, Type: narrow.BuiltinTypeKey("number")}, true, AtomKindHasType},
		{"NotHasType", NotHasType{Path: path, Type: narrow.BuiltinTypeKey("string")}, true, AtomKindNotHasType},
		{"EqPath", NewEqPath(path, Path{Root: "y"}), true, AtomKindEq},
		{"NotEqPath", NewNotEqPath(path, Path{Root: "y"}), true, AtomKindNe},
		{"HasField", HasField{Path: path, Field: "f"}, false, AtomKindInvalid},
		{"FieldEquals", FieldEquals{Target: path, Field: "f"}, false, AtomKindInvalid},
		{"FieldEqualsPath", FieldEqualsPath{Target: path, Field: "f", Value: Path{Root: "z"}}, false, AtomKindInvalid},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			atom, ok := TypeConstraintToAtom(tc.constraint)
			if ok != tc.expectOk {
				t.Errorf("ok = %v, want %v", ok, tc.expectOk)
			}
			if ok && atom.Kind != tc.expectKind {
				t.Errorf("Kind = %v, want %v", atom.Kind, tc.expectKind)
			}
		})
	}
}

func TestNumericConstraintToAtom(t *testing.T) {
	x := Path{Root: "x"}
	y := Path{Root: "y"}
	arr := Path{Root: "arr"}

	tests := []struct {
		name       string
		constraint NumericConstraint
		expectOk   bool
		expectKind AtomKind
	}{
		{"Lt", Lt{X: x, Y: y}, true, AtomKindLt},
		{"Le c=0", Le{X: x, Y: y, C: 0}, true, AtomKindLe},
		{"Le c!=0", Le{X: x, Y: y, C: 5}, false, AtomKindInvalid},
		{"Gt", Gt{X: x, Y: y}, true, AtomKindGt},
		{"Ge", Ge{X: x, Y: y}, true, AtomKindGe},
		{"Eq", Eq{X: x, Y: y}, true, AtomKindEq},
		{"EqConst", EqConst{X: x, C: 42}, true, AtomKindEq},
		{"LeConst", LeConst{X: x, C: 100}, true, AtomKindLe},
		{"GeConst", GeConst{X: x, C: 0}, true, AtomKindGe},
		{"ModEq", ModEq{X: x, M: 5, R: 2}, true, AtomKindModEq},
		{"LeLenOf", LeLenOf{X: x, Array: arr}, true, AtomKindLe},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			atom, ok := NumericConstraintToAtom(tc.constraint)
			if ok != tc.expectOk {
				t.Errorf("ok = %v, want %v", ok, tc.expectOk)
			}
			if ok && atom.Kind != tc.expectKind {
				t.Errorf("Kind = %v, want %v", atom.Kind, tc.expectKind)
			}
		})
	}
}

func TestConstraintsToAtoms(t *testing.T) {
	pathX := Path{Root: "x"}
	pathY := Path{Root: "y"}

	constraints := []Constraint{
		Truthy{Path: pathX},
		IsNil{Path: pathY},
		HasField{Path: pathX, Field: "f"},
		NewEqPath(pathX, pathY),
	}

	result := ToAtoms(constraints)

	if len(result.Atoms) != 3 {
		t.Errorf("len(Atoms) = %d, want 3", len(result.Atoms))
	}
	if len(result.Leftover) != 1 {
		t.Errorf("len(Leftover) = %d, want 1", len(result.Leftover))
	}
	if _, ok := result.Leftover[0].(HasField); !ok {
		t.Errorf("Leftover[0] should be HasField, got %T", result.Leftover[0])
	}
}

func TestAtomString(t *testing.T) {
	x := TermVar("x")
	y := TermVar("y")
	c := TermConst(10)

	tests := []struct {
		atom   Atom
		expect string
	}{
		{AtomEq(x, y), "x == y"},
		{AtomNe(x, c), "x != 10"},
		{AtomLt(x, y), "x < y"},
		{AtomLe(x, c), "x <= 10"},
		{AtomGt(x, y), "x > y"},
		{AtomGe(x, c), "x >= 10"},
		{AtomTruthy(x), "truthy(x)"},
		{AtomFalsy(x), "falsy(x)"},
		{AtomModEq(x, 5, 2), "x % 5 == 2"},
		{AtomEq(x, TermNil()), "x == nil"},
		{AtomLe(x, TermLen("arr")), "x <= len(arr)"},
	}

	for _, tc := range tests {
		t.Run(tc.expect, func(t *testing.T) {
			if got := tc.atom.String(); got != tc.expect {
				t.Errorf("String = %q, want %q", got, tc.expect)
			}
		})
	}
}

func TestTermString(t *testing.T) {
	tests := []struct {
		term   Term
		expect string
	}{
		{TermVar("foo"), "foo"},
		{TermLen("arr"), "len(arr)"},
		{TermConst(42), "42"},
		{TermConst(-5), "-5"},
		{TermNil(), "nil"},
	}

	for _, tc := range tests {
		t.Run(tc.expect, func(t *testing.T) {
			if got := tc.term.String(); got != tc.expect {
				t.Errorf("String = %q, want %q", got, tc.expect)
			}
		})
	}
}

func TestPlaceholderPathSubstituteWithAtom(t *testing.T) {
	placeholder := Path{Root: "$0"}
	argPath := Path{Root: "actualArg", Symbol: 123}

	truthy := Truthy{Path: placeholder}
	substituted, ok := truthy.Substitute([]Path{argPath})
	if !ok {
		t.Fatal("Substitute failed")
	}

	atom, ok := TypeConstraintToAtom(substituted)
	if !ok {
		t.Fatal("TypeConstraintToAtom failed")
	}

	if atom.Kind != AtomKindTruthy {
		t.Errorf("Kind = %v, want AtomKindTruthy", atom.Kind)
	}

	paths := atom.Paths()
	if len(paths) != 1 {
		t.Fatalf("len(Paths) = %d, want 1", len(paths))
	}
	if paths[0] != argPath.Key() {
		t.Errorf("Path = %s, want %s", paths[0], argPath.Key())
	}
}

func TestTermKindMethods(t *testing.T) {
	varTerm := TermVar("x")
	lenTerm := TermLen("arr")
	constTerm := TermConst(42)
	nilTerm := TermNil()

	if !varTerm.IsVar() {
		t.Error("TermVar.IsVar() should return true")
	}
	if varTerm.IsLen() || varTerm.IsConst() || varTerm.IsNil() {
		t.Error("TermVar should not match other kinds")
	}

	if !lenTerm.IsLen() {
		t.Error("TermLen.IsLen() should return true")
	}
	if lenTerm.IsVar() || lenTerm.IsConst() || lenTerm.IsNil() {
		t.Error("TermLen should not match other kinds")
	}

	if !constTerm.IsConst() {
		t.Error("TermConst.IsConst() should return true")
	}
	if constTerm.IsVar() || constTerm.IsLen() || constTerm.IsNil() {
		t.Error("TermConst should not match other kinds")
	}

	if !nilTerm.IsNil() {
		t.Error("TermNil.IsNil() should return true")
	}
	if nilTerm.IsVar() || nilTerm.IsLen() || nilTerm.IsConst() {
		t.Error("TermNil should not match other kinds")
	}
}

func TestTermInvalidKindString(t *testing.T) {
	invalid := Term{Kind: TermKind(99)}
	if got := invalid.String(); got != "<invalid>" {
		t.Errorf("Invalid term String() = %q, want %q", got, "<invalid>")
	}
}

func TestTermInvalidKindEqual(t *testing.T) {
	invalid1 := Term{Kind: TermKind(99)}
	invalid2 := Term{Kind: TermKind(99)}

	if invalid1.Equal(invalid2) {
		t.Error("Invalid terms should not be equal")
	}
}

func TestAtomHasTypeString(t *testing.T) {
	x := TermVar("x")
	tk := narrow.BuiltinTypeKey("number")

	hasType := AtomHasType(x, tk)
	got := hasType.String()
	if got == "" {
		t.Error("AtomHasType.String() should not be empty")
	}

	notHasType := AtomNotHasType(x, tk)
	got = notHasType.String()
	if got == "" {
		t.Error("AtomNotHasType.String() should not be empty")
	}
}

func TestAtomInvalidKindString(t *testing.T) {
	invalid := Atom{Kind: AtomKind(99)}
	got := invalid.String()
	if got != "<invalid>" {
		t.Errorf("Invalid atom String() = %q, want %q", got, "<invalid>")
	}
}

func TestAtomInvalidKindHash(t *testing.T) {
	invalid := Atom{Kind: AtomKind(99), Left: TermVar("x"), Right: TermVar("y")}
	h := invalid.Hash()
	if h == 0 {
		t.Error("Invalid atom should still produce non-zero hash from terms")
	}
}

func TestAtomInvalidKindEqual(t *testing.T) {
	x := TermVar("x")
	y := TermVar("y")

	invalid1 := Atom{Kind: AtomKind(99), Left: x, Right: y}
	invalid2 := Atom{Kind: AtomKind(99), Left: x, Right: y}

	if invalid1.Equal(invalid2) {
		t.Error("Invalid atoms should not be equal")
	}
}
