// Package main — tool-pdf 插件的 PDF 内容流 tokenizer。
//
// PDF 内容流是一系列操作符（如 BT, ET, Tj, TJ, Tf, Tm, Td）与操作数
// （字符串字面量 `(Hello)`、十六进制字符串 `<48656C6C6F>`、数字、名称、数组等）
// 的混合。本 tokenizer 仅关心与文本相关的操作符，识别 BT/ET 块、Tf（字体切换）、
// Tj（显示字符串）、TJ（显示字符串数组，含字偶间距数值）、' 与 "（换行显示）。
//
// 字符串字面量解析支持 PDF 转义序列（\n, \r, \t, \b, \f, \(, \), \\, \ddd 八进制）。
// 十六进制字符串解析跳过空白与不完整字节的高位补零。
//
// 设计原则：tokenizer 仅做语法分析（字节流 → token 流），不做语义解释。
// 字符串到 Unicode 的解码由 font 编码层负责。
package main

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// tokenKind 区分 token 类型。
type tokenKind int

const (
	tokOp     tokenKind = iota // 操作符（如 "Tj", "BT", "Tf"）
	tokString                  // 字符串字面量（PDF 字面或十六进制）
	tokNumber                  // 数字
	tokName                    // 名称 /Foo
	tokArray                   // 数组 [ ... ]（仅记录边界，元素作为独立 token 流出）
	tokBOA                     // Begin Of Array 标记 "[" —— TJ 的参数边界
	tokEOA                     // End Of Array 标记 "]"
)

// token 表示一个内容流 token。
type token struct {
	kind  tokenKind
	str   string  // 字符串值（tokString / tokName）
	num   float64 // 数字值（tokNumber）
	op    string  // 操作符名（tokOp）
	isHex bool    // tokString 是否为十六进制字符串
}

// tokenizeContentStream 把 PDF 内容流字节切片解析为 token 列表。
// 仅识别文本相关操作符；其他操作符及其操作数也作为 token 流出（供调用方按需忽略）。
// 解析错误（如未闭合字符串、非法转义）转为 token 中的占位错误字符串，不中断整体扫描。
func tokenizeContentStream(bb []byte) []token {
	t := &tokenizer{src: bb}
	return t.run()
}

type tokenizer struct {
	src []byte
	pos int
	out []token
}

func (t *tokenizer) run() []token {
	for t.pos < len(t.src) {
		c := t.src[t.pos]
		switch {
		case c == '(':
			t.readStringLiteral()
		case c == '<':
			if t.peek() == '<' {
				// "<< dict begin" / ">> dict end" —— 跳过字典结构（不关心）
				t.skipDict()
			} else {
				t.readHexString()
			}
		case c == '[':
			t.out = append(t.out, token{kind: tokBOA})
			t.pos++
		case c == ']':
			t.out = append(t.out, token{kind: tokEOA})
			t.pos++
		case c == '/':
			t.readName()
		case isNumberStart(c, t.peek()):
			t.readNumber()
		case isOpChar(c):
			t.readOp()
		case c == '%' || c == 0xEF:
			// 注释（%）或 BOM（0xEF 0xBB 0xBF）—— 跳过到行尾或文件尾
			t.skipCommentOrBOM()
		default:
			// 空白 / 其他分隔符：直接跳过
			t.pos++
		}
	}
	return t.out
}

func (t *tokenizer) peek() byte {
	if t.pos+1 >= len(t.src) {
		return 0
	}
	return t.src[t.pos+1]
}

// readStringLiteral 解析 ( ... ) 字符串字面量。
// 支持嵌套括号（成对匹配）与转义序列：\n \r \t \b \f \( \) \\ \ddd（八进制 1-3 位）。
// 非法转义按字面字符处理（PDF 规范行为）。
func (t *tokenizer) readStringLiteral() {
	t.pos++ // skip '('
	var b strings.Builder
	depth := 1
	for t.pos < len(t.src) {
		c := t.src[t.pos]
		if c == '\\' {
			t.pos++
			if t.pos >= len(t.src) {
				break
			}
			esc := t.src[t.pos]
			switch esc {
			case 'n':
				b.WriteByte('\n')
				t.pos++
			case 'r':
				b.WriteByte('\r')
				t.pos++
			case 't':
				b.WriteByte('\t')
				t.pos++
			case 'b':
				b.WriteByte('\b')
				t.pos++
			case 'f':
				b.WriteByte('\f')
				t.pos++
			case '(':
				b.WriteByte('(')
				t.pos++
			case ')':
				b.WriteByte(')')
				t.pos++
			case '\\':
				b.WriteByte('\\')
				t.pos++
			case '\n':
				// 行延续符：跳过
				t.pos++
			case '\r':
				t.pos++
				if t.pos < len(t.src) && t.src[t.pos] == '\n' {
					t.pos++
				}
			default:
				// 八进制转义 \ddd（1-3 位八进制）
				if esc >= '0' && esc <= '7' {
					oct := string(esc)
					t.pos++
					for i := 0; i < 2 && t.pos < len(t.src); i++ {
						if t.src[t.pos] >= '0' && t.src[t.pos] <= '7' {
							oct += string(t.src[t.pos])
							t.pos++
						} else {
							break
						}
					}
					if n, err := strconv.ParseUint(oct, 8, 16); err == nil {
						b.WriteByte(byte(n))
					}
				} else {
					// 非法转义：忽略反斜杠，保留后续字符（PDF 规范）
					b.WriteByte(esc)
					t.pos++
				}
			}
			continue
		}
		if c == '(' {
			depth++
			b.WriteByte('(')
			t.pos++
			continue
		}
		if c == ')' {
			depth--
			t.pos++
			if depth == 0 {
				t.out = append(t.out, token{kind: tokString, str: b.String(), isHex: false})
				return
			}
			b.WriteByte(')')
			continue
		}
		b.WriteByte(c)
		t.pos++
	}
	// 流提前结束（未闭合）：仍把已读内容作为字符串 token
	t.out = append(t.out, token{kind: tokString, str: b.String(), isHex: false})
}

// readHexString 解析 < ... > 十六进制字符串。
// 跳过空白；奇数位末尾补零。非法字符忽略。
func (t *tokenizer) readHexString() {
	t.pos++ // skip '<'
	var hexStr strings.Builder
	for t.pos < len(t.src) {
		c := t.src[t.pos]
		if c == '>' {
			t.pos++
			break
		}
		if isHexDigit(c) {
			hexStr.WriteByte(c)
		}
		// 空白与非法字符跳过
		t.pos++
	}
	// 奇数位补零
	h := hexStr.String()
	if len(h)%2 == 1 {
		h += "0"
	}
	var b strings.Builder
	for i := 0; i+1 < len(h); i += 2 {
		if n, err := strconv.ParseUint(h[i:i+2], 16, 8); err == nil {
			b.WriteByte(byte(n))
		}
	}
	t.out = append(t.out, token{kind: tokString, str: b.String(), isHex: true})
}

// readName 解析 /Name。
func (t *tokenizer) readName() {
	t.pos++ // skip '/'
	var b strings.Builder
	for t.pos < len(t.src) {
		c := t.src[t.pos]
		if isOpChar(c) && c != '#' {
			b.WriteByte(c)
			t.pos++
			continue
		}
		if c == '#' && t.pos+2 < len(t.src) {
			// #xx 十六进制转义
			if h, err := strconv.ParseUint(string(t.src[t.pos+1:t.pos+3]), 16, 8); err == nil {
				b.WriteByte(byte(h))
				t.pos += 3
				continue
			}
		}
		break
	}
	t.out = append(t.out, token{kind: tokName, str: b.String()})
}

// readNumber 解析数字（含负号与小数点）。
func (t *tokenizer) readNumber() {
	start := t.pos
	if t.src[t.pos] == '-' || t.src[t.pos] == '+' {
		t.pos++
	}
	for t.pos < len(t.src) && (isDigit(t.src[t.pos]) || t.src[t.pos] == '.') {
		t.pos++
	}
	numStr := string(t.src[start:t.pos])
	n, _ := strconv.ParseFloat(numStr, 64)
	t.out = append(t.out, token{kind: tokNumber, num: n, str: numStr})
}

// readOp 读取操作符 / 关键字。
// 操作符由可打印非分隔符字符组成，以空白或分隔符（()<>[]{}/%）结束。
func (t *tokenizer) readOp() {
	start := t.pos
	for t.pos < len(t.src) {
		c := t.src[t.pos]
		if isOpChar(c) {
			t.pos++
			continue
		}
		break
	}
	op := string(t.src[start:t.pos])
	t.out = append(t.out, token{kind: tokOp, op: op})
}

// skipDict 跳过 << ... >> 字典结构（内容流中偶尔出现，对文本提取无意义）。
func (t *tokenizer) skipDict() {
	t.pos += 2 // skip <<
	depth := 1
	for t.pos < len(t.src) && depth > 0 {
		c := t.src[t.pos]
		if c == '<' && t.pos+1 < len(t.src) && t.src[t.pos+1] == '<' {
			depth++
			t.pos += 2
			continue
		}
		if c == '>' && t.pos+1 < len(t.src) && t.src[t.pos+1] == '>' {
			depth--
			t.pos += 2
			continue
		}
		if c == '(' {
			// 跳过字符串字面量（避免字典内字符串中含 << >> 误判）
			t.readStringLiteral()
			continue
		}
		if c == '<' {
			t.readHexString()
			continue
		}
		t.pos++
	}
}

func (t *tokenizer) skipCommentOrBOM() {
	for t.pos < len(t.src) {
		c := t.src[t.pos]
		if c == '\n' || c == '\r' {
			break
		}
		t.pos++
	}
}

func isNumberStart(c byte, next byte) bool {
	if isDigit(c) {
		return true
	}
	if (c == '-' || c == '+' || c == '.') && isDigit(next) {
		return true
	}
	return false
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isHexDigit(c byte) bool {
	return isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// isOpChar 报告字符可作为操作符 / 关键字的一部分（非分隔符）。
// PDF 内容流分隔符为：( ) < > [ ] { } / %
// 嵌入 PostScript 计算中可能出现的 { } 也作为分隔符。
func isOpChar(c byte) bool {
	if c <= 0x20 { // 空白字符
		return false
	}
	switch c {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return false
	}
	return true
}

// _ = unicode.IsSpace 保留 unicode import 供未来字符分类扩展
var _ = unicode.IsSpace

// formatTokens 仅供调试：把 token 列表转为可读字符串。
func formatTokens(toks []token) string {
	var b strings.Builder
	for _, t := range toks {
		switch t.kind {
		case tokOp:
			fmt.Fprintf(&b, "[%s] ", t.op)
		case tokString:
			fmt.Fprintf(&b, "(%q) ", t.str)
		case tokNumber:
			fmt.Fprintf(&b, "%g ", t.num)
		case tokName:
			fmt.Fprintf(&b, "/%s ", t.str)
		case tokBOA:
			b.WriteString("[ ")
		case tokEOA:
			b.WriteString("] ")
		}
	}
	return b.String()
}
