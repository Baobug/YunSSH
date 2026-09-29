// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

// Package dialog 提供最简的原生提示框。
//
// 实现集中在 internal/platform/{windows,linux}；本包只是面向调用方
// （internal/tray、cmd/ysshtray）的稳定接口，因此调用方不必关心平台。
// 非 Windows 平台没有原生消息框，实现在 linux 包里退化为日志输出。
package dialog

import "github.com/Baobug/YunSSH/internal/platform/linux"

// Info 输出一条信息日志。
func Info(title, text string) { linux.DialogInfo(title, text) }

// Error 输出一条错误日志。
func Error(title, text string) { linux.DialogError(title, text) }

// Confirm 在非 Windows 平台一律返回 false。
func Confirm(title, text string) bool { return linux.DialogConfirm(title, text) }
