package core

import (
	"io"
	"sync"
)

// LogFanout 是一个扇出 io.Writer：把写入的数据同时转发给「原始目的地」
// （os.Stderr 或日志文件）与零到多个订阅者（供 ADMIN /logs SSE 流消费）。
//
// 监听宿主日志与核心（插件）日志的 Output，即可同时捕获宿主 INFO 日志与
// 经 go-plugin 转发上来的插件子进程 stderr（如 notify 插件 log.Println）。
// 订阅通道带缓冲，慢消费者不会阻塞日志路径（溢出即丢弃）。
type LogFanout struct {
	mu   sync.Mutex
	dst  io.Writer           // 原始目的地
	subs map[int]chan []byte // 订阅者 id → 行通道
	next int
}

// NewLogFanout 创建以 dst 为原始目的地的扇出 writer。
func NewLogFanout(dst io.Writer) *LogFanout {
	return &LogFanout{dst: dst, subs: make(map[int]chan []byte)}
}

// Write 实现 io.Writer：写原始目的地，并向每个订阅者广播当前行。
// 广播使用非阻塞投递，避免慢消费者阻塞日志路径。
func (f *LogFanout) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n, err := f.dst.Write(p)
	if err != nil {
		return n, err
	}
	line := append([]byte(nil), p...)
	for _, ch := range f.subs {
		select {
		case ch <- line:
		default: // 慢消费者溢出即丢弃，不阻塞
		}
	}
	return n, nil
}

// Subscribe 注册一个订阅者，返回订阅 id 与行通道。调用方须在结束订阅时
// 调用 Unsubscribe 释放通道。
func (f *LogFanout) Subscribe() (int, <-chan []byte) {
	f.mu.Lock()
	f.next++
	id := f.next
	ch := make(chan []byte, 256)
	f.subs[id] = ch
	f.mu.Unlock()
	return id, ch
}

// Unsubscribe 移除指定 id 的订阅者，释放其通道。
func (f *LogFanout) Unsubscribe(id int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if ch, ok := f.subs[id]; ok {
		delete(f.subs, id)
		close(ch)
	}
}
