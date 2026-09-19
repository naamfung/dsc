package session

import (
	"testing"
)

// TestSystemMessageAppendBecomeNode0 校验：首条 system/message 以 append 入位后
// 占据 surface node 0，DeriveMessages 派生 msgs[0] 为 system 消息。
func TestSystemMessageAppendBecomeNode0(t *testing.T) {
	s := New()
	s.Append(SystemMessage, &SystemMessageData{Turn: 1, Step: 0, Content: "你是一个助手"}, &SurfaceOp{Op: SurfaceAppend})
	s.Append(UserMessage, &UserMessageData{Content: "你好"}, &SurfaceOp{Op: SurfaceAppend})

	nodes := s.SurfaceNodes()
	if len(nodes) != 2 {
		t.Fatalf("surface nodes = %d, want 2", len(nodes))
	}
	if nodes[0] != 0 {
		t.Errorf("node 0 seq = %d, want 0 (system/message)", nodes[0])
	}

	msgs := s.DeriveMessages("")
	if len(msgs) != 2 {
		t.Fatalf("derived msgs = %d, want 2", len(msgs))
	}
	if msgs[0].Role != "system" || msgs[0].Content != "你是一个助手" {
		t.Errorf("msgs[0] = %+v, want system/你是一个助手", msgs[0])
	}
	if msgs[1].Role != "user" || msgs[1].Content != "你好" {
		t.Errorf("msgs[1] = %+v, want user/你好", msgs[1])
	}
}

// TestSystemMessageReplaceNode0 校验：prompt 变化时 replace node 0
// （范围恰好 [旧 seq, 旧 seq]），新内容成为 surface node 0。
func TestSystemMessageReplaceNode0(t *testing.T) {
	s := New()
	s.Append(SystemMessage, &SystemMessageData{Turn: 1, Step: 0, Content: "v1"}, &SurfaceOp{Op: SurfaceAppend})
	s.Append(UserMessage, &UserMessageData{Content: "hi"}, &SurfaceOp{Op: SurfaceAppend})

	// 渲染出 v2，经 ProjectSystemPrompt 决策
	op, target := s.ProjectSystemPrompt("v2")
	if op == nil {
		t.Fatal("ProjectSystemPrompt 应返回 replace op（内容变化）")
	}
	if op.Op != SurfaceReplace {
		t.Errorf("op = %q, want replace", op.Op)
	}
	if op.Start != 0 || op.End != 0 {
		t.Errorf("replace range = [%d,%d], want [0,0] (恰好覆盖 node 0)", op.Start, op.End)
	}
	if target != 0 {
		t.Errorf("target = %d, want 0", target)
	}

	s.Append(SystemMessage, &SystemMessageData{Turn: 2, Step: 0, Content: "v2"}, op)

	nodes := s.SurfaceNodes()
	if len(nodes) != 2 {
		t.Fatalf("after replace, surface nodes = %d, want 2", len(nodes))
	}
	if nodes[0] != 2 {
		t.Errorf("node 0 seq = %d, want 2 (new system/message)", nodes[0])
	}
	if s.ReplaceGeneration() != 1 {
		t.Errorf("replaceGen = %d, want 1", s.ReplaceGeneration())
	}

	msgs := s.DeriveMessages("")
	if msgs[0].Content != "v2" {
		t.Errorf("msgs[0] content = %q, want v2", msgs[0].Content)
	}
}

// TestProjectSystemPromptNoopOnIdenticalContent 校验：渲染内容与 surface
// 当前 system 节点相同 → ProjectSystemPrompt 返回 nil（no-op，无需提交）。
func TestProjectSystemPromptNoopOnIdenticalContent(t *testing.T) {
	s := New()
	s.Append(SystemMessage, &SystemMessageData{Turn: 1, Step: 0, Content: "same"}, &SurfaceOp{Op: SurfaceAppend})

	op, target := s.ProjectSystemPrompt("same")
	if op != nil {
		t.Fatalf("ProjectSystemPrompt on identical content should return nil, got %+v (target=%d)", op, target)
	}
	if target != 0 {
		t.Errorf("target = %d, want 0 (node 0 seq)", target)
	}
}

// TestProjectSystemPromptAppendOnEmptySurface 校验：surface 完全空时
// ProjectSystemPrompt 返回 append op。
func TestProjectSystemPromptAppendOnEmptySurface(t *testing.T) {
	s := New()
	op, target := s.ProjectSystemPrompt("first prompt")
	if op == nil {
		t.Fatal("ProjectSystemPrompt on empty surface should return append op")
	}
	if op.Op != SurfaceAppend {
		t.Errorf("op = %q, want append", op.Op)
	}
	if target != -1 {
		t.Errorf("target = %d, want -1 (no existing node)", target)
	}
}

// TestProjectSystemPromptEmptyContentPlaceholder 校验：首次渲染空 prompt
// 也提交（作为占位 node 0），后续非空 prompt 才能 replace node 0。
// 这是 DSH v3 的关键不变量：先空后非空仍走 replace 路径而非追加到 user 之后。
func TestProjectSystemPromptEmptyContentPlaceholder(t *testing.T) {
	s := New()
	// 首次渲染空 prompt → append 占位
	op, _ := s.ProjectSystemPrompt("")
	if op == nil || op.Op != SurfaceAppend {
		t.Fatalf("empty content first render should append, got %+v", op)
	}
	s.Append(SystemMessage, &SystemMessageData{Turn: 1, Step: 0, Content: ""}, op)

	// 后续非空 prompt → replace node 0
	op2, target := s.ProjectSystemPrompt("now non-empty")
	if op2 == nil || op2.Op != SurfaceReplace {
		t.Fatalf("non-empty after empty should replace, got %+v", op2)
	}
	if target != 0 {
		t.Errorf("target = %d, want 0", target)
	}
	s.Append(SystemMessage, &SystemMessageData{Turn: 2, Step: 0, Content: "now non-empty"}, op2)

	msgs := s.DeriveMessages("")
	if len(msgs) != 1 {
		t.Fatalf("derived msgs = %d, want 1 (just the non-empty system)", len(msgs))
	}
	if msgs[0].Role != "system" || msgs[0].Content != "now non-empty" {
		t.Errorf("msgs[0] = %+v, want system/now non-empty", msgs[0])
	}
}

// TestEmptySystemMessageProjectsToNil 校验：空 content 的 system/message
// 投影为 nil（不派生模型消息），但保留 surface 位置。
func TestEmptySystemMessageProjectsToNil(t *testing.T) {
	s := New()
	s.Append(SystemMessage, &SystemMessageData{Turn: 1, Step: 0, Content: ""}, &SurfaceOp{Op: SurfaceAppend})
	s.Append(UserMessage, &UserMessageData{Content: "hi"}, &SurfaceOp{Op: SurfaceAppend})

	// surface 2 节点，但空 system 投影为 nil → 派生消息只有 1 条（user）
	msgs := s.DeriveMessages("")
	if len(msgs) != 1 {
		t.Fatalf("derived msgs = %d, want 1 (empty system projects to nil)", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "hi" {
		t.Errorf("msgs[0] = %+v, want user/hi", msgs[0])
	}
}

// TestHeadInvariantRejectsNonSystemReplaceNode0 校验：覆盖 surface node 0
// （system/message）的 replace 必须由 system/message 触发且范围恰好覆盖 node 0。
// 其他事件类型触发覆盖 node 0 → panic。
func TestHeadInvariantRejectsNonSystemReplaceNode0(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when non-system/message event replaces node 0")
		}
	}()
	s := New()
	s.Append(SystemMessage, &SystemMessageData{Content: "v1"}, &SurfaceOp{Op: SurfaceAppend})
	s.Append(UserMessage, &UserMessageData{Content: "hi"}, &SurfaceOp{Op: SurfaceAppend})

	// 试图用 user/message 替换 node 0（system/message）→ 应 panic
	s.Append(UserMessage, &UserMessageData{Content: "hijack"}, &SurfaceOp{Op: SurfaceReplace, Start: 0, End: 0})
}

// TestHeadInvariantRejectsSystemReplaceOutOfRange 校验：system/message 的
// replace 范围不是恰好 [node 0, node 0] → panic。
func TestHeadInvariantRejectsSystemReplaceOutOfRange(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when system/message replace range doesn't cover exactly node 0")
		}
	}()
	s := New()
	s.Append(SystemMessage, &SystemMessageData{Content: "v1"}, &SurfaceOp{Op: SurfaceAppend})
	s.Append(UserMessage, &UserMessageData{Content: "hi"}, &SurfaceOp{Op: SurfaceAppend})

	// replace 范围 [0, 1] 覆盖 node 0 + node 1 → 应 panic
	s.Append(SystemMessage, &SystemMessageData{Content: "v2"}, &SurfaceOp{Op: SurfaceReplace, Start: 0, End: 1})
}

// TestCompactionCanShadowNonHeadSystem 校验：system/message 在 surface 后位
// （非 node 0）时不享受 head 不变量保护，可被 compaction replace 覆盖。
// （对齐 DSH："System nodes at later positions carry no such protection"）
// 当前 DSC 不实现 mid-surface system 节点，但若手工构造，replace 不应 panic。
func TestCompactionCanShadowNonHeadSystem(t *testing.T) {
	s := New()
	// 构造：node 0 = system v1，node 1 = user，node 2 = system v2（非 head）
	s.Append(SystemMessage, &SystemMessageData{Content: "head"}, &SurfaceOp{Op: SurfaceAppend})
	s.Append(UserMessage, &UserMessageData{Content: "u1"}, &SurfaceOp{Op: SurfaceAppend})
	// 第二条 system 直接 append 到 node 2（实际场景是 replace node 0 后旧 node
	// 影子仍在 surface 之外，此处手工构造以测试后位 system 不被 head 保护）
	s.Append(SystemMessage, &SystemMessageData{Content: "tail"}, &SurfaceOp{Op: SurfaceAppend})

	// compaction 覆盖 [1, 2]（user + tail system）应允许（不 panic）
	s.Append(CompactionSummary, &CompactionSummaryData{Content: "summary"},
		&SurfaceOp{Op: SurfaceReplace, Start: 1, End: 2})

	nodes := s.SurfaceNodes()
	if len(nodes) != 2 {
		t.Fatalf("after compaction, surface = %d nodes, want 2 (head + summary)", len(nodes))
	}
}
