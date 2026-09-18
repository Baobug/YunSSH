//go:build windows

// Package dialog 提供最简的 Windows 原生对话框。
//
// 只依赖 user32.dll 的 MessageBoxW，不引入任何 GUI 框架——
// 对这个工具来说，安装确认和错误提示各一个对话框就够了。
package dialog

import (
	"syscall"
	"unsafe"
)

// MessageBox 的标志位。
const (
	flagOK            = 0x00000000
	flagYesNo         = 0x00000004
	flagIconError     = 0x00000010
	flagIconQuestion  = 0x00000020
	flagIconInfo      = 0x00000040
	flagSetForeground = 0x00010000
)

// MessageBox 的返回值。
const resultYes = 6

var procMessageBoxW = syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW")

// Info 弹出信息提示。
func Info(title, text string) {
	messageBox(title, text, flagOK|flagIconInfo)
}

// Error 弹出错误提示。
func Error(title, text string) {
	messageBox(title, text, flagOK|flagIconError)
}

// Confirm 弹出「是/否」询问，返回用户是否选择了「是」。
func Confirm(title, text string) bool {
	return messageBox(title, text, flagYesNo|flagIconQuestion) == resultYes
}

// messageBox 封装 Win32 MessageBoxW。
func messageBox(title, text string, flags uint32) int {
	titlePtr, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return 0
	}
	textPtr, err := syscall.UTF16PtrFromString(text)
	if err != nil {
		return 0
	}

	// 带上 SetForeground：托盘程序通常没有前台窗口，
	// 否则对话框可能被其它窗口盖住，用户看不到。
	ret, _, _ := procMessageBoxW.Call(
		0,
		uintptr(unsafe.Pointer(textPtr)),
		uintptr(unsafe.Pointer(titlePtr)),
		uintptr(flags|flagSetForeground),
	)
	return int(ret)
}
