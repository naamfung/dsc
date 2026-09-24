package dsp

import (
	"fmt"
	"strings"
)

// GuardSQL 对插件脚本经 dsc.sql.* 提交的语句做保守静态检查，维护 .dsp 的两条不变量：
//
//  1. 单文件自持：禁止 ATTACH / DETACH —— 它们会让插件在自身库之外挂载、读写别的文件，
//     破坏「一个 .dsp 即插件全部」的边界。
//  2. 代码与元数据只读：禁止运行期写保留表 dsp_meta / dsp_blobs —— 加载器读它们取语义
//     与代码，运行期改动会让「当前跑的是哪个版本」失控（自改代码后重载更是隐性升级）。
//     同时禁止全部 DDL —— 插件结构属打包期产物，运行期建/删表（含建触发器绕道改保留表）
//     没有正当需求。
//
// 这是保守的静态检查而非安全边界：插件脚本本就在宿主进程内执行，守卫的目的是守住
// 格式不变量与可审计性，不是沙箱。故宁可对可疑语句一律拒绝（假阳性优于静默放行）。
func GuardSQL(stmt string) error {
	body := strings.TrimSpace(stmt)
	if body == "" {
		return fmt.Errorf("SQL 为空")
	}
	stmts := SplitStatements(body)
	if len(stmts) == 0 {
		return fmt.Errorf("SQL 为空")
	}
	if len(stmts) > 1 {
		return fmt.Errorf("只允许单条 SQL 语句（收到 %d 条）", len(stmts))
	}
	stripped := stripLiterals(stmts[0])
	toks := tokens(stripped)
	if len(toks) == 0 {
		return fmt.Errorf("SQL 无可执行内容")
	}
	kw := strings.ToUpper(toks[0])

	switch kw {
	case "ATTACH", "DETACH":
		return fmt.Errorf("禁止 %s：.dsp 插件只能读写自身库（单文件自持）", kw)
	case "CREATE", "DROP", "ALTER", "VACUUM", "REINDEX", "ANALYZE", "TRUNCATE":
		return fmt.Errorf("禁止 %s：插件库结构属打包期产物，运行期不得变更", kw)
	case "PRAGMA":
		// 只读 PRAGMA（查询类）可用；带 = 的设置会改库状态，一律拒绝。
		if strings.Contains(stripped, "=") {
			return fmt.Errorf("禁止写入型 PRAGMA：插件不得在运行期变更库设置")
		}
		return nil
	case "SELECT", "VALUES", "WITH", "INSERT", "UPDATE", "DELETE", "REPLACE":
	default:
		return fmt.Errorf("不支持的 SQL 起始关键字 %q（仅允许 SELECT/VALUES/WITH/INSERT/UPDATE/DELETE/REPLACE/只读 PRAGMA）", kw)
	}

	upper := make([]string, len(toks))
	for i, t := range toks {
		upper[i] = strings.ToUpper(t)
	}
	if hasAnyWord(upper, "ATTACH", "DETACH") {
		return fmt.Errorf("禁止 ATTACH/DETACH：.dsp 插件只能读写自身库（单文件自持）")
	}
	if isWriteStatement(kw, upper) && hasAnyWord(upper, upperWord(TableMeta), upperWord(TableBlobs)) {
		return fmt.Errorf("禁止运行期写保留表 %s/%s：元数据与代码由打包期固化", TableMeta, TableBlobs)
	}
	return nil
}

// upperWord 把标识符统一为大写（与 upper 词元表同口径比较）。
func upperWord(s string) string { return strings.ToUpper(s) }

// writeWords 是判定「语句是否可能写入」的关键字。
var writeWords = []string{"INSERT", "UPDATE", "DELETE", "REPLACE"}

// isWriteStatement 判断语句是否可能写库：以写关键字起手，或 WITH 体中出现写关键字。
func isWriteStatement(kw string, upperToks []string) bool {
	switch kw {
	case "INSERT", "UPDATE", "DELETE", "REPLACE":
		return true
	case "WITH":
		return hasAnyWord(upperToks, writeWords...)
	default:
		return false
	}
}

func hasAnyWord(upperToks []string, words ...string) bool {
	for _, t := range upperToks {
		for _, w := range words {
			if t == w {
				return true
			}
		}
	}
	return false
}

// SplitStatements 按顶层分号切分 SQL 脚本段（字符串字面量与注释内的分号不参与切分），
// 并丢弃空段与注释。打包期执行作者 schema.sql 时按段逐条执行——database/sql 的 Exec
// 一次只跑一条语句，多段 SQL 必须切开才不会静默丢弃后半截。
//
// 已知边界：不做 BEGIN…END 嵌套识别，故触发器体（CREATE TRIGGER … BEGIN … ; … END）
// 会被切成多段并因语句不完整而在打包期报错（显式失败，不会静默产出坏结构）。
func SplitStatements(sql string) []string {
	var out []string
	var cur strings.Builder
	i := 0
	for i < len(sql) {
		switch c := sql[i]; {
		case c == '\'' || c == '"' || c == '`':
			i = copyQuoted(sql, i, c, &cur)
		case c == '[':
			i = copyBracket(sql, i, &cur)
		case c == '-' && i+1 < len(sql) && sql[i+1] == '-':
			for i < len(sql) && sql[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < len(sql) && sql[i+1] == '*':
			i += 2
			for i+1 < len(sql) && !(sql[i] == '*' && sql[i+1] == '/') {
				i++
			}
			i = minInt(i+2, len(sql))
		case c == ';':
			if s := strings.TrimSpace(cur.String()); s != "" {
				out = append(out, s)
			}
			cur.Reset()
			i++
		default:
			cur.WriteByte(c)
			i++
		}
	}
	if s := strings.TrimSpace(cur.String()); s != "" {
		out = append(out, s)
	}
	return out
}

// copyQuoted 把引号字面量原样拷入 cur，返回下一位置（处理 SQL 的双写引号转义）。
func copyQuoted(sql string, i int, q byte, cur *strings.Builder) int {
	start := i
	i++
	for i < len(sql) {
		if sql[i] == q {
			if i+1 < len(sql) && sql[i+1] == q {
				i += 2
				continue
			}
			i++
			break
		}
		i++
	}
	cur.WriteString(sql[start:i])
	return i
}

// copyBracket 把 [标识符] 原样拷入 cur，返回下一位置。
func copyBracket(sql string, i int, cur *strings.Builder) int {
	start := i
	for i < len(sql) && sql[i] != ']' {
		i++
	}
	if i < len(sql) {
		i++
	}
	cur.WriteString(sql[start:i])
	return i
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// stripLiterals 把字符串/标识符字面量与注释替换为等长空格，使后续分词不会被
// 字面量内容干扰（如 WHERE note = 'attach; DROP' 不会被误判为多语句或多词）。
func stripLiterals(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == '\'' || c == '"' || c == '`':
			i = skipQuoted(s, i, c, &b)
		case c == '[':
			i = skipBracket(s, i, &b)
		case c == '-' && i+1 < len(s) && s[i+1] == '-':
			i = skipLineComment(s, i, &b)
		case c == '/' && i+1 < len(s) && s[i+1] == '*':
			i = skipBlockComment(s, i, &b)
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String()
}

// skipQuoted 跳过以 q 起始的引号字面量（SQL 中以双写引号转义），返回下一位置。
func skipQuoted(s string, i int, q byte, b *strings.Builder) int {
	i++ // 起始引号
	for i < len(s) {
		if s[i] == q {
			if i+1 < len(s) && s[i+1] == q { // '' 转义
				b.WriteString("  ")
				i += 2
				continue
			}
			i++
			break
		}
		b.WriteByte(' ')
		i++
	}
	b.WriteByte(' ')
	return i
}

// skipBracket 跳过 [标识符] 形式（用于屏蔽含特殊字符的表名）。
func skipBracket(s string, i int, b *strings.Builder) int {
	for i < len(s) {
		if s[i] == ']' {
			i++
			break
		}
		b.WriteByte(' ')
		i++
	}
	b.WriteByte(' ')
	return i
}

// skipLineComment 跳过 -- 行注释。
func skipLineComment(s string, i int, b *strings.Builder) int {
	for i < len(s) && s[i] != '\n' {
		b.WriteByte(' ')
		i++
	}
	return i
}

// skipBlockComment 跳过 /* */ 块注释。
func skipBlockComment(s string, i int, b *strings.Builder) int {
	b.WriteString("  ")
	i += 2
	for i < len(s) {
		if s[i] == '*' && i+1 < len(s) && s[i+1] == '/' {
			b.WriteString("  ")
			i += 2
			break
		}
		b.WriteByte(' ')
		i++
	}
	return i
}

// tokens 抽取标识符/关键字词元（字母数字下划线），忽略运算符与空白。
func tokens(s string) []string {
	var out []string
	var cur strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if isWordByte(c) {
			cur.WriteByte(c)
			continue
		}
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

func isWordByte(c byte) bool {
	return c == '_' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
