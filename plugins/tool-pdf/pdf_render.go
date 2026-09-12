// Package main — tool-pdf 插件的「页面转图」模块。
//
// 临时禁用：本模块依赖外部命令（mutool / pdftoppm / Ghostscript），未真机验证，
// 当前不在 main.go 注册工具（注册块已注释）。恢复时取消 main.go 中 pdf_to_images
// 注册块注释即可；命令构造与渲染器探测逻辑均在本文件，保持可编译。
//
// pdfcpu 本身不提供整页光栅化（只提取嵌入图），故本模块委托外部 PDF 渲染器
// （mutool / pdftoppm / Ghostscript）把选页渲染为 PNG，供模型作视觉/版面分析。
//
// 渲染器在进程内按顺序探测：DSC_PDF_RENDERER 环境变量覆盖 → PATH 中的
// mutool → pdftoppm → gswin64c/gswin32c/gs。探测不到时明确提示，并建议回退
// pdf_extract_images（提取嵌入图）。
//
// 设计原则：对外只暴露 pdftoppm 风格的「连续页段 + 分辨率 + 输出前缀」能力；
// 任意页集合简化为覆盖 [min..max] 的连续段，避免多次子进程调用。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// rasterizer 描述一个外部 PDF→PNG 渲染器的启动方式。
type rasterizer struct {
	name string // 二进制名（探测用）
	abs  string // 探测到的完整路径
	// buildArgs 返回渲染 first..last 页（1-based，含）到 outPrefix-页码.png 的参数
	family string // mutool / pdftoppm / gs
}

// rendererEnv 环境变量覆写渲染器路径。
const rendererEnv = "DSC_PDF_RENDERER"

// renderDPI 默认渲染分辨率（DPI）。可用 DSC_PDF_RENDER_DPI 覆盖。
const renderDPI = 150

// rasterizerCandidates 按优先级列出可探测的渲染器二进制名。
var rasterizerCandidates = []string{
	"mutool",
	"pdftoppm",
	"gswin64c", "gswin32c", "gs",
}

// findRasterizer 探测可用的渲染器：优先环境变量，其次 PATH 中候选名。
func findRasterizer() *rasterizer {
	if exe := os.Getenv(rendererEnv); exe != "" {
		if abs, err := exec.LookPath(exe); err == nil {
			return &rasterizer{name: filepath.Base(abs), abs: abs, family: familyFor(abs)}
		}
		if st, err := os.Stat(exe); err == nil && !st.IsDir() {
			return &rasterizer{name: filepath.Base(exe), abs: exe, family: familyFor(exe)}
		}
	}
	for _, name := range rasterizerCandidates {
		if abs, err := exec.LookPath(name); err == nil {
			return &rasterizer{name: name, abs: abs, family: familyFor(abs)}
		}
	}
	return nil
}

// familyFor 由二进制名推断渲染器家族（决定命令行构造）。
func familyFor(abs string) string {
	base := strings.ToLower(filepath.Base(abs))
	switch {
	case strings.Contains(base, "mutool"):
		return "mutool"
	case strings.Contains(base, "pdftoppm"):
		return "pdftoppm"
	case strings.HasPrefix(base, "gs") || strings.Contains(base, "ghostscript"):
		return "gs"
	default:
		return "unknown"
	}
}

// renderArgs 构建渲染 first..last 页到 outPrefix 的参数。
func (r *rasterizer) renderArgs(inFile, outPrefix string, first, last, dpi int) []string {
	switch r.family {
	case "mutool":
		// mutool draw -F png -r dpi -o outPrefix-页码.png inFile
		return []string{"draw", "-F", "png", "-r", fmt.Sprintf("%d", dpi), "-o", outPrefix + "-%d.png", inFile}
	case "pdftoppm":
		// pdftoppm -f first -l last -r dpi -png inFile outPrefix
		return []string{"-f", fmt.Sprintf("%d", first), "-l", fmt.Sprintf("%d", last), "-r", fmt.Sprintf("%d", dpi), "-png", inFile, outPrefix}
	default: // gs
		// gs -sDEVICE=png16m -r dpi -dFirstPage=f -dLastPage=l -o outPrefix-%d.png inFile
		return []string{"-sDEVICE=png16m", "-r", fmt.Sprintf("%d", dpi),
			fmt.Sprintf("-dFirstPage=%d", first), fmt.Sprintf("-dLastPage=%d", last),
			"-o", outPrefix + "-%d.png", inFile}
	}
}

// contiguousPages 计算覆盖 selected（1-based）的最小连续页段 [first..last]。
func contiguousPages(selected []int) (int, int) {
	if len(selected) == 0 {
		return 1, 1
	}
	sorted := append([]int(nil), selected...)
	sort.Ints(sorted)
	return sorted[0], sorted[len(sorted)-1]
}

// handlePageToImages 是 pdf_to_images 工具的处理器。
// 把指定页码范围渲染为 PNG 输出到目录。
func handlePageToImages(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		FilePath string `json:"file_path"`
		Pages    string `json:"pages,omitempty"`
		OutDir   string `json:"out_dir,omitempty"`
		DPI      int    `json:"dpi,omitempty"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if p.FilePath == "" {
		return "", fmt.Errorf("file_path is required")
	}

	pdfCtx, err := loadPDFContext(p.FilePath)
	if err != nil {
		return "", err
	}
	pageCount := pdfCtx.PageCount

	selected, err := parsePageSelection(p.Pages, pageCount)
	if err != nil {
		return "", fmt.Errorf("invalid pages %q: %w", p.Pages, err)
	}
	if len(selected) == 0 {
		return "", fmt.Errorf("no pages selected")
	}
	first, last := contiguousPages(selected)

	if p.OutDir == "" {
		p.OutDir = filepath.Join(workspaceRoot(), "pdf-images",
			strings.TrimSuffix(filepath.Base(p.FilePath), ".pdf"), "render")
	}
	// 沙箱策略由宿主流水线统一判定，本插件不做越界检查
	dpi := p.DPI
	if dpi <= 0 {
		dpi = renderDPI
	}
	outAbs, err := filepath.Abs(p.OutDir)
	if err != nil {
		return "", fmt.Errorf("resolve out_dir: %w", err)
	}
	if err := os.MkdirAll(outAbs, 0755); err != nil {
		return "", fmt.Errorf("create out_dir: %w", err)
	}

	r := findRasterizer()
	if r == nil {
		return "", fmt.Errorf("no PDF rasterizer found (mutool/pdftoppm/gs). Install one or set %s, or use pdf_extract_images to extract embedded images", rendererEnv)
	}

	absIn, err := filepath.Abs(p.FilePath)
	if err != nil {
		return "", fmt.Errorf("resolve file_path: %w", err)
	}
	outPrefix := filepath.Join(outAbs, "page")
	cmdArgs := r.renderArgs(absIn, outPrefix, first, last, dpi)

	cmd := exec.Command(r.abs, cmdArgs...)
	cmd.Stdout, cmd.Stderr = new(strings.Builder), new(strings.Builder)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("render via %s failed: %v\n%s", r.abs, err, cmd.Stderr.(*strings.Builder).String())
	}

	entries, _ := os.ReadDir(outAbs)
	var rendered []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "page-") && strings.HasSuffix(e.Name(), ".png") {
			rendered = append(rendered, filepath.Join(outAbs, e.Name()))
		}
	}
	if len(rendered) == 0 {
		return "", fmt.Errorf("renderer finished but no PNG output in %s", outAbs)
	}
	sort.Strings(rendered)

	var b strings.Builder
	fmt.Fprintf(&b, "Rendered %d page(s) to %s (DPI %d):\n", len(rendered), outAbs, dpi)
	for _, f := range rendered {
		b.WriteString("  - " + f + "\n")
	}
	return b.String(), nil
}
