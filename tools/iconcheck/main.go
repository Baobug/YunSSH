// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

//go:build windows

// Command iconcheck 检查一个路径上的图标：Shell 认不认、画出来是什么样。
//
// 排查「图标显示空白」时，前两步只能证明资源存在：
//
//	pecheck  看 .rsrc 段里有没有 RT_ICON / RT_GROUP_ICON
//	icondump 看 ICO 里的图像画得对不对
//
// 两者都正常而问题依旧，剩下两种可能——Shell 读不出来，或者外壳缓存是旧的。
// 这个工具就是用来卡在第两者之间的：直接问 Shell「这个路径的图标是什么」，
// 并把拿到的图落到 PNG，眼见为实。
//
// 对 .lnk 同样有效：Shell 会顺着快捷方式解析到目标，正是桌面显示的那张图。
//
//	go run ./tools/iconcheck <路径> [更多路径]
//
// 每个路径会在临时目录下生成 iconcheck-<序号>-<名称>.png。
//
// 已知限制：SHGetFileInfo 需要完整的用户令牌。在受限令牌下运行（某些自动化
// 环境、服务上下文）会返回 ERROR_NO_TOKEN，此时改用文件副本或换个目录再试——
// 直接查询「桌面」命名空间下的文件尤其容易触发。
package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

var (
	shell32 = syscall.NewLazyDLL("shell32.dll")
	user32  = syscall.NewLazyDLL("user32.dll")
	gdi32   = syscall.NewLazyDLL("gdi32.dll")

	procExtractIconExW = shell32.NewProc("ExtractIconExW")
	procSHGetFileInfoW = shell32.NewProc("SHGetFileInfoW")
	procDestroyIcon    = user32.NewProc("DestroyIcon")
	procGetDC          = user32.NewProc("GetDC")
	procReleaseDC      = user32.NewProc("ReleaseDC")
	procGetIconInfo    = user32.NewProc("GetIconInfo")
	procGetObjectW     = gdi32.NewProc("GetObjectW")
	procGetDIBits      = gdi32.NewProc("GetDIBits")
	procDeleteObject   = gdi32.NewProc("DeleteObject")
)

// SHGetFileInfo 的标志位。
const (
	shgfiIcon         = 0x000000100 // 取图标句柄
	shgfiLargeIcon    = 0x000000000 // 32x32
	shgfiSmallIcon    = 0x000000001 // 16x16
	shgfiSysIconIndex = 0x000040000 // 只取系统图标索引
	dibRGBColors      = 0
)

// shFileInfo 对应 SHFILEINFO。字段顺序与大小必须与 C 侧一致。
type shFileInfo struct {
	hIcon         uintptr
	iIcon         int32
	dwAttributes  uint32
	szDisplayName [260]uint16
	szTypeName    [80]uint16
}

// iconInfo 对应 ICONINFO。Go 会自动为 uintptr 补齐对齐。
type iconInfo struct {
	fIcon    int32
	xHotspot uint32
	yHotspot uint32
	hbmMask  uintptr
	hbmColor uintptr
}

// bitmap 对应 BITMAP。
type bitmap struct {
	bmType       int32
	bmWidth      int32
	bmHeight     int32
	bmWidthBytes int32
	bmPlanes     uint16
	bmBitsPixel  uint16
	bmBits       uintptr
}

// bitmapInfoHeader 对应 BITMAPINFOHEADER。
type bitmapInfoHeader struct {
	biSize          uint32
	biWidth         int32
	biHeight        int32
	biPlanes        uint16
	biBitCount      uint16
	biCompression   uint32
	biSizeImage     uint32
	biXPelsPerMeter int32
	biYPelsPerMeter int32
	biClrUsed       uint32
	biClrImportant  uint32
}

func main() {
	paths := os.Args[1:]
	if len(paths) == 0 {
		fmt.Fprintln(os.Stderr, "用法: iconcheck <路径> [更多路径]")
		os.Exit(2)
	}

	dir, err := os.MkdirTemp("", "iconcheck-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "iconcheck:", err)
		os.Exit(1)
	}

	bad := false
	for i, path := range paths {
		fmt.Printf("=== %s ===\n", path)

		if _, err := os.Stat(path); err != nil {
			fmt.Printf("  ✗ 路径不可访问：%v\n\n", err)
			bad = true
			continue
		}

		// exe / ico / dll 才数得出图标个数；.lnk 走这条只会得到 0，故仅作参考
		fmt.Printf("  ExtractIconEx 图标数 : %d\n", countIcons(path))
		fmt.Printf("  Shell 系统图标索引   : %d\n", sysIconIndex(path))

		icon, err := shellIcon(path)
		if err != nil {
			fmt.Printf("  ✗ Shell 取不到图标：%v\n\n", err)
			bad = true
			continue
		}

		opaque, ink := analyse(icon)
		name := fmt.Sprintf("iconcheck-%02d-%s.png", i+1, safeName(filepath.Base(path)))
		out := filepath.Join(dir, name)
		if err := writePNG(out, icon); err != nil {
			fmt.Printf("  ✗ 写出 PNG 失败：%v\n\n", err)
			bad = true
			continue
		}

		fmt.Printf("  尺寸                 : %dx%d\n", icon.Bounds().Dx(), icon.Bounds().Dy())
		fmt.Printf("  不透明像素           : %d / %d\n", opaque, icon.Bounds().Dx()*icon.Bounds().Dy())
		fmt.Printf("  其中非背景色         : %d\n", ink)
		fmt.Printf("  → %s\n\n", out)

		if ink == 0 {
			fmt.Println("  ⚠ Shell 交出的是一张空白图——外壳图标缓存很可能是旧的")
		}
	}

	fmt.Printf("输出目录 %s\n", dir)
	if bad {
		os.Exit(1)
	}
}

// countIcons 数文件里的图标总数。
//
// nIconIndex 传 -1、句柄数组传 NULL 时，ExtractIconEx 只统计数量而不分配图标。
func countIcons(path string) int {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0
	}

	// 必须走变量：uintptr(int32(-1)) 是常量表达式，编译期会判定溢出。
	countOnly := int32(-1)

	ret, _, _ := procExtractIconExW.Call(
		uintptr(unsafe.Pointer(p)),
		uintptr(countOnly),
		0,
		0,
		0,
	)
	return int(int32(uint32(ret)))
}

// sysIconIndex 返回 Shell 给出的系统图标索引。
//
// 索引不同就是不同图标：拿一个目标失联的快捷方式作对照，
// 索引相同即说明 Shell 没解析出专属图标。
func sysIconIndex(path string) int32 {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return -1
	}

	var info shFileInfo
	ret, _, _ := procSHGetFileInfoW.Call(
		uintptr(unsafe.Pointer(p)),
		0,
		uintptr(unsafe.Pointer(&info)),
		unsafe.Sizeof(info),
		shgfiSysIconIndex,
	)
	if ret == 0 {
		return -1
	}
	return info.iIcon
}

// shellIcon 走 SHGetFileInfo 取大图标，再把句柄里的位图读出来。
//
// 这条路与资源管理器画图标是同一条：得到什么，桌面上就是什么。
func shellIcon(path string) (*image.NRGBA, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}

	var info shFileInfo
	ret, _, callErr := procSHGetFileInfoW.Call(
		uintptr(unsafe.Pointer(p)),
		0,
		uintptr(unsafe.Pointer(&info)),
		unsafe.Sizeof(info),
		shgfiIcon|shgfiLargeIcon,
	)
	if ret == 0 || info.hIcon == 0 {
		return nil, fmt.Errorf("SHGetFileInfo 未返回图标（ret=%d, %v）", ret, callErr)
	}
	defer procDestroyIcon.Call(info.hIcon)

	return decodeIcon(info.hIcon)
}

// decodeIcon 把 HICON 的彩色位图读成像素。
func decodeIcon(hIcon uintptr) (*image.NRGBA, error) {
	var ii iconInfo
	ok, _, _ := procGetIconInfo.Call(hIcon, uintptr(unsafe.Pointer(&ii)))
	if ok == 0 {
		return nil, fmt.Errorf("GetIconInfo 失败")
	}
	if ii.hbmColor != 0 {
		defer procDeleteObject.Call(ii.hbmColor)
	}
	if ii.hbmMask != 0 {
		defer procDeleteObject.Call(ii.hbmMask)
	}
	if ii.hbmColor == 0 {
		return nil, fmt.Errorf("图标没有彩色位图（只有掩码）")
	}

	var bm bitmap
	n, _, _ := procGetObjectW.Call(
		ii.hbmColor,
		unsafe.Sizeof(bm),
		uintptr(unsafe.Pointer(&bm)),
	)
	if n == 0 {
		return nil, fmt.Errorf("GetObject 读不出位图信息")
	}

	w, h := int(bm.bmWidth), int(bm.bmHeight)
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("位图尺寸异常：%dx%d", w, h)
	}

	// 负高度要求自上而下填充，省去再翻一次
	head := bitmapInfoHeader{
		biSize:        40,
		biWidth:       int32(w),
		biHeight:      int32(-h),
		biPlanes:      1,
		biBitCount:    32,
		biCompression: dibRGBColors,
	}

	pixels := make([]byte, w*h*4)
	hdc, _, _ := procGetDC.Call(0)
	if hdc == 0 {
		return nil, fmt.Errorf("GetDC 失败")
	}
	lines, _, _ := procGetDIBits.Call(
		hdc,
		ii.hbmColor,
		0,
		uintptr(h),
		uintptr(unsafe.Pointer(&pixels[0])),
		uintptr(unsafe.Pointer(&head)),
		dibRGBColors,
	)
	procReleaseDC.Call(0, hdc)
	if lines == 0 {
		return nil, fmt.Errorf("GetDIBits 失败")
	}

	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	anyAlpha := false
	for i := 0; i < w*h; i++ {
		if pixels[i*4+3] != 0 {
			anyAlpha = true
			break
		}
	}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			o := (y*w + x) * 4
			b, g, r, a := pixels[o], pixels[o+1], pixels[o+2], pixels[o+3]
			if !anyAlpha {
				// 老式掩码图标没有 alpha，当作完全不透明处理
				a = 0xFF
			}
			img.SetNRGBA(x, y, color.NRGBA{R: r, G: g, B: b, A: a})
		}
	}

	return img, nil
}

// analyse 统计不透明像素与「非背景」像素。
//
// 图标的底色是深蓝，白色折线是内容。只要还有明显不是背景色的像素，
// 说明图确实画出来了，而不是一张空壳。
func analyse(img *image.NRGBA) (opaque, ink int) {
	b := img.Bounds()

	// 取四角为参考背景：圆角处透明，露出的合成底色即背景
	bg := img.NRGBAAt(0, 0)

	for y := range b.Dy() {
		for x := range b.Dx() {
			c := img.NRGBAAt(x, y)
			if c.A > 0 {
				opaque++
			}
			d := diff(c, bg)
			// 明显偏离背景，且自身有覆盖度，才算「画了东西」
			if c.A > 128 && d > 60 {
				ink++
			}
		}
	}
	return opaque, ink
}

// diff 返回两个颜色的通道差之和，粗略衡量像不像。
func diff(a, b color.NRGBA) int {
	abs := func(v int) int {
		if v < 0 {
			return -v
		}
		return v
	}
	return abs(int(a.R)-int(b.R)) + abs(int(a.G)-int(b.G)) + abs(int(a.B)-int(b.B))
}

func safeName(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			out = append(out, r)
		default:
			out = append(out, '_')
		}
	}
	return string(out)
}

func writePNG(path string, img image.Image) error {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}
