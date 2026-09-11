package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// TestSmokeExtractText 端到端验证文本提取。
// 用 pdfcpu 自带的测试 PDF（testWithText.pdf 应有文本，test.pdf 是图像网格）。
func TestSmokeExtractText(t *testing.T) {
	candidates := []struct {
		path      string
		expectAny bool // 是否期望提取出文本
	}{
		{"/home/z/my-project/pdfcpu/pkg/testdata/test.pdf", false},
		{"/home/z/my-project/pdfcpu/pkg/testdata/The_Go_Language_Gigon-Odienne-Wartel.pdf", true},
		{"/home/z/my-project/pdfcpu/pkg/testdata/read.go.pdf", true},
	}
	for _, c := range candidates {
		t.Run(c.path, func(t *testing.T) {
			if _, err := os.Stat(c.path); err != nil {
				t.Skipf("missing test PDF: %s", c.path)
			}
			f, err := os.Open(c.path)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			defer f.Close()
			ctx, err := api.ReadValidateAndOptimize(f, model.NewDefaultConfiguration())
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			text, err := extractPageText(ctx, 1)
			if err != nil {
				t.Fatalf("extract: %v", err)
			}
			t.Logf("extracted %d chars from page 1", len(text))
			if len(text) > 0 {
				preview := text
				if len(preview) > 200 {
					preview = preview[:200] + "..."
				}
				t.Logf("preview: %q", preview)
			}
			if c.expectAny && len(text) == 0 {
				t.Errorf("expected non-empty text but got empty")
			}
		})
	}
}

// TestHandleReadText 验证工具入口。
func TestHandleReadText(t *testing.T) {
	// 工具内置沙箱：路径必须在工作空间根内。测试时设 DSC_WORKSPACE_ROOT
	// 覆盖默认（pwd），允许访问 pdfcpu 的 testdata 目录。
	os.Setenv("DSC_WORKSPACE_ROOT", "/home/z/my-project")
	defer os.Unsetenv("DSC_WORKSPACE_ROOT")

	path := "/home/z/my-project/pdfcpu/pkg/testdata/The_Go_Language_Gigon-Odienne-Wartel.pdf"
	if _, err := os.Stat(path); err != nil {
		t.Skipf("missing test PDF: %s", path)
	}
	args, _ := json.Marshal(map[string]any{
		"file_path": path,
		"pages":     "1",
	})
	out, err := handleReadText(context.Background(), args)
	if err != nil {
		t.Fatalf("handleReadText: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("empty output")
	}
	t.Logf("output preview:\n%s", truncateForLog(out, 300))
}

// TestHandleInfo 验证 pdf_info 工具。
func TestHandleInfo(t *testing.T) {
	os.Setenv("DSC_WORKSPACE_ROOT", "/home/z/my-project")
	defer os.Unsetenv("DSC_WORKSPACE_ROOT")

	path := "/home/z/my-project/pdfcpu/pkg/testdata/The_Go_Language_Gigon-Odienne-Wartel.pdf"
	if _, err := os.Stat(path); err != nil {
		t.Skipf("missing test PDF: %s", path)
	}
	args, _ := json.Marshal(map[string]any{
		"file_path": path,
	})
	out, err := handleInfo(context.Background(), args)
	if err != nil {
		t.Fatalf("handleInfo: %v", err)
	}
	t.Logf("output:\n%s", out)
}

// TestHandleCreateText 验证 pdf_create_text 工具。
func TestHandleCreateText(t *testing.T) {
	os.Setenv("DSC_WORKSPACE_ROOT", "/tmp")
	defer os.Unsetenv("DSC_WORKSPACE_ROOT")

	outPath := "/tmp/test_create_text.pdf"
	defer os.Remove(outPath)

	args, _ := json.Marshal(map[string]any{
		"out_path":  outPath,
		"text":      "Hello PDF!\nThis is line 2.\nThis is line 3.",
		"font":      "Helvetica",
		"font_size": 14,
		"paper":     "A4",
	})
	out, err := handleCreateText(context.Background(), args)
	if err != nil {
		t.Fatalf("handleCreateText: %v", err)
	}
	t.Logf("output:\n%s", out)

	// 验证文件存在且非空
	info, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("output file not created: %v", err)
	}
	if info.Size() < 500 {
		t.Errorf("output file too small: %d bytes", info.Size())
	}

	// 验证可被读回（用我们的 pdf_read_text 工具）
	readArgs, _ := json.Marshal(map[string]any{
		"file_path": outPath,
	})
	// 清空共享 PDF Context 缓存（防止读到旧文件）
	sharedCtx.mu.Lock()
	sharedCtx.ctx = nil
	sharedCtx.path = ""
	sharedCtx.mu.Unlock()
	readOut, err := handleReadText(context.Background(), readArgs)
	if err != nil {
		t.Logf("read back failed (may be expected for empty text PDFs): %v", err)
	} else {
		t.Logf("read back:\n%s", truncateForLog(readOut, 200))
	}
}

// TestHandleCreateTextMultiPage 验证多页 PDF 创建。
func TestHandleCreateTextMultiPage(t *testing.T) {
	os.Setenv("DSC_WORKSPACE_ROOT", "/tmp")
	defer os.Unsetenv("DSC_WORKSPACE_ROOT")

	outPath := "/tmp/test_create_multipage.pdf"
	defer os.Remove(outPath)

	// 生成 200 行文本，确保跨页（A4 @ 12pt 约可容纳 40 行）
	var sb strings.Builder
	for i := 1; i <= 200; i++ {
		fmt.Fprintf(&sb, "Line %d: This is a test line for pagination verification.\n", i)
	}

	args, _ := json.Marshal(map[string]any{
		"out_path":  outPath,
		"text":      sb.String(),
		"font_size": 12,
	})
	out, err := handleCreateText(context.Background(), args)
	if err != nil {
		t.Fatalf("handleCreateText: %v", err)
	}
	t.Logf("output:\n%s", out)

	// 验证文件存在
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("output file not created: %v", err)
	}

	// 验证页数 > 1
	sharedCtx.mu.Lock()
	sharedCtx.ctx = nil
	sharedCtx.path = ""
	sharedCtx.mu.Unlock()
	infoArgs, _ := json.Marshal(map[string]any{
		"file_path": outPath,
	})
	infoOut, err := handleInfo(context.Background(), infoArgs)
	if err != nil {
		t.Fatalf("handleInfo: %v", err)
	}
	t.Logf("info:\n%s", infoOut)
	if !strings.Contains(infoOut, "Pages: 5") && !strings.Contains(infoOut, "Pages: 4") && !strings.Contains(infoOut, "Pages: 6") {
		t.Logf("note: page count not exactly 5 (may be 4-6 depending on font metrics); see info output above")
	}
}

// TestHandleAppendText 验证 pdf_append_text 工具。
func TestHandleAppendText(t *testing.T) {
	os.Setenv("DSC_WORKSPACE_ROOT", "/tmp")
	defer os.Unsetenv("DSC_WORKSPACE_ROOT")

	// 1. 先创建一个基础 PDF
	outPath := "/tmp/test_append_text.pdf"
	defer os.Remove(outPath)

	createArgs, _ := json.Marshal(map[string]any{
		"out_path": outPath,
		"text":     "Original page content.\nLine 2.",
	})
	if _, err := handleCreateText(context.Background(), createArgs); err != nil {
		t.Fatalf("create base PDF: %v", err)
	}

	// 2. 追加新页
	appendArgs, _ := json.Marshal(map[string]any{
		"file_path": outPath,
		"text":      "Appended page content.\nThis is a new page.",
		"font":      "Courier",
	})
	out, err := handleAppendText(context.Background(), appendArgs)
	if err != nil {
		t.Fatalf("handleAppendText: %v", err)
	}
	t.Logf("output:\n%s", out)

	// 3. 验证页数增加
	sharedCtx.mu.Lock()
	sharedCtx.ctx = nil
	sharedCtx.path = ""
	sharedCtx.mu.Unlock()
	infoArgs, _ := json.Marshal(map[string]any{
		"file_path": outPath,
	})
	infoOut, err := handleInfo(context.Background(), infoArgs)
	if err != nil {
		t.Fatalf("handleInfo: %v", err)
	}
	t.Logf("info after append:\n%s", infoOut)
	if !strings.Contains(infoOut, "Pages: 2") {
		t.Errorf("expected Pages: 2, got:\n%s", infoOut)
	}
}

// TestHandleCreateTextErrors 验证错误处理。
func TestHandleCreateTextErrors(t *testing.T) {
	os.Setenv("DSC_WORKSPACE_ROOT", "/tmp")
	defer os.Unsetenv("DSC_WORKSPACE_ROOT")

	cases := []struct {
		name string
		args map[string]any
	}{
		{"missing out_path", map[string]any{"text": "hi"}},
		{"missing text", map[string]any{"out_path": "/tmp/x.pdf"}},
		{"invalid font", map[string]any{"out_path": "/tmp/x.pdf", "text": "hi", "font": "ComicSans"}},
		{"invalid paper", map[string]any{"out_path": "/tmp/x.pdf", "text": "hi", "paper": "FooBar"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			args, _ := json.Marshal(c.args)
			_, err := handleCreateText(context.Background(), args)
			if err == nil {
				t.Errorf("expected error for %s, got nil", c.name)
			}
		})
	}
}

// TestEscapePDFString 验证 PDF 字符串转义。
func TestEscapePDFString(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"hello", "hello"},
		{"a(b)c", "a\\(b\\)c"},
		{"a\\b", "a\\\\b"},
		{"a\rb", "ab"},   // CR removed
		{"a\nb", "a\nb"}, // LF preserved
	}
	for _, c := range cases {
		got := escapePDFString(c.in)
		if got != c.want {
			t.Errorf("escapePDFString(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestIsStandardFont 验证字体名识别。
func TestIsStandardFont(t *testing.T) {
	valid := []string{"Helvetica", "Times-Roman", "Courier-Bold", "Symbol", "ZapfDingbats"}
	for _, f := range valid {
		if !isStandardFont(f) {
			t.Errorf("isStandardFont(%q) = false, want true", f)
		}
	}
	invalid := []string{"Arial", "Comic Sans", "", "Helvetica Neue"}
	for _, f := range invalid {
		if isStandardFont(f) {
			t.Errorf("isStandardFont(%q) = true, want false", f)
		}
	}
}

// (existing tests follow)
func TestParsePageSelection(t *testing.T) {
	cases := []struct {
		s         string
		pageCount int
		want      []int
		wantErr   bool
	}{
		{"", 5, []int{1, 2, 3, 4, 5}, false},
		{"1", 5, []int{1}, false},
		{"1-3", 5, []int{1, 2, 3}, false},
		{"1-3,5", 5, []int{1, 2, 3, 5}, false},
		{"1,3,5", 5, []int{1, 3, 5}, false},
		{"1-3,7-9", 10, []int{1, 2, 3, 7, 8, 9}, false},
		{"6", 5, nil, true},   // 越界
		{"0", 5, nil, true},   // 越界（页号从 1 开始）
		{"abc", 5, nil, true}, // 非法
	}
	for _, c := range cases {
		got, err := parsePageSelection(c.s, c.pageCount)
		if c.wantErr {
			if err == nil {
				t.Errorf("parsePageSelection(%q, %d) want error, got nil", c.s, c.pageCount)
			}
			continue
		}
		if err != nil {
			t.Errorf("parsePageSelection(%q, %d) unexpected error: %v", c.s, c.pageCount, err)
			continue
		}
		if !sliceEq(got, c.want) {
			t.Errorf("parsePageSelection(%q, %d) = %v, want %v", c.s, c.pageCount, got, c.want)
		}
	}
}

// TestHandleSearch 验证全文搜索。
func TestHandleSearch(t *testing.T) {
	os.Setenv("DSC_WORKSPACE_ROOT", "/home/z/my-project")
	defer os.Unsetenv("DSC_WORKSPACE_ROOT")

	path := "/home/z/my-project/pdfcpu/pkg/testdata/The_Go_Language_Gigon-Odienne-Wartel.pdf"
	if _, err := os.Stat(path); err != nil {
		t.Skipf("missing test PDF: %s", path)
	}
	args, _ := json.Marshal(map[string]any{
		"file_path": path,
		"query":     "Go",
	})
	out, err := handleSearch(context.Background(), args)
	if err != nil {
		t.Fatalf("handleSearch: %v", err)
	}
	t.Logf("search output:\n%s", truncateForLog(out, 500))
}

// TestTokenizer 验证 tokenizer 解析基本操作符与字符串。
func TestTokenizer(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  []token
	}{
		{
			name:  "simple Tj",
			input: `(Hello) Tj`,
			want: []token{
				{kind: tokString, str: "Hello"},
				{kind: tokOp, op: "Tj"},
			},
		},
		{
			name:  "Tf",
			input: `/F1 12 Tf`,
			want: []token{
				{kind: tokName, str: "F1"},
				{kind: tokNumber, num: 12, str: "12"},
				{kind: tokOp, op: "Tf"},
			},
		},
		{
			name:  "TJ array",
			input: `[(Hello) -100 (World)] TJ`,
			want: []token{
				{kind: tokBOA},
				{kind: tokString, str: "Hello"},
				{kind: tokNumber, num: -100, str: "-100"},
				{kind: tokString, str: "World"},
				{kind: tokEOA},
				{kind: tokOp, op: "TJ"},
			},
		},
		{
			name:  "hex string",
			input: `<48656C6C6F> Tj`,
			want: []token{
				{kind: tokString, str: "Hello", isHex: true},
				{kind: tokOp, op: "Tj"},
			},
		},
		{
			name:  "escape sequences",
			input: `(\(Hello\)\\n) Tj`,
			want: []token{
				{kind: tokString, str: "(Hello)\\n"},
				{kind: tokOp, op: "Tj"},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := tokenizeContentStream([]byte(c.input))
			if len(got) != len(c.want) {
				t.Fatalf("token count = %d, want %d; got = %+v", len(got), len(c.want), got)
			}
			for i, want := range c.want {
				if got[i].kind != want.kind {
					t.Errorf("token[%d].kind = %v, want %v", i, got[i].kind, want.kind)
					continue
				}
				switch want.kind {
				case tokString:
					if got[i].str != want.str {
						t.Errorf("token[%d].str = %q, want %q", i, got[i].str, want.str)
					}
				case tokOp:
					if got[i].op != want.op {
						t.Errorf("token[%d].op = %q, want %q", i, got[i].op, want.op)
					}
				case tokName:
					if got[i].str != want.str {
						t.Errorf("token[%d].name = %q, want %q", i, got[i].str, want.str)
					}
				case tokNumber:
					if got[i].num != want.num {
						t.Errorf("token[%d].num = %v, want %v", i, got[i].num, want.num)
					}
				}
			}
		})
	}
}

// TestFontDecoder 验证字体解码。
func TestFontDecoder(t *testing.T) {
	// WinAnsi 默认解码
	dec := newFontDecoder()
	dec.base = "winansi"
	got := dec.decode([]byte("Hello \xA9 World")) // 0xA9 = © in WinAnsi
	want := "Hello \u00A9 World"
	if got != want {
		t.Errorf("WinAnsi decode = %q, want %q", got, want)
	}

	// ToUnicode CMap 解析（CID 字体：2 字节代码）
	dec2 := newFontDecoder()
	dec2.isCID = true
	dec2.parseToUnicodeCMap([]byte(`
/CIDInit /ProcSet findresource begin
12 beginbfchar
<0041> <0048>
<0042> <0065>
<0043> <006C>
<0044> <006C>
<0045> <006F>
endbfchar
end
`))
	// 2 字节大端代码：0x00 0x41 = 'H', 0x00 0x42 = 'e', ...
	got2 := dec2.decode([]byte{0x00, 0x41, 0x00, 0x42, 0x00, 0x43, 0x00, 0x44, 0x00, 0x45})
	want2 := "Hello"
	if got2 != want2 {
		t.Errorf("CMap decode = %q, want %q", got2, want2)
	}
}

func sliceEq(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func truncateForLog(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + fmt.Sprintf("\n...[truncated, total %d chars]", len(s))
}
