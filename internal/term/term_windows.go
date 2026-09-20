// SPDX-FileCopyrightText: 2026 Zhou Tianbo (Baobug)
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package term

import (
	"syscall"
	"unsafe"
)

// enableVirtualTerminalProcessing 是 SetConsoleMode 的 VT 处理标志位。
const enableVirtualTerminalProcessing = 0x0004

var (
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleMode = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode = kernel32.NewProc("SetConsoleMode")
)

// EnableVT 尝试为当前控制台开启 ANSI 转义序列处理。
//
// Windows Terminal 默认已开启；这里是为传统 conhost / cmd.exe 做的兜底。
// 失败时静默返回：最坏情况只是窗口标题和颜色不生效，不影响连接功能。
func EnableVT() {
	handle, err := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
	if err != nil {
		return
	}

	var mode uint32
	if ret, _, _ := procGetConsoleMode.Call(uintptr(handle), uintptr(unsafe.Pointer(&mode))); ret == 0 {
		// 输出被重定向到文件或管道，没有控制台模式可设置。
		return
	}
	if mode&enableVirtualTerminalProcessing != 0 {
		return
	}
	_, _, _ = procSetConsoleMode.Call(uintptr(handle), uintptr(mode|enableVirtualTerminalProcessing))
}

// SupportsANSI 报告当前输出是否为支持 ANSI 的终端。
// 输出被重定向时返回 false，调用方据此关闭颜色与标题控制序列。
func SupportsANSI() bool {
	handle, err := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
	if err != nil {
		return false
	}
	var mode uint32
	ret, _, _ := procGetConsoleMode.Call(uintptr(handle), uintptr(unsafe.Pointer(&mode)))
	return ret != 0
}
