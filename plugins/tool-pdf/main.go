// Package main — tool-pdf 插件：为模型提供读取 PDF 文件以获取内容信息的能力。
//
// 基于 pdfcpu 库（github.com/pdfcpu/pdfcpu）实现 PDF 结构解析与内容流提取，
// 自写文本操作符解释器与字体编码解码层（WinAnsi/MacRoman/StandardEncoding + ToUnicode CMap）。
// 创建侧支持标准 14 字体与内置 CJK 字体（Type0 嵌入子集，渲染中文）。
//
// 暴露的工具：
//   - 读取：pdf_read_text（提取纯文本）、pdf_info、pdf_outline、pdf_search、pdf_extract_images
//   - 创建：pdf_create_text、pdf_images_to_pdf、pdf_append_text
//
// 字体解码策略优先级：ToUnicode CMap > Differences > 基础编码（WinAnsi/MacRoman/Standard）。
// 不可识别的字节回退为 '?'，保证不返回错误——便于模型判断是否值得继续。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	dsc "dsc-sdk"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// ---------- 共享状态 ----------

// workspaceRoot 返回宿主注入的工作空间根目录（沙箱边界，仅用于默认输出路径推断）。
// 注意：本插件不做 workspace 越界检查——沙箱策略由宿主工具流水线 pre-execute
// 瀑布统一判定（与 tool-filesystem / tool-str-replace-editor 一致）。
// 这避免了 full-access 模式下插件仍自行拒绝 workspace 外路径的问题。
func workspaceRoot() string {
	if r := os.Getenv("DSC_WORKSPACE_ROOT"); r != "" {
		return r
	}
	if r, err := os.Getwd(); err == nil {
		return r
	}
	return "."
}

// pdfReadContext 缓存最近打开的 PDF 的 Context，避免重复解析。
// 同一文件多次工具调用时复用（如先 info 再 read_text 再 search）。
type pdfReadContext struct {
	mu      sync.Mutex
	path    string
	modTime int64
	ctx     *model.Context
}

var sharedCtx pdfReadContext

// loadPDFContext 打开 PDF 文件并返回可复用的 pdfcpu Context。
// 文件未变化时复用缓存的 Context；变化时重新解析。
//
// 沙箱策略由宿主工具流水线 pre-execute 瀑布统一判定（read-only 拒绝写、
// workspace-write 限制 workspace 内、full-access 全开），本插件不做越界检查——
// 与 tool-filesystem / tool-str-replace-editor 一致，避免 full-access 模式下
// 插件仍自行拒绝 workspace 外路径。
func loadPDFContext(path string) (*model.Context, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", absPath, err)
	}

	sharedCtx.mu.Lock()
	defer sharedCtx.mu.Unlock()

	if sharedCtx.ctx != nil && sharedCtx.path == absPath && sharedCtx.modTime == info.ModTime().UnixNano() {
		return sharedCtx.ctx, nil
	}

	f, err := os.Open(absPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", absPath, err)
	}
	defer f.Close()

	// 用 pdfcpu ReadValidateAndOptimize 读 PDF（不写文件）
	conf := model.NewDefaultConfiguration()
	ctx, err := api.ReadValidateAndOptimize(f, conf)
	if err != nil {
		return nil, fmt.Errorf("parse PDF: %w", err)
	}

	sharedCtx.ctx = ctx
	sharedCtx.path = absPath
	sharedCtx.modTime = info.ModTime().UnixNano()
	return ctx, nil
}

// isWithinWorkspace 已移除——沙箱策略由宿主工具流水线 pre-execute 瀑布统一判定。
// 本插件不做 workspace 越界检查，与 tool-filesystem / tool-str-replace-editor 一致。

// ---------- 工具 1: pdf_read_text ----------

// handleReadText 提取 PDF 纯文本。
// 参数 file_path 必填；pages 选填（如 "1-3,5,7-9"），缺省全文档。
func handleReadText(ctx context.Context, args json.RawMessage) (string, error) {
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

	// 解析选页
	selected, err := parsePageSelection(p.Pages, pageCount)
	if err != nil {
		return "", fmt.Errorf("invalid pages %q: %w", p.Pages, err)
	}

	// 限制最大页数（防大 PDF 撑爆上下文）
	maxPages := p.MaxPages
	if maxPages == 0 {
		maxPages = 100
	}
	if len(selected) > maxPages {
		selected = selected[:maxPages]
	}

	// 逐页提取
	var b strings.Builder
	fmt.Fprintf(&b, "=== %s (%d pages, extracting %d) ===\n\n",
		filepath.Base(p.FilePath), pageCount, len(selected))

	for i, pageNr := range selected {
		text, err := extractPageText(pdfCtx, pageNr)
		if err != nil {
			fmt.Fprintf(&b, "--- Page %d (extraction failed: %v) ---\n", pageNr, err)
			continue
		}
		text = strings.TrimSpace(text)
		if text == "" {
			fmt.Fprintf(&b, "--- Page %d (no text content; likely image-only) ---\n", pageNr)
		} else {
			fmt.Fprintf(&b, "--- Page %d ---\n%s\n\n", pageNr, text)
		}
		_ = i
	}

	result := b.String()
	// 估算字符数并按需截断（防超大 PDF 撑爆上下文）
	const maxResultChars = 100 * 1024
	if len(result) > maxResultChars {
		result = result[:maxResultChars] + fmt.Sprintf("\n\n[... truncated: total %d chars, showing first %d ...]", len(result), maxResultChars)
	}
	return result, nil
}

// parsePageSelection 解析页选择字符串（如 "1-3,5,7-9"）为页号列表（1-based）。
// 空字符串返回全部页 [1..pageCount]。
func parsePageSelection(s string, pageCount int) ([]int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		out := make([]int, pageCount)
		for i := 0; i < pageCount; i++ {
			out[i] = i + 1
		}
		return out, nil
	}

	var out []int
	parts := strings.Split(s, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "-") {
			rangeParts := strings.SplitN(part, "-", 2)
			lo, err1 := parseInt(strings.TrimSpace(rangeParts[0]))
			hi, err2 := parseInt(strings.TrimSpace(rangeParts[1]))
			if err1 != nil || err2 != nil || lo < 1 || hi < lo || hi > pageCount {
				return nil, fmt.Errorf("invalid range %q", part)
			}
			for p := lo; p <= hi; p++ {
				out = append(out, p)
			}
		} else {
			p, err := parseInt(part)
			if err != nil || p < 1 || p > pageCount {
				return nil, fmt.Errorf("invalid page %q", part)
			}
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no pages selected")
	}
	return out, nil
}

func parseInt(s string) (int, error) {
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return 0, err
	}
	return n, nil
}

// ---------- 工具 2: pdf_info ----------

// handleInfo 返回 PDF 元数据：页数、版本、加密状态、页面尺寸、字体列表。
func handleInfo(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		FilePath string `json:"file_path"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if p.FilePath == "" {
		return "", fmt.Errorf("file_path is required")
	}

	f, err := os.Open(p.FilePath)
	if err != nil {
		return "", fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	conf := model.NewDefaultConfiguration()
	info, err := api.PDFInfo(f, filepath.Base(p.FilePath), nil, true, conf)
	if err != nil {
		return "", fmt.Errorf("PDFInfo: %w", err)
	}

	// 重新解析 Context 以拿 PageCount 与字体
	pdfCtx, err := loadPDFContext(p.FilePath)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "=== %s ===\n", filepath.Base(p.FilePath))
	fmt.Fprintf(&b, "Pages: %d\n", pdfCtx.PageCount)
	if info != nil {
		fmt.Fprintf(&b, "PDF Version: %s\n", strOrDefault(info.Version, "1.4"))
		if len(info.Dimensions) > 0 {
			d := info.Dimensions[0]
			fmt.Fprintf(&b, "Page Size: %.0f x %.0f points\n", d.Width, d.Height)
		}
		fmt.Fprintf(&b, "Encrypted: %v\n", pdfCtx.Encrypt != nil)
		if info.Title != "" {
			fmt.Fprintf(&b, "Title: %s\n", info.Title)
		}
		if info.Author != "" {
			fmt.Fprintf(&b, "Author: %s\n", info.Author)
		}
		if info.Subject != "" {
			fmt.Fprintf(&b, "Subject: %s\n", info.Subject)
		}
		if len(info.Keywords) > 0 {
			fmt.Fprintf(&b, "Keywords: %s\n", strings.Join(info.Keywords, ", "))
		}
		if info.Creator != "" {
			fmt.Fprintf(&b, "Creator: %s\n", info.Creator)
		}
		if info.Producer != "" {
			fmt.Fprintf(&b, "Producer: %s\n", info.Producer)
		}
		if info.CreationDate != "" {
			fmt.Fprintf(&b, "Created: %s\n", info.CreationDate)
		}
		if info.ModificationDate != "" {
			fmt.Fprintf(&b, "Modified: %s\n", info.ModificationDate)
		}
	}

	return b.String(), nil
}

func strOrDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// ---------- 工具 3: pdf_outline ----------

// handleOutline 返回 PDF 书签大纲（目录树）。
func handleOutline(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		FilePath string `json:"file_path"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if p.FilePath == "" {
		return "", fmt.Errorf("file_path is required")
	}

	f, err := os.Open(p.FilePath)
	if err != nil {
		return "", fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	conf := model.NewDefaultConfiguration()
	bms, err := api.Bookmarks(f, conf)
	if err != nil {
		return "", fmt.Errorf("bookmarks: %w", err)
	}
	if len(bms) == 0 {
		return "This PDF has no bookmarks/outline.", nil
	}

	var b strings.Builder
	b.WriteString("=== Outline ===\n")
	for _, bm := range bms {
		renderBookmark(&b, bm, 0)
	}
	return b.String(), nil
}

func renderBookmark(b *strings.Builder, bm pdfcpu.Bookmark, depth int) {
	indent := strings.Repeat("  ", depth)
	pageStr := ""
	if bm.PageFrom > 0 {
		pageStr = fmt.Sprintf(" (p.%d)", bm.PageFrom)
	}
	fmt.Fprintf(b, "%s- %s%s\n", indent, bm.Title, pageStr)
	for _, kid := range bm.Kids {
		renderBookmark(b, kid, depth+1)
	}
}

// ---------- 工具 4: pdf_search ----------

// handleSearch 在 PDF 全文搜索关键词，返回命中页号与上下文片段。
func handleSearch(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		FilePath string `json:"file_path"`
		Query    string `json:"query"`
		MaxHits  int    `json:"max_hits,omitempty"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if p.FilePath == "" {
		return "", fmt.Errorf("file_path is required")
	}
	if p.Query == "" {
		return "", fmt.Errorf("query is required")
	}
	if p.MaxHits == 0 {
		p.MaxHits = 20
	}

	pdfCtx, err := loadPDFContext(p.FilePath)
	if err != nil {
		return "", err
	}

	query := strings.ToLower(p.Query)
	var b strings.Builder
	fmt.Fprintf(&b, "Searching %q in %s (%d pages)...\n\n", p.Query, filepath.Base(p.FilePath), pdfCtx.PageCount)

	hits := 0
	for pageNr := 1; pageNr <= pdfCtx.PageCount && hits < p.MaxHits; pageNr++ {
		text, err := extractPageText(pdfCtx, pageNr)
		if err != nil || text == "" {
			continue
		}
		lower := strings.ToLower(text)
		idx := strings.Index(lower, query)
		if idx < 0 {
			continue
		}
		// 取上下文片段（前后 80 字符）
		ctxStart := idx - 80
		if ctxStart < 0 {
			ctxStart = 0
		}
		ctxEnd := idx + len(query) + 80
		if ctxEnd > len(text) {
			ctxEnd = len(text)
		}
		snippet := text[ctxStart:ctxEnd]
		// 清理换行
		snippet = strings.ReplaceAll(snippet, "\n", " ⏎ ")
		fmt.Fprintf(&b, "Page %d: ...%s...\n\n", pageNr, snippet)
		hits++
	}

	if hits == 0 {
		b.WriteString("No matches found.\n")
	} else {
		fmt.Fprintf(&b, "=== %d hit(s) ===\n", hits)
	}
	return b.String(), nil
}

// ---------- 工具 5: pdf_extract_images ----------

// handleExtractImages 提取 PDF 嵌入图片到指定目录。
func handleExtractImages(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		FilePath string `json:"file_path"`
		OutDir   string `json:"out_dir,omitempty"`
		Pages    string `json:"pages,omitempty"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if p.FilePath == "" {
		return "", fmt.Errorf("file_path is required")
	}
	if p.OutDir == "" {
		// 默认输出到工作空间内 spill 同级目录
		p.OutDir = filepath.Join(workspaceRoot(), "pdf-images",
			strings.TrimSuffix(filepath.Base(p.FilePath), ".pdf"))
	}

	// 沙箱策略由宿主流水线统一判定，本插件不做越界检查
	if err := os.MkdirAll(p.OutDir, 0755); err != nil {
		return "", fmt.Errorf("create out_dir: %w", err)
	}

	f, err := os.Open(p.FilePath)
	if err != nil {
		return "", fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	// 解析选页（缺省全部）
	pdfCtx, err := loadPDFContext(p.FilePath)
	if err != nil {
		return "", err
	}
	selectedPages, err := parsePageSelection(p.Pages, pdfCtx.PageCount)
	if err != nil {
		return "", err
	}

	// 转为 pdfcpu 选页格式（"1,3,5-7"）
	var pageStrs []string
	if p.Pages != "" {
		pageStrs = []string{p.Pages}
	}
	_ = selectedPages

	var extracted []string
	err = api.ExtractImages(f, pageStrs, func(img model.Image, singleImgPerPage bool, maxPageDigits int) error {
		// 使用 pdfcpu 提供的 WriteImageToDisk
		return api.WriteImageToDisk(p.OutDir, filepath.Base(p.FilePath))(img, singleImgPerPage, maxPageDigits)
	}, nil)
	if err != nil {
		// 部分图片提取失败不中断整体
		extracted = append(extracted, fmt.Sprintf("(partial: %v)", err))
	}

	// 列出实际写入的文件
	entries, _ := os.ReadDir(p.OutDir)
	for _, e := range entries {
		if !e.IsDir() {
			extracted = append(extracted, filepath.Join(p.OutDir, e.Name()))
		}
	}

	if len(extracted) == 0 {
		return "No images extracted (PDF may have no embedded images, or extraction failed).", nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Extracted %d image(s) to %s:\n", len(extracted), p.OutDir)
	for _, f := range extracted {
		b.WriteString("  - " + f + "\n")
	}
	return b.String(), nil
}

// ---------- 字符串处理辅助 ----------

// truncateUTF8 安全截断 UTF-8 字符串，避免在多字节字符中间切断。
func truncateUTF8(s string, max int) string {
	if len(s) <= max {
		return s
	}
	// 找到不超过 max 字节的最长合法 UTF-8 边界
	for max > 0 {
		if utf8.RuneStart(s[max]) {
			break
		}
		max--
	}
	return s[:max]
}

// ---------- 主入口 ----------

func main() {
	sdk := dsc.New(dsc.Config{
		Name:    "pdf",
		Version: "1.0.0",
		Type:    dsc.TypeTool,
		Provides: map[string]string{
			"pdf": "true", // 提供 PDF 处理能力
		},
	})

	// 工具 1: pdf_read_text
	sdk.Tool(dsc.Tool{
		Name:        "pdf_read_text",
		Description: "Extract plain text content from a PDF file. Handles WinAnsi/MacRoman encoded fonts and ToUnicode CMap for CJK. Returns text per page. Use this to read PDF documents directly rather than relying on visual model interpretation.",
		Schema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "file_path": {"type": "string", "description": "Path to the PDF file (must be within workspace root)."},
    "pages": {"type": "string", "description": "Optional page selection, e.g. \"1-3,5,7-9\". Omit for all pages.", "default": ""},
    "max_pages": {"type": "integer", "description": "Maximum pages to extract (default 100, prevents huge PDFs from overflowing context).", "default": 100, "minimum": 1, "maximum": 1000}
  },
  "required": ["file_path"]
}`),
		Handler: handleReadText,
		ContextFn: func() string {
			return "PDF 工具集（读取 + 创建）：读取侧 pdf_read_text/pdf_info/pdf_outline/pdf_search/pdf_extract_images；创建侧 pdf_create_text（从文本生成 PDF，自动分页，支持中文）/pdf_images_to_pdf（图片转 PDF）/pdf_append_text（向已有 PDF 追加文本页）。标准 14 字体开箱即用。" + listAvailableCJKFonts()
		},
	})

	// 工具 2: pdf_info
	sdk.Tool(dsc.Tool{
		Name:        "pdf_info",
		Description: "Get metadata of a PDF file: page count, PDF version, page dimensions, encryption status, title/author/subject/keywords. Useful for understanding a PDF before extracting text.",
		Schema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "file_path": {"type": "string", "description": "Path to the PDF file."}
  },
  "required": ["file_path"]
}`),
		Handler: handleInfo,
	})

	// 工具 3: pdf_outline
	sdk.Tool(dsc.Tool{
		Name:        "pdf_outline",
		Description: "Get the bookmark outline (table of contents) of a PDF. Returns the tree structure of bookmarks with page numbers if available.",
		Schema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "file_path": {"type": "string", "description": "Path to the PDF file."}
  },
  "required": ["file_path"]
}`),
		Handler: handleOutline,
	})

	// 工具 4: pdf_search
	sdk.Tool(dsc.Tool{
		Name:        "pdf_search",
		Description: "Search for a keyword in a PDF file and return matching pages with context snippets. Case-insensitive. Useful for finding specific content in large PDFs without extracting all text.",
		Schema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "file_path": {"type": "string", "description": "Path to the PDF file."},
    "query": {"type": "string", "description": "Search query (case-insensitive)."},
    "max_hits": {"type": "integer", "description": "Maximum number of matches to return (default 20).", "default": 20, "minimum": 1, "maximum": 200}
  },
  "required": ["file_path", "query"]
}`),
		Handler: handleSearch,
	})

	// 工具 5: pdf_extract_images
	sdk.Tool(dsc.Tool{
		Name:        "pdf_extract_images",
		Description: "Extract embedded images from a PDF file to a local directory. Useful when a PDF is image-based (no extractable text) and you need the images for visual analysis.",
		Schema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "file_path": {"type": "string", "description": "Path to the PDF file."},
    "out_dir": {"type": "string", "description": "Output directory for extracted images. Must be within workspace root. If omitted, defaults to <workspace>/pdf-images/<filename>/."},
    "pages": {"type": "string", "description": "Optional page selection for image extraction, e.g. \"1-3\".", "default": ""}
  },
  "required": ["file_path"]
}`),
		Handler: handleExtractImages,
	})

	// 工具 6: pdf_create_text —— 从纯文本创建 PDF
	sdk.Tool(dsc.Tool{
		Name:        "pdf_create_text",
		Description: "Create a new PDF file from plain text. Supports the standard 14 fonts (Times/Helvetica/Courier variants, Symbol, ZapfDingbats) and any bundled CJK TrueType fonts (.ttf in the plugin fonts/ directory) that are embedded automatically. Auto-pagination and A4/Letter/Legal paper sizes. Text is split by newlines; each page holds as many lines as fit. Use this to generate PDF reports, documents, or code listings, including Chinese-language documents. Available CJK fonts are listed in the system prompt context.",
		Schema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "out_path": {"type": "string", "description": "Output PDF file path (must be within workspace root). Will be created or overwritten."},
    "text": {"type": "string", "description": "Text content for the PDF. Newlines (\\n) start new lines. For Chinese document use one of the bundled CJK fonts."},
    "font": {"type": "string", "description": "Font name (default Helvetica). Standard 14: Times-Roman, Times-Bold, Times-Italic, Times-BoldItalic, Helvetica, Helvetica-Bold, Helvetica-Oblique, Helvetica-BoldOblique, Courier, Courier-Bold, Courier-Oblique, Courier-BoldOblique, Symbol, ZapfDingbats. Or any bundled CJK TrueType font name from the plugin fonts/ directory (see system prompt for available fonts).", "default": "Helvetica"},
    "font_size": {"type": "number", "description": "Font size in points (default 12).", "default": 12, "minimum": 4, "maximum": 200},
    "paper": {"type": "string", "description": "Paper size (default A4). Options: A4, A4P, A4L (landscape), Letter, LetterP, LetterL, Legal, LegalP, LegalL.", "default": "A4"},
    "margin": {"type": "number", "description": "Page margin in points (default 50). Must be less than half of paper width/height.", "default": 50, "minimum": 0, "maximum": 300}
  },
  "required": ["out_path", "text"]
}`),
		Handler: handleCreateText,
	})

	// 工具 7: pdf_images_to_pdf —— 图片列表转 PDF
	sdk.Tool(dsc.Tool{
		Name:        "pdf_images_to_pdf",
		Description: "Convert a list of image files into a PDF (one image per page). Supports JPG, PNG, TIFF, and WEBP. Useful for creating PDFs from screenshots, scanned documents, or when you need to embed visual content that text-based PDF creation cannot handle (e.g., CJK text without embedded fonts, complex layouts).",
		Schema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "image_paths": {"type": "array", "items": {"type": "string"}, "description": "List of image file paths (must be within workspace root). Each image becomes one page."},
    "out_path": {"type": "string", "description": "Output PDF file path (must be within workspace root)."}
  },
  "required": ["image_paths", "out_path"]
}`),
		Handler: handleImagesToPDF,
	})

	// 工具 8: pdf_append_text —— 在已有 PDF 末尾追加文本页
	sdk.Tool(dsc.Tool{
		Name:        "pdf_append_text",
		Description: "Append text as new page(s) to an existing PDF file. The original content is preserved; new pages are added at the end. Useful for adding conclusions, appendices, or notes to an existing document. Supports standard 14 fonts and bundled CJK fonts (embedded). Uses the same font/pagination as pdf_create_text.",
		Schema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "file_path": {"type": "string", "description": "Path to the existing PDF file (must be within workspace root). Will be modified in place."},
    "text": {"type": "string", "description": "Text content to append. Newlines (\\n) start new lines."},
    "font": {"type": "string", "description": "Font name (default Helvetica). Must be a standard 14 font or a bundled CJK TrueType font (see system prompt for available fonts).", "default": "Helvetica"},
    "font_size": {"type": "number", "description": "Font size in points (default 12).", "default": 12, "minimum": 4, "maximum": 200},
    "paper": {"type": "string", "description": "Paper size for new pages (default A4). Original pages keep their size.", "default": "A4"},
    "margin": {"type": "number", "description": "Page margin in points (default 50).", "default": 50, "minimum": 0, "maximum": 300}
  },
  "required": ["file_path", "text"]
}`),
		Handler: handleAppendText,
	})

	// 工具 9: pdf_extract_tables —— 提取 PDF 表格结构
	sdk.Tool(dsc.Tool{
		Name:        "pdf_extract_tables",
		Description: "Detect and extract table-like structure from a PDF: returns pages as column-aligned grids (columns separated by '|'). Works when text is laid out in aligned columns (reports, invoices, schedules). If no aligned-column table is detected, falls back to advising pdf_read_text. Use this when you need row/column structure rather than a flat textual stream.",
		Schema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "file_path": {"type": "string", "description": "Path to the PDF file (must be within workspace root)."},
    "pages": {"type": "string", "description": "Optional page selection, e.g. \"1,3,5-7\". Omit for all pages.", "default": ""},
    "max_pages": {"type": "integer", "description": "Maximum pages to scan (default 20).", "default": 20, "minimum": 1, "maximum": 1000}
  },
  "required": ["file_path"]
}`),
		Handler: handleExtractTables,
	})

	// 工具 10: pdf_merge_pdfs —— 合并多个 PDF
	sdk.Tool(dsc.Tool{
		Name:        "pdf_merge_pdfs",
		Description: "Merge multiple PDF files into a single PDF in the given order. Useful for combining scanned docs, reports, or chapters into one file.",
		Schema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "input_paths": {"type": "array", "items": {"type": "string"}, "description": "List of PDF paths to merge, in order (must be within workspace root)."},
    "out_path": {"type": "string", "description": "Output PDF path (must be within workspace root)."},
    "divider_page": {"type": "boolean", "description": "Insert a blank divider page between each merged document (default false).", "default": false}
  },
  "required": ["input_paths", "out_path"]
}`),
		Handler: handleMergePDFs,
	})

	// 工具 11: pdf_split_pdfs —— 按页拆分 PDF
	sdk.Tool(dsc.Tool{
		Name:        "pdf_split_pdfs",
		Description: "Split a PDF into multiple PDFs by page span (default 1 page each), writing them to an output directory. Useful for separating pages or chapters.",
		Schema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "file_path": {"type": "string", "description": "Path to the PDF to split (must be within workspace root)."},
    "out_dir": {"type": "string", "description": "Output directory for the split PDFs (must be within workspace root)."},
    "span": {"type": "integer", "description": "Number of pages per output part (default 1).", "default": 1, "minimum": 1, "maximum": 10000}
  },
  "required": ["file_path", "out_dir"]
}`),
		Handler: handleSplitPDFs,
	})

	// 工具 12: pdf_extract_pages —— 抽选页生成新 PDF
	sdk.Tool(dsc.Tool{
		Name:        "pdf_extract_pages",
		Description: "Extract a selection of pages from a PDF into a new single PDF, preserving order. Pages can be listed and ranged, e.g. \"1,3,5-7\". Useful for building a sub-document or removing pages.",
		Schema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "file_path": {"type": "string", "description": "Path to the source PDF (must be within workspace root)."},
    "out_path": {"type": "string", "description": "Output PDF path (must be within workspace root)."},
    "pages": {"type": "string", "description": "Page selection, e.g. \"1,3,5-7\"."}
  },
  "required": ["file_path", "out_path", "pages"]
}`),
		Handler: handleExtractPagesTool,
	})

	// 工具 13: pdf_to_images —— 页面转 PNG（需外部渲染器）
	// 暂时禁用：依赖外部命令（mutool/pdftoppm/gs），未真机验证。恢复时取消本注册块注释即可，
	// 实现保留于 pdf_render.go（含命令构造与渲染器探测逻辑）。
	/*
	           sdk.Tool(dsc.Tool{
	                   Name:        "pdf_to_images",
	                   Description: "Render PDF pages to PNG images in an output directory for visual/layout analysis. Requires an external rasterizer (mutool / pdftoppm / Ghostscript) on PATH, or set DSC_PDF_RENDERER. If none is available, use pdf_extract_images to extract embedded images instead.",
	                   Schema: json.RawMessage(`{
	     "type": "object",
	     "properties": {
	       "file_path": {"type": "string", "description": "Path to the PDF file (must be within workspace root)."},
	       "pages": {"type": "string", "description": "Optional page selection. Rendered as a contiguous page block covering the selection. Omit for all pages.", "default": ""},
	       "out_dir": {"type": "string", "description": "Output directory for PNG images (default <workspace>/pdf-images/<name>/render/)."},
	       "dpi": {"type": "integer", "description": "Render resolution in DPI (default 150).", "default": 150, "minimum": 50, "maximum": 600}
	     },
	     "required": ["file_path"]
	   }`),
	                   Handler: handlePageToImages,
	           })
	*/

	sdk.Serve()
}

// 抑制未使用警告（loadPageFontDecoders 中的部分代码路径暂未用到 sort）
var _ = sort.Strings
