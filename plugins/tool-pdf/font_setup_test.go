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
// TestMain 启动时自动检测：若 fonts/ 目录已存在任意 .ttf 字体则用之；
// 否则从 Google Fonts gstatic 下载 Noto Sans SC Regular（TrueType 格式）。
// License: OFL-1.1。
var cjkFontName = "NotoSansSC-Regular"

// cjkFontFile 字体文件名（.ttf 格式，TrueType glyf table）。
var cjkFontFile = "NotoSansSC-Regular.ttf"

// defaultCJKFontName / defaultCJKFontFile 是 TestMain 自动下载的默认字体。
const (
	defaultCJKFontName = "NotoSansSC-Regular"
	defaultCJKFontFile = "NotoSansSC-Regular.ttf"
)

// cjkFontURLs 字体下载源（按优先级尝试；Google Fonts gstatic 优先）。
// 注意：必须使用 TrueType (.ttf) 格式——pdfcpu 不支持 OpenType CFF (.otf) 字体。
var cjkFontURLs = []string{
	"https://fonts.gstatic.com/s/notosanssc/v40/k3kCo84MPvpLmixcA63oeAL7Iqp5IZJF9bmaG9_FnYw.ttf",
	"https://cdn.jsdelivr.net/gh/notofonts/noto-cjk@main/Sans/Variable/TTF/Subset/NotoSansSC-VF.ttf",
}

// TestMain 在所有测试前自动检查 fonts/ 目录：
//  1. 若已存在任意 .ttf 字体（如用户手动放置的 HarmonyOS Sans SC），直接用之，不下载
//  2. 若无任何 .ttf 字体，从 Google Fonts gstatic 下载 Noto Sans SC Regular
//  3. 下载失败时 CJK 测试自动 skip（不报错）
//
// 设计目标：让插件在任何开发环境（含 CI）中都能跑完整测试，无需手动下载字体；
// 同时尊重用户已有的字体配置（不覆盖、不忽略用户手动放置的 .ttf 字体）。
//
// 字体检测复用插件的 scanBundledTTFs()——与生产代码同一路径，确保测试与
// 实际行为一致（而非测试自写一套独立扫描逻辑）。
func TestMain(m *testing.M) {
	if err := resolveCJKFont(); err != nil {
		// 下载失败不阻止测试——CJK 测试会自行 skip
		fmt.Fprintf(os.Stderr, "[font_setup] CJK font setup failed: %v\n", err)
		fmt.Fprintf(os.Stderr, "[font_setup] CJK tests will be skipped. Manually place a .ttf CJK font in plugins/tool-pdf/fonts/ to enable them.\n")
	}
	os.Exit(m.Run())
}

// resolveCJKFont 解析测试使用的 CJK 字体：
//  1. 用插件的 scanBundledTTFs() 扫描 fonts/ 目录（与生产代码同路径）
//  2. 若已有任意 .ttf 字体则用最大的那个（CJK 字体通常 >5MB）
//  3. 若无，下载默认字体（Noto Sans SC Regular）
func resolveCJKFont() error {
	srcDir := testSourceDir()
	fontsDir := filepath.Join(srcDir, "fonts")

	// 确保 fonts 目录存在（scanBundledTTFs 依赖它）
	_ = os.MkdirAll(fontsDir, 0755)

	// 1. 复用插件的 scanBundledTTFs 检测已有字体（按大小降序，最大的在前）
	names := scanBundledTTFs()
	if len(names) > 0 {
		cjkFontName = names[0]
		cjkFontFile = cjkFontName + ".ttf"
		fmt.Fprintf(os.Stderr, "[font_setup] using existing font: %s\n", cjkFontFile)
		return nil
	}

	// 2. 无已有字体，下载默认字体
	cjkFontName = defaultCJKFontName
	cjkFontFile = defaultCJKFontFile
	fontPath := filepath.Join(fontsDir, cjkFontFile)

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
