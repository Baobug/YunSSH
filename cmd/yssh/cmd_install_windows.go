// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/Baobug/YunSSH/internal/install"
)

// cmdInstall 把 YunSSH 安装到当前用户目录。
//
// 与双击 ysshtray.exe 的效果完全一致，区别只是这里用文本交互，
// 适合从终端或脚本调用。
func cmdInstall(args []string) int {
	autoStart := false
	addToPath := true
	askAutoStart := true

	for _, a := range args {
		switch a {
		case "--autostart":
			autoStart, askAutoStart = true, false
		case "--no-autostart":
			autoStart, askAutoStart = false, false
		case "--no-path":
			addToPath = false
		case "-y", "--yes":
			// 保持与 uninstall 一致的参数习惯，安装本身无需确认
		}
	}

	if askAutoStart {
		autoStart = askYesNo("是否设置开机自启？", true)
	}

	result, err := install.Install(install.Options{
		AutoStart: autoStart,
		AddToPath: addToPath,
		Version:   version,
	})
	if err != nil {
		errf("%v", err)
		return exitError
	}

	fmt.Printf("已安装到 %s\n", paint(ansiBlue, result.Dir))
	for _, f := range result.Files {
		hintf("  文件        %s", f)
	}
	if result.AutoStart {
		hintf("  开机自启    已开启")
	} else {
		hintf("  开机自启    未开启（可用 yssh autostart on 打开）")
	}
	if result.StartMenu {
		hintf("  开始菜单    已创建快捷方式")
	}
	if result.PathUpdated {
		hintf("  PATH        已加入，重新打开终端后 yssh 可直接使用")
	}
	hintf("  卸载        设置 → 应用 → YunSSH，或运行 yssh uninstall")
	fmt.Println()
	hintf("下一步：用 `yssh add <别名> <用户@主机>` 添加主机，或直接从托盘菜单操作。")

	return exitOK
}

// cmdUninstall 从当前用户目录移除 YunSSH。
func cmdUninstall(args []string) int {
	yes := false
	for _, a := range args {
		switch a {
		case "-y", "--yes":
			yes = true
		}
	}

	dir, err := install.Dir()
	if err != nil {
		hintf("YunSSH 尚未安装。")
		return exitOK
	}

	if !yes && !askYesNo(fmt.Sprintf("将从 %s 移除 YunSSH，继续？", dir), false) {
		hintf("已取消。")
		return exitOK
	}

	if err := install.Uninstall(); err != nil {
		errf("卸载失败: %v", err)
		return exitError
	}

	fmt.Println("已卸载。程序目录会在本进程退出后自动清理。")
	hintf("你的 ~/.ssh/config 未被改动。")
	return exitOK
}

// cmdAutoStart 查询或切换开机自启。
func cmdAutoStart(args []string) int {
	if len(args) == 0 {
		state := "已关闭"
		if install.AutoStartEnabled() {
			state = "已开启"
		}
		fmt.Printf("开机自启：%s\n", state)
		return exitOK
	}

	var enable bool
	switch strings.ToLower(args[0]) {
	case "on", "enable", "true", "1":
		enable = true
	case "off", "disable", "false", "0":
		enable = false
	default:
		errf("用法: yssh autostart on|off")
		return exitUsage
	}

	if err := install.SetAutoStart(enable); err != nil {
		errf("%v", err)
		return exitError
	}

	if enable {
		fmt.Println("已开启开机自启。")
	} else {
		fmt.Println("已关闭开机自启。")
	}
	return exitOK
}

// askYesNo 询问是/否，直接回车采用默认值。
//
// stdin 被重定向（非交互）时读取会立即返回，此时同样落到默认值，
// 因此该函数可以安全地用在校本里。
func askYesNo(prompt string, def bool) bool {
	hint := " [y/N] "
	if def {
		hint = " [Y/n] "
	}
	fmt.Print(prompt + hint)

	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return def
	}

	switch strings.ToLower(strings.TrimSpace(line)) {
	case "":
		return def
	case "y", "yes":
		return true
	default:
		return false
	}
}
