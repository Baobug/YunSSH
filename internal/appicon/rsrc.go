// SPDX-FileCopyrightText: 2026 Zhou Tianbo (Baobug)
// SPDX-License-Identifier: Apache-2.0

package appicon

import (
	"bytes"
	"encoding/binary"
)

// 用到的 Windows 资源类型。
const (
	rtIcon      = 3  // RT_ICON：单个图标图像
	rtGroupIcon = 14 // RT_GROUP_ICON：图标组，决定 exe 显示哪个图标
)

// resourceLang 是资源的语言 ID。
//
// 用 en-US 是通用做法：图标不含本地化内容，系统会按回退规则取用。
const resourceLang = 0x0409

// 资源目录里各结构的固定大小。
const (
	resDirHeaderSize = 16 // IMAGE_RESOURCE_DIRECTORY
	resDirEntrySize  = 8  // IMAGE_RESOURCE_DIRECTORY_ENTRY
	resDataEntrySize = 16 // IMAGE_RESOURCE_DATA_ENTRY
	resDirFlag       = 0x80000000
)

// COFF 目标文件里与本文件相关的常量。
const (
	// IMAGE_FILE_MACHINE_AMD64。Go 链接器按文件开头两字节识别目标平台，
	// 小端序下这两个字节正好落在它认的 amd64 分支上。
	machineAMD64 = 0x8664
	// IMAGE_SCN_CNT_INITIALIZED_DATA | IMAGE_SCN_MEM_READ。
	//
	// 特性必须恰好是这两个标志：Go 链接器会用特性位判断段类型，
	// 多带一个 IMAGE_SCN_MEM_DISCARDABLE 就会整段跳过，图标也就丢了。
	sectionCharacteristics = 0x40000040
	// COFF 头 + 段头，段数据紧随其后。
	coffHeaderSize = 20
	sectionHdrSize = 40

	// IMAGE_REL_AMD64_ADDR32：往目标位置填一个 32 位绝对虚拟地址。
	relAddr32 = 0x0002
	// IMAGE_SYM_CLASS_STATIC，段符号的存储类别。
	symClassStatic = 3

	coffSymbolSize = 18 // COFF 符号表条目
	coffRelocSize  = 10 // COFF 重定位条目
)

// ResourceObject 返回可被 Go 链接器并入 exe 的 Windows 资源对象。
//
// Go 的 PE 链接器会把输入目标文件里名为 .rsrc 的段原样并进最终可执行文件，
// 所以这里手写一个最小 COFF 目标文件就够了——不必依赖 rsrc、go-winres
// 这类外部工具，仓库里也仍然不用存放任何二进制文件。
//
// 段里的目录偏移是「相对段起始」的，但 IMAGE_RESOURCE_DATA_ENTRY 的
// OffsetToData 必须是 RVA（对照系统自带的 PE 文件确认过）。产出 RVA 需要
// 段基址，只有链接时才知道，因此这里为每个数据偏移字段补一条
// IMAGE_REL_AMD64_ADDR32 重定位——链接器读取该位置上的原值作为加数，
// 再加上段基址回写，正好完成段内偏移到 RVA 的换算。
func ResourceObject() []byte {
	entries := images()

	icons := make([][]byte, len(entries))
	for i, e := range entries {
		icons[i] = e.data
	}

	section, relocSites := buildResourceSection(icons, groupIconEntries(entries))

	var buf bytes.Buffer

	// 先在纸上把各区块的位置算出来，再依次落盘
	dataOffset := coffHeaderSize + sectionHdrSize
	relocOffset := align4(dataOffset + len(section))
	symtabOffset := align4(relocOffset + len(relocSites)*coffRelocSize)
	symCount := 2 // 一条段符号 + 一条辅助记录

	// --- COFF 头 ---
	writeU16(&buf, machineAMD64)
	writeU16(&buf, 1) // 段数量
	writeU32(&buf, 0) // 时间戳
	writeU32(&buf, uint32(symtabOffset))
	writeU32(&buf, uint32(symCount))
	writeU16(&buf, 0) // 可选头大小
	writeU16(&buf, 0) // 特性

	// --- 段头 ---
	name := make([]byte, 8)
	copy(name, ".rsrc")
	buf.Write(name)
	writeU32(&buf, 0)                       // 虚拟大小
	writeU32(&buf, 0)                       // 虚拟地址
	writeU32(&buf, uint32(len(section)))    // 原始数据大小
	writeU32(&buf, uint32(dataOffset))      // 原始数据偏移
	writeU32(&buf, uint32(relocOffset))     // 重定位表偏移
	writeU32(&buf, 0)                       // 行号表偏移
	writeU16(&buf, uint16(len(relocSites))) // 重定位数量
	writeU16(&buf, 0)                       // 行号数量
	writeU32(&buf, sectionCharacteristics)

	// --- 段数据 ---
	buf.Write(section)
	padTo(&buf, relocOffset)

	// --- 重定位表：全部指向段符号（索引 0）---
	for _, site := range relocSites {
		writeU32(&buf, uint32(site)) // 待改写字段在段内的偏移
		writeU32(&buf, 0)            // 符号表索引
		writeU16(&buf, relAddr32)
	}
	padTo(&buf, symtabOffset)

	// --- 符号表：一条段符号 ---
	buf.Write(coffName(".rsrc"))
	writeU32(&buf, 0)             // 值
	writeU16(&buf, 1)             // 段号
	writeU16(&buf, 0)             // 类型：0 表示非函数
	buf.WriteByte(symClassStatic) // 存储类别
	buf.WriteByte(1)              // 后续辅助记录数

	// --- 辅助记录：段定义 ---
	writeU32(&buf, uint32(len(section)))    // 段长度
	writeU16(&buf, uint16(len(relocSites))) // 重定位数量
	writeU16(&buf, 0)                       // 行号数量
	writeU32(&buf, 0)                       // 校验和
	writeU16(&buf, 0)                       // 关联度
	buf.WriteByte(0)                        // 选择类型
	buf.Write([]byte{0, 0, 0})              // 填充

	// --- 字符串表：没有长名字，留一个空表 ---
	writeU32(&buf, 0)

	return buf.Bytes()
}

// coffName 把不超过 8 字节的符号名填进 COFF 的定长名字字段。
func coffName(name string) []byte {
	if len(name) > 8 {
		panic("appicon: COFF 符号名过长: " + name)
	}
	out := make([]byte, 8)
	copy(out, name)
	return out
}

// padTo 补零，直到缓冲区长度达到 target。
func padTo(buf *bytes.Buffer, target int) {
	if n := target - buf.Len(); n > 0 {
		buf.Write(make([]byte, n))
	}
}

// groupIconEntries 生成 RT_GROUP_ICON 的内容：GRPICONDIR + 每个图像的条目。
//
// 与 ICO 的文件目录不同，这里每个条目用「资源 ID」代替文件偏移，
// 必须和 buildResourceSection 写出的 RT_ICON 编号一一对应。
func groupIconEntries(entries []imageEntry) []byte {
	var buf bytes.Buffer
	writeU16(&buf, 0)                    // 保留字段
	writeU16(&buf, 1)                    // 类型：1 = 图标
	writeU16(&buf, uint16(len(entries))) // 图像数量

	for i, e := range entries {
		buf.WriteByte(dimByte(e.width))  // 宽度（0 表示 256）
		buf.WriteByte(dimByte(e.height)) // 高度
		buf.WriteByte(0)                 // 调色板颜色数
		buf.WriteByte(0)                 // 保留字段
		writeU16(&buf, 1)                // 颜色平面数
		writeU16(&buf, 32)               // 每像素位数
		writeU32(&buf, uint32(len(e.data)))
		writeU16(&buf, uint16(i+1)) // 对应的 RT_ICON 资源 ID
	}

	return buf.Bytes()
}

// buildResourceSection 组装 .rsrc 段的内容，并返回需要重定位的位置，
// 也就是各数据条目里那个「数据偏移」字段在段内的偏移。
//
// 目录树固定三层：类型 → 资源 ID → 语言。同一层内按 ID 升序排列，
// 因为 Windows 是拿 ID 做二分查找来定位资源的。
//
// 关键约束：IMAGE_RESOURCE_DIRECTORY 结构里没有「条目数组偏移」这个字段，
// 读取方固定按「目录偏移 + 16」去找条目。所以每个目录的条目数组必须紧贴
// 它的头部，不能把头部和条目拆成两片区域分别排布。下面一律以 dirSize 为
// 步长推进，保证每个目录都是「头部 + 条目」连续存放。
//
// 段内布局（偏移均相对段起始，n 为图标图像数量）：
//
//	0        根目录                    头部 + 2 个类型条目
//	32       RT_ICON 的 ID 目录        头部 + n 个条目
//	48+8n    RT_GROUP_ICON 的 ID 目录  头部 + 1 个条目
//	72+8n    各图像的「语言」目录      头部 + 1 个条目，每块 24 字节
//	72+32n   RT_GROUP_ICON 的「语言」目录
//	96+32n   数据条目 × (n+1)          IMAGE_RESOURCE_DATA_ENTRY
//	112+48n  数据块，每块按 4 字节对齐
func buildResourceSection(icons [][]byte, group []byte) (section []byte, relocSites []int) {
	n := len(icons)

	offIconIDs := dirSize(2)                            // 32
	offGroupIDs := offIconIDs + dirSize(n)              // 48+8n
	offIconLangs := offGroupIDs + dirSize(1)            // 72+8n
	offGroupLang := offIconLangs + n*dirSize(1)         // 72+32n
	offDataEntries := offGroupLang + dirSize(1)         // 96+32n
	offBlobs := offDataEntries + (n+1)*resDataEntrySize // 112+48n

	// 先排好每块数据的位置：紧跟目录区，每块按 4 字节对齐
	blobAt := make([]int, n+1)
	pos := offBlobs
	for i, blob := range icons {
		pos = align4(pos)
		blobAt[i] = pos
		pos += len(blob)
	}
	pos = align4(pos)
	blobAt[n] = pos
	pos += len(group)

	sec := make([]byte, pos)

	put16 := func(off int, v uint16) { binary.LittleEndian.PutUint16(sec[off:], v) }
	put32 := func(off int, v uint32) { binary.LittleEndian.PutUint32(sec[off:], v) }

	// directory 写一个目录头。本文件只产生数字 ID 条目，命名条目恒为 0。
	directory := func(off, count int) {
		put16(off+12, 0)
		put16(off+14, uint16(count))
	}
	// child 写一个指向子目录的条目
	child := func(off int, id uint32, target int) {
		put32(off, id)
		put32(off+4, uint32(target)|resDirFlag)
	}
	// leaf 写一个指向数据的条目
	leaf := func(off int, id uint32, target int) {
		put32(off, id)
		put32(off+4, uint32(target))
	}

	// --- 根目录：两个资源类型 ---
	directory(0, 2)
	child(resDirHeaderSize, rtIcon, offIconIDs)
	child(resDirHeaderSize+resDirEntrySize, rtGroupIcon, offGroupIDs)

	// --- 类型层下的 ID 目录 ---
	directory(offIconIDs, n)
	for i := range icons {
		child(offIconIDs+resDirHeaderSize+i*resDirEntrySize,
			uint32(i+1), offIconLangs+i*dirSize(1))
	}
	directory(offGroupIDs, 1)
	child(offGroupIDs+resDirHeaderSize, 1, offGroupLang)

	// --- 语言层：每个目录只有一条 en-US 条目，紧跟在头部之后 ---
	for i := range icons {
		block := offIconLangs + i*dirSize(1)
		directory(block, 1)
		leaf(block+resDirHeaderSize, resourceLang, offDataEntries+i*resDataEntrySize)
	}
	directory(offGroupLang, 1)
	leaf(offGroupLang+resDirHeaderSize, resourceLang, offDataEntries+n*resDataEntrySize)

	// --- 数据条目 ---
	// 这里的「数据偏移」写的是段内偏移，链接器会按重定位把它换算成 RVA。
	relocSites = make([]int, 0, n+1)
	for i, blob := range icons {
		base := offDataEntries + i*resDataEntrySize
		put32(base, uint32(blobAt[i]))   // 数据偏移
		put32(base+4, uint32(len(blob))) // 数据长度
		relocSites = append(relocSites, base)
		// 代码页与保留字段留 0
	}
	groupBase := offDataEntries + n*resDataEntrySize
	put32(groupBase, uint32(blobAt[n]))
	put32(groupBase+4, uint32(len(group)))
	relocSites = append(relocSites, groupBase)

	// --- 数据块 ---
	for i, blob := range icons {
		copy(sec[blobAt[i]:], blob)
	}
	copy(sec[blobAt[n]:], group)

	return sec, relocSites
}

// dirSize 返回一个资源目录占用的字节数：16 字节头部加上紧随其后的条目数组。
func dirSize(entries int) int {
	return resDirHeaderSize + entries*resDirEntrySize
}

// align4 把偏移向上取整到 4 的倍数。
func align4(v int) int {
	return (v + 3) &^ 3
}
