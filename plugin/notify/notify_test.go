package notify

import (
	"context"
	"testing"

	"dsc/proto"
	"google.golang.org/grpc"
)

// fakeNotify 模拟 proto.PluginNotifyServiceClient（嵌入接口以满足 mustEmbed）。
type fakeNotify struct {
	proto.PluginNotifyServiceClient
	req *proto.NotifyRequest
	err error
}

func (f *fakeNotify) Notify(_ context.Context, req *proto.NotifyRequest, _ ...grpc.CallOption) (*proto.NotifyResponse, error) {
	f.req = req
	return &proto.NotifyResponse{}, f.err
}

func TestDialFromEnvMissing(t *testing.T) {
	t.Setenv(EnvServiceID, "")
	n, err := DialFromEnv(nil)
	if err != nil || n != nil {
		t.Fatalf("missing env should yield (nil, nil), got %v, %v", n, err)
	}
}

func TestDialFromEnvBad(t *testing.T) {
	t.Setenv(EnvServiceID, "nope")
	if _, err := DialFromEnv(nil); err == nil {
		t.Fatal("bad env should fail")
	}
}

func TestNotifyProxy(t *testing.T) {
	f := &fakeNotify{}
	n := &Notifier{c: f}
	if err := n.Notify(context.Background(), "job/done", `{"ID":"x"}`); err != nil {
		t.Fatalf("notify: %v", err)
	}
	if f.req.GetName() != "job/done" || f.req.GetData() != `{"ID":"x"}` {
		t.Fatalf("request = %+v", f.req)
	}
	// 未连接 → 错误
	if err := (&Notifier{}).Notify(context.Background(), "x", ""); err == nil {
		t.Fatal("unconnected notifier should fail")
	}
	// Close 对 nil/未连接安全
	if err := (&Notifier{}).Close(); err != nil {
		t.Fatalf("close empty: %v", err)
	}
	if err := (*Notifier)(nil).Close(); err != nil {
		t.Fatalf("close nil: %v", err)
	}
}
