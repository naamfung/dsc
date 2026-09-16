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
//   - reminder.go       重复工具调用提醒（对齐 DSH guard/repeat-tool-reminder，
//     advisory 形态：只产出 notice，不否决/不改写）
package main

import (
	"fmt"
	"os"

	"dsc-sdk"
	"dsc/proto"
)

func main() {
	fsObs := newFsObservationServer()
	reminder, err := newReminderServer()
	if err != nil {
		// fail-loud（对齐 DSH 插件装载校验）：配置非法启动即退出，绝不静默回退
		fmt.Fprintf(os.Stderr, "dsc-system: %v\n", err)
		os.Exit(2)
	}

	sdk := dsc.New(dsc.Config{
		Name:    "dsc-system",
		Version: "1.1.0",
		Type:    dsc.TypeDsc,
	})
	// 通用类型叠加 policy 服务：宿主按 PluginInfo.services 的 "policy" 声明，
	// 把内部策略瀑布（多驻留扇出合并）桥接到工具流水线——与独立 policy 插件同一桥。
	// 驻留顺序对齐原 preset 中独立插件的声明顺序（瀑布语义同构）。
	sdk.Policy(&policyPipeline{residents: []proto.PolicyServiceServer{fsObs, reminder}})
	// hook 订阅宿主事件：agent/pre-step 的「新用户输入」重置重复链
	//（对齐 DSH repeat-tool-reminder 的 agent/pre-step reset hook——用户插话
	// 改变了上下文，跨插话的重复不是循环）。
	sdk.Hook(dsc.Hook{OnEvent: reminder.handleHostEvent})
	sdk.Serve()
}
