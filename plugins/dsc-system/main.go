// dsc-system 核心插件混合体：多个策略/后台插件的同一插件目录——仅共用包名
// （package main）与同一编译产物，各驻留插件的声明、逻辑与文件尽量分离
// （每插件独立文件，装配只在 main.go）。后续较为核心的插件逐步迁移至此，
// 新增驻留 = 新文件 + main.go 一行注册，宿主零改动。
//
// 进程形态：通用（dsc）类型插件（目录前缀 dsc-），经 PluginInfo.services
// 声明实际暴露的服务（"tool"/"hook" 恒有，注册了 policy 即加 "policy"），
// 宿主对通用类型按声明机械桥接（对齐 DSH/Cordis「插件类型与服务正交」）：
//   - PolicyService → 工具流水线（pre/execute 拦截、post-execute replace/notice）
//   - HookService   → 宿主事件广播（agent/pre-step 等）
//
// 现有驻留插件：
//   - fsobservation.go  读前改写策略（自 policy-fs-observation 迁入，对齐 DSH
//     fs-observation-policy：deny 拦截 + sha256 新鲜度）
//   - timeout.go        超时决策（自 policy-timeout 迁入，对齐 DSH timeout-policy：
//     tool/execute 槽裁决活跃续命执行域）
//   - spill.go          外置决策（自 policy-spill 迁入，对齐 DSH spill-policy：
//     tool/post-execute 槽超长结果外置为文件 + replace 预览替换）
//   - compaction-basic.go 基础上下文压缩（自宿主 core/compaction.go 迁入，对齐 DSH
//     compaction-basic：agent/pre-step 压力驱动改写消息列表 + agent/request-error
//     溢出紧急压缩重试；LLM 摘要经 interconnect，未互联退化截断式；
//     config.yaml compaction: dsc-system 选其为本插件后端）
//   - reminder.go       重复工具调用提醒（对齐 DSH guard/repeat-tool-reminder，
//     advisory 形态：只产出 notice，不否决/不改写）
//   - skill.go          技能工具（自 tool-skill 迁入：skill / install_skill /
//     uninstall_skill + ContextFn 注入技能索引；tool 服务叠加——TypeDsc 恒注册
//     ToolServiceServer，宿主 ListTools 探测非空后登记为 tool provider）
package main

import (
	"context"
	"fmt"
	"os"

	"dsc-sdk"
	"dsc/proto"
)

func main() {
	fsObservationServer := newFsObservationServer()
	timeoutServer := newTimeoutServer()
	spillServer := newSpillServer()
	reminderServer, err := newReminderServer()
	if err != nil {
		// fail-loud（对齐 DSH 插件装载校验）：配置非法启动即退出，绝不静默回退
		fmt.Fprintf(os.Stderr, "dsc-system: %v\n", err)
		os.Exit(2)
	}
	compactionBasicServer := newCompactionBasicServer()

	skillStore, skillInstalledDir := newSkillResident()
	sdk := dsc.New(dsc.Config{
		Name:    "dsc-system",
		Version: "1.5.3",
		Type:    dsc.TypeDsc,
		// 声明压缩后端能力：config.yaml 的 compaction: dsc-system 选中时，
		// 宿主 registerDscCoreLocked 验证此声明并标记后端生效
		//（对齐 DSH preset compaction group 的能力验证）。
		Provides: map[string]string{
			"compaction": "true",
		},
	})
	// 通用类型叠加 policy 服务：宿主按 PluginInfo.services 的 "policy" 声明，
	// 把内部策略瀑布（多驻留扇出合并）桥接到工具流水线——与独立 policy 插件同一桥。
	// 驻留顺序对齐原 preset 中独立插件的声明顺序（瀑布语义同构）。
	sdk.Policy(&policyPipeline{residents: []proto.PolicyServiceServer{fsObservationServer, timeoutServer, spillServer, reminderServer}})
	// hook 订阅宿主事件（SDK 每插件一个 Hook，多驻留在此多路复用）：
	//   - reminder：agent/pre-step 的「新用户输入」重置重复链（对齐 DSH
	//     repeat-tool-reminder 的 agent/pre-step reset hook——用户插话改变了
	//     上下文，跨插话的重复不是循环）；恒返回空，不影响其他驻留的改写结果
	//   - compaction-basic：agent/pre-step 压缩改写消息列表 + agent/request-error
	//     溢出紧急压缩重试（改写结果非空时优先透传给宿主）
	sdk.Hook(dsc.Hook{
		OnEvent: func(ctx context.Context, eventType, dataJSON string) (string, error) {
			if res, err := reminderServer.handleHostEvent(ctx, eventType, dataJSON); res != "" || err != nil {
				return res, err
			}
			return compactionBasicServer.handleHostEvent(ctx, eventType, dataJSON)
		},
	})
	// 互通：缓存宿主聚合 LLM 客户端供压缩摘要生成（未互联时截断式退化）
	sdk.SetInterconnect(func(ctx context.Context, ic *dsc.Interconnect) error {
		compactionBasicServer.attachLLM(ic.LLM())
		return nil
	})
	// 工具服务叠加：skill 驻留的三个模型可见工具（宿主 ListTools 探测非空
	// 工具集后把本进程同时登记为 tool provider——服务正交）。
	for _, t := range newSkillTools(skillStore, skillInstalledDir) {
		sdk.Tool(t)
	}
	sdk.Serve()
}
