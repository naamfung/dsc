// Package main — tool-pdf 插件的 PDF 创建模块。
//
// 基于 pdfcpu 的 XRefTable + Page + TextDescriptor API 创建 PDF 文件。
// 支持标准 14 字体（Times / Helvetica / Courier 及 Bold/Italic 变体），
// 以及插件自带的内嵌 CJK 字体（如 HarmonyOS Sans SC，走 Type0 嵌入子集路径）。
// CJK 文本按可用行宽做字符级折行（含避头尾），自动分页，纸张可选（A4 / Letter / Legal）。
//
// 设计原则：
//   - 标准 14 字体无需嵌入，开箱即用
//   - 中文字体经 pdfcpu 用户字体注册表嵌入子集，渲染后可正常被读取器提取
//   - 自动分页按字号 × 1.5 行距估算，保守不溢出
//   - 所有输出路径必须在工作空间根内（沙箱边界）
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	pdfcpulib "github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	pdffont "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/font"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// 标准纸张尺寸（points，1 inch = 72 points）
var paperSizes = map[string]types.Dim{
	"A4":      {Width: 595, Height: 842},
	"A4P":     {Width: 595, Height: 842},
	"A4L":     {Width: 842, Height: 595},
	"Letter":  {Width: 612, Height: 792},
	"LetterP": {Width: 612, Height: 792},
	"LetterL": {Width: 792, Height: 612},
	"Legal":   {Width: 612, Height: 1008},
	"LegalP":  {Width: 612, Height: 1008},
	"LegalL":  {Width: 1008, Height: 612},
}

// 标准 14 字体清单
var standardFonts = []string{
	"Times-Roman", "Times-Bold", "Times-Italic", "Times-BoldItalic",
	"Helvetica", "Helvetica-Bold", "Helvetica-Oblique", "Helvetica-BoldOblique",
	"Courier", "Courier-Bold", "Courier-Oblique", "Courier-BoldOblique",
	"Symbol", "ZapfDingbats",
}

// createPDFFromText 从纯文本创建 PDF 文件。
//
// 参数：
//   - outPath: 输出 PDF 路径（必须在工作空间内）
//   - text: 文本内容（\n 分行）
//   - fontName: 字体名（默认 "Helvetica"；可选标准 14 字体之一或内置 CJK 字体名）
//   - fontSize: 字号（默认 12）
//   - paper: 纸张（默认 "A4"）
//   - margin: 页边距（默认 50 points）
//
// 返回：成功后返回输出文件路径与总页数。
func createPDFFromText(outPath, text, fontName string, fontSize float64, paper string, margin float64) (string, int, error) {
	// 1. 参数校验 + 默认值
	if outPath == "" {
		return "", 0, fmt.Errorf("out_path is required")
	}
	if text == "" {
		return "", 0, fmt.Errorf("text is required (cannot create empty PDF)")
	}
	if fontName == "" {
		fontName = "Helvetica"
	}
	// 解析字体：标准 14 字体原样；内置 CJK 字体解析为 pdfcpu PostScript 名并走嵌入子集路径
	pdfName, cjk, err := resolveFontName(fontName)
	if err != nil {
		return "", 0, err
	}
	fontName = pdfName
	if fontSize <= 0 || fontSize > 200 {
		fontSize = 12
	}
	if paper == "" {
		paper = "A4"
	}
	dim, ok := paperSizes[paper]
	if !ok {
		return "", 0, fmt.Errorf("unsupported paper %q (available: A4, Letter, Legal, with P/L suffix for portrait/landscape)", paper)
	}
	if margin < 0 || margin > dim.Width/2 || margin > dim.Height/2 {
		margin = 50
	}

	// 2. 沙箱边界：out_path 必须在工作空间内
	absOut, err := filepath.Abs(outPath)
	if err != nil {
		return "", 0, fmt.Errorf("resolve out_path: %w", err)
	}
	wsRoot := workspaceRoot()
	if !isWithinWorkspace(absOut, wsRoot) {
		return "", 0, fmt.Errorf("out_path %q is outside workspace root %q", absOut, wsRoot)
	}

	// 3. 创建空 PDF Context（含 pageTree root，无页）
	conf := model.NewDefaultConfiguration()
	ctx, err := pdfcpulib.CreateContextWithXRefTable(conf, &dim)
	if err != nil {
		return "", 0, fmt.Errorf("create PDF context: %w", err)
	}
	xRefTable := ctx.XRefTable

	// 重新断言字体目录：NewDefaultConfiguration 会把 font.UserFontDir 重置为用户目录，
	// 而嵌入字体（.gob）实际安装在进程级目录中，必须在使用字体前改回。
	if cjk {
		if _, err := ensureUserFontDir(); err != nil {
			return "", 0, err
		}
	}

	// 4. 注册字体到 XRefTable
	// 标准 14 字体仅创建字体字典引用；CJK 字体创建嵌入 Type0 子集字典（插入 W/CIDSet/ToUnicode 引用，
	// 占位内容；实际子集在步骤 7.5 写入前按已用 GID 收尾）。
	fontIndRef, err := pdffont.EnsureFontDict(xRefTable, fontName, "", "", false, nil)
	if err != nil {
		return "", 0, fmt.Errorf("ensure font %s: %w", fontName, err)
	}

	// 5. 计算每页可容纳的行数（保守估算：行高 = 字号 × 1.5）
	lineHeight := fontSize * 1.5
	usableHeight := dim.Height - 2*margin
	maxLinesPerPage := int(usableHeight / lineHeight)
	if maxLinesPerPage < 1 {
		maxLinesPerPage = 1
	}

	// 6. 按行切分文本并按可用行宽折行（CJK 字符级折行，Latin 保持原样避免回归）
	lines, err := wrapTextForRender(text, fontName, fontSize, dim.Width-2*margin, cjk)
	if err != nil {
		return "", 0, err
	}

	// 7. 分页渲染
	pageCount := 0
	for start := 0; start < len(lines); start += maxLinesPerPage {
		end := start + maxLinesPerPage
		if end > len(lines) {
			end = len(lines)
		}
		pageLines := lines[start:end]
		if err := addPageWithText(ctx, xRefTable, fontIndRef, fontName, pageLines, fontSize, dim, margin, lineHeight, cjk); err != nil {
			return "", 0, fmt.Errorf("page %d: %w", pageCount+1, err)
		}
		pageCount++
	}

	if pageCount == 0 {
		// 至少一页（即使内容为空）
		if err := addPageWithText(ctx, xRefTable, fontIndRef, fontName, []string{""}, fontSize, dim, margin, lineHeight, cjk); err != nil {
			return "", 0, fmt.Errorf("page 1: %w", err)
		}
		pageCount = 1
	}

	// 7.5 收尾嵌入字体：按写入过程中累计的已用 GID 重新子集化、写宽度/CIDSet/ToUnicode。
	// pdfcpu 的 WriteContext 不自动做此步（仅 stamp/form 等路径手动调用），必须显式收尾，
	// 否则嵌入字体将只有 .notdef（glyph 0），渲染为空。
	if cjk {
		if _, err := ensureUserFontDir(); err != nil {
			return "", 0, err
		}
		if err := pdffont.UpdateUserfonts(xRefTable, map[string]types.IndirectRef{fontName: *fontIndRef}); err != nil {
			return "", 0, fmt.Errorf("finalize font %s: %w", fontName, err)
		}
	}

	// 8. 写入文件
	if err := os.MkdirAll(filepath.Dir(absOut), 0755); err != nil {
		return "", 0, fmt.Errorf("create output dir: %w", err)
	}
	if err := api.CreatePDFFile(xRefTable, absOut, conf); err != nil {
		return "", 0, fmt.Errorf("write PDF: %w", err)
	}

	return absOut, pageCount, nil
}

// addPageWithText 向 PDF Context 添加一页，写入多行文本。
// 文本从页面左上角开始，按行高向下排列（PDF 坐标系 y 轴向上，故首行 y 最高）。
// embed 为 true 表示字体为内嵌 CJK 字体：WriteMultiLine 需以 Embed 模式把每个
// Unicode 码点编码为 2 字节 GID，并累计 UsedGIDs。
func addPageWithText(ctx *model.Context, xRefTable *model.XRefTable, fontIndRef *types.IndirectRef, fontName string, lines []string, fontSize float64, dim types.Dim, margin, lineHeight float64, embed bool) error {
	// 1. 创建页面对象（pdfcpu model.Page：包含 MediaBox + FontMap + 内容 Buf）
	p := model.Page{
		MediaBox: types.RectForDim(dim.Width, dim.Height),
		Fm:       model.FontMap{},
		Buf:      new(bytes.Buffer),
	}

	// 注册字体到页面字体映射（FontKey 用于 WriteMultiLine 引用）
	fontKey := p.Fm.EnsureKey(fontName)
	// 显式覆盖资源 ID 与引用（确保字体引用一致）
	if fr, ok := p.Fm[fontName]; ok {
		fr.Res.IndRef = fontIndRef
		p.Fm[fontName] = fr
	} else {
		p.Fm[fontName] = model.FontResource{
			Res: model.Resource{ID: fontKey, IndRef: fontIndRef},
		}
	}

	// 2. 逐行写入文本
	// 起始 y：页面顶部 - margin - 行高（首行基线）
	startY := dim.Height - margin - lineHeight
	for i, line := range lines {
		y := startY - float64(i)*lineHeight
		if y < margin {
			break // 超出页面底部，剩余行丢弃（应由分页逻辑避免）
		}
		// 空行跳过 WriteMultiLine（pdfcpu 对空文本会报 "no text lines"）
		// 但仍需占位以保持行高
		if strings.TrimSpace(line) == "" {
			continue
		}
		// 转义 PDF 字符串字面量特殊字符。
		// 标准 14 字体绕开 WriteMultiLine 的自动转义，故在此预转义；
		// 内嵌 CJK 字体由 WriteMultiLine(PrepBytes) 统一编码为 GID 并转义，此处不再预转义。
		text := line
		if !embed {
			text = escapePDFString(line)
		}
		td := model.TextDescriptor{
			Text:     text,
			FontName: fontName,
			FontKey:  fontKey,
			FontSize: fontSize,
			Scale:    1.0,
			ScaleAbs: true,
			Embed:    embed,
			X:        margin,
			Y:        y,
		}
		if _, err := model.WriteMultiLine(xRefTable, p.Buf, p.MediaBox, nil, td); err != nil {
			return fmt.Errorf("write line %d: %w", i+1, err)
		}
	}

	// 3. 创建页面字典 + 内容流 + 注册到 pageTree
	// 获取 Pages 根 indirect ref（作为新页面的 Parent）
	pagesIndRef, err := xRefTable.Pages()
	if err != nil || pagesIndRef == nil {
		return fmt.Errorf("get Pages root: %w", err)
	}

	pageDict := types.Dict{
		"Type":   types.Name("Page"),
		"Parent": *pagesIndRef,
	}

	// 字体资源字典
	fontResDict := types.Dict{}
	for _, fr := range p.Fm {
		if fr.Res.IndRef != nil {
			fontResDict[fr.Res.ID] = *fr.Res.IndRef
		}
	}
	if len(fontResDict) > 0 {
		pageDict["Resources"] = types.Dict{
			"Font": fontResDict,
		}
	}

	// MediaBox
	pageDict["MediaBox"] = p.MediaBox.Array()

	// 内容流
	sd, _ := xRefTable.NewStreamDictForBuf(p.Buf.Bytes())
	if err := sd.Encode(); err != nil {
		return fmt.Errorf("encode content stream: %w", err)
	}
	contentIR, err := xRefTable.IndRefForNewObject(*sd)
	if err != nil {
		return fmt.Errorf("create content stream: %w", err)
	}
	pageDict["Contents"] = *contentIR

	// 注册页面到 pageTree
	pageIR, err := xRefTable.IndRefForNewObject(pageDict)
	if err != nil {
		return fmt.Errorf("create page object: %w", err)
	}

	// 找到 Pages 根字典并 append
	pagesDictObj, err := xRefTable.DereferenceDict(*pagesIndRef)
	if err != nil || pagesDictObj == nil {
		return fmt.Errorf("dereference Pages dict: %w", err)
	}
	if err := model.AppendPageTree(pageIR, 1, pagesDictObj); err != nil {
		return fmt.Errorf("append to page tree: %w", err)
	}

	// 更新 Context.PageCount
	ctx.PageCount++
	return nil
}

// escapePDFString 转义 PDF 字符串字面量中的特殊字符。
// WriteMultiLine 内部会处理字符串字面量生成，但输入若含 ( ) \ 等字符可能破坏结构。
// 保守做法：把 \ ( ) 分别转义为 \\ \( \)。
func escapePDFString(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString("\\\\")
		case '(':
			b.WriteString("\\(")
		case ')':
			b.WriteString("\\)")
		case '\r':
			// CR 转为空（避免与 LF 重复换行）
			continue
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// isStandardFont 报告字体名是否为 PDF 标准 14 字体。
func isStandardFont(name string) bool {
	for _, f := range standardFonts {
		if f == name {
			return true
		}
	}
	return false
}

// handleCreateText 是 pdf_create_text 工具的处理器。
// 从纯文本创建 PDF 文件。
func handleCreateText(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		OutPath  string  `json:"out_path"`
		Text     string  `json:"text"`
		Font     string  `json:"font,omitempty"`
		FontSize float64 `json:"font_size,omitempty"`
		Paper    string  `json:"paper,omitempty"`
		Margin   float64 `json:"margin,omitempty"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}

	absPath, pages, err := createPDFFromText(p.OutPath, p.Text, p.Font, p.FontSize, p.Paper, p.Margin)
	if err != nil {
		return "", err
	}

	// 返回结构化结果
	result := fmt.Sprintf("Created PDF: %s\nPages: %d\nFont: %s, Size: %g, Paper: %s\n",
		absPath, pages, strOrDefault(p.Font, "Helvetica"), defaultFloat(p.FontSize, 12), strOrDefault(p.Paper, "A4"))
	return result, nil
}

// handleImagesToPDF 是 pdf_images_to_pdf 工具的处理器。
// 把图片列表转换为 PDF（每张图一页）。
func handleImagesToPDF(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		ImagePaths []string `json:"image_paths"`
		OutPath    string   `json:"out_path"`
		Paper      string   `json:"paper,omitempty"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if len(p.ImagePaths) == 0 {
		return "", fmt.Errorf("image_paths is required (at least one image)")
	}
	if p.OutPath == "" {
		return "", fmt.Errorf("out_path is required")
	}

	// 沙箱边界
	absOut, err := filepath.Abs(p.OutPath)
	if err != nil {
		return "", fmt.Errorf("resolve out_path: %w", err)
	}
	wsRoot := workspaceRoot()
	if !isWithinWorkspace(absOut, wsRoot) {
		return "", fmt.Errorf("out_path %q is outside workspace root %q", absOut, wsRoot)
	}
	for _, imgPath := range p.ImagePaths {
		absImg, err := filepath.Abs(imgPath)
		if err != nil || !isWithinWorkspace(absImg, wsRoot) {
			return "", fmt.Errorf("image_path %q is outside workspace root", imgPath)
		}
	}

	// 使用 pdfcpu 的 ImportImagesFile
	imp := api.DefaultImportConfig()
	if p.Paper != "" {
		// paper 仅用于布局提示，ImportImages 默认按图片尺寸生成页面
		_ = p.Paper
	}

	if err := api.ImportImagesFile(p.ImagePaths, p.OutPath, imp, nil); err != nil {
		return "", fmt.Errorf("import images: %w", err)
	}

	result := fmt.Sprintf("Created PDF: %s\nPages: %d (one image per page)\n", absOut, len(p.ImagePaths))
	return result, nil
}

// handleAppendText 是 pdf_append_text 工具的处理器。
// 在已有 PDF 末尾追加若干页文本。
func handleAppendText(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		FilePath string  `json:"file_path"`
		Text     string  `json:"text"`
		Font     string  `json:"font,omitempty"`
		FontSize float64 `json:"font_size,omitempty"`
		Paper    string  `json:"paper,omitempty"`
		Margin   float64 `json:"margin,omitempty"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if p.FilePath == "" {
		return "", fmt.Errorf("file_path is required")
	}
	if p.Text == "" {
		return "", fmt.Errorf("text is required")
	}

	// 沙箱边界
	absPath, err := filepath.Abs(p.FilePath)
	if err != nil {
		return "", fmt.Errorf("resolve file_path: %w", err)
	}
	wsRoot := workspaceRoot()
	if !isWithinWorkspace(absPath, wsRoot) {
		return "", fmt.Errorf("file_path %q is outside workspace root %q", absPath, wsRoot)
	}

	// 读取原 PDF
	f, err := os.Open(absPath)
	if err != nil {
		return "", fmt.Errorf("open: %w", err)
	}
	conf := model.NewDefaultConfiguration()
	pdfCtx, err := api.ReadValidateAndOptimize(f, conf)
	// 解析完成后立即关闭源文件：ReadValidateAndOptimize 已把结构读入内存，
	// 尽早释放句柄，Windows 下才能原地覆盖写回（rename 目标文件不能正被打开）。
	closeErr := f.Close()
	if err != nil {
		return "", fmt.Errorf("parse PDF: %w", err)
	}
	if closeErr != nil {
		return "", fmt.Errorf("close source PDF: %w", closeErr)
	}
	xRefTable := pdfCtx.XRefTable

	// 参数默认值 + 字体解析
	fontName := p.Font
	if fontName == "" {
		fontName = "Helvetica"
	}
	pdfName, cjk, err := resolveFontName(fontName)
	if err != nil {
		return "", err
	}
	fontName = pdfName

	fontSize := p.FontSize
	if fontSize <= 0 || fontSize > 200 {
		fontSize = 12
	}
	margin := p.Margin
	if margin < 0 || margin > 300 {
		margin = 50
	}
	dim, ok := paperSizes[strOrDefault(p.Paper, "A4")]
	if !ok {
		dim = paperSizes["A4"]
	}

	// 注册字体前重新断言字体目录（NewDefaultConfiguration 已把 UserFontDir 重置）
	if cjk {
		if _, err := ensureUserFontDir(); err != nil {
			return "", err
		}
	}
	fontIndRef, err := pdffont.EnsureFontDict(xRefTable, fontName, "", "", false, nil)
	if err != nil {
		return "", fmt.Errorf("ensure font %s: %w", fontName, err)
	}

	// 分页 + 追加：先按行切分并按可用行宽折行（同 createPDFFromText）
	lines, err := wrapTextForRender(p.Text, fontName, fontSize, dim.Width-2*margin, cjk)
	if err != nil {
		return "", err
	}
	lineHeight := fontSize * 1.5
	usableHeight := dim.Height - 2*margin
	maxLinesPerPage := int(usableHeight / lineHeight)
	if maxLinesPerPage < 1 {
		maxLinesPerPage = 1
	}
	addedPages := 0
	for start := 0; start < len(lines); start += maxLinesPerPage {
		end := start + maxLinesPerPage
		if end > len(lines) {
			end = len(lines)
		}
		pageLines := lines[start:end]
		if err := addPageWithText(pdfCtx, xRefTable, fontIndRef, fontName, pageLines, fontSize, dim, margin, lineHeight, cjk); err != nil {
			return "", fmt.Errorf("append page %d: %w", addedPages+1, err)
		}
		addedPages++
	}

	// 写回文件（覆盖原文件）——先收尾嵌入字体（同 createPDFFromText）
	if cjk {
		if _, err := ensureUserFontDir(); err != nil {
			return "", err
		}
		if err := pdffont.UpdateUserfonts(xRefTable, map[string]types.IndirectRef{fontName: *fontIndRef}); err != nil {
			return "", fmt.Errorf("finalize font %s: %w", fontName, err)
		}
	}

	tmpOut := absPath + ".tmp"
	if err := api.CreatePDFFile(xRefTable, tmpOut, conf); err != nil {
		return "", fmt.Errorf("write PDF: %w", err)
	}
	if err := os.Rename(tmpOut, absPath); err != nil {
		_ = os.Remove(tmpOut)
		return "", fmt.Errorf("replace file: %w", err)
	}

	result := fmt.Sprintf("Appended %d page(s) to: %s\nTotal pages now: %d\nFont: %s, Size: %g\n",
		addedPages, absPath, pdfCtx.PageCount, fontName, fontSize)
	return result, nil
}

// defaultFloat 返回非零值，否则返回 def。
func defaultFloat(v, def float64) float64 {
	if v == 0 {
		return def
	}
	return v
}
