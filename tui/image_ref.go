package tui

import (
	"os"
	"path/filepath"
	"strings"

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

// resolveImageRefs 解析输入行中的「@图片路径」引用为内容寻址图像引用列表
// （dsc-img://<sha256><ext>，字节已写入附件库；对齐 DSH：历史只存引用）。
// 仅把「@ 开头、可解析为本地存在的图片文件」的 token 转为附件；其余 @ 引用
// （目录、普通文件、不存在的路径）一律忽略，保持输入文本原样（@路径 仍以文字
// 传给模型作为引用）。图片仅当轮/注入消息携带。
func resolveImageRefs(line string) []string {
	wsRoot := os.Getenv("DSC_WORKSPACE_ROOT")
	var refs []string
	seen := map[string]bool{}
	for _, tok := range strings.Fields(line) {
		if !strings.HasPrefix(tok, "@") || len(tok) < 2 {
			continue
		}
		ref := strings.TrimSpace(tok[1:])
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

// saveImageAttachment 读取图片文件并把字节写入内容寻址附件库，返回引用。
func saveImageAttachment(path string) (string, error) {
	ext := strings.ToLower(filepath.Ext(path))
	mime, ok := imageExtMIME[ext]
	if !ok {
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
	return core.SaveImageAttachment(data, mime)
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
