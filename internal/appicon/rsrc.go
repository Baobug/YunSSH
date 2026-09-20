// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

package appicon

import (
	"bytes"
	"encoding/binary"
	"sort"
)

// 用到的 Windows 资源类型。
const (
	rtIcon      = 3  // RT_ICON：单个图标图像
	rtGroupIcon = 14 // RT_GROUP_ICON：图标组，决定 exe 显示哪个图标
	rtVersion   = 16 // RT_VERSION：版本信息，资源管理器「详细信息」读它显示
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
//
// originalFilename 会写进版本资源的 OriginalFilename 栏（如 "yssh.exe"），
// 因此两个 exe 各生成一份；version 形如 "0.2.4"，由构建脚本注入。
func ResourceObject(originalFilename, version string) []byte {
	entries := images()

	// RT_ICON 的 ID 从 1 起编号，与 RT_GROUP_ICON 里记录的编号一一对应
	resources := make([]resource, 0, len(entries)+2)
	for i, e := range entries {
		resources = append(resources, resource{
			typ: rtIcon, id: uint32(i + 1), lang: resourceLang, data: e.data,
		})
	}
	resources = append(resources,
		resource{typ: rtGroupIcon, id: 1, lang: resourceLang, data: groupIconEntries(entries)},
		resource{typ: rtVersion, id: 1, lang: resourceLang, data: versionInfo(originalFilename, version)},
	)

	section, relocSites := buildResourceSection(resources)

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

// resource 是资源段里的一个条目。
type resource struct {
	typ  uint32 // 资源类型，如 rtIcon
	id   uint32 // 同一类型内唯一的编号
	lang uint32 // 语言 ID
	data []byte
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
// 段内布局（偏移均相对段起始，k 为资源类型数、n 为资源条目总数）：
//
//	0            根目录                  头部 + k 个类型条目
//	dirSize(k)   各类型的 ID 目录        依次排布，每块是头部 + 该类型的条目
//	…            各条目的「语言」目录    每块 24 字节（头部 + 1 个条目）
//	…            数据条目 × n            IMAGE_RESOURCE_DATA_ENTRY
//	…            数据块，每块按 4 字节对齐
func buildResourceSection(resources []resource) (section []byte, relocSites []int) {
	// 先按「类型升序、同类型内 ID 升序」排好，调用方因此不必关心传入顺序。
	// 这一步不能省：Windows 在每一层都做二分查找，排错了就找不到资源。
	sorted := make([]resource, len(resources))
	copy(sorted, resources)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].typ != sorted[j].typ {
			return sorted[i].typ < sorted[j].typ
		}
		return sorted[i].id < sorted[j].id
	})

	// 按类型切成连续段，记下每段覆盖 sorted 的哪一段区间
	type span struct {
		typ   uint32
		start int
		count int
	}
	var spans []span
	for i := 0; i < len(sorted); {
		j := i
		for j < len(sorted) && sorted[j].typ == sorted[i].typ {
			j++
		}
		spans = append(spans, span{typ: sorted[i].typ, start: i, count: j - i})
		i = j
	}

	// 各区域的起始偏移
	typeDirs := make([]int, len(spans))
	langDirs := make([]int, len(sorted))

	off := dirSize(len(spans))
	for i, sp := range spans {
		typeDirs[i] = off
		off += dirSize(sp.count)
	}
	for i := range sorted {
		langDirs[i] = off
		off += dirSize(1)
	}
	offDataEntries := off
	offBlobs := offDataEntries + len(sorted)*resDataEntrySize

	// 先排好每块数据的位置：紧跟目录区，每块按 4 字节对齐
	blobAt := make([]int, len(sorted))
	pos := offBlobs
	for i, r := range sorted {
		pos = align4(pos)
		blobAt[i] = pos
		pos += len(r.data)
	}

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

	// --- 根目录：各资源类型，按类型 ID 升序 ---
	directory(0, len(spans))
	for i, sp := range spans {
		child(resDirHeaderSize+i*resDirEntrySize, sp.typ, typeDirs[i])
	}

	// --- 类型层下的 ID 目录，同类型内按 ID 升序 ---
	for i, sp := range spans {
		directory(typeDirs[i], sp.count)
		for j := 0; j < sp.count; j++ {
			entry := typeDirs[i] + resDirHeaderSize + j*resDirEntrySize
			child(entry, sorted[sp.start+j].id, langDirs[sp.start+j])
		}
	}

	// --- 语言层：每个目录只有一条 en-US 条目，紧跟在头部之后 ---
	for i, r := range sorted {
		directory(langDirs[i], 1)
		leaf(langDirs[i]+resDirHeaderSize, r.lang, offDataEntries+i*resDataEntrySize)
	}

	// --- 数据条目 ---
	// 这里的「数据偏移」写的是段内偏移，链接器会按重定位把它换算成 RVA。
	relocSites = make([]int, 0, len(sorted))
	for i, r := range sorted {
		base := offDataEntries + i*resDataEntrySize
		put32(base, uint32(blobAt[i]))     // 数据偏移
		put32(base+4, uint32(len(r.data))) // 数据长度
		relocSites = append(relocSites, base)
		// 代码页与保留字段留 0
	}

	// --- 数据块 ---
	for i, r := range sorted {
		copy(sec[blobAt[i]:], r.data)
	}

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
