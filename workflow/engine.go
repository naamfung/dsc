package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dop251/goja"
)

// newID 生成运行 id：时间戳毫秒。
func newID() string {
	return fmt.Sprintf("wf-%d", time.Now().UnixMilli())
}

// checkScriptSyntax 语法预检（Start 同步执行）：只编译包装后的脚本，不运行。
func checkScriptSyntax(script string) error {
	wrapped := "(function(){\n" + script + "\n})()"
	_, err := goja.Compile("workflow.js", wrapped, false)
	return err
}

// execute 运行协程：应用超时后结算并投递结果。
func execute(ctx context.Context, req StartRequest, id string, timeout time.Duration, ch chan<- Result) {
	defer close(ch)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if req.Events != nil {
		req.Events.OnStart(id, req.Meta)
	}
	r := settle(ctx, req, id)
	if req.Events != nil {
		req.Events.OnEnd(id, r)
	}
	ch <- r
}

// settle 执行脚本并结算：脚本语法已在 Start 校验；此处运行包装后的同步
// IIFE（agent() 同步阻塞，脚本无需 await），取消/超时经 vm.Interrupt 中断。
func settle(ctx context.Context, req StartRequest, id string) Result {
	vm := goja.New()

	var (
		mu          sync.Mutex
		agentsCount int32
		agentSeq    int
	)
	emit := func(f func(EventSink)) {
		if req.Events != nil {
			f(req.Events)
		}
	}
	panicErr := func(code string, err error) {
		panic(vm.ToValue(&RunError{Code: code, Err: err}))
	}

	// agent(prompt, options?)：同步执行一个子 agent，返回其结果文本；
	// 子 agent 失败返回 null（对齐 DSH：非基础设施失败脚本侧可见 null）。
	vm.Set("agent", func(call goja.FunctionCall) goja.Value {
		prompt := call.Argument(0).String()
		if strings.TrimSpace(prompt) == "" {
			panicErr(ErrInvalidArgument, fmt.Errorf("agent(prompt) requires a non-empty prompt"))
		}
		label := prompt
		if opt := call.Argument(1); opt != nil && opt != goja.Undefined() && !goja.IsNull(opt) {
			if o := opt.ToObject(vm); o != nil {
				if l := o.Get("label"); l != nil && l != goja.Undefined() && !goja.IsNull(l) {
					label = l.String()
				}
			}
		}
		if req.MaxTotalAgents > 0 && int(atomic.LoadInt32(&agentsCount)) >= req.MaxTotalAgents {
			panicErr(ErrAgentCap, fmt.Errorf("total agent cap %d exceeded", req.MaxTotalAgents))
		}
		mu.Lock()
		agentSeq++
		seq := agentSeq
		mu.Unlock()
		atomic.StoreInt32(&agentsCount, int32(seq))
		emit(func(s EventSink) { s.OnAgentStart(id, seq, label) })

		result, err := req.Runner.RunAgent(ctx, prompt)
		outcome := "completed"
		if err != nil {
			outcome = "failed"
		}
		emit(func(s EventSink) { s.OnAgentEnd(id, seq, outcome) })
		if err != nil {
			return goja.Null()
		}
		return vm.ToValue(result)
	})

	// phase(title)：声明了 meta.phases 时须精确匹配，否则报错。
	vm.Set("phase", func(call goja.FunctionCall) goja.Value {
		title := call.Argument(0).String()
		if strings.TrimSpace(title) == "" {
			panicErr(ErrInvalidArgument, fmt.Errorf("phase(title) requires a non-empty title"))
		}
		if len(req.Meta.Phases) > 0 {
			found := false
			for _, p := range req.Meta.Phases {
				if p.Title == title {
					found = true
					break
				}
			}
			if !found {
				panicErr(ErrInvalidArgument, fmt.Errorf("phase %q not declared in meta.phases", title))
			}
		}
		emit(func(s EventSink) { s.OnPhase(id, title) })
		return goja.Undefined()
	})

	// log(msg)：进度叙述。
	vm.Set("log", func(call goja.FunctionCall) goja.Value {
		emit(func(s EventSink) { s.OnLog(id, call.Argument(0).String()) })
		return goja.Undefined()
	})

	// args 全局：请求参数原样暴露（无参数时为空对象）。
	if req.Args != nil {
		vm.Set("args", vm.ToValue(req.Args))
	} else {
		vm.Set("args", vm.ToValue(map[string]any{}))
	}

	// 取消/超时：中断脚本执行。
	interrupt := &RunError{Code: ErrCancelled, Err: fmt.Errorf("workflow run cancelled")}
	stopInterrupt := context.AfterFunc(ctx, func() { vm.Interrupt(interrupt) })
	defer stopInterrupt()

	wrapped := "(function(){\n" + req.Script + "\n})()"
	val, err := vm.RunString(wrapped)
	if err != nil {
		return classifyError(err, agentsCount)
	}
	value, merr := materialize(val)
	if merr != nil {
		return Result{StopReason: StopError, Error: merr.Error(), AgentsStarted: int(agentsCount)}
	}
	return Result{Value: value, StopReason: StopCompleted, AgentsStarted: int(agentsCount)}
}

// classifyError 把脚本抛错/中断归类为运行结果：
// 钩子 panic 的 *RunError 原样保留其 code；取消/超时中断按 CANCELLED 归类。
func classifyError(err error, agents int32) Result {
	re := unwrapRunError(err)
	if re == nil {
		// goja 中断（vm.Interrupt）把错误包成带位置后缀的异常，按 code 前缀识别
		if strings.Contains(err.Error(), ErrCancelled) {
			re = &RunError{Code: ErrCancelled, Err: err}
		} else {
			re = &RunError{Code: "SCRIPT_RUNTIME", Err: err}
		}
	}
	reason := StopError
	if re.Code == ErrCancelled {
		reason = StopCancelled
	}
	return Result{StopReason: reason, Error: re.Error(), AgentsStarted: int(agents)}
}

// unwrapRunError 尝试从 goja 异常中还原钩子 panic 的 *RunError。
func unwrapRunError(err error) *RunError {
	if ex, ok := err.(*goja.Exception); ok {
		if v := ex.Value(); v != nil {
			if re, ok := v.Export().(*RunError); ok {
				return re
			}
		}
	}
	return nil
}

// materialize 物化脚本返回值：必须是普通 JSON 数据，否则 RESULT_UNSERIALIZABLE。
func materialize(v goja.Value) (any, error) {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return nil, nil
	}
	exported := v.Export()
	if _, err := json.Marshal(exported); err != nil {
		return nil, &RunError{Code: ErrResultUnserializable, Err: fmt.Errorf("script returned non-JSON value: %v", err)}
	}
	return exported, nil
}
