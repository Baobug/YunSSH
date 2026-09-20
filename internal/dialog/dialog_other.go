// SPDX-FileCopyrightText: 2026 Zhou Tianbo (Baobug)
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package dialog

import "log"

// 非 Windows 平台没有原生消息框，退化为日志输出。
// 保留这组实现是为了让依赖它的包在其它平台上仍能通过编译。

// Info 输出一条信息日志。
func Info(title, text string) { log.Printf("[info] %s: %s", title, text) }

// Error 输出一条错误日志。
func Error(title, text string) { log.Printf("[error] %s: %s", title, text) }

// Confirm 在非 Windows 平台一律返回 false。
// 这些平台本来也不支持本工具的安装流程，返回否定值更安全。
func Confirm(title, text string) bool {
	log.Printf("[confirm] %s: %s -> false", title, text)
	return false
}
