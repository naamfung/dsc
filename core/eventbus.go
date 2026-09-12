package core

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"sync"

	"github.com/hashicorp/go-hclog"
)

// 通用事件分发总线（对齐 DSH Cordis events.ts 的分发模式）：
// 监听器按事件名注册，宿主按分发模式调用。五种模式：
//
//	emit     同步顺序调用，忽略返回值（监听器错误仅记录，不影响调用方）
//	parallel 并发执行，聚合所有错误
//	serial   顺序执行，遇错误即停止
//	bail     顺序执行，首个非 nil 返回值即短路停止
//	waterfall 洋葱模型：监听器通过 next() 委托给链上后续监听器，不调 next 即 veto
//
// 与插件生命周期事件（Subscribe/publishEventLocked）正交：后者是状态机专用的
// 推送通道，此处是宿主内通用的事件扩展点，供工具流水线、请求拦截等使用。

// EventName 事件名（字符串，可扩展）。
type EventName string

// EventContext 事件分发时的上下文载荷。
type EventContext struct {
	Name EventName
	Data any
	// Context 发起事件的调用方上下文（waterfall 工具流水线把原始请求 ctx 一并携带，
	// 供监听器感知取消/截止；emit 等无 ctx 的构造可留 nil）。
	Context context.Context
}

// Listener 普通监听器：返回值仅对 bail 模式有意义，其余模式忽略。
type Listener func(ctx EventContext) (any, error)

// WaterfallListener 洋葱监听器：调用 next 委托给链上后续监听器，
// 不调用 next 即中断（veto）。
type WaterfallListener func(ctx EventContext, next func(EventContext) error) error

type listenerEntry struct {
	order int
	fn    Listener
}

type waterfallEntry struct {
	order int
	fn    WaterfallListener
}

// EventBus 事件分发总线。
type EventBus struct {
	mu   sync.RWMutex
	next int
	emit map[EventName][]listenerEntry
	wf   map[EventName][]waterfallEntry
	any  []listenerEntry // 全局监听器（带 order，供移除；Emit 时与按名监听器一起调用）
	// logger 可选：监听器 panic 恢复时的日志记录器（nil 时静默——不中断是首要目标，
	// 日志仅用于定位根因，未配置时放弃记录）。
	logger hclog.Logger
}

// NewEventBus 创建事件总线。
func NewEventBus() *EventBus {
	return &EventBus{
		emit: make(map[EventName][]listenerEntry),
		wf:   make(map[EventName][]waterfallEntry),
	}
}

// SetLogger 设置监听器 panic 日志记录器（nil 时静默忽略 panic 日志）。
func (b *EventBus) SetLogger(l hclog.Logger) {
	b.logger = l
}

// logPanic 记录监听器 panic（含调用栈），供各分发模式 recover 后调用。
func (b *EventBus) logPanic(name EventName, r any) {
	if b.logger == nil {
		return
	}
	b.logger.Error("event listener panicked (recovered)",
		"event", string(name), "panic", fmt.Sprint(r), "stack", string(debug.Stack()))
}

// callListener 调用单个监听器，把 panic 转为错误返回并记录日志（供 Emit/Parallel/Serial/Bail 复用）。
func (b *EventBus) callListener(name EventName, fn Listener, ctx EventContext) (v any, err error) {
	defer func() {
		if r := recover(); r != nil {
			b.logPanic(name, r)
			v, err = nil, fmt.Errorf("event %s listener panicked: %v", name, r)
		}
	}()
	return fn(ctx)
}

// OnAny 注册全局监听器（每次 Emit 都会调用，无论事件名；供宿主向插件广播
// 事件）。返回移除函数。
func (b *EventBus) OnAny(fn Listener) func() {
	b.mu.Lock()
	b.next++
	entry := listenerEntry{order: b.next, fn: fn}
	b.any = append(b.any, entry)
	b.mu.Unlock()
	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		for i, e := range b.any {
			if e.order == entry.order {
				b.any = append(b.any[:i], b.any[i+1:]...)
				break
			}
		}
	}
}

// On 注册普通监听器（按注册顺序执行），返回移除函数。
func (b *EventBus) On(name EventName, fn Listener) func() {
	b.mu.Lock()
	b.next++
	entry := listenerEntry{order: b.next, fn: fn}
	b.emit[name] = append(b.emit[name], entry)
	b.mu.Unlock()
	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		entries := b.emit[name]
		for i, e := range entries {
			if e.order == entry.order {
				b.emit[name] = append(entries[:i], entries[i+1:]...)
				break
			}
		}
	}
}

// OnWaterfall 注册洋葱监听器（按注册顺序嵌套），返回移除函数。
func (b *EventBus) OnWaterfall(name EventName, fn WaterfallListener) func() {
	b.mu.Lock()
	b.next++
	entry := waterfallEntry{order: b.next, fn: fn}
	b.wf[name] = append(b.wf[name], entry)
	b.mu.Unlock()
	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		entries := b.wf[name]
		for i, e := range entries {
			if e.order == entry.order {
				b.wf[name] = append(entries[:i], entries[i+1:]...)
				break
			}
		}
	}
}

// snapshotEmit 返回普通监听器快照（按注册顺序）。
func (b *EventBus) snapshotEmit(name EventName) []Listener {
	b.mu.RLock()
	defer b.mu.RUnlock()
	entries := b.emit[name]
	out := make([]Listener, len(entries))
	for i, e := range entries {
		out[i] = e.fn
	}
	return out
}

// snapshotAny 返回全局监听器快照（按注册顺序）。
func (b *EventBus) snapshotAny() []Listener {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]Listener, len(b.any))
	for i, e := range b.any {
		out[i] = e.fn
	}
	return out
}

// snapshotWaterfall 返回洋葱监听器快照（按注册顺序）。
func (b *EventBus) snapshotWaterfall(name EventName) []WaterfallListener {
	b.mu.RLock()
	defer b.mu.RUnlock()
	entries := b.wf[name]
	out := make([]WaterfallListener, len(entries))
	for i, e := range entries {
		out[i] = e.fn
	}
	return out
}

// Emit 同步顺序调用所有监听器，忽略返回值；监听器错误仅记录不中断。
// 按名监听器与全局监听器（OnAny）都会收到事件。
// 监听器 panic 不冒泡、不中断后续监听器：单个坏监听器不影响事件通知（已记录日志）。
func (b *EventBus) Emit(name EventName, ctx EventContext) {
	ctx.Name = name
	for _, fn := range b.snapshotEmit(name) {
		if _, err := b.callListener(name, fn, ctx); err != nil {
			_ = err // panic 已转错误并记录日志；Emit 忽略返回值
		}
	}
	for _, fn := range b.snapshotAny() {
		if _, err := b.callListener(name, fn, ctx); err != nil {
			_ = err
		}
	}
}

// Parallel 并发调用所有监听器并聚合错误。
func (b *EventBus) Parallel(name EventName, ctx EventContext) error {
	ctx.Name = name
	fns := b.snapshotEmit(name)
	var wg sync.WaitGroup
	errCh := make(chan error, len(fns))
	for _, fn := range fns {
		wg.Add(1)
		go func(fn Listener) {
			defer wg.Done()
			// 监听器 panic 在 goroutine 内 recover：goroutine panic 无法被外层 defer
			// 捕获，若不在此处接住会直接 crash 整个宿主进程。
			if _, err := b.callListener(name, fn, ctx); err != nil {
				errCh <- err
			}
		}(fn)
	}
	wg.Wait()
	close(errCh)
	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// Serial 顺序调用所有监听器，遇错误即停止并返回。
func (b *EventBus) Serial(name EventName, ctx EventContext) error {
	ctx.Name = name
	for _, fn := range b.snapshotEmit(name) {
		if _, err := b.callListener(name, fn, ctx); err != nil {
			return err
		}
	}
	return nil
}

// Bail 顺序调用监听器，首个非 nil 返回值即短路停止并返回该值。
// 无监听器或全部返回 nil 时返回 nil。
func (b *EventBus) Bail(name EventName, ctx EventContext) (any, error) {
	ctx.Name = name
	for _, fn := range b.snapshotEmit(name) {
		if v, err := b.callListener(name, fn, ctx); v != nil || err != nil {
			return v, err
		}
	}
	return nil, nil
}

// Waterfall 按洋葱模型委托调用监听器链：从最外层监听器开始，每个监听器
// 通过 next 委托给链上后续监听器（含兜底 next）；监听器不调用 next 即
// 中断链（veto），其返回值为最终结果。无监听器时直接调用兜底 next。
// 监听器 panic 不冒泡：洋葱链任意一层（含兜底 next）panic 都转为错误返回
// 并记录日志——工具流水线 pre/execute/post 与 LLM 请求均走 Waterfall，
// 监听器 panic 不得 crash 宿主进程导致 LLM 连接中断。
func (b *EventBus) Waterfall(name EventName, ctx EventContext, next func(EventContext) error) (errRet error) {
	ctx.Name = name
	defer func() {
		if r := recover(); r != nil {
			b.logPanic(name, r)
			errRet = fmt.Errorf("event %s listener panicked: %v", name, r)
		}
	}()
	listeners := b.snapshotWaterfall(name)
	if len(listeners) == 0 {
		return next(ctx)
	}
	// 从链尾向前包装：最内层是兜底 next
	var chain func(EventContext) error = next
	for i := len(listeners) - 1; i >= 0; i-- {
		fn := listeners[i]
		inner := chain
		chain = func(c EventContext) error { return fn(c, inner) }
	}
	return chain(ctx)
}
