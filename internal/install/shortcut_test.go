// SPDX-FileCopyrightText: 2026 Zhou Tianbo (Baobug)
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package install

import (
	"bytes"
	"testing"
	"unicode/utf16"
)

// TestBuildShellLinkHeader 验证 Shell Link 的固定头部。
// 头部一旦写错，Windows 就完全不会把它识别成快捷方式，且不会有任何报错。
func TestBuildShellLinkHeader(t *testing.T) {
	const target = `C:\Users\test\AppData\Local\Programs\YunSSH\ysshtray.exe`
	link := buildShellLink(target, `C:\Users\test\AppData\Local\Programs\YunSSH`, target+",0", swShowNormal)

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
	for _, want := range []uint32{flagHasLinkInfo, flagHasWorkingDir, flagHasIconLocation, flagIsUnicode} {
		if flags&want == 0 {
			t.Errorf("flags 缺少位 0x%X（实际 0x%X）", want, flags)
		}
	}
}

// TestBuildShellLinkOptionalFlags 验证没提供可选项时不会置上对应标志位。
func TestBuildShellLinkOptionalFlags(t *testing.T) {
	link := buildShellLink(`C:\a\b.exe`, "", "", swShowNormal)

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
	link := buildShellLink(target, `C:\Users\测试用户\Programs\YunSSH`, target+",0", swShowNormal)

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
	link := buildShellLink(target, workingDir, "", swShowNormal)

	const base = shellLinkHeaderSize // LinkInfo 紧随固定头部

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
	link := buildShellLink(target, `C:\Users\ZhouTB\AppData\Local\Programs\YunSSH`, target+",0", swShowNormal)

	if len(link) < 200 || len(link) > 1000 {
		t.Errorf("输出长度 %d 不在合理范围（200~1000）", len(link))
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

func leU16(b []byte, off int) uint16 {
	return uint16(b[off]) | uint16(b[off+1])<<8
}

func leU32(b []byte, off int) uint32 {
	return uint32(b[off]) |
		uint32(b[off+1])<<8 |
		uint32(b[off+2])<<16 |
		uint32(b[off+3])<<24
}
