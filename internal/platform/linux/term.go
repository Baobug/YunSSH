// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package linux

// EnableVT 在非 Windows 平台上无需额外处理，ANSI 转义序列本就受支持。
func EnableVT() {}

// SupportsANSI 报告当前输出是否为支持 ANSI 的终端。
func SupportsANSI() bool { return true }
