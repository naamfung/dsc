// computer-use 插件：基于 robotgo 的桌面观察与操作（Computer Use）工具集。
//
// 工具面（computer_use_ 前缀）：screen（截图）、click、move、drag、scroll、
// type、key、paste、cursor_position、screen_size、check（可用性自检）。
//
// 截图回传：经 dsc.Tool.ImagesFn 产出 data URL，随 ExecuteToolResponse.images
// 送回宿主；宿主入库口将其折算为内容寻址引用（dsc-shot://，字节存宿主
// temp/screenshots/，24 小时过期）后进会话历史并投影给视觉模型（Anthropic 内嵌
// tool_result 图像块；OpenAI 在工具消息段后接 user 图像消息）。插件不自行落盘，
// 不在用户 workspace 留任何产物。
//
// 坐标纪律（对齐上游 computer-use 实践）：截图即坐标系——工具参数 x/y 一律取自
// 最近一次 computer_use_screen 返回的截图（像素、左上原点）；结果 JSON 携带 scale，
// scale < 1 时模型须先把截图坐标除以 scale 换算回屏幕像素。
//
// Linux 构建依赖：gcc 与 X11 开发头文件（libx11-dev / libxtst-dev / libxi-dev）、
// libXtst 链接库；运行期需要可用 DISPLAY（X11，Wayland 下能力受限）。
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	dsc "dsc-sdk"
	"github.com/go-vgo/robotgo"
)

// robotgo 薄封装：var 间接层便于单测注入替身（不触达真实 X11）。
var (
	rbCapture     = robotgo.Capture
	rbScreenSize  = robotgo.GetScreenSize
	rbSysScale    = robotgo.SysScale
	rbDisplaysNum = robotgo.DisplaysNum
	rbMousePos    = robotgo.GetMousePos
	rbMoveClick   = robotgo.MoveClick
	rbClick       = robotgo.Click
	rbMoveMouse   = robotgo.MoveMouse
	rbMoveSmooth  = robotgo.MoveMouseSmooth
	rbScrollDir   = robotgo.ScrollDir
	rbTypeStr     = robotgo.TypeStr
	rbKeyTap      = robotgo.KeyTap
	rbMouseDown   = robotgo.MouseDown
	rbMouseUp     = robotgo.MouseUp
	rbWriteAll    = robotgo.WriteAll
	rbReadAll     = robotgo.ReadAll
	rbMilliSleep  = robotgo.MilliSleep
)

// dragPressDelayMS 拖拽按下后与移动前的停顿（让目标应用吃住按下态）。
const dragPressDelayMS = 120

// defaultMaxDimension 截图默认最长边上限（0=不限）。与宿主投影上限
// （core.DefaultProjectionMaxSide）对齐：Anthropic 视觉最优分辨率——更大的图
// 先等比降采样到最长边 1568 再进模型，token 与延迟双优；插件侧一次降采样后
// 投影链路零二次缩放（坐标纪律：scale 随结果回传，模型坐标 ÷ scale = 屏幕像素）。
const defaultMaxDimension = 1568

// envMaxDimension 环境变量名：覆盖截图最长边（"0" 表示禁用降采样）。
const envMaxDimension = "DSC_COMPUTER_USE_MAX_DIMENSION"

// guidanceMsg Computer Use 使用约定（ListContext 注入 system prompt；
// 观察→动作→验证纪律对齐上游 computer-use 驱动的系统提示实践）。
const guidanceMsg = `COMPUTER USE（桌面观察与操作）约定：
- computer_use_screen 返回的截图即当前动作坐标系（像素、左上原点）。click/move/drag/scroll 的 x/y 一律取自最近一次截图；结果 JSON 的 scale<1 时，先把截图坐标除以 scale 再传参。
- 先观察、后动作、再验证：截图定位 → 执行操作 → 再次截图确认效果。点击未生效时先重新截图核实，不要盲目重复同一动作。
- 截图默认在鼠标位置画红色十字标记（show_cursor=false 关闭）；show_grid=true 可叠加坐标网格辅助读数。
- 键盘输入前确认目标窗口已聚焦：必要时先点击目标区域获得焦点，再 type/key/paste。
- 桌面是共享资源：其他会话与程序可能同时改变它；动作一经送达不可回滚，取消调用不撤销已生效的输入。`

func main() {
	sdk := dsc.New(dsc.Config{
		Name:    "computer-use",
		Version: "1.0.0",
		Type:    dsc.TypeTool,
		Provides: map[string]string{
			"computer-use": "true",
		},
	})

	sdk.Tool(dsc.Tool{
		Name:        "computer_use_screen",
		Description: "Capture the screen (full display or a region) and return it as a PNG image attached to the result. The screenshot defines the coordinate frame for all other computer_use_* tools: pick x/y from the latest screenshot. Optionally draws a cursor crosshair and a coordinate grid to help ground pixel coordinates.",
		Schema:      json.RawMessage(screenSchema),
		Context:     guidanceMsg,
		Handler:     screenHandler,
		ImagesFn:    screenImages,
		ViewFn: func(ctx context.Context, args json.RawMessage, result string) (json.RawMessage, error) {
			return screenView(result)
		},
	})

	sdk.Tool(dsc.Tool{
		Name:        "computer_use_click",
		Description: "Move the mouse to (x, y) and click. Coordinates are pixels from the latest computer_use_screen screenshot (left-top origin). Use button for left/right/middle and double=true for double click; omit x/y to click at the current cursor position.",
		Schema:      json.RawMessage(clickSchema),
		Handler:     clickHandler,
		ViewFn: func(ctx context.Context, args json.RawMessage, result string) (json.RawMessage, error) {
			return actionView(result)
		},
	})

	sdk.Tool(dsc.Tool{
		Name:        "computer_use_move",
		Description: "Move the mouse cursor to absolute (x, y) in screenshot pixel coordinates without clicking. smooth=true glides the cursor (human-like) instead of jumping.",
		Schema:      json.RawMessage(moveSchema),
		Handler:     moveHandler,
		ViewFn:      actionViewFn,
	})

	sdk.Tool(dsc.Tool{
		Name:        "computer_use_drag",
		Description: "Press the mouse button at (from_x, from_y), drag to (to_x, to_y) and release. Coordinates are screenshot pixels. Use for moving windows, selecting text, sliders and canvas strokes.",
		Schema:      json.RawMessage(dragSchema),
		Handler:     dragHandler,
		ViewFn:      actionViewFn,
	})

	sdk.Tool(dsc.Tool{
		Name:        "computer_use_scroll",
		Description: "Scroll the mouse wheel in a direction (up/down/left/right) by amount of wheel clicks (default 3). Optionally move to (x, y) first so the scroll lands under the cursor.",
		Schema:      json.RawMessage(scrollSchema),
		Handler:     scrollHandler,
		ViewFn:      actionViewFn,
	})

	sdk.Tool(dsc.Tool{
		Name:        "computer_use_type",
		Description: "Type UTF-8 text (CJK supported) into the focused window via unicode input. Focus the target field first (click it) if unsure. delay_ms adds a per-character delay.",
		Schema:      json.RawMessage(typeSchema),
		Handler:     typeHandler,
		ViewFn:      actionViewFn,
	})

	sdk.Tool(dsc.Tool{
		Name:        "computer_use_key",
		Description: "Press a key (e.g. 'enter', 'esc', 'tab', 'space', 'f1', 'a', 'up') optionally with modifiers ['ctrl','alt','shift','cmd']. Key names are normalized (e.g. 'return'->'enter', 'escape'->'esc').",
		Schema:      json.RawMessage(keySchema),
		Handler:     keyHandler,
		ViewFn:      actionViewFn,
	})

	sdk.Tool(dsc.Tool{
		Name:        "computer_use_paste",
		Description: "Write text to the clipboard and paste it into the focused window (clipboard + ctrl+v). Prefer over computer_use_type for long or CJK text in apps that drop unicode input events.",
		Schema:      json.RawMessage(pasteSchema),
		Handler:     pasteHandler,
		ViewFn:      actionViewFn,
	})

	sdk.Tool(dsc.Tool{
		Name:        "computer_use_cursor_position",
		Description: "Return the current mouse cursor position in screen pixel coordinates (left-top origin).",
		Schema:      json.RawMessage(emptySchema),
		Handler:     cursorHandler,
		ViewFn:      actionViewFn,
	})

	sdk.Tool(dsc.Tool{
		Name:        "computer_use_screen_size",
		Description: "Return the main display size in pixels, the DPI scale factor and the number of displays.",
		Schema:      json.RawMessage(emptySchema),
		Handler:     screenSizeHandler,
		ViewFn:      actionViewFn,
	})

	sdk.Tool(dsc.Tool{
		Name:        "computer_use_check",
		Description: "Probe desktop-control availability without side effects: display reachability, screen size, clipboard helper presence. Call this first when computer_use_* tools fail unexpectedly.",
		Schema:      json.RawMessage(emptySchema),
		Handler:     checkHandler,
		ViewFn:      actionViewFn,
	})

	sdk.Serve()
}

// ---------------------------------------------------------------------------
// JSON Schema 定义
// ---------------------------------------------------------------------------

const emptySchema = `{"type":"object","properties":{}}`

const screenSchema = `{
  "type": "object",
  "properties": {
    "x": {"type": "integer", "description": "Region left (default 0)"},
    "y": {"type": "integer", "description": "Region top (default 0)"},
    "width": {"type": "integer", "description": "Region width (default full screen)"},
    "height": {"type": "integer", "description": "Region height (default full screen)"},
    "show_cursor": {"type": "boolean", "description": "Draw a red crosshair at the cursor position (default true)"},
    "show_grid": {"type": "boolean", "description": "Overlay a coordinate grid with labels (default false)"},
    "grid_size": {"type": "integer", "description": "Grid spacing in pixels (default 100, used with show_grid)"},
    "max_dimension": {"type": "integer", "description": "Downscale so the longest edge fits this many pixels; 0 disables (default 1568 via DSC_COMPUTER_USE_MAX_DIMENSION)"}
  }
}`

const clickSchema = `{
  "type": "object",
  "properties": {
    "x": {"type": "integer", "description": "Pixel x from the latest screenshot (default: current position)"},
    "y": {"type": "integer", "description": "Pixel y from the latest screenshot (default: current position)"},
    "button": {"type": "string", "enum": ["left", "right", "middle"], "description": "Mouse button (default left)"},
    "double": {"type": "boolean", "description": "Double click (default false)"}
  }
}`

const moveSchema = `{
  "type": "object",
  "properties": {
    "x": {"type": "integer", "description": "Pixel x from the latest screenshot"},
    "y": {"type": "integer", "description": "Pixel y from the latest screenshot"},
    "smooth": {"type": "boolean", "description": "Glide instead of jump (default false)"}
  },
  "required": ["x", "y"]
}`

const dragSchema = `{
  "type": "object",
  "properties": {
    "from_x": {"type": "integer", "description": "Press point x (screenshot pixels)"},
    "from_y": {"type": "integer", "description": "Press point y (screenshot pixels)"},
    "to_x": {"type": "integer", "description": "Release point x (screenshot pixels)"},
    "to_y": {"type": "integer", "description": "Release point y (screenshot pixels)"},
    "button": {"type": "string", "enum": ["left", "right", "middle"], "description": "Button held during drag (default left)"},
    "smooth": {"type": "boolean", "description": "Glide to the target (default true)"}
  },
  "required": ["from_x", "from_y", "to_x", "to_y"]
}`

const scrollSchema = `{
  "type": "object",
  "properties": {
    "direction": {"type": "string", "enum": ["up", "down", "left", "right"], "description": "Scroll direction"},
    "amount": {"type": "integer", "description": "Wheel clicks (default 3)"},
    "x": {"type": "integer", "description": "Optional: move to pixel x first"},
    "y": {"type": "integer", "description": "Optional: move to pixel y first"}
  },
  "required": ["direction"]
}`

const typeSchema = `{
  "type": "object",
  "properties": {
    "text": {"type": "string", "description": "UTF-8 text to type"},
    "delay_ms": {"type": "integer", "description": "Per-character delay in ms (default 0)"}
  },
  "required": ["text"]
}`

const keySchema = `{
  "type": "object",
  "properties": {
    "key": {"type": "string", "description": "Key name, e.g. enter/esc/tab/space/f1/a/up"},
    "modifiers": {"type": "array", "items": {"type": "string"}, "description": "Modifier keys: ctrl/alt/shift/cmd"}
  },
  "required": ["key"]
}`

const pasteSchema = `{
  "type": "object",
  "properties": {
    "text": {"type": "string", "description": "Text to write to clipboard and paste"}
  },
  "required": ["text"]
}`

// ---------------------------------------------------------------------------
// 结果信封（工具结果统一 JSON 形态；投影层 TOON 规范化后供模型消费）
// ---------------------------------------------------------------------------

type baseResult struct {
	Success bool   `json:"success"`
	Tool    string `json:"tool"`
	Error   string `json:"error,omitempty"`
}

type screenResult struct {
	baseResult
	Width   int     `json:"width"`
	Height  int     `json:"height"`
	Scale   float64 `json:"scale"`
	Bytes   int     `json:"bytes"`
	ScreenW int     `json:"screen_width"`
	ScreenH int     `json:"screen_height"`
	Cursor  *cursor `json:"cursor,omitempty"`
}

type cursor struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type pointResult struct {
	baseResult
	X int `json:"x"`
	Y int `json:"y"`
}

type sizeResult struct {
	baseResult
	Width    int     `json:"width"`
	Height   int     `json:"height"`
	Scale    float64 `json:"scale"`
	Displays int     `json:"displays"`
}

type actionResult struct {
	baseResult
	Detail string `json:"detail,omitempty"`
}

// ---------------------------------------------------------------------------
// computer_use_screen
// ---------------------------------------------------------------------------

type screenArgs struct {
	X, Y, Width, Height *int
	ShowCursor          *bool
	ShowGrid            *bool
	GridSize            *int
	MaxDimension        *int
}

// screenDataURL 缓存最近一次截图的 data URL，供 ImagesFn 取用（同一次调用内
// 顺序执行，无需加锁；ImagesFn 在 Handler 成功返回后被 SDK 调起）。
var screenDataURL string

func screenHandler(ctx context.Context, args json.RawMessage) (string, error) {
	var p screenArgs
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	sw, sh := rbScreenSize()
	res := screenResult{baseResult: baseResult{Success: true, Tool: "computer_use_screen"}, ScreenW: sw, ScreenH: sh}

	pngBytes, w, h, scale, cur, err := captureAndAnnotate(p, sw, sh)
	if err != nil {
		return "", fmt.Errorf("screen capture failed: %w", err)
	}
	res.Width, res.Height, res.Scale, res.Cursor = w, h, scale, cur
	res.Bytes = len(pngBytes)

	// 图像通道：data URL 随 ExecuteToolResponse.images 回传；宿主入库口
	// 折算为 dsc-shot:// 内容寻址引用后进会话历史与视觉模型投影。
	screenDataURL = "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes)

	out, err := json.Marshal(res)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// screenImages ImagesFn：把最近一次截图的 data URL 交给 SDK 填充
// ExecuteToolResponse.images。仅在 Handler 成功路径产出。
func screenImages(ctx context.Context, args json.RawMessage, result string) []string {
	if screenDataURL == "" {
		return nil
	}
	return []string{screenDataURL}
}

// screenView 截图结果卡片（TUI 渲染；模型路径不受影响）。
func screenView(result string) (json.RawMessage, error) {
	var r screenResult
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		return nil, nil
	}
	body := fmt.Sprintf("size: %dx%d (screen %dx%d, scale %.3f)\nbytes: %d",
		r.Width, r.Height, r.ScreenW, r.ScreenH, r.Scale, r.Bytes)
	if r.Cursor != nil {
		body += fmt.Sprintf("\ncursor: (%d,%d)", r.Cursor.X, r.Cursor.Y)
	}
	return dsc.PlainView("Screen", &dsc.ViewBadge{Text: "ok", Tone: "green"}, body), nil
}

// actionViewFn 通用动作结果卡片：只展示 success/error 与 detail。
func actionViewFn(ctx context.Context, args json.RawMessage, result string) (json.RawMessage, error) {
	return actionView(result)
}

func actionView(result string) (json.RawMessage, error) {
	var r actionResult
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		return nil, nil
	}
	badge := &dsc.ViewBadge{Text: "ok", Tone: "green"}
	if !r.Success {
		badge = &dsc.ViewBadge{Text: "error", Tone: "red"}
	}
	return dsc.PlainView("Computer", badge, r.Detail), nil
}

// ---------------------------------------------------------------------------
// 其余动作工具
// ---------------------------------------------------------------------------

func clickHandler(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		X, Y   *int
		Button string `json:"button"`
		Double bool   `json:"double"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	btn, err := normalizeButton(p.Button)
	if err != nil {
		return "", err
	}
	detail := fmt.Sprintf("click %s", btn)
	if p.Double {
		detail = fmt.Sprintf("double-click %s", btn)
	}
	if p.X != nil || p.Y != nil {
		if p.X == nil || p.Y == nil {
			return "", fmt.Errorf("x and y must be provided together")
		}
		sw, sh := rbScreenSize()
		x, y := clampInt(*p.X, 0, sw-1), clampInt(*p.Y, 0, sh-1)
		detail += fmt.Sprintf(" at (%d,%d)", x, y)
		rbMoveClick(x, y, btn, p.Double)
	} else {
		// 缺省坐标：在当前游标位置原地点击
		rbClick(btn, p.Double)
	}
	return envelope(actionResult{baseResult: baseResult{Success: true, Tool: "computer_use_click"}, Detail: detail})
}

func moveHandler(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		X, Y   int
		Smooth bool `json:"smooth"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	sw, sh := rbScreenSize()
	x, y := clampInt(p.X, 0, sw-1), clampInt(p.Y, 0, sh-1)
	if p.Smooth {
		rbMoveSmooth(x, y)
	} else {
		rbMoveMouse(x, y)
	}
	return envelope(actionResult{baseResult: baseResult{Success: true, Tool: "computer_use_move"}, Detail: fmt.Sprintf("moved to (%d,%d)", x, y)})
}

func dragHandler(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		FromX, FromY, ToX, ToY int
		Button                 string `json:"button"`
		Smooth                 *bool  `json:"smooth"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	btn, err := normalizeButton(p.Button)
	if err != nil {
		return "", err
	}
	sw, sh := rbScreenSize()
	fx, fy := clampInt(p.FromX, 0, sw-1), clampInt(p.FromY, 0, sh-1)
	tx, ty := clampInt(p.ToX, 0, sw-1), clampInt(p.ToY, 0, sh-1)
	smooth := p.Smooth == nil || *p.Smooth
	rbMoveMouse(fx, fy)
	rbMilliSleep(dragPressDelayMS / 2)
	if err := rbMouseDown(btn); err != nil {
		return "", fmt.Errorf("mouse down: %w", err)
	}
	rbMilliSleep(dragPressDelayMS)
	if smooth {
		rbMoveSmooth(tx, ty)
	} else {
		rbMoveMouse(tx, ty)
	}
	rbMilliSleep(dragPressDelayMS / 2)
	if err := rbMouseUp(btn); err != nil {
		return "", fmt.Errorf("mouse up: %w", err)
	}
	return envelope(actionResult{baseResult: baseResult{Success: true, Tool: "computer_use_drag"}, Detail: fmt.Sprintf("dragged (%d,%d) -> (%d,%d) [%s]", fx, fy, tx, ty, btn)})
}

func scrollHandler(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Direction string
		Amount    *int
		X, Y      *int
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	dir := strings.ToLower(strings.TrimSpace(p.Direction))
	switch dir {
	case "up", "down", "left", "right":
	default:
		return "", fmt.Errorf("direction must be one of up/down/left/right, got %q", p.Direction)
	}
	amount := 3
	if p.Amount != nil {
		amount = clampInt(*p.Amount, 1, 100)
	}
	if p.X != nil && p.Y != nil {
		sw, sh := rbScreenSize()
		rbMoveMouse(clampInt(*p.X, 0, sw-1), clampInt(*p.Y, 0, sh-1))
	}
	rbScrollDir(amount, dir)
	return envelope(actionResult{baseResult: baseResult{Success: true, Tool: "computer_use_scroll"}, Detail: fmt.Sprintf("scrolled %s x%d", dir, amount)})
}

func typeHandler(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Text    string
		DelayMS int `json:"delay_ms"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if p.Text == "" {
		return "", fmt.Errorf("text is required")
	}
	// TypeStr 的变参为 (pid, tm, tm1)：pid=0 表示前台全局输入
	rbTypeStr(p.Text, 0, clampInt(p.DelayMS, 0, 5000))
	return envelope(actionResult{baseResult: baseResult{Success: true, Tool: "computer_use_type"}, Detail: fmt.Sprintf("typed %d chars", len([]rune(p.Text)))})
}

func keyHandler(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Key       string
		Modifiers []string
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	key, err := normalizeKey(p.Key)
	if err != nil {
		return "", err
	}
	mods := make([]string, 0, len(p.Modifiers))
	for _, m := range p.Modifiers {
		nm, err := normalizeKey(m)
		if err != nil {
			return "", err
		}
		mods = append(mods, nm)
	}
	var kerr error
	if len(mods) > 0 {
		kerr = rbKeyTap(key, mods)
	} else {
		kerr = rbKeyTap(key)
	}
	if kerr != nil {
		return "", fmt.Errorf("key tap: %w", kerr)
	}
	detail := key
	if len(mods) > 0 {
		detail = strings.Join(mods, "+") + "+" + key
	}
	return envelope(actionResult{baseResult: baseResult{Success: true, Tool: "computer_use_key"}, Detail: "pressed " + detail})
}

func pasteHandler(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Text string
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if p.Text == "" {
		return "", fmt.Errorf("text is required")
	}
	if err := rbWriteAll(p.Text); err != nil {
		return "", fmt.Errorf("clipboard write (is xclip installed?): %w", err)
	}
	if err := rbKeyTap("v", []string{"ctrl"}); err != nil {
		return "", fmt.Errorf("paste key: %w", err)
	}
	return envelope(actionResult{baseResult: baseResult{Success: true, Tool: "computer_use_paste"}, Detail: fmt.Sprintf("pasted %d chars", len([]rune(p.Text)))})
}

func cursorHandler(ctx context.Context, args json.RawMessage) (string, error) {
	x, y := rbMousePos()
	out, err := json.Marshal(pointResult{baseResult: baseResult{Success: true, Tool: "computer_use_cursor_position"}, X: x, Y: y})
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func screenSizeHandler(ctx context.Context, args json.RawMessage) (string, error) {
	w, h := rbScreenSize()
	out, err := json.Marshal(sizeResult{baseResult: baseResult{Success: true, Tool: "computer_use_screen_size"}, Width: w, Height: h, Scale: rbSysScale(), Displays: rbDisplaysNum()})
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// checkHandler 可用性自检：显示可达性、屏幕尺寸、剪贴板助手（xclip）。
// 全程 recover 兜底——robotgo 在无 DISPLAY 时可能 panic（cgo 空指针），
// 自检工具必须把失败转成诊断信息而非崩溃插件。
func checkHandler(ctx context.Context, args json.RawMessage) (string, error) {
	res := map[string]interface{}{"success": true, "tool": "computer_use_check"}
	sw, sh, derr := safeScreenSize()
	if derr != nil {
		res["display"] = "unavailable: " + derr.Error()
	} else {
		res["display"] = "ok"
		res["screen"] = map[string]int{"width": sw, "height": sh}
	}
	if _, cerr := rbReadAll(); cerr != nil {
		res["clipboard"] = "unavailable: " + cerr.Error()
	} else {
		res["clipboard"] = "ok"
	}
	out, err := json.Marshal(res)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// safeScreenSize recover 包裹的屏幕尺寸探测。
func safeScreenSize() (w, h int, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	w, h = rbScreenSize()
	if w <= 0 || h <= 0 {
		return 0, 0, fmt.Errorf("zero screen size (no DISPLAY?)")
	}
	return w, h, nil
}

// envelope 统一出口：marshal 失败转为 error。
func envelope(v interface{}) (string, error) {
	out, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// envMaxDim 读取截图最长边配置：环境变量优先（"0"=禁用），否则默认 1568。
func envMaxDim() int {
	if v := strings.TrimSpace(os.Getenv(envMaxDimension)); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return n
		}
		log.Printf("⚠️ 无效 %s=%q，回退默认 %d", envMaxDimension, v, defaultMaxDimension)
	}
	return defaultMaxDimension
}
