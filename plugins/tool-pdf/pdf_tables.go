// Package main — tool-pdf 插件的表格结构提取模块。
//
// 在位置感知（text_extractor）基础上，检测页面上是否存在「多行共享 x 对齐」的表格布局：
// 把整页正常横行的片段起点按 1D 聚类成列槽，若列槽 ≥2 且行平均覆盖 ≥2 列，判定为表格，
// 按列槽输出对齐网格（列间以竖线分隔），保留「行 × 列」结构供模型阅读。
//
// 设计原则：尽力而为的启发式。未检测到明显表格时明确告知，建议回落 pdf_read_text；
// 不追求恢复表格边框/合并单元格等复杂结构。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// tableTolEm 列槽聚类的 x 容差（em 单位），与页面中位字号相乘。
const tableTolEm = 0.9

// tableMinRows 判定表格的最少行数。
const tableMinRows = 3

// tableMinAvgCols 判定表格的平均每行覆盖列数下限。
const tableMinAvgCols = 1.8

// detectTableColumns 从整页视觉行中检测列槽（列 x 中心），无表格时返回 false。
// 只使用正常横行片段（rot≈0）。
func detectTableColumns(lines []textLine) ([]float64, bool) {
	// 收集正常横行的行内片段起点
	var rowFrags [][]textFragment
	medFont := 10.0
	var allFont []float64
	for _, l := range lines {
		var fr []textFragment
		for _, f := range l.frag {
			if abs(f.rot) < rotTol {
				fr = append(fr, f)
				allFont = append(allFont, f.fontSize)
			}
		}
		if len(fr) > 0 {
			rowFrags = append(rowFrags, fr)
		}
	}
	if len(rowFrags) < tableMinRows {
		return nil, false
	}
	if len(allFont) > 0 {
		sort.Float64s(allFont)
		medFont = allFont[len(allFont)/2]
	}
	if medFont <= 0 {
		medFont = 10
	}
	tol := medFont * tableTolEm

	// 收集所有片段起点 x，做 1D 聚类
	var xs []float64
	for _, fr := range rowFrags {
		for _, f := range fr {
			xs = append(xs, f.x)
		}
	}
	cols := clusterX(xs, tol)
	if len(cols) < 2 {
		return nil, false
	}

	// 计算每行平均命中的列槽数
	totalHits := 0
	for _, fr := range rowFrags {
		hit := map[int]bool{}
		for _, f := range fr {
			if idx := nearestCol(cols, f.x, tol); idx >= 0 {
				hit[idx] = true
			}
		}
		totalHits += len(hit)
	}
	avgCols := float64(totalHits) / float64(len(rowFrags))
	if avgCols < tableMinAvgCols {
		return nil, false
	}
	return cols, true
}

// clusterX 对 x 坐标做 1D 聚类：按升序聚拢彼此距离 ≤ tol 的点，返回簇中心（升序）。
func clusterX(xs []float64, tol float64) []float64 {
	if len(xs) == 0 {
		return nil
	}
	sorted := append([]float64(nil), xs...)
	sort.Float64s(sorted)
	var centers []float64
	for _, x := range sorted {
		placed := false
		for i, c := range centers {
			if abs(c-x) <= tol {
				centers[i] = (c + x) / 2
				placed = true
				break
			}
		}
		if !placed {
			centers = append(centers, x)
		}
	}
	sort.Float64s(centers)
	return centers
}

// nearestCol 返回距 x 最近且不超过 tol 的列槽下标；无则返回 -1。
func nearestCol(cols []float64, x, tol float64) int {
	best, bestDist := -1, tol
	for i, c := range cols {
		if d := abs(c - x); d <= bestDist {
			best, bestDist = i, d
		}
	}
	return best
}

// renderTable 把整页行按列槽输出为对齐网格。
// 每行片段归入最近列槽，同槽内按 x 拼接；缺失列输出空。
func renderTable(lines []textLine, cols []float64, tol float64) string {
	var b strings.Builder
	for _, l := range lines {
		var fr []textFragment
		for _, f := range l.frag {
			if abs(f.rot) < rotTol {
				fr = append(fr, f)
			}
		}
		if len(fr) == 0 {
			continue
		}
		sort.SliceStable(fr, func(i, j int) bool { return fr[i].x < fr[j].x })

		cells := make([]strings.Builder, len(cols))
		for _, f := range fr {
			idx := nearestCol(cols, f.x, tol)
			if idx < 0 {
				continue
			}
			if cells[idx].Len() > 0 {
				cells[idx].WriteString(" ")
			}
			cells[idx].WriteString(f.s)
		}
		for i, c := range cells {
			if i > 0 {
				b.WriteString(" | ")
			}
			b.WriteString(strings.TrimSpace(c.String()))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// handleExtractTables 是 pdf_extract_tables 工具的处理器。
// 检测并输出 PDF 页面中的表格结构（列对齐网格）；未检测到则提示。
func handleExtractTables(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		FilePath string `json:"file_path"`
		Pages    string `json:"pages,omitempty"`
		MaxPages int    `json:"max_pages,omitempty"`
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
	if pageCount == 0 {
		return "", fmt.Errorf("PDF has no pages")
	}

	selected, err := parsePageSelection(p.Pages, pageCount)
	if err != nil {
		return "", fmt.Errorf("invalid pages %q: %w", p.Pages, err)
	}
	maxPages := p.MaxPages
	if maxPages == 0 {
		maxPages = 20
	}
	if len(selected) > maxPages {
		selected = selected[:maxPages]
	}

	var b strings.Builder
	fmt.Fprintf(&b, "=== %s (%d pages, scanning %d) ===\n\n", pageBase(p.FilePath), pageCount, len(selected))

	foundAny := false
	for _, pageNr := range selected {
		lines, err := extractPageLines(pdfCtx, pageNr)
		if err != nil {
			fmt.Fprintf(&b, "--- Page %d (extraction failed: %v) ---\n", pageNr, err)
			continue
		}
		cols, ok := detectTableColumns(lines)
		if !ok {
			continue
		}
		foundAny = true
		// 容差与 detect 一致（用页面中位字号）
		med := medianFontSize(lines)
		tol := med * tableTolEm
		fmt.Fprintf(&b, "--- Page %d (%d columns) ---\n", pageNr, len(cols))
		b.WriteString(renderTable(lines, cols, tol))
		b.WriteString("\n")
	}

	if !foundAny {
		b.WriteString("No obvious table layout detected on the scanned pages (no column-aligned text). Use pdf_read_text for plain text extraction.\n")
	}
	return b.String(), nil
}

// medianFontSize 返回页面正常横行片段字号的中位数（默认 10）。
func medianFontSize(lines []textLine) float64 {
	var fs []float64
	for _, l := range lines {
		for _, f := range l.frag {
			if abs(f.rot) < rotTol && f.fontSize > 0 {
				fs = append(fs, f.fontSize)
			}
		}
	}
	if len(fs) == 0 {
		return 10
	}
	sort.Float64s(fs)
	return fs[len(fs)/2]
}

// pageBase 返回路径基名（不含目录）。
func pageBase(path string) string {
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		return path[i+1:]
	}
	return path
}
