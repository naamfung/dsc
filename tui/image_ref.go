package tui

import (
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"dsc/core"
)

// 支持作为图像附件传入的图片扩展名 → MIME 类型（对齐 DeepSeek vision 支持范围）。
var imageExtMIME = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
}

// ResolveImageRefs 解析输入行中的「@图片路径」引用为内容寻址图像引用列表
// （dsc-img://<sha256><ext>，字节已写入附件库；对齐 DSH：历史只存引用）。
// 仅把「@ 开头、可解析为本地存在的图片文件」的 token 转为附件；其余 @ 引用
// （目录、普通文件、不存在的路径）一律忽略，保持输入文本原样（@路径 仍以文字
// 传给模型作为引用）。含空格路径可用反斜杆转义（@ 补全菜单的插入形式）保持单一
// token。图片仅当轮/注入消息携带。TUI 提交与 -input 自动化共用。
func ResolveImageRefs(line string) []string {
	wsRoot := os.Getenv("DSC_WORKSPACE_ROOT")
	var refs []string
	seen := map[string]bool{}
	for _, tok := range splitRefTokens(line) {
		ref := strings.TrimSpace(strings.TrimPrefix(tok, "@"))
		// 去掉常见引号/行尾标点（如 "…。@" 引用后跟标点）
		ref = strings.Trim(ref, "\"'`，。！？,;")
		if ref == "" {
			continue
		}
		path := resolveRefPath(wsRoot, ref)
		imgRef, err := saveImageAttachment(path)
		if err != nil {
			continue // 非图片/不可读：忽略，保留文字
		}
		if seen[imgRef] {
			continue
		}
		seen[imgRef] = true
		refs = append(refs, imgRef)
	}
	return refs
}

// splitRefTokens 按空白与句读标点把一行拆成 @ token：反斜杆转义的空白（@ 补全
// 菜单对含空格路径的插入形式）视为 token 一部分并去掉反斜杆；空白与句读标点
// （，。！？；： ,;!?）为 token 边界——使句中「@file.png，后续文字」在标点处截断，
// 不吞掉后续文字导致文件查找失败；行尾常见标点被剔除（"…。@" 引用后跟标点）。
func splitRefTokens(line string) []string {
	var toks []string
	for i := 0; i < len(line); i++ {
		if line[i] != '@' {
			continue
		}
		var b strings.Builder
		j := i + 1
		for j < len(line) {
			ch := line[j]
			if ch == '\\' && j+1 < len(line) && (line[j+1] == ' ' || line[j+1] == '\t') {
				b.WriteByte(line[j+1])
				j += 2
				continue
			}
			if isRefDelim(line[j:]) {
				break
			}
			b.WriteByte(ch)
			j++
		}
		if tok := strings.TrimRight(b.String(), "，。！？,;.!?)]}"); tok != "" {
			toks = append(toks, "@"+tok)
		}
		i = j - 1
	}
	return toks
}

// isRefDelim 判断 @ token 是否在此处结束（句读边界）：ASCII 空白、ASCII ,;!?、
// 以及中文标点（，。！？；：）。这些字符几乎不会出现在文件名里，作为句中 @
// 引用的自然分隔符；反斜杆转义过的空白已在上游单独处理，不在此列。
func isRefDelim(s string) bool {
	switch s[0] {
	case ' ', '\t', '\n', '\r', '\f', ',', ';', '!', '?':
		return true
	}
	r, _ := utf8.DecodeRuneInString(s)
	switch r {
	case '，', '。', '！', '？', '；', '：':
		return true
	}
	return false
}

// escapeRefPath 把路径中的空格/tab 反斜杆转义（@ 补全菜单的插入形式），使含空格
// 路径在 @ 引用的空白分隔语法中保持单一 token（splitRefTokens 逆转）；其余字节
// 原样（Windows 反斜杆分隔符保留含义）。
func escapeRefPath(path string) string {
	if !strings.ContainsAny(path, " \t") {
		return path
	}
	var b strings.Builder
	b.Grow(len(path) + 8)
	for i := 0; i < len(path); i++ {
		if path[i] == ' ' || path[i] == '\t' {
			b.WriteByte('\\')
		}
		b.WriteByte(path[i])
	}
	return b.String()
}

// unescapeRefPath 逆转 escapeRefPath：反斜杆+空格/tab 去掉反斜杆，其余原样。
func unescapeRefPath(path string) string {
	if !strings.Contains(path, "\\") {
		return path
	}
	var b strings.Builder
	b.Grow(len(path))
	for i := 0; i < len(path); i++ {
		if path[i] == '\\' && i+1 < len(path) && (path[i+1] == ' ' || path[i+1] == '\t') {
			continue
		}
		b.WriteByte(path[i])
	}
	return b.String()
}

// saveImageAttachment 读取图片文件并把字节写入内容寻址附件库，返回引用
// （文件名纯哈希不带后缀；MIME 由字节嗅探，不依赖声明扩展名）。
func saveImageAttachment(path string) (string, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if _, ok := imageExtMIME[ext]; !ok {
		return "", os.ErrInvalid
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return "", os.ErrNotExist
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return core.SaveImageAttachment(data)
}

// resolveRefPath 把 @ 引用路径解析为绝对路径：/workspace 虚拟根映射到工作区
// 真实根（与 sandbox/编辑器工具的别名约定一致），其余相对路径按工作区根解析。
func resolveRefPath(wsRoot, ref string) string {
	if strings.HasPrefix(ref, "/workspace") || strings.HasPrefix(ref, "\\workspace") {
		rel := strings.TrimPrefix(strings.TrimPrefix(ref, "/workspace"), "\\workspace")
		rel = strings.TrimPrefix(rel, "/")
		if wsRoot != "" {
			return filepath.Join(wsRoot, filepath.FromSlash(rel))
		}
		return filepath.FromSlash(rel)
	}
	if filepath.IsAbs(ref) {
		return ref
	}
	if wsRoot != "" {
		return filepath.Join(wsRoot, ref)
	}
	if abs, err := filepath.Abs(ref); err == nil {
		return abs
	}
	return ref
}
