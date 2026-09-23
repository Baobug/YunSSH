// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package main

// 安装、卸载与开机自启在 Windows 上依赖注册表与托盘；其它平台用不了，
// 但必须给出可执行的替代路径，而不是一句「仅支持 Windows」把人堵回去。

func cmdInstall(args []string) int {
	errf("yssh install 仅适用于 Windows —— 它做的是注册表登记、快捷方式与托盘常驻。")
	hintf("Linux 请用随发布包提供的安装脚本：")
	hintf("    bash install.sh                  # 装到 ~/.local/bin")
	hintf("    sudo bash install.sh --system    # 装到 /usr/local/bin")
	hintf("可选：--path 把安装目录写入 shell rc，--completion 安装 bash 补全。")
	return exitError
}

func cmdUninstall(args []string) int {
	errf("yssh uninstall 仅适用于 Windows。")
	hintf("Linux 请用安装脚本卸载：bash install.sh --uninstall")
	hintf("（系统级安装需 sudo：sudo bash install.sh --system --uninstall）")
	return exitError
}

func cmdAutoStart(args []string) int {
	errf("yssh 在 Linux 上不提供开机自启。")
	hintf("这里没有托盘常驻进程，也没有需要开机唤起的东西 ——")
	hintf("列表与连接都是按需执行的命令行操作。")
	return exitError
}
