// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

// Command lnkcheck 解析 Shell Link 文件，打印影响图标显示的关键字段。
//
// 排查「快捷方式图标空白」时，需要确认三件事：LinkFlags 里 HasIconLocation
// 有没有置上、ICON_LOCATION 字符串是不是写成了 "路径,索引"、以及
// 头部 IconIndex 字段的值。这些都无法靠肉眼扫十六进制看出来。
//
//	go run ./tools/lnkcheck <文件.lnk> [更多 .lnk]
package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"unicode/utf16"
)

// walkIDList 逐个走 ItemID，返回项数与结构是否自洽。
//
// 判定标准是「走完之后正好停在终止项上」：每个 ItemID 的头两个字节是它自身的
// 长度，长度为 0 即终止项。注意 Windows 的 ItemID 并不都按 2 字节对齐——
// 卷项就可能出现奇数长度，不能拿总长度是否为偶数来判断。
func walkIDList(data []byte) (items int, ok bool) {
	pos := 0
	for pos+2 <= len(data) {
		size := int(leU16(data, pos))
		if size == 0 {
			// 终止项之后不应还有数据
			return items, pos == len(data)-2
		}
		if size < 3 || pos+size > len(data) {
			return items, false
		}
		pos += size
		items++
	}
	return items, false
}

func leU16(b []byte, off int) uint16 {
	return uint16(b[off]) | uint16(b[off+1])<<8
}

// LinkFlags 位定义（MS-SHLLINK 2.1.1）。
var flagLabels = []struct {
	bit  uint32
	name string
}{
	{0x00000001, "HasLinkTargetIDList"},
	{0x00000002, "HasLinkInfo"},
	{0x00000004, "HasName"},
	{0x00000008, "HasRelativePath"},
	{0x00000010, "HasWorkingDir"},
	{0x00000020, "HasArguments"},
	{0x00000040, "HasIconLocation"},
	{0x00000080, "IsUnicode"},
	{0x00000200, "HasExpString"},
	{0x00002000, "RunAsUser"},
}

// StringData 段按规范固定顺序出现。
var stringOrder = []struct {
	bit  uint32
	name string
}{
	{0x00000004, "NAME_STRING"},
	{0x00000008, "RELATIVE_PATH"},
	{0x00000010, "WORKING_DIR"},
	{0x00000020, "COMMAND_LINE_ARGUMENTS"},
	{0x00000040, "ICON_LOCATION"},
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "用法: lnkcheck <文件.lnk> [更多 .lnk]")
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

func inspect(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	fmt.Printf("=== %s（%d 字节）===\n", path, len(data))

	if len(data) < 76 {
		return fmt.Errorf("文件只有 %d 字节，不足一个 Shell Link 头部", len(data))
	}

	headerSize := binary.LittleEndian.Uint32(data[0:4])
	flags := binary.LittleEndian.Uint32(data[20:24])
	iconIndex := int32(binary.LittleEndian.Uint32(data[0x38:0x3C]))
	showCmd := binary.LittleEndian.Uint32(data[0x3C:0x40])

	fmt.Printf("  HeaderSize  : %d（应为 76）\n", headerSize)
	fmt.Printf("  LinkFlags   : 0x%08X\n", flags)
	fmt.Printf("  IconIndex   : %d  ← 头部里的图标索引，有 ICON_LOCATION 时以字符串为准\n", iconIndex)
	fmt.Printf("  ShowCommand : %d\n", showCmd)

	fmt.Println("  ── 标志位 ──")
	for _, l := range flagLabels {
		mark := " "
		if flags&l.bit != 0 {
			mark = "x"
		}
		fmt.Printf("    [%s] %s\n", mark, l.name)
	}

	if flags&0x00000080 == 0 {
		fmt.Println("\n  ✗ 未设置 IsUnicode，本工具只解析 Unicode 形式")
		return nil
	}

	pos := int(headerSize)

	// 跳过 LinkTargetIDList。
	// 外壳生成的快捷方式都带这一段，不跳过去的话后面的字段会全部错位，
	// 表现成「数据越界」——看起来像文件坏了，其实只是没按结构走。
	if flags&0x00000001 != 0 {
		if pos+2 > len(data) {
			return fmt.Errorf("LinkTargetIDList 长度字段越界")
		}
		listSize := int(leU16(data, pos))
		fmt.Printf("\n  ── LinkTargetIDList ──\n")
		fmt.Printf("    IDListSize  : %d（占用 [%d, %d)）\n", listSize, pos+2, pos+2+listSize)

		if pos+2+listSize > len(data) {
			return fmt.Errorf("LinkTargetIDList 声明 %d 字节，但文件只剩 %d 字节",
				listSize, len(data)-pos-2)
		}

		items, ok := walkIDList(data[pos+2 : pos+2+listSize])
		fmt.Printf("    ItemID 个数 : %d\n", items)
		if !ok {
			fmt.Printf("    ✗ 列表结构不自洽：项长度没有正好走到终止项\n")
		}
		pos += 2 + listSize
	} else {
		fmt.Printf("\n  ⚠ 没有 LinkTargetIDList——外壳解析主要靠它，\n")
		fmt.Printf("    缺了会退化成通用空白图标（这类快捷方式在资源管理器里看着是坏的）\n")
	}

	// 跳过 LinkInfo
	if flags&0x00000002 != 0 {
		if pos+4 > len(data) {
			return fmt.Errorf("LinkInfo 头部越界")
		}
		liSize := int(binary.LittleEndian.Uint32(data[pos : pos+4]))
		fmt.Printf("\n  LinkInfoSize: %d（占用 [%d, %d)）\n", liSize, pos, pos+liSize)
		pos += liSize
	}

	fmt.Println("  ── 字符串段 ──")
	for _, o := range stringOrder {
		if flags&o.bit == 0 {
			continue
		}
		if pos+2 > len(data) {
			fmt.Printf("    %-24s ✗ 数据越界\n", o.name)
			return nil
		}

		chars := int(binary.LittleEndian.Uint16(data[pos : pos+2]))
		pos += 2

		if pos+chars*2+2 > len(data) {
			fmt.Printf("    %-24s ✗ 声明 %d 个字符，但数据越界\n", o.name, chars)
			return nil
		}

		raw := make([]uint16, chars)
		for i := 0; i < chars; i++ {
			raw[i] = binary.LittleEndian.Uint16(data[pos+i*2 : pos+i*2+2])
		}
		fmt.Printf("    %-24s = %q\n", o.name, string(utf16.Decode(raw)))

		pos += chars*2 + 2 // 内容 + 终止符
	}

	return nil
}
