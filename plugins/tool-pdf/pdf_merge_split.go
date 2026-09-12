// Package main — tool-pdf 插件的「合并 / 拆分」模块。
//
// 基于 pdfcpu 的 Merge / ExtractPages 能力，为模型提供：
//   - pdf_merge_pdfs：把多个 PDF 按序合并为一个
//   - pdf_split_pdfs：把一个 PDF 按页拆分为多个（或多页一段）
//   - pdf_extract_pages：从一个 PDF 抽选页生成新 PDF
//
// 全部输入/输出路径均须在工作空间根内（沙箱边界）。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// resolveWorkspacePaths 把全部路径解析为绝对路径并校验均在沙箱内。
// resolveWorkspacePaths 把路径列表绝对化（沙箱策略由宿主流水线统一判定，
// 本插件不做越界检查——与 tool-filesystem / tool-str-replace-editor 一致）。
func resolveWorkspacePaths(paths []string) ([]string, error) {
	abs := make([]string, 0, len(paths))
	for _, p := range paths {
		a, err := filepath.Abs(p)
		if err != nil {
			return nil, fmt.Errorf("resolve path %q: %w", p, err)
		}
		abs = append(abs, a)
	}
	return abs, nil
}

// handleMergePDFs 是 pdf_merge_pdfs 工具的处理器。
func handleMergePDFs(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		InputPaths  []string `json:"input_paths"`
		OutPath     string   `json:"out_path"`
		DividerPage bool     `json:"divider_page,omitempty"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if len(p.InputPaths) < 2 {
		return "", fmt.Errorf("input_paths is required (at least 2 PDFs)")
	}
	if p.OutPath == "" {
		return "", fmt.Errorf("out_path is required")
	}
	inputs, err := resolveWorkspacePaths(p.InputPaths)
	if err != nil {
		return "", err
	}
	outAbs, err := resolveWorkspacePaths([]string{p.OutPath})
	if err != nil {
		return "", err
	}
	outPath := outAbs[0]
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return "", fmt.Errorf("create output dir: %w", err)
	}

	conf := model.NewDefaultConfiguration()
	if err := api.MergeCreateFile(inputs, outPath, p.DividerPage, conf); err != nil {
		return "", fmt.Errorf("merge PDFs: %w", err)
	}

	var names []string
	for _, in := range inputs {
		names = append(names, filepath.Base(in))
	}
	return fmt.Sprintf("Merged %d PDF(s) into: %s\nOrder: %s\n",
		len(inputs), outPath, strings.Join(names, ", ")), nil
}

// handleSplitPDFs 是 pdf_split_pdfs 工具的处理器。
// span 为每段页数（>=1）；out_dir 必填。
func handleSplitPDFs(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		FilePath string `json:"file_path"`
		OutDir   string `json:"out_dir"`
		Span     int    `json:"span,omitempty"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if p.FilePath == "" {
		return "", fmt.Errorf("file_path is required")
	}
	if p.OutDir == "" {
		return "", fmt.Errorf("out_dir is required")
	}
	inAbs, err := resolveWorkspacePaths([]string{p.FilePath})
	if err != nil {
		return "", err
	}
	outAbs, err := resolveWorkspacePaths([]string{p.OutDir})
	if err != nil {
		return "", err
	}
	span := p.Span
	if span <= 0 {
		span = 1
	}
	if err := os.MkdirAll(outAbs[0], 0755); err != nil {
		return "", fmt.Errorf("create out_dir: %w", err)
	}

	conf := model.NewDefaultConfiguration()
	if err := api.SplitFile(inAbs[0], outAbs[0], span, conf); err != nil {
		return "", fmt.Errorf("split PDF: %w", err)
	}

	files := listPDFs(outAbs[0])
	return fmt.Sprintf("Split %s into %d file(s) under %s (span %d page(s) each):\n",
		filepath.Base(inAbs[0]), len(files), outAbs[0], span) + joinLines(files), nil
}

// handleExtractPagesTool 是 pdf_extract_pages 工具的处理器。
// 从 PDF 抽选页合并为新 PDF（用选页 + 合并实现）。
func handleExtractPagesTool(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		FilePath string `json:"file_path"`
		OutPath  string `json:"out_path"`
		Pages    string `json:"pages"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if p.FilePath == "" {
		return "", fmt.Errorf("file_path is required")
	}
	if p.OutPath == "" {
		return "", fmt.Errorf("out_path is required")
	}
	if p.Pages == "" {
		return "", fmt.Errorf("pages is required (e.g. \"1,3,5-7\")")
	}
	inAbs, err := resolveWorkspacePaths([]string{p.FilePath})
	if err != nil {
		return "", err
	}
	outAbs, err := resolveWorkspacePaths([]string{p.OutPath})
	if err != nil {
		return "", err
	}
	outPath := outAbs[0]

	pdfCtx, err := loadPDFContext(p.FilePath)
	if err != nil {
		return "", err
	}
	selected, err := parsePageSelection(p.Pages, pdfCtx.PageCount)
	if err != nil {
		return "", fmt.Errorf("invalid pages %q: %w", p.Pages, err)
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return "", fmt.Errorf("create output dir: %w", err)
	}

	// 用 ExtractPagesFile 把选页写到临时目录，再合并回单个输出
	tmpDir, err := os.MkdirTemp("", "pdf-extract-pages-")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	pageList := renderPageList(selected)
	conf := model.NewDefaultConfiguration()
	if err := api.ExtractPagesFile(inAbs[0], tmpDir, []string{pageList}, conf); err != nil {
		return "", fmt.Errorf("extract pages: %w", err)
	}
	partFiles := listPDFs(tmpDir)
	if len(partFiles) == 0 {
		return "", fmt.Errorf("no pages extracted")
	}
	if len(partFiles) == 1 {
		// 单页：直接复制
		if err := copyFile(partFiles[0], outPath); err != nil {
			return "", err
		}
	} else {
		sort.Strings(partFiles)
		if err := api.MergeCreateFile(partFiles, outPath, false, conf); err != nil {
			return "", fmt.Errorf("merge extracted pages: %w", err)
		}
	}
	return fmt.Sprintf("Extracted %d page(s) from %s into: %s\nPages: %s\n",
		len(selected), filepath.Base(inAbs[0]), outPath, pageList), nil
}

// renderPageList 把页号列表渲染为 pdfcpu 选页串（如 "1,3,5-7"）。
func renderPageList(pages []int) string {
	var b strings.Builder
	first := true
	for i := 0; i < len(pages); {
		j := i
		for j+1 < len(pages) && pages[j+1] == pages[j]+1 {
			j++
		}
		if !first {
			b.WriteString(",")
		}
		first = false
		if j > i {
			fmt.Fprintf(&b, "%d-%d", pages[i], pages[j])
		} else {
			fmt.Fprintf(&b, "%d", pages[i])
		}
		i = j + 1
	}
	return b.String()
}

// listPDFs 列出目录下全部 .pdf 文件（排序）。
func listPDFs(dir string) []string {
	entries, _ := os.ReadDir(dir)
	var out []string
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".pdf") {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(out)
	return out
}

// copyFile 复制文件。
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}
	if err := os.WriteFile(dst, data, 0644); err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}
	return nil
}

// joinLines 把路径列表拼成带缩进的文本。
func joinLines(items []string) string {
	var b strings.Builder
	for _, it := range items {
		b.WriteString("  - " + it + "\n")
	}
	return b.String()
}
