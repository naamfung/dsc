package acp

import (
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
//      [summary1] [summary2] ... [最近新增消息]
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
        if len(results) != 1 {
                t.Fatalf("results = %d, want 1 (only active b0 matches)", len(results))
        }
        if results[0].BlockID != "b0" {
                t.Errorf("first result blockID = %q, want b0", results[0].BlockID)
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
        // 渲染后的消息应含 <acp> 标签
        foundTag := false
        for _, m := range result.Messages {
                if m.ID == "a" && containsStr(m.Text, "<acp") {
                        foundTag = true
                }
        }
        if !foundTag {
                t.Errorf("rendered messages should contain <acp> tags")
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

        // 压缩 a..d（切在 b 与 c 之间：b 被压缩，c 留下成孤儿 tool-result；
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
