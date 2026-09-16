package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"dsc/proto"
)

// 请求面图像预算卸载（对齐 DSH RequestImageOffloadPolicy / offloadRequestImagesWithPolicy）：
// 会话历史中的工具截图随轮次线性累积（computer-use 高频截图场景尤甚），全部随每轮
// 请求重放会让 token 与费用无谓膨胀，且旧截图都是过时观察。本投影在请求组装时把
// 超出预算的最旧图像引用退役为稳定占位文本——最新观察永远在场；决策只依赖完整
// 历史（确定性、可复现），且不改动会话存储（append-only 日志原样保留，纯瞬态投影）。

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
		nm := *m
		drop := len(m.Images)
		if remaining < drop {
			drop = remaining
		}
		nm.Images = m.Images[drop:]
		nm.Content = m.Content + omittedImagesText(m.Images[:drop])
		remaining -= drop
		out[i] = &nm
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
