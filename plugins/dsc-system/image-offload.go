// dsc-system 驻留：请求面图像预算卸载（对齐 DSH RequestImageOffloadPolicy 的
// count 预算语义）。自 agent-react-loop/image_offload.go 抽取迁入——策略归还
// 插件，agent 请求组装零预处理；经 agent/pre-step 事件在消息列表上做纯瞬态投影
// （不改会话存储），与 compaction-basic 共享单 Hook 多路复用链：压缩先于卸载，
// 卸载作用于压缩改写结果（保持既有次序：结构改写在前、请求面投影在后）。
//
// DSH 语义对照：RequestImageOffloadPolicy 的 maxImages 决定「最旧优先退役」；
// maxBytes/byteQuantum 依赖请求面内联 base64 字节计量——dsc 消息面仅承载
// dsc-img:// 引用（字节计量在附件/Provider 解析层，引用本身无字节可计），故
// 平移 count 预算，byte 预算不做。
//
// 会话历史中的工具截图随轮次线性累积（computer-use 高频截图场景尤甚），全部随
// 每轮请求重放会让 token 与费用无谓膨胀，且旧截图都是过时观察。本投影在请求
// 组装时把超出预算的最旧图像引用退役为稳定占位文本——最新观察永远在场；决策
// 只依赖完整历史（确定性、可复现），且不改动会话存储（append-only 日志原样
// 保留，纯瞬态投影）。
//
// 配置（DSC_ 前缀 env 白名单天然可见）：
//   - DSC_MAX_REQUEST_IMAGES  单请求图像引用上限（默认 12；0 = 不限制）
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"dsc/core"
	"dsc/proto"

	pbproto "google.golang.org/protobuf/proto"
)

// defaultMaxRequestImages 单次请求携带的图像引用上限。CU 截图默认最长边 1568、
// 单图约 1.1K token，12 张约合 13K token，观察密度与上下文占用的平衡点；更长的
// 历史价值递减，按最旧优先退役。
const defaultMaxRequestImages = 12

// envMaxRequestImages 环境变量名：覆盖请求图像上限（"0" = 不限制）。
const envMaxRequestImages = "DSC_MAX_REQUEST_IMAGES"

// maxRequestImages 读取请求图像上限配置：环境变量优先（"0"=不限制），否则默认值。
func maxRequestImages() int {
	if v := strings.TrimSpace(os.Getenv(envMaxRequestImages)); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return n
		}
	}
	return defaultMaxRequestImages
}

// offloadRequestImages 返回请求历史中超出 maxImages 预算的图像退役后的消息列表：
// 按历史顺序最旧优先，超出部分从消息的 Images 移除并在内容尾部追加占位文本行
// （模型由此知道该处曾有图、已过期退役）。预算内（含不限制）时原样返回，零拷贝；
// 超预算时仅浅拷贝受影响消息，不改动传入消息与会话存储。
func offloadRequestImages(msgs []*proto.Message, maxImages int) []*proto.Message {
	if maxImages <= 0 || len(msgs) == 0 {
		return msgs
	}
	total := 0
	for _, m := range msgs {
		total += len(m.Images)
	}
	if total <= maxImages {
		return msgs
	}
	remaining := total - maxImages
	out := make([]*proto.Message, len(msgs))
	for i, m := range msgs {
		if remaining == 0 || len(m.Images) == 0 {
			out[i] = m
			continue
		}
		// proto.Clone 深拷贝受影响消息（Message 内嵌状态不可值拷贝，vet 线）：
		// 瞬态投影对象，深浅不影响语义，原消息仍只读不动
		nm := pbproto.Clone(m).(*proto.Message)
		drop := len(m.Images)
		if remaining < drop {
			drop = remaining
		}
		nm.Images = m.Images[drop:]
		nm.Content = m.Content + omittedImagesText(m.Images[:drop])
		remaining -= drop
		out[i] = nm
	}
	return out
}

// omittedImagesText 为退役图像引用生成稳定的占位文本行（对齐 DSH offloadedImageText：
// 身份留痕——引用写进占位，模型可请求用户重新提供或再截一张新图）。
func omittedImagesText(refs []string) string {
	var b strings.Builder
	for _, ref := range refs {
		if ref == "" {
			continue
		}
		fmt.Fprintf(&b, "\n[image omitted to fit request image limits; %s]", ref)
	}
	return b.String()
}

// imageOffloadServer 请求面图像预算卸载驻留（pre-step 消息投影，无状态）。
type imageOffloadServer struct{}

func newImageOffloadServer() *imageOffloadServer { return &imageOffloadServer{} }

// handleHostEvent 事件入口（main.go 的 hook 多路复用之一）。
func (s *imageOffloadServer) handleHostEvent(ctx context.Context, eventType, dataJSON string) (string, error) {
	switch eventType {
	case string(core.EventAgentPreStep):
		return s.handlePreStep(dataJSON)
	default:
		return "", nil
	}
}

// handlePreStep 预算卸载投影（agent/pre-step 拦截）。
// 返回 {"messages": [...]}（卸载后列表）或 ""（预算内，零拷贝零开销）。
func (s *imageOffloadServer) handlePreStep(dataJSON string) (string, error) {
	var ev core.AgentPreStepEvent
	if err := json.Unmarshal([]byte(dataJSON), &ev); err != nil {
		return "", fmt.Errorf("image-offload: parse pre-step event: %w", err)
	}
	var msgs []*proto.Message
	if ev.MessagesJSON != "" {
		if err := json.Unmarshal([]byte(ev.MessagesJSON), &msgs); err != nil {
			return "", fmt.Errorf("image-offload: parse messages: %w", err)
		}
	}
	limit := maxRequestImages()
	total := 0
	for _, m := range msgs {
		total += len(m.Images)
	}
	if len(msgs) == 0 || limit <= 0 || total <= limit {
		return "", nil // 预算内：零开销，不产出改写结果
	}
	projected := offloadRequestImages(msgs, limit)
	out, err := json.Marshal(map[string]any{"messages": projected})
	if err != nil {
		return "", fmt.Errorf("image-offload: marshal messages: %w", err)
	}
	return string(out), nil
}

// chainPreStep 把上游驻留的 pre-step 改写结果（{"messages": [...]}）拼接回事件
// 载荷，供本驻留在「压缩改写后的列表」上继续投影——多驻留共享单 Hook 的链式
// 改写（宿主对每插件只收一个结果，链内拼接是共享 Hook 的接缝）。
func chainPreStep(dataJSON, upstreamResult string) (string, error) {
	var ev core.AgentPreStepEvent
	if err := json.Unmarshal([]byte(dataJSON), &ev); err != nil {
		return "", fmt.Errorf("image-offload: parse pre-step event: %w", err)
	}
	var upstream struct {
		Messages []*proto.Message `json:"messages"`
	}
	if err := json.Unmarshal([]byte(upstreamResult), &upstream); err != nil {
		return "", fmt.Errorf("image-offload: parse upstream rewrite: %w", err)
	}
	msgs, err := json.Marshal(upstream.Messages)
	if err != nil {
		return "", fmt.Errorf("image-offload: marshal upstream messages: %w", err)
	}
	ev.MessagesJSON = string(msgs)
	out, err := json.Marshal(ev)
	if err != nil {
		return "", fmt.Errorf("image-offload: re-marshal pre-step event: %w", err)
	}
	return string(out), nil
}
