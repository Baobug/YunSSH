// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package linux

import "log"

// 非 Windows 平台没有原生消息框，退化为日志输出。
//
// 这几个函数目前只有 Windows 侧的托盘会调用，这里保留一份实现是为了让
// internal/dialog 的接口在两个平台上都成立——将来若有跨平台代码要用提示框，
// 不必再回头补。

// DialogInfo 输出一条信息日志。
func DialogInfo(title, text string) { log.Printf("[info] %s: %s", title, text) }

// DialogError 输出一条错误日志。
func DialogError(title, text string) { log.Printf("[error] %s: %s", title, text) }

// DialogConfirm 在非 Windows 平台一律返回 false。
// 这些平台本来也不支持本工具的安装流程，返回否定值更安全。
func DialogConfirm(title, text string) bool {
	log.Printf("[confirm] %s: %s -> false", title, text)
	return false
}
