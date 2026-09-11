package main

import (
	"math"
	"strings"
	"testing"
)

// TestRotationDegrees 验证从文本矩阵 a/b 推导旋转角的正确性。
func TestRotationDegrees(t *testing.T) {
	cases := []struct {
		tm   [6]float64
		want float64
	}{
		{[6]float64{1, 0, 0, 1, 0, 0}, 0},
		{[6]float64{0, 1, -1, 0, 0, 0}, 90},   // 逆时针 90°（竖排/侧注常见）
		{[6]float64{0, -1, 1, 0, 0, 0}, -90},  // 顺时针 90°
		{[6]float64{-1, 0, 0, -1, 0, 0}, 180}, // 倒置
	}
	for _, c := range cases {
		got := rotationDegrees(c.tm)
		if math.Abs(got-c.want) > 0.1 {
			t.Errorf("rotationDegrees(%v) = %v, want %v", c.tm, got, c.want)
		}
	}
}

// TestRenderLinesColumns 验证多栏识别：同行内横向间隙 ≥ fontSize*columnGapEm 时以制表符分隔。
func TestRenderLinesColumns(t *testing.T) {
	input := []textLine{
		{
			y: 100,
			frag: []textFragment{
				{x: 50, s: "hello", rot: 0, fontSize: 12},
				{x: 120, s: "world", rot: 0, fontSize: 12}, // 右端 estimated ≈ 50+5*6=80，gap=40 ≥ 18 → 栏
			},
		},
	}
	got := renderLines(input)
	if want := "hello\tworld"; strings.TrimRight(got, "\n") != want {
		t.Errorf("column render = %q, want %q", got, want)
	}

	// 同一栏内紧邻片段：以空格相连
	tight := []textLine{
		{y: 100, frag: []textFragment{{x: 50, s: "hello", rot: 0, fontSize: 12}, {x: 85, s: "world", rot: 0, fontSize: 12}}},
	}
	if got := renderLines(tight); strings.TrimRight(got, "\n") != "hello world" {
		t.Errorf("tight render = %q, want %q", got, "hello world")
	}
}

// TestRenderLinesRotated 验证旋转文本被单独分区输出，不混入正常行。
func TestRenderLinesRotated(t *testing.T) {
	input := []textLine{
		{y: 100, frag: []textFragment{{x: 50, s: "Left", rot: 0, fontSize: 12}, {x: 120, s: "Right", rot: 0, fontSize: 12}}},
		{y: 90, frag: []textFragment{{x: 10, s: "边注", rot: 90, fontSize: 12}}},
	}
	got := renderLines(input)
	if !strings.Contains(got, "Left\tRight") {
		t.Errorf("expected normal row Left<TAB>Right, got:\n%s", got)
	}
	if !strings.Contains(got, "〔rotate 90°〕") || !strings.Contains(got, "边注") {
		t.Errorf("expected rotated marker and text, got:\n%s", got)
	}
}

// TestEstFragmentWidth 验证片段宽度估算：全角≈1em、半角≈0.5em、空格≈0.3em。
func TestEstFragmentWidth(t *testing.T) {
	cjk := estFragmentWidth("中文", 10)
	ascii := estFragmentWidth("Hi", 10)
	spacey := estFragmentWidth("a b", 10)
	if cjk != 20 {
		t.Errorf("CJK width = %v, want 20", cjk)
	}
	if ascii != 10 {
		t.Errorf("ASCII width = %v, want 10", ascii)
	}
	if math.Abs(spacey-(5+3+5)) > 0.001 {
		t.Errorf("spacey width = %v, want 13", spacey)
	}
}
