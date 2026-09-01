package core

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// 图像附件的内容寻址引用。对齐 DSH：会话历史/事件日志只保存引用（体积小），
// 图片字节写入内容寻址附件库（同内容只存一份、可去重），LLM 请求时再解析嵌入
// （或上传 Files API 引用 file_id）。
const imageRefPrefix = "dsc-img://"

// imageExtMIME 支持的图片扩展名 → MIME（对齐 DeepSeek vision 支持范围）。
var imageExtMIME = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
}

// imageMIMEToExt 由图片 MIME 推导扩展名（附件文件名用；image/jpeg 统一 .jpg 保证确定性）。
func imageMIMEToExt(mime string) string {
	switch mime {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	}
	return ".img"
}

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

// SaveImageAttachment 把图片字节以内容寻址方式写入附件库并返回引用
// （dsc-img://<sha256><ext>）；同内容已存在时直接返回既有引用（去重）。
func SaveImageAttachment(data []byte, mime string) (string, error) {
	sum := sha256.Sum256(data)
	name := hex.EncodeToString(sum[:]) + imageMIMEToExt(mime)
	ref := imageRefPrefix + name
	dir := AttachmentDir()
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); err == nil {
		return ref, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("创建附件库失败: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("写入附件失败: %w", err)
	}
	return ref, nil
}

// ResolveImageRef 把图像引用解析为 data:image/<mime>;base64,... 数据 URL。
// 兼容两种格式：dsc-img:// 引用（从附件库读取字节）与已内联的 data URL（原样返回，
// 兼容旧版本会话历史中直接存 base64 的消息）。
func ResolveImageRef(ref string) (string, error) {
	if strings.HasPrefix(ref, "data:") {
		return ref, nil
	}
	if !strings.HasPrefix(ref, imageRefPrefix) {
		return "", fmt.Errorf("不支持的图像引用: %s", ref)
	}
	name := strings.TrimPrefix(ref, imageRefPrefix)
	data, err := os.ReadFile(filepath.Join(AttachmentDir(), name))
	if err != nil {
		return "", fmt.Errorf("读取图像附件 %s 失败: %w", name, err)
	}
	ext := strings.ToLower(filepath.Ext(name))
	mime, ok := imageExtMIME[ext]
	if !ok {
		mime = "application/octet-stream"
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}
