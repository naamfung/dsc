package core

import (
	"encoding/base64"
	"strings"
)

// 工具结果图像入库口（对齐 DSH mcp-client 的图像 admission：图像字节在进入会话
// 历史前折算为内容寻址引用，data URL 不落事件日志）。单一汇聚点在工具流水线
// executeToolBody——宿主内置工具与全部插件工具（RemoteTool gRPC）都经此返回。

// admitToolImages 把工具回传的图像批量折算为内容寻址引用：
//   - data URL：解码出图像字节，写入 temp/screenshots/（dsc-shot:// 引用，24 小时
//     生命周期——操作截图只有短期观察价值，不进持久附件库也不进用户 workspace）；
//   - 已是内容寻址引用（dsc-img:// / dsc-shot://）：原样透传；
//   - 其余形态（非 image/* 或 base64 非法）：丢弃并留痕——无效图像不进入持久
//     历史（对齐 DSH admission 语义），投影层不会见到失效条目。
//
// 入库失败（磁盘等）同样丢弃留痕，不阻塞工具结果本身。
func (m *Manager) admitToolImages(images []string) []string {
	if len(images) == 0 {
		return nil
	}
	out := make([]string, 0, len(images))
	for _, img := range images {
		if strings.HasPrefix(img, imageRefPrefix) || strings.HasPrefix(img, shotRefPrefix) {
			out = append(out, img)
			continue
		}
		data, ok := decodeDataURL(img)
		if !ok {
			m.logger.Warn("tool image admission dropped non-image payload", "bytes", len(img))
			continue
		}
		ref, err := SaveScreenshotAttachment(data)
		if err != nil {
			m.logger.Warn("tool image admission failed", "error", err)
			continue
		}
		out = append(out, ref)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// decodeDataURL 校验并解码 data:image/*;base64,... 数据 URL，返回图像字节。
// 非 image/* 媒体类型或非法 base64 一律拒绝。
func decodeDataURL(dataURL string) ([]byte, bool) {
	const marker = ";base64,"
	if !strings.HasPrefix(dataURL, "data:") {
		return nil, false
	}
	idx := strings.Index(dataURL, marker)
	if idx < 0 {
		return nil, false
	}
	meta := dataURL[len("data:"):idx]
	if !strings.HasPrefix(meta, "image/") {
		return nil, false
	}
	data, err := base64.StdEncoding.DecodeString(dataURL[idx+len(marker):])
	if err != nil {
		return nil, false
	}
	return data, true
}
