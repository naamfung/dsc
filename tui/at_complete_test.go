package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/bubbles/v2/textarea"
)

func TestActiveAtToken(t *testing.T) {
	cases := []struct {
		in   string
		at   int
		tok  string
		want bool
	}{
		{"@foo", 0, "foo", true},
		{"看看 @foo", 7, "foo", true},
		{"看看 @foo bar", 0, "", false}, // token 已被空白结束
		{"a@b", 0, "", false},         // 行中 @ 不触发（邮箱等）
		{"@foo\n", 0, "", false},
		{"@foo\\ bar", 0, "foo\\ bar", true}, // 转义空格属于 token
		{"@sub/dir/", 0, "sub/dir/", true},
	}
	for _, c := range cases {
		at, tok, ok := activeAtToken(c.in)
		if ok != c.want || at != c.at || tok != c.tok {
			t.Errorf("activeAtToken(%q) = (%d,%q,%v), want (%d,%q,%v)", c.in, at, tok, ok, c.at, c.tok, c.want)
		}
	}
}

func TestSplitPathToken(t *testing.T) {
	if dir, frag := splitPathToken("sub/dir/x"); dir != "sub/dir/" || frag != "x" {
		t.Fatalf("got (%q,%q)", dir, frag)
	}
	if dir, frag := splitPathToken("x"); dir != "" || frag != "x" {
		t.Fatalf("got (%q,%q)", dir, frag)
	}
	if dir, frag := splitPathToken("sub/dir/"); dir != "sub/dir/" || frag != "" {
		t.Fatalf("got (%q,%q)", dir, frag)
	}
}

func TestEscapeUnescapeRefPath(t *testing.T) {
	withSpace := "my shot file.png"
	esc := escapeRefPath(withSpace)
	if esc != "my\\ shot\\ file.png" {
		t.Fatalf("escape = %q", esc)
	}
	if got := unescapeRefPath(esc); got != withSpace {
		t.Fatalf("round trip = %q", got)
	}
	// 无反斜杆转义路径（如 Windows 分隔符）保持原样
	win := `C:\Users\a\shot.png`
	if unescapeRefPath(win) != win {
		t.Fatalf("windows backslashes must stay: %q", unescapeRefPath(win))
	}
}

func TestSplitRefTokens(t *testing.T) {
	line := "看 @a.png 与 @my\\ shot.jpg 和 @b.png。"
	got := splitRefTokens(line)
	want := []string{"@a.png", "@my shot.jpg", "@b.png"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("token[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if toks := splitRefTokens("无 at 符号"); len(toks) != 0 {
		t.Fatalf("no @ tokens expected, got %v", toks)
	}
}

func TestFileItems(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DSC_WORKSPACE_ROOT", dir)
	os.MkdirAll(filepath.Join(dir, "docs"), 0o755)
	os.MkdirAll(filepath.Join(dir, ".hidden"), 0o755)
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644)
	os.WriteFile(filepath.Join(dir, "shot.png"), []byte("png"), 0o644)
	os.WriteFile(filepath.Join(dir, "docs", "b.md"), []byte("b"), 0o644)

	m := &Model{}

	// 顶层：目录优先、隐藏项默认跳过
	items := m.fileItems("")
	if len(items) != 3 {
		t.Fatalf("top-level items = %d, want 3 (docs/, a.txt, shot.png)", len(items))
	}
	if items[0].label != "docs/" || !items[0].descend || items[0].insert != "@docs/" {
		t.Fatalf("first item = %+v, want docs/ descend @docs/", items[0])
	}
	foundShot := false
	for _, it := range items {
		if it.label == "shot.png" && it.insert == "@shot.png" {
			foundShot = true
		}
	}
	if !foundShot {
		t.Fatalf("missing shot.png with insert @shot.png: %+v", items)
	}

	// 前缀过滤
	if items = m.fileItems("sh"); len(items) != 1 || items[0].label != "shot.png" {
		t.Fatalf("filtered items = %+v", items)
	}

	// 下钻：选中目录后进入下一层
	if items = m.fileItems("docs/"); len(items) != 1 || items[0].insert != "@docs/b.md" {
		t.Fatalf("descend items = %+v", items)
	}

	// /workspace 虚拟根与工作区根等价
	if items = m.fileItems("/workspace/"); len(items) != 3 {
		t.Fatalf("/workspace listing = %d items, want 3", len(items))
	}
}

// TestAcceptCompletionAtReplacesToken 校验 @ 补全只替换 token（replaceFrom 起），
// 而非整行；唯一匹配项选中后菜单自动关闭（下一次 Enter 走提交），且文件路径后补
// 一个空格便于继续输入。
func TestAcceptCompletionAtReplacesToken(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DSC_WORKSPACE_ROOT", dir)
	os.WriteFile(filepath.Join(dir, "foo.txt"), []byte("x"), 0o644)

	ta := textarea.New()
	ta.SetValue("看看 @fo")
	m := &Model{input: ta}
	m.completion = completion{active: true, kind: compAt, items: m.fileItems("fo"), sel: 0, replaceFrom: strings.Index("看看 @fo", "@")}
	m.acceptCompletion()

	if got := m.input.Value(); got != "看看 @foo.txt " {
		t.Fatalf("value = %q, want %q", got, "看看 @foo.txt ")
	}
	if m.completion.active {
		t.Fatal("menu should close after accepting the single matching file")
	}
}

// TestAcceptCompletionAtDescend 校验选中目录后菜单保持打开并下钻到下一层。
func TestAcceptCompletionAtDescend(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DSC_WORKSPACE_ROOT", dir)
	os.MkdirAll(filepath.Join(dir, "docs"), 0o755)
	os.WriteFile(filepath.Join(dir, "docs", "b.md"), []byte("b"), 0o644)

	ta := textarea.New()
	ta.SetValue("看 @do")
	m := &Model{input: ta}
	m.completion = completion{active: true, kind: compAt, items: m.fileItems("do"), sel: 0, replaceFrom: strings.Index("看 @do", "@")}
	m.acceptCompletion()

	if got := m.input.Value(); got != "看 @docs/" {
		t.Fatalf("value = %q, want %q", got, "看 @docs/")
	}
	// 菜单应保持打开，且候选为下一层（b.md）
	if !m.completion.active || len(m.completion.items) != 1 || m.completion.items[0].insert != "@docs/b.md" {
		t.Fatalf("descend menu = %+v", m.completion)
	}
}

// TestResolveImageRefsEscapedSpace 校验反斜杆转义空格的含空格图片路径仍解析为附件。
func TestResolveImageRefsEscapedSpace(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DSC_ATTACHMENT_DIR", t.TempDir())
	t.Setenv("DSC_WORKSPACE_ROOT", dir)
	os.WriteFile(filepath.Join(dir, "my shot.png"), []byte("png"), 0o644)

	refs := ResolveImageRefs("看 @my\\ shot.png 图")
	if len(refs) != 1 || !strings.HasPrefix(refs[0], "dsc-img://") {
		t.Fatalf("escaped-space image refs = %v, want one dsc-img ref", refs)
	}
}
