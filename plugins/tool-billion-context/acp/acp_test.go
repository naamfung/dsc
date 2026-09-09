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

        // 渲染：a, b 应被替换为 summary，c 保留
        rendered := RenderMessages(msgs, state, config, true)
        if len(rendered) < 2 {
                t.Fatalf("rendered messages = %d, want ≥2 (summary + c)", len(rendered))
        }
        // 第一条应是块 summary（ID 形如 block:b0）
        if rendered[0].ID != "block:b0" {
                t.Errorf("first rendered msg ID = %q, want block:b0", rendered[0].ID)
        }
        // 最后一条应是 c（保留区）
        last := rendered[len(rendered)-1]
        if last.ID != "c" {
                t.Errorf("last rendered msg ID = %q, want c", last.ID)
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
