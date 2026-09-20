// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

// Command icondump 把 ICO 里的图像拆出来落到 PNG，好肉眼确认画得对不对。
//
// 「图标资源存在」和「图标能画出来」是两件事：数据坏了 Shell 同样无从显示，
// 于是 Windows 给出一张白纸。pecheck 只能证明前者，这个工具补的是后者。
//
// 用法：
//
//	go run ./tools/genicon          # 先生成 app.ico
//	go run ./tools/icondump         # 默认读仓库根的 app.ico
//	go run ./tools/icondump <路径.ico>
package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

// checker 是背景色。透明像素铺在它上面，全透明的图会露出整片棋盘格。
var checker = [2]color.NRGBA{
	{R: 0xC8, G: 0xC8, B: 0xC8, A: 0xFF},
	{R: 0x8C, G: 0x8C, B: 0x8C, A: 0xFF},
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "icondump:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	src, err := icoPath(args)
	if err != nil {
		return err
	}

	raw, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	entries, err := parseICO(raw)
	if err != nil {
		return err
	}

	out := filepath.Join(filepath.Dir(src), "icondump")
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}

	fmt.Printf("源文件 %s（%d 字节，%d 个图像条目）\n\n", src, len(raw), len(entries))

	var imgs []image.Image
	var names []string
	for i, e := range entries {
		img, err := e.decode()
		if err != nil {
			return fmt.Errorf("第 %d 个条目：%w", i+1, err)
		}

		name := fmt.Sprintf("%03dx%03d.png", img.Bounds().Dx(), img.Bounds().Dy())
		flat := composite(img)
		if err := writePNG(filepath.Join(out, name), flat); err != nil {
			return err
		}

		opaque := countOpaque(img)
		total := img.Bounds().Dx() * img.Bounds().Dy()
		fmt.Printf("  %-14s 不透明像素 %5d/%5d（%5.1f%%）\n",
			name, opaque, total, 100*float64(opaque)/float64(total))

		imgs = append(imgs, img)
		names = append(names, name)
	}

	sheet := tile(imgs)
	if err := writePNG(filepath.Join(out, "sheet.png"), sheet); err != nil {
		return err
	}
	fmt.Printf("\n  sheet.png     全部 %d 档尺寸并排（每格上方标注文件名）\n", len(imgs))
	fmt.Printf("  输出目录 %s\n", out)

	if len(names) > 0 && countOpaque(imgs[0]) == 0 {
		fmt.Println("\n  ⚠ 第一档全透明——图标画出来是空的")
	}
	return nil
}

// icoPath 决定要读的 ICO：命令行给出就用给出的，否则取仓库根的 app.ico。
func icoPath(args []string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}
	root, err := moduleRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "app.ico"), nil
}

// entry 是 ICO 目录中的一项，连同它指向的数据。
type entry struct {
	width  int
	height int
	data   []byte
}

// parseICO 读出 ICO 的目录与各个条目的数据。
func parseICO(raw []byte) ([]entry, error) {
	if len(raw) < 6 {
		return nil, errors.New("文件过短，不像 ICO")
	}
	if kind := binary.LittleEndian.Uint16(raw[2:]); kind != 1 {
		return nil, fmt.Errorf("类型为 %d，不是图标（应为 1）", kind)
	}

	count := int(binary.LittleEndian.Uint16(raw[4:]))
	entries := make([]entry, 0, count)

	for i := range count {
		off := 6 + i*16
		if off+16 > len(raw) {
			return nil, fmt.Errorf("第 %d 个目录项超出文件长度", i+1)
		}
		d := raw[off:]

		// 目录里的宽高各占 1 字节，256 记作 0
		w := int(d[0])
		if w == 0 {
			w = 256
		}
		h := int(d[1])
		if h == 0 {
			h = 256
		}

		size := int(binary.LittleEndian.Uint32(d[8:]))
		at := int(binary.LittleEndian.Uint32(d[12:]))
		if at+size > len(raw) {
			return nil, fmt.Errorf("第 %d 个图像数据超出文件长度", i+1)
		}

		entries = append(entries, entry{width: w, height: h, data: raw[at : at+size]})
	}
	return entries, nil
}

// pngMagic 用于区分条目是 PNG 还是传统位图。
var pngMagic = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n'}

// decode 把一个条目解成带 alpha 的图像。
func (e entry) decode() (image.Image, error) {
	if bytes.HasPrefix(e.data, pngMagic) {
		img, err := png.Decode(bytes.NewReader(e.data))
		if err != nil {
			return nil, err
		}
		return toNRGBA(img), nil
	}
	return e.decodeDIB()
}

// decodeDIB 解传统位图条目：BITMAPINFOHEADER + BGRA 像素 + AND 掩码。
//
// 像素行自下而上存放，掩码行同样如此；掩码位为 1 表示该像素透明。
func (e entry) decodeDIB() (image.Image, error) {
	if len(e.data) < 40 {
		return nil, errors.New("条目小于 BITMAPINFOHEADER")
	}
	w := int(int32(binary.LittleEndian.Uint32(e.data[4:])))
	h := int(int32(binary.LittleEndian.Uint32(e.data[8:]))) / 2 // 目录头里记的是含掩码的两倍高
	bits := binary.LittleEndian.Uint16(e.data[14:])
	if bits != 32 {
		return nil, fmt.Errorf("只处理 32bpp，实际为 %dbpp", bits)
	}

	xorOff := 40
	xorSize := w * h * 4
	if xorOff+xorSize > len(e.data) {
		return nil, errors.New("像素数据不完整")
	}

	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		// 第 y 行画面在文件里存于倒数第 y+1 行
		row := e.data[xorOff+(h-1-y)*w*4:]
		for x := range w {
			b, g, r, a := row[x*4], row[x*4+1], row[x*4+2], row[x*4+3]
			img.SetNRGBA(x, y, color.NRGBA{R: r, G: g, B: b, A: a})
		}
	}

	// 掩码：位为 1 处强制透明，带回老式渲染路径的透明度判断
	andPitch := ((w + 31) / 32) * 4
	if xorOff+xorSize+h*andPitch <= len(e.data) {
		for y := range h {
			row := e.data[xorOff+xorSize+(h-1-y)*andPitch:]
			for x := range w {
				if row[x/8]&(0x80>>(uint(x)%8)) != 0 {
					img.SetNRGBA(x, y, color.NRGBA{})
				}
			}
		}
	}

	return img, nil
}

// composite 把图像铺到棋盘格上，透明之处露出格子。
func composite(src image.Image) image.Image {
	b := src.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := range b.Dy() {
		for x := range b.Dx() {
			dst.SetNRGBA(x, y, over(toNRGBA(src).NRGBAAt(x, y), checkerAt(x, y)))
		}
	}
	return dst
}

// tile 把各尺寸自左向右排在 32px 基线上，便于一趟看全。
func tile(imgs []image.Image) image.Image {
	const (
		pad  = 8
		line = 32
	)

	width := pad
	for _, img := range imgs {
		width += img.Bounds().Dx() + pad
	}
	canvas := image.NewNRGBA(image.Rect(0, 0, width, line+pad*2))

	for y := range canvas.Bounds().Dy() {
		for x := range canvas.Bounds().Dx() {
			canvas.SetNRGBA(x, y, checkerAt(x, y))
		}
	}

	x := pad
	for _, img := range imgs {
		w := img.Bounds().Dx()
		dst := image.Rect(x, pad+line-w, x+w, pad+line)
		src := toNRGBA(img)
		for row := range w {
			for col := range w {
				c := src.NRGBAAt(col, row)
				if c.A == 0 {
					continue
				}
				canvas.SetNRGBA(dst.Min.X+col, dst.Min.Y+row, over(c, canvas.NRGBAAt(dst.Min.X+col, dst.Min.Y+row)))
			}
		}
		x += w + pad
	}
	return canvas
}

// countOpaque 数一数不完全透明的像素，用来判断「是不是画空了」。
func countOpaque(img image.Image) int {
	b := img.Bounds()
	n := 0
	for y := range b.Dy() {
		for x := range b.Dx() {
			if toNRGBA(img).NRGBAAt(x, y).A > 0 {
				n++
			}
		}
	}
	return n
}

// toNRGBA 统一转成本工具使用的格式。
func toNRGBA(img image.Image) *image.NRGBA {
	if n, ok := img.(*image.NRGBA); ok {
		return n
	}
	b := img.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := range b.Dy() {
		for x := range b.Dx() {
			dst.Set(x, y, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

// over 是标准的 source-over 合成。
func over(fg, bg color.NRGBA) color.NRGBA {
	if fg.A == 0xFF {
		return fg
	}
	a := float64(fg.A) / 255
	return color.NRGBA{
		R: uint8(float64(fg.R)*a + float64(bg.R)*(1-a)),
		G: uint8(float64(fg.G)*a + float64(bg.G)*(1-a)),
		B: uint8(float64(fg.B)*a + float64(bg.B)*(1-a)),
		A: 0xFF,
	}
}

// checkerAt 取棋盘格在 (x,y) 处的颜色。
func checkerAt(x, y int) color.NRGBA {
	const cell = 8
	return checker[((x/cell)+(y/cell))%2]
}

func writePNG(path string, img image.Image) error {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// moduleRoot 从当前目录向上找 go.mod 所在的那一层。
func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("未找到 go.mod，请在仓库内运行")
		}
		dir = parent
	}
}
