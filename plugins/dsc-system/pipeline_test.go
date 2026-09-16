package main

import (
        "context"
        "strings"
        "testing"

        "dsc/proto"
)

// 内部策略瀑布合并语义：与宿主多 policy 插件顺序瀑布同构（replace 前馈、
// deny 占槽短路、notice 全量聚合、TimeoutSpec 取首个非空）。

type stubPolicy struct {
        proto.UnimplementedPolicyServiceServer
        dec *proto.PolicyDecision
        err error
        // seen 记录本驻留实际收到的事件（验证 replace 前馈）
        seen []*proto.PolicyEvent
}

func (s *stubPolicy) OnEvent(ctx context.Context, ev *proto.PolicyEvent) (*proto.PolicyDecision, error) {
        s.seen = append(s.seen, ev)
        return s.dec, s.err
}

func TestPipelineAdvisoryNoticeAggregates(t *testing.T) {
        p := &policyPipeline{residents: []proto.PolicyServiceServer{
                &stubPolicy{dec: &proto.PolicyDecision{Notice: "first"}},
                &stubPolicy{dec: &proto.PolicyDecision{Notice: "second"}},
        }}
        dec, err := p.OnEvent(context.Background(), &proto.PolicyEvent{Kind: "tool/post-execute"})
        if err != nil {
                t.Fatalf("OnEvent: %v", err)
        }
        if dec.GetAction() != "" || dec.GetNotice() != "first\n\nsecond" {
                t.Fatalf("allow + aggregated notices expected, got %+v", dec)
        }
}

func TestPipelineDenyShortCircuits(t *testing.T) {
        third := &stubPolicy{dec: &proto.PolicyDecision{}}
        p := &policyPipeline{residents: []proto.PolicyServiceServer{
                &stubPolicy{dec: &proto.PolicyDecision{Notice: "n1"}},
                &stubPolicy{dec: &proto.PolicyDecision{Action: "deny", Reason: "sealed"}},
                third,
        }}
        dec, err := p.OnEvent(context.Background(), &proto.PolicyEvent{Kind: "tool/post-execute"})
        if err != nil {
                t.Fatalf("OnEvent: %v", err)
        }
        if dec.GetAction() != "deny" || dec.GetReason() != "sealed" || dec.GetNotice() != "n1" {
                t.Fatalf("deny must short-circuit with reason + aggregated notice, got %+v", dec)
        }
        if len(third.seen) != 0 {
                t.Fatalf("residents after deny must not be evaluated")
        }
}

func TestPipelineReplaceFeedsForward(t *testing.T) {
        second := &stubPolicy{dec: &proto.PolicyDecision{}}
        p := &policyPipeline{residents: []proto.PolicyServiceServer{
                &stubPolicy{dec: &proto.PolicyDecision{Action: "replace", Result: "rewritten"}},
                second,
        }}
        dec, err := p.OnEvent(context.Background(), &proto.PolicyEvent{Kind: "tool/post-execute", Result: "original"})
        if err != nil {
                t.Fatalf("OnEvent: %v", err)
        }
        if dec.GetAction() != "replace" || dec.GetResult() != "rewritten" {
                t.Fatalf("replace must surface, got %+v", dec)
        }
        if len(second.seen) != 1 || second.seen[0].GetResult() != "rewritten" {
                t.Fatalf("later resident must see the replaced result (feed-forward), got %+v", second.seen)
        }
}

func TestPipelineForwardErrorPropagates(t *testing.T) {
        p := &policyPipeline{residents: []proto.PolicyServiceServer{
                &stubPolicy{err: context.DeadlineExceeded},
        }}
        if _, err := p.OnEvent(context.Background(), &proto.PolicyEvent{}); err == nil {
                t.Fatalf("forward error must propagate (fail-open is the host bridge's job)")
        }
}

func TestCanonicalArgsRawFallback(t *testing.T) {
        if got := canonicalArgs(`{bad json`); got != `{bad json` {
                t.Fatalf("malformed args must fall back to raw string, got %q", got)
        }
        if got := canonicalArgs(`{"b":2,"a":1}`); got != `{"a":1,"b":2}` {
                t.Fatalf("keys must be sorted, got %q", got)
        }
        if got := canonicalArgs(`{"a":{"z":1,"y":[2,1]}}`); !strings.Contains(got, `"y":[2,1],"z":1`) {
                t.Fatalf("nested keys must be sorted too, got %q", got)
        }
}
