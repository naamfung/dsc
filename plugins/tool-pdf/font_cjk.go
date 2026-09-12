// Package main — tool-pdf 插件的中文（CJK）字体注册、发现与折行辅助模块。
//
// 创建侧若要渲染中文，必须把 TrueType 字体嵌入 PDF 并走 Identity-H + CIDToGIDMap
// 的 Type0 子集路径（标准 14 字体不含 CJK 字形）。pdfcpu 的字体嵌入依赖其内部的
// 「用户字体注册表」（UserFontDir 目录下的 .gob 指标文件，含 CMap 与字形表）——
// 但该注册机制默认指向用户主目录下的全局字体目录，不适合作为工作空间内的插件使用。
//
// 本模块把 pdfcpu 的 UserFontDir 指向一个进程级临时目录，并把插件 fonts/ 目录下
// 自带的 .ttf 按需安装进该注册表，从而让 pdfcpu 的 EnsureFontDict / WriteMultiLine /
// UpdateUserfonts 能对自带中文字体完成嵌入、Unicode→GID 编码与子集化。
// 另提供 CJK 字符级折行（含避头尾 kinsoku 规则），避免中文超长单行横向溢出。
//
// 注意：pdfcpu 的 model.NewDefaultConfiguration() 每次创建配置都会把 font.UserFontDir
// 重置为用户目录。因此 ensureUserFontDir 在每次调用时都重新断言进程级目录，调用方须在
// 创建 PDF Context 之后、使用字体之前再次调用它。
//
// 对外入口：
//   - resolveFontName：把模型传入的字体名解析为 pdfcpu 可用的字体名，并标明是否走 CJK 嵌入路径。
//   - wrapTextForRender：按可用行宽把文本折行为视觉行（CJK 才折行）。
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/pdfcpu/pdfcpu/pkg/font"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// fontsDirEnv 覆写字体目录（仅测试/调试用；生产按可执行文件所在目录与工作目录探测）。
const fontsDirEnv = "TOOL_PDF_FONTS_DIR"

var (
	fontDirOnce sync.Once
	fontDirVal  string
	fontDirErr  error
)

// cjkFontRegistry 缓存已安装字体（key: .ttf 绝对路径 → pdfcpu PostScript 名），
// 避免同一字体重复安装解析。
var cjkFontRegistry = struct {
	sync.Mutex
	installed map[string]string
}{installed: map[string]string{}}

// bundledFontsDir 返回插件自带的 fonts 目录。
// 探测优先级：环境变量覆写 → 可执行文件同级 fonts/ → 工作目录 fonts/。
func bundledFontsDir() string {
	if d := os.Getenv(fontsDirEnv); d != "" {
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			return d
		}
	}
	var dirs []string
	if exe, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Join(filepath.Dir(exe), "fonts"))
	}
	if wd, err := os.Getwd(); err == nil {
		dirs = append(dirs, filepath.Join(wd, "fonts"))
	}
	for _, d := range dirs {
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			return d
		}
	}
	return ""
}

// listAvailableCJKFonts 扫描 fonts/ 目录，返回所有可用 .ttf 字体名的列表字符串。
// 经 ContextFn 注入 system prompt，让模型知道实际有哪些字体可用——
// 而非硬编码特定字体名。空目录返回提示让模型知道可用标准 14 字体。
func listAvailableCJKFonts() string {
	names := scanBundledTTFs()
	if len(names) == 0 {
		return " 当前无 CJK 字体（仅支持标准 14 字体）。"
	}
	return " 可用 CJK 字体（.ttf）: " + strings.Join(names, ", ") + "。"
}

// scanBundledTTFs 扫描 fonts/ 目录，返回所有 .ttf 字体文件的主干名（不含扩展名）。
// 按文件大小降序排列（CJK 字体通常 >5MB，排在前面优先被选中）。
// 供 listAvailableCJKFonts（system prompt 注入）与 TestMain（字体检测）共用。
func scanBundledTTFs() []string {
	fsDir := bundledFontsDir()
	if fsDir == "" {
		return nil
	}
	type fontEntry struct {
		name string
		size int64
	}
	var entries []fontEntry
	_ = filepath.WalkDir(fsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".ttf" {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		stem := strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))
		entries = append(entries, fontEntry{name: stem, size: info.Size()})
		return nil
	})
	// 按文件大小降序（CJK 字体优先）
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].size > entries[j].size
	})
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.name
	}
	return names
}

// fontNameStem 归一并取 .ttf 文件名主干（不含路径与扩展名），用于字体名匹配。
func fontNameStem(name string) string {
	return strings.TrimSuffix(strings.ToLower(filepath.Base(name)), filepath.Ext(name))
}

// findBundledFont 在 fonts 目录中按「文件名主干」精确匹配一个 .ttf。
// 仅支持 TrueType (.ttf) 格式——pdfcpu 不支持 OpenType CFF (.otf) 字体。
// 存在多个同名（不同子目录）视为歧义错误。
func findBundledFont(name string) (string, error) {
	fsDir := bundledFontsDir()
	if fsDir == "" {
		return "", fmt.Errorf("fonts directory not found (looked beside the plugin executable and the working directory; set %s to override)", fontsDirEnv)
	}
	target := fontNameStem(name)
	var matches []string
	err := filepath.WalkDir(fsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".ttf" {
			return nil // 仅支持 TrueType，不支持 .otf（CFF）
		}
		if fontNameStem(path) == target {
			matches = append(matches, path)
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("walk fonts directory: %w", err)
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("bundled font %q not found in %s (use a standard 14 font or a bundled CJK .ttf name; .otf/CFF not supported)", name, fsDir)
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("bundled font %q is ambiguous, matches: %v", name, matches)
	}
	return matches[0], nil
}

// ensureUserFontDir 确保 font.UserFontDir 指向进程级临时目录（幂等，每次调用都重新断言）。
// 之所以每次重新断言，是因为 pdfcpu 的 NewDefaultConfiguration() 会把它重置为用户目录。
func ensureUserFontDir() (string, error) {
	fontDirOnce.Do(func() {
		dir, err := os.MkdirTemp("", "tool-pdf-fonts-")
		if err != nil {
			fontDirErr = fmt.Errorf("create user font directory: %w", err)
			return
		}
		fontDirVal = dir
	})
	if fontDirErr != nil {
		return "", fontDirErr
	}
	font.UserFontDir = fontDirVal
	return fontDirVal, nil
}

// registerBundledFont 把一个自带 .ttf 安装进 pdfcpu 用户字体注册表并返回其 PostScript 名。
// 结果按 .ttf 路径缓存；重复调用直接返回。
func registerBundledFont(ttfPath string) (string, error) {
	cjkFontRegistry.Lock()
	defer cjkFontRegistry.Unlock()
	if ps, ok := cjkFontRegistry.installed[ttfPath]; ok {
		return ps, nil
	}
	if _, err := ensureUserFontDir(); err != nil {
		return "", err
	}
	report, err := font.InstallTrueTypeFont(font.UserFontDir, ttfPath)
	if err != nil {
		return "", fmt.Errorf("install bundled font %s: %w", ttfPath, err)
	}
	if len(report.Fonts) != 1 || report.Fonts[0].PostScriptName == "" {
		return "", fmt.Errorf("install bundled font %s: unexpected install report", ttfPath)
	}
	ps := report.Fonts[0].PostScriptName
	if err := font.ReloadUserFonts(); err != nil {
		return "", fmt.Errorf("reload user fonts: %w", err)
	}
	cjkFontRegistry.installed[ttfPath] = ps
	return ps, nil
}

// resolveFontName 把模型传入的字体名解析为 pdfcpu 可用字体名，并指明是否需走 CJK 嵌入路径。
// 标准 14 字体（Helvetica 等）原样返回，cjk=false；内置 CJK 字体返回其 PostScript 名，cjk=true。
func resolveFontName(name string) (pdfName string, cjk bool, err error) {
	if name == "" || isStandardFont(name) {
		return name, false, nil
	}
	ttfPath, err := findBundledFont(name)
	if err != nil {
		return "", false, err
	}
	ps, err := registerBundledFont(ttfPath)
	if err != nil {
		return "", false, err
	}
	return ps, true, nil
}

// wrapTextForRender 按可用行宽把多行文本折行为视觉行。
// 仅对 CJK 字体做字符级折行（复用 pdfcpu 的 WordWrapFloat，含避头尾 kinsoku 规则，
// 避免行首/行尾悬挂禁则标点）；标准 14 字体保持原样，避免回归既有行为。
// 空行原样保留（占位，保障分页与行高对齐）。
// 调用方须在此之前已确保 UserFontDir 断言正确（CJK 折行依赖字体度量）。
func wrapTextForRender(text, fontName string, fontSize, maxLineWidth float64, cjk bool) ([]string, error) {
	raw := strings.Split(text, "\n")
	if !cjk || maxLineWidth <= 0 {
		return raw, nil
	}
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		if strings.TrimSpace(line) == "" {
			lines = append(lines, "")
			continue
		}
		wrapped, err := model.WordWrapFloat(line, fontName, fontSize, maxLineWidth)
		if err != nil {
			return nil, fmt.Errorf("wrap line %q: %w", line, err)
		}
		lines = append(lines, wrapped...)
	}
	return lines, nil
}
