// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package session

import "errors"

// ErrSSHNotFound 表示系统中找不到 ssh 可执行文件。
var ErrSSHNotFound = errors.New("未找到 ssh 可执行文件，请先安装 OpenSSH 客户端（Debian/Ubuntu: apt install openssh-client）")

// sshFallbackPaths 是 PATH 中找不到 ssh 时的兜底搜索路径。
//
// Linux 上 ssh 几乎总在 PATH 里，这里只是给 PATH 被裁剪过的运行环境
// （cron、systemd 服务、精简容器）留一条退路。
var sshFallbackPaths = []string{
	"/usr/bin/ssh",
	"/usr/local/bin/ssh",
	"/bin/ssh",
}
