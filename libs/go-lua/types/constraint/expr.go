package constraint

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/wippyai/go-lua/internal"
)

// Expr represents an arithmetic expression in the constraint language.
// Expressions are immutable and can be composed freely.
type Expr interface {
	exprNode()
	String() string
	// Substitute replaces variables according to the substitution map.
	Substitute(subst map[string]Expr) Expr
	// Variables returns all variable names in this expression.
	Variables() []string
	// Eval evaluates the expression given concrete variable values.
	// Returns (value, true) if fully evaluated, (0, false) if unknown variables remain.
	Eval(env map[string]int64) (int64, bool)
}

// Var represents a symbolic variable (e.g., "i", "len_arr").
type Var struct {
	Name string
}

func (Var) exprNode()        {}
func (v Var) String() string { return v.Name }

func (v Var) Substitute(subst map[string]Expr) Expr {
	if e, ok := subst[v.Name]; ok {
		return e
	}

	return v
}

func (v Var) Variables() []string {
	return []string{v.Name}
}

func (v Var) Eval(env map[string]int64) (int64, bool) {
	if val, ok := env[v.Name]; ok {
		return val, true
	}

	return 0, false
}

// Const represents a constant integer value.
type Const struct {
	Value int64
}

func (Const) exprNode()        {}
func (c Const) String() string { return strconv.FormatInt(c.Value, 10) }

func (c Const) Substitute(map[string]Expr) Expr {
	return c
}

func (Const) Variables() []string {
	return nil
}

func (c Const) Eval(map[string]int64) (int64, bool) {
	return c.Value, true
}

// BinOp represents a binary arithmetic operation.
type BinOp struct {
	Op    Op
	Left  Expr
	Right Expr
}

// Op represents an arithmetic operator.
type Op int

const (
	OpAdd Op = iota // +
	OpSub           // -
	OpMul           // *
	OpDiv           // /
	OpMod           // %
)

func (op Op) String() string {
	switch op {
	case OpAdd:
		return "+"
	case OpSub:
		return "-"
	case OpMul:
		return "*"
	case OpDiv:
		return "/"
	case OpMod:
		return "%"
	default:
		return "?"
	}
}

func (BinOp) exprNode() {}

func (b BinOp) String() string {
	return fmt.Sprintf("(%s %s %s)", b.Left, b.Op, b.Right)
}

func (b BinOp) Substitute(subst map[string]Expr) Expr {
	return BinOp{
		Op:    b.Op,
		Left:  b.Left.Substitute(subst),
		Right: b.Right.Substitute(subst),
	}
}

func (b BinOp) Variables() []string {
	return collectVars(b.Left, b.Right)
}

func (b BinOp) Eval(env map[string]int64) (int64, bool) {
	left, ok := b.Left.Eval(env)
	if !ok {
		return 0, false
	}

	right, ok := b.Right.Eval(env)
	if !ok {
		return 0, false
	}

	switch b.Op {
	case OpAdd:
		return internal.SafeAdd(left, right)
	case OpSub:
		return internal.SafeSub(left, right)
	case OpMul:
		return internal.SafeMul(left, right)
	case OpDiv:
		if right == 0 {
			return 0, false
		}

		if left == internal.MinInt64 && right == -1 {
			return 0, false
		}

		return left / right, true
	case OpMod:
		if right == 0 {
			return 0, false
		}

		if left == internal.MinInt64 && right == -1 {
			return 0, true
		}

		return left % right, true
	default:
		return 0, false
	}
}

// Len represents the length of a symbolic array/tuple.
type Len struct {
	Of string // variable name of the array
}

func (Len) exprNode()        {}
func (l Len) String() string { return fmt.Sprintf("len(%s)", l.Of) }

func (l Len) Substitute(subst map[string]Expr) Expr {
	// Check if we have a substitution for this length (stored as "name.len")
	if e, ok := subst[l.Of+".len"]; ok {
		return e
	}

	return l
}

func (l Len) Variables() []string {
	return []string{l.Of + ".len"}
}

func (l Len) Eval(env map[string]int64) (int64, bool) {
	if val, ok := env[l.Of+".len"]; ok {
		return val, true
	}

	return 0, false
}

// Param represents a reference to a function parameter by index.
// Used in function refinements: len(Param(0)) = 5
type Param struct {
	Index int // 0-based parameter index
}

func (Param) exprNode()        {}
func (p Param) String() string { return paramExprKey(p.Index) }

func (p Param) Substitute(subst map[string]Expr) Expr {
	key := paramExprKey(p.Index)
	if e, ok := subst[key]; ok {
		return e
	}

	return p
}

func (p Param) Variables() []string {
	return []string{paramExprKey(p.Index)}
}

func (p Param) Eval(env map[string]int64) (int64, bool) {
	key := paramExprKey(p.Index)
	if val, ok := env[key]; ok {
		return val, true
	}

	return 0, false
}

// Ret represents a reference to a function return value by index.
// Used in function refinements: len(Ret(0)) = len(Param(0))
type Ret struct {
	Index int // 0-based return value index
}

func (Ret) exprNode()        {}
func (r Ret) String() string { return retExprKey(r.Index) }

func (r Ret) Substitute(subst map[string]Expr) Expr {
	key := retExprKey(r.Index)
	if e, ok := subst[key]; ok {
		return e
	}

	return r
}

func (r Ret) Variables() []string {
	return []string{retExprKey(r.Index)}
}

func (r Ret) Eval(env map[string]int64) (int64, bool) {
	key := retExprKey(r.Index)
	if val, ok := env[key]; ok {
		return val, true
	}

	return 0, false
}

// ParamLen represents the length of a function parameter.
// Shorthand for Len applied to Param.
type ParamLen struct {
	Index int // 0-based parameter index
}

func (ParamLen) exprNode()        {}
func (p ParamLen) String() string { return "len(" + paramExprKey(p.Index) + ")" }

func (p ParamLen) Substitute(subst map[string]Expr) Expr {
	key := paramLenExprKey(p.Index)
	if e, ok := subst[key]; ok {
		return e
	}

	return p
}

func (p ParamLen) Variables() []string {
	return []string{paramLenExprKey(p.Index)}
}

func (p ParamLen) Eval(env map[string]int64) (int64, bool) {
	key := paramLenExprKey(p.Index)
	if val, ok := env[key]; ok {
		return val, true
	}

	return 0, false
}

// RetLen represents the length of a function return value.
// Shorthand for Len applied to Ret.
type RetLen struct {
	Index int // 0-based return value index
}

func (RetLen) exprNode()        {}
func (r RetLen) String() string { return "len(" + retExprKey(r.Index) + ")" }

func (r RetLen) Substitute(subst map[string]Expr) Expr {
	key := retLenExprKey(r.Index)
	if e, ok := subst[key]; ok {
		return e
	}

	return r
}

func (r RetLen) Variables() []string {
	return []string{retLenExprKey(r.Index)}
}

func (r RetLen) Eval(env map[string]int64) (int64, bool) {
	key := retLenExprKey(r.Index)
	if val, ok := env[key]; ok {
		return val, true
	}

	return 0, false
}

// Constructors for ergonomic building

// V creates a variable expression.
func V(name string) Var { return Var{Name: name} }

// C creates a constant expression.
func C(val int64) Const { return Const{Value: val} }

// Add creates an addition expression.
func Add(left, right Expr) BinOp { return BinOp{Op: OpAdd, Left: left, Right: right} }

// Sub creates a subtraction expression.
func Sub(left, right Expr) BinOp { return BinOp{Op: OpSub, Left: left, Right: right} }

// Mul creates a multiplication expression.
func Mul(left, right Expr) BinOp { return BinOp{Op: OpMul, Left: left, Right: right} }

// Div creates a division expression.
func Div(left, right Expr) BinOp { return BinOp{Op: OpDiv, Left: left, Right: right} }

// Mod creates a modulo expression.
func Mod(left, right Expr) BinOp { return BinOp{Op: OpMod, Left: left, Right: right} }

// L creates a length expression.
func L(name string) Len { return Len{Of: name} }

// P creates a parameter reference expression.
func P(index int) Param { return Param{Index: index} }

// R creates a return value reference expression.
func R(index int) Ret { return Ret{Index: index} }

// PL creates a parameter length expression.
func PL(index int) ParamLen { return ParamLen{Index: index} }

// RL creates a return length expression.
func RL(index int) RetLen { return RetLen{Index: index} }

func paramExprKey(index int) string {
	return "param[" + strconv.Itoa(index) + "]"
}

func retExprKey(index int) string {
	return "ret[" + strconv.Itoa(index) + "]"
}

func paramLenExprKey(index int) string {
	return paramExprKey(index) + ".len"
}

func retLenExprKey(index int) string {
	return retExprKey(index) + ".len"
}

// Min represents the minimum of two expressions.
type Min struct {
	Left  Expr
	Right Expr
}

func (Min) exprNode()        {}
func (m Min) String() string { return fmt.Sprintf("min(%s, %s)", m.Left, m.Right) }

func (m Min) Substitute(subst map[string]Expr) Expr {
	return Min{
		Left:  m.Left.Substitute(subst),
		Right: m.Right.Substitute(subst),
	}
}

func (m Min) Variables() []string {
	return collectVars(m.Left, m.Right)
}

func (m Min) Eval(env map[string]int64) (int64, bool) {
	left, ok := m.Left.Eval(env)
	if !ok {
		return 0, false
	}

	right, ok := m.Right.Eval(env)
	if !ok {
		return 0, false
	}

	if left < right {
		return left, true
	}

	return right, true
}

// MinExpr creates a min expression.
func MinExpr(left, right Expr) Min { return Min{Left: left, Right: right} }

// Max represents the maximum of two expressions.
type Max struct {
	Left  Expr
	Right Expr
}

func (Max) exprNode()        {}
func (m Max) String() string { return fmt.Sprintf("max(%s, %s)", m.Left, m.Right) }

func (m Max) Substitute(subst map[string]Expr) Expr {
	return Max{
		Left:  m.Left.Substitute(subst),
		Right: m.Right.Substitute(subst),
	}
}

func (m Max) Variables() []string {
	return collectVars(m.Left, m.Right)
}

func (m Max) Eval(env map[string]int64) (int64, bool) {
	left, ok := m.Left.Eval(env)
	if !ok {
		return 0, false
	}

	right, ok := m.Right.Eval(env)
	if !ok {
		return 0, false
	}

	if left > right {
		return left, true
	}

	return right, true
}

// MaxExpr creates a max expression.
func MaxExpr(left, right Expr) Max { return Max{Left: left, Right: right} }

// Simplify attempts to simplify an expression by constant folding.
func Simplify(e Expr) Expr {
	return VisitExpr(e, ExprVisitor[Expr]{
		Var: func(Var) Expr {
			return e
		},
		Const: func(Const) Expr {
			return e
		},
		Len: func(Len) Expr {
			return e
		},
		Param: func(Param) Expr {
			return e
		},
		Ret: func(Ret) Expr {
			return e
		},
		ParamLen: func(ParamLen) Expr {
			return e
		},
		RetLen: func(RetLen) Expr {
			return e
		},
		Min: func(x Min) Expr {
			left := Simplify(x.Left)
			right := Simplify(x.Right)
			lc, lok := left.(Const)
			rc, rok := right.(Const)

			if lok && rok {
				if lc.Value < rc.Value {
					return lc
				}

				return rc
			}

			return Min{Left: left, Right: right}
		},
		Max: func(x Max) Expr {
			left := Simplify(x.Left)
			right := Simplify(x.Right)
			lc, lok := left.(Const)
			rc, rok := right.(Const)

			if lok && rok {
				if lc.Value > rc.Value {
					return lc
				}

				return rc
			}

			return Max{Left: left, Right: right}
		},
		BinOp: func(x BinOp) Expr {
			left := Simplify(x.Left)
			right := Simplify(x.Right)

			lc, lok := left.(Const)
			rc, rok := right.(Const)

			if lok && rok {
				result, ok := BinOp{Op: x.Op, Left: lc, Right: rc}.Eval(nil)
				if ok {
					return C(result)
				}
			}

			// x + 0 = x, 0 + x = x
			if x.Op == OpAdd {
				if lok && lc.Value == 0 {
					return right
				}

				if rok && rc.Value == 0 {
					return left
				}
			}

			// x - 0 = x
			if x.Op == OpSub && rok && rc.Value == 0 {
				return left
			}

			// x * 1 = x, 1 * x = x, x * 0 = 0, 0 * x = 0
			if x.Op == OpMul {
				if lok && lc.Value == 1 {
					return right
				}

				if rok && rc.Value == 1 {
					return left
				}

				if (lok && lc.Value == 0) || (rok && rc.Value == 0) {
					return C(0)
				}
			}

			return BinOp{Op: x.Op, Left: left, Right: right}
		},
		Default: func(Expr) Expr {
			return e
		},
	})
}

// ExprEquals compares two Expr values for structural equality.
func ExprEquals(a, b Expr) bool {
	if a == nil && b == nil {
		return true
	}

	if a == nil || b == nil {
		return false
	}

	return VisitExpr(a, ExprVisitor[bool]{
		Var: func(av Var) bool {
			return VisitExpr(b, ExprVisitor[bool]{
				Var: func(bv Var) bool {
					return av.Name == bv.Name
				},
				Default: func(Expr) bool { return false },
			})
		},
		Const: func(av Const) bool {
			return VisitExpr(b, ExprVisitor[bool]{
				Const: func(bv Const) bool {
					return av.Value == bv.Value
				},
				Default: func(Expr) bool { return false },
			})
		},
		Len: func(av Len) bool {
			return VisitExpr(b, ExprVisitor[bool]{
				Len: func(bv Len) bool {
					return av.Of == bv.Of
				},
				Default: func(Expr) bool { return false },
			})
		},
		Param: func(av Param) bool {
			return VisitExpr(b, ExprVisitor[bool]{
				Param: func(bv Param) bool {
					return av.Index == bv.Index
				},
				Default: func(Expr) bool { return false },
			})
		},
		Ret: func(av Ret) bool {
			return VisitExpr(b, ExprVisitor[bool]{
				Ret: func(bv Ret) bool {
					return av.Index == bv.Index
				},
				Default: func(Expr) bool { return false },
			})
		},
		ParamLen: func(av ParamLen) bool {
			return VisitExpr(b, ExprVisitor[bool]{
				ParamLen: func(bv ParamLen) bool {
					return av.Index == bv.Index
				},
				Default: func(Expr) bool { return false },
			})
		},
		RetLen: func(av RetLen) bool {
			return VisitExpr(b, ExprVisitor[bool]{
				RetLen: func(bv RetLen) bool {
					return av.Index == bv.Index
				},
				Default: func(Expr) bool { return false },
			})
		},
		BinOp: func(av BinOp) bool {
			return VisitExpr(b, ExprVisitor[bool]{
				BinOp: func(bv BinOp) bool {
					return av.Op == bv.Op && ExprEquals(av.Left, bv.Left) && ExprEquals(av.Right, bv.Right)
				},
				Default: func(Expr) bool { return false },
			})
		},
		Min: func(av Min) bool {
			return VisitExpr(b, ExprVisitor[bool]{
				Min: func(bv Min) bool {
					return ExprEquals(av.Left, bv.Left) && ExprEquals(av.Right, bv.Right)
				},
				Default: func(Expr) bool { return false },
			})
		},
		Max: func(av Max) bool {
			return VisitExpr(b, ExprVisitor[bool]{
				Max: func(bv Max) bool {
					return ExprEquals(av.Left, bv.Left) && ExprEquals(av.Right, bv.Right)
				},
				Default: func(Expr) bool { return false },
			})
		},
		Default: func(Expr) bool {
			return false
		},
	})
}

// collectVars collects unique variable names from expressions.
func collectVars(exprs ...Expr) []string {
	seen := make(map[string]bool)

	for _, e := range exprs {
		for _, v := range e.Variables() {
			seen[v] = true
		}
	}

	result := make([]string, 0, len(seen))

	for v := range seen {
		result = append(result, v)
	}

	sort.Strings(result)

	return result
}
