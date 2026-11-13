package core

import (
	"testing"
	"time"
)

// TestEventDispatchTimeout 事件分发的单轮预算是可调的，且非法值一律回默认 30s。
//
// 为什么必须有这个开关：agent/pre-step 里挂着压缩后端，它要在这一步内真的跑完一次
// 模型摘要，而摘要耗时与模型速度成正比。本地引擎（prefill 1~2K tok/s、decode 20~40
// tok/s）上，一次 40K 的摘要光读 prompt 就要 25s——30s 的固定预算下每一轮都超时，
// 摘要被丢弃，用户看到的是「过了 70% 阈值却始终没压缩」（现场 144 轮全部如此）。
func TestEventDispatchTimeout(t *testing.T) {
	t.Setenv("DSC_EVENT_DISPATCH_TIMEOUT", "")
	if got := eventDispatchTimeout(); got != defaultEventDispatchBudget {
		t.Fatalf("缺省应为 %s，got %s", defaultEventDispatchBudget, got)
	}
	if defaultEventDispatchBudget != 30*time.Second {
		t.Fatalf("缺省值改动会影响所有部署的压缩预算，当前 %s", defaultEventDispatchBudget)
	}

	t.Setenv("DSC_EVENT_DISPATCH_TIMEOUT", "120")
	if got := eventDispatchTimeout(); got != 120*time.Second {
		t.Fatalf("显式值应生效，got %s", got)
	}

	// 非法/越界一律回默认：这个值是超时上限，读错方向就是「事件永久挂住」或「摘要必然被掐」。
	for _, bad := range []string{"abc", "0", "-5", "4", "601", "  "} {
		t.Setenv("DSC_EVENT_DISPATCH_TIMEOUT", bad)
		if got := eventDispatchTimeout(); got != defaultEventDispatchBudget {
			t.Fatalf("非法值 %q 应回默认 %s，got %s", bad, defaultEventDispatchBudget, got)
		}
	}
}
