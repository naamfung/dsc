package main

import (
	"encoding/json"
	"testing"

	"dsc-sdk"
)

// TestSSHConnectView ssh_connect 视图：成功绿色徽标 + session_id；失败红色 + error。
func TestSSHConnectView(t *testing.T) {
	view, _ := sshConnectView(`{"success":true,"session_id":"ssh-123"}`)
	var v dsc.View
	if err := json.Unmarshal(view, &v); err != nil {
		t.Fatalf("视图 JSON 非法: %v", err)
	}
	if v.Kind != "card" || v.Title != "SSH" || v.Badge == nil || v.Badge.Text != "connected" || v.Badge.Tone != "green" {
		t.Fatalf("view 头 = %+v", v)
	}
	if len(v.Fields) != 1 || v.Fields[0].Value != "ssh-123" {
		t.Fatalf("fields = %+v", v.Fields)
	}

	view, _ = sshConnectView(`{"success":false,"session_id":"","error":"dial failed"}`)
	if err := json.Unmarshal(view, &v); err != nil {
		t.Fatalf("视图 JSON 非法: %v", err)
	}
	if v.Badge.Text != "failed" || v.Badge.Tone != "red" || v.Fields[1].Value != "dial failed" || v.Fields[1].Tone != "red" {
		t.Fatalf("failed view = %+v", v)
	}
}

// TestSSHExecView ssh_exec 视图：纯文本 + 退出码徽标；失败时正文取 error。
func TestSSHExecView(t *testing.T) {
	view, _ := sshExecView(`{"success":true,"output":"hello\nworld","exit_code":0}`)
	var v dsc.View
	if err := json.Unmarshal(view, &v); err != nil {
		t.Fatalf("视图 JSON 非法: %v", err)
	}
	if v.Kind != "plain" || v.Title != "SSH" || v.Badge == nil || v.Badge.Text != "exit 0" || v.Badge.Tone != "gray" {
		t.Fatalf("view 头 = %+v", v)
	}
	if v.Body != "hello\nworld" {
		t.Fatalf("body = %q", v.Body)
	}

	view, _ = sshExecView(`{"success":false,"output":"","exit_code":1,"error":"command not found"}`)
	if err := json.Unmarshal(view, &v); err != nil {
		t.Fatalf("视图 JSON 非法: %v", err)
	}
	if v.Badge.Text != "error" || v.Badge.Tone != "red" || v.Body != "command not found" {
		t.Fatalf("failed view = %+v", v)
	}
}

// TestSSHListView ssh_list 视图：表格，徽标为会话数。
func TestSSHListView(t *testing.T) {
	view, _ := sshListView(`{"success":true,"sessions":["ssh-1","ssh-2"]}`)
	var v dsc.View
	if err := json.Unmarshal(view, &v); err != nil {
		t.Fatalf("视图 JSON 非法: %v", err)
	}
	if v.Kind != "table" || v.Title != "SSH Sessions" || v.Badge == nil || v.Badge.Text != "2" || v.Badge.Tone != "teal" {
		t.Fatalf("view 头 = %+v", v)
	}
	if len(v.Columns) != 1 || len(v.Rows) != 2 || v.Rows[0]["id"] != "ssh-1" {
		t.Fatalf("rows = %+v", v.Rows)
	}

	// 空会话列表：仍为表格（空行由 TUI 回退）
	if view, _ := sshListView(`{"success":true,"sessions":[]}`); len(view) == 0 {
		t.Fatal("空列表应仍返回视图（含列定义）")
	}
}

// TestSSHCloseView ssh_close 视图：session_id 取自参数；closed 绿色、未找到灰色。
func TestSSHCloseView(t *testing.T) {
	view, _ := sshCloseView([]byte(`{"session_id":"ssh-9"}`), `{"success":true,"closed":true}`)
	var v dsc.View
	if err := json.Unmarshal(view, &v); err != nil {
		t.Fatalf("视图 JSON 非法: %v", err)
	}
	if v.Badge.Text != "closed" || v.Badge.Tone != "green" || v.Fields[0].Value != "ssh-9" {
		t.Fatalf("closed view = %+v", v)
	}

	view, _ = sshCloseView([]byte(`{"session_id":"ssh-9"}`), `{"success":true,"closed":false}`)
	if err := json.Unmarshal(view, &v); err != nil {
		t.Fatalf("视图 JSON 非法: %v", err)
	}
	if v.Badge.Text != "not found" || v.Badge.Tone != "gray" {
		t.Fatalf("not-found view = %+v", v)
	}
}
