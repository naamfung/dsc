package main

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// benchCases 是内置集成运行时测试集。每个用例只把 Task（任务陈述）暴露给模型
// 与用户；期望值 Expected 与判定规则 Matcher/Tol 仅存于本文件与进程内，绝不可经
// 工具描述 / Context / ListContext 发给模型——否则模型可作弊取巧，测试失去意义。

// 判定规则（Matcher）：
//   - exact    去掉尾部换行后的逐字相等
//   - ieq      大小写不敏感相等
//   - num      解析为 float64，与 Expected 的绝对误差不超过 Tol
//   - contains 忽略大小写是否包含 Expected
//   - regex    MatchString 是否命中 Expected（正则）
//
// 结果来源（Kind）：
//   - answer   模型经 bench_submit 提交的文本 answer
//   - file     插件直接读取 <benchRoot>/bench-out/<ID>/reply.txt 的文本
//
// Task 中的占位符在返回给模型前替换：<benchRoot>（绝对）、<caseOut>（该用例产物
// 目录绝对路径）。Relative 给出相对 workspace 的产物路径，便于沙箱内写文件。

type CaseQuery struct {
	ID       string
	Title    string
	Task     string
	Relative string // 相对 workspace 的产物路径（file 类用）
	Kind     string // answer | file
	Matcher  string
	Expected string     // exact/ieq/num/contains/regex 的期望值
	Accept   []string   // anyof：任一（忽略大小写且去首尾空白）相等即命中
	Groups   [][]string // groups：每个内层集合为「任一写法出现即可」的一组；所有组都必须 ≥1 命中
	Tol      float64
	Weight   int
	// NoLeak 用于「格式合规」类用例：其任务本身就是要求的输出格式（期望值即规格，
	// 无可保密的答案），跳过防作弊扫描；其余用例默认受防作弊守卫。
	NoLeak bool
}

var benchCases = []CaseQuery{
	{
		ID:       "arith_power",
		Title:    "整数幂运算",
		Task:     "计算 2 的 10 次方，结果是多少？只回复那个数字，不附加其它文字。",
		Kind:     "answer",
		Matcher:  "num",
		Expected: "1024",
		Tol:      0,
		Weight:   1,
	},
	{
		ID:       "sqrt_2",
		Title:    "开平方精度",
		Task:     "计算 2 的平方根（根号 2），用小数表示，精确到小数点后 4 位即可。只回复那个数字，不附加其它文字。",
		Kind:     "answer",
		Matcher:  "num",
		Expected: "1.4142135623730951",
		Tol:      1e-4,
		Weight:   1,
	},
	{
		ID:      "capital_france",
		Title:   "常识问答（联合国官方语言）",
		Task:    "法国的首都是哪座城市？请用联合国任何一种官方语言作答（阿拉伯语、汉语、英语、法语、俄语、西班牙语），只回复该城市名的正确写法，不附加其它文字。",
		Kind:    "answer",
		Matcher: "anyof",
		Accept:  []string{"巴黎", "Paris", "Париж", "باريس", "París"},
		Weight:  1,
	},
	{
		ID:      "ming_capital",
		Title:   "文言史实问答（明朝国都·洪武开国）",
		Task:    "「大明帝国，洪武皇帝开国之时，国都所在地谓何？」请用联合国任何一种官方语言作答（阿拉伯语、汉语、英语、法语、俄语、西班牙语），只回复该城市的正确写法，不附加其它文字。",
		Kind:    "answer",
		Matcher: "anyof",
		// 把题干限定在「洪武开国建都」，消除迁都歧义：明初国都即应天府（南京）。
		Accept: []string{"南京", "Nanjing", "Nankin", "Нанкин", "نانجينغ", "Nankín"},
		Weight: 1,
	},
	{
		ID:      "ming_capital_judgment",
		Title:   "判断力（明朝国都的歧义）",
		Task:    "「明朝的国都在哪里」这个问题本身存在歧义。请运用你的判断，把它指出来并完整作答：分别指出明朝开国初期与永乐迁都之后的国都各是哪一座城市，各用联合国官方语言写出一处即可（顺序不限，两处的语言可不同）。",
		Kind:    "answer",
		Matcher: "groups",
		// groups：必须同时命中两处国都（明初南京 + 迁都后北京），任一处缺失即判未充分识别歧义。
		Groups: [][]string{
			{"南京", "Nanjing", "Nankin", "Нанкин", "نانجينغ", "Nankín"},
			{"北京", "Beijing", "Pékin", "Пекин", "بكين", "Pekín"},
		},
		Weight: 1,
	},
	{
		ID:       "file_reverse",
		Title:    "文件工具写入（端态校验）",
		Task:     "请用你的文件工具（而非直接回答）创建一个文件，路径为 bench-out/file_reverse/reply.txt（绝对路径：<caseOut>/reply.txt），文件内容为英文句子 `the quick brown fox` 中各个单词的逆序排列（把第一个词放到最后、最后一个词放到最前，各单词间用一个空格分隔，全部小写）。",
		Relative: "bench-out/file_reverse/reply.txt",
		Kind:     "file",
		Matcher:  "ieq",
		Expected: "fox brown quick the",
		Weight:   1,
	},
	{
		ID:       "file_sum",
		Title:    "计算并落盘（运行时集成）",
		Task:     "计算 1 到 100 的和。请用你的文件工具（而非直接回答）把结果这个数字单独写到路径 bench-out/file_sum/reply.txt（绝对路径：<caseOut>/reply.txt），不要附加其它文字或换行。",
		Relative: "bench-out/file_sum/reply.txt",
		Kind:     "file",
		Matcher:  "num",
		Expected: "5050",
		Tol:      0,
		Weight:   1,
	},
	{
		ID:       "odd_sum",
		Title:    "数列求和（奇数）",
		Task:     "1 到 99 之间所有奇数的和是多少？只回复那个数字，不附加其它文字。",
		Kind:     "answer",
		Matcher:  "num",
		Expected: "2500",
		Tol:      0,
		Weight:   1,
	},
	{
		ID:       "fib_10",
		Title:    "递归/递推数列",
		Task:     "在斐波那契数列 1, 1, 2, 3, 5, 8, 13, …（每项为前两项之和）中，第 10 项是多少？只回复那个数字，不附加其它文字。",
		Kind:     "answer",
		Matcher:  "num",
		Expected: "55",
		Tol:      0,
		Weight:   1,
	},
	{
		ID:       "prime_below_20",
		Title:    "数论（质数）",
		Task:     "小于 20 的最大质数是多少？只回复那个数字，不附加其它文字。",
		Kind:     "answer",
		Matcher:  "num",
		Expected: "19",
		Tol:      0,
		Weight:   1,
	},
	{
		ID:       "json_health",
		Title:    "结构化 JSON 输出",
		Task:     "输出一个 JSON 对象 `{\"sum\": <数字>}`，其中 `<数字>` 为 1 到 10 的和。只输出这一行 JSON，不要附加其它文字或解释。",
		Kind:     "answer",
		Matcher:  "regex",
		Expected: `^\s*\{\s*"sum"\s*:\s*55\s*\}\s*$`,
		// 格式合规类：键名即任务规格、值 55 由模型计算得出，期望格式无法也不需保密。
		NoLeak: true,
		Weight: 1,
	},
	{
		ID:       "fraction_sum",
		Title:    "精确分数运算（lisp_eval）",
		Task:     "用 lisp_eval 工具精确计算 1/3 与 1/7 的和（该工具以精确有理数求值），把结果换算成小数表示并提交一个数字（保留到小数点后 4 位即可），只回复那个数字，不附加其它文字。",
		Kind:     "answer",
		Matcher:  "num",
		Expected: "0.4761905",
		Tol:      1e-4,
		// 10/21 为无限循环小数，精确分数正确性才不致被浮点误差掩盖。
		Weight: 1,
	},
	{
		ID:       "file_multi",
		Title:    "多次写盘（追加）",
		Task:     "请用你的文件工具（而非直接回答）创建文件 bench-out/file_multi/reply.txt（绝对路径：<caseOut>/reply.txt），内容是两行，第一行为英文单词 `hello`，再在上面之后另起一行追加英文单词 `world`（先后分两次写入或一次写完皆可，但要确保两行都存在）。",
		Relative: "bench-out/file_multi/reply.txt",
		Kind:     "file",
		Matcher:  "groups",
		Groups:   [][]string{{"hello"}, {"world"}},
		// 格式合规类：两行内容（hello/world）由任务直接指定，测试的是「多行/追加写入」的
		// 工具执行而非保密知识。
		NoLeak: true,
		Weight: 1,
	},
	{
		ID:       "file_wc_lines",
		Title:    "命令统计并落盘（wc）",
		Task:     "请用你的文件工具（而非直接回答）：先用 shell 写入三行内容（内容任意）到 bench-out/file_wc_lines/source.txt（绝对路径：<caseOut>/source.txt），再用 shell 的 wc 内建命令统计该文件共有几行，把行数这个数字写到 bench-out/file_wc_lines/reply.txt（绝对路径：<caseOut>/reply.txt），不要附加其它文字。",
		Relative: "bench-out/file_wc_lines/reply.txt",
		Kind:     "file",
		Matcher:  "num",
		Expected: "3",
		Tol:      0,
		Weight:   1,
	},
}

// matchCaseText 对给定的候选文本按用例规则判定，返回是否命中与失败原因（不回显期望值）。
func matchCaseText(c CaseQuery, got string) (bool, string) {
	got = strings.TrimSpace(got)
	switch c.Matcher {
	case "exact":
		if strings.TrimRight(got, "\r\n") == c.Expected {
			return true, ""
		}
		return false, "内容不完全一致"
	case "ieq":
		if strings.EqualFold(got, strings.TrimSpace(c.Expected)) {
			return true, ""
		}
		return false, "内容不一致"
	case "num":
		g, err := strconv.ParseFloat(got, 64)
		if err != nil {
			return false, "无法解析为数字"
		}
		e, _ := strconv.ParseFloat(c.Expected, 64)
		if math.Abs(g-e) <= c.Tol {
			return true, ""
		}
		return false, fmt.Sprintf("数值不在容差范围内（偏差 %.6g）", math.Abs(g-e))
	case "contains":
		if strings.Contains(strings.ToLower(got), strings.ToLower(c.Expected)) {
			return true, ""
		}
		return false, "未包含预期关键词"
	case "anyof":
		for _, a := range c.Accept {
			if strings.EqualFold(strings.TrimSpace(got), strings.TrimSpace(a)) {
				return true, ""
			}
		}
		return false, "不在可接受答案集合内"
	case "groups":
		// groups：每组「任一写法出现即可」，且所有组都必须至少命中一处。
		// 用于测「是否识别歧义并完整枚举多个正解」的判断力，不受单一语言约束。
		gg := strings.ToLower(got)
		for _, group := range c.Groups {
			found := false
			for _, form := range group {
				if strings.Contains(gg, strings.ToLower(form)) {
					found = true
					break
				}
			}
			if !found {
				return false, "答案未覆盖全部应指出的项（漏识别了歧义的某一部分）"
			}
		}
		return true, ""
	case "regex":
		re, err := regexp.Compile(c.Expected)
		if err != nil {
			return false, "用例正则非法（内部错误）"
		}
		if re.MatchString(got) {
			return true, ""
		}
		return false, "不匹配要求的格式"
	default:
		return false, "未识别的判定规则（内部错误）"
	}
}
