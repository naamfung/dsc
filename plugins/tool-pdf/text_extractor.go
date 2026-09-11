// Package main — tool-pdf 插件的文本提取器。
//
// 从 pdfcpu 的 XRefTable + PageDict 出发，逐页：
//  1. 加载页面字体字典 → 构建 fontDecoder（按字体资源名）
//  2. 读页面内容流字节 → 经 tokenizer 解析为 token 流
//  3. 解释文本相关操作符（BT/ET, Tf, Tm, Td, T*, Tj, TJ, ', "）→ 提取字符串
//  4. 经 fontDecoder 解码为 Unicode → 拼接成行
//  5. 按文本矩阵的 y 坐标对齐分块 → 重组为视觉行序
//
// 位置感知策略：
//   - 用 Tm/Td 跟踪当前文本位置（x, y）
//   - 同一 y 坐标（容差 = 字号 * 0.5）的字符串归为同一行
//   - 行按 y 降序排列（PDF 坐标原点在左下，y 越大越靠上）
//   - 行内字符串按 x 升序排列
//   - 不同行用 \n 分隔，行内字符串用空格分隔（TJ 数组中负 kerning 时不加空格）
package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// textLine 表示一个视觉行（按 y 跨度聚合）。
type textLine struct {
	y    float64
	frag []textFragment
}

type textFragment struct {
	x float64
	s string
}

// extractPageText 从 pdfcpu Context 提取单页文本。
// 失败时返回空串与错误；尽力而为的部分错误（字体解码失败、内容流解析警告）
// 不返回错误，仅影响输出完整性。
func extractPageText(ctx *model.Context, pageNr int) (string, error) {
	// 1. 加载页面字体字典
	decoders, err := loadPageFontDecoders(ctx, pageNr)
	if err != nil {
		// 字体加载失败不应中断提取——继续用默认 WinAnsi 解码
		decoders = map[string]*fontDecoder{}
	}

	// 2. 读页面内容流
	pageDict, _, _, err := ctx.PageDict(pageNr, false)
	if err != nil {
		return "", fmt.Errorf("page %d: page dict: %w", pageNr, err)
	}
	content, err := ctx.PageContent(pageDict, pageNr)
	if err != nil {
		if err == model.ErrNoContent {
			return "", nil
		}
		return "", fmt.Errorf("page %d: content: %w", pageNr, err)
	}

	// 3. tokenize
	toks := tokenizeContentStream(content)

	// 4. 解释操作符，提取带位置的文本片段
	lines := interpretTextOperators(toks, decoders)
	if len(lines) == 0 {
		return "", nil
	}

	// 5. 行排序（y 降序）+ 行内排序（x 升序）+ 拼接
	return renderLines(lines), nil
}

// loadPageFontDecoders 从页面资源字典加载字体，构建 fontDecoder。
// 字体资源名（如 /F1, /F2）映射到对应的 decoder。
func loadPageFontDecoders(ctx *model.Context, pageNr int) (map[string]*fontDecoder, error) {
	pageDict, _, _, err := ctx.PageDict(pageNr, false)
	if err != nil {
		return nil, err
	}
	// 找 Resources → Font
	resObj, found := pageDict.Find("Resources")
	if !found || resObj == nil {
		return map[string]*fontDecoder{}, nil
	}
	resObj, err = ctx.XRefTable.Dereference(resObj)
	if err != nil || resObj == nil {
		return map[string]*fontDecoder{}, nil
	}
	resDict, ok := resObj.(types.Dict)
	if !ok {
		return map[string]*fontDecoder{}, nil
	}
	fontObj, found := resDict.Find("Font")
	if !found || fontObj == nil {
		return map[string]*fontDecoder{}, nil
	}
	fontObj, err = ctx.XRefTable.Dereference(fontObj)
	if err != nil || fontObj == nil {
		return map[string]*fontDecoder{}, nil
	}
	fontDict, ok := fontObj.(types.Dict)
	if !ok {
		return map[string]*fontDecoder{}, nil
	}

	decoders := map[string]*fontDecoder{}
	for name, obj := range fontDict {
		fontName := name
		fontDictObj, err := ctx.XRefTable.DereferenceDict(obj)
		if err != nil || fontDictObj == nil {
			continue
		}
		decoders[fontName] = buildDecoderFromFontDict(ctx.XRefTable, fontDictObj)
	}
	return decoders, nil
}

// buildDecoderFromFontDict 从字体字典构建 decoder。
// 支持的字体类型：Type0（CID）, Type1, TrueType。
// 优先级：ToUnicode CMap > Encoding > BaseEncoding > 默认 WinAnsi。
func buildDecoderFromFontDict(xref *model.XRefTable, d types.Dict) *fontDecoder {
	dec := newFontDecoder()

	// 1. ToUnicode CMap（最高优先级，直接给出 Unicode）
	if toUnicodeRef := d.IndirectRefEntry("ToUnicode"); toUnicodeRef != nil {
		if cmapText, err := loadCMapStream(xref, *toUnicodeRef); err == nil && len(cmapText) > 0 {
			dec.parseToUnicodeCMap(cmapText)
		}
	}

	// 2. Subtype 判断字体类型
	subtype := d.NameEntry("Subtype")
	if subtype != nil && *subtype == "Type0" {
		dec.isCID = true
		// Type0 字体：检查 DescendantFonts → CIDFont → Encoding
		// 通常 ToUnicode CMap 已足够，但若无 ToUnicode 则回退 Identity-H（无法解码）
		if len(dec.toUnicode) == 0 {
			// 没有 ToUnicode，CID 字体无法可靠解码——标记为 Identity
			dec.base = "identity-h"
		}
		return dec
	}

	// 3. Encoding 字典（仅 Type1/TrueType）
	if enc, found := d.Find("Encoding"); found && enc != nil {
		enc, err := xref.Dereference(enc)
		if err == nil && enc != nil {
			switch e := enc.(type) {
			case types.Name:
				switch string(e) {
				case "WinAnsiEncoding":
					dec.base = "winansi"
				case "MacRomanEncoding":
					dec.base = "macroman"
				case "MacExpertEncoding":
					dec.base = "macroman" // 近似处理
				case "StandardEncoding":
					dec.base = "standard"
				}
			case types.Dict:
				// Encoding 字典：BaseEncoding + Differences
				if base := e.NameEntry("BaseEncoding"); base != nil {
					switch *base {
					case "WinAnsiEncoding":
						dec.base = "winansi"
					case "MacRomanEncoding":
						dec.base = "macroman"
					case "StandardEncoding":
						dec.base = "standard"
					}
				}
				if diff, found := e.Find("Differences"); found {
					parseDifferences(xref, diff, dec)
				}
			}
		}
	}

	// 4. 默认：基础 14 字体用 StandardEncoding；其余用 WinAnsi
	if dec.base == "" {
		if baseFont := d.NameEntry("BaseFont"); baseFont != nil && isStandard14Font(*baseFont) {
			dec.base = "standard"
		} else {
			dec.base = "winansi"
		}
	}
	return dec
}

// loadCMapStream 读 ToUnicode CMap 流并解码为字节切片。
func loadCMapStream(xref *model.XRefTable, indRef types.IndirectRef) ([]byte, error) {
	sd, _, err := xref.DereferenceStreamDict(indRef)
	if err != nil {
		return nil, err
	}
	if sd == nil {
		return nil, nil
	}
	// 解码（应用 FlateDecode 等过滤器）
	if err := sd.Decode(); err != nil {
		// 部分情况下内容已是 raw，尝试直接用
		if sd.Content == nil {
			return nil, err
		}
	}
	return sd.Content, nil
}

// parseDifferences 解析 Encoding.Differences 数组，填充 decoder.differences。
// Differences 格式：[ 65 /A 66 /B ... ]（数字后跟名称）
func parseDifferences(xref *model.XRefTable, obj types.Object, dec *fontDecoder) {
	arr, err := xref.DereferenceArray(obj)
	if err != nil || arr == nil {
		return
	}
	var pendingCode byte
	hasPending := false
	for _, o := range arr {
		o, err := xref.Dereference(o)
		if err != nil || o == nil {
			continue
		}
		switch v := o.(type) {
		case types.Integer:
			if v.Value() >= 0 && v.Value() <= 255 {
				pendingCode = byte(v.Value())
				hasPending = true
			}
		case types.Name:
			if hasPending {
				glyphName := string(v)
				if r, ok := glyphToUnicode(glyphName); ok {
					dec.differences[pendingCode] = string(r)
				}
				hasPending = false
			}
		}
	}
}

// isStandard14Font 报告字体名是否为 PDF 标准 14 字体。
var standard14Fonts = map[string]bool{
	"Times-Roman": true, "Times-Bold": true, "Times-Italic": true, "Times-BoldItalic": true,
	"Helvetica": true, "Helvetica-Bold": true, "Helvetica-Oblique": true, "Helvetica-BoldOblique": true,
	"Courier": true, "Courier-Bold": true, "Courier-Oblique": true, "Courier-BoldOblique": true,
	"Symbol": true, "ZapfDingbats": true,
}

func isStandard14Font(name string) bool {
	return standard14Fonts[name]
}

// glyphToUnicode 把 Adobe Glyph Name 映射到 Unicode 字符。
// 仅覆盖最常用的一小部分；其余返回 (0, false)。
var glyphToUnicodeMap = map[string]rune{
	"A": 'A', "B": 'B', "C": 'C', "D": 'D', "E": 'E', "F": 'F', "G": 'G',
	"H": 'H', "I": 'I', "J": 'J', "K": 'K', "L": 'L', "M": 'M', "N": 'N',
	"O": 'O', "P": 'P', "Q": 'Q', "R": 'R', "S": 'S', "T": 'T', "U": 'U',
	"V": 'V', "W": 'W', "X": 'X', "Y": 'Y', "Z": 'Z',
	"a": 'a', "b": 'b', "c": 'c', "d": 'd', "e": 'e', "f": 'f', "g": 'g',
	"h": 'h', "i": 'i', "j": 'j', "k": 'k', "l": 'l', "m": 'm', "n": 'n',
	"o": 'o', "p": 'p', "q": 'q', "r": 'r', "s": 's', "t": 't', "u": 'u',
	"v": 'v', "w": 'w', "x": 'x', "y": 'y', "z": 'z',
	"zero": '0', "one": '1', "two": '2', "three": '3', "four": '4',
	"five": '5', "six": '6', "seven": '7', "eight": '8', "nine": '9',
	"space": ' ', "exclam": '!', "quotedbl": '"', "numbersign": '#',
	"dollar": '$', "percent": '%', "ampersand": '&', "quoteright": '\'',
	"parenleft": '(', "parenright": ')', "asterisk": '*', "plus": '+',
	"comma": ',', "hyphen": '-', "period": '.', "slash": '/',
	"colon": ':', "semicolon": ';', "less": '<', "equal": '=', "greater": '>',
	"question": '?', "at": '@',
	"bracketleft": '[', "backslash": '\\', "bracketright": ']',
	"asciicircum": '^', "underscore": '_', "quoteleft": '`',
	"braceleft": '{', "bar": '|', "braceright": '}', "asciitilde": '~',
	"bullet": 0x2022, "endash": 0x2013, "emdash": 0x2014,
	"ellipsis": 0x2026, "trademark": 0x2122, "copyright": 0x00A9,
	"registered": 0x00AE, "degree": 0x00B0, "plusminus": 0x00B1,
	"multiply": 0x00D7, "divide": 0x00F7, "mu": 0x00B5,
}

func glyphToUnicode(name string) (rune, bool) {
	r, ok := glyphToUnicodeMap[name]
	return r, ok
}

// ===== 文本操作符解释器 =====
//
// 文本状态机：BT 进入文本模式 → Tf 设置字体 → Tm/Td 移动 → Tj/TJ 输出 → ET 退出。
// 坐标系：PDF 文本空间（Tm 矩阵的 e, f 分量即 (x, y) 位置）。

type textState struct {
	fontName string
	fontSize float64
	tm       [6]float64 // 文本矩阵 (a, b, c, d, e, f)
	tl       float64    // 文本行距
	tj       float64    // 字符间距（Tc 操作符）
	tw       float64    // 词间距（Tw 操作符）
	inText   bool
	currentX float64
	currentY float64
}

func newTextState() *textState {
	return &textState{
		tm:       [6]float64{1, 0, 0, 1, 0, 0},
		fontSize: 10,
	}
}

// interpretTextOperators 解释 token 流中的文本相关操作符，
// 返回按 y 坐标聚合的视觉行列表。
func interpretTextOperators(toks []token, decoders map[string]*fontDecoder) []textLine {
	st := newTextState()
	var lines []textLine
	var curLine *textLine

	// 行查找：相同 y（容差）的字符串归为同一行
	findOrCreateLine := func(y, fontSize float64) *textLine {
		tol := fontSize * 0.5
		if tol < 4 {
			tol = 4
		}
		for i := range lines {
			if abs(lines[i].y-y) < tol {
				return &lines[i]
			}
		}
		lines = append(lines, textLine{y: y})
		return &lines[len(lines)-1]
	}

	// 栈用于操作数
	var stack []token

	emitString := func(s string) {
		if s == "" || !st.inText {
			return
		}
		x := st.tm[4] // 当前 x 位置
		y := st.tm[5] // 当前 y 位置
		if curLine == nil || abs(curLine.y-y) > st.fontSize*0.5 {
			curLine = findOrCreateLine(y, st.fontSize)
		}
		curLine.frag = append(curLine.frag, textFragment{x: x, s: s})
	}

	for i := 0; i < len(toks); i++ {
		t := toks[i]
		switch t.kind {
		case tokOp:
			switch t.op {
			case "BT":
				st.inText = true
				st.tm = [6]float64{1, 0, 0, 1, 0, 0}
				curLine = nil
				stack = stack[:0]
			case "ET":
				st.inText = false
				curLine = nil
				stack = stack[:0]
			case "Tf":
				// /F1 12 Tf —— 设置字体
				if len(stack) >= 2 {
					var fontName string
					var size float64
					if stack[len(stack)-2].kind == tokName {
						fontName = stack[len(stack)-2].str
					}
					if stack[len(stack)-1].kind == tokNumber {
						size = stack[len(stack)-1].num
					}
					st.fontName = fontName
					if size > 0 {
						st.fontSize = size
					}
				}
				stack = stack[:0]
			case "Tm":
				// a b c d e f Tm —— 设置文本矩阵
				if len(stack) >= 6 {
					st.tm[0] = stack[len(stack)-6].num
					st.tm[1] = stack[len(stack)-5].num
					st.tm[2] = stack[len(stack)-4].num
					st.tm[3] = stack[len(stack)-3].num
					st.tm[4] = stack[len(stack)-2].num
					st.tm[5] = stack[len(stack)-1].num
				}
				stack = stack[:0]
				curLine = nil
			case "Td":
				// tx ty Td —— 相对移动
				if len(stack) >= 2 {
					tx := stack[len(stack)-2].num
					ty := stack[len(stack)-1].num
					st.tm[4] += tx
					st.tm[5] += ty
				}
				stack = stack[:0]
				curLine = nil
			case "TD":
				// tx ty TD —— 相对移动并设置行距
				if len(stack) >= 2 {
					tx := stack[len(stack)-2].num
					ty := stack[len(stack)-1].num
					st.tl = -ty
					st.tm[4] += tx
					st.tm[5] += ty
				}
				stack = stack[:0]
				curLine = nil
			case "T*":
				// 移到下一行（按行距）
				st.tm[5] -= st.tl
				curLine = nil
				stack = stack[:0]
			case "Tc":
				if len(stack) >= 1 {
					st.tj = stack[len(stack)-1].num
				}
				stack = stack[:0]
			case "Tw":
				if len(stack) >= 1 {
					st.tw = stack[len(stack)-1].num
				}
				stack = stack[:0]
			case "TL":
				if len(stack) >= 1 {
					st.tl = stack[len(stack)-1].num
				}
				stack = stack[:0]
			case "Tj":
				// string Tj —— 显示字符串
				if len(stack) >= 1 && stack[len(stack)-1].kind == tokString {
					dec := decoders[st.fontName]
					if dec == nil {
						dec = newFontDecoder()
					}
					emitString(dec.decode([]byte(stack[len(stack)-1].str)))
				}
				stack = stack[:0]
			case "TJ":
				// [ string num string ... ] TJ —— 显示字符串数组
				if len(stack) >= 1 && stack[len(stack)-1].kind == tokBOA {
					// 实际上我们用专门的 TJ 处理路径——见下方
				}
				stack = stack[:0]
			case "'", "\"":
				// ' = T* string Tj ；" = aw ac string T* Tj
				// 简化：移动到下一行并显示字符串
				st.tm[5] -= st.tl
				curLine = nil
				if t.op == "\"" && len(stack) >= 3 {
					if stack[len(stack)-1].kind == tokString {
						dec := decoders[st.fontName]
						if dec == nil {
							dec = newFontDecoder()
						}
						emitString(dec.decode([]byte(stack[len(stack)-1].str)))
					}
				} else if t.op == "'" && len(stack) >= 1 {
					if stack[len(stack)-1].kind == tokString {
						dec := decoders[st.fontName]
						if dec == nil {
							dec = newFontDecoder()
						}
						emitString(dec.decode([]byte(stack[len(stack)-1].str)))
					}
				}
				stack = stack[:0]
			default:
				// 其他操作符清空栈（不关心其参数）
				stack = stack[:0]
			}
		case tokString:
			// 检查是否在 TJ 数组上下文：前一个是 BOA 或字符串/数字
			// 简化：始终把字符串压栈，Tj/TJ 处理时再消费
			stack = append(stack, t)
		case tokBOA:
			// 开始 TJ 数组：立即把栈上现有操作数清空，标记数组开始
			stack = append(stack, t)
			// TJ 数组元素处理：直接消费后续字符串与数字
			j := i + 1
			dec := decoders[st.fontName]
			if dec == nil {
				dec = newFontDecoder()
			}
			var sb strings.Builder
			for j < len(toks) && toks[j].kind != tokEOA {
				if toks[j].kind == tokString {
					sb.WriteString(dec.decode([]byte(toks[j].str)))
				}
				// 数字（kerning）忽略；只有大幅度负值才加空格（避免误把字偶间距当词间隔）
				// 阈值 -500 是经验值：典型 TJ 字偶间距 -50 ~ -200，词间距 -500 ~ -2000
				if toks[j].kind == tokNumber && toks[j].num < -500 {
					sb.WriteString(" ")
				}
				j++
			}
			emitString(sb.String())
			i = j // 跳到 EOA
			stack = stack[:0]
		case tokNumber, tokName:
			stack = append(stack, t)
		case tokEOA:
			stack = stack[:0]
		}
	}

	// 过滤空行
	out := lines[:0]
	for _, l := range lines {
		if len(l.frag) > 0 {
			out = append(out, l)
		}
	}
	return out
}

// renderLines 把视觉行按 y 降序排列、行内按 x 升序排列，拼成最终文本。
func renderLines(lines []textLine) string {
	sort.SliceStable(lines, func(i, j int) bool {
		return lines[i].y > lines[j].y // y 大的在上
	})
	var b strings.Builder
	for i, l := range lines {
		// 行内按 x 升序
		sort.SliceStable(l.frag, func(i, j int) bool {
			return l.frag[i].x < l.frag[j].x
		})
		for j, f := range l.frag {
			if j > 0 {
				b.WriteString(" ")
			}
			b.WriteString(f.s)
		}
		if i < len(lines)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
