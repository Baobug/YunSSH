// SPDX-FileCopyrightText: 2026 Zhou Tianbo (Baobug)
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package main

// 安装、卸载与开机自启依赖 Windows 注册表，其它平台给出明确提示而非静默失败。

func cmdInstall(args []string) int {
	errf("安装功能目前仅支持 Windows。")
	hintf("其它平台可自行把 yssh 复制到 PATH 目录，例如 /usr/local/bin。")
	return exitError
}

func cmdUninstall(args []string) int {
	errf("卸载功能目前仅支持 Windows。")
	return exitError
}

func cmdAutoStart(args []string) int {
	errf("开机自启目前仅支持 Windows。")
	return exitError
}
