package main

import (
	"encoding/json"
	"testing"

	"dsc-sdk"
)

// TestNotifyView notify 视图：徽标与字段按音效类型着色，file 优先于 type。
func TestNotifyView(t *testing.T) {
	view, _ := notifyView(`{"ok":true,"message":"正在播放内置音效: success","type":"success","time":"12:00:00"}`)
	var v dsc.View
	if err := json.Unmarshal(view, &v); err != nil {
		t.Fatalf("视图 JSON 非法: %v", err)
	}
	if v.Kind != "card" || v.Title != "Notify" || v.Badge == nil || v.Badge.Text != "success" || v.Badge.Tone != "green" {
		t.Fatalf("view 头 = %+v", v)
	}
	if len(v.Fields) != 3 || v.Fields[0].Value != "正在播放内置音效: success" || v.Fields[1].Tone != "green" || v.Fields[2].Value != "12:00:00" {
		t.Fatalf("fields = %+v", v.Fields)
	}

	// warning → 黄色徽标
	view, _ = notifyView(`{"type":"warning","message":"w","time":"t"}`)
	if err := json.Unmarshal(view, &v); err != nil {
		t.Fatalf("视图 JSON 非法: %v", err)
	}
	if v.Badge.Tone != "yellow" || v.Fields[1].Tone != "yellow" {
		t.Fatalf("warning view = %+v", v)
	}

	// 自定义文件：显示 file 字段而非 type
	view, _ = notifyView(`{"ok":true,"message":"正在播放自定义文件: /tmp/a.mp3","file":"/tmp/a.mp3","time":"12:00:00"}`)
	if err := json.Unmarshal(view, &v); err != nil {
		t.Fatalf("视图 JSON 非法: %v", err)
	}
	if v.Badge.Tone != "gray" || v.Fields[1].Key != "file" || v.Fields[1].Value != "/tmp/a.mp3" {
		t.Fatalf("file view = %+v", v.Fields)
	}

	// 非法 JSON 回退
	if view, _ := notifyView("not json"); len(view) != 0 {
		t.Fatalf("非法 JSON 应返回空视图: %q", view)
	}
}
