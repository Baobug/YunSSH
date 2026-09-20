// SPDX-FileCopyrightText: 2026 Zhou Tianbo (Baobug)
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package install

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"unicode/utf16"
)

// 开始菜单里显示的名字。
const shortcutName = "YunSSH"

// Shell Link 的固定头部大小与 CLSID（MS-SHLLINK 2.1）。
const (
	shellLinkHeaderSize = 76
	// 00021401-0000-0000-C000-000000000046
	shellLinkCLSID = "\x01\x14\x02\x00\x00\x00\x00\x00\xc0\x00\x00\x00\x00\x00\x00\x46"
)

// LinkFlags 位定义，只列出本文件用到的几个。
const (
	flagHasLinkInfo     = 0x00000002
	flagHasWorkingDir   = 0x00000010
	flagHasIconLocation = 0x00000040
	flagIsUnicode       = 0x00000080
)

// ShowCommand 取值：1 = SW_SHOWNORMAL
const swShowNormal = 1

// StartMenuDir 返回当前用户的开始菜单「程序」目录。
func StartMenuDir() (string, error) {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		return "", errors.New("环境变量 APPDATA 未设置")
	}
	return filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs"), nil
}

// ShortcutPath 返回开始菜单中快捷方式的完整路径。
func ShortcutPath() (string, error) {
	dir, err := StartMenuDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, shortcutName+".lnk"), nil
}

// StartMenuShortcutExists 报告开始菜单快捷方式是否存在。
func StartMenuShortcutExists() bool {
	path, err := ShortcutPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// createStartMenuShortcut 创建（或覆盖）开始菜单快捷方式。
//
// 快捷方式指向托盘程序而不是 CLI：从开始菜单点开应当直接进入托盘，
// 而不是弹出一个命令行窗口。
func createStartMenuShortcut(trayExe string) error {
	path, err := ShortcutPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建开始菜单目录失败: %w", err)
	}

	link := buildShellLink(
		trayExe,
		filepath.Dir(trayExe),
		trayExe+",0", // 图标取自 exe 自身的第 0 号资源
		swShowNormal,
	)

	if err := os.WriteFile(path, link, 0o644); err != nil {
		return fmt.Errorf("写入开始菜单快捷方式失败: %w", err)
	}
	return nil
}

// removeStartMenuShortcut 删除开始菜单快捷方式。
func removeStartMenuShortcut() error {
	path, err := ShortcutPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除开始菜单快捷方式失败: %w", err)
	}
	return nil
}

// buildShellLink 按 MS-SHLLINK 规范构造一个 .lnk 文件的内容。
//
// 直接写二进制而不借助 COM 或 PowerShell，有两个原因：不引入额外依赖，
// 以及安装过程不需要启动外部进程。格式本身是固定的，写对一次就长期稳定。
func buildShellLink(target, workingDir, iconLocation string, showCommand uint32) []byte {
	flags := uint32(flagHasLinkInfo | flagIsUnicode)
	if workingDir != "" {
		flags |= flagHasWorkingDir
	}
	if iconLocation != "" {
		flags |= flagHasIconLocation
	}

	var buf bytes.Buffer

	// --- ShellLinkHeader，共 76 字节 ---
	writeU32(&buf, shellLinkHeaderSize)
	buf.WriteString(shellLinkCLSID)
	writeU32(&buf, flags)
	writeU32(&buf, 0) // FileAttributes
	writeU64(&buf, 0) // CreationTime
	writeU64(&buf, 0) // AccessTime
	writeU64(&buf, 0) // WriteTime
	writeU32(&buf, 0) // FileSize
	writeU32(&buf, 0) // IconIndex
	writeU32(&buf, showCommand)
	writeU16(&buf, 0) // HotKey
	writeU16(&buf, 0) // Reserved1
	writeU32(&buf, 0) // Reserved2
	writeU32(&buf, 0) // Reserved3

	// --- LinkInfo ---
	buf.Write(buildLinkInfo(target))

	// --- StringData，顺序必须与规范一致：WorkingDir 在 IconLocation 之前 ---
	if workingDir != "" {
		buf.Write(encodeStringData(workingDir))
	}
	if iconLocation != "" {
		buf.Write(encodeStringData(iconLocation))
	}

	return buf.Bytes()
}

// buildLinkInfo 构造 LinkInfo 结构（MS-SHLLINK 2.3）。
//
// 路径同时提供 ANSI 与 Unicode 两份：虽然规范允许只用 ANSI，
// 但 Unicode 那份能保证含中文的安装路径也不会解析失败。
func buildLinkInfo(target string) []byte {
	ansiPath := append([]byte(target), 0) // 含结尾 NUL
	uniPath := encodeUTF16(target)        // 含结尾 NUL

	const (
		headerLen   = 0x24
		volumeIDLen = 0x10
	)

	headerSize := uint32(headerLen)
	volumeIDOffset := headerSize
	localBasePathOffset := volumeIDOffset + volumeIDLen
	commonPathSuffixOffset := localBasePathOffset + uint32(len(ansiPath))
	localBasePathOffsetUnicode := commonPathSuffixOffset + 1 // ANSI 后缀占 1 字节
	commonPathSuffixOffsetUnicode := localBasePathOffsetUnicode + uint32(len(uniPath))
	linkInfoSize := commonPathSuffixOffsetUnicode + 2 // Unicode 后缀占 2 字节

	var buf bytes.Buffer
	writeU32(&buf, linkInfoSize)
	writeU32(&buf, headerSize)
	writeU32(&buf, 0x00000001) // VolumeIDAndLocalBasePath
	writeU32(&buf, volumeIDOffset)
	writeU32(&buf, localBasePathOffset)
	writeU32(&buf, 0) // CommonNetworkRelativeLinkOffset
	writeU32(&buf, commonPathSuffixOffset)
	writeU32(&buf, localBasePathOffsetUnicode)
	writeU32(&buf, commonPathSuffixOffsetUnicode)

	// VolumeID：只写固定部分。VolumeLabelOffset 指向 VolumeID 之外，
	// 即表示没有卷标——这是被广泛采用的写法。
	writeU32(&buf, volumeIDLen)
	writeU32(&buf, 3) // DriveType = DRIVE_FIXED
	writeU32(&buf, 0) // DriveSerialNumber，0 表示不做校验
	writeU32(&buf, volumeIDLen)

	buf.Write(ansiPath) // LocalBasePath
	buf.WriteByte(0)    // CommonPathSuffix（空）
	buf.Write(uniPath)  // LocalBasePathUnicode
	writeU16(&buf, 0)   // CommonPathSuffixUnicode（空）

	return buf.Bytes()
}

// encodeStringData 按 StringData 格式编码字符串。
// 结构：字符数（uint16，不含终止符）+ UTF-16LE 内容 + NUL。
func encodeStringData(s string) []byte {
	runes := utf16.Encode([]rune(s))

	var buf bytes.Buffer
	writeU16(&buf, uint16(len(runes)))
	for _, r := range runes {
		writeU16(&buf, r)
	}
	writeU16(&buf, 0)

	return buf.Bytes()
}

// encodeUTF16 把字符串编码为 UTF-16LE 并追加 NUL 终止符。
// 与 encodeStringData 的区别是不写前置的字符数——LinkInfo 里的路径就是这个格式。
func encodeUTF16(s string) []byte {
	runes := utf16.Encode([]rune(s))

	out := make([]byte, 0, (len(runes)+1)*2)
	for _, r := range runes {
		out = append(out, byte(r), byte(r>>8))
	}
	return append(out, 0, 0)
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

func writeU64(buf *bytes.Buffer, v uint64) {
	writeU32(buf, uint32(v))
	writeU32(buf, uint32(v>>32))
}
