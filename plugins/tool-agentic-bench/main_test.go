package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMatchCaseTextRules(t *testing.T) {
	tests := []struct {
		name     string
		rule     CaseQuery
		got      string
		wantPass bool
	}{
		{"num 精确", CaseQuery{Matcher: "num", Expected: "1024", Tol: 0}, "1024", true},
		{"num 容差内", CaseQuery{Matcher: "num", Expected: "1.41421356", Tol: 1e-4}, "1.4142", true},
		{"num 越界", CaseQuery{Matcher: "num", Expected: "1024", Tol: 0}, "1025", false},
		{"num 非法", CaseQuery{Matcher: "num", Expected: "1024", Tol: 0}, "abc", false},
		{"ieq 忽略大小写", CaseQuery{Matcher: "ieq", Expected: "fox brown quick the"}, "  FOX Brown Quick The\n", true},
		{"exact 逐字", CaseQuery{Matcher: "exact", Expected: "x"}, "x", true},
		{"exact 不同", CaseQuery{Matcher: "exact", Expected: "x"}, "y", false},
		{"contains 忽略大小写", CaseQuery{Matcher: "contains", Expected: "paris"}, "它是 Paris。", true},
		{"regex 命中", CaseQuery{Matcher: "regex", Expected: `^[0-9]+$`}, "5050", true},
		{"regex 未命中", CaseQuery{Matcher: "regex", Expected: `^[0-9]+$`}, "50x", false},
		{"anyof 命中其一", CaseQuery{Matcher: "anyof", Accept: []string{"巴黎", "Paris", "Париж", "باريس", "París"}}, "  Париж ", true},
		{"anyof 忽略大小写+重音", CaseQuery{Matcher: "anyof", Accept: []string{"Paris", "París"}}, "paris", true},
		{"anyof 集合外", CaseQuery{Matcher: "anyof", Accept: []string{"巴黎", "Paris"}}, "柏林", false},
	}
	for _, tc := range tests {
		got, _ := matchCaseText(tc.rule, tc.got)
		if got != tc.wantPass {
			t.Errorf("%s: matchCaseText rule=%s got=%s want pass=%v", tc.name, tc.rule.Matcher, tc.got, tc.wantPass)
		}
	}
	if _, fb := matchCaseText(CaseQuery{Matcher: "exact", Expected: "expect"}, "got"); strings.TrimSpace(fb) == "" {
		t.Error("fail 时应返回非空反馈")
	}
}

// TestCapitalFranceAcceptsAnyUnOfficialLanguage capital_france 的 anyof 应接受联合国
// 全部五种城市的官方语言写法（其接受集合贡献「识别联合国官方语言并正确书写」的考察）。
func TestCapitalFranceAcceptsAnyUnOfficialLanguage(t *testing.T) {
	var c CaseQuery
	for i := range benchCases {
		if benchCases[i].ID == "capital_france" {
			c = benchCases[i]
			break
		}
	}
	if c.Matcher != "anyof" {
		t.Fatalf("capital_france 应用 anyof 匹配，got %q", c.Matcher)
	}
	for _, cand := range c.Accept {
		if ok, _ := matchCaseText(c, cand); !ok {
			t.Errorf("官方语言写法应命中: %q", cand)
		}
	}
	for _, cand := range []string{"柏林", "北京市", ""} {
		if ok, _ := matchCaseText(c, cand); ok {
			t.Errorf("非该城市写法不应命中: %q", cand)
		}
	}
}

// TestMingCapitalAcceptsAnyUnOfficialLanguage ming_capital 的 anyof 应接受北京/南京的
// 联合国官方语言各写法（含史实二义性的两个正解），并拒绝非都城写法。
func TestMingCapitalAcceptsAnyUnOfficialLanguage(t *testing.T) {
	var c CaseQuery
	for i := range benchCases {
		if benchCases[i].ID == "ming_capital" {
			c = benchCases[i]
			break
		}
	}
	if c.Matcher != "anyof" {
		t.Fatalf("ming_capital 应用 anyof 匹配，got %q", c.Matcher)
	}
	for _, cand := range c.Accept {
		if ok, _ := matchCaseText(c, cand); !ok {
			t.Errorf("官方语言都城写法应命中: %q", cand)
		}
	}
	for _, cand := range []string{"北京预测", "开封", ""} {
		if ok, _ := matchCaseText(c, cand); ok {
			t.Errorf("非都城写法不应命中: %q", cand)
		}
	}
}

// TestAutoAdvanceSkipsScored bench_submit 交卷后应自动推进：已评分用例被跳过，
// submit 响应含 next 字段直接给出下一题，模型无须也无法反复重试。
func TestAutoAdvanceSkipsScored(t *testing.T) {
	state.mu.Lock()
	state.root = t.TempDir()
	state.results = map[string]CaseResult{}
	state.caseStart = map[string]time.Time{}
	state.startedAt = time.Now()
	state.mu.Unlock()

	state.mu.Lock()
	next1, done1 := serveNext(state.root)
	state.mu.Unlock()
	if done1 || next1["case_id"] != "arith_power" {
		t.Fatalf("首个应为 arith_power，got %v done=%v", next1, done1)
	}

	// 模拟交卷第一个（正确），自动推进应跳到第二个未评分用例。
	state.mu.Lock()
	recordAndCheck(benchCases[0], "1024", time.Now())
	next2, done2 := serveNext(state.root)
	state.mu.Unlock()
	if done2 || next2["case_id"] != "sqrt_2" {
		t.Fatalf("评分后应推进到 sqrt_2，got %v done=%v", next2, done2)
	}

	// submit 响应应含 next（自动推进），且不会把已评分的 arith_power 再抛出。
	state.mu.Lock()
	pl := submitPayload("sqrt_2", "pass", "PASS", false, state.root)
	state.mu.Unlock()
	if !strings.Contains(pl, `"next"`) || strings.Contains(pl, `"arith_power"`) {
		t.Fatalf("submit 响应应含 next 且不含已评分用例，got %q", pl)
	}
	if strings.Count(pl, `"case_id"`) < 1 {
		t.Fatalf("submit 响应应至少含本用例 case_id，got %q", pl)
	}
}

// TestMingCapitalUnambiguousFounding ming_capital 已完成消歧（洪武开国 → 南京），
// 应接受南京的各官方语言写法、拒绝北京。
func TestMingCapitalUnambiguousFounding(t *testing.T) {
	var c CaseQuery
	for i := range benchCases {
		if benchCases[i].ID == "ming_capital" {
			c = benchCases[i]
			break
		}
	}
	if c.Matcher != "anyof" {
		t.Fatalf("ming_capital 应用 anyof 匹配，got %q", c.Matcher)
	}
	for _, cand := range c.Accept {
		if ok, _ := matchCaseText(c, cand); !ok {
			t.Errorf("洪武开国 = 南京，官方语言写法应命中: %q", cand)
		}
	}
	// 洪武开国消歧后，仅北京不应命中（迁都后非此题语境）。
	for _, cand := range []string{"北京", "Beijing", "开封", ""} {
		if ok, _ := matchCaseText(c, cand); ok {
			t.Errorf("消歧后北京不是本题答案，不应命中: %q", cand)
		}
	}
}

// TestMingCapitalJudgmentStrongerGroups ming_capital_judgment 应同时识别两处国都：
// 同时出现南京与北京（各任一官方语言写法）判通过；只给一处则判失败。
func TestMingCapitalJudgmentGroups(t *testing.T) {
	var c CaseQuery
	for i := range benchCases {
		if benchCases[i].ID == "ming_capital_judgment" {
			c = benchCases[i]
			break
		}
	}
	if c.Matcher != "groups" {
		t.Fatalf("ming_capital_judgment 应用 groups 匹配，got %q", c.Matcher)
	}
	passCases := []string{
		"南京; Beijing",
		"Beijing 和 Nankín。",
		"明初都 Nanjing，永乐迁都后都北京。",
	}
	// 前 3 条同时覆盖两处 → 通过
	for _, a := range passCases {
		if ok, _ := matchCaseText(c, a); !ok {
			t.Errorf("同时指出两处国都应命中: %q", a)
		}
	}
	// 只给一处（无论官方语言与否）→ 失败
	for _, a := range []string{"Пекин", "南京", "Beijing。", ""} {
		if ok, _ := matchCaseText(c, a); ok {
			t.Errorf("只指出一处国都（未识别完整歧义）不应命中: %q", a)
		}
	}
}

func TestFindNextCaseOrderAndAllDone(t *testing.T) {
	state.mu.Lock()
	state.root = t.TempDir()
	state.results = map[string]CaseResult{}
	state.mu.Unlock()

	idx := findNextCase(state.results)
	if idx != 0 {
		t.Fatalf("空结果应从头开始，got %d", idx)
	}
	// 标记第一个为 pass，下一个应是第二个（file_reverse, 下标 3 之前的下标 1 尚未考虑——用真实 list 判序）。
	state.mu.Lock()
	state.results["arith_power"] = CaseResult{Status: "pass"}
	state.mu.Unlock()
	idx = findNextCase(state.results)
	if idx != 1 {
		t.Fatalf("应该推进到下一未评分用例，got %d", idx)
	}
	// 全部标记后应 done。
	state.mu.Lock()
	for _, c := range benchCases {
		state.results[c.ID] = CaseResult{Status: "pass"}
	}
	state.mu.Unlock()
	if idx := findNextCase(state.results); idx >= 0 {
		t.Fatalf("全部完成后 findNextCase 应为 -1，got %d", idx)
	}
}

// 防作弊约束：非格式合规（NoLeak=false）用例的任务/标题文本不得包含其期望值
// （Expected / Accept / Groups 任一）。
func TestNoAnswerLeakInTask(t *testing.T) {
	for _, c := range benchCases {
		if c.NoLeak {
			continue // 格式合规类：期望格式即任务规格，无保密值，跳过
		}
		out := renderTask(c, "/tmp/b")
		var probes []string
		if c.Expected != "" {
			probes = append(probes, c.Expected)
		}
		probes = append(probes, c.Accept...)
		for _, g := range c.Groups {
			probes = append(probes, g...)
		}
		for _, probe := range probes {
			if probe == "" {
				continue
			}
			lowerExp := strings.ToLower(probe)
			lowerTask := strings.ToLower(out)
			if strings.Contains(lowerTask, lowerExp) {
				t.Errorf("case %s: 任务陈述泄漏期望值 %q", c.ID, probe)
			}
		}
	}
}

// 报告字段保真：完整负载往返后逐字段存活（数值、状态、反馈、产量内容均保留）。
func TestBuildReportRoundTripPreservesFields(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, benchOutDirName, "file_sum"), 0o755)
	if err := os.WriteFile(filepath.Join(root, benchOutDirName, "file_sum", "reply.txt"), []byte("5050"), 0o644); err != nil {
		t.Fatal(err)
	}
	state.mu.Lock()
	state.root = root
	state.results = map[string]CaseResult{
		"arith_power":           {Status: "pass"},
		"sqrt_2":                {Status: "fail", Feedback: "数值不在容差范围内"},
		"capital_france":        {Status: "pass"},
		"ming_capital":          {Status: "pass"},
		"ming_capital_judgment": {Status: "pass"},
		"file_reverse":          {Status: "skip"},
		"file_sum":              {Status: "pass"},
		"odd_sum":               {Status: "pass"},
		"fib_10":                {Status: "pass"},
		"prime_below_20":        {Status: "pass"},
		"json_health":           {Status: "pass"},
		"file_multi":            {Status: "pass"},
		"file_wc_lines":         {Status: "pass"},
		"fraction_sum":          {Status: "pass"},
	}
	state.mu.Unlock()

	summary, reportJSON, view := buildReport(root, state.results, 12345)
	if !strings.Contains(summary, "通过 12/14") {
		t.Errorf("摘要应含通过计数，got %q", summary)
	}
	if view == nil {
		t.Fatal("表格视图 spec 不应为空")
	}

	parsed := struct {
		Harness    string           `json:"harness"`
		Passed     int              `json:"passed"`
		Failed     int              `json:"failed"`
		Total      int              `json:"total"`
		Score      string           `json:"score"`
		Ratio      string           `json:"ratio"`
		DurationMs int64            `json:"duration_ms"`
		Cases      []map[string]any `json:"cases"`
	}{}
	if err := json.Unmarshal([]byte(reportJSON), &parsed); err != nil {
		t.Fatalf("报告 JSON 解析失败: %v", err)
	}
	if parsed.Passed != 12 || parsed.Total != 14 || parsed.Failed != 1 {
		t.Errorf("计分不对: passed=%d failed=%d total=%d", parsed.Passed, parsed.Failed, parsed.Total)
	}
	// 得分=成功数/总案例×100 两位小数；成败比=成功:失败（无 percent 冗余字段）。
	if parsed.Score != "85.71" {
		t.Errorf("总得分应为 85.71，got %q", parsed.Score)
	}
	if parsed.Ratio != "12:1" {
		t.Errorf("成败比应为 12:1，got %q", parsed.Ratio)
	}
	if parsed.DurationMs != 12345 {
		t.Errorf("总耗时未保真: %d", parsed.DurationMs)
	}
	// 不应再输出 percent 冗余字段（得分即那个数值，无需重复）。
	var top map[string]any
	_ = json.Unmarshal([]byte(reportJSON), &top)
	if _, ok := top["percent"]; ok {
		t.Error("不应再输出 percent 字段")
	}
	for _, rec := range parsed.Cases {
		if _, ok := rec["status"]; !ok {
			t.Errorf("case %v 缺 status 字段", rec["id"])
		}
		if _, ok := rec["duration_ms"]; !ok {
			t.Errorf("case %v 缺 duration_ms 字段", rec["id"])
		}
	}
	// 字段保真：file_sum 的产量内容应原样保留。
	for _, rec := range parsed.Cases {
		if rec["id"] == "file_sum" && rec["artifact_content"] != "5050" {
			t.Errorf("artifact_content 未字段保真: %v", rec["artifact_content"])
		}
	}
	// 落盘校验。
	data, err := os.ReadFile(filepath.Join(root, benchOutDirName, "report.json"))
	if err != nil {
		t.Fatalf("report.json 未落盘: %v", err)
	}
	var onDisk map[string]any
	if err := json.Unmarshal(data, &onDisk); err != nil || onDisk["passed"] != float64(12) {
		t.Fatalf("report.json 内容异常: %v err=%v", string(data), err)
	}
}

// TestFinalizeMarksPendingSkip 交卷预算终局：未评分用例标 skip 且不再被下发，报告可即时落盘。
func TestFinalizeMarksPendingSkip(t *testing.T) {
	state.mu.Lock()
	state.root = t.TempDir()
	state.results = map[string]CaseResult{}
	state.caseStart = map[string]time.Time{}
	state.startedAt = time.Now()
	state.finalized = false
	// 模拟已评分一例
	recordAndCheck(benchCases[0], "1024", time.Now())
	state.mu.Unlock()

	state.mu.Lock()
	finalizePendingAsSkipLocked()
	state.mu.Unlock()

	state.mu.Lock()
	next, done := serveNext(state.root)
	// finalize 后不应再被下发（done），且剩余用例都是 skip。
	if !done {
		t.Fatalf("finalize 后 serveNext 应 done，got %v", next)
	}
	res0 := state.results[benchCases[0].ID]
	if res0.Status != "pass" {
		t.Errorf("已评分用例不应被改写，got %q", res0.Status)
	}
	if r, ok := state.results[benchCases[1].ID]; !ok || r.Status != "skip" {
		t.Errorf("未评分用例应标 skip，got %+v", r)
	}
	state.mu.Unlock()
}

// TestCaseBudgetSkipsOverdue 每案例预算：已下发且超时的用例被标 skip 并推进，
// 未下发（caseStart 未置位）的用例不被误伤。
func TestCaseBudgetSkipsOverdue(t *testing.T) {
	state.mu.Lock()
	state.root = t.TempDir()
	state.results = map[string]CaseResult{}
	state.caseStart = map[string]time.Time{
		benchCases[0].ID: time.Now().Add(-5 * time.Second), // 首例已下发且超 5s
	}
	state.caseBudget = 1 * time.Second
	state.caseTimer = nil
	state.mu.Unlock()

	state.mu.Lock()
	armCaseBudgetLocked()
	state.mu.Unlock()

	state.mu.Lock()
	if r := state.results[benchCases[0].ID]; r.Status != "skip" {
		t.Errorf("超预算用例应被标 skip，got %q", r.Status)
	}
	// 未下发（caseStart 未置位）的用例不应被连带 skip。
	if r, ok := state.results[benchCases[1].ID]; ok && r.Status == "skip" {
		t.Error("未下发用例不应被 skip")
	}
	// 清空预算与计时器，避免污染后续测试。
	state.caseBudget = 0
	if state.caseTimer != nil {
		_ = state.caseTimer.Stop()
		state.caseTimer = nil
	}
	state.mu.Unlock()
}

func TestRenderTaskPlaceholders(t *testing.T) {
	c := CaseQuery{ID: "x", Task: "写到 <caseOut>/reply.txt（根 <benchRoot>）"}
	out := renderTask(c, `C:\tmp`)
	if !strings.Contains(out, filepath.ToSlash(filepath.Join(`C:\tmp`, benchOutDirName, "x"))+"/reply.txt") {
		t.Errorf("占位符未正确替换: %q", out)
	}
	if strings.Contains(out, "<caseOut>") || strings.Contains(out, "<benchRoot>") {
		t.Errorf("占位符残留: %q", out)
	}
}
