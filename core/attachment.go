package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// 图像附件的内容寻址引用。对齐 DSH：会话历史/事件日志只保存引用（体积小），
// 图片字节写入内容寻址附件库；附件文件名**只取内容哈希、不带后缀**（对齐 DSH 的
// objects/<sha256> 命名），同一内容无论声明/改写什么扩展名都落同一文件（去重不
// 受后缀影响）；图片 MIME 由字节嗅探得出，请求时再解析嵌入（或上传 Files API
// 引用 file_id）。
const imageRefPrefix = "dsc-img://"

// 操作截图引用前缀（dsc-shot://<sha256>）：computer-use 等工具产生的操作截图。
// 与 dsc-img:// 同为内容寻址，但字节存可执行目录 temp/screenshots/——操作截图
// 只有短期观察价值（24 小时后随 temp/ 目录清理一并过期，见 manager.go
// cleanupOldTempDirs），不得混入持久附件库，也不得写入用户 workspace。
const shotRefPrefix = "dsc-shot://"

// TextRefPrefix 文本附件的引用前缀（与 @ 补全的图片引用平行：文件字节写入内容
// 寻址附件库，会话/LLM 以 dsc-txt://<sha256> 引用读取内容，对齐图像 dsc-img://）。
const TextRefPrefix = "dsc-txt://"

// 常见图片格式的魔数（用于从字节嗅探 MIME，避免把类型编进文件名）。
var (
	pngSig  = []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	jpgSig  = []byte{0xFF, 0xD8, 0xFF}
	riffSig = []byte("RIFF")
	webpSig = []byte("WEBP")
)

// AttachmentDir 返回附件库根目录：DSC_ATTACHMENT_DIR 覆盖（宿主启动时注入
// <ExecDir>/attachments，对齐 sessions/、memory/ 等可执行目录旧例；各插件进程
// 继承宿主环境路径一致），未设置时回退可执行文件所在目录的 attachments/。
func AttachmentDir() string {
	if d := strings.TrimSpace(os.Getenv("DSC_ATTACHMENT_DIR")); d != "" {
		return d
	}
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), "attachments")
	}
	return "attachments"
}

// saveAttachment 把字节以内容寻址方式写入 dir 并返回引用（纯哈希文件名）；
// 同内容已存在时直接返回既有引用（去重不受扩展名影响）。已存在分支校验文件大小，
// 大小与数据不符视为坏文件（如先前写入中断残留的部分文件）删除后重写，避免坏文件
// 被内容寻址去重永久复用导致后续读到损坏内容。
func saveAttachment(dir, prefix string, data []byte) (string, error) {
	sum := sha256.Sum256(data)
	name := hex.EncodeToString(sum[:])
	ref := prefix + name
	path := filepath.Join(dir, name)
	if fi, err := os.Stat(path); err == nil {
		if fi.Size() == int64(len(data)) {
			return ref, nil
		}
		// 坏文件：大小不符（写入中断残留），删除后按新内容重写
		os.Remove(path)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("创建附件库失败: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		// 写入失败清理残留，避免坏文件被后续内容寻址去重误复用
		os.Remove(path)
		return "", fmt.Errorf("写入附件失败: %w", err)
	}
	return ref, nil
}

// ScreenshotDir 返回操作截图存根目录：<可执行目录>/temp/screenshots（DSC_TEMP_DIR
// 覆盖，宿主启动时注入 <ExecDir>/temp，各插件进程继承环境路径一致；未设置时回退
// 可执行文件所在目录的 temp/）。目录内超 24 小时未被整体触碰即由宿主启动时的
// cleanupOldTempDirs 移除——截图引用随之失效，LLM 投影时降级为占位文本。
func ScreenshotDir() string {
	temp := strings.TrimSpace(os.Getenv("DSC_TEMP_DIR"))
	if temp == "" {
		if exe, err := os.Executable(); err == nil {
			temp = filepath.Join(filepath.Dir(exe), "temp")
		} else {
			temp = "temp"
		}
	}
	return filepath.Join(temp, "screenshots")
}

// SaveImageAttachment 把图片字节以内容寻址方式写入附件库并返回引用
// （dsc-img://<sha256>，文件名纯哈希不带后缀）；同内容已存在时直接返回既有引用
// （去重不受扩展名影响）。
func SaveImageAttachment(data []byte) (string, error) {
	return saveAttachment(AttachmentDir(), imageRefPrefix, data)
}

// SaveScreenshotAttachment 把操作截图字节以内容寻址方式写入 temp/screenshots/
// 并返回 dsc-shot://<sha256> 引用；同内容去重同上。截图不进持久附件库（生命周期
// 归 temp 清理管），会话日志与 LLM 投影只见引用。
func SaveScreenshotAttachment(data []byte) (string, error) {
	return saveAttachment(ScreenshotDir(), shotRefPrefix, data)
}

// SaveTextAttachment 把文本字节以内容寻址方式写入附件库并返回 dsc-txt://<sha256>
// 引用（同内容去重）；供 @文本文件 引用注入模型读取。
func SaveTextAttachment(data []byte) (string, error) {
	return saveAttachment(AttachmentDir(), TextRefPrefix, data)
}

// ResolveImageRef 把图像引用解析为 data:image/<mime>;base64,... 数据 URL。
// 支持两种内容寻址引用：dsc-img://<sha256>（持久附件库 attachments/，@图片 等用户
// 显式引用）与 dsc-shot://<sha256>（temp/screenshots/ 操作截图，24 小时过期）。
// 引用是图像进入会话历史的唯一形态——data URL 与带后缀旧引用一律拒绝，不保留
// 兼容分支；引用失效（截图过期、附件缺失）由调用方降级为占位文本。
// MIME 由附件字节嗅探，不依赖声明/文件名后缀。
func ResolveImageRef(ref string) (string, error) {
	var dir string
	switch {
	case strings.HasPrefix(ref, imageRefPrefix):
		dir = AttachmentDir()
		ref = strings.TrimPrefix(ref, imageRefPrefix)
	case strings.HasPrefix(ref, shotRefPrefix):
		dir = ScreenshotDir()
		ref = strings.TrimPrefix(ref, shotRefPrefix)
	default:
		return "", fmt.Errorf("不支持的图像引用: %s", ref)
	}
	data, err := os.ReadFile(filepath.Join(dir, ref))
	if err != nil {
		return "", fmt.Errorf("读取图像附件 %s 失败: %w", ref, err)
	}
	mime := sniffImageMime(data)
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

// ResolveTextRef 把文本附件引用（dsc-txt://<sha256>）解析为文本内容字符串，
// 供 LLM 插件把 @文本文件 内容注入请求。
func ResolveTextRef(ref string) (string, error) {
	if !strings.HasPrefix(ref, TextRefPrefix) {
		return "", fmt.Errorf("不支持的文本引用: %s", ref)
	}
	sha := strings.TrimPrefix(ref, TextRefPrefix)
	data, err := os.ReadFile(filepath.Join(AttachmentDir(), sha))
	if err != nil {
		return "", fmt.Errorf("读取文本附件 %s 失败: %w", sha, err)
	}
	return string(data), nil
}

// sniffImageMime 由字节魔数嗅探图片 MIME（JPEG/PNG/GIF/WebP；未知返回
// application/octet-stream）。不依赖文件名与声明类型。
func sniffImageMime(data []byte) string {
	switch {
	case len(data) >= 8 && bytes.Equal(data[:8], pngSig):
		return "image/png"
	case len(data) >= 3 && bytes.Equal(data[:3], jpgSig):
		return "image/jpeg"
	case len(data) >= 6 && (bytes.HasPrefix(data[:6], []byte("GIF87a")) || bytes.HasPrefix(data[:6], []byte("GIF89a"))):
		return "image/gif"
	case len(data) >= 12 && bytes.Equal(data[:4], riffSig) && bytes.Equal(data[8:12], webpSig):
		return "image/webp"
	}
	return "application/octet-stream"
}
