// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package term

import "github.com/Baobug/YunSSH/internal/platform/linux"

// 终端能力的平台实现集中在 internal/platform/linux；本文件只做转发，
// 让 internal/term 的调用方（含与平台无关的 main.go / ui.go）不必关心平台。

// EnableVT 在非 Windows 平台上无需额外处理，ANSI 转义序列本就受支持。
func EnableVT() { linux.EnableVT() }

// SupportsANSI 报告当前输出是否为支持 ANSI 的终端。
func SupportsANSI() bool { return linux.SupportsANSI() }
