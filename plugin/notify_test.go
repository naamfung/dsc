package plugin

import (
	"context"
	"testing"
	"time"

	"dsc/jobs"
	"dsc/proto"
)

// TestPluginNotifyJobDone 验证 job/done 特化：插件通知的任务快照被解析为
// JobSnapshot 并发布 JobDoneEvent（TUI 完成通知唤醒体系复用）。
func TestPluginNotifyJobDone(t *testing.T) {
	m := NewManager(&ManagerConfig{})
	got := make(chan jobs.JobSnapshot, 1)
	m.OnEvent(JobDoneEvent, func(ctx EventContext) (any, error) {
		if s, ok := ctx.Data.(jobs.JobSnapshot); ok {
			got <- s
		}
		return nil, nil
	})
	srv := &pluginNotifyServer{m: m}
	_, err := srv.Notify(context.Background(), &proto.NotifyRequest{
		Name: string(JobDoneEvent),
		Data: `{"ID":"novelforge-1","Kind":"novelforge","Label":"逆推生成","Status":"completed"}`,
	})
	if err != nil {
		t.Fatalf("notify: %v", err)
	}
	select {
	case s := <-got:
		if s.ID != "novelforge-1" || s.Kind != "novelforge" || s.Status != jobs.StatusCompleted {
			t.Fatalf("snapshot = %+v", s)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("JobDoneEvent should be delivered")
	}

	// 坏 data → 错误，不发布
	if _, err := srv.Notify(context.Background(), &proto.NotifyRequest{
		Name: string(JobDoneEvent), Data: `{bad`,
	}); err == nil {
		t.Fatal("bad job/done data should fail")
	}
}

// TestPluginNotifyCustomEvent 验证插件自定义事件：data 按 JSON 解析后广播。
func TestPluginNotifyCustomEvent(t *testing.T) {
	m := NewManager(&ManagerConfig{})
	got := make(chan any, 1)
	m.OnEvent(EventName("novelforge/progress"), func(ctx EventContext) (any, error) {
		got <- ctx.Data
		return nil, nil
	})
	srv := &pluginNotifyServer{m: m}
	_, err := srv.Notify(context.Background(), &proto.NotifyRequest{
		Name: "novelforge/progress", Data: `{"chapter":3}`,
	})
	if err != nil {
		t.Fatalf("notify: %v", err)
	}
	select {
	case d := <-got:
		obj, ok := d.(map[string]any)
		if !ok || obj["chapter"] != float64(3) {
			t.Fatalf("custom event data = %#v", d)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("custom event should be delivered")
	}
}
