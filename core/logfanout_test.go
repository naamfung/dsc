package core

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
)

// TestLogFanoutWritesDestinationAndFansOutToSubscriber 验证 LogFanout 同时写原始
// 目的地并向订阅者广播同一行（字段保真：订阅者收到的字节与写入的原始字节一致）。
func TestLogFanoutWritesDestinationAndFansOutToSubscriber(t *testing.T) {
	var dst bytes.Buffer
	f := NewLogFanout(&dst)
	id, ch := f.Subscribe()
	defer f.Unsubscribe(id)

	input := []byte("2026-08-31 11:04:19 [INFO] hello log line\n")
	if _, err := f.Write(input); err != nil {
		t.Fatalf("Write err: %v", err)
	}

	// 原始目的地收到原样内容
	if dst.String() != string(input) {
		t.Errorf("destination got %q, want %q", dst.String(), string(input))
	}
	// 订阅者收到同一份原始字节（字段保真）
	select {
	case got := <-ch:
		if !bytes.Equal(got, input) {
			t.Errorf("subscriber got %q, want %q", got, input)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("subscriber did not receive written line")
	}
}

// TestLogFanoutSlowConsumerDoesNotBlockWrite 验证慢消费者溢出即丢弃、不阻塞日志路径。
func TestLogFanoutSlowConsumerDoesNotBlockWrite(t *testing.T) {
	f := NewLogFanout(ioDiscard{})
	id, ch := f.Subscribe()
	defer f.Unsubscribe(id)
	// 填满缓冲，后续写入仅能丢弃
	for i := 0; i < 300; i++ {
		if _, err := f.Write([]byte("x\n")); err != nil {
			t.Fatalf("Write err: %v", err)
		}
	}
	// 清空一个，再写应能继续不被阻塞
	<-ch
	done := make(chan struct{})
	go func() {
		_, _ = f.Write([]byte("after-full\n"))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Write blocked on full subscriber buffer")
	}
}

// TestUnsubscribeReleasesChannel 验证 Unsubscribe 后通道被关闭，不再广播。
func TestUnsubscribeReleasesChannel(t *testing.T) {
	var dst bytes.Buffer
	f := NewLogFanout(&dst)
	id, ch := f.Subscribe()
	f.Unsubscribe(id)
	if _, err := f.Write([]byte("m\n")); err != nil {
		t.Fatalf("Write err: %v", err)
	}
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected closed channel after unsubscribe, got value")
		}
	case <-time.After(2 * time.Second):
		// 关闭通道读取立即 ok=false；此处若仍阻塞说明未关闭
		t.Fatal("channel not closed after unsubscribe")
	}
}

// TestHostLoggerReachesLogFanoutSubscriber 验证把 hclog 的 Output 设为 LogFanout 后，
// 宿主 logger.Info 产生的完整日志行能被日志流订阅者收到（/logs 链路的单元入口：
// 插件日志经 coreLogger 转发后同样汇聚到同一 fanout）。
func TestHostLoggerReachesLogFanoutSubscriber(t *testing.T) {
	f := NewLogFanout(ioDiscard{})
	id, ch := f.Subscribe()
	defer f.Unsubscribe(id)

	logger := hclog.New(&hclog.LoggerOptions{
		Name:   "dsc-host",
		Level:  hclog.Info,
		Output: f,
	})
	logger.Info("hello-from-host", "key", "value")

	select {
	case line := <-ch:
		if !strings.Contains(string(line), "hello-from-host") ||
			!strings.Contains(string(line), "value") {
			t.Errorf("host log line %q missing message or field", string(line))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("host logger output did not reach subscriber")
	}
}

// ioDiscard 恒丢弃内容的 io.Writer（避免 bytes.Buffer 需清理）。
type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
