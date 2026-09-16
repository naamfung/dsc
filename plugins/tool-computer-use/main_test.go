package main

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// 纯函数单测（无 X11 依赖）
// ---------------------------------------------------------------------------

func TestClampInt(t *testing.T) {
	cases := []struct{ v, lo, hi, want int }{
		{5, 0, 10, 5},
		{-1, 0, 10, 0},
		{99, 0, 10, 10},
		{3, 5, 2, 5}, // hi<lo 时退化为 lo
	}
	for _, c := range cases {
		if got := clampInt(c.v, c.lo, c.hi); got != c.want {
			t.Fatalf("clampInt(%d,%d,%d) = %d, want %d", c.v, c.lo, c.hi, got, c.want)
		}
	}
}

func TestNormalizeKey(t *testing.T) {
	cases := map[string]string{
		"Enter": "enter", "RETURN": "enter", "escape": "esc", "ESC": "esc",
		"Ctrl": "ctrl", "CONTROL": "ctrl", "Win": "cmd", "Meta": "cmd",
		"spacebar": "space", "PGDN": "pagedown", "plus": "=", "a": "a", "f1": "f1",
	}
	for in, want := range cases {
		got, err := normalizeKey(in)
		if err != nil || got != want {
			t.Fatalf("normalizeKey(%q) = (%q,%v), want %q", in, got, err, want)
		}
	}
	if _, err := normalizeKey("  "); err == nil {
		t.Fatal("normalizeKey(\"\") 应报错")
	}
}

func TestNormalizeButton(t *testing.T) {
	if b, _ := normalizeButton(""); b != "left" {
		t.Fatalf("缺省按钮 = %q, want left", b)
	}
	if b, _ := normalizeButton("CENTER"); b != "middle" {
		t.Fatalf("center 归一 = %q, want middle", b)
	}
	if b, _ := normalizeButton("Right"); b != "right" {
		t.Fatalf("Right = %q", b)
	}
	if _, err := normalizeButton("wheel"); err == nil {
		t.Fatal("未知按钮应报错")
	}
}

func TestDrawCrosshair(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 40, 40))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	cx, cy := 20, 20
	drawCrosshair(img, cx, cy)
	isRed := func(x, y int) bool {
		r, g, b, _ := img.At(x, y).RGBA()
		return r>>8 == 255 && g>>8 == 0 && b>>8 == 0
	}
	// 四臂上应有红色像素
	if !isRed(cx+crosshairArm, cy) || !isRed(cx-crosshairArm, cy) || !isRed(cx, cy+crosshairArm) || !isRed(cx, cy-crosshairArm) {
		t.Fatal("十字臂端应为红色")
	}
	// 中心 gap 内不应有红色（不遮挡指向像素）
	if isRed(cx, cy) || isRed(cx+centerGap, cy) {
		t.Fatal("中心留空区不应为红")
	}
	// 描边帽应为黑
	r, g, b, _ := img.At(cx+crosshairArm+1, cy).RGBA()
	if r>>8 != 0 || g>>8 != 0 || b>>8 != 0 {
		t.Fatalf("描边应为黑，got (%d,%d,%d)", r>>8, g>>8, b>>8)
	}
}

func TestDrawGrid(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 250, 250))
	drawGrid(img, 100)
	isGray := func(x, y int) bool {
		r, g, b, _ := img.At(x, y).RGBA()
		return r>>8 == 180 && g>>8 == 180 && b>>8 == 180
	}
	if !isGray(100, 50) || !isGray(50, 100) || !isGray(200, 200) {
		t.Fatal("网格线应在 100/200 倍数处")
	}
	if isGray(99, 99) {
		t.Fatal("非网格处不应被涂色")
	}
	// 顶部标注区应存在深色像素（数字标签）
	found := false
	for x := 100; x < 140 && !found; x++ {
		for y := 0; y < 16 && !found; y++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r>>8 == 60 && g>>8 == 60 && b>>8 == 60 {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("网格纵线顶部应存在坐标标注像素")
	}
}

func TestDownscaleRGBA(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 100, 50))
	dst := downscaleRGBA(src, 50, 25)
	if dst.Bounds().Dx() != 50 || dst.Bounds().Dy() != 25 {
		t.Fatalf("降采样尺寸 = %v", dst.Bounds())
	}
}

func TestEnvMaxDim(t *testing.T) {
	t.Setenv(envMaxDimension, "500")
	if got := envMaxDim(); got != 500 {
		t.Fatalf("envMaxDim = %d, want 500", got)
	}
	t.Setenv(envMaxDimension, "0")
	if got := envMaxDim(); got != 0 {
		t.Fatalf("envMaxDim(0) = %d, want 0（禁用）", got)
	}
	t.Setenv(envMaxDimension, "bogus")
	if got := envMaxDim(); got != defaultMaxDimension {
		t.Fatalf("非法值应回退默认，got %d", got)
	}
	os.Unsetenv(envMaxDimension)
	if got := envMaxDim(); got != defaultMaxDimension {
		t.Fatalf("未设置应取默认，got %d", got)
	}
}

func TestScreenResultJSON(t *testing.T) {
	r := screenResult{baseResult: baseResult{Success: true, Tool: "computer_use_screen"}, Width: 1280, Height: 800, Scale: 1, Bytes: 10, ScreenW: 1280, ScreenH: 800}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var back screenResult
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.Width != r.Width || back.Success != true {
		t.Fatalf("JSON 往返失配: %+v", back)
	}
	// Cursor 为 omitempty：零值不出现
	if bytes.Contains(b, []byte("cursor")) {
		t.Fatalf("omitempty 字段不应出现: %s", b)
	}
}

func TestPNGRoundtrip(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	drawCrosshair(img, 4, 4)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	dec, err := png.Decode(&buf)
	if err != nil {
		t.Fatalf("PNG 解码失败: %v", err)
	}
	if dec.Bounds().Dx() != 8 {
		t.Fatalf("解码尺寸失配: %v", dec.Bounds())
	}
	// 颜色断言用 RGBA 结构（crosshairColor 常量回归）
	if crosshairColor != (color.RGBA{R: 255, G: 0, B: 0, A: 255}) {
		t.Fatal("十字标记颜色应为纯红")
	}
}

// ---------------------------------------------------------------------------
// X11 e2e（需要真实 DISPLAY；无显示环境自动跳过：
//   xvfb-run -a go test ./... ）
// ---------------------------------------------------------------------------

func requireDisplay(t *testing.T) {
	t.Helper()
	if os.Getenv("DISPLAY") == "" {
		t.Skip("需要 DISPLAY；请用 xvfb-run -a go test 运行本用例")
	}
}

func TestXScreenSizeAndCapture(t *testing.T) {
	requireDisplay(t)
	w, h := rbScreenSize()
	if w <= 0 || h <= 0 {
		t.Fatalf("屏幕尺寸异常: %dx%d", w, h)
	}
	pngBytes, gw, gh, scale, _, err := captureAndAnnotate(screenArgs{}, w, h)
	if err != nil {
		t.Fatalf("截屏失败: %v", err)
	}
	if gw != w || gh != h {
		t.Fatalf("截屏尺寸 = %dx%d, want %dx%d", gw, gh, w, h)
	}
	if scale != 1 {
		t.Fatalf("小屏不应降采样，scale = %v", scale)
	}
	dec, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		t.Fatalf("PNG 解码失败: %v", err)
	}
	if dec.Bounds().Dx() != w {
		t.Fatalf("解码宽 = %d, want %d", dec.Bounds().Dx(), w)
	}
}

func TestXRegionCaptureAndScale(t *testing.T) {
	requireDisplay(t)
	w, _ := rbScreenSize()
	big := 640
	if w >= 640 {
		big = 320 // 真实屏幕较大时取小区域走非全屏路径
	}
	half := big
	_, gw, gh, scale, _, err := captureAndAnnotate(screenArgs{X: intPtr(0), Y: intPtr(0), Width: intPtr(half), Height: intPtr(half), ShowCursor: boolPtr(false)}, w, w)
	if err != nil {
		t.Fatalf("区域截屏失败: %v", err)
	}
	if gw != half || gh != half {
		t.Fatalf("区域尺寸 = %dx%d, want %dx%d", gw, gh, half, half)
	}
	if scale != 1 {
		t.Fatalf("小区域不应降采样，scale = %v", scale)
	}
}

func TestXMouseMoveReadback(t *testing.T) {
	requireDisplay(t)
	rbMoveMouse(100, 100)
	x, y := rbMousePos()
	// XTEST 注入为异步生效，Xvfb 下通常即时；给一点宽限
	if x != 100 || y != 100 {
		rbMilliSleep(50)
		x, y = rbMousePos()
	}
	if x != 100 || y != 100 {
		t.Fatalf("游标回读 = (%d,%d), want (100,100)", x, y)
	}
}

func TestXKeyTapNoError(t *testing.T) {
	requireDisplay(t)
	if err := rbKeyTap("enter"); err != nil {
		t.Fatalf("KeyTap enter 失败: %v", err)
	}
	if err := rbKeyTap("c", []string{"ctrl"}); err != nil {
		t.Fatalf("KeyTap ctrl+c 失败: %v", err)
	}
}

func TestXScreenHandlerEnvelope(t *testing.T) {
	requireDisplay(t)
	args := json.RawMessage(`{"show_grid": true}`)
	out, err := screenHandler(context.Background(), args)
	if err != nil {
		t.Fatalf("screenHandler: %v", err)
	}
	var r screenResult
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("信封解析: %v", err)
	}
	if !r.Success || r.Tool != "computer_use_screen" || r.Width != 1280 || r.Height != 800 {
		t.Fatalf("信封字段异常: %+v", r)
	}
	if r.Bytes == 0 {
		t.Fatalf("字节数缺失: %+v", r)
	}
	// 插件不落盘：结果信封不含任何文件路径字段（截图字节只经 data URL 回传，
	// 落盘由宿主入库口按内容寻址管理）
	if strings.Contains(out, "saved_file") || strings.Contains(out, "image_file") {
		t.Fatalf("信封不应含落盘字段: %s", out)
	}
	imgs := screenImages(context.Background(), args, out)
	if len(imgs) != 1 || !strings.HasPrefix(imgs[0], "data:image/png;base64,") {
		t.Fatalf("ImagesFn 应产出 PNG data URL: %d", len(imgs))
	}
}

func intPtr(v int) *int    { return &v }
func boolPtr(v bool) *bool { return &v }
