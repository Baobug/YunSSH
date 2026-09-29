// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package session

import "github.com/Baobug/YunSSH/internal/platform/linux"

// ssh 兜底路径与错误文案的平台实现集中在 internal/platform/linux；
// 这里把名字转发进 session 包，于是 resolve.go 里完全不出现平台判断。

var (
	ErrSSHNotFound   = linux.ErrSSHNotFound
	sshFallbackPaths = linux.SSHFallbackPaths
)
