package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestRenderTJItems 验证 TJ 词间隔判定：
// 单次大间距→词间隔；连续 ≥3 个大间距→均匀字距（不插空格）；小间距→不插。
func TestRenderTJItems(t *testing.T) {
	cases := []struct {
		name  string
		items []tjItem
		want  string
	}{
		{
			"isolated word gap",
			[]tjItem{{text: "Hello"}, {text: "World", gapEm: 0.3}},
			"Hello World",
		},
		{
			"small gap no space",
			[]tjItem{{text: "ab"}, {text: "cd", gapEm: 0.05}},
			"abcd",
		},
		{
			"two-word justified preserved",
			// 两个词间隔（run 长度 2）→ 保留空格
			[]tjItem{{text: "one"}, {text: "two", gapEm: 0.4}, {text: "three", gapEm: 0.4}},
			"one two three",
		},
		{
			"letterspacing run suppressed",
			// 连续 4 个大间距（逐字加宽）→ 不插空格
			[]tjItem{{text: "A"}, {text: "B", gapEm: 0.8}, {text: "C", gapEm: 0.8}, {text: "D", gapEm: 0.8}, {text: "E", gapEm: 0.8}},
			"ABCDE",
		},
		{
			"endpoints of long run suppressed",
			[]tjItem{{text: "X"}, {text: "Y", gapEm: 0.9}, {text: "Z", gapEm: 0.9}, {text: "W", gapEm: 0.9}},
			"XYZW",
		},
		{
			"first item no gap",
			[]tjItem{{text: "click"}, {text: "here", gapEm: 0.25}},
			"click here",
		},
	}
	for _, tc := range cases {
		if got := renderTJItems(tc.items); got != tc.want {
			t.Errorf("%s: renderTJItems = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// findCJKFontFile 定位并读取自带简体中文字体的字节（测试用）。
// 使用 cjkFontFile 常量（TestMain 自动下载的字体），不再硬编码特定字体名。
func findCJKFontFile(t *testing.T) []byte {
	t.Helper()
	fsDir := bundledFontsDir()
	if fsDir == "" {
		t.Skip("fonts dir not found; set TOOL_PDF_FONTS_DIR")
	}
	path := filepath.Join(fsDir, cjkFontFile)
	bb, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("%s not found under %s (TestMain should have downloaded it)", cjkFontFile, fsDir)
	}
	return bb
}

// TestGIDToUnicodeFromTTF 验证从内嵌 TrueType 的 cmap 解析出的 GID→Unicode
// 至少覆盖若干 CJK 区汉字——这是读侧无 ToUnicode 时的中文回退基础。
func TestGIDToUnicodeFromTTF(t *testing.T) {
	setupCJKEnv(t)
	m := gidToUnicodeFromTTF(findCJKFontFile(t))
	if len(m) == 0 {
		t.Fatal("expected non-empty GID→Unicode map")
	}
	cjkCount := 0
	latinCount := 0
	for _, s := range m {
		rs := []rune(s)
		if len(rs) != 1 {
			continue
		}
		r := rs[0]
		if r >= 0x4E00 && r <= 0x9FFF {
			cjkCount++
		} else if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			latinCount++
		}
	}
	if cjkCount == 0 {
		t.Errorf("expected at least one CJK ideograph in map, got only %d entries", len(m))
	}
	t.Logf("map entries=%d, CJK=%d, latin=%d", len(m), cjkCount, latinCount)

	// 端到端：用一个 isCID 解码器填充该 fallback 映射，验证「中」(U+4E2D) 的 2 字节 GID 码可被解码。
	// 先找一个 GID→U+4E2D 的条目，再用它的 2 字节代码解码。
	gidStr := ""
	for gid, s := range m {
		if s == "中" {
			gidStr = gid
			break
		}
	}
	if gidStr == "" {
		t.Fatalf("expected U+4E2D (中) in GID→Unicode map")
	}
	gid, _ := strconv.ParseUint(gidStr, 16, 16)
	code := make([]byte, 2)
	binary.BigEndian.PutUint16(code, uint16(gid))
	dec := newFontDecoder()
	dec.isCID = true
	dec.toUnicode = m
	if got := dec.decode(code); got != "中" {
		t.Errorf("decode fallback GID %s = %q, want 中", gidStr, got)
	}
}

// TestLineHeightForFont 验证行距计算：
// 标准 14 字体用 1.5×字号；CJK 用真实字体行高且不低于 1.2×字号。
func TestLineHeightForFont(t *testing.T) {
	setupCJKEnv(t)
	if got := lineHeightForFont("Helvetica", 12, false); got != 18 {
		t.Errorf("Helvetica lineHeight = %v, want 18 (1.5×12)", got)
	}
	// 标准 14 字体走 1.5×字号分支（cjk=false）
	if got := lineHeightForFont("Courier", 12, false); got != 18 {
		t.Errorf("Courier lineHeight = %v, want 18", got)
	}

	ps, cjk, err := resolveFontName(cjkFontName)
	if err != nil || !cjk {
		t.Skipf("skip CJK line-height check: %v (%v)", cjk, err)
	}
	if _, err := ensureUserFontDir(); err != nil {
		t.Fatalf("ensure font dir: %v", err)
	}
	got := lineHeightForFont(ps, 12, true)
	min := 12 * 1.2
	if got < min {
		t.Errorf("CJK lineHeight = %v, want >= floor %v", got, min)
	}
	// CJK 字体的实际行高由字体度量决定，可能远大于 1.5×字号（如 Noto Sans SC 约 34pt）。
	// 关键是它 != 18（标准 14 字体的 1.5×12），证明走的是真实字体度量而非固定值。
	if got == 18 {
		t.Errorf("CJK lineHeight = 18 (1.5×12), expected real font metrics to be used (got %v for %s)", got, ps)
	}
	if !strings.Contains(strings.ToLower(ps), "harmony") {
		t.Logf("CJK psName=%q", ps)
	}
}
