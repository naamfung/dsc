package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// sourceDir 返回本文件的目录，用于定位插件 fonts/ 目录（测试时工作目录可能不是插件目录）。
func sourceDir(t *testing.T) string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(file)
}

// setupCJKEnv 配置测试环境：DSC_WORKSPACE_ROOT 与 TOOL_PDF_FONTS_DIR。
func setupCJKEnv(t *testing.T) string {
	t.Helper()
	srcDir := sourceDir(t)
	fontsDir := filepath.Join(srcDir, "fonts")
	os.Setenv("TOOL_PDF_FONTS_DIR", fontsDir)

	ws := t.TempDir()
	os.Setenv("DSC_WORKSPACE_ROOT", ws)
	t.Cleanup(func() {
		os.Unsetenv("TOOL_PDF_FONTS_DIR")
		os.Unsetenv("DSC_WORKSPACE_ROOT")
	})
	return ws
}

// TestCreateReadChineseRoundTrip 端到端验证中文 PDF 创建并读回：
// 用内置 HarmonyOS Sans SC 字体生成含中文的 PDF，再用 pdf_read_text 读回，断言中文存活。
func TestCreateReadChineseRoundTrip(t *testing.T) {
	ws := setupCJKEnv(t)
	outPath := filepath.Join(ws, "chinese.pdf")

	body := "这是一个用于验证 PDF 中文生成的测试文档。\n第二行也有中文内容，Hello 世界。"

	createArgs, _ := json.Marshal(map[string]any{
		"out_path":  outPath,
		"text":      body,
		"font":      "HarmonyOS_Sans_SC_Regular",
		"font_size": 14,
		"paper":     "A4",
	})
	out, err := handleCreateText(context.Background(), createArgs)
	if err != nil {
		t.Fatalf("create Chinese PDF: %v", err)
	}
	t.Logf("create output:\n%s", out)

	info, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("output not created: %v", err)
	}
	t.Logf("PDF size: %d bytes", info.Size())

	// 读回验证
	sharedCtx.mu.Lock()
	sharedCtx.ctx = nil
	sharedCtx.path = ""
	sharedCtx.mu.Unlock()

	readArgs, _ := json.Marshal(map[string]any{
		"file_path": outPath,
	})
	readOut, err := handleReadText(context.Background(), readArgs)
	if err != nil {
		t.Fatalf("read back Chinese PDF: %v", err)
	}
	t.Logf("read back:\n%s", readOut)

	for _, wanted := range []string{"验证", "中文", "测试文档", "世界"} {
		if !strings.Contains(readOut, wanted) {
			t.Errorf("read-back text missing %q", wanted)
		}
	}
}

// TestCreateReadChineseAppend 验证 pdf_append_text 追加中文页并读回。
func TestCreateReadChineseAppend(t *testing.T) {
	ws := setupCJKEnv(t)
	outPath := filepath.Join(ws, "chinese_append.pdf")

	createArgs, _ := json.Marshal(map[string]any{
		"out_path": outPath,
		"text":     "Original English page.",
	})
	if _, err := handleCreateText(context.Background(), createArgs); err != nil {
		t.Fatalf("create base PDF: %v", err)
	}

	appendArgs, _ := json.Marshal(map[string]any{
		"file_path": outPath,
		"text":      "追加的中文页面内容。",
		"font":      "HarmonyOS_Sans_SC_Regular",
	})
	if _, err := handleAppendText(context.Background(), appendArgs); err != nil {
		t.Fatalf("append Chinese page: %v", err)
	}

	sharedCtx.mu.Lock()
	sharedCtx.ctx = nil
	sharedCtx.path = ""
	sharedCtx.mu.Unlock()

	infoArgs, _ := json.Marshal(map[string]any{"file_path": outPath})
	infoOut, err := handleInfo(context.Background(), infoArgs)
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	t.Logf("info after append:\n%s", infoOut)
	if !strings.Contains(infoOut, "Pages: 2") {
		t.Errorf("expected Pages: 2, got:\n%s", infoOut)
	}

	readArgs, _ := json.Marshal(map[string]any{"file_path": outPath})
	readOut, err := handleReadText(context.Background(), readArgs)
	if err != nil {
		t.Fatalf("read back appended PDF: %v", err)
	}
	t.Logf("read back after append:\n%s", readOut)
	if !strings.Contains(readOut, "中文页面内容") {
		t.Errorf("read-back text missing appended Chinese content")
	}
}
