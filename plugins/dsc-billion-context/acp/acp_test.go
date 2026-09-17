package acp

import (
	"fmt"
	"strings"
	"testing"
)

func TestAssignRefs(t *testing.T) {
	state := CreateInitialState()
	msgs := []CoreMessage{
		{ID: "a", Role: RoleUser, Text: "hello"},
		{ID: "b", Role: RoleAssistant, Text: "hi"},
		{ID: "c", Role: RoleUser, Text: "do something"},
	}
	AssignRefs(msgs, state)
	if got := RefForRaw("a", state); got != "m00000" {
		t.Errorf("ref for a = %q, want m00000", got)
	}
	if got := RefForRaw("c", state); got != "m00002" {
		t.Errorf("ref for c = %q, want m00002", got)
	}
	// 重复 AssignRefs 不应重新分配
	AssignRefs(msgs, state)
	if len(state.MessageRefs.ByRaw) != 3 {
		t.Errorf("ref count = %d, want 3 (should not re-assign)", len(state.MessageRefs.ByRaw))
	}
}

func TestApplyCompression(t *testing.T) {
	state := CreateInitialState()
	config := DefaultConfig(100000)
	msgs := []CoreMessage{
		{ID: "a", Role: RoleUser, Text: "first message"},
		{ID: "b", Role: RoleAssistant, Text: "second message"},
		{ID: "c", Role: RoleUser, Text: "third message"},
		{ID: "d", Role: RoleAssistant, Text: "fourth message"},
	}
	AssignRefs(msgs, state)

	ranges := []PruneRange{
		{StartRef: "m00000", EndRef: "m00001", Summary: "first two messages compressed"},
	}
	newState, result := ApplyCompression(ranges, msgs, state, config)
	if result.BlocksCreated != 1 {
		t.Fatalf("blocksCreated = %d, want 1", result.BlocksCreated)
	}
	if len(newState.Blocks) != 1 {
		t.Fatalf("blocks count = %d, want 1", len(newState.Blocks))
	}
	b := newState.Blocks[0]
	if b.BlockID != "b0" {
		t.Errorf("blockID = %q, want b0", b.BlockID)
	}
	if b.Tier != Tier1 {
		t.Errorf("tier = %v, want 1", b.Tier)
	}
	if !b.Active {
		t.Errorf("block should be active")
	}
	if len(b.DirectMessageIDs) != 2 {
		t.Errorf("directMessageIDs = %d, want 2", len(b.DirectMessageIDs))
	}
	// 验证覆盖消息集合
	covered := newState.CoveredMessageIDs()
	if !covered["a"] || !covered["b"] {
		t.Errorf("messages a, b should be covered, got %v", covered)
	}
	if covered["c"] || covered["d"] {
		t.Errorf("messages c, d should NOT be covered")
	}
}

func TestRenderMessages(t *testing.T) {
	state := CreateInitialState()
	config := DefaultConfig(100000)
	config.PreserveRecent = 1 // 只保留最后 1 条
	msgs := []CoreMessage{
		{ID: "a", Role: RoleUser, Text: "first"},
		{ID: "b", Role: RoleAssistant, Text: "second"},
		{ID: "c", Role: RoleUser, Text: "third"},
	}
	AssignRefs(msgs, state)

	// 压缩 a..b
	ranges := []PruneRange{
		{StartRef: "m00000", EndRef: "m00001", Summary: "a+b compressed"},
	}
	_, _ = ApplyCompression(ranges, msgs, state, config)

	// 渲染：a, b 应被替换为 summary（放在 a 的原位置），c 保留
	rendered := RenderMessages(msgs, state, config, true)
	if len(rendered) < 2 {
		t.Fatalf("rendered messages = %d, want ≥2 (summary + c)", len(rendered))
	}
	// 第一条应是块 summary（ID 形如 acp_summary_b0，role=system）
	if rendered[0].ID != "acp_summary_b0" {
		t.Errorf("first rendered msg ID = %q, want acp_summary_b0", rendered[0].ID)
	}
	if rendered[0].Role != RoleSystem {
		t.Errorf("summary role = %q, want system", rendered[0].Role)
	}
	// 最后一条应是 c（保留区）
	last := rendered[len(rendered)-1]
	if last.ID != "c" {
		t.Errorf("last rendered msg ID = %q, want c", last.ID)
	}
}

// TestRenderMessagesPrefixStable 验证前缀稳定性（核心语义，对齐原作者描述）：
//
//	[summary1] [summary2] ... [最近新增消息]
//
// summary 放在它替换的原始范围的最早消息位置（insertAt），不是堆到列表开头。
// 一旦 summary 写定，后续轮次渲染时经 summaryMessageId 识别"已渲染的 summary"，
// 保持其位置稳定——前缀缓存命中率因此达 98-99%。
func TestRenderMessagesPrefixStable(t *testing.T) {
	state := CreateInitialState()
	config := DefaultConfig(100000)
	msgs := []CoreMessage{
		{ID: "a", Role: RoleUser, Text: "first message"},
		{ID: "b", Role: RoleAssistant, Text: "second message"},
		{ID: "c", Role: RoleUser, Text: "third message"},
		{ID: "d", Role: RoleAssistant, Text: "fourth message"},
		{ID: "e", Role: RoleUser, Text: "fifth message"},
	}
	AssignRefs(msgs, state)

	// 压缩 a..b（块 b0）
	ranges1 := []PruneRange{
		{StartRef: "m00000", EndRef: "m00001", Summary: "summary of a+b", Topic: "intro"},
	}
	_, _ = ApplyCompression(ranges1, msgs, state, config)

	rendered1 := RenderMessages(msgs, state, config, true)
	// 验证：summary 在最前（insertAt=a 的位置=0）
	// 原版 acp-kernel 保留首条 user 消息（firstUserIndex），故 a 也保留
	if rendered1[0].ID != "acp_summary_b0" {
		t.Errorf("first rendered = %q, want acp_summary_b0", rendered1[0].ID)
	}
	// 第二条应是 a（首条 user 保留规则）或 c（如果 a 在 firstUserIndex 处被替换）
	// 原版语义：firstUserIndex 处的 a 即使被覆盖也保留——验证此行为
	foundA := false
	for _, m := range rendered1 {
		if m.ID == "a" {
			foundA = true
			break
		}
	}
	if !foundA {
		t.Errorf("first user message 'a' should be retained (firstUserIndex rule)")
	}

	// 现在把 rendered1 当作"上一轮发给 LLM 的消息列表"（含已渲染 summary）
	// 压缩 c..d（块 b1）
	ranges2 := []PruneRange{
		{StartRef: "m00002", EndRef: "m00003", Summary: "summary of c+d", Topic: "middle"},
	}
	_, _ = ApplyCompression(ranges2, rendered1, state, config)

	// 重新渲染：b0 summary 应保持在原位置（前缀稳定）
	rendered2 := RenderMessages(rendered1, state, config, true)
	if len(rendered2) < 3 {
		t.Fatalf("rendered2 = %d msgs, want ≥3", len(rendered2))
	}
	// b0 summary 仍是第一条（前缀稳定）
	if rendered2[0].ID != "acp_summary_b0" {
		t.Errorf("after second compress: first rendered = %q, want acp_summary_b0 (prefix stable)", rendered2[0].ID)
	}
	// b1 summary 应存在（在 b0 之后某位置）
	foundB1 := false
	for _, m := range rendered2 {
		if m.ID == "acp_summary_b1" {
			foundB1 = true
			break
		}
	}
	if !foundB1 {
		t.Errorf("acp_summary_b1 should be present after second compress")
	}
	// 最后是 e
	last := rendered2[len(rendered2)-1]
	if last.ID != "e" {
		t.Errorf("last rendered = %q, want e", last.ID)
	}
}

func TestDecideNudge(t *testing.T) {
	state := CreateInitialState()
	config := DefaultConfig(10000) // 10K 窗口
	config.NudgeThresholdPct = 0.45
	config.MinCompressRangeChars = 0 // 关闭最小门：本测验证阈值门控，非范围大小门
	// 构造足够长的消息（每条 > 500 token 才能进入可压缩范围）
	longText := string(make([]byte, 2500)) // ~625 tokens (bytes/4)
	for i := range longText {
		longText = longText[:i] + "x" + longText[i+1:]
	}
	msgs := []CoreMessage{
		{ID: "a", Role: RoleUser, Text: longText},
		{ID: "b", Role: RoleAssistant, Text: longText},
		{ID: "c", Role: RoleUser, Text: longText},
		{ID: "d", Role: RoleAssistant, Text: "short tail"},
		{ID: "e", Role: RoleUser, Text: "short tail 2"},
	}
	AssignRefs(msgs, state)

	// 用量低于阈值：不应 nudge
	decision := DecideNudge(msgs, state, config, 1000) // 10% < 45%
	if decision.ShouldInject {
		t.Errorf("shouldInject = true, want false (below threshold)")
	}

	// 用量高于阈值且有可压缩范围：应 nudge
	decision = DecideNudge(msgs, state, config, 5000) // 50% > 45%
	if !decision.ShouldInject {
		t.Errorf("shouldInject = false, want true (above threshold with compressible ranges). reason: %s", decision.Reason)
	}
	if len(decision.CompressibleRanges) == 0 {
		t.Errorf("compressibleRanges = 0, want ≥1")
	}
}

func TestSearch(t *testing.T) {
	state := CreateInitialState()
	state.Blocks = []CompressionBlock{
		{BlockID: "b0", Active: true, Topic: "authentication", Summary: "User logged in with token X and refreshed it."},
		{BlockID: "b1", Active: true, Topic: "deployment", Summary: "Deployed app to production server."},
		{BlockID: "b2", Active: false, Topic: "old", Summary: "deprecated auth token"}, // inactive，不应被搜到
	}
	results := Search("auth token", state, 5)
	// hybrid 算法（BM25+fuzzy）下弱相关块可能带小分残留（fuzzy 召回通道），
	// 核心断言：active 且强相关的 b0 必须居首；inactive 的 b2 绝不出现
	if len(results) == 0 {
		t.Fatalf("results = 0, want >= 1")
	}
	if results[0].BlockID != "b0" {
		t.Errorf("first result blockID = %q, want b0", results[0].BlockID)
	}
	for _, r := range results {
		if r.BlockID == "b2" {
			t.Errorf("inactive block b2 must never appear")
		}
	}
}

func TestDecompress(t *testing.T) {
	state := CreateInitialState()
	state.Blocks = []CompressionBlock{
		{BlockID: "b0", Active: true, Summary: "compressed summary text"},
	}
	summary, err := Decompress("b0", state, false)
	if err != nil {
		t.Fatalf("decompress error: %v", err)
	}
	if summary != "compressed summary text" {
		t.Errorf("summary = %q, want 'compressed summary text'", summary)
	}

	// 不存在的块
	_, err = Decompress("b999", state, false)
	if err == nil {
		t.Errorf("decompress non-existent block should error")
	}
}

func TestProcessTurn(t *testing.T) {
	state := CreateInitialState()
	config := DefaultConfig(10000)
	msgs := []CoreMessage{
		{ID: "a", Role: RoleUser, Text: "first message"},
		{ID: "b", Role: RoleAssistant, Text: "second message"},
	}
	result := ProcessTurn(msgs, state, config, 2000)
	if result.State == nil {
		t.Fatal("state should not be nil")
	}
	if len(result.State.MessageRefs.ByRaw) != 2 {
		t.Errorf("refs assigned = %d, want 2", len(result.State.MessageRefs.ByRaw))
	}
	// renderTags=false：不注入 <acp> 标签到消息文本（保持前缀缓存稳定）
	// 验证 ref 映射已建立（模型经 acp_status 工具查询 ref）
	if RefForRaw("a", result.State) != "m00000" {
		t.Errorf("ref for 'a' = %q, want m00000", RefForRaw("a", result.State))
	}
	if RefForRaw("b", result.State) != "m00001" {
		t.Errorf("ref for 'b' = %q, want m00001", RefForRaw("b", result.State))
	}
}

func TestStatePersistence(t *testing.T) {
	state := CreateInitialState()
	state.Blocks = []CompressionBlock{
		{BlockID: "b0", Active: true, Summary: "test", Tier: Tier1},
	}
	state.MessageRefs.ByRaw["a"] = "m00000"
	state.MessageRefs.ByRef["m00000"] = "a"
	state.NextBlockID = 1

	// 序列化反序列化
	data, _ := jsonMarshal(state)
	var restored CompressionState
	_ = jsonUnmarshal(data, &restored)
	if len(restored.Blocks) != 1 {
		t.Errorf("restored blocks = %d, want 1", len(restored.Blocks))
	}
	if restored.Blocks[0].BlockID != "b0" {
		t.Errorf("restored blockID = %q, want b0", restored.Blocks[0].BlockID)
	}
	if restored.MessageRefs.ByRaw["a"] != "m00000" {
		t.Errorf("restored ref map lost")
	}
}

// TestStripOrphanedToolResults 验证孤儿 tool-result 清理。
// 场景：压缩范围切在 tool-call 与 tool-result 之间——tool-call 被压缩，
// tool-result 留下成为孤儿，多数 provider 会报 HTTP 400。
func TestStripOrphanedToolResults(t *testing.T) {
	msgs := []CoreMessage{
		{ID: "a", Role: RoleUser, ContentType: ContentTypeText, Text: "do task"},
		// tool-call "tc1" 在这里
		{ID: "b", Role: RoleAssistant, ContentType: ContentTypeToolCall, ToolCallID: "tc1", ToolName: "shell"},
		// tool-result for tc1
		{ID: "c", Role: RoleTool, ContentType: ContentTypeToolResult, ToolCallID: "tc1", Text: "result"},
		// 孤儿 tool-result：没有对应 tool-call（已被压缩）
		{ID: "d", Role: RoleTool, ContentType: ContentTypeToolResult, ToolCallID: "tc2", Text: "orphan result"},
		{ID: "e", Role: RoleAssistant, ContentType: ContentTypeText, Text: "done"},
	}
	out := stripOrphanedToolResults(msgs)
	// d 应被移除
	for _, m := range out {
		if m.ID == "d" {
			t.Errorf("orphan tool-result 'd' should be stripped")
		}
	}
	if len(out) != 4 {
		t.Errorf("after strip: %d messages, want 4 (d removed)", len(out))
	}
}

// TestStripOrphanedToolCalls 验证孤儿 tool-call 清理。
// 场景：压缩范围切在 tool-call 与 tool-result 之间——tool-result 被压缩，
// tool-call 留下成为孤儿。
func TestStripOrphanedToolCalls(t *testing.T) {
	msgs := []CoreMessage{
		{ID: "a", Role: RoleUser, ContentType: ContentTypeText, Text: "do task"},
		// tool-call "tc1" 有对应 result
		{ID: "b", Role: RoleAssistant, ContentType: ContentTypeToolCall, ToolCallID: "tc1", ToolName: "shell"},
		{ID: "c", Role: RoleTool, ContentType: ContentTypeToolResult, ToolCallID: "tc1", Text: "result"},
		// 孤儿 tool-call：没有对应 tool-result（已被压缩）
		{ID: "d", Role: RoleAssistant, ContentType: ContentTypeToolCall, ToolCallID: "tc2", ToolName: "shell"},
		{ID: "e", Role: RoleAssistant, ContentType: ContentTypeText, Text: "done"},
	}
	out := stripOrphanedToolCalls(msgs)
	// d 应被移除
	for _, m := range out {
		if m.ID == "d" {
			t.Errorf("orphan tool-call 'd' should be stripped")
		}
	}
	if len(out) != 4 {
		t.Errorf("after strip: %d messages, want 4 (d removed)", len(out))
	}
}

// TestStripOrphanedToolCallsCompressExempt 验证 compress 工具的 tool-call
// 不被清理——它是模型发起的压缩请求，不需要 tool-result（其"结果"是消息列表改写本身）。
func TestStripOrphanedToolCallsCompressExempt(t *testing.T) {
	msgs := []CoreMessage{
		{ID: "a", Role: RoleUser, ContentType: ContentTypeText, Text: "do task"},
		// compress tool-call 无对应 result——应保留（exempt）
		{ID: "b", Role: RoleAssistant, ContentType: ContentTypeToolCall, ToolCallID: "tc1", ToolName: "compress"},
		{ID: "c", Role: RoleAssistant, ContentType: ContentTypeText, Text: "done"},
	}
	out := stripOrphanedToolCalls(msgs)
	if len(out) != 3 {
		t.Errorf("compress tool-call should be exempt: got %d msgs, want 3", len(out))
	}
}

// TestStripOrphanedReasoning 验证孤儿 reasoning 清理。
// 场景：严格 thinking 模型的 reasoning 必须紧跟 assistant text/tool-call，
// 否则返回 HTTP 400。
func TestStripOrphanedReasoning(t *testing.T) {
	msgs := []CoreMessage{
		{ID: "a", Role: RoleUser, ContentType: ContentTypeText, Text: "do task"},
		// reasoning 有 companion（后面的 assistant text）
		{ID: "b", Role: RoleAssistant, ContentType: ContentTypeReasoning, Text: "thinking..."},
		{ID: "c", Role: RoleAssistant, ContentType: ContentTypeText, Text: "answer"},
		// 孤儿 reasoning：companion 被压缩，只留下 reasoning
		{ID: "d", Role: RoleAssistant, ContentType: ContentTypeReasoning, Text: "more thinking..."},
		{ID: "e", Role: RoleUser, ContentType: ContentTypeText, Text: "next"},
	}
	out := stripOrphanedReasoning(msgs)
	// d 应被移除（companion 缺失）
	for _, m := range out {
		if m.ID == "d" {
			t.Errorf("orphan reasoning 'd' should be stripped")
		}
	}
	if len(out) != 4 {
		t.Errorf("after strip: %d messages, want 4 (d removed)", len(out))
	}
}

// TestRenderMessagesWithOrphanCleanup 验证 RenderMessages 在压缩后自动清理孤儿。
// 模拟压缩边界切断 tool-call ↔ tool-result 对的场景。
func TestRenderMessagesWithOrphanCleanup(t *testing.T) {
	state := CreateInitialState()
	config := DefaultConfig(100000)
	msgs := []CoreMessage{
		{ID: "a", Role: RoleUser, ContentType: ContentTypeText, Text: "do task"},
		{ID: "b", Role: RoleAssistant, ContentType: ContentTypeToolCall, ToolCallID: "tc1", ToolName: "shell"},
		{ID: "c", Role: RoleTool, ContentType: ContentTypeToolResult, ToolCallID: "tc1", Text: "result1"},
		{ID: "d", Role: RoleAssistant, ContentType: ContentTypeText, Text: "next step"},
		{ID: "e", Role: RoleAssistant, ContentType: ContentTypeToolCall, ToolCallID: "tc2", ToolName: "shell"},
		{ID: "f", Role: RoleTool, ContentType: ContentTypeToolResult, ToolCallID: "tc2", Text: "result2"},
		{ID: "g", Role: RoleAssistant, ContentType: ContentTypeText, Text: "done"},
	}
	AssignRefs(msgs, state)

	// 压缩 a..b（切在 b 与 c 之间：b 被压缩，c 留下成孤儿 tool-result；
	// e..f 完整保留；d 也被压缩）
	// 实际压缩 a..d 会同时压 b（tool-call）和 c（tool-result），不会产生孤儿——
	// 这里改为压缩 a..b（只压 user 和 tool-call，留下孤儿 tool-result c）
	ranges := []PruneRange{
		{StartRef: "m00000", EndRef: "m00001", Summary: "user + first tool-call compressed"},
	}
	_, _ = ApplyCompression(ranges, msgs, state, config)

	rendered := RenderMessages(msgs, state, config, false)
	// 验证：孤儿 tool-result c 应被清理（其对应 tool-call b 被压缩）
	for _, m := range rendered {
		if m.ID == "c" {
			t.Errorf("orphan tool-result 'c' should be stripped after render")
		}
	}
	// 验证：完整的 e..f 对应保留
	foundE, foundF := false, false
	for _, m := range rendered {
		if m.ID == "e" {
			foundE = true
		}
		if m.ID == "f" {
			foundF = true
		}
	}
	if !foundE || !foundF {
		t.Errorf("complete tool pair e..f should be retained (got e=%v f=%v)", foundE, foundF)
	}
}

// TestParseBoundary 验证边界引用解析（mNNNNN 与 bN）。
func TestParseBoundary(t *testing.T) {
	cases := []struct {
		input string
		kind  BoundaryKind
		num   int
		ok    bool
	}{
		{"m00000", BoundaryMessage, 0, true},
		{"m00005", BoundaryMessage, 5, true},
		{"m5", BoundaryMessage, 5, true},
		{"b0", BoundaryBlock, 0, true},
		{"b3", BoundaryBlock, 3, true},
		{"B3", BoundaryBlock, 3, true},   // 大写也接受
		{" b3 ", BoundaryBlock, 3, true}, // 带空格也接受
		{"x3", BoundaryKind(""), 0, false},
		{"", BoundaryKind(""), 0, false},
	}
	for _, c := range cases {
		b := ParseBoundary(c.input)
		if !c.ok {
			if b != nil {
				t.Errorf("ParseBoundary(%q) = %+v, want nil", c.input, b)
			}
			continue
		}
		if b == nil {
			t.Errorf("ParseBoundary(%q) = nil, want kind=%s num=%d", c.input, c.kind, c.num)
			continue
		}
		if b.Kind != c.kind || b.NumericID != c.num {
			t.Errorf("ParseBoundary(%q) = kind=%s num=%d, want kind=%s num=%d",
				c.input, b.Kind, b.NumericID, c.kind, c.num)
		}
	}
}

// TestApplyDistillationT2 验证 T2 蒸馏：多个 T1 块合并为 T2。
// 模型用 bN 形式作为 startId/endId 触发蒸馏。
func TestApplyDistillationT2(t *testing.T) {
	state := CreateInitialState()
	config := DefaultConfig(100000)
	msgs := []CoreMessage{
		{ID: "a", Role: RoleUser, Text: "first message"},
		{ID: "b", Role: RoleAssistant, Text: "second message"},
		{ID: "c", Role: RoleUser, Text: "third message"},
		{ID: "d", Role: RoleAssistant, Text: "fourth message"},
		{ID: "e", Role: RoleUser, Text: "fifth message"},
		{ID: "f", Role: RoleAssistant, Text: "sixth message"},
	}
	AssignRefs(msgs, state)

	// 先创建两个 T1 块：b0 (a..b) 和 b1 (c..d)
	ranges1 := []PruneRange{
		{StartRef: "m00000", EndRef: "m00001", Summary: "summary of a+b"},
		{StartRef: "m00002", EndRef: "m00003", Summary: "summary of c+d"},
	}
	_, _ = ApplyCompression(ranges1, msgs, state, config)
	if len(state.Blocks) != 2 {
		t.Fatalf("after T1: blocks = %d, want 2", len(state.Blocks))
	}

	// 验证两个块都是 T1、active
	for _, b := range state.Blocks {
		if b.Tier != Tier1 {
			t.Errorf("block %s tier = %d, want 1", b.BlockID, b.Tier)
		}
		if !b.Active {
			t.Errorf("block %s should be active", b.BlockID)
		}
	}

	// 蒸馏 b0..b1 为 T2
	ranges2 := []PruneRange{
		{StartRef: "b0", EndRef: "b1", Summary: "distilled T2 summary of b0+b1", Topic: "early conversation"},
	}
	_, result := ApplyCompression(ranges2, msgs, state, config)
	if result.BlocksCreated != 1 {
		t.Fatalf("distillation: blocksCreated = %d, want 1", result.BlocksCreated)
	}
	if len(state.Blocks) != 3 {
		t.Fatalf("after T2: blocks = %d, want 3 (2 inactive + 1 T2)", len(state.Blocks))
	}

	// 验证 T2 块
	t2Block := state.Blocks[2]
	if t2Block.Tier != Tier2 {
		t.Errorf("T2 block tier = %d, want 2", t2Block.Tier)
	}
	if !t2Block.Active {
		t.Errorf("T2 block should be active")
	}
	if len(t2Block.DirectBlockIDs) != 2 {
		t.Errorf("T2 block directBlockIDs = %d, want 2 (consumed b0, b1)", len(t2Block.DirectBlockIDs))
	}
	if t2Block.DirectBlockIDs[0] != "b0" || t2Block.DirectBlockIDs[1] != "b1" {
		t.Errorf("T2 block directBlockIDs = %v, want [b0, b1]", t2Block.DirectBlockIDs)
	}

	// 验证旧块被标记为 inactive
	if state.Blocks[0].Active {
		t.Errorf("b0 should be inactive after distillation")
	}
	if state.Blocks[1].Active {
		t.Errorf("b1 should be inactive after distillation")
	}

	// 验证 effectiveMessageIDs 是两个 T1 块的并集
	if len(t2Block.EffectiveMessageIDs) != 4 {
		t.Errorf("T2 effectiveMessageIDs = %d, want 4 (union of b0+b1)", len(t2Block.EffectiveMessageIDs))
	}
}

// TestApplyDistillationT3 验证 T3 凝结：多个 T2 块合并为 T3。
func TestApplyDistillationT3(t *testing.T) {
	state := CreateInitialState()
	config := DefaultConfig(100000)
	// 直接构造两个 active T2 块（模拟已蒸馏状态）
	state.Blocks = []CompressionBlock{
		{BlockID: "b0", Active: true, Tier: Tier2, Summary: "T2 summary 1", EffectiveMessageIDs: []string{"a", "b"}, CompressedTokens: 1000},
		{BlockID: "b1", Active: true, Tier: Tier2, Summary: "T2 summary 2", EffectiveMessageIDs: []string{"c", "d"}, CompressedTokens: 2000},
	}
	state.NextBlockID = 2

	// 凝结 b0..b1 为 T3
	ranges := []PruneRange{
		{StartRef: "b0", EndRef: "b1", Summary: "condensed T3 summary", Topic: "long-term memory"},
	}
	_, result := ApplyCompression(ranges, nil, state, config)
	if result.BlocksCreated != 1 {
		t.Fatalf("T3 condense: blocksCreated = %d, want 1", result.BlocksCreated)
	}

	t3Block := state.Blocks[2]
	if t3Block.Tier != Tier3 {
		t.Errorf("T3 block tier = %d, want 3", t3Block.Tier)
	}
	if len(t3Block.DirectBlockIDs) != 2 {
		t.Errorf("T3 directBlockIDs = %d, want 2", len(t3Block.DirectBlockIDs))
	}
	if len(t3Block.EffectiveMessageIDs) != 4 {
		t.Errorf("T3 effectiveMessageIDs = %d, want 4 (union)", len(t3Block.EffectiveMessageIDs))
	}
	// 旧块 inactive
	if state.Blocks[0].Active || state.Blocks[1].Active {
		t.Errorf("T2 blocks should be inactive after T3 condense")
	}
}

// TestDecideNudgeT2Trigger 验证 T2 蒸馏 nudge 触发。
func TestDecideNudgeT2Trigger(t *testing.T) {
	state := CreateInitialState()
	config := DefaultConfig(10000)
	config.NudgeThresholdPct = 0.30 // 降低阈值便于触发
	config.Tiers.Tier2Trigger = 3   // 3 个 T1 块就触发 T2

	// 构造 3 个 active T1 块
	state.Blocks = []CompressionBlock{
		{BlockID: "b0", Active: true, Tier: Tier1, Summary: "s0", CompressedTokens: 1000},
		{BlockID: "b1", Active: true, Tier: Tier1, Summary: "s1", CompressedTokens: 1000},
		{BlockID: "b2", Active: true, Tier: Tier1, Summary: "s2", CompressedTokens: 1000},
	}
	state.NextBlockID = 3

	// 构造足够长的消息
	longText := string(make([]byte, 2500))
	for i := range longText {
		longText = longText[:i] + "x" + longText[i+1:]
	}
	msgs := []CoreMessage{
		{ID: "a", Role: RoleUser, Text: longText},
		{ID: "b", Role: RoleAssistant, Text: "tail"},
		{ID: "c", Role: RoleUser, Text: "tail 2"},
		{ID: "d", Role: RoleAssistant, Text: "tail 3"},
		{ID: "e", Role: RoleUser, Text: "tail 4"},
	}
	AssignRefs(msgs, state)

	decision := DecideNudge(msgs, state, config, 5000) // 50% > 30%
	if !decision.ShouldInject {
		t.Fatalf("shouldInject = false, want true. reason: %s", decision.Reason)
	}
	if decision.Tier == nil || *decision.Tier != Tier2 {
		t.Errorf("tier = %v, want 2", decision.Tier)
	}
	if len(decision.TierTargetBlocks) != 3 {
		t.Errorf("tierTargetBlocks = %d, want 3", len(decision.TierTargetBlocks))
	}
}

// TestDecideNudgeT3Trigger 验证 T3 凝结 nudge 触发。
func TestDecideNudgeT3Trigger(t *testing.T) {
	state := CreateInitialState()
	config := DefaultConfig(10000)
	config.NudgeThresholdPct = 0.30
	config.Tiers.Tier3Trigger = 2 // 2 个 T2 块就触发 T3

	// 构造 2 个 active T2 块
	state.Blocks = []CompressionBlock{
		{BlockID: "b0", Active: true, Tier: Tier2, Summary: "t2-0", CompressedTokens: 1000},
		{BlockID: "b1", Active: true, Tier: Tier2, Summary: "t2-1", CompressedTokens: 1000},
	}
	state.NextBlockID = 2

	longText := string(make([]byte, 2500))
	for i := range longText {
		longText = longText[:i] + "x" + longText[i+1:]
	}
	msgs := []CoreMessage{
		{ID: "a", Role: RoleUser, Text: longText},
		{ID: "b", Role: RoleAssistant, Text: "tail"},
		{ID: "c", Role: RoleUser, Text: "tail 2"},
		{ID: "d", Role: RoleAssistant, Text: "tail 3"},
		{ID: "e", Role: RoleUser, Text: "tail 4"},
	}
	AssignRefs(msgs, state)

	decision := DecideNudge(msgs, state, config, 5000)
	if !decision.ShouldInject {
		t.Fatalf("shouldInject = false, want true. reason: %s", decision.Reason)
	}
	if decision.Tier == nil || *decision.Tier != Tier3 {
		t.Errorf("tier = %v, want 3", decision.Tier)
	}
	if len(decision.TierTargetBlocks) != 2 {
		t.Errorf("tierTargetBlocks = %d, want 2", len(decision.TierTargetBlocks))
	}
}

// TestFormatNudgeTextT2 验证 T2 蒸馏提示文本格式。
func TestFormatNudgeTextT2(t *testing.T) {
	tier := Tier2
	d := NudgeDecision{
		ShouldInject: true,
		Reason:       "T2 distill: 5 tier-1 blocks >= tier2Trigger 5",
		Tier:         &tier,
		TierTargetBlocks: []CompressionBlock{
			{BlockID: "b0", Tier: Tier1, Topic: "intro", CompressedTokens: 2000},
			{BlockID: "b1", Tier: Tier1, Topic: "auth", CompressedTokens: 1500},
		},
	}
	text := FormatNudgeText(d)
	if !containsStr(text, "tier-1") {
		t.Errorf("T2 nudge text should mention tier-1, got: %s", text)
	}
	if !containsStr(text, "b0") || !containsStr(text, "b1") {
		t.Errorf("T2 nudge text should list target blocks b0, b1, got: %s", text)
	}
	if !containsStr(text, "startId") {
		t.Errorf("T2 nudge text should show compress example with startId, got: %s", text)
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// TestComputeCompressibleRangesSkipsRenderedSummary 回归测试：
// 已渲染的 summary 消息（ID 前缀 acp_summary_）不应被纳入可压缩范围。
//
// Bug 重现：原实现用 strings.HasPrefix(msg.ID, "block:") 判断 summary，但实际
// summary ID 前缀是 "acp_summary_"（见 prune.go summaryIDPrefix）。结果 summary
// 被错误地纳入 compressible range，模型按 nudge 提示调用 compress 时又因
// isRenderedSummary 跳过（prune.go:83），最终报 "no compressible messages"，
// 形成 "nudge→compress→error→再 nudge" 死循环。
func TestComputeCompressibleRangesSkipsRenderedSummary(t *testing.T) {
	state := CreateInitialState()
	config := DefaultConfig(100000)
	config.PreserveRecent = 1 // 仅保留最后 1 条

	// 模拟第二轮 pre-step：消息列表已含上一轮渲染的 summary（在头部）
	// summary + 1 条长消息（≥500 token 才进入范围）+ 1 条短尾部（在保留区）
	longText := strings.Repeat("x", 2500) // ~625 tokens
	msgs := []CoreMessage{
		{ID: "acp_summary_b0", Role: RoleSystem, ContentType: ContentTypeText, Text: "[Compressed section] earlier discussion"},
		{ID: "new1", Role: RoleUser, Text: longText},
		{ID: "new2", Role: RoleAssistant, Text: "short tail"},
	}
	AssignRefs(msgs, state)

	ranges := ComputeCompressibleRanges(msgs, state, config)
	for _, r := range ranges {
		startID := RawForRef(r.StartRef, state)
		endID := RawForRef(r.EndRef, state)
		if strings.HasPrefix(startID, summaryIDPrefix) {
			t.Errorf("compressible range starts at rendered summary: startRef=%s (raw=%s)", r.StartRef, startID)
		}
		if strings.HasPrefix(endID, summaryIDPrefix) {
			t.Errorf("compressible range ends at rendered summary: endRef=%s (raw=%s)", r.EndRef, endID)
		}
	}
	// 期望：只有 new1 在可压缩范围里（summary 被跳过，new2 在保留区）
	if len(ranges) != 1 {
		t.Fatalf("expected exactly 1 compressible range (new1 only), got %d: %+v", len(ranges), ranges)
	}
	if startID := RawForRef(ranges[0].StartRef, state); startID != "new1" {
		t.Errorf("range start = %q, want new1 (summary should be skipped)", startID)
	}
}

// ---------------- sync-blocks ----------------

// TestSyncBlocks 验证块活性对齐：蒸馏消费停用 / Expanded 保持停用 /
// 覆盖消息与 summary 双缺席停用 / 其余保持 active。
func TestSyncBlocks(t *testing.T) {
	msgs := []CoreMessage{
		{ID: "a", Role: RoleUser, Text: "u1"},
		{ID: "acp_summary_b2", Role: RoleSystem, ContentType: ContentTypeText, Text: "[Compressed conversation section]\nsummary"},
		{ID: "acp_summary_b5", Role: RoleSystem, ContentType: ContentTypeText, Text: "[Compressed conversation section]\ndistilled"},
	}
	state := CreateInitialState()
	state.Blocks = []CompressionBlock{
		{BlockID: "b0", Active: true, EffectiveMessageIDs: []string{"x1"}}, // 消息缺席 → 停用
		{BlockID: "b1", Active: true, EffectiveMessageIDs: []string{"x2"}, Expanded: true},
		{BlockID: "b2", Active: false, DirectBlockIDs: nil}, // summary 在场 → 重新激活
		{BlockID: "b3", Active: true, EffectiveMessageIDs: []string{"x3"}},
	}
	// b4 被 b5 蒸馏消费
	state.Blocks = append(state.Blocks,
		CompressionBlock{BlockID: "b4", Active: true, EffectiveMessageIDs: []string{"x4"}},
		CompressionBlock{BlockID: "b5", Active: true, EffectiveMessageIDs: []string{"x5"}, DirectBlockIDs: []string{"b4"}})
	deactivated := SyncBlocks(msgs, state)
	seen := map[string]bool{}
	for _, id := range deactivated {
		seen[id] = true
	}
	if !seen["b0"] {
		t.Errorf("b0 (messages absent) must be deactivated, got %v", deactivated)
	}
	if state.BlockByID("b1").Active {
		t.Errorf("expanded block must stay inactive")
	}
	if !state.BlockByID("b2").Active {
		t.Errorf("b2 (summary present) must be re-activated")
	}
	if state.BlockByID("b4").Active {
		t.Errorf("consumed block b4 must be deactivated")
	}
	if !state.BlockByID("b5").Active {
		t.Errorf("b5 must stay active")
	}
}

// ---------------- emergency-truncate ----------------

// TestTruncateLargeToolOutputs 验证近满截断：阈值门 / 近端保护 / 标记与保留 /
// 已标记消息跳过 / 候选计数。
func TestTruncateLargeToolOutputs(t *testing.T) {
	config := DefaultConfig(10000)
	config.TruncateThreshold = 0.9
	config.PreserveRecentMessages = 3 // 近端保护窗（缺省 5 会让本例全表受保护）
	big := strings.Repeat("x", 12000) // ~3000 tokens
	msgs := []CoreMessage{
		{ID: "t1", Role: RoleTool, ContentType: ContentTypeToolResult, ToolCallID: "c1", Text: big},
		{ID: "t2", Role: RoleTool, ContentType: ContentTypeToolResult, ToolCallID: "c2", Text: big},
		{ID: "keep1", Role: RoleUser, Text: "recent"},
		{ID: "keep2", Role: RoleAssistant, Text: "recent"},
		{ID: "keep3", Role: RoleUser, Text: "recent"},
	}
	// 用量 5000/10000 = 50% < 90%：不触发
	got := TruncateLargeToolOutputs(msgs, 5000, config, true)
	if got.TruncatedCount != 0 || got.Messages[0].Text != big {
		t.Fatalf("below threshold must be no-op")
	}
	// 用量 9500/10000 = 95% ≥ 90%：按 token 降序截断，降到目标线（9000×0.9=
	// 8100）即停——单条 ~3000 token 已够 → 只截 1 条；近端 3 条保护
	got = TruncateLargeToolOutputs(msgs, 9500, config, true)
	if got.TruncatedCount != 1 {
		t.Fatalf("truncatedCount = %d, want 1 (stop at target)", got.TruncatedCount)
	}
	if got.CandidatesFound != 2 {
		t.Fatalf("candidatesFound = %d, want 2", got.CandidatesFound)
	}
	if !strings.Contains(got.Messages[0].Text, "[truncated for context space]") ||
		!strings.HasPrefix(got.Messages[0].Text, "xxxx") {
		t.Fatalf("truncated text must keep prefix + marker")
	}
	if got.Messages[1].Text != big {
		t.Fatalf("second candidate must stay verbatim (target already met)")
	}
	for i := 2; i < 5; i++ {
		if got.Messages[i].Text != msgs[i].Text {
			t.Fatalf("recent message %d must be protected", i)
		}
	}
	// 已标记消息不再入选：t1 已带标记被排除，未截断的 t2 仍是合法候选
	again := TruncateLargeToolOutputs(got.Messages, 9900, config, true)
	if again.CandidatesFound != 1 {
		t.Fatalf("marked messages must not re-qualify, want 1 candidate (t2 only), got %d", again.CandidatesFound)
	}
	for _, m := range again.Messages {
		if m.ID == "t1" && !strings.Contains(m.Text, "[truncated for context space]") {
			t.Fatalf("already-truncated t1 must be left untouched")
		}
	}
}

// ---------------- absorb ----------------

// TestAbsorbFlow 验证吸收闭环：候选判定 → 提示追加 → apply 记录 → 消息对隐藏。
func TestAbsorbFlow(t *testing.T) {
	state := CreateInitialState()
	config := DefaultConfig(100000)
	config.Absorb.Enabled = true
	config.Absorb.MinToolTokens = 10
	msgs := []CoreMessage{
		{ID: "call", Role: RoleAssistant, ContentType: ContentTypeToolCall, ToolName: "shell", ToolCallID: "c1", Text: `{"command":"ls"}`},
		{ID: "result", Role: RoleTool, ContentType: ContentTypeToolResult, ToolCallID: "c1", ToolName: "shell", Text: strings.Repeat("log ", 200)},
	}
	AssignRefs(msgs, state)
	// 候选：非 ACP 工具的 tool-result
	if !IsAbsorbCandidate(msgs[1], config) {
		t.Fatalf("tool result must be an absorb candidate")
	}
	// 提示追加（带 ref）
	applied := AppendAbsorbPrompts(msgs, state, config, 100)
	if applied.PromptedCount != 1 || !strings.Contains(applied.Messages[1].Text, AbsorbPromptMarker) ||
		!strings.Contains(applied.Messages[1].Text, "m00001") {
		t.Fatalf("absorb prompt must be appended with ref, got %+v", applied)
	}
	// apply：记录吸收
	outcome := ApplyAbsorb("m00001", "distilled: empty dir", "", applied.Messages, state, config)
	if !outcome.OK {
		t.Fatalf("absorb failed: %s", outcome.ResultText)
	}
	if len(state.Absorbed) != 1 || state.Stats.AbsorbedTokens <= 0 {
		t.Fatalf("absorb record missing: %+v", state.Absorbed)
	}
	// 下一轮：消息对隐藏
	hidden := HideAbsorbedMessages(msgs, state)
	if len(hidden) != 0 {
		t.Fatalf("absorbed pair must be hidden, got %d messages", len(hidden))
	}
	// ACP 管理工具不可吸收
	acpMsg := CoreMessage{ID: "r2", Role: RoleTool, ContentType: ContentTypeToolResult, ToolCallID: "c2", ToolName: "compress", Text: "x"}
	if IsAbsorbCandidate(acpMsg, config) {
		t.Fatalf("ACP-managed tool result must not be absorbable")
	}
}

// ---------------- hide-compress-calls ----------------

// TestHideConsumedCompressCalls 验证：被块消费的调用对隐藏、孤儿只留最新两对、
// 存活调用 args 的超长 summary 存根化。
func TestHideConsumedCompressCalls(t *testing.T) {
	state := CreateInitialState()
	// b0：由 call-1 产出（callId 回填后）
	state.Blocks = []CompressionBlock{{
		BlockID: "b0", Active: true, StartRef: "m00000", EndRef: "m00001",
		Summary: "s", CompressCallID: "call-1",
	}}
	call1 := CoreMessage{ID: "mc1", Role: RoleAssistant, ContentType: ContentTypeToolCall,
		ToolName: "compress", ToolCallID: "call-1",
		Text: `{"content":[{"startId":"m00000","endId":"m00001","summary":"` + strings.Repeat("s", 500) + `"}]}`}
	result1 := CoreMessage{ID: "mr1", Role: RoleTool, ContentType: ContentTypeToolResult, ToolCallID: "call-1", Text: `{"ok":true,"blocksCreated":1}`}
	// 孤儿调用三对（无块引用）
	orphan := func(n int) []CoreMessage {
		return []CoreMessage{
			{ID: fmt.Sprintf("mo%d", n), Role: RoleAssistant, ContentType: ContentTypeToolCall,
				ToolName: "compress", ToolCallID: fmt.Sprintf("orphan-%d", n), Text: `{"content":[{"startId":"m00000","endId":"m00000","summary":"x"}]}`},
			{ID: fmt.Sprintf("mro%d", n), Role: RoleTool, ContentType: ContentTypeToolResult,
				ToolCallID: fmt.Sprintf("orphan-%d", n), Text: `{"ok":true,"blocksCreated":0}`},
		}
	}
	msgs := append([]CoreMessage{call1, result1}, append(orphan(1), append(orphan(2), orphan(3)...)...)...)
	got := HideConsumedCompressCalls(state, msgs)
	// 孤儿 3 对 → 隐藏最旧 1 对（保留最新 2 对）
	if got.Hidden != 2 {
		t.Fatalf("hidden = %d, want 2 (oldest orphan pair)", got.Hidden)
	}
	for _, m := range got.Messages {
		if m.ToolCallID == "orphan-1" {
			t.Fatalf("oldest orphan must be hidden")
		}
	}
	// 存活调用（call-1）的 summary 被存根化
	found := false
	for _, m := range got.Messages {
		if m.ToolCallID == "call-1" && m.ContentType == ContentTypeToolCall {
			found = true
			if strings.Contains(m.Text, strings.Repeat("s", 500)) {
				t.Fatalf("live call summary must be stubbed")
			}
			if !strings.Contains(m.Text, `"startId":"m00000"`) {
				t.Fatalf("live call range must be preserved")
			}
		}
	}
	if !found {
		t.Fatalf("live compress call must be kept")
	}
}

// ---------------- recommend（mergeRangesToThreshold，#309） ----------------

// TestMergeRangesToThreshold 验证：逐批越过最小门 / 尾部并入前批 /
// 整段低于门不出推荐。
func TestMergeRangesToThreshold(t *testing.T) {
	r := func(start, end string, chars int) CompressibleRange {
		return CompressibleRange{StartRef: start, EndRef: end, Count: 2, Tokens: chars / 4, Chars: chars}
	}
	// 3000 + 3000 = 6000 ≥ 5000：合并成一批；4000 尾部并入前批
	merged := MergeRangesToThreshold([]CompressibleRange{
		r("m00000", "m00001", 3000), r("m00002", "m00003", 3000), r("m00004", "m00005", 4000),
	}, 5000)
	if len(merged) != 1 || merged[0].StartRef != "m00000" || merged[0].EndRef != "m00005" || merged[0].Chars != 10000 {
		t.Fatalf("batch + tail fold mismatch: %+v", merged)
	}
	// 单段 3000 < 5000：无前批可并 → 不出推荐（#847）
	merged = MergeRangesToThreshold([]CompressibleRange{r("m00000", "m00001", 3000)}, 5000)
	if len(merged) != 0 {
		t.Fatalf("below-gate-only input must emit nothing, got %+v", merged)
	}
	// minChars<=0：原样返回
	ranges := []CompressibleRange{r("m00000", "m00001", 1)}
	if got := MergeRangesToThreshold(ranges, 0); len(got) != 1 {
		t.Fatalf("disabled gate must pass through")
	}
}

// ---------------- prune pair-safe anchor（v0.0.71 2f60bfb） ----------------

// TestPairSafeAnchorIndex 验证：assistant 连续 run 内部不落锚（回退 run 起点）/
// 并行工具突发内不落锚（移过 result）。
func TestPairSafeAnchorIndex(t *testing.T) {
	msgs := []CoreMessage{
		{ID: "m0", Role: RoleUser, ContentType: ContentTypeText, Text: "task"},
		{ID: "m1", Role: RoleAssistant, ContentType: ContentTypeReasoning, Text: "think"},
		{ID: "m2", Role: RoleAssistant, ContentType: ContentTypeText, Text: "answer"},
	}
	// 锚点落在 m2（assistant run 内部）→ 回退到 run 起点 m1
	if got := pairSafeAnchorIndex(msgs, 2); got != 1 {
		t.Fatalf("anchor inside assistant run must move to run start, got %d", got)
	}
	toolMsgs := []CoreMessage{
		{ID: "u", Role: RoleUser, ContentType: ContentTypeText, Text: "go"},
		{ID: "c", Role: RoleAssistant, ContentType: ContentTypeToolCall, ToolCallID: "c1", ToolName: "shell", Text: "{}"},
		{ID: "r", Role: RoleTool, ContentType: ContentTypeToolResult, ToolCallID: "c1", Text: "out"},
		{ID: "n", Role: RoleUser, ContentType: ContentTypeText, Text: "next"},
	}
	// 锚点落在 call 与 result 之间（index 2 指向 result 前的位置不可行）→ 移过 result
	if got := pairSafeAnchorIndex(toolMsgs, 2); got != 3 {
		t.Fatalf("anchor splitting tool pair must move past result, got %d", got)
	}
}

// ---------------- rebuild（fork/恢复 state 重建） ----------------

// TestRebuildStateFromMessages 验证：压缩事件流重放后块覆盖关系与 callId 还原。
func TestRebuildStateFromMessages(t *testing.T) {
	config := DefaultConfig(100000)
	// 第一段：正常会话 → compress → 后续消息
	original := []CoreMessage{
		{ID: "a", Role: RoleUser, Text: strings.Repeat("x", 400)},
		{ID: "b", Role: RoleAssistant, Text: strings.Repeat("y", 400)},
		{ID: "mc1", Role: RoleAssistant, ContentType: ContentTypeToolCall, ToolName: "compress", ToolCallID: "call-1",
			Text: `{"content":[{"startId":"m00000","endId":"m00001","summary":"combined summary","topic":"t"}]}`},
		{ID: "mr1", Role: RoleTool, ContentType: ContentTypeToolResult, ToolCallID: "call-1",
			Text: `{"ok":true,"blocksCreated":1,"tokensCompressed":200}`},
		{ID: "c", Role: RoleUser, Text: strings.Repeat("z", 400)},
	}
	// 现场压缩
	liveState := CreateInitialState()
	AssignRefs(original, liveState)
	ranges := ParseCompressArgs(original[2].Text)
	if len(ranges) != 1 {
		t.Fatalf("parse args: %v", ranges)
	}
	if _, result := ApplyCompression(ranges, original, liveState, config); result.BlocksCreated != 1 {
		t.Fatalf("live compress failed: %+v", result)
	}
	// 模拟 fork/恢复：state 丢失 → 从消息列表重放
	rebuilt, n := RebuildStateFromMessages(original, config)
	if n != 1 {
		t.Fatalf("replayed blocks = %d, want 1", n)
	}
	if len(rebuilt.Blocks) != 1 {
		t.Fatalf("rebuilt blocks = %d, want 1", len(rebuilt.Blocks))
	}
	liveBlock := liveState.Blocks[0]
	rebuiltBlock := rebuilt.Blocks[0]
	if rebuiltBlock.Summary != "combined summary" || rebuiltBlock.Topic != "t" {
		t.Fatalf("rebuilt block content mismatch: %+v", rebuiltBlock)
	}
	if rebuiltBlock.StartRef != liveBlock.StartRef || rebuiltBlock.EndRef != liveBlock.EndRef {
		t.Fatalf("rebuilt range mismatch: %s..%s vs %s..%s",
			rebuiltBlock.StartRef, rebuiltBlock.EndRef, liveBlock.StartRef, liveBlock.EndRef)
	}
	if rebuiltBlock.CompressCallID != "call-1" {
		t.Fatalf("rebuilt block must carry callId, got %q", rebuiltBlock.CompressCallID)
	}
	if rebuilt.MessageRefs.ByRaw["c"] != liveState.MessageRefs.ByRaw["c"] {
		t.Fatalf("ref map must reproduce identically")
	}
	// HasACPStateTrace 探测
	if !HasACPStateTrace(original) {
		t.Fatalf("trace must be detected")
	}
	plain := []CoreMessage{{ID: "a", Role: RoleUser, Text: "hi"}}
	if HasACPStateTrace(plain) {
		t.Fatalf("plain history must not carry trace")
	}
}
