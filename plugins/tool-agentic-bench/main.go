package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	dsc "dsc-sdk"
)

// CaseResult 记录单个用例的评分结果。
type CaseResult struct {
	Status     string // "pass" | "fail" | "skip"
	Feedback   string
	DurationMs int64 // 该用例从首次 bench_next 下发到 bench_submit 的毫秒耗时
}

// benchState 保存本次 test run 的共享状态（进程内，多工具串行调用）。
type benchState struct {
	mu           sync.Mutex
	root         string
	results      map[string]CaseResult
	startedAt    time.Time            // bench_start 起算的总耗时钟源
	caseStart    map[string]time.Time // 各用例首次 bench_next 下发的时戳（单项耗时起点）
	finalized    bool                 // 交卷预算触发后置位，剩余用例已按 skip 收尾
	timeoutTimer *time.Timer          // 全局交卷预算看门狗（DSC_BENCH_TIMEOUT，不设=无限制）
	caseBudget   time.Duration        // 每案例预算（DSC_BENCH_CASE_TIMEOUT，默认 0=关闭）
	caseTimer    *time.Timer          // 当前待办用例的每案例预算看门狗
}

var state = &benchState{results: map[string]CaseResult{}, caseStart: map[string]time.Time{}}

// benchRoot 返回 bench 产物根：宿主注入的 workspace 根（对齐沙箱，相对路径写
// 天然落在其内）；未注入时回退当前工作目录。bench 无人值守运行时建议把
// workspace 指向一个干净的临时目录。
func benchRoot() string {
	if r := os.Getenv("DSC_WORKSPACE_ROOT"); r != "" {
		return r
	}
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return os.TempDir()
}

// caseOutDir 返回某用例在产物根下的绝对目录。
func caseOutDir(root, id string) string {
	return filepath.Join(root, benchOutDirName, id)
}

// renderTask 把用例任务陈述里的 <benchRoot>/<caseOut> 占位符替换为真实路径后返回。
func renderTask(c CaseQuery, root string) string {
	out := caseOutDir(root, c.ID)
	return strings.ReplaceAll(strings.ReplaceAll(c.Task, "<caseOut>", filepath.ToSlash(out)), "<benchRoot>", filepath.ToSlash(root))
}

// findNextCase 返回下一个待办（尚无 pass/fail，且非显式 skip 终态）用例下标，
// -1 表示全部完成。skip 视为预算收尾的终态，不再下发。
func findNextCase(results map[string]CaseResult) int {
	for i, c := range benchCases {
		res, ok := results[c.ID]
		if !ok || res.Status == "" {
			return i
		}
	}
	return -1
}

// recordAndCheck 按用例规则对给定候选文本判定并落结果；返回判定文本与命中与否。
// startedAt 为该用例首次下发的时戳，据此记录单项耗时（毫秒）。
func recordAndCheck(c CaseQuery, candidate string, startedAt time.Time) (string, bool) {
	ok, fb := matchCaseText(c, candidate)
	status := "fail"
	if ok {
		status = "pass"
	}
	state.results[c.ID] = CaseResult{Status: status, Feedback: fb, DurationMs: msSince(startedAt)}
	if ok {
		return "PASS", true
	}
	return "FAIL（" + fb + "）", false
}

// msSince 返回从 t 到现在的毫秒耗时；t 为零值时返回 0（未开始计时）。
func msSince(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return time.Since(t).Milliseconds()
}

// ---------- 工具处理器 ----------

func hBenchStart(ctx context.Context, args json.RawMessage) (string, error) {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.root = benchRoot()
	state.results = map[string]CaseResult{}
	state.startedAt = time.Now() // 总耗时起算点
	state.caseStart = map[string]time.Time{}
	state.finalized = false
	// 预算看门狗（都要「设置才启用」）：
	//   DSC_BENCH_TIMEOUT      全局交卷预算（秒）——不设 = 无限制；到期把剩余未交卷用例标 skip 并落盘报告
	//   DSC_BENCH_CASE_TIMEOUT 每案例预算（秒）——不设 = 关闭；某题下发后超时即标 skip 并推进到下一题
	if state.timeoutTimer != nil {
		state.timeoutTimer.Stop()
		state.timeoutTimer = nil
	}
	if state.caseTimer != nil {
		state.caseTimer.Stop()
		state.caseTimer = nil
	}
	state.caseBudget = 0
	if v := os.Getenv("DSC_BENCH_TIMEOUT"); v != "" {
		if s, err := strconv.Atoi(v); err == nil && s > 0 {
			state.timeoutTimer = time.AfterFunc(time.Duration(s)*time.Second, finalizeRun)
		}
	}
	if v := os.Getenv("DSC_BENCH_CASE_TIMEOUT"); v != "" {
		if s, err := strconv.Atoi(v); err == nil && s > 0 {
			state.caseBudget = time.Duration(s) * time.Second
		}
	}
	outRoot := filepath.Join(state.root, benchOutDirName)
	if err := os.MkdirAll(outRoot, 0o755); err != nil {
		return "", fmt.Errorf("创建产物目录失败: %w", err)
	}
	type caseListItem struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	list := make([]caseListItem, 0, len(benchCases))
	for _, c := range benchCases {
		list = append(list, caseListItem{ID: c.ID, Title: c.Title})
	}
	payload, _ := json.Marshal(map[string]any{
		"harness":     "dsc-tool-agentic-bench",
		"bench_root":  state.root,
		"out_dir":     filepath.ToSlash(outRoot),
		"total_cases": len(benchCases),
		"cases":       list,
	})
	return string(payload), nil
}

func hBenchNext(ctx context.Context, args json.RawMessage) (string, error) {
	state.mu.Lock()
	defer state.mu.Unlock()
	root := state.root
	if root == "" {
		root = benchRoot()
	}
	fields, done := serveNext(root)
	if done {
		return `{"done":true}`, nil
	}
	payload, _ := json.Marshal(fields)
	return string(payload), nil
}

// serveNext 返回「下一未评分用例」的负载字段，并记录其首次下发的时戳（单项耗时起点）。
// 调用方须持有 state.mu。全部完成时返回 (nil, true)。这是 bench 的推进机制：比如
// bench_submit 交卷后会自动推进到这里，让模型没有机会反复重试已评分用例。
func serveNext(root string) (map[string]any, bool) {
	idx := findNextCase(state.results)
	done := 0
	for _, cc := range benchCases {
		if r, ok := state.results[cc.ID]; ok && (r.Status == "pass" || r.Status == "fail") {
			done++
		}
	}
	if idx < 0 {
		return nil, true
	}
	c := benchCases[idx]
	// 记录该用例首次下发的时戳（单项耗时起点）；重复请求幂等，不覆盖。
	if _, ok := state.caseStart[c.ID]; !ok {
		state.caseStart[c.ID] = time.Now()
	}
	armCaseBudgetLocked() // 布防每案例预算（若启用），从本用例下发时刻起算
	return map[string]any{
		"done":    false,
		"index":   idx + 1,
		"total":   len(benchCases),
		"case_id": c.ID,
		"title":   c.Title,
		"kind":    c.Kind,
		"out_dir": filepath.ToSlash(caseOutDir(root, c.ID)),
		"task":    renderTask(c, root),
	}, false
}

func hBenchSubmit(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		CaseID string `json:"case_id"`
		Answer string `json:"answer,omitempty"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if p.CaseID == "" {
		return "", fmt.Errorf("case_id is required")
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	root := state.root
	if root == "" {
		root = benchRoot()
	}
	var c *CaseQuery
	for i := range benchCases {
		if benchCases[i].ID == p.CaseID {
			c = &benchCases[i]
			break
		}
	}
	if c == nil {
		return "", fmt.Errorf("unknown case_id %q", p.CaseID)
	}
	// 幂等：已评分则返回既有结果，但仍自动推进到下一未评分用例，避免模型反复卡在同一题。
	if r, ok := state.results[c.ID]; ok && (r.Status == "pass" || r.Status == "fail") {
		txt := "PASS"
		if r.Status != "pass" {
			txt = "FAIL（" + r.Feedback + "）"
		}
		return submitPayload(c.ID, r.Status, txt, true, root), nil
	}
	var candidate string
	startAt := state.caseStart[c.ID]
	if c.Kind == "answer" {
		candidate = p.Answer
	} else { // file：插件直接读产物文件判定（端态校验，无需模型回传内容）。
		raw, err := os.ReadFile(filepath.Join(caseOutDir(root, c.ID), "reply.txt"))
		if err != nil {
			state.results[c.ID] = CaseResult{Status: "fail", Feedback: "未找到产物文件 " + c.Relative, DurationMs: msSince(startAt)}
			return submitPayload(c.ID, "fail", "FAIL（未找到产物文件 "+c.Relative+"）", false, root), nil
		}
		candidate = string(raw)
	}
	verdict, _ := recordAndCheck(*c, candidate, startAt)
	return submitPayload(c.ID, verdictStatus(verdict), verdict, false, root), nil
}

// submitPayload 组装 bench_submit 的返回：本用例评分结果 + 自动推进的「下一用例」负载。
// 交卷后插件立即揭晓下一题，模型照做即可，不再需要手动调 bench_next 前进，也无从反复
// 重试已评分用例。
func submitPayload(caseID, status, verdict string, idempotent bool, root string) string {
	payload := map[string]any{
		"case_id": caseID,
		"status":  status,
		"result":  verdict,
	}
	if idempotent {
		payload["idempotent"] = true
	}
	next, done := serveNext(root)
	if done {
		payload["done"] = true
	} else {
		payload["done"] = false
		payload["next"] = next
	}
	b, _ := json.Marshal(payload)
	return string(b)
}

// verdictStatus 从判定文本取出状态值（供 JSON 输出）。
func verdictStatus(verdict string) string {
	if strings.HasPrefix(verdict, "PASS") {
		return "pass"
	}
	return "fail"
}

func hBenchReport(ctx context.Context, args json.RawMessage) (string, error) {
	state.mu.Lock()
	defer state.mu.Unlock()
	root := state.root
	if root == "" {
		root = benchRoot()
	}
	summary, _, _ := buildReport(root, state.results, totalMs())
	// 报告 JSON 已落盘到 <root>/bench-out/report.json，结果字符串给出摘要与路径。
	return fmt.Sprintf("%s（报告已写入 %s）", summary, filepath.ToSlash(filepath.Join(root, benchOutDirName, "report.json"))), nil
}

// totalMs 返回从 bench_start 到现在（或上次已报告节点）的总耗时毫秒；未开始为 0。
func totalMs() int64 {
	return msSince(state.startedAt)
}

// finalizeRun 全局交卷预算到期的强制终局：把仍未评分（未 pass/fail）的用例标为 skip，
// 并即时落盘最终报告——即便模型卡在某题拒不交卷，也能拿到完整的 report.json。
func finalizeRun() {
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.finalized {
		return
	}
	state.finalized = true
	if state.caseTimer != nil {
		state.caseTimer.Stop()
		state.caseTimer = nil
	}
	finalizePendingAsSkipLocked()
	buildReport(state.root, state.results, totalMs())
}

// armCaseBudgetLocked 为「当前已下发未交卷」的用例按每案例预算布防看门狗（须持锁）。
// 预算关闭或当前待办用例尚未下发（caseStart 未置位）时不布防；已超时则标 skip 并推进。
func armCaseBudgetLocked() {
	if state.caseTimer != nil {
		state.caseTimer.Stop()
		state.caseTimer = nil
	}
	if state.caseBudget <= 0 {
		return
	}
	idx := findNextCase(state.results)
	if idx < 0 {
		return
	}
	c := benchCases[idx]
	st := state.caseStart[c.ID]
	if st.IsZero() {
		return // 尚未下发，不算进行中，不布防
	}
	elapsed := time.Since(st)
	if elapsed >= state.caseBudget {
		state.results[c.ID] = CaseResult{Status: "skip", Feedback: "单案例预算超时，未提交"}
		armCaseBudgetLocked() // 推进到下一题继续布防
		return
	}
	state.caseTimer = time.AfterFunc(state.caseBudget-elapsed, caseBudgetFired)
}

// caseBudgetFired 每案例预算到期：当前待办已超预算则标 skip 并推进到下一题。
func caseBudgetFired() {
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.caseTimer != nil {
		state.caseTimer.Stop()
		state.caseTimer = nil
	}
	idx := findNextCase(state.results)
	if idx < 0 {
		return
	}
	c := benchCases[idx]
	if r, ok := state.results[c.ID]; ok && (r.Status == "pass" || r.Status == "fail") {
		return
	}
	if _, ok := state.caseStart[c.ID]; !ok {
		return
	}
	state.results[c.ID] = CaseResult{Status: "skip", Feedback: "单案例预算超时，未提交"}
	armCaseBudgetLocked()
}

// finalizePendingAsSkipLocked 把未评分（无 pass/fail 结果）的用例置为 skip（须持锁）。
func finalizePendingAsSkipLocked() {
	for _, c := range benchCases {
		res, ok := state.results[c.ID]
		if !ok || (res.Status != "pass" && res.Status != "fail") {
			state.results[c.ID] = CaseResult{Status: "skip", Feedback: "交卷预算超时，未提交"}
		}
	}
}

// ---------- 视图 ----------

func viewBenchStart(ctx context.Context, args json.RawMessage, result string) (json.RawMessage, error) {
	var p struct {
		Harness   string `json:"harness"`
		BenchRoot string `json:"bench_root"`
		OutDir    string `json:"out_dir"`
		Total     int    `json:"total_cases"`
	}
	_ = json.Unmarshal([]byte(result), &p)
	return dsc.CardView("bench 已就绪", &dsc.ViewBadge{Text: fmt.Sprintf("%d 用例", p.Total), Tone: "teal"}, []dsc.ViewField{
		{Key: "产物根", Value: p.BenchRoot},
		{Key: "输出目录", Value: p.OutDir},
		{Key: "用例数", Value: fmt.Sprintf("%d", p.Total)},
	}), nil
}

func viewBenchNext(ctx context.Context, args json.RawMessage, result string) (json.RawMessage, error) {
	var p struct {
		Done   bool   `json:"done"`
		Index  int    `json:"index"`
		Total  int    `json:"total"`
		ID     string `json:"case_id"`
		Title  string `json:"title"`
		Kind   string `json:"kind"`
		OutDir string `json:"out_dir"`
		Task   string `json:"task"`
	}
	_ = json.Unmarshal([]byte(result), &p)
	if p.Done {
		return dsc.PlainView("bench 完成", &dsc.ViewBadge{Text: "all done", Tone: "green"}, "所有用例已评分，返回 bench_report 查看汇总。"), nil
	}
	return dsc.PlainView(fmt.Sprintf("%d/%d · %s", p.Index, p.Total, p.Title),
		&dsc.ViewBadge{Text: p.ID, Tone: "teal"}, p.Task+"\n\n（结果类型: "+p.Kind+"）"), nil
}

func viewBenchSubmit(ctx context.Context, args json.RawMessage, result string) (json.RawMessage, error) {
	var p struct {
		CaseID string `json:"case_id"`
		Status string `json:"status"`
		Result string `json:"result"`
	}
	_ = json.Unmarshal([]byte(result), &p)
	tone := "red"
	if p.Status == "pass" {
		tone = "green"
	}
	return dsc.CardView("bench 结果", &dsc.ViewBadge{Text: p.Result, Tone: tone}, []dsc.ViewField{
		{Key: "用例", Value: p.CaseID},
		{Key: "状态", Value: p.Status},
	}), nil
}

func viewBenchReport(ctx context.Context, args json.RawMessage, result string) (json.RawMessage, error) {
	state.mu.Lock()
	defer state.mu.Unlock()
	root := state.root
	if root == "" {
		root = benchRoot()
	}
	_, _, view := buildReport(root, state.results, totalMs())
	return view, nil
}

func main() {
	sdk := dsc.New(dsc.Config{Name: "agentic-bench", Version: "1.0.0", Type: dsc.TypeTool})

	emptySchema := json.RawMessage(`{"type":"object","properties":{}}`)
	submitSchema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"case_id": {"type": "string", "description": "bench_next 返回的用例 id"},
			"answer": {"type": "string", "description": "仅对 answer 类用例提交；file 类由插件直接读取产物文件判定，无需传入"}
		},
		"required": ["case_id"]
	}`)

	// bench 工具是纯 Go + dsc-sdk，可交叉编译全部目标平台（七端均零 CGO）。
	sdk.Tool(dsc.Tool{
		Name:        "bench_start",
		Description: "初始化一次 bench 评分测试：重置全部用例结果，创建产物目录，返回用例总数、产物根目录与各用例 id/标题清单。开始测试前必须先调用本工具。",
		Schema:      emptySchema,
		Handler:     hBenchStart,
		ViewFn:      viewBenchStart,
		// Context 进入 ListContext / system prompt：引导模型真实完成任务、不得向
		// bench 工具索要答案，否则评分失真。
		Context: "bench_* 工具是一套模型能力评分测试台（bench），评分对象是模型自身。请真实使用你的工具（文件系统、shell、lisp_eval 等）逐一完成任务：对 file 类用例必须用文件工具把产物写到规定的 reply.txt，对 answer 类用例把答案经 bench_submit 提交。bench 只负责评分，不会也不会告诉你期望答案——不要向 bench 工具索要标准答案，这会判 FAIL。全部用例完成后调用 bench_report 查看自动计分汇总。",
	})
	sdk.Tool(dsc.Tool{
		Name:        "bench_next",
		Description: "取下一个未评分的测试用例（含用例 id、标题与要完成的任务陈述）。返回 done:true 表示全部完成。首个用例可用它获取，其后经 bench_submit 会自动推进到下一用例，无需反复调用。",
		Schema:      emptySchema,
		Handler:     hBenchNext,
		ViewFn:      viewBenchNext,
	})
	sdk.Tool(dsc.Tool{
		Name:        "bench_submit",
		Description: "提交某用例的完成结果并自动评分。answer 类需带 answer 文本；file 类只传 case_id，插件直接读取规定产物文件判定。返回 PASS/FAIL（幂等，不会重复评分），并**自动推进到下一用例**：响应里的 next 字段直接给出下一题（case_id+task），照做即可——已评分用例无法重试，逐题前进直至 next 消失（done:true）。",
		Schema:      submitSchema,
		Handler:     hBenchSubmit,
		ViewFn:      viewBenchSubmit,
	})
	sdk.Tool(dsc.Tool{
		Name:        "bench_report",
		Description: "自动汇总并输出本次测试的计分报告（通过/总数/得分百分比，逐用例状态），并把 JSON 报告写到 bench-out/report.json。",
		Schema:      emptySchema,
		Handler:     hBenchReport,
		ViewFn:      viewBenchReport,
	})
	sdk.Serve()
}
