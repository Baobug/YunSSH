// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package install

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unicode/utf16"

	"golang.org/x/sys/windows/registry"
)

// 快捷方式显示的名字。
const shortcutName = "YunSSH"

// Place 表示快捷方式放在哪里。
type Place int

const (
	// PlaceStartMenu 是开始菜单的「程序」目录。
	PlaceStartMenu Place = iota
	// PlaceDesktop 是当前用户的桌面。
	PlaceDesktop
)

// String 返回用于提示文案的位置名称。
func (p Place) String() string {
	if p == PlaceDesktop {
		return "桌面"
	}
	return "开始菜单"
}

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

// DesktopDir 返回当前用户的桌面目录。
//
// 桌面位置可能被重定向（OneDrive 接管、组策略调整等），所以优先读注册表里
// 的记录，读不到再回退到 %USERPROFILE%\Desktop。直接拼 %USERPROFILE%\Desktop
// 在这类环境里会指向一个并不存在的目录——快捷方式写了，用户却看不到。
func DesktopDir() (string, error) {
	const keyPath = `Software\Microsoft\Windows\CurrentVersion\Explorer\User Shell Folders`

	if key, err := registry.OpenKey(registry.CURRENT_USER, keyPath, registry.QUERY_VALUE); err == nil {
		value, _, readErr := key.GetStringValue("Desktop")
		key.Close()

		if readErr == nil && value != "" {
			// 存的是 REG_EXPAND_SZ，需要展开 %USERPROFILE% 这类变量
			if expanded, expandErr := registry.ExpandString(value); expandErr == nil {
				return expanded, nil
			}
			return value, nil
		}
	}

	home := os.Getenv("USERPROFILE")
	if home == "" {
		return "", errors.New("无法确定桌面路径")
	}
	return filepath.Join(home, "Desktop"), nil
}

// Dir 返回该位置对应的目录。
func (p Place) Dir() (string, error) {
	if p == PlaceDesktop {
		return DesktopDir()
	}
	return StartMenuDir()
}

// Path 返回快捷方式的完整路径。
func (p Place) Path() (string, error) {
	dir, err := p.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, shortcutName+".lnk"), nil
}

// Exists 报告该位置的快捷方式是否已存在。
func (p Place) Exists() bool {
	path, err := p.Path()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// Create 创建（或覆盖）快捷方式。
//
// 快捷方式指向托盘程序而不是 CLI：从这里点开应当直接进入托盘，
// 而不是弹出一个命令行窗口。
func (p Place) Create(trayExe string) error {
	path, err := p.Path()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建%s目录失败: %w", p, err)
	}

	link := buildShellLink(
		trayExe,
		filepath.Dir(trayExe),
		trayExe+",0", // 图标取自 exe 自身的第一个图标组
		swShowNormal,
	)

	if err := os.WriteFile(path, link, 0o644); err != nil {
		return fmt.Errorf("写入%s快捷方式失败: %w", p, err)
	}

	notifyShell()
	return nil
}

// Remove 删除快捷方式。文件不存在时视为成功。
func (p Place) Remove() error {
	path, err := p.Path()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除%s快捷方式失败: %w", p, err)
	}

	notifyShell()
	return nil
}

// notifyShell 通知外壳「关联信息已变更」，促使它重新扫描开始菜单与桌面。
//
// 这一步不能省。直接往磁盘写 .lnk 只是放了个文件，外壳有自己的索引——
// 不同步通知的话，开始菜单不会刷新，用户看到的仍然是"装完了但哪都没有"。
// SHCNE_ASSOCCHANGED 是让外壳重读快捷方式集合的标准方式，
// 安装程序普遍在创建快捷方式后调用它。
func notifyShell() {
	const (
		shcneAssocChanged = 0x08000000
		shcnfIdList       = 0x0000
		shcnfFlush        = 0x1000
	)

	shell32 := syscall.NewLazyDLL("shell32.dll")
	proc := shell32.NewProc("SHChangeNotify")

	_, _, _ = proc.Call(
		shcneAssocChanged,
		shcnfIdList|shcnfFlush, // FLUSH 让调用返回前就把变更广播出去
		0,
		0,
	)
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
