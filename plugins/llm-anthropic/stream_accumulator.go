package main

import "github.com/anthropics/anthropic-sdk-go"

// streamAccumulator 按块索引容错累积 Anthropic 流事件。上游（如 DeepSeek anthropic
// 兼容接口）偶发事件乱序：content_block_start 可能越过缺失索引提前到达（例如
// index 3 先于 index 2），SDK 的严格校验（expected index N）会因此中断整条流、
// 导致对话报错。本累积器把越过当前块末尾的 start/delta/stop 事件暂存，待缺失块
// 就位后按序补入；缺失块始终未到达时，流结束丢弃暂存尾部（尽力而为，避免中断）。
type streamAccumulator struct {
	msg     anthropic.Message
	pending map[int64][]anthropic.MessageStreamEventUnion
}

func newStreamAccumulator() *streamAccumulator {
	return &streamAccumulator{pending: make(map[int64][]anthropic.MessageStreamEventUnion)}
}

// accumulate 处理单个流事件；返回非 nil 表示流应终止并上报该错误。
func (a *streamAccumulator) accumulate(event anthropic.MessageStreamEventUnion) error {
	switch event.Type {
	case "content_block_start", "content_block_delta", "content_block_stop":
		if event.Index >= int64(len(a.msg.Content)) {
			// 越过当前末尾：start 乱序或块缺失，暂存等待就位
			queue := a.pending[event.Index]
			if event.Type == "content_block_start" {
				// start 定义块头，必须最先应用：插入队首（可能已有先到的 delta/stop）
				queue = append([]anthropic.MessageStreamEventUnion{event}, queue...)
			} else {
				queue = append(queue, event)
			}
			a.pending[event.Index] = queue
		} else if event.Type == "content_block_start" {
			// 重复 start：对应块已存在，直接忽略
		} else if err := a.msg.Accumulate(event); err != nil {
			return err
		}
	default:
		// message_start / message_delta / message_stop 等直接累积
		if err := a.msg.Accumulate(event); err != nil {
			return err
		}
	}
	return a.flush()
}

// flush 把暂存中已就绪的事件按序应用到消息上：start 必须恰好开启当前末尾的下一个
// 块，delta/stop 在其块存在后即可补入（同一 index 的队列保持块内原始顺序）。
func (a *streamAccumulator) flush() error {
	for {
		progressed := false
		// 1. 补入块已存在的 delta/stop（及冗余 start 的清理）
		for idx, queue := range a.pending {
			if idx >= int64(len(a.msg.Content)) || len(queue) == 0 {
				continue
			}
			ev := queue[0]
			if ev.Type == "content_block_start" {
				// 块已存在时的重复 start：丢弃
				a.pending[idx] = queue[1:]
				progressed = true
				continue
			}
			if err := a.msg.Accumulate(ev); err != nil {
				return err
			}
			a.pending[idx] = queue[1:]
			progressed = true
		}
		// 2. 按序开启新块：当前末尾恰好有待处理的 start
		next := int64(len(a.msg.Content))
		if queue := a.pending[next]; len(queue) > 0 && queue[0].Type == "content_block_start" {
			if err := a.msg.Accumulate(queue[0]); err != nil {
				return err
			}
			a.pending[next] = queue[1:]
			progressed = true
		}
		if !progressed {
			return nil
		}
	}
}
