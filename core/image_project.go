package core

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	jpegenc "image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	xdraw "golang.org/x/image/draw"
	_ "image/gif"  // 注册 gif 解码（与 image/jpeg、image/png 共同供 image.Decode 分发）
	_ "image/jpeg" // 注册 jpeg 解码

	"golang.org/x/image/bmp"
	"golang.org/x/image/tiff"
	"golang.org/x/image/webp"
)

// 请求面图像投影（对齐 DSH request-image：请求版本与存储版本分离）。会话历史里的
// 图像引用在进入 LLM 请求时才解析为字节，按路由像素策略缩放、按 alpha 有无选择
// 输出格式重编码——存储保持原样，请求按需瘦身。

// DefaultProjectionMaxSide 投影默认最长边（Anthropic 视觉文档推荐的最优分辨率：
// 更大的图先等比降采样到最长边 1568 再喂模型，token 与延迟双优）。CU 截图出图
// 上限与此一致（见 tool-computer-use 插件），投影链路对截图零二次缩放。
const DefaultProjectionMaxSide = 1568

// projectionJPEGQuality 缩放重编码时 JPEG 质量档（对齐 DSH 编码阶梯的实用单档；
// 截图/照片类内容 q85 与更高档视觉不可分，体积约为 PNG 的 1/10）。
const projectionJPEGQuality = 85

// ProjectImageRef 把图像引用（dsc-img:// / dsc-shot://）投影为 LLM 请求用
// data URL：最长边超过 maxSide 时等比降采样（CatmullRom），不透明图重编码为
// JPEG、带 alpha 图保留 PNG；未超限时原样透传（零重编码、零画质损失）。
// 解码失败或引用失效返回错误，由调用方降级为占位文本。maxSide<=0 表示不限。
func ProjectImageRef(ref string, maxSide int) (string, error) {
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

	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("探测图像尺寸失败: %w", err)
	}
	longest := cfg.Width
	if cfg.Height > longest {
		longest = cfg.Height
	}
	if maxSide <= 0 || longest <= maxSide {
		// 未超路由上限：原样透传，不做任何重编码
		return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), nil
	}

	src, err := decodeImage(data)
	if err != nil {
		return "", fmt.Errorf("解码图像失败: %w", err)
	}
	dst := resizeLongestSide(src, maxSide)
	if hasAlpha(src) {
		return encodeDataURL(dst, "image/png")
	}
	return encodeDataURL(dst, "image/jpeg")
}

// decodeImage 按字节魔数解码图像（png/jpeg/gif 由 stdlib image.Decode 分发；
// webp/bmp/tiff 显式调用 x/image 解码器——image 仓库未为它们注册全局分发）。
func decodeImage(data []byte) (image.Image, error) {
	switch sniffImageMime(data) {
	case "image/webp":
		return webp.Decode(bytes.NewReader(data))
	case "image/bmp":
		return bmp.Decode(bytes.NewReader(data))
	case "image/tiff":
		return tiff.Decode(bytes.NewReader(data))
	default:
		img, _, err := image.Decode(bytes.NewReader(data))
		return img, err
	}
}

// resizeLongestSide 等比缩放到最长边 maxSide（CatmullRom 高质量重采样）。
func resizeLongestSide(src image.Image, maxSide int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return src
	}
	nw, nh := w, h
	if w >= h {
		nw = maxSide
		nh = int(float64(h) * float64(maxSide) / float64(w))
	} else {
		nh = maxSide
		nw = int(float64(w) * float64(maxSide) / float64(h))
	}
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, b, xdraw.Src, nil)
	return dst
}

// hasAlpha 判断图像是否携带有效透明度：含 alpha 通道的类型做网格抽样
// （全不透明按不透明处理，走体积更优的 JPEG 输出）；YCbCr/Gray/CMYK 等
// 天然不透明的类型直接判否。
func hasAlpha(img image.Image) bool {
	switch img.(type) {
	case *image.YCbCr, *image.Gray, *image.Gray16, *image.CMYK:
		return false
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return false
	}
	stepX, stepY := w/16+1, h/16+1
	for y := b.Min.Y; y < b.Max.Y; y += stepY {
		for x := b.Min.X; x < b.Max.X; x += stepX {
			if _, _, _, a := img.At(x, y).RGBA(); a != 0xffff {
				return true
			}
		}
	}
	return false
}

// encodeDataURL 把图像编码为 data URL：JPEG q85 或 PNG 无损（缩放路径仅输出
// 这两种格式；未超限的 GIF/WebP 等原样透传，不进入此函数）。
func encodeDataURL(img image.Image, mime string) (string, error) {
	var buf bytes.Buffer
	var err error
	if mime == "image/jpeg" {
		err = jpegenc.Encode(&buf, img, &jpegenc.Options{Quality: projectionJPEGQuality})
	} else {
		mime = "image/png"
		err = png.Encode(&buf, img)
	}
	if err != nil {
		return "", fmt.Errorf("编码图像失败: %w", err)
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}
