package cfg

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	basecfg "github.com/wippyai/go-lua/types/cfg"
)

// TestVersion_IsZero tests Version zero check.
func TestVersion_IsZero(t *testing.T) {
	zero := Version{}
	if !zero.IsZero() {
		t.Error("Zero version should be zero")
	}

	zeroID := Version{Root: "x", ID: 0}
	if !zeroID.IsZero() {
		t.Error("Version with ID=0 should be zero")
	}

	nonZero := Version{Root: "x", ID: 1}
	if nonZero.IsZero() {
		t.Error("Non-zero version should not be zero")
	}
}

// TestNodeInfo_Kind tests NodeInfo Kind() method.
func TestNodeInfo_Kind(t *testing.T) {
	tests := []struct {
		name string
		info NodeInfo
		want basecfg.NodeKind
	}{
		{"AssignInfo", &AssignInfo{}, basecfg.NodeAssign},
		{"CallInfo", &CallInfo{}, basecfg.NodeCall},
		{"ReturnInfo", &ReturnInfo{}, basecfg.NodeReturn},
		{"BranchInfo", &BranchInfo{}, basecfg.NodeBranch},
		{"TypeDefInfo", &TypeDefInfo{}, basecfg.NodeTypeDef},
		{"FuncDefInfo", &FuncDefInfo{}, basecfg.NodeAssign},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.info.Kind(); got != tt.want {
				t.Errorf("%s.Kind() = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

// TestTargetKind_Values tests TargetKind constants.
func TestTargetKind_Values(t *testing.T) {
	if TargetIdent != 0 {
		t.Error("TargetIdent should be 0")
	}
	if TargetField != 1 {
		t.Error("TargetField should be 1")
	}
	if TargetIndex != 2 {
		t.Error("TargetIndex should be 2")
	}
}

// TestFuncDefTargetKind_Values tests FuncDefTargetKind constants.
func TestFuncDefTargetKind_Values(t *testing.T) {
	if FuncDefGlobal != 0 {
		t.Error("FuncDefGlobal should be 0")
	}
	if FuncDefField != 1 {
		t.Error("FuncDefField should be 1")
	}
	if FuncDefMethod != 2 {
		t.Error("FuncDefMethod should be 2")
	}
}

// TestAssignTarget_Fields tests AssignTarget field access.
func TestAssignTarget_Fields(t *testing.T) {
	target := AssignTarget{
		Kind:      TargetField,
		Name:      "",
		BaseName:  "obj",
		FieldPath: []string{"a", "b"},
	}

	if target.Kind != TargetField {
		t.Error("Kind should be TargetField")
	}
	if target.BaseName != "obj" {
		t.Error("BaseName should be obj")
	}
	if len(target.FieldPath) != 2 {
		t.Error("FieldPath should have 2 elements")
	}
}

// TestAssignInfo_Fields tests AssignInfo fields.
func TestAssignInfo_Fields(t *testing.T) {
	info := &AssignInfo{
		IsLocal: true,
		Targets: []AssignTarget{
			{Kind: TargetIdent, Name: "x"},
		},
		SourceNames:     []string{"y"},
		TargetVersions:  []Version{{Root: "x", ID: 1}},
		TypeAnnotations: nil,
	}

	if !info.IsLocal {
		t.Error("IsLocal should be true")
	}
	if len(info.Targets) != 1 {
		t.Error("Should have 1 target")
	}
	if info.Targets[0].Name != "x" {
		t.Error("Target name should be x")
	}
}

func TestAssignInfo_TargetAtAndFirstTarget(t *testing.T) {
	info := &AssignInfo{
		Targets: []AssignTarget{
			{Kind: TargetIdent, Name: "x"},
			{Kind: TargetField, BaseName: "t"},
		},
	}

	if target, ok := info.TargetAt(0); !ok || target.Name != "x" {
		t.Fatalf("TargetAt(0) mismatch: ok=%v target=%+v", ok, target)
	}
	if target, ok := info.TargetAt(2); ok || target.Name != "" {
		t.Fatalf("TargetAt(2) should be missing, got ok=%v target=%+v", ok, target)
	}
	if target, ok := info.FirstTarget(); !ok || target.Name != "x" {
		t.Fatalf("FirstTarget mismatch: ok=%v target=%+v", ok, target)
	}
}

func TestAssignInfo_SourceCallAt(t *testing.T) {
	info := &AssignInfo{
		SourceCalls: []*CallInfo{
			{CalleeName: "a"},
			nil,
			{CalleeName: "c"},
		},
	}

	if got := info.SourceCallAt(0); got == nil || got.CalleeName != "a" {
		t.Fatalf("SourceCallAt(0) mismatch: got %+v", got)
	}
	if got := info.SourceCallAt(1); got != nil {
		t.Fatalf("SourceCallAt(1) should be nil, got %+v", got)
	}
	if got := info.SourceCallAt(3); got != nil {
		t.Fatalf("SourceCallAt(3) should be nil, got %+v", got)
	}
}

func TestAssignInfo_SourceAt(t *testing.T) {
	info := &AssignInfo{
		Sources: []ast.Expr{
			&ast.StringExpr{Value: "a"},
			nil,
			&ast.NumberExpr{Value: "1"},
		},
	}
	if got := info.SourceAt(0); got == nil {
		t.Fatal("SourceAt(0) should be non-nil")
	}
	if got := info.SourceAt(1); got != nil {
		t.Fatalf("SourceAt(1) should be nil, got %+v", got)
	}
	if got := info.SourceAt(3); got != nil {
		t.Fatalf("SourceAt(3) should be nil, got %+v", got)
	}
}

func TestAssignInfo_LastSource(t *testing.T) {
	info := &AssignInfo{
		Sources: []ast.Expr{
			&ast.StringExpr{Value: "a"},
			&ast.NumberExpr{Value: "2"},
		},
	}
	if got := info.LastSource(); got == nil {
		t.Fatal("LastSource should be non-nil")
	}
	if num, ok := info.LastSource().(*ast.NumberExpr); !ok || num.Value != "2" {
		t.Fatalf("LastSource mismatch: got %+v", info.LastSource())
	}

	if got := (&AssignInfo{}).LastSource(); got != nil {
		t.Fatalf("LastSource should be nil for empty sources, got %+v", got)
	}
}

func TestAssignInfo_TypeAnnotationAt(t *testing.T) {
	info := &AssignInfo{
		TypeAnnotations: []ast.TypeExpr{
			&ast.PrimitiveTypeExpr{Name: "string"},
			nil,
		},
	}
	if got := info.TypeAnnotationAt(0); got == nil {
		t.Fatal("TypeAnnotationAt(0) should be non-nil")
	}
	if got := info.TypeAnnotationAt(1); got != nil {
		t.Fatalf("TypeAnnotationAt(1) should be nil, got %+v", got)
	}
	if got := info.TypeAnnotationAt(3); got != nil {
		t.Fatalf("TypeAnnotationAt(3) should be nil, got %+v", got)
	}
}

func TestAssignInfo_CallForTarget(t *testing.T) {
	info := &AssignInfo{
		SourceCalls: []*CallInfo{
			{CalleeName: "first"},
			nil,
			{CalleeName: "last"},
		},
	}

	if call, retIndex := info.CallForTarget(0); call == nil || call.CalleeName != "first" || retIndex != 0 {
		t.Fatalf("CallForTarget(0) mismatch: call=%+v retIndex=%d", call, retIndex)
	}
	if call, retIndex := info.CallForTarget(2); call == nil || call.CalleeName != "last" || retIndex != 0 {
		t.Fatalf("CallForTarget(2) mismatch: call=%+v retIndex=%d", call, retIndex)
	}
	if call, retIndex := info.CallForTarget(3); call == nil || call.CalleeName != "last" || retIndex != 1 {
		t.Fatalf("CallForTarget(3) mismatch: call=%+v retIndex=%d", call, retIndex)
	}
	if call, retIndex := info.CallForTarget(1); call != nil || retIndex != 0 {
		t.Fatalf("CallForTarget(1) should be nil, got call=%+v retIndex=%d", call, retIndex)
	}
}

func TestAssignInfo_SingleSourceCall(t *testing.T) {
	info := &AssignInfo{
		SourceCalls: []*CallInfo{{CalleeName: "only"}},
	}
	if got := info.SingleSourceCall(); got == nil || got.CalleeName != "only" {
		t.Fatalf("SingleSourceCall mismatch: got %+v", got)
	}

	info = &AssignInfo{
		SourceCalls: []*CallInfo{{CalleeName: "a"}, {CalleeName: "b"}},
	}
	if got := info.SingleSourceCall(); got != nil {
		t.Fatalf("SingleSourceCall should be nil for multiple source calls, got %+v", got)
	}

	info = &AssignInfo{
		SourceCalls: []*CallInfo{nil},
	}
	if got := info.SingleSourceCall(); got != nil {
		t.Fatalf("SingleSourceCall should be nil when sole entry is nil, got %+v", got)
	}
}

func TestAssignInfo_HasSiblingCallExpansion(t *testing.T) {
	info := &AssignInfo{
		Targets: []AssignTarget{{Kind: TargetIdent}, {Kind: TargetIdent}},
		Sources: []ast.Expr{&ast.FuncCallExpr{}},
		SourceCalls: []*CallInfo{
			{CalleeName: "f"},
		},
	}
	if !info.HasSiblingCallExpansion() {
		t.Fatal("HasSiblingCallExpansion should be true for multi-target single-call assignment")
	}

	info = &AssignInfo{
		Targets:     []AssignTarget{{Kind: TargetIdent}, {Kind: TargetIdent}, {Kind: TargetIdent}},
		Sources:     []ast.Expr{&ast.NumberExpr{Value: "1"}, &ast.FuncCallExpr{}},
		SourceCalls: []*CallInfo{nil, {CalleeName: "f"}},
	}
	if !info.HasSiblingCallExpansion() {
		t.Fatal("HasSiblingCallExpansion should be true for trailing call expansion")
	}

	info = &AssignInfo{
		Targets:     []AssignTarget{{Kind: TargetIdent}},
		Sources:     []ast.Expr{&ast.FuncCallExpr{}},
		SourceCalls: []*CallInfo{{CalleeName: "f"}},
	}
	if info.HasSiblingCallExpansion() {
		t.Fatal("HasSiblingCallExpansion should be false for single-target assignment")
	}

	info = &AssignInfo{
		Targets:     []AssignTarget{{Kind: TargetIdent}, {Kind: TargetIdent}},
		Sources:     []ast.Expr{&ast.FuncCallExpr{}},
		SourceCalls: []*CallInfo{nil},
	}
	if info.HasSiblingCallExpansion() {
		t.Fatal("HasSiblingCallExpansion should be false when source call is nil")
	}
}

func TestAssignInfo_ExpandingSourceCall(t *testing.T) {
	info := &AssignInfo{
		Targets:     []AssignTarget{{Kind: TargetIdent}, {Kind: TargetIdent}, {Kind: TargetIdent}},
		Sources:     []ast.Expr{&ast.NumberExpr{Value: "1"}, &ast.FuncCallExpr{}},
		SourceCalls: []*CallInfo{nil, {CalleeName: "tail"}},
	}

	call, start := info.ExpandingSourceCall()
	if call == nil || call.CalleeName != "tail" || start != 1 {
		t.Fatalf("ExpandingSourceCall mismatch: call=%+v start=%d", call, start)
	}

	info = &AssignInfo{
		Targets:     []AssignTarget{{Kind: TargetIdent}, {Kind: TargetIdent}},
		Sources:     []ast.Expr{&ast.FuncCallExpr{}, &ast.NumberExpr{Value: "2"}},
		SourceCalls: []*CallInfo{{CalleeName: "head"}, nil},
	}
	call, _ = info.ExpandingSourceCall()
	if call != nil {
		t.Fatalf("ExpandingSourceCall should be nil when only non-trailing source is a call, got %+v", call)
	}
}

func TestAssignInfo_EachSourceCall(t *testing.T) {
	info := &AssignInfo{
		SourceCalls: []*CallInfo{
			{CalleeName: "a"},
			nil,
			{CalleeName: "c"},
		},
	}
	var got []string
	info.EachSourceCall(func(i int, call *CallInfo) {
		got = append(got, call.CalleeName)
		if (i != 0 && i != 2) || call == nil {
			t.Fatalf("unexpected callback payload: i=%d call=%+v", i, call)
		}
	})
	if len(got) != 2 || got[0] != "a" || got[1] != "c" {
		t.Fatalf("EachSourceCall mismatch: got %v", got)
	}
}

func TestAssignInfo_EachSource(t *testing.T) {
	info := &AssignInfo{
		Sources: []ast.Expr{
			&ast.StringExpr{Value: "a"},
			nil,
			&ast.NumberExpr{Value: "1"},
		},
	}
	var idxs []int
	info.EachSource(func(i int, src ast.Expr) {
		if src == nil {
			t.Fatalf("EachSource should not yield nil src at index %d", i)
		}
		idxs = append(idxs, i)
	})
	if len(idxs) != 2 || idxs[0] != 0 || idxs[1] != 2 {
		t.Fatalf("EachSource indexes mismatch: got %v", idxs)
	}
}

func TestAssignInfo_EachTarget(t *testing.T) {
	info := &AssignInfo{
		Targets: []AssignTarget{
			{Kind: TargetIdent, Name: "x"},
			{Kind: TargetIdent, Name: "y"},
		},
	}
	var names []string
	info.EachTarget(func(i int, target AssignTarget) {
		if i < 0 || i > 1 {
			t.Fatalf("unexpected target index %d", i)
		}
		names = append(names, target.Name)
	})
	if len(names) != 2 || names[0] != "x" || names[1] != "y" {
		t.Fatalf("EachTarget names mismatch: got %v", names)
	}
}

func TestAssignInfo_EachTargetSource(t *testing.T) {
	info := &AssignInfo{
		Targets: []AssignTarget{
			{Kind: TargetIdent, Name: "a"},
			{Kind: TargetIdent, Name: "b"},
			{Kind: TargetIdent, Name: "c"},
		},
		Sources: []ast.Expr{
			&ast.StringExpr{Value: "x"},
			nil,
		},
	}

	var names []string
	var nilSources int
	info.EachTargetSource(func(i int, target AssignTarget, src ast.Expr) {
		names = append(names, target.Name)
		if (i == 1 || i == 2) && src == nil {
			nilSources++
		}
	})

	if len(names) != 3 || names[0] != "a" || names[1] != "b" || names[2] != "c" {
		t.Fatalf("EachTargetSource target order mismatch: got %v", names)
	}
	if nilSources != 2 {
		t.Fatalf("EachTargetSource nil source count mismatch: got %d", nilSources)
	}
}

// TestCallInfo_Fields tests CallInfo fields.
func TestCallInfo_Fields(t *testing.T) {
	info := &CallInfo{
		CalleeName:    "print",
		Method:        "",
		ReceiverName:  "",
		IsStmt:        true,
		IsTypeCheck:   false,
		TypeCheckName: "",
		ArgNames:      []string{"a", ""},
	}

	if info.CalleeName != "print" {
		t.Error("CalleeName should be print")
	}
	if !info.IsStmt {
		t.Error("IsStmt should be true")
	}
	if info.IsTypeCheck {
		t.Error("IsTypeCheck should be false")
	}
}

// TestReturnInfo_Fields tests ReturnInfo fields.
func TestReturnInfo_Fields(t *testing.T) {
	info := &ReturnInfo{
		Names:   []string{"x", ""},
		Symbols: []basecfg.SymbolID{1, 0},
	}

	if len(info.Names) != 2 {
		t.Error("Should have 2 names")
	}
	if info.Names[0] != "x" {
		t.Error("First name should be x")
	}
}

func TestReturnInfo_SourceCallAt(t *testing.T) {
	info := &ReturnInfo{
		SourceCalls: []*CallInfo{
			{CalleeName: "x"},
			nil,
		},
	}

	if got := info.SourceCallAt(0); got == nil || got.CalleeName != "x" {
		t.Fatalf("SourceCallAt(0) mismatch: got %+v", got)
	}
	if got := info.SourceCallAt(1); got != nil {
		t.Fatalf("SourceCallAt(1) should be nil, got %+v", got)
	}
	if got := info.SourceCallAt(2); got != nil {
		t.Fatalf("SourceCallAt(2) should be nil, got %+v", got)
	}
}

func TestReturnInfo_EachSourceCall(t *testing.T) {
	info := &ReturnInfo{
		SourceCalls: []*CallInfo{
			{CalleeName: "x"},
			nil,
			{CalleeName: "z"},
		},
	}
	var got []string
	info.EachSourceCall(func(i int, call *CallInfo) {
		got = append(got, call.CalleeName)
		if (i != 0 && i != 2) || call == nil {
			t.Fatalf("unexpected callback payload: i=%d call=%+v", i, call)
		}
	})
	if len(got) != 2 || got[0] != "x" || got[1] != "z" {
		t.Fatalf("EachSourceCall mismatch: got %v", got)
	}
}

// TestBranchInfo_Fields tests BranchInfo fields.
func TestBranchInfo_Fields(t *testing.T) {
	info := &BranchInfo{
		CondVar:   "x",
		CondCheck: basecfg.CondCheck{Kind: basecfg.CheckNil},
	}

	if info.CondVar != "x" {
		t.Error("CondVar should be x")
	}
	if info.CondCheck.Kind != basecfg.CheckNil {
		t.Error("CondCheck.Kind should be CheckNil")
	}
}

// TestTypeDefInfo_Fields tests TypeDefInfo fields.
func TestTypeDefInfo_Fields(t *testing.T) {
	info := &TypeDefInfo{
		Name: "MyType",
		TypeParams: []TypeParamInfo{
			{Name: "T"},
		},
	}

	if info.Name != "MyType" {
		t.Error("Name should be MyType")
	}
	if len(info.TypeParams) != 1 {
		t.Error("Should have 1 type param")
	}
}

// TestFuncDefInfo_Fields tests FuncDefInfo fields.
func TestFuncDefInfo_Fields(t *testing.T) {
	info := &FuncDefInfo{
		TargetKind: FuncDefMethod,
		Name:       "doIt",
		IsMethod:   true,
	}

	if info.TargetKind != FuncDefMethod {
		t.Error("TargetKind should be FuncDefMethod")
	}
	if info.Name != "doIt" {
		t.Error("Name should be doIt")
	}
	if !info.IsMethod {
		t.Error("IsMethod should be true")
	}
}

// TestPhiInfo_Fields tests PhiInfo fields.
func TestPhiInfo_Fields(t *testing.T) {
	info := &PhiInfo{
		Target: Version{Root: "x", ID: 3},
		Operands: []PhiOperand{
			{From: 1, Version: Version{Root: "x", ID: 1}},
			{From: 2, Version: Version{Root: "x", ID: 2}},
		},
	}

	if info.Target.Root != "x" || info.Target.ID != 3 {
		t.Error("Target should be x@3")
	}
	if len(info.Operands) != 2 {
		t.Error("Should have 2 operands")
	}
}

// TestPhiOperand_Fields tests PhiOperand fields.
func TestPhiOperand_Fields(t *testing.T) {
	op := PhiOperand{
		From:    basecfg.Point(5),
		Version: Version{Root: "y", ID: 2},
	}

	if op.From != 5 {
		t.Error("From should be 5")
	}
	if op.Version.Root != "y" || op.Version.ID != 2 {
		t.Error("Version should be y@2")
	}
}

// TestNestedFunc_Fields tests NestedFunc fields.
func TestNestedFunc_Fields(t *testing.T) {
	nf := NestedFunc{
		Point: basecfg.Point(10),
		Func:  nil,
	}

	if nf.Point != 10 {
		t.Error("Point should be 10")
	}
}

// TestNumericForInfo_Fields tests NumericForInfo fields.
func TestNumericForInfo_Fields(t *testing.T) {
	info := &NumericForInfo{
		VarName: "i",
	}

	if info.VarName != "i" {
		t.Error("VarName should be i")
	}
}

// TestTypeParamInfo_Fields tests TypeParamInfo fields.
func TestTypeParamInfo_Fields(t *testing.T) {
	info := TypeParamInfo{
		Name:       "T",
		Constraint: nil,
	}

	if info.Name != "T" {
		t.Error("Name should be T")
	}
}

// TestSymbolID_Zero tests basecfg.SymbolID zero value.
func TestSymbolID_Zero(t *testing.T) {
	var sym basecfg.SymbolID
	if sym != 0 {
		t.Error("Zero value should be 0")
	}
}

// TestPoint_Alias tests Point type alias.
func TestPoint_Alias(t *testing.T) {
	var p = basecfg.Point(5)
	if p != 5 {
		t.Error("Point should be aliased correctly")
	}
}

// TestNodeKind_Constants tests node kind constants match base.
func TestNodeKind_Constants(t *testing.T) {
	if NodeEntry != basecfg.NodeEntry {
		t.Error("NodeEntry mismatch")
	}
	if NodeExit != basecfg.NodeExit {
		t.Error("NodeExit mismatch")
	}
	if NodeAssign != basecfg.NodeAssign {
		t.Error("NodeAssign mismatch")
	}
	if NodeCall != basecfg.NodeCall {
		t.Error("NodeCall mismatch")
	}
	if NodeBranch != basecfg.NodeBranch {
		t.Error("NodeBranch mismatch")
	}
	if NodeJoin != basecfg.NodeJoin {
		t.Error("NodeJoin mismatch")
	}
	if NodeReturn != basecfg.NodeReturn {
		t.Error("NodeReturn mismatch")
	}
	if NodeScopeEnter != basecfg.NodeScopeEnter {
		t.Error("NodeScopeEnter mismatch")
	}
	if NodeScopeExit != basecfg.NodeScopeExit {
		t.Error("NodeScopeExit mismatch")
	}
	if NodeTypeDef != basecfg.NodeTypeDef {
		t.Error("NodeTypeDef mismatch")
	}
}
