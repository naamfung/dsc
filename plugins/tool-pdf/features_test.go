package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestDetectAndRenderTable 验证表格检测与对齐网格输出。
func TestDetectAndRenderTable(t *testing.T) {
	rows := []textLine{
		{y: 100, frag: []textFragment{{x: 50, s: "Name", rot: 0, fontSize: 12}, {x: 200, s: "Qty", rot: 0, fontSize: 12}}},
		{y: 80, frag: []textFragment{{x: 50, s: "Apple", rot: 0, fontSize: 12}, {x: 200, s: "3", rot: 0, fontSize: 12}}},
		{y: 60, frag: []textFragment{{x: 50, s: "Banana", rot: 0, fontSize: 12}, {x: 200, s: "5", rot: 0, fontSize: 12}}},
	}
	cols, ok := detectTableColumns(rows)
	if !ok {
		t.Fatal("expected table detected")
	}
	if len(cols) != 2 {
		t.Fatalf("expected 2 columns, got %v", cols)
	}
	out := renderTable(rows, cols, medianFontSize(rows)*tableTolEm)
	for _, want := range []string{"Name | Qty", "Apple | 3", "Banana | 5"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q:\n%s", want, out)
		}
	}

	// 非对齐文本：不应误判为表格
	flat := []textLine{
		{y: 100, frag: []textFragment{{x: 50, s: "hello", rot: 0, fontSize: 12}}},
		{y: 80, frag: []textFragment{{x: 50, s: "world today", rot: 0, fontSize: 12}}},
	}
	if _, ok := detectTableColumns(flat); ok {
		t.Errorf("unexpected table detection on non-tabular text")
	}
}

// TestContiguousAndRenderArgs 验证页段归并与渲染命令构造。
func TestContiguousAndRenderArgs(t *testing.T) {
	f, l := contiguousPages([]int{3, 1, 5})
	if f != 1 || l != 5 {
		t.Errorf("contiguousPages = (%d,%d), want (1,5)", f, l)
	}
	r := &rasterizer{family: "pdftoppm"}
	got := r.renderArgs("in.pdf", "pre", 1, 3, 150)
	want := []string{"-f", "1", "-l", "3", "-r", "150", "-png", "in.pdf", "pre"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("pdftoppm args = %v, want %v", got, want)
	}
	if familyFor("/usr/bin/mutool") != "mutool" {
		t.Errorf("familyFor(mutool) wrong")
	}
	// 无渲染器时 findRasterizer 可能返回 nil（本机未装），不应 panic
	_ = findRasterizer()
}

// TestMergeSplitExtract 端到端验证合并 / 拆分 / 抽页（用 handleCreateText 造样本文档）。
func TestMergeSplitExtract(t *testing.T) {
	ws := setupCJKEnv(t)

	// 1. 造两个样本文档
	f1 := filepath.Join(ws, "a.pdf")
	f2 := filepath.Join(ws, "b.pdf")
	create := func(path, text string) {
		t.Helper()
		args, _ := json.Marshal(map[string]any{"out_path": path, "text": text})
		if _, err := handleCreateText(context.Background(), args); err != nil {
			t.Fatalf("create %s: %v", path, err)
		}
	}
	create(f1, "Doc A page one.\nDoc A second line.")
	create(f2, "Doc B page one.\nDoc B second line.\nDoc B third line.")

	// 2. 合并
	merged := filepath.Join(ws, "merged.pdf")
	mergeArgs, _ := json.Marshal(map[string]any{"input_paths": []string{f1, f2}, "out_path": merged})
	if _, err := handleMergePDFs(context.Background(), mergeArgs); err != nil {
		t.Fatalf("merge: %v", err)
	}
	mergedPages := readPageCount(t, merged)
	if mergedPages < 2 {
		t.Fatalf("merged page count = %d, want >= 2", mergedPages)
	}

	// 3. 拆分（每 1 页一段）
	splitDir := filepath.Join(ws, "split")
	splitArgs, _ := json.Marshal(map[string]any{"file_path": merged, "out_dir": splitDir, "span": 1})
	if _, err := handleSplitPDFs(context.Background(), splitArgs); err != nil {
		t.Fatalf("split: %v", err)
	}
	files := listPDFs(splitDir)
	if len(files) != mergedPages {
		t.Errorf("split produced %d files, want %d", len(files), mergedPages)
	}

	// 4. 抽页（第 1 页）生成新 PDF
	extractOut := filepath.Join(ws, "extracted.pdf")
	extractArgs, _ := json.Marshal(map[string]any{"file_path": merged, "out_path": extractOut, "pages": "1"})
	if _, err := handleExtractPagesTool(context.Background(), extractArgs); err != nil {
		t.Fatalf("extract pages: %v", err)
	}
	if n := readPageCount(t, extractOut); n != 1 {
		t.Errorf("extracted page count = %d, want 1", n)
	}
}

// readPageCount 读取 PDF 页数（经 handleInfo）。
func readPageCount(t *testing.T, path string) int {
	t.Helper()
	sharedCtx.mu.Lock()
	sharedCtx.ctx = nil
	sharedCtx.path = ""
	sharedCtx.mu.Unlock()
	args, _ := json.Marshal(map[string]any{"file_path": path})
	out, err := handleInfo(context.Background(), args)
	if err != nil {
		t.Fatalf("info %s: %v", path, err)
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "Pages: ") {
			var n int
			if _, err := fmt.Sscanf(line[len("Pages: "):], "%d", &n); err == nil {
				return n
			}
		}
	}
	return 0
}
