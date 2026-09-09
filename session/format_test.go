package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFormatVersion(t *testing.T) {
	if SessionFormatVersion != 2 {
		t.Errorf("SessionFormatVersion = %d, want 2", SessionFormatVersion)
	}
}

func TestMigrateV0ToV1(t *testing.T) {
	events := []*Event{{Seq: 0, Type: TurnStart}}
	migrated, err := MigrateToCurrent(0, events)
	if err != nil {
		t.Fatalf("MigrateToCurrent: %v", err)
	}
	if len(migrated) != 1 {
		t.Fatalf("expected 1 event, got %d", len(migrated))
	}
}

func TestMigrateV1ToV2(t *testing.T) {
	// v1→v2 确保所有事件有 Seq
	events := []*Event{
		{Seq: 0, Type: TurnStart},
		{Seq: 0, Type: UserMessage},
	}
	migrated, err := MigrateToCurrent(1, events)
	if err != nil {
		t.Fatalf("MigrateToCurrent: %v", err)
	}
	if migrated[0].Seq != 1 {
		t.Errorf("event 0 Seq = %d, want 1", migrated[0].Seq)
	}
	if migrated[1].Seq != 2 {
		t.Errorf("event 1 Seq = %d, want 2", migrated[1].Seq)
	}
}

func TestMigrateAlreadyCurrent(t *testing.T) {
	events := []*Event{{Seq: 1, Type: TurnStart}}
	migrated, err := MigrateToCurrent(SessionFormatVersion, events)
	if err != nil {
		t.Fatalf("MigrateToCurrent: %v", err)
	}
	if len(migrated) != 1 {
		t.Error("should return same events")
	}
}

func TestSaveLoadWithFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	events := []*Event{
		{Seq: 1, Type: TurnStart, Data: &TurnData{Turn: 1}},
		{Seq: 2, Type: UserMessage, Data: &UserMessageData{Content: "hello"}},
	}

	if err := SaveWithFormat(path, events); err != nil {
		t.Fatalf("SaveWithFormat: %v", err)
	}

	// 检测版本
	version, err := DetectFormatVersion(path)
	if err != nil {
		t.Fatalf("DetectFormatVersion: %v", err)
	}
	if version != SessionFormatVersion {
		t.Errorf("detected version = %d, want %d", version, SessionFormatVersion)
	}

	// 加载
	loaded, err := LoadWithFormat(path)
	if err != nil {
		t.Fatalf("LoadWithFormat: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("loaded %d events, want 2", len(loaded))
	}
	if loaded[0].Type != TurnStart {
		t.Errorf("event 0 type = %q, want TurnStart", loaded[0].Type)
	}
}

func TestDetectFormatV0(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "v0.jsonl")
	// 旧格式：无版本头，直接是 JSONL
	os.WriteFile(path, []byte(`{"seq":1,"type":"turn_start","data":{"turn":1}}
{"seq":2,"type":"user/message","data":{"content":"hello"}}
`), 0644)

	version, err := DetectFormatVersion(path)
	if err != nil {
		t.Fatalf("DetectFormatVersion: %v", err)
	}
	if version != 0 {
		t.Errorf("v0 file version = %d, want 0", version)
	}

	// 加载并迁移
	events, err := LoadWithFormat(path)
	if err != nil {
		t.Fatalf("LoadWithFormat: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("loaded %d events, want 2", len(events))
	}
	// v1→v2 迁移后应有 Seq
	if events[0].Seq != 1 {
		t.Errorf("event 0 Seq = %d, want 1", events[0].Seq)
	}
}
