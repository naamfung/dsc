package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dsc/plugin"
	"dsc/proto"
)

// newTestServer 創建帶臨時 workspace 的測試服務器
func newTestServer(t *testing.T) (*ToolServiceServer, string) {
	t.Helper()
	dir := t.TempDir()
	ws := filepath.Join(dir, "workspace")
	if err := os.MkdirAll(ws, 0755); err != nil {
		t.Fatal(err)
	}
	// 統一工作空間根：handler 現讀 plugin.WorkspaceRoot（宿主經 DSC_WORKSPACE_ROOT 注入），
	// 測試需將根指到臨時 ws 以便斷言寫入位置。
	plugin.WorkspaceRoot = ws
	server := &ToolServiceServer{
		tools:        nil,
		observations: make(map[string]*proto.FsObservation),
	}
	oldCwd, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(oldCwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	return server, dir
}

// exec 執行一次工具調用並返回結果/錯誤
func exec(t *testing.T, server *ToolServiceServer, args map[string]interface{}) (string, error) {
	t.Helper()
	data, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	return strReplaceEditorHandler(context.Background(), server, data)
}

func TestCreateView(t *testing.T) {
	server, _ := newTestServer(t)

	// create
	res, err := exec(t, server, map[string]interface{}{
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
	res, err = exec(t, server, map[string]interface{}{
		"command": "view",
		"path":    "/workspace/test/fib.go",
	})
	if err != nil {
		t.Fatalf("view failed: %v", err)
	}
	if res != "package main\nfunc main() {\n\tprintln(\"hi\")\n}\n" {
		t.Fatalf("unexpected view result: %q", res)
	}
}

func TestStrReplace(t *testing.T) {
	server, _ := newTestServer(t)

	_, err := exec(t, server, map[string]interface{}{
		"command":   "create",
		"path":      "/workspace/test/fib.go",
		"file_text": "package main\nfunc main() {\n\tprintln(\"hi\")\n}\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	// 需先 view 建立觀測狀態
	if _, err = exec(t, server, map[string]interface{}{"command": "view", "path": "/workspace/test/fib.go"}); err != nil {
		t.Fatal(err)
	}

	res, err := exec(t, server, map[string]interface{}{
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

	// 未先 view 的文件應拒絕 str_replace
	server2, _ := newTestServer(t)
	_, err = exec(t, server2, map[string]interface{}{
		"command": "str_replace",
		"path":    "/workspace/x.go",
		"old_str": "a",
		"new_str": "b",
	})
	if err == nil {
		t.Fatalf("str_replace without view should fail")
	}
}

func TestInsert(t *testing.T) {
	server, _ := newTestServer(t)

	_, err := exec(t, server, map[string]interface{}{
		"command":   "create",
		"path":      "/workspace/test/fib.go",
		"file_text": "package main\n\nfunc main() {\n}\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = exec(t, server, map[string]interface{}{"command": "view", "path": "/workspace/test/fib.go"}); err != nil {
		t.Fatal(err)
	}

	res, err := exec(t, server, map[string]interface{}{
		"command":     "insert",
		"path":        "/workspace/test/fib.go",
		"insert_line": 2,
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
	if string(content) != "package main\n// inserted\n\nfunc main() {\n}\n" {
		t.Fatalf("unexpected content after insert: %q", string(content))
	}
}

func TestNormalizeWorkspacePath(t *testing.T) {
	cases := map[string]string{
		"/workspace/test/fib.go": "test/fib.go",
		"/workspace/fib.go":      "fib.go",
		`\workspace\a\b.go`:      `a\b.go`,
		"test/plain.go":          "test/plain.go",
		"workspace/nested.go":    "workspace/nested.go",
	}
	for in, want := range cases {
		if got := normalizeWorkspacePath(in); got != want {
			t.Errorf("normalizeWorkspacePath(%q) = %q, want %q", in, got, want)
		}
	}
}
