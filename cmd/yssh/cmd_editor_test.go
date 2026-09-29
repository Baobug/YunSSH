// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"runtime"
	"testing"
)

// TestFallbackEditors 钉住两个平台的兜底编辑器契约。
//
// 非 Windows 侧的重点是「不得出现 notepad」：Linux 上那个名字来自 wine 的包装
// 脚本（/usr/bin/notepad → wine notepad.exe），一旦有人把它加回候选表，
// yssh edit 就会去开一个 win32 记事本，这条测试会立刻失败。
//
// Windows 侧则锁定「首位仍是 notepad」，防止有人为了 Linux 顺手把它挪走，
// 从而改变 Windows 上的既有行为。
func TestFallbackEditors(t *testing.T) {
	list := fallbackEditors()
	if len(list) == 0 {
		t.Fatal("兜底候选表不应为空")
	}

	if runtime.GOOS == "windows" {
		if list[0] != "notepad" {
			t.Errorf("Windows 首位应为 notepad（保持既有行为），实际为 %q", list[0])
		}
		return
	}

	for _, name := range list {
		if name == "notepad" {
			t.Errorf("非 Windows 平台的兜底候选不应包含 notepad(那是 wine 的包装脚本): %v", list)
		}
	}
}
