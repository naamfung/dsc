package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"

	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// crosshairColor 截图鼠标十字标记颜色（纯红，置于深浅背景均醒目）。
var crosshairColor = color.RGBA{R: 255, G: 0, B: 0, A: 255}

// crosshairShadow 十字标记描边色（黑，保证浅色背景下可见）。
var crosshairShadow = color.RGBA{R: 0, G: 0, B: 0, A: 255}

// gridColor 坐标网格线颜色（浅灰，不遮蔽 UI 内容）。
var gridColor = color.RGBA{R: 180, G: 180, B: 180, A: 255}

// gridLabelColor 网格坐标标注颜色（深灰）。
var gridLabelColor = color.RGBA{R: 60, G: 60, B: 60, A: 255}

// crosshairArm 十字臂长（像素，自中心向外）；centerGap 为中心留空半径，
// 避免遮挡光标指向的像素本身。
const (
	crosshairArm  = 14
	centerGap     = 3
	defaultGridSz = 100
)

// captureAndAnnotate 截屏（全屏或区域）→ 按需降采样 → 标注 → PNG 编码。
// 返回 PNG 字节、最终宽高、scale（最终/原始；模型坐标 ÷ scale = 屏幕像素）、
// 标注用的游标屏幕坐标（未标注时 nil）。
func captureAndAnnotate(p screenArgs, sw, sh int) (pngBytes []byte, w, h int, scale float64, cur *cursor, err error) {
	full := p.Width == nil || p.Height == nil || *p.Width <= 0 || *p.Height <= 0
	var img *image.RGBA
	ox, oy := 0, 0 // 截图区域左上角在屏幕坐标系的偏移
	if full {
		img, err = rbCapture()
		if err != nil {
			return nil, 0, 0, 0, nil, err
		}
	} else {
		ox = clampInt(derefInt(p.X, 0), 0, maxInt(sw-1, 0))
		oy = clampInt(derefInt(p.Y, 0), 0, maxInt(sh-1, 0))
		rw := clampInt(*p.Width, 1, maxInt(sw-ox, 1))
		rh := clampInt(*p.Height, 1, maxInt(sh-oy, 1))
		img, err = rbCapture(ox, oy, rw, rh)
		if err != nil {
			return nil, 0, 0, 0, nil, err
		}
	}
	bounds := img.Bounds()
	w, h = bounds.Dx(), bounds.Dy()

	// 降采样：最长边超过上限时等比缩小（坐标纪律：scale 随结果回传）。
	// 上限来源：env（DSC_COMPUTER_USE_MAX_DIMENSION）为默认，参数显式给定时优先。
	maxDim := envMaxDim()
	if p.MaxDimension != nil {
		maxDim = *p.MaxDimension
	}
	if maxDim < 0 {
		maxDim = 0
	}
	scale = 1
	if maxDim > 0 && (w > maxDim || h > maxDim) {
		nw, nh := w, h
		if w >= h {
			nw = maxDim
			nh = int(float64(h)*float64(maxDim)/float64(w) + 0.5)
		} else {
			nh = maxDim
			nw = int(float64(w)*float64(maxDim)/float64(h) + 0.5)
		}
		if nw >= 1 && nh >= 1 && (nw != w || nh != h) {
			img = downscaleRGBA(img, nw, nh)
			scale = float64(w) / float64(nw)
			w, h = nw, nh
		}
	}

	// 鼠标十字标记（默认开启）：屏幕坐标 → 截图坐标（区域偏移 + scale 换算）
	if p.ShowCursor == nil || *p.ShowCursor {
		if mx, my, ok := safeMousePos(); ok {
			px := int(float64(mx-ox)/scale + 0.5)
			py := int(float64(my-oy)/scale + 0.5)
			if px >= 0 && py >= 0 && px < w && py < h {
				drawCrosshair(img, px, py)
				cur = &cursor{X: mx, Y: my}
			}
		}
	}

	// 坐标网格（默认关闭）
	if p.ShowGrid != nil && *p.ShowGrid {
		gsz := defaultGridSz
		if p.GridSize != nil {
			gsz = clampInt(*p.GridSize, 10, 1000)
		}
		drawGrid(img, gsz)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, 0, 0, 0, nil, fmt.Errorf("png encode: %w", err)
	}
	return buf.Bytes(), w, h, scale, cur, nil
}

// safeMousePos recover 包裹的游标位置读取（无 DISPLAY 时 cgo 可能 panic）。
func safeMousePos() (x, y int, ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	x, y = rbMousePos()
	return x, y, true
}

// downscaleRGBA 等比降采样到 (nw, nh)（CatmullRom 高质量重采样）。
// 纯函数（输入不变量由调用方保证 nw/nh >= 1），单测覆盖。
func downscaleRGBA(src *image.RGBA, nw, nh int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
	return dst
}

// drawCrosshair 在 (cx, cy) 画红色十字（黑色描边、中心留空）。
// 纯函数，单测断言臂上像素为红、中心 gap 为空、描边为黑。
func drawCrosshair(img *image.RGBA, cx, cy int) {
	b := img.Bounds()
	set := func(x, y int, c color.RGBA) {
		if x >= b.Min.X && x < b.Max.X && y >= b.Min.Y && y < b.Max.Y {
			img.SetRGBA(x, y, c)
		}
	}
	for d := centerGap + 1; d <= crosshairArm; d++ {
		// 黑色描边（四方向各偏移 1px 的正交线 + 主色）
		set(cx+d, cy, crosshairColor)
		set(cx-d, cy, crosshairColor)
		set(cx, cy+d, crosshairColor)
		set(cx, cy-d, crosshairColor)
	}
	// 描边：臂端与两侧各一圈黑（简化为端点帽）
	set(cx+crosshairArm+1, cy, crosshairShadow)
	set(cx-crosshairArm-1, cy, crosshairShadow)
	set(cx, cy+crosshairArm+1, crosshairShadow)
	set(cx, cy-crosshairArm-1, crosshairShadow)
}

// drawGrid 以 gsz 为间距画浅灰网格线，并在每条纵线顶端 / 横线左端标注坐标值。
func drawGrid(img *image.RGBA, gsz int) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	for x := gsz; x < w; x += gsz {
		for y := 0; y < h; y++ {
			img.SetRGBA(x, y, gridColor)
		}
		drawLabel(img, x+2, 2, itoa(x))
	}
	for y := gsz; y < h; y += gsz {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, gridColor)
		}
		drawLabel(img, 2, y+2, itoa(y))
	}
}

// drawLabel 用 basicfont 7x13 在 (x, y) 画左对齐文本（越界像素静默裁剪）。
func drawLabel(img *image.RGBA, x, y int, s string) {
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(gridLabelColor),
		Face: basicfont.Face7x13,
		Dot:  fixed.P(x, y+13),
	}
	d.DrawString(s)
}

// ---------------------------------------------------------------------------
// 小工具（纯函数，供 handler 与测试共用）
// ---------------------------------------------------------------------------

func clampInt(v, lo, hi int) int {
	if hi < lo {
		hi = lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func derefInt(p *int, def int) int {
	if p == nil {
		return def
	}
	return *p
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func itoa(v int) string {
	return fmt.Sprintf("%d", v)
}
