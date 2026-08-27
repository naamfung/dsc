package core

import (
	"os"
	"path/filepath"
	"testing"

	"dsc/session"
)

func newSessionMgr(t *testing.T) *Manager {
	t.Helper()
	return NewManager(&ManagerConfig{ExecDir: t.TempDir()})
}

func TestManagerCreateSession(t *testing.T) {
	m := newSessionMgr(t)
	id, err := m.CreateSession()
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if id == "" || id == "default" {
		t.Fatalf("id = %q, want an allocated session id", id)
	}
	// 空会话不落盘（有事件才写文件），但分配的 id 应可用于后续 Ensure/Save
	st, _ := session.NewStore(filepath.Join(m.config.ExecDir, "sessions"))
	sess, _ := st.Ensure(id)
	sess.Append(session.UserMessage, &session.UserMessageData{Content: "x", Source: "user"},
		&session.SurfaceOp{Op: session.SurfaceAppend})
	if err := st.Save(sess); err != nil {
		t.Fatalf("save created session: %v", err)
	}
	path := filepath.Join(m.config.ExecDir, "sessions", id+".jsonl")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("saved session file missing: %v", err)
	}
}

func TestManagerCreateAndDeleteSession(t *testing.T) {
	m := newSessionMgr(t)
	id, err := m.CreateSession()
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// 写入一条事件再删除，验证列表随后不含它
	st, _ := session.NewStore(filepath.Join(m.config.ExecDir, "sessions"))
	sess, _ := st.Ensure(id)
	sess.Append(session.UserMessage, &session.UserMessageData{Content: "x", Source: "user"},
		&session.SurfaceOp{Op: session.SurfaceAppend})
	_ = st.Save(sess)

	if err := m.DeleteSession(id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	summaries, err := m.ListSessions()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, s := range summaries {
		if s.ID == id {
			t.Fatalf("session %s should be deleted", id)
		}
	}
}

func TestManagerDeleteMissingSessionIdempotent(t *testing.T) {
	m := newSessionMgr(t)
	if err := m.DeleteSession("session-999"); err != nil {
		t.Fatalf("delete missing should be idempotent: %v", err)
	}
}
