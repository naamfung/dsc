package session

import (
	"testing"
)

func TestStoreCreateLoadSave(t *testing.T) {
	st, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	sess, err := st.Create()
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if sess.id == "" {
		t.Fatal("created session should have an id")
	}
	appendSurface(t, sess, UserMessage, &UserMessageData{Content: "hello world", Source: "user"})
	appendSurface(t, sess, AssistantMessage, &AssistantMessageData{Content: "hi"})
	if err := st.Save(sess); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := st.Load(sess.id)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded == nil {
		t.Fatalf("load %s returned nil", sess.id)
	}
	if loaded.Len() != sess.Len() {
		t.Fatalf("loaded len = %d, want %d", loaded.Len(), sess.Len())
	}
	msgs := loaded.DeriveMessages("")
	if len(msgs) != 2 || msgs[0].Content != "hello world" {
		t.Fatalf("derived = %+v, want 2 messages with first 'hello world'", msgs)
	}
}

func TestStoreCreateIncrements(t *testing.T) {
	st, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	a, _ := st.Create()
	b, _ := st.Create()
	if a.id == b.id {
		t.Fatalf("ids should differ, got %s and %s", a.id, b.id)
	}
	// 编号递增：session-1, session-2
	if a.id != "session-1" || b.id != "session-2" {
		t.Fatalf("ids = %s, %s, want session-1, session-2", a.id, b.id)
	}
}

func TestStoreList(t *testing.T) {
	st, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	older, _ := st.Create()
	appendSurface(t, older, UserMessage, &UserMessageData{Content: "old prompt here", Source: "user"})
	appendSurface(t, older, AssistantMessage, &AssistantMessageData{Content: "a"})
	_ = st.Save(older)

	newer, _ := st.Create()
	appendSurface(t, newer, UserMessage, &UserMessageData{Content: "new prompt", Source: "user"})
	_ = st.Save(newer)

	infos, err := st.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(infos) != 2 {
		t.Fatalf("listed %d sessions, want 2", len(infos))
	}
	// 按最后活动时间倒序：newer 在前
	if infos[0].ID != newer.id || infos[1].ID != older.id {
		t.Fatalf("order = [%s, %s], want [%s, %s]", infos[0].ID, infos[1].ID, newer.id, older.id)
	}
	if infos[1].Events != 2 {
		t.Fatalf("older events = %d, want 2", infos[1].Events)
	}
	if infos[1].Preview != "old prompt here" {
		t.Fatalf("older preview = %q, want 'old prompt here'", infos[1].Preview)
	}
	if infos[0].Preview != "new prompt" {
		t.Fatalf("newer preview = %q, want 'new prompt'", infos[0].Preview)
	}
}

func TestStoreListPreviewTruncated(t *testing.T) {
	st, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	sess, _ := st.Create()
	long := "这是一个非常长的用户消息用于测试预览截断行为是否正确生效的完整句子"
	appendSurface(t, sess, UserMessage, &UserMessageData{Content: long, Source: "user"})
	_ = st.Save(sess)

	infos, err := st.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("listed %d sessions, want 1", len(infos))
	}
	if len([]rune(infos[0].Preview)) > 41 {
		t.Fatalf("preview not truncated: %q", infos[0].Preview)
	}
}

func TestStoreDelete(t *testing.T) {
	st, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	sess, _ := st.Create()
	appendSurface(t, sess, UserMessage, &UserMessageData{Content: "x", Source: "user"})
	_ = st.Save(sess)

	if err := st.Delete(sess.id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	loaded, err := st.Load(sess.id)
	if err != nil {
		t.Fatalf("load after delete: %v", err)
	}
	if loaded != nil {
		t.Fatalf("session %s should be gone", sess.id)
	}
	// 重复删除幂等
	if err := st.Delete(sess.id); err != nil {
		t.Fatalf("double delete should be idempotent: %v", err)
	}
}

// TestStoreListReflectsDeletion 校验删除后列表实时更新（同一 Store 实例，
// 无缓存：List 每次扫描目录，Delete 立即移除文件）。
func TestStoreListReflectsDeletion(t *testing.T) {
	st, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	a, _ := st.Create()
	appendSurface(t, a, UserMessage, &UserMessageData{Content: "a", Source: "user"})
	_ = st.Save(a)
	b, _ := st.Create()
	appendSurface(t, b, UserMessage, &UserMessageData{Content: "b", Source: "user"})
	_ = st.Save(b)

	infos, err := st.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(infos) != 2 {
		t.Fatalf("listed %d sessions, want 2", len(infos))
	}

	// 删除 a 后立即 List：同一实例应实时反映只剩 b
	if err := st.Delete(a.id); err != nil {
		t.Fatalf("delete %s: %v", a.id, err)
	}
	infos, err = st.List()
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(infos) != 1 || infos[0].ID != b.id {
		t.Fatalf("after delete, list = %+v, want only %s", infos, b.id)
	}
}

func TestStoreLoadMissing(t *testing.T) {
	st, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	sess, err := st.Load("session-99")
	if err != nil {
		t.Fatalf("load missing: %v", err)
	}
	if sess != nil {
		t.Fatalf("expected nil for missing session, got %+v", sess)
	}
}
