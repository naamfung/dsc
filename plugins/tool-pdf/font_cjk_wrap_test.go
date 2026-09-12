package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// TestCreateReadChineseWrap 验证 CJK 折行后的端到端完整性：
// 生成一个超长中文段落（无显式换行）的 PDF，读回后内容完整（折行仅拆分视觉行，不应丢字）。
func TestCreateReadChineseWrap(t *testing.T) {
	ws := setupCJKEnv(t)
	outPath := filepath.Join(ws, "chinese_wrap.pdf")

	paragraph := "这是一个非常长的中文段落，用来验证生成 PDF 时能否按照可用行宽自动折行，从而避免超长的单行文本横向越过页面边界，同时保证全部文字内容在折行之后依然完整无缺。"

	createArgs, _ := json.Marshal(map[string]any{
		"out_path":  outPath,
		"text":      paragraph,
		"font":      cjkFontName,
		"font_size": 14,
		"paper":     "A4",
	})
	if _, err := handleCreateText(context.Background(), createArgs); err != nil {
		t.Fatalf("create wrapped Chinese PDF: %v", err)
	}

	sharedCtx.mu.Lock()
	sharedCtx.ctx = nil
	sharedCtx.path = ""
	sharedCtx.mu.Unlock()

	readArgs, _ := json.Marshal(map[string]any{"file_path": outPath})
	readOut, err := handleReadText(context.Background(), readArgs)
	if err != nil {
		t.Fatalf("read back wrapped Chinese PDF: %v", err)
	}
	t.Logf("read back:\n%s", readOut)

	// 归一化空白 + Unicode 兼容性分解后比对内容完整性
	// （NFKC 归一化处理 CJK 兼容字符差异，如 U+F918 → U+843D 落）
	normalize := func(s string) string {
		return strings.Join(strings.Fields(strings.Map(func(r rune) rune {
			if unicode.IsSpace(r) {
				return ' '
			}
			return r
		}, s)), "")
	}
	// 先做 NFKC 归一化再比对，消除兼容字符差异
	normalizedRead := normalize(string(norm.NFKC.Bytes([]byte(readOut))))
	normalizedPara := normalize(string(norm.NFKC.Bytes([]byte(paragraph))))
	if !strings.Contains(normalizedRead, normalizedPara) {
		t.Errorf("read-back content missing portions of the wrapped paragraph")
	}
}

// TestWrapTextForRenderCJK 直接验证折行辅助函数：
// 超长中文单行在给定行宽下被拆分为多行，且拼接后内容完整（这是避免横向溢出的机制）。
func TestWrapTextForRenderCJK(t *testing.T) {
	setupCJKEnv(t)
	ps, cjk, err := resolveFontName(cjkFontName)
	if err != nil {
		t.Fatalf("resolve CJK font: %v", err)
	}
	if !cjk {
		t.Fatalf("expected %s to be resolved as CJK", cjkFontName)
	}
	if _, err := ensureUserFontDir(); err != nil {
		t.Fatalf("ensure font dir: %v", err)
	}

	const maxWidth = 140.0 // 收窄行宽确保必然折行
	paragraph := strings.Repeat("中文测试段落", 20)

	out, err := wrapTextForRender(paragraph, ps, 14, maxWidth, true)
	if err != nil {
		t.Fatalf("wrapTextForRender: %v", err)
	}
	if len(out) < 2 {
		t.Errorf("expected wrapping into multiple lines under width %g, got %d line(s)", maxWidth, len(out))
	}
	joined := strings.Join(out, "")
	if joined != paragraph {
		t.Errorf("wrap lost content: got %d chars, want %d", len(joined), len(paragraph))
	}

	// 非 CJK 字体不做折行，保持原样
	raw, err := wrapTextForRender(paragraph, "Helvetica", 14, maxWidth, false)
	if err != nil {
		t.Fatalf("wrapTextForRender (latin): %v", err)
	}
	if len(raw) != 1 || raw[0] != paragraph {
		t.Errorf("expected non-CJK text unmodified, got %d line(s)", len(raw))
	}
}
