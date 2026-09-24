package tui

import (
	"context"
	"strings"
	"testing"

	"charm.land/bubbles/v2/viewport"
	"dsc/userquestions"
	"github.com/charmbracelet/x/ansi"
)

// TestQuestionOverlayRowBudgetAnalysis 模拟用户截图场景：终端高度较小，
// 3 个选项的覆盖层渲染后逻辑行数与 vpHeight 扣减是否一致；并验证
// 总布局（标题+viewport+覆盖层+composer+info+status）是否超出 m.high。
func TestQuestionOverlayRowBudgetAnalysis(t *testing.T) {
	// 模拟用户截图：~37 行终端
	for _, high := range []int{37, 30, 25} {
		t.Run("high="+itoa(high), func(t *testing.T) {
			m := New(&stubAgent{}, nil, context.Background(), "m", "minimal", 131072)
			m.high = high
			m.width = 93 // 用户截图估算宽度
			m.viewport = viewport.New(viewport.WithWidth(93), viewport.WithHeight(m.vpHeight()))
			m.viewport.SetWidth(93)
			m.viewport.SetHeight(m.vpHeight())
			m.ready = true
			m.input.SetWidth(85)

			ansCh := make(chan *userquestions.Answer, 1)
			errCh := make(chan error, 1)
			m.Update(questionMsg{request: &userquestions.Request{Questions: []userquestions.Question{{
				ID:       "q",
				Question: "請告訴我您想分析哪個專案或哪個目錄的源碼？",
				Options: []userquestions.Option{
					{Label: "當前目錄下的源碼", Description: "當前工作目錄下的所有源碼"},
					{Label: "特定專案的源碼", Description: "特定專案的源碼"},
					{Label: "特定功能的源碼", Description: "特定功能的源碼"},
				},
			}}}, answer: ansCh, err: errCh})

			v := m.questionView()
			logicalRows := strings.Count(v, "\n") + 1

			// 计算实际可见行数（考虑宽度 wrap）
			// 终端宽度 - boxBorder(2) - padding(2) = 内部内容宽度
			innerWidth := m.width - 4
			if innerWidth < 1 {
				innerWidth = 1
			}
			actualRows := 0
			for _, line := range strings.Split(v, "\n") {
				if line == "" {
					actualRows++
					continue
				}
				w := ansi.StringWidth(line)
				rows := (w + innerWidth - 1) / innerWidth
				if rows < 1 {
					rows = 1
				}
				actualRows += rows
			}

			vh := m.vpHeight()
			// 总布局行数：标题(1) + viewport + 覆盖层 + composer(input.Height+2) + info(1) + status(2)
			total := 1 + vh + actualRows + (m.input.Height() + 2) + 1 + 2
			t.Logf("high=%d: vpHeight=%d, overlay logical=%d actual(wrap)=%d, total=%d (over by %d)",
				high, vh, logicalRows, actualRows, total, total-high)
			if total > high {
				t.Errorf("LAYOUT OVERFLOW: total=%d > high=%d → overlay clipped by %d rows",
					total, high, total-high)
			}
		})
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
