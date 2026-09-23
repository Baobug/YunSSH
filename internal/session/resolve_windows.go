// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package session

import "errors"

// ErrSSHNotFound 表示系统中找不到 ssh 可执行文件。
var ErrSSHNotFound = errors.New("未找到 ssh 可执行文件，请确认已启用 Windows 的 OpenSSH 客户端功能")

// sshFallbackPaths 是 PATH 中找不到 ssh 时的兜底搜索路径。
//
// Windows 的 OpenSSH 由「可选功能」装入系统目录，不一定出现在 PATH 里，
// 因此必须显式列出默认安装位置。
var sshFallbackPaths = []string{
	`C:\Windows\System32\OpenSSH\ssh.exe`,
	`C:\Program Files\OpenSSH\ssh.exe`,
}
