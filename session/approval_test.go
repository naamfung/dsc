package session

import (
	"path/filepath"
	"testing"
)

func TestFoldApprovalPolicy(t *testing.T) {
	s := New()
	if got := FoldApprovalPolicy(s.Events()); got != "" {
		t.Fatalf("empty log fold = %q, want empty", got)
	}
	s.Append(ApprovalPolicy, &ApprovalPolicyData{Policy: "never"}, nil)
	s.Append(ApprovalPolicy, &ApprovalPolicyData{Policy: "ask"}, nil)
	s.Append(ApprovalPolicy, &ApprovalPolicyData{Policy: "never"}, nil)
	if got := FoldApprovalPolicy(s.Events()); got != "never" {
		t.Fatalf("last approval/policy should win, got %q", got)
	}
}

// TestApprovalEventsPersistRoundtrip 校验审批审计与策略事件经 persist 落盘/回放保真。
func TestApprovalEventsPersistRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	s := New()
	s.Append(ApprovalAsked, &ApprovalAskedData{Tool: "str_replace_editor", Mode: "workspace-write", Reason: "need to edit a file"}, nil)
	s.Append(ApprovalDecided, &ApprovalDecidedData{Tool: "str_replace_editor", Mode: "workspace-write", Outcome: "allowed-once"}, nil)
	s.Append(ApprovalPolicy, &ApprovalPolicyData{Policy: "never"}, nil)
	if err := s.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := FoldApprovalPolicy(loaded.Events()); got != "never" {
		t.Fatalf("fold after reload = %q, want never", got)
	}
	events := loaded.Events()
	if len(events) != 3 {
		t.Fatalf("events = %d, want 3", len(events))
	}
	if d, ok := events[0].Data.(*ApprovalAskedData); !ok || d.Tool != "str_replace_editor" || d.Mode != "workspace-write" {
		t.Fatalf("asked data = %+v", events[0].Data)
	}
	if d, ok := events[2].Data.(*ApprovalPolicyData); !ok || d.Policy != "never" {
		t.Fatalf("policy data = %+v", events[2].Data)
	}
}
