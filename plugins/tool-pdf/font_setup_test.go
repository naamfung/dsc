package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// cjkFontName 测试使用的 CJK 字体名（文件名主干，不含扩展名）。
// Noto Sans SC 是 Google 开源的 CJK 字体（基于思源黑体），经 Google Fonts gstatic
// 公开分发 TrueType 格式（.ttf，含 glyf 表），pdfcpu 可直接嵌入。
// License: OFL-1.1。
const cjkFontName = "NotoSansSC-Regular"

// cjkFontFile 字体文件名（.ttf 格式，TrueType glyf table）。
const cjkFontFile = "NotoSansSC-Regular.ttf"

// cjkFontURLs 字体下载源（按优先级尝试；Google Fonts gstatic 优先）。
// 注意：必须使用 TrueType (.ttf) 格式——pdfcpu 不支持 OpenType CFF (.otf) 字体。
var cjkFontURLs = []string{
	"https://fonts.gstatic.com/s/notosanssc/v40/k3kCo84MPvpLmixcA63oeAL7Iqp5IZJF9bmaG9_FnYw.ttf",
	"https://cdn.jsdelivr.net/gh/notofonts/noto-cjk@main/Sans/Variable/TTF/Subset/NotoSansSC-VF.ttf",
}

// TestMain 在所有测试前自动检查 fonts/ 目录，若 CJK 字体缺失则自动下载。
//
// 设计目标：让插件在任何开发环境（含 CI）中都能跑完整测试，无需手动下载字体。
// 下载的字体保留在 fonts/ 目录（不删除），后续测试可直接复用。
//
// 字体选择：Source Han Sans SC（思源黑体）—— Adobe + Google 联合开源（OFL-1.1），
// GitHub 公开仓库 adobe-fonts/source-han-sans 稳定分发，无需 license 担忧。
// 原硬编码 HarmonyOS Sans SC 改为通用名，任何 .ttf/.otf CJK 字体均可工作。
func TestMain(m *testing.M) {
	if err := ensureCJKFont(); err != nil {
		// 下载失败不阻止测试——CJK 测试会自行 skip
		fmt.Fprintf(os.Stderr, "[font_setup] CJK font download skipped: %v\n", err)
		fmt.Fprintf(os.Stderr, "[font_setup] CJK tests will be skipped. Manually download %s to plugins/tool-pdf/fonts/ to enable them.\n", cjkFontFile)
	}
	os.Exit(m.Run())
}

// ensureCJKFont 确保 fonts/ 目录中存在 CJK 字体文件。
// 已存在则跳过；不存在则依次尝试 cjkFontURLs 中的下载源。
func ensureCJKFont() error {
	srcDir := testSourceDir()
	fontsDir := filepath.Join(srcDir, "fonts")
	fontPath := filepath.Join(fontsDir, cjkFontFile)

	// 已存在则跳过
	if st, err := os.Stat(fontPath); err == nil && st.Size() > 1000000 {
		return nil
	}

	// 确保 fonts 目录存在
	if err := os.MkdirAll(fontsDir, 0755); err != nil {
		return fmt.Errorf("create fonts dir: %w", err)
	}

	// 依次尝试下载源
	for _, url := range cjkFontURLs {
		if err := downloadFont(url, fontPath); err != nil {
			fmt.Fprintf(os.Stderr, "[font_setup] download from %s failed: %v\n", url, err)
			continue
		}
		fmt.Fprintf(os.Stderr, "[font_setup] downloaded %s (%.1f MB) to %s\n", cjkFontFile, float64(fileSize(fontPath))/(1024*1024), fontPath)
		return nil
	}
	return fmt.Errorf("all download sources failed")
}

// downloadFont 从 url 下载字体到 outPath（先写临时文件再 rename，避免半成品）。
func downloadFont(url, outPath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	tmpPath := outPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, outPath)
}

// testSourceDir 返回本测试文件的目录（用于定位插件 fonts/ 目录）。
func testSourceDir() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}
	return filepath.Dir(file)
}

// fileSize 返回文件大小（字节），不存在返回 0。
func fileSize(path string) int64 {
	st, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return st.Size()
}
