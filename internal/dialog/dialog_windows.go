// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

//go:build windows

// Package dialog 提供最简的原生提示框。
//
// 实现集中在 internal/platform/{windows,linux}；本包只是面向调用方
// （internal/tray、cmd/ysshtray）的稳定接口，因此调用方不必关心平台。
// Windows 侧走 user32.dll 的 MessageBoxW，不引入任何 GUI 框架。
package dialog

import "github.com/Baobug/YunSSH/internal/platform/windows"

// Info 弹出信息提示。
func Info(title, text string) { windows.DialogInfo(title, text) }

// Error 弹出错误提示。
func Error(title, text string) { windows.DialogError(title, text) }

// Confirm 弹出「是/否」询问，返回用户是否选择了「是」。
func Confirm(title, text string) bool { return windows.DialogConfirm(title, text) }
