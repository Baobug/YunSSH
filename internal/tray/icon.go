// Package tray 提供 Windows 托盘常驻能力。
package tray

import (
	"bytes"
	"image"
	"image/color"
	"math"
)

// IconSize 是图标的边长（像素）。
const IconSize = 32

// IconColors 定义图标配色，便于统一调整。
var (
	// background 深蓝，取自设计文档中用于「结构层」的主色。
	background = color.RGBA{R: 0x0C, G: 0x44, B: 0x7C, A: 0xFF}
	// foreground 纯白，保证在深浅任务栏上都清晰。
	foreground = color.RGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
)

// BuildIcon 生成托盘图标的 ICO 数据。
//
// 用代码绘制而不是嵌入二进制资源，好处有二：仓库里不放二进制文件，
// 以及配色和形状可直接改这里的常量。
//
// 输出为传统 BMP 格式的 ICO（32bpp BGRA + AND 掩码），
// 而非 Vista 起支持的 PNG 内嵌格式——后者的兼容性取决于调用方如何解析。
func BuildIcon() []byte {
	img := image.NewRGBA(image.Rect(0, 0, IconSize, IconSize))

	drawRoundedBackground(img)
	drawPrompt(img)

	return encodeICO(img)
}

// drawRoundedBackground 填充一个圆角方形背景。
func drawRoundedBackground(img *image.RGBA) {
	const radius = 7

	for y := 0; y < IconSize; y++ {
		for x := 0; x < IconSize; x++ {
			if insideRoundedRect(x, y, IconSize, IconSize, radius) {
				img.SetRGBA(x, y, background)
			}
		}
	}
}

// insideRoundedRect 判断像素是否落在圆角矩形内。
func insideRoundedRect(x, y, w, h, r int) bool {
	// 把点钳制到「圆角圆心」构成的矩形上，再比较距离。
	cx, cy := x, y
	switch {
	case x < r:
		cx = r
	case x > w-1-r:
		cx = w - 1 - r
	}
	switch {
	case y < r:
		cy = r
	case y > h-1-r:
		cy = h - 1 - r
	}

	dx, dy := x-cx, y-cy
	return dx*dx+dy*dy <= r*r
}

// drawPrompt 画一个终端提示符样的 ">" 折线。
func drawPrompt(img *image.RGBA) {
	const (
		left   = 11
		right  = 21
		top    = 9
		mid    = 16
		bottom = 23
	)
	const halfStroke = 1.9

	drawThickLine(img, left, top, right, mid, halfStroke)
	drawThickLine(img, right, mid, left, bottom, halfStroke)
}

// drawThickLine 以给定半径的圆头笔刷绘制线段。
//
// 32x32 的尺寸下逐像素做点线距离判断完全够用，不需要 Bresenham。
func drawThickLine(img *image.RGBA, x1, y1, x2, y2 int, halfStroke float64) {
	pad := int(math.Ceil(halfStroke)) + 1

	minX := max(0, min(x1, x2)-pad)
	maxX := min(IconSize-1, max(x1, x2)+pad)
	minY := max(0, min(y1, y2)-pad)
	maxY := min(IconSize-1, max(y1, y2)+pad)

	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			d := distanceToSegment(
				float64(x), float64(y),
				float64(x1), float64(y1),
				float64(x2), float64(y2),
			)
			if d <= halfStroke {
				img.SetRGBA(x, y, foreground)
			}
		}
	}
}

// distanceToSegment 返回点到线段的最短距离。
func distanceToSegment(px, py, x1, y1, x2, y2 float64) float64 {
	dx, dy := x2-x1, y2-y1
	if dx == 0 && dy == 0 {
		return math.Hypot(px-x1, py-y1)
	}

	// 把点投影到线段所在直线上，参数 t 限制在 [0,1] 内。
	t := ((px-x1)*dx + (py-y1)*dy) / (dx*dx + dy*dy)
	t = math.Max(0, math.Min(1, t))

	return math.Hypot(px-(x1+t*dx), py-(y1+t*dy))
}

// encodeICO 把图像编码为 ICO。
//
// 结构：ICONDIR + ICONDIRENTRY + BITMAPINFOHEADER + XOR 位图 + AND 掩码。
// 注意 DIB 的高度要写成实际高度的两倍，因为其中包含了 AND 掩码部分。
func encodeICO(img *image.RGBA) []byte {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()

	// 32bpp BGRA，行内本身就是 4 字节对齐
	xorSize := w * h * 4
	// AND 掩码每行按 4 字节对齐，每像素 1 bit
	andPitch := ((w + 31) / 32) * 4
	andSize := andPitch * h

	var buf bytes.Buffer

	// --- ICONDIR ---
	writeU16(&buf, 0) // 保留字段
	writeU16(&buf, 1) // 类型：1 = 图标
	writeU16(&buf, 1) // 图像数量

	// --- ICONDIRENTRY ---
	buf.WriteByte(byte(w)) // 宽度（0 表示 256）
	buf.WriteByte(byte(h)) // 高度
	buf.WriteByte(0)       // 调色板颜色数，真彩色为 0
	buf.WriteByte(0)       // 保留字段
	writeU16(&buf, 1)      // 颜色平面数
	writeU16(&buf, 32)     // 每像素位数
	writeU32(&buf, uint32(40+xorSize+andSize))
	writeU32(&buf, 22) // 图像数据偏移 = ICONDIR(6) + ICONDIRENTRY(16)

	// --- BITMAPINFOHEADER ---
	writeU32(&buf, 40)         // 结构体大小
	writeI32(&buf, int32(w))   // 宽度
	writeI32(&buf, int32(h*2)) // 高度（含掩码，故为两倍）
	writeU16(&buf, 1)          // 平面数
	writeU16(&buf, 32)         // 每像素位数
	writeU32(&buf, 0)          // 压缩方式：0 = BI_RGB
	writeU32(&buf, uint32(xorSize))
	writeU32(&buf, 0) // 水平分辨率
	writeU32(&buf, 0) // 垂直分辨率
	writeU32(&buf, 0) // 调色板颜色数
	writeU32(&buf, 0) // 重要颜色数

	// --- XOR 位图：自下而上，BGRA 顺序 ---
	for y := h - 1; y >= 0; y-- {
		for x := 0; x < w; x++ {
			c := img.RGBAAt(x, y)
			buf.WriteByte(c.B)
			buf.WriteByte(c.G)
			buf.WriteByte(c.R)
			buf.WriteByte(c.A)
		}
	}

	// --- AND 掩码 ---
	// 32bpp 图标的透明度由 alpha 通道决定，掩码留 0 即可；
	// 但结构上必须存在，否则部分解析器会读越界。
	mask := make([]byte, andSize)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if img.RGBAAt(x, h-1-y).A == 0 {
				mask[y*andPitch+x/8] |= 0x80 >> (uint(x) % 8)
			}
		}
	}
	buf.Write(mask)

	return buf.Bytes()
}

func writeU16(buf *bytes.Buffer, v uint16) {
	buf.WriteByte(byte(v))
	buf.WriteByte(byte(v >> 8))
}

func writeU32(buf *bytes.Buffer, v uint32) {
	buf.WriteByte(byte(v))
	buf.WriteByte(byte(v >> 8))
	buf.WriteByte(byte(v >> 16))
	buf.WriteByte(byte(v >> 24))
}

func writeI32(buf *bytes.Buffer, v int32) {
	writeU32(buf, uint32(v))
}
