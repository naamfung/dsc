package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
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

// TestParsePageSelection 验证页选择字符串解析。
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
