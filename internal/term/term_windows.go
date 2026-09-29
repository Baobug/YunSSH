// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package term

import "github.com/Baobug/YunSSH/internal/platform/windows"

// 终端能力的平台实现集中在 internal/platform/windows；本文件只做转发，
// 让 internal/term 的调用方（含与平台无关的 main.go / ui.go）不必关心平台。

// EnableVT 尝试为当前控制台开启 ANSI 转义序列处理。
func EnableVT() { windows.EnableVT() }

// SupportsANSI 报告当前输出是否为支持 ANSI 的终端。
func SupportsANSI() bool { return windows.SupportsANSI() }
