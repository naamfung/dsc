package dsc

import (
	"context"
	"fmt"
	"sync"

	"dsc/plugin/llmclient"
	"dsc/plugin/notify"
	"dsc/plugin/toolclient"
	goplugin "github.com/hashicorp/go-plugin"
)

// InterconnectHandler 互通回调：宿主把聚合 LLM/Tool/Notify 服务挂载到本插件
// client broker 后回调，插件在其中缓存客户端（供后续工具执行/钩子使用）。
type InterconnectHandler func(ctx context.Context, ic *Interconnect) error

// Interconnect 宿主能力客户端集（插件→宿主）。
// 独立插件之间互不感知：经宿主聚合路由调用其他插件的工具/LLM。
type Interconnect struct {
	llm  *llmclient.Client
	tool *toolclient.Client
	ntf  *notify.Notifier

	closeOnce sync.Once
}

// LLM 返回宿主聚合 LLM 客户端（未互联时为 nil；调用方自行判空）。
func (ic *Interconnect) LLM() *llmclient.Client { return ic.llm }

// Tool 返回宿主聚合 Tool 客户端（未互联时为 nil）。
func (ic *Interconnect) Tool() *toolclient.Client { return ic.tool }

// Notifier 返回宿主插件通知客户端（未互联时为 nil）。
// 与 Notify（发布事件的便捷方法）区分：需要把客户端实例传给第三方
// （如 LUA 宿主 bindings.Services.Notify）时用本 getter。
func (ic *Interconnect) Notifier() *notify.Notifier { return ic.ntf }

// Notify 把事件发布到宿主事件总线（name 为宿主事件名，如 "job/done" 或插件自定义；
// dataJSON 为事件数据 JSON）；未互联时静默忽略。
func (ic *Interconnect) Notify(name, dataJSON string) error {
	if ic == nil || ic.ntf == nil {
		return nil
	}
	return ic.ntf.Notify(context.Background(), name, dataJSON)
}

// Close 关闭所有客户端连接（幂等）。
func (ic *Interconnect) Close() error {
	if ic == nil {
		return nil
	}
	var err error
	ic.closeOnce.Do(func() {
		if ic.llm != nil {
			err = ic.llm.Close()
		}
		if ic.tool != nil {
			if e := ic.tool.Close(); err == nil && e != nil {
				err = e
			}
		}
		if ic.ntf != nil {
			if e := ic.ntf.Close(); err == nil && e != nil {
				err = e
			}
		}
	})
	return err
}

// llmDial/toolDial/notifyDial 是 llmclient/toolclient/notify.Dial 的薄封装，
// 统一处理「未提供 serviceID 时返回 nil 而非错误」（tool-lua-host 同款语义）。
func llmDial(broker *goplugin.GRPCBroker, id uint32) (*llmclient.Client, error) {
	if broker == nil || id == 0 {
		return nil, nil
	}
	c, err := llmclient.Dial(broker, id)
	if err != nil {
		return nil, fmt.Errorf("dsc-sdk: dial LLM service: %w", err)
	}
	return c, nil
}

func toolDial(broker *goplugin.GRPCBroker, id uint32) (*toolclient.Client, error) {
	if broker == nil || id == 0 {
		return nil, nil
	}
	c, err := toolclient.Dial(broker, id)
	if err != nil {
		return nil, fmt.Errorf("dsc-sdk: dial tool service: %w", err)
	}
	return c, nil
}

func notifyDial(broker *goplugin.GRPCBroker, id uint32) (*notify.Notifier, error) {
	if broker == nil || id == 0 {
		return nil, nil
	}
	n, err := notify.Dial(broker, id)
	if err != nil {
		return nil, fmt.Errorf("dsc-sdk: dial notify service: %w", err)
	}
	return n, nil
}
