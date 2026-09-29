// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package main

// fallbackEditors 返回 $VISUAL / $EDITOR 都不可用时依次探测的编辑器。
//
// 刻意不含 notepad：Linux 上名为 notepad 的可执行文件来自 wine 的包装脚本
// （/usr/bin/notepad → wine notepad.exe），不是原生编辑器，命中它只会弹出一个
// win32 记事本窗口，远不如 nano / vi 可用。确实想用它的话，自己
// export EDITOR=notepad 即可——显式选择始终优先于兜底候选。
func fallbackEditors() []string {
	return []string{"sensible-editor", "editor", "nano", "vi"}
}
