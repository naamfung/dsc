package main

import (
        "context"

        "dsc/proto"
        pbproto "google.golang.org/protobuf/proto"
)

// policyPipeline dsc-system 内部策略瀑布：宿主把本进程视为单个 policy 服务，
// 内部按驻留声明顺序扇出。合并语义与宿主多 policy 插件的顺序瀑布同构：
//   - 每个驻留策略收到的是同一事件的顺序转发；replace 结果前馈——后续驻留
//     看到的是前驻留改写后的 result（宿主桥对各 policy 插件亦按此顺序构造
//     post-execute 事件，见 core.bridgePolicyToPipeline）
//   - deny 占槽短路：任一驻留 deny 即返回（后续驻留不再评估），reason 原文透传
//   - notice 全量聚合（advisory 不占决策槽，多个驻留的 notice 以空行连接）
//   - TimeoutSpec 取首个非空裁决（execute 槽语义：执行域由宿主机械安装）
//
// 转发失败即整体失败：fail-open（放行）由宿主桥统一负责，插件内不吞错。
type policyPipeline struct {
        proto.UnimplementedPolicyServiceServer
        residents []proto.PolicyServiceServer
}

func (p *policyPipeline) OnEvent(ctx context.Context, ev *proto.PolicyEvent) (*proto.PolicyDecision, error) {
        merged := &proto.PolicyDecision{}
        cur := ev
        for _, r := range p.residents {
                dec, err := r.OnEvent(ctx, cur)
                if err != nil {
                        return nil, err
                }
                if n := dec.GetNotice(); n != "" {
                        if merged.Notice != "" {
                                merged.Notice += "\n\n"
                        }
                        merged.Notice += n
                }
                if merged.Timeout == nil {
                        merged.Timeout = dec.GetTimeout()
                }
                switch dec.GetAction() {
                case "deny":
                        return &proto.PolicyDecision{
                                Action: "deny",
                                Reason: dec.GetReason(),
                                Notice: merged.Notice,
                        }, nil
                case "replace":
                        if dec.GetResult() != "" {
                                merged.Action = "replace"
                                merged.Result = dec.GetResult()
                                fed := pbproto.Clone(cur).(*proto.PolicyEvent) // 深拷贝（proto 消息不可浅拷贝）
                                fed.Result = dec.GetResult()
                                cur = fed
                        }
                }
        }
        return merged, nil
}
