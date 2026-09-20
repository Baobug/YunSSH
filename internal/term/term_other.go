// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package term

// EnableVT 在非 Windows 平台上无需额外处理，ANSI 转义序列本就受支持。
func EnableVT() {}

// SupportsANSI 在非 Windows 平台上假定终端支持 ANSI。
// 严格判断可检查 TERM 环境变量或调用 isatty，但本项目当前只面向 Windows。
func SupportsANSI() bool { return true }
