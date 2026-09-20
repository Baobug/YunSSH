// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

package appicon

import (
	"bytes"
	"encoding/binary"
	"strconv"
	"strings"
	"unicode/utf16"
)

// VS_FIXEDFILEINFO 的布局与固定取值。
//
// 结构见 https://learn.microsoft.com/windows/win32/api/verrsrc/ns-verrsrc-vs_fixedfileinfo
const (
	vsFixedFileInfoSize = 52
	vsSignature         = 0xFEEF04BD // VS_FFI_SIGNATURE
	vsStrucVersion      = 0x00010000 // VS_FFI_STRUCVERSION，即结构版本 1.0
	vsFileFlagsMask     = 0x0000003F // VS_FFI_FILEFLAGSMASK
	vsFileOS            = 0x00040004 // VOS_NT_WINDOWS32
	vsFileType          = 0x00000001 // VFT_APP
)

// 版本资源的语言与代码页。二者拼起来就是 StringTable 的键名 "040904b0"：
// 0x0409 是 en-US，0x04b0 表示 Unicode。
//
// 资源本身不含需要本地化的文案——面向用户的文字由程序自己输出——所以固定
// 用 en-US，让系统按回退规则取用即可。
const (
	versionLang    = 0x0409
	versionCharset = 0x04b0
	stringTableKey = "040904b0"
)

// 版本资源里各栏位的取值。
//
// CompanyName 用 GitHub 账号而不是真名：这一栏会显示在资源管理器的
// 「详细信息」里，与提交作者保持一致更容易对上号；完整的著作权归属
// 写在 LegalCopyright 里，那是真正有声明作用的一栏。
const (
	companyName     = "Baobug"
	productName     = "YunSSH"
	fileDescription = "YunSSH — Windows 终端 SSH 会话管理工具"
	legalCopyright  = "Copyright 2026 Zhou Tianbao (Baobug). Licensed under the Apache License, Version 2.0."
	comments        = "含第三方开源组件，见随附的 THIRD-PARTY-NOTICES.md"
)

// versionBlock 是版本资源里的一层节点。
//
// 每一层都是同样的形状：
//
//	wLength      WORD     本层（含全部子层）的总字节数
//	wValueLength WORD     值长度：二进制层是字节数，文本层是字符数
//	wType        WORD     0 = 二进制，1 = 文本
//	szKey        WCHAR[]  键名，UTF-16LE，以 0 结尾
//	padding1              补零对齐到 4 字节
//	Value                 可省略
//	padding2              补零对齐到 4 字节
//	Children              子层
//
// wLength 要等子层全部写完才知道，所以 encode 先占位、最后回填。
type versionBlock struct {
	key      string
	typ      uint16
	valueLen int
	value    []byte
	children []*versionBlock
}

// encode 序列化本层及全部子层，并回填自己的 wLength。
func (b *versionBlock) encode() []byte {
	var buf bytes.Buffer

	writeU16(&buf, 0) // wLength：见函数末尾回填
	writeU16(&buf, uint16(b.valueLen))
	writeU16(&buf, b.typ)
	buf.Write(utf16Z(b.key))
	padTo4(&buf)

	if len(b.value) > 0 {
		buf.Write(b.value)
		padTo4(&buf)
	}
	for _, child := range b.children {
		buf.Write(child.encode())
	}

	out := buf.Bytes()
	binary.LittleEndian.PutUint16(out, uint16(len(out)))
	return out
}

// versionInfo 生成 RT_VERSION 资源的内容。
//
// originalFilename 写进 OriginalFilename 栏（如 "yssh.exe"）；version 形如
// "0.2.4"，也可带 v 前缀或 -dirty 之类的后缀。解析不出的部分按 0 处理，
// 因此临时构建不会写出一个看似正式的版本号。
func versionInfo(originalFilename, version string) []byte {
	major, minor, patch := parseVersion(version)

	// 文本版本号保留原始字符串（只去掉 v 前缀）：写成 "0.0.0-dev" 比伪造成
	// "0.0.0" 更诚实，也能一眼看出这是未经 build.bat 注入的临时构建。
	text := strings.TrimPrefix(version, "v")

	// 栏位按字母序排列，与 Windows 自带资源的排版一致。
	// 注意 InternalName 取的是不含扩展名的程序名，这是该栏的惯例。
	fields := []struct{ key, value string }{
		{"Comments", comments},
		{"CompanyName", companyName},
		{"FileDescription", fileDescription},
		{"FileVersion", text},
		{"InternalName", strings.TrimSuffix(originalFilename, ".exe")},
		{"LegalCopyright", legalCopyright},
		{"OriginalFilename", originalFilename},
		{"ProductName", productName},
		{"ProductVersion", text},
	}

	table := &versionBlock{key: stringTableKey, typ: 1}
	for _, f := range fields {
		table.children = append(table.children, textBlock(f.key, f.value))
	}

	// Translation 的值是一对 16 位：语言在前、代码页在后。
	translationValue := make([]byte, 4)
	binary.LittleEndian.PutUint16(translationValue, uint16(versionLang))
	binary.LittleEndian.PutUint16(translationValue[2:], uint16(versionCharset))

	translation := &versionBlock{
		key:      "Translation",
		typ:      0,
		valueLen: 4,
		value:    translationValue,
	}

	root := &versionBlock{
		key:      "VS_VERSION_INFO",
		typ:      0,
		valueLen: vsFixedFileInfoSize,
		value:    fixedFileInfo(major, minor, patch),
		children: []*versionBlock{
			{key: "StringFileInfo", typ: 1, children: []*versionBlock{table}},
			{key: "VarFileInfo", typ: 1, children: []*versionBlock{translation}},
		},
	}

	return root.encode()
}

// textBlock 构造一个文本层。
//
// wValueLength 记的是**字符数且不含结尾的 0**，这是 Windows 对 String 层的约定；
// 根层的二进制值才按字节数记。
func textBlock(key, value string) *versionBlock {
	encoded := utf16Z(value)
	return &versionBlock{
		key:      key,
		typ:      1,
		valueLen: len(encoded)/2 - 1,
		value:    encoded,
	}
}

// fixedFileInfo 生成定长 52 字节的 VS_FIXEDFILEINFO。
func fixedFileInfo(major, minor, patch int) []byte {
	b := make([]byte, vsFixedFileInfoSize)
	put := func(off int, v uint32) { binary.LittleEndian.PutUint32(b[off:], v) }

	// 高 16 位放主版本、低 16 位放次版本；修订号放低位双字的
	// 高 16 位，这样读出来就是熟悉的 主.次.修.构建 形态。
	versions := uint32(major)<<16 | uint32(minor)
	revision := uint32(patch) << 16

	put(0, vsSignature)
	put(4, vsStrucVersion)
	put(8, versions)  // dwFileVersionMS
	put(12, revision) // dwFileVersionLS
	put(16, versions) // dwProductVersionMS
	put(20, revision) // dwProductVersionLS
	put(24, vsFileFlagsMask)
	put(28, 0) // dwFileFlags：不使用任何标志位
	put(32, vsFileOS)
	put(36, vsFileType)
	put(40, 0) // dwFileSubtype：VFT_APP 无子类型
	put(44, 0) // dwFileDateMS
	put(48, 0) // dwFileDateLS

	return b
}

// parseVersion 从 "v0.2.4"、"0.2.4"、"0.2.4-5-gabc123" 里取出前三段数字。
func parseVersion(version string) (int, int, int) {
	v := strings.TrimPrefix(strings.TrimSpace(version), "v")
	// 丢掉 -dirty / +meta 之类的后缀
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}

	var nums [3]int
	for i, part := range strings.Split(v, ".") {
		if i >= len(nums) {
			break
		}
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || n < 0 || n > 0xFFFF {
			return 0, 0, 0
		}
		nums[i] = n
	}
	return nums[0], nums[1], nums[2]
}

// utf16Z 把字符串编码成 UTF-16LE，并补一个结尾的 0。
func utf16Z(s string) []byte {
	units := append(utf16.Encode([]rune(s)), 0)
	out := make([]byte, len(units)*2)
	for i, u := range units {
		binary.LittleEndian.PutUint16(out[i*2:], u)
	}
	return out
}

// padTo4 补零到 4 字节边界。
func padTo4(buf *bytes.Buffer) {
	if n := align4(buf.Len()) - buf.Len(); n > 0 {
		buf.Write(make([]byte, n))
	}
}
