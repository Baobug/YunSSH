// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package main

import "github.com/Baobug/YunSSH/internal/platform/linux"

// 兜底编辑器的候选表是平台差异，实现放在 internal/platform/linux。
func fallbackEditors() []string { return linux.FallbackEditors() }
