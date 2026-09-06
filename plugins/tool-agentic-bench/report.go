package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	dsc "dsc-sdk"
)

// benchOutDirName 是 bench 产物在 workspace 根下的固定子目录；各用例按
// <benchRoot>/bench-out/<ID>/ 自成一目录，避免相互污染。
const benchOutDirName = "bench-out"

// buildReport 依据各用例结果自动计分并生成汇总。
// totalMs 为从 bench_start 起算的总耗时毫秒，落盘进报告并以徽标展示。
// 返回：摘要文本、报告 JSON 字符串、表格视图 spec。
func buildReport(root string, results map[string]CaseResult, totalMs int64) (string, string, json.RawMessage) {
	passed, total := 0, len(benchCases)
	columns := []dsc.ViewColumn{
		{Key: "id", Title: "Case"},
		{Key: "title", Title: "任务"},
		{Key: "status", Title: "状态"},
		{Key: "score", Title: "得分"},
		{Key: "dur", Title: "耗时"},
	}
	rows := []dsc.ViewRow{}
	recs := make([]map[string]any, 0, len(benchCases))
	for _, c := range benchCases {
		res, ok := results[c.ID]
		if !ok || res.Status == "" {
			res = CaseResult{Status: "skip", Feedback: "未作答"}
		}
		if res.Status == "pass" {
			passed++
		}
		symbol := map[string]string{"pass": "PASS", "fail": "FAIL", "skip": "SKIP"}[res.Status]
		rows = append(rows, dsc.ViewRow{
			"id":     c.ID,
			"title":  c.Title,
			"status": symbol,
			"score":  fmt.Sprintf("%d/%d", scoreOf(c, res), c.Weight),
			"dur":    fmt.Sprintf("%dms", res.DurationMs),
		})
		got := ""
		if details, err := os.ReadFile(filepath.Join(root, benchOutDirName, c.ID, "reply.txt")); err == nil {
			got = strings.TrimSpace(string(details))
		}
		recs = append(recs, map[string]any{
			"id":               c.ID,
			"title":            c.Title,
			"status":           res.Status,
			"weight":           c.Weight,
			"earned":           scoreOf(c, res),
			"duration_ms":      res.DurationMs,
			"feedback":         res.Feedback,
			"artifact_content": got,
		})
	}
	// 汇总口径（单点制：每用例 1 点，只判成败）：
	//   score 总得分 = 成功数/总案例×100，两位小数的数值（如 85.71）
	//   ratio 成败比 = 成功数:失败数（如 12:2）
	ratio := float64(passed) / float64(total)
	scoreStr := fmt.Sprintf("%.2f", ratio*100)
	failed := 0
	for _, res := range results {
		if res.Status == "fail" {
			failed++
		}
	}
	ratioStr := fmt.Sprintf("%d:%d", passed, failed)
	// 表格末尾补一行汇总（总得分 + 成败比 + 总耗时）。
	rows = append(rows, dsc.ViewRow{
		"id":     "— 汇总 —",
		"title":  fmt.Sprintf("通过 %d/%d · 成败 %s", passed, total, ratioStr),
		"status": "",
		"score":  scoreStr,
		"dur":    humanMs(totalMs),
	})

	summaryJSON, _ := json.MarshalIndent(map[string]any{
		"harness":     "dsc-tool-agentic-bench",
		"bench_root":  root,
		"started_at":  state.startedAt.Format(time.RFC3339Nano),
		"duration_ms": totalMs,
		"passed":      passed,
		"failed":      failed,
		"total":       total,
		"score":       scoreStr, // 总得分（0–100，两位小数）
		"ratio":       ratioStr, // 成败比 = 成功数:失败数
		"cases":       recs,
	}, "", "  ")

	// 便于无人值守运行收集的固定路径 JSON 报告。
	reportPath := filepath.Join(root, benchOutDirName, "report.json")
	_ = os.MkdirAll(filepath.Dir(reportPath), 0o755)
	_ = os.WriteFile(reportPath, summaryJSON, 0o644)

	badgeTone := "red"
	if passed == total {
		badgeTone = "green"
	}
	view := dsc.TableView("bench 结果", &dsc.ViewBadge{Text: fmt.Sprintf("%d/%d PASS · 得分 %s · %s", passed, total, scoreStr, humanMs(totalMs)), Tone: badgeTone}, columns, rows)
	return fmt.Sprintf("通过 %d/%d，得分 %s，耗时 %s", passed, total, scoreStr, humanMs(totalMs)), string(summaryJSON), view
}

// humanMs 把毫秒格式化为可读时长（<1000ms 用毫秒，否则秒/分）。
func humanMs(ms int64) string {
	if ms < 0 {
		return "0ms"
	}
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	if ms < 60*1000 {
		return fmt.Sprintf("%.1fs", float64(ms)/1000)
	}
	return fmt.Sprintf("%d分%d秒", ms/(60*1000), (ms/1000)%60)
}

// scoreOf 返回用例食得的分值（pass → Weight，其余 0）。
func scoreOf(c CaseQuery, res CaseResult) int {
	if res.Status == "pass" {
		return c.Weight
	}
	return 0
}
