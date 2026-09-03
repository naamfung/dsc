package tui

import (
	"bytes"
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

// maxTextRefBytes @ 文本引用注入的字节上限：超过则跳过（保留 @路径 文字），
// 避免超大文件把上下文撑爆。
const maxTextRefBytes = 1 << 20 // 1 MiB

// refPaths 从输入行解析全部 @ 引用为去重后的绝对路径列表（图片/文本/其它）。
// 含空格路径可用反斜杆转义（@ 补全菜单的插入形式）保持单一 token。
func refPaths(line string) []string {
	wsRoot := os.Getenv("DSC_WORKSPACE_ROOT")
	var paths []string
	seen := map[string]bool{}
	for _, tok := range splitRefTokens(line) {
		ref := strings.TrimSpace(strings.TrimPrefix(tok, "@"))
		// 去掉常见引号/行尾标点（如 "…。@" 引用后跟标点）
		ref = strings.Trim(ref, "\"'`，。！？,;")
		if ref == "" {
			continue
		}
		p := resolveRefPath(wsRoot, ref)
		if seen[p] {
			continue
		}
		seen[p] = true
		paths = append(paths, p)
	}
	return paths
}

// ResolveImageRefs 解析输入行中的「@图片路径」引用为内容寻址图像引用列表
// （dsc-img://<sha256>，字节已写入附件库；对齐 DSH：历史只存引用）。
// 仅把可解析为本地图片文件的 @ 引用转为附件；其余（文本、目录、二进制、缺失）
// 一律忽略。TUI 提交与 -input 自动化共用。
func ResolveImageRefs(line string) []string {
	var refs []string
	for _, p := range refPaths(line) {
		ref, err := saveImageAttachment(p)
		if err != nil {
			continue
		}
		refs = append(refs, ref)
	}
	return refs
}

// ResolveFileRefs 解析输入行中的 @ 引用为内容寻址附件引用列表，与图像读取方式对齐：
// 图片文件 → dsc-img://<sha256>（多模态 image 块）；文本文件（含 NUL 嗅探判二进制）
// → dsc-txt://<sha256>（内容注入为文本块）。仅当文件存在且可读、尺寸未超限；目录、
// 二进制、缺失路径一律忽略，保持 @路径 以文字传给模型作为引用。
func ResolveFileRefs(line string) []string {
	var refs []string
	for _, p := range refPaths(line) {
		if ref, err := saveImageAttachment(p); err == nil {
			refs = append(refs, ref)
			continue
		}
		if ref, err := saveTextAttachment(p); err == nil {
			refs = append(refs, ref)
		}
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

// saveTextAttachment 读取文本文件（含 NUL 嗅探判二进制、尺寸上限）并把字节写入
// 内容寻址附件库，返回 dsc-txt:// 引用；非文本（含 NUL）/超限/目录返回错误跳过。
func saveTextAttachment(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return "", os.ErrNotExist
	}
	if info.Size() > maxTextRefBytes {
		return "", os.ErrPermission // 超限视为不可注入
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return "", os.ErrInvalid // 含 NUL：视为二进制，跳过
	}
	return core.SaveTextAttachment(data)
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
