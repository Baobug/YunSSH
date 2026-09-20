// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

package appicon

import (
	"image"
	"image/color"
	"math"
)

// supersample 是每个像素在两个方向上的采样数，用来做边缘抗锯齿。
//
// 每个像素因此有 16 个覆盖级别，对 16px 的小图也够用；
// 再提高一档，观感提升有限而代价是采样数按平方增长。
const supersample = 4

// render 把图形光栅化成一张直通 alpha 的 RGBA 图。
func (l layout) render() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, l.size, l.size))

	step := 1.0 / supersample
	total := float64(supersample * supersample)
	half := l.stroke / 2

	for y := 0; y < l.size; y++ {
		for x := 0; x < l.size; x++ {
			var inBase, inMark int

			for sy := 0; sy < supersample; sy++ {
				for sx := 0; sx < supersample; sx++ {
					// 取子采样单元的中心，避免整张图系统性地偏移半格
					px := float64(x) + (float64(sx)+0.5)*step
					py := float64(y) + (float64(sy)+0.5)*step

					if !l.covers(px, py) {
						continue
					}
					inBase++
					if l.onMark(px, py, half) {
						inMark++
					}
				}
			}

			img.SetNRGBA(x, y, blend(float64(inBase)/total, float64(inMark)/total))
		}
	}

	return img
}

// covers 判断点是否落在圆角方形底色内。
func (l layout) covers(px, py float64) bool {
	w := float64(l.size)
	if px < 0 || py < 0 || px >= w || py >= w {
		return false
	}

	// 把点钳制到「圆角圆心」构成的矩形上，再比较距离
	r := math.Min(l.radius, w/2)
	cx := clamp(px, r, w-r)
	cy := clamp(py, r, w-r)

	dx, dy := px-cx, py-cy
	return dx*dx+dy*dy <= r*r
}

// onMark 判断点是否落在提示符折线的笔画内。
//
// 折线由两段组成：(left,top) → (right,mid) → (left,bottom)。
func (l layout) onMark(px, py, half float64) bool {
	d := math.Min(
		distanceToSegment(px, py, l.left, l.top, l.right, l.mid),
		distanceToSegment(px, py, l.right, l.mid, l.left, l.bottom),
	)
	return d <= half
}

// blend 把底色与字形两层按覆盖度合成为最终像素。
//
// baseCover 是底色对该像素的覆盖比例，也就是最终 alpha；
// markCover 是字形在其中的覆盖比例，必然不超过 baseCover。
func blend(baseCover, markCover float64) color.NRGBA {
	if baseCover <= 0 {
		return color.NRGBA{}
	}
	if markCover > baseCover {
		markCover = baseCover
	}

	// 直通 alpha：颜色是底色与字形按各自覆盖度的加权平均
	mix := func(b, m uint8) uint8 {
		v := float64(m)*markCover + float64(b)*(baseCover-markCover)
		return uint8(math.Round(v / baseCover))
	}

	return color.NRGBA{
		R: mix(base.R, mark.R),
		G: mix(base.G, mark.G),
		B: mix(base.B, mark.B),
		A: uint8(math.Round(baseCover * 255)),
	}
}

// distanceToSegment 返回点到线段的最短距离。
//
// 距离天然带圆头效果：端点外侧的采样点也算在内，笔画端头因此是半圆。
func distanceToSegment(px, py, x1, y1, x2, y2 float64) float64 {
	dx, dy := x2-x1, y2-y1
	if dx == 0 && dy == 0 {
		return math.Hypot(px-x1, py-y1)
	}

	// 把点投影到线段所在直线上，参数 t 限制在 [0,1] 内
	t := ((px-x1)*dx + (py-y1)*dy) / (dx*dx + dy*dy)
	t = math.Max(0, math.Min(1, t))

	return math.Hypot(px-(x1+t*dx), py-(y1+t*dy))
}

func clamp(v, lo, hi float64) float64 {
	return math.Max(lo, math.Min(hi, v))
}
