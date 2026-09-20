// SPDX-FileCopyrightText: 2026 Zhou Tianbo (Baobug)
// SPDX-License-Identifier: Apache-2.0

// Package appicon 用代码生成 YunSSH 的应用图标。
//
// 图标完全由参数光栅化得到，仓库里因此不需要存放任何图片文件；
// 配色、圆角、字形都在这里直接调整。
//
// 对外提供两个产物：
//
//   - ICO：覆盖各档尺寸的多尺寸图标，托盘取它来显示。
//   - ResourceObject：把图标塞进可执行文件的 Windows 资源对象，
//     由 go build 自动并入 exe。
package appicon

import (
	"bytes"
	"image/color"
	"image/png"
	"math"
)

// 配色。底色取自设计文档中用于「结构层」的主色。
//
// 用 NRGBA 而非 RGBA：绘制时算出来的是「直通 alpha」的颜色，
// 而 color.RGBA 的语义是已预乘 alpha，直接拿来存会被重复乘一次。
var (
	base = color.NRGBA{R: 0x0C, G: 0x44, B: 0x7C, A: 0xFF} // 深蓝底
	mark = color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF} // 白色提示符
)

// Sizes 是 ICO 中收录的全部边长，从小到大。
//
// 覆盖了 Windows 需要的主要档位：托盘槽位在 100% / 125% / 150% / 200%
// 缩放下分别是 16 / 20 / 24 / 32，列表视图和任务栏用 32 / 48，
// 资源管理器的「特大图标」用 256。
var Sizes = []int{16, 20, 24, 32, 48, 64, 128, 256}

// 几何参数一律按边长等比推导，32px 及以下再叠一层光学补偿。
//
// 屏幕上的实际像素越少，等比缩下来的字形就越显单薄、折线转角也越容易糊。
// 所以小尺寸同时放大字形、加粗笔画——只加粗会让内角糊成一团，只放大又显得
// 飘。补偿量随边长线性衰减，到 32px 归零，也就是完全回到原始设计。
const (
	radiusRatio     = 0.219 // 圆角半径 / 边长
	glyphHalfWidth  = 0.156 // 折线半宽 / 边长
	glyphHalfHeight = 0.219 // 折线半高 / 边长
	strokeRatio     = 0.119 // 笔画宽度 / 边长

	smallFullSize  = 16   // 补偿取满值的边长
	smallMaxSize   = 32   // 补偿衰减到 0 的边长
	widthBoost     = 0.13 // 满补偿时字形半宽的增幅
	strokeBoostAmt = 0.25 // 满补偿时笔画宽度的增幅
)

// pngThreshold 是改用 PNG 压缩存储的最小边长。
//
// Vista 之后的 Windows 都认 ICO 内嵌 PNG。256x256 的位图条目要 260KB，
// 压成 PNG 只要几 KB，值得；更小的尺寸位图本身不大，就不折腾了。
const pngThreshold = 256

// ICO 返回覆盖 Sizes 全部尺寸的图标数据。
func ICO() []byte {
	return encodeICO(images())
}

// images 逐个尺寸光栅化，并把结果编码成 ICO 的图像条目。
func images() []imageEntry {
	out := make([]imageEntry, 0, len(Sizes))
	for _, size := range Sizes {
		img := layoutFor(size).render()
		if size >= pngThreshold {
			var buf bytes.Buffer
			if err := png.Encode(&buf, img); err != nil {
				// 这里只可能因为内存不足失败，没有可恢复的余地
				panic("appicon: 编码 PNG 失败: " + err.Error())
			}
			out = append(out, imageEntry{width: size, height: size, data: buf.Bytes()})
			continue
		}
		out = append(out, imageEntry{width: size, height: size, data: encodeDIB(img)})
	}
	return out
}

// layout 描述图标在某个边长下的几何参数，单位均为像素。
type layout struct {
	size   int
	radius float64 // 圆角半径
	left   float64 // 折线两个端点的 x
	right  float64 // 折线拐点的 x
	top    float64 // 上端点 y
	mid    float64 // 拐点 y
	bottom float64 // 下端点 y
	stroke float64 // 笔画宽度
}

// layoutFor 按边长推导绘制参数。
func layoutFor(size int) layout {
	w := float64(size)

	// 补偿系数：16px 及以下取满值，32px 及以上归零
	boost := float64(smallMaxSize-size) / float64(smallMaxSize-smallFullSize)
	boost = math.Max(0, math.Min(1, boost))

	halfWidth := glyphHalfWidth * (1 + widthBoost*boost)
	stroke := strokeRatio * (1 + strokeBoostAmt*boost)

	return layout{
		size:   size,
		radius: radiusRatio * w,
		left:   w/2 - halfWidth*w,
		right:  w/2 + halfWidth*w,
		top:    w/2 - glyphHalfHeight*w,
		mid:    w / 2,
		bottom: w/2 + glyphHalfHeight*w,
		stroke: stroke * w,
	}
}
