package core

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	jpegenc "image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/go-hclog"
)

// encodePNG / encodeJPEG 测试辅助：把内存图像编码为真实格式字节。
func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func encodeJPEG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpegenc.Encode(&buf, img, &jpegenc.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// solidImage 生成 w×h 的纯色图（A=255 不透明；否则带 alpha 通道）。
func solidImage(w, h int, c color.RGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	n := color.NRGBA{R: c.R, G: c.G, B: c.B, A: c.A}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, n)
		}
	}
	return img
}

// TestSaveScreenshotAttachmentLifecycle 操作截图生命周期：字节落 DSC_TEMP_DIR 下的
// screenshots/（纯哈希文件名）、引用前缀 dsc-shot://、同内容去重、ResolveImageRef
// 解析回原 MIME 的 data URL——截图不进持久附件库。
func TestSaveScreenshotAttachmentLifecycle(t *testing.T) {
	tempDir := t.TempDir()
	attachDir := t.TempDir()
	t.Setenv("DSC_TEMP_DIR", tempDir)
	t.Setenv("DSC_ATTACHMENT_DIR", attachDir)

	data := encodePNG(t, solidImage(64, 48, color.RGBA{R: 200, G: 10, B: 10, A: 255}))
	ref, err := SaveScreenshotAttachment(data)
	if err != nil {
		t.Fatalf("SaveScreenshotAttachment: %v", err)
	}
	if !strings.HasPrefix(ref, "dsc-shot://") || strings.Contains(strings.TrimPrefix(ref, "dsc-shot://"), ".") {
		t.Fatalf("ref = %q, want dsc-shot://<sha256>", ref)
	}
	// 字节落 temp/screenshots/<sha256>，不落附件库
	name := strings.TrimPrefix(ref, "dsc-shot://")
	if _, err := os.Stat(filepath.Join(tempDir, "screenshots", name)); err != nil {
		t.Fatalf("screenshot not stored in temp/screenshots: %v", err)
	}
	if _, err := os.Stat(filepath.Join(attachDir, name)); err == nil {
		t.Fatal("screenshot must not be written into the durable attachment store")
	}

	// 同内容去重
	ref2, err := SaveScreenshotAttachment(data)
	if err != nil {
		t.Fatal(err)
	}
	if ref2 != ref {
		t.Fatalf("dedup failed: %q != %q", ref2, ref)
	}

	// 解析回 data URL
	url, err := ResolveImageRef(ref)
	if err != nil {
		t.Fatalf("ResolveImageRef: %v", err)
	}
	if !strings.HasPrefix(url, "data:image/png;base64,") {
		t.Fatalf("resolved url = %q, want png data url", url)
	}
}

// TestAdmitToolImages 入库口：data URL 折算为 dsc-shot:// 引用、内容寻址引用
// 原样透传、非图像载荷丢弃——data URL 绝不进入返回结果（即不进会话日志）。
func TestAdmitToolImages(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("DSC_TEMP_DIR", tempDir)
	m := &Manager{logger: hclog.NewNullLogger()}

	data := encodePNG(t, solidImage(16, 16, color.RGBA{R: 1, G: 2, B: 3, A: 255}))
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
	persisted, err := SaveImageAttachment(data)
	if err != nil {
		t.Fatal(err)
	}

	got := m.admitToolImages([]string{
		dataURL,                           // 插件回传的 data URL → 入库折算
		persisted,                         // 已是内容寻址引用 → 原样透传
		"data:text/plain;base64,SEVMTE8=", // 非 image/* → 丢弃
		"not a url",                       // 非法 → 丢弃
	})
	if len(got) != 2 {
		t.Fatalf("admitToolImages = %d refs, want 2 (%v)", len(got), got)
	}
	if !strings.HasPrefix(got[0], "dsc-shot://") {
		t.Fatalf("data URL not converted to screenshot ref: %q", got[0])
	}
	if got[1] != persisted {
		t.Fatalf("existing ref not passed through: %q", got[1])
	}
	for _, ref := range got {
		if strings.HasPrefix(ref, "data:") {
			t.Fatalf("inline data URL leaked into admission result: %q", ref)
		}
	}
	// 入库字节与原 data URL 字节一致（内容寻址）
	url, err := ResolveImageRef(got[0])
	if err != nil {
		t.Fatalf("ResolveImageRef: %v", err)
	}
	if !strings.HasPrefix(url, "data:image/png;base64,") {
		t.Fatalf("resolved url = %q", url)
	}
}

// TestProjectImageRefPassthrough 未超路由上限的图原样透传（零重编码）。
func TestProjectImageRefPassthrough(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DSC_ATTACHMENT_DIR", dir)
	t.Setenv("DSC_TEMP_DIR", t.TempDir())

	data := encodeJPEG(t, solidImage(100, 80, color.RGBA{R: 9, G: 9, B: 9, A: 255}))
	ref, err := SaveImageAttachment(data)
	if err != nil {
		t.Fatal(err)
	}
	url, err := ProjectImageRef(ref, DefaultProjectionMaxSide)
	if err != nil {
		t.Fatalf("ProjectImageRef: %v", err)
	}
	if !strings.HasPrefix(url, "data:image/jpeg;base64,") {
		t.Fatalf("passthrough should keep original mime (jpeg), got %q", strings.Split(url, ",")[0])
	}
	assertDataURLSize(t, url, 100, 80)
}

// TestProjectImageRefDownscaleOpaque 超限不透明图等比缩放到最长边并重编码为
// JPEG（体积最优）；投影产物尺寸与路由上限一致。
func TestProjectImageRefDownscaleOpaque(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DSC_ATTACHMENT_DIR", dir)
	t.Setenv("DSC_TEMP_DIR", t.TempDir())

	// 3000×1500 的不透明 JPEG（YCbCr 解码产物，无 alpha）
	data := encodeJPEG(t, solidImage(3000, 1500, color.RGBA{R: 30, G: 60, B: 90, A: 255}))
	ref, err := SaveImageAttachment(data)
	if err != nil {
		t.Fatal(err)
	}
	url, err := ProjectImageRef(ref, DefaultProjectionMaxSide)
	if err != nil {
		t.Fatalf("ProjectImageRef: %v", err)
	}
	if !strings.HasPrefix(url, "data:image/jpeg;base64,") {
		t.Fatalf("opaque downscaled output should be jpeg, got %q", strings.Split(url, ",")[0])
	}
	assertDataURLSize(t, url, DefaultProjectionMaxSide, DefaultProjectionMaxSide/2)
}

// TestProjectImageRefDownscaleAlpha 超限带 alpha 图保留 PNG 输出（透明度不丢）。
func TestProjectImageRefDownscaleAlpha(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DSC_ATTACHMENT_DIR", dir)
	t.Setenv("DSC_TEMP_DIR", t.TempDir())

	img := solidImage(2000, 1000, color.RGBA{R: 10, G: 20, B: 30, A: 128})
	data := encodePNG(t, img)
	ref, err := SaveImageAttachment(data)
	if err != nil {
		t.Fatal(err)
	}
	url, err := ProjectImageRef(ref, 500)
	if err != nil {
		t.Fatalf("ProjectImageRef: %v", err)
	}
	if !strings.HasPrefix(url, "data:image/png;base64,") {
		t.Fatalf("alpha downscaled output should be png, got %q", strings.Split(url, ",")[0])
	}
	assertDataURLSize(t, url, 500, 250)
}

// TestProjectImageRefErrors 非法引用报错（调用方降级为占位文本）。
func TestProjectImageRefErrors(t *testing.T) {
	t.Setenv("DSC_ATTACHMENT_DIR", t.TempDir())
	t.Setenv("DSC_TEMP_DIR", t.TempDir())

	if _, err := ProjectImageRef("dsc-shot://deadbeef", 1568); err == nil {
		t.Fatal("missing screenshot should error")
	}
	if _, err := ProjectImageRef("data:image/png;base64,AAAA", 1568); err == nil {
		t.Fatal("data URL is not a valid ref")
	}
	if _, err := ProjectImageRef("dsc-img://zzzz", 1568); err == nil {
		t.Fatal("missing attachment should error")
	}
}

// assertDataURLSize 解码 data URL 并断言图像尺寸。
func assertDataURLSize(t *testing.T, dataURL string, w, h int) {
	t.Helper()
	i := strings.Index(dataURL, ";base64,")
	if i < 0 {
		t.Fatalf("not a base64 data URL: %.40s", dataURL)
	}
	decoded, err := base64.StdEncoding.DecodeString(dataURL[i+len(";base64,"):])
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(decoded))
	if err != nil {
		t.Fatalf("DecodeConfig: %v", err)
	}
	if cfg.Width != w || cfg.Height != h {
		t.Fatalf("projected size = %dx%d, want %dx%d", cfg.Width, cfg.Height, w, h)
	}
}
