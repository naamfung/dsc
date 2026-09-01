package core

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// testModelsServer 返回一个 /v1/models 端点返回指定 JSON 的测试服务器。
func testModelsServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" && r.URL.Path != "/models" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestModelSupportsImages 自动判断：仅当模型明确上报 input_modalities 且不含
// image 时返回 false；未上报 / 未知 / 请求失败一律放行（对齐 DSH）。
func TestModelSupportsImages(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"上报含 image → 支持", `{"data":[{"id":"Agentic-Turbo","input_modalities":["text","image"]}]}`, true},
		{"上报仅 text → 不支持", `{"data":[{"id":"Agentic-Turbo","input_modalities":["text"]}]}`, false},
		{"未上报 input_modalities → 放行", `{"data":[{"id":"Agentic-Turbo"}]}`, true},
		{"上报空数组 → 视为不支持", `{"data":[{"id":"Agentic-Turbo","input_modalities":[]}]}`, false},
		{"模型不在列表 → 放行", `{"data":[{"id":"other"}]}`, true},
		{"空列表 → 放行", `{"data":[]}`, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := testModelsServer(t, c.body)
			if got := ModelSupportsImages(srv.URL, "Agentic-Turbo"); got != c.want {
				t.Fatalf("ModelSupportsImages = %v, want %v", got, c.want)
			}
		})
	}
}

// TestModelSupportsImagesServerErrors 服务端不可达/非 200 视为未知放行。
func TestModelSupportsImagesServerErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	if !ModelSupportsImages(srv.URL, "Agentic-Turbo") {
		t.Fatal("server error should default to allow (unknown)")
	}
	if !ModelSupportsImages("http://127.0.0.1:1", "Agentic-Turbo") {
		t.Fatal("unreachable server should default to allow (unknown)")
	}
}
