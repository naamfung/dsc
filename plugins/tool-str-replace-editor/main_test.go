package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"dsc/core"
)

// newTestWS 建立带臨時 workspace 的測試環境（統一工作空間根：handler 讀
// core.WorkspaceRoot，宿主經 DSC_WORKSPACE_ROOT 注入；測試直接置變數指到臨時 ws）
func newTestWS(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	ws := filepath.Join(dir, "workspace")
	if err := os.MkdirAll(ws, 0755); err != nil {
		t.Fatal(err)
	}
	core.WorkspaceRoot = ws
	oldCwd, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(oldCwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	return dir
}

// exec 執行一次工具調用並返回結果/錯誤
func exec(t *testing.T, args map[string]interface{}) (string, error) {
	t.Helper()
	data, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	return strReplaceEditorHandler(context.Background(), data)
}

func TestCreateView(t *testing.T) {
	newTestWS(t)

	// create
	res, err := exec(t, map[string]interface{}{
		"command":   "create",
		"path":      "/workspace/test/fib.go",
		"file_text": "package main\nfunc main() {\n\tprintln(\"hi\")\n}\n",
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if !strings.HasPrefix(res, "File created successfully.") {
		t.Fatalf("unexpected create result: %q", res)
	}
	// 写操作结果应附带 unified diff（相对路径文件头 + hunk + 全新增行）
	if !strings.Contains(res, "--- a/test/fib.go") || !strings.Contains(res, "+++ b/test/fib.go") || !strings.Contains(res, "@@") || !strings.Contains(res, "+package main") {
		t.Fatalf("create 结果应含 unified diff: %q", res)
	}

	// 文件應存在於 workspace/test/fib.go
	if _, err := os.Stat(filepath.Join("workspace", "test", "fib.go")); err != nil {
		t.Fatalf("file not created: %v", err)
	}

	// view
	res, err = exec(t, map[string]interface{}{
		"command": "view",
		"path":    "/workspace/test/fib.go",
	})
	if err != nil {
		t.Fatalf("view failed: %v", err)
	}
	// view 返回带行号的内容（对齐 DSH cat -n 风格）
	if !strings.Contains(res, "     1  package main") {
		t.Fatalf("unexpected view result: %q", res)
	}
}

func TestStrReplace(t *testing.T) {
	newTestWS(t)

	_, err := exec(t, map[string]interface{}{
		"command":   "create",
		"path":      "/workspace/test/fib.go",
		"file_text": "package main\nfunc main() {\n\tprintln(\"hi\")\n}\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	// 先 view 贴近真实调用顺序（读前改写裁决由 policy 插件承载，编辑器自身不再检查）
	if _, err = exec(t, map[string]interface{}{"command": "view", "path": "/workspace/test/fib.go"}); err != nil {
		t.Fatal(err)
	}

	res, err := exec(t, map[string]interface{}{
		"command": "str_replace",
		"path":    "/workspace/test/fib.go",
		"old_str": `println("hi")`,
		"new_str": `fmt.Println("hi")`,
	})
	if err != nil {
		t.Fatalf("str_replace failed: %v", err)
	}
	if !strings.HasPrefix(res, "File replaced successfully.") {
		t.Fatalf("unexpected str_replace result: %q", res)
	}
	// 应附 diff：删行 -println、加行 +fmt.Println
	if !strings.Contains(res, "-\tprintln(\"hi\")") || !strings.Contains(res, "+\tfmt.Println(\"hi\")") {
		t.Fatalf("str_replace 结果应含 unified diff: %q", res)
	}

	content, _ := os.ReadFile(filepath.Join("workspace", "test", "fib.go"))
	if string(content) != "package main\nfunc main() {\n\tfmt.Println(\"hi\")\n}\n" {
		t.Fatalf("unexpected content after replace: %q", string(content))
	}

	// old_str 确实未匹配时：应返回简洁错误（含 did not appear verbatim），
	// 且不得把整个文件内容 dump 进错误消息（回归：if 判断曾被误删导致无条件报错）
	_, err = exec(t, map[string]interface{}{
		"command": "str_replace",
		"path":    "/workspace/test/fib.go",
		"old_str": "func nonexistent()",
		"new_str": "func replaced()",
	})
	if err == nil {
		t.Fatal("str_replace with missing old_str should fail")
	}
	if !strings.Contains(err.Error(), "did not appear verbatim") {
		t.Fatalf("unexpected missing-old_str error: %v", err)
	}
	if strings.Contains(err.Error(), "package main") {
		t.Fatalf("missing-old_str error should not dump file content: %v", err)
	}
}

func TestInsert(t *testing.T) {
	newTestWS(t)

	_, err := exec(t, map[string]interface{}{
		"command":   "create",
		"path":      "/workspace/test/fib.go",
		"file_text": "package main\n\nfunc main() {\n}\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = exec(t, map[string]interface{}{"command": "view", "path": "/workspace/test/fib.go"}); err != nil {
		t.Fatal(err)
	}

	// insert_line=2 在 DSH 语义中是 0-based AFTER：在第 2 行之后插入
	// 文件有 3 行：package main / 空行 / func main... 插入到第 2 行之后 = 空行之后
	res, err := exec(t, map[string]interface{}{
		"command":     "insert",
		"path":        "/workspace/test/fib.go",
		"insert_line": 1, // 0-based AFTER line 1 = 在第 1 行之后（空行之前）
		"new_str":     "// inserted",
	})
	if err != nil {
		t.Fatalf("insert failed: %v", err)
	}
	if !strings.HasPrefix(res, "File inserted successfully.") {
		t.Fatalf("unexpected insert result: %q", res)
	}
	// 应附 diff：新增行 +// inserted
	if !strings.Contains(res, "+// inserted") {
		t.Fatalf("insert 结果应含 unified diff: %q", res)
	}

	content, _ := os.ReadFile(filepath.Join("workspace", "test", "fib.go"))
	// insert_line=1 意为 AFTER line 1（0-based），即在第一行之后插入
	// 文件内容：package main / 空 / func main... → 插入后：package main / // inserted / 空 / func main...
	if string(content) != "package main\n// inserted\n\nfunc main() {\n}\n" {
		t.Fatalf("unexpected content after insert: %q", string(content))
	}
}

// TestNormalizeWorkspacePath 已随 normalizeWorkspacePath 删除：/workspace 前缀
// 剥离与裸 / 锚定统一由 core.MapWorkspacePath 承担（规则矩阵见 core/workspace_test.go，
// 入口接线见 TestVirtualRootMappedToWorkspaceRoot），插件内不再保留第二份实现。

// TestViewDirectoryRejected 对齐 DSH：view 目录时列出 2 层深度的文件/目录（而非报错）。
func TestViewDirectoryRejected(t *testing.T) {
	dir := newTestWS(t)
	ws := filepath.Join(dir, "workspace")

	res, err := exec(t, map[string]interface{}{
		"command": "view",
		"path":    ws,
	})
	if err != nil {
		t.Fatalf("view on directory should list contents (对齐 DSH), got error: %v", err)
	}
	if !strings.Contains(res, "files and directories") {
		t.Fatalf("expected directory listing, got: %q", res)
	}
}

// TestSlashErr 断言文件系统错误的路径字段被归一为正斜杆（Windows 上 os.* 错误
// 内嵌反斜杆路径，透传前须统一展示风格）。实现已委托 core.SlashErr（公共实现
// 见 core/errslash.go），此处保留行为级断言防止委托链回退。
func TestSlashErr(t *testing.T) {
	err := &os.PathError{Op: "read", Path: `D:\a\b.txt`, Err: errors.New("Incorrect function.")}
	got := slashErr(err).Error()
	if got != "read D:/a/b.txt: Incorrect function." {
		t.Fatalf("slashErr = %q, want %q", got, "read D:/a/b.txt: Incorrect function.")
	}
	if slashErr(nil) != nil {
		t.Fatal("slashErr(nil) should be nil")
	}
}

// TestTopLevelSlashErrOnMissingPath 回归测试（真实复现报告场景）：view 一个
// 不存在的路径（父目录亦不存在）时，safePath 返回 EvalSymlinks(parent) 的原生
// *os.PathError——历史回归即漏在此分支：内嵌反斜杆原生路径未经归一直接透传
// （Windows 实测表现为「GetFileAttributesEx D:\Agents\...: The system cannot
// find the file specified.」）。经包级入口 strReplaceEditor 顶层归一后，错误
// 文本不得再含任何反斜杆；路径反斜杆同样不得残留在错误信息里。Linux 上传入
// 的反斜杆被 filepath.Abs 原样保留进 PathError.Path，同样能验证归一链路。
func TestTopLevelSlashErrOnMissingPath(t *testing.T) {
	newTestWS(t)

	for _, path := range []string{
		`D:\nonexistent-dir\nonexistent-file.txt`, // Windows 反斜杆原生形式
		"D:/nonexistent-dir/nonexistent-file.txt", // 正斜杆形式（亦不存在）
	} {
		_, err := strReplaceEditor(context.Background(),
			[]byte(`{"command":"view","path":`+strconv.Quote(path)+`}`))
		if err == nil {
			t.Fatalf("view on missing path %q should fail", path)
		}
		if strings.Contains(err.Error(), `\`) {
			t.Fatalf("error text for %q should be slash-normalized, got %q", path, err.Error())
		}
	}
}

// TestWithinBaseForwardSlash 回归测试：withinBase 必须用 "/" 而非
// string(os.PathSeparator) 作为路径分隔符。safePath 的入参来自 dsc.PAbs /
// dsc.PJoin / dsc.PClean / core.CanonicalPath，这些 P* 函数在 Windows 上
// 也返回正斜杆结果。若 withinBase 用 string(os.PathSeparator)（Windows 上
// 是反斜杆），前缀检查永远失败，导致所有相对路径都被拒绝（报
// "permission denied"）。此测试在所有平台上断言正斜杆路径的前缀检查生效。
func TestWithinBaseForwardSlash(t *testing.T) {
	cases := []struct {
		name   string
		real   string
		base   string
		expect bool
	}{
		{"forward slash child", "/tmp/ws/README.md", "/tmp/ws", true},
		{"forward slash equal", "/tmp/ws", "/tmp/ws", true},
		{"forward slash sibling", "/tmp/other/README.md", "/tmp/ws", false},
		{"forward slash not prefix", "/tmp/ws-other/README.md", "/tmp/ws", false},
		// Windows 盘符正斜杆形式（P* 函数返回形态）
		{"windows drive forward slash", "G:/Dev/quark-go/README.md", "G:/Dev/quark-go", true},
		{"windows drive forward slash equal", "G:/Dev/quark-go", "G:/Dev/quark-go", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := withinBase(c.real, c.base)
			if got != c.expect {
				t.Errorf("withinBase(%q, %q) = %v, want %v", c.real, c.base, got, c.expect)
			}
		})
	}
}

// TestViewByAbsolutePathInsideWorkspace 回归测试（真实复现报告场景）：
// 模型传入 workspace 内文件的真实绝对路径（如 G:/Dev/quark-go/README.md），
// 经 MapWorkspacePath + filepath.Rel 归并为相对路径后，safePath 走相对分支，
// withinBase 前缀检查必须通过（不得报 "permission denied"）。Linux 上用
// /tmp/xxx/workspace/README.md 等价覆盖。
func TestViewByAbsolutePathInsideWorkspace(t *testing.T) {
	dir := newTestWS(t)
	ws := filepath.Join(dir, "workspace")
	content := "# README\nabsolute path view works"
	if err := os.WriteFile(filepath.Join(ws, "README.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// 用 workspace 内文件的真实绝对路径调用 view
	out, err := strReplaceEditor(context.Background(),
		[]byte(`{"command":"view","path":`+strconv.Quote(filepath.Join(ws, "README.md"))+`}`))
	if err != nil {
		t.Fatalf("view by absolute path inside workspace should succeed, got error: %v", err)
	}
	if !strings.Contains(out, "absolute path view works") {
		t.Fatalf("view output missing file content, got %q", out)
	}
}

// TestVirtualRootMappedToWorkspaceRoot 回归测试（虚拟根映射报告场景）：模型按
// 「虚拟根 = 工作空间根」契约传入 /workspace/docs/architecture.md，必须映射到
// 工作空间根下的真实文件并成功读出内容。裸 / 形态保持真实根语义（Windows 上为
// 当前盘根，与 Linux 一致），由 core/workspace_test.go 纯函数矩阵覆盖；工作区
// 文件须显式 /workspace 前缀，此处验证入口接线。
func TestVirtualRootMappedToWorkspaceRoot(t *testing.T) {
	newTestWS(t)
	ws := core.WorkspaceRoot

	dir := filepath.Join(ws, "docs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "# Architecture\nvirtual root mapping works"
	if err := os.WriteFile(filepath.Join(dir, "architecture.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := strReplaceEditor(context.Background(),
		[]byte(`{"command":"view","path":"/workspace/docs/architecture.md"}`))
	if err != nil {
		t.Fatalf("view via /workspace virtual root: %v", err)
	}
	if !strings.Contains(out, "virtual root mapping works") {
		t.Fatalf("view output missing file content, got %q", out)
	}
}
