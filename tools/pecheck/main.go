// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

// Command pecheck 列出 PE 文件里的资源类型，用于确认图标与版本信息是否
// 真的被链接进了二进制。
//
// 为什么需要它：资源段的存在与否无法从文件大小推断，而搜字符串也不可靠——
// PE 里的资源类型是整数 ID（3=RT_ICON、14=RT_GROUP_ICON、16=RT_VERSION），
// 资源名以 UTF-16 存储，用 ASCII 搜"VS_VERSION_INFO"永远搜不到。
//
//	go run ./tools/pecheck <exe> [更多 exe]
package main

import (
	"debug/pe"
	"fmt"
	"os"
)

// 资源类型 ID，取自 WinUser.h。
var resourceTypeNames = map[uint32]string{
	1:  "RT_CURSOR",
	2:  "RT_BITMAP",
	3:  "RT_ICON",
	4:  "RT_MENU",
	5:  "RT_DIALOG",
	6:  "RT_STRING",
	14: "RT_GROUP_ICON",
	16: "RT_VERSION",
	24: "RT_MANIFEST",
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "用法: pecheck <exe> [更多 exe]")
		os.Exit(2)
	}

	failed := false
	for _, path := range os.Args[1:] {
		if err := inspect(path); err != nil {
			fmt.Printf("  ✗ %v\n", err)
			failed = true
		}
		fmt.Println()
	}

	if failed {
		os.Exit(1)
	}
}

// inspect 打印单个 PE 文件的资源类型清单。
func inspect(path string) error {
	f, err := pe.Open(path)
	if err != nil {
		return fmt.Errorf("打开 %s 失败: %w", path, err)
	}
	defer f.Close()

	stat, err := os.Stat(path)
	if err != nil {
		return err
	}
	fmt.Printf("=== %s (%d 字节) ===\n", path, stat.Size())

	section := f.Section(".rsrc")
	if section == nil {
		fmt.Println("  ✗ 没有 .rsrc 段——资源完全没有链接进二进制")
		return nil
	}
	fmt.Printf("  .rsrc 段大小: %d 字节\n", section.Size)

	data, err := section.Data()
	if err != nil {
		return fmt.Errorf("读取资源段失败: %w", err)
	}

	// IMAGE_RESOURCE_DIRECTORY 头部 16 字节，末尾两个 uint16 是条目计数
	if len(data) < 16 {
		fmt.Println("  ✗ 资源段太短，无法解析")
		return nil
	}

	named := int(leU16(data, 12))
	byID := int(leU16(data, 14))
	fmt.Printf("  一级条目: %d 个（具名 %d，按 ID %d）\n", named+byID, named, byID)

	found := map[uint32]bool{}
	for i := 0; i < named+byID; i++ {
		off := 16 + i*8 // 每项 8 字节：ID(4) + 偏移(4)
		if off+8 > len(data) {
			break
		}

		typeID := leU32(data, off)
		childOffset := leU32(data, off+4)
		if childOffset&0x80000000 == 0 {
			continue // 高位为 0 表示这不是子目录
		}

		name := resourceTypeNames[typeID]
		if name == "" {
			name = fmt.Sprintf("类型 %d", typeID)
		}

		// 子目录里的条目数即该类型的资源个数
		count := 0
		if child := int(childOffset & 0x7FFFFFFF); child+16 <= len(data) {
			count = int(leU16(data, child+12)) + int(leU16(data, child+14))
		}

		fmt.Printf("    %-16s → %d 个\n", name, count)
		found[typeID] = true
	}

	// 关键的几项单独点出来，避免只扫一眼数字
	fmt.Println("  ── 关键项 ──")
	check := func(id uint32, label string) {
		if found[id] {
			fmt.Printf("    ✓ %s\n", label)
		} else {
			fmt.Printf("    ✗ %s 缺失\n", label)
		}
	}
	check(3, "RT_ICON（图标图像）")
	check(14, "RT_GROUP_ICON（图标组，exe 显示哪个图标靠它）")
	check(16, "RT_VERSION（版本信息，资源管理器「详细信息」读它）")

	return nil
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
