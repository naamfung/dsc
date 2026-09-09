package main

import (
	"encoding/json"
	"testing"

	"dsc-sdk"
)

// TestWebFetchView web_fetch 视图：纯文本，成功/失败徽标。
func TestWebFetchView(t *testing.T) {
	view, _ := webFetchView(`{"success":true,"url":"https://example.com","content":"Hello 世界"}`)
	var v dsc.View
	if err := json.Unmarshal(view, &v); err != nil {
		t.Fatalf("视图 JSON 非法: %v", err)
	}
	if v.Kind != "plain" || v.Title != "Fetch" || v.Badge == nil || v.Badge.Text != "ok" || v.Badge.Tone != "green" {
		t.Fatalf("view 头 = %+v", v)
	}
	if v.Body != "Hello 世界" {
		t.Fatalf("body = %q", v.Body)
	}

	view, _ = webFetchView(`{"success":false,"url":"https://example.com","error":"timeout"}`)
	var v2 dsc.View
	if err := json.Unmarshal(view, &v2); err != nil {
		t.Fatalf("视图 JSON 非法: %v", err)
	}
	if v2.Badge.Text != "error" || v2.Badge.Tone != "red" || v2.Body != "timeout" {
		t.Fatalf("failed view = %+v", v2)
	}
}

// TestWebSearchView web_search 视图：表格 + 结果数徽标。
func TestWebSearchView(t *testing.T) {
	result := `{"success":true,"query":"go","results":[{"title":"Go 官网","url":"https://go.dev","description":"desc","source":"google"}],"sources":["google"]}`
	view, _ := webSearchView(result)
	var v dsc.View
	if err := json.Unmarshal(view, &v); err != nil {
		t.Fatalf("视图 JSON 非法: %v", err)
	}
	if v.Kind != "table" || v.Title != "Search" || v.Badge == nil || v.Badge.Text != "1 result(s)" || v.Badge.Tone != "teal" {
		t.Fatalf("view 头 = %+v", v)
	}
	if len(v.Columns) != 4 || len(v.Rows) != 1 {
		t.Fatalf("columns/rows = %d/%d", len(v.Columns), len(v.Rows))
	}
	if v.Rows[0]["title"] != "Go 官网" || v.Rows[0]["url"] != "https://go.dev" || v.Rows[0]["source"] != "google" {
		t.Fatalf("row = %+v", v.Rows[0])
	}
}

// TestBrowserClickView browser_click 视图：卡片，成功/失败徽标。
func TestBrowserClickView(t *testing.T) {
	view, _ := browserClickView(`{"success":true,"message":"成功點擊元素: #btn","url":"https://example.com"}`)
	var v dsc.View
	if err := json.Unmarshal(view, &v); err != nil {
		t.Fatalf("视图 JSON 非法: %v", err)
	}
	if v.Kind != "card" || v.Title != "Browser" || v.Badge.Text != "clicked" || v.Badge.Tone != "green" {
		t.Fatalf("view = %+v", v)
	}
	if v.Fields[1].Value != "https://example.com" {
		t.Fatalf("fields = %+v", v.Fields)
	}

	view, _ = browserClickView(`{"success":false,"message":"未找到元素"}`)
	var v2 dsc.View
	if err := json.Unmarshal(view, &v2); err != nil {
		t.Fatalf("视图 JSON 非法: %v", err)
	}
	if v2.Badge.Text != "failed" || v2.Badge.Tone != "red" || len(v2.Fields) != 1 {
		t.Fatalf("failed view = %+v", v2)
	}
}

// TestBrowserTypeView browser_type 视图：卡片，含输入值字段。
func TestBrowserTypeView(t *testing.T) {
	view, _ := browserTypeView(`{"success":true,"message":"成功輸入文本到: #q","value":"hello"}`)
	var v dsc.View
	if err := json.Unmarshal(view, &v); err != nil {
		t.Fatalf("视图 JSON 非法: %v", err)
	}
	if v.Badge.Text != "typed" || v.Badge.Tone != "green" || v.Fields[1].Value != "hello" {
		t.Fatalf("view = %+v", v)
	}
}

// TestBrowserScreenshotView browser_screenshot 视图：卡片，尺寸/大小格式化。
func TestBrowserScreenshotView(t *testing.T) {
	view, _ := browserScreenshotView(`{"url":"https://example.com","success":true,"savedFile":"C:\\ws\\screenshots\\a.png","fullPage":false,"width":1280,"height":800,"size":204800}`)
	var v dsc.View
	if err := json.Unmarshal(view, &v); err != nil {
		t.Fatalf("视图 JSON 非法: %v", err)
	}
	if v.Title != "Screenshot" || v.Badge.Text != "saved" || v.Badge.Tone != "green" {
		t.Fatalf("view 头 = %+v", v)
	}
	if v.Fields[1].Value != "C:\\ws\\screenshots\\a.png" || v.Fields[2].Value != "1280 × 800 · 200 KB" {
		t.Fatalf("fields = %+v", v.Fields)
	}

	view, _ = browserScreenshotView(`{"success":false,"error":"xxx"}`)
	var v2 dsc.View
	if err := json.Unmarshal(view, &v2); err != nil {
		t.Fatalf("视图 JSON 非法: %v", err)
	}
	if v2.Badge.Text != "failed" || v2.Badge.Tone != "red" {
		t.Fatalf("failed view = %+v", v2)
	}
}
