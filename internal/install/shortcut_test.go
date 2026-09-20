// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package install

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf16"
)

// testIDList 是一个形状合法的目标标识：带终止项的字节串。
// 真实内容由外壳生成，结构测试只需要一个可以被读回来的占位。
var testIDList = []byte{0x08, 0x00, 0x2F, 0x00, 0x61, 0x00, 0x00, 0x00}

// TestBuildShellLinkHeader 验证 Shell Link 的固定头部。
// 头部一旦写错，Windows 就完全不会把它识别成快捷方式，且不会有任何报错。
func TestBuildShellLinkHeader(t *testing.T) {
	const target = `C:\Users\test\AppData\Local\Programs\YunSSH\ysshtray.exe`
	link := buildShellLink(target, `C:\Users\test\AppData\Local\Programs\YunSSH`, target+",0", testIDList, swShowNormal)

	if len(link) < shellLinkHeaderSize {
		t.Fatalf("输出太短：%d 字节", len(link))
	}

	if got := leU32(link, 0); got != shellLinkHeaderSize {
		t.Errorf("HeaderSize = %d，期望 %d", got, shellLinkHeaderSize)
	}

	// CLSID 必须逐字节匹配
	if got := string(link[4:20]); got != shellLinkCLSID {
		t.Errorf("CLSID 不匹配：% x", link[4:20])
	}

	flags := leU32(link, 20)
	for _, want := range []uint32{flagHasLinkTargetIDList, flagHasLinkInfo, flagHasWorkingDir, flagHasIconLocation, flagIsUnicode} {
		if flags&want == 0 {
			t.Errorf("flags 缺少位 0x%X（实际 0x%X）", want, flags)
		}
	}
}

// TestBuildShellLinkTargetIDList 验证目标标识段的长度字段与实际内容一致。
//
// 这一段是外壳解析快捷方式的主要依据。长度写错会让外壳读到一个错位的列表，
// 表现和完全没有这段一样：图标变成通用的空白图标，且不会有任何报错。
func TestBuildShellLinkTargetIDList(t *testing.T) {
	link := buildShellLink(`C:\a\b.exe`, "", "", testIDList, swShowNormal)

	// 头部之后紧跟 LinkTargetIDList：2 字节长度 + 列表本体
	if got := leU16(link, shellLinkHeaderSize); int(got) != len(testIDList) {
		t.Fatalf("IDListSize = %d，期望 %d", got, len(testIDList))
	}

	got := link[shellLinkHeaderSize+2 : shellLinkHeaderSize+2+len(testIDList)]
	if !bytes.Equal(got, testIDList) {
		t.Errorf("IDList 内容不匹配：% x", got)
	}

	// 列表必须以终止项（两个 0 字节）收尾
	if n := len(testIDList); n < 2 || testIDList[n-2] != 0 || testIDList[n-1] != 0 {
		t.Error("测试数据本身不以终止项收尾")
	}
}

// TestBuildShellLinkWithoutTargetIDList 验证没有标识时不会置上对应标志位，
// 也不会在头部之后留下半截长度字段——那会让整条记录从 LinkInfo 开始错位。
func TestBuildShellLinkWithoutTargetIDList(t *testing.T) {
	link := buildShellLink(`C:\a\b.exe`, "", "", nil, swShowNormal)

	if leU32(link, 20)&flagHasLinkTargetIDList != 0 {
		t.Error("没有目标标识时不应设置 HasLinkTargetIDList")
	}
	// 头部之后应当直接是 LinkInfo：它的第一个字段是总长，第二个是头部长 0x24
	if got := leU32(link, shellLinkHeaderSize+4); got != 0x24 {
		t.Errorf("头部之后应当是 LinkInfo（其 HeaderSize 字段为 0x24），实际读到 0x%X", got)
	}
}

// TestBuildShellLinkOptionalFlags 验证没提供可选项时不会置上对应标志位。
func TestBuildShellLinkOptionalFlags(t *testing.T) {
	link := buildShellLink(`C:\a\b.exe`, "", "", testIDList, swShowNormal)

	flags := leU32(link, 20)
	if flags&flagHasWorkingDir != 0 {
		t.Error("未提供工作目录时不应设置 HasWorkingDir")
	}
	if flags&flagHasIconLocation != 0 {
		t.Error("未提供图标时不应设置 HasIconLocation")
	}
	if flags&flagHasLinkInfo == 0 {
		t.Error("必须设置 HasLinkInfo，否则无法定位目标")
	}
}

// TestBuildShellLinkContainsPaths 验证目标路径以 ANSI 与 UTF-16 两种形式写入。
// 只写 ANSI 的话，含中文的安装路径会解析失败。
func TestBuildShellLinkContainsPaths(t *testing.T) {
	const target = `C:\Users\测试用户\Programs\YunSSH\ysshtray.exe`
	link := buildShellLink(target, `C:\Users\测试用户\Programs\YunSSH`, target+",0", testIDList, swShowNormal)

	if !bytes.Contains(link, []byte(target)) {
		t.Error("ANSI 段未包含目标路径")
	}
	if !bytes.Contains(link, encodeUTF16(target)) {
		t.Error("Unicode 段未包含目标路径")
	}
}

// TestBuildShellLinkLinkInfoConsistency 验证 LinkInfo 内部的长度与偏移自洽。
// 偏移算错会让 Shell 读到越界位置，表现为快捷方式"存在但打不开"。
func TestBuildShellLinkLinkInfoConsistency(t *testing.T) {
	const (
		target     = `C:\a\b\ysshtray.exe`
		workingDir = `C:\a\b`
	)
	link := buildShellLink(target, workingDir, "", testIDList, swShowNormal)

	// LinkInfo 位于头部与目标标识之后
	base := shellLinkHeaderSize + 2 + len(testIDList)

	size := leU32(link, base)

	// LinkInfo 之后紧跟 StringData，所以不能用文件总长来校验。
	// 精确做法是看边界处读到的值是否正好是 WorkingDir 的字符数——
	// 若 size 算错一个字节，这里读到的就会对不上。
	afterLinkInfo := int(base) + int(size)
	if afterLinkInfo >= len(link) {
		t.Fatalf("LinkInfoSize = %d 越过了文件末尾（共 %d 字节）", size, len(link))
	}

	wantChars := len(utf16.Encode([]rune(workingDir)))
	if got := leU16(link, afterLinkInfo); int(got) != wantChars {
		t.Errorf("边界处应读到 WorkingDir 的字符数 %d，实际读到 %d —— LinkInfoSize 算错了",
			wantChars, got)
	}

	if got := leU32(link, base+4); got != 0x24 {
		t.Errorf("LinkInfoHeaderSize = 0x%X，期望 0x24", got)
	}
	if got := leU32(link, base+8); got != 0x00000001 {
		t.Errorf("LinkInfoFlags = 0x%X，期望 0x00000001", got)
	}

	// 每个偏移都必须落在 LinkInfo 范围内
	offsets := map[string]uint32{
		"VolumeIDOffset":                leU32(link, base+12),
		"LocalBasePathOffset":           leU32(link, base+16),
		"CommonPathSuffixOffset":        leU32(link, base+24),
		"LocalBasePathOffsetUnicode":    leU32(link, base+28),
		"CommonPathSuffixOffsetUnicode": leU32(link, base+32),
	}
	for name, off := range offsets {
		if off >= size {
			t.Errorf("%s = 0x%X 超出 LinkInfo 范围（size = %d）", name, off, size)
		}
	}
}

// TestBuildShellLinkSizeIsStable 记录典型输入下的输出长度。
// 只用于发现意外变化，不是精确契约。
func TestBuildShellLinkSizeIsStable(t *testing.T) {
	const target = `C:\Users\ZhouTB\AppData\Local\Programs\YunSSH\ysshtray.exe`
	link := buildShellLink(target, `C:\Users\ZhouTB\AppData\Local\Programs\YunSSH`, target+",0", testIDList, swShowNormal)

	if len(link) < 200 || len(link) > 1000 {
		t.Errorf("输出长度 %d 不在合理范围（200~1000）", len(link))
	}
}

// TestLinkTargetIDListIsWellFormed 验证能向外壳要到一个形状正确的目标标识。
//
// 用一个必然存在的系统程序作为目标，避开对安装结果的依赖。
func TestLinkTargetIDListIsWellFormed(t *testing.T) {
	systemRoot := os.Getenv("SystemRoot")
	if systemRoot == "" {
		t.Skip("SystemRoot 未设置，跳过")
	}

	idlist, err := linkTargetIDList(filepath.Join(systemRoot, "System32", "notepad.exe"))
	if err != nil {
		t.Fatalf("取目标标识失败: %v", err)
	}

	if len(idlist) < 8 {
		t.Fatalf("标识只有 %d 字节，太短", len(idlist))
	}

	// 逐个 ItemID 走一遍：每个项的头两字节是自身长度，长度为 0 即终止项。
	//
	// 这里不能限定长度为偶数——Windows 的 ItemID 并不都按 2 字节对齐，
	// 卷项实测就是 25 字节。判定依据只能是「走完之后正好停在终止项上」。
	pos, items := 0, 0
	for {
		if pos+2 > len(idlist) {
			t.Fatalf("走到第 %d 字节时越过了末尾，说明项长度不自洽", pos)
		}
		size := int(idlist[pos]) | int(idlist[pos+1])<<8
		if size == 0 {
			break
		}
		if size < 3 || pos+size > len(idlist) {
			t.Fatalf("第 %d 个项声明的长度 %d 不合理（位于偏移 %d，共 %d 字节）",
				items, size, pos, len(idlist))
		}
		pos += size
		items++
	}

	if items == 0 {
		t.Error("标识里一个项都没有")
	}
	if pos != len(idlist)-2 {
		t.Errorf("终止项位于 %d，但列表长度为 %d——终止项之后不应还有数据", pos, len(idlist))
	}
}

// TestLinkTargetIDListRejectsMissingPath 验证目标不存在时如实报错，
// 而不是返回一个外壳读不懂的空标识。
func TestLinkTargetIDListRejectsMissingPath(t *testing.T) {
	_, err := linkTargetIDList(`C:\这个路径\显然\不存在\a.exe`)
	if err == nil {
		t.Error("目标不存在时应当报错")
	}
}

// TestEncodeStringData 验证 StringData 的前置字符数是字符数而非字节数。
// 含中文时两者不同——写错的话路径会被截断。
func TestEncodeStringData(t *testing.T) {
	const s = `C:\中文路径\a.exe`
	data := encodeStringData(s)

	runes := utf16.Encode([]rune(s))
	if got := leU16(data, 0); int(got) != len(runes) {
		t.Errorf("字符数 = %d，期望 %d", got, len(runes))
	}

	// 长度 = 2(计数) + 2×字符数 + 2(终止符)
	if want := 2 + len(runes)*2 + 2; len(data) != want {
		t.Errorf("编码长度 = %d，期望 %d", len(data), want)
	}
}

// TestDesktopDirIsResolvable 验证能解析出一个真实存在的桌面目录。
//
// 这条断言针对一个具体的坑：桌面可能被 OneDrive 或组策略重定向，
// 此时直接拼 %USERPROFILE%\Desktop 会得到一个并不存在的路径。
// 快捷方式写进去，用户那边什么也看不到，而且不会有任何报错。
func TestDesktopDirIsResolvable(t *testing.T) {
	dir, err := DesktopDir()
	if err != nil {
		t.Fatalf("无法确定桌面目录: %v", err)
	}
	if dir == "" {
		t.Fatal("桌面目录为空")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("解析出的桌面目录 %q 不存在: %v", dir, err)
	}
}

// TestPlaceDirsAreDistinct 两个快捷方式位置必须指向不同目录。
func TestPlaceDirsAreDistinct(t *testing.T) {
	startMenu, err := PlaceStartMenu.Dir()
	if err != nil {
		t.Fatalf("开始菜单目录: %v", err)
	}
	desktop, err := PlaceDesktop.Dir()
	if err != nil {
		t.Fatalf("桌面目录: %v", err)
	}
	if startMenu == desktop {
		t.Errorf("两个位置不应指向同一目录: %q", startMenu)
	}
}

// TestPlacePathUsesShortcutName 验证两个位置的路径都以约定的文件名结尾。
func TestPlacePathUsesShortcutName(t *testing.T) {
	want := shortcutName + ".lnk"

	for _, p := range []Place{PlaceStartMenu, PlaceDesktop} {
		path, err := p.Path()
		if err != nil {
			t.Fatalf("%s 路径: %v", p, err)
		}
		if got := filepath.Base(path); got != want {
			t.Errorf("%s 文件名 = %q，期望 %q", p, got, want)
		}
	}
}

// TestPlaceString 验证位置名称，它会被拼进提示文案里。
func TestPlaceString(t *testing.T) {
	if got := PlaceStartMenu.String(); got != "开始菜单" {
		t.Errorf("PlaceStartMenu.String() = %q", got)
	}
	if got := PlaceDesktop.String(); got != "桌面" {
		t.Errorf("PlaceDesktop.String() = %q", got)
	}
}

func leU16(b []byte, off int) uint16 {
	return uint16(b[off]) | uint16(b[off+1])<<8
}

func leU32(b []byte, off int) uint32 {
	return uint32(b[off]) |
		uint32(b[off+1])<<8 |
		uint32(b[off+2])<<16 |
		uint32(b[off+3])<<24
}
