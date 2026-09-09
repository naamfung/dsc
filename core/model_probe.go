package core

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// ModelSupportsImages 自动判断模型是否支持图像输入（对齐 DSH 按模型能力校验）：
// 请求 OpenAI 兼容的 /models 端点（依次尝试 {baseURL}/v1/models 与 {baseURL}/models），
// 读取指定模型上报的 input_modalities。规则与 DSH 一致——仅当模型明确声明
// 不支持 image 才返回 false；服务端未上报该字段、请求失败或模型未找到时按
// 「未知 → 放行」返回 true。各 LLM 插件启动时据此决定是否启用视觉。
func ModelSupportsImages(baseURL, model string) bool {
	mods, reported, ok := probeModelModalities(baseURL, model)
	if !ok || !reported {
		return true // 未知 / 未上报 → 放行（对齐 DSH）
	}
	return mods["image"]
}

// probeModelModalities 请求 /models 读取指定模型上报的 input_modalities 集合。
// 返回值：mods 为能力集合；reported 标记该模型是否明确上报了 input_modalities
// （false 表示未上报/未知）；ok 标记是否成功找到该模型（请求失败/模型不在列表中
// 为 false）。
func probeModelModalities(baseURL, model string) (mods map[string]bool, reported, ok bool) {
	for _, suffix := range []string{"/v1/models", "/models"} {
		u := strings.TrimRight(baseURL, "/") + suffix
		set, rep, found := fetchModelModalities(u, model)
		if found {
			return set, rep, true
		}
	}
	return nil, false, false
}

// fetchModelModalities 请求单个 /models 端点，返回指定模型的能力集合。
// 模型找到但 input_modalities 字段缺失（nil）→ reported=false（视为未知，放行）；
// 字段存在（含空数组 []）→ reported=true，按集合判断。
func fetchModelModalities(url, model string) (mods map[string]bool, reported, found bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, false, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false, false
	}
	var payload struct {
		Data []struct {
			ID              string   `json:"id"`
			InputModalities []string `json:"input_modalities"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, false, false
	}
	for _, m := range payload.Data {
		if strings.EqualFold(m.ID, model) {
			if m.InputModalities == nil {
				return nil, false, true // 找到模型但未上报能力 → 未知
			}
			set := map[string]bool{}
			for _, mod := range m.InputModalities {
				set[mod] = true
			}
			return set, true, true
		}
	}
	return nil, false, false // 模型不在列表中
}
