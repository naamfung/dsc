package session

import (
        "os"
        "path/filepath"
        "testing"
)

func TestFormatVersion(t *testing.T) {
        // v3：新增 llm/attempt 诊断事件类型 + system/message surface node 0 不变量
        // （system prompt 由事件日志派生，不再由调用方参数前置）。
        if SessionFormatVersion != 3 {
                t.Errorf("SessionFormatVersion = %d, want 3", SessionFormatVersion)
        }
}

// TestMigrateV0ToCurrent v0→v3 走完整迁移链：v0→v1（无内容变更）→v1→v2
// （Seq 重排）→v2→v3（首部插入空 system/message 占位）。最终结果应比输入
// 多 1 个事件（system/message 占位），且原事件保留不变。
func TestMigrateV0ToCurrent(t *testing.T) {
        events := []*Event{{Seq: 0, Type: TurnStart}}
        migrated, err := MigrateToCurrent(0, events)
        if err != nil {
                t.Fatalf("MigrateToCurrent: %v", err)
        }
        if len(migrated) != 2 {
                t.Fatalf("v0→v3 应 2 个事件（1 system/message 占位 + 1 原始），got %d", len(migrated))
        }
        if migrated[0].Type != SystemMessage {
                t.Errorf("事件 0 应 SystemMessage 占位，got %q", migrated[0].Type)
        }
        if migrated[1].Type != TurnStart {
                t.Errorf("原 TurnStart 应保留，got %q", migrated[1].Type)
        }
}

// TestMigrateV1ToCurrent v1→v3 走 v1→v2（Seq 重排）+ v2→v3（首部插入
// system/message 占位）。原事件 Seq 经 v1→v2 后被赋值为序号+1，经 v2→v3
// 占位插入后整体 +1。最终占位 Seq=0，原事件 Seq=2, 3。
func TestMigrateV1ToCurrent(t *testing.T) {
        events := []*Event{
                {Seq: 0, Type: TurnStart},
                {Seq: 0, Type: UserMessage},
        }
        migrated, err := MigrateToCurrent(1, events)
        if err != nil {
                t.Fatalf("MigrateToCurrent: %v", err)
        }
        if len(migrated) != 3 {
                t.Fatalf("v1→v3 应 3 个事件（1 占位 + 2 原始），got %d", len(migrated))
        }
        if migrated[0].Type != SystemMessage || migrated[0].Seq != 0 {
                t.Errorf("事件 0 应 SystemMessage 占位 Seq=0，got %+v", migrated[0])
        }
        if migrated[1].Seq != 2 {
                t.Errorf("原事件 1 Seq 应 2（v1→v2 给 +1，v2→v3 又 +1），got %d", migrated[1].Seq)
        }
        if migrated[2].Seq != 3 {
                t.Errorf("原事件 2 Seq 应 3，got %d", migrated[2].Seq)
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
        // 旧格式：无版本头，直接是 JSONL（事件类型用 / 分隔，对齐 EventType 常量）
        os.WriteFile(path, []byte(`{"seq":1,"type":"turn/start","data":{"turn":1}}
{"seq":2,"type":"user/message","data":{"content":"hello"}}
`), 0644)

        version, err := DetectFormatVersion(path)
        if err != nil {
                t.Fatalf("DetectFormatVersion: %v", err)
        }
        if version != 0 {
                t.Errorf("v0 file version = %d, want 0", version)
        }

        // 加载并迁移：v0→v1（无内容变更）→v1→v2（Seq 重排）→v2→v3（首部插入
        // 空 system/message 占位）。最终 3 个事件：占位 + TurnStart + UserMessage
        events, err := LoadWithFormat(path)
        if err != nil {
                t.Fatalf("LoadWithFormat: %v", err)
        }
        if len(events) != 3 {
                t.Fatalf("loaded %d events, want 3 (1 system/message placeholder + 2 original)", len(events))
        }
        if events[0].Type != SystemMessage {
                t.Errorf("event 0 should be SystemMessage placeholder, got %q", events[0].Type)
        }
        // 原 TurnStart 经 v1→v2 Seq 重排为 1，经 v2→v3 占位插入后 +1 = 2
        if events[1].Seq != 2 || events[1].Type != TurnStart {
                t.Errorf("event 1 should be TurnStart Seq=2, got %+v", events[1])
        }
        if events[2].Seq != 3 || events[2].Type != UserMessage {
                t.Errorf("event 2 should be UserMessage Seq=3, got %+v", events[2])
        }
}
