// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package main

// fallbackEditors 返回 $VISUAL / $EDITOR 都不可用时依次探测的编辑器。
//
// Windows 上 notepad 必然存在，放首位是为了让 yssh edit 的行为与本改动之前
// 逐字一致——若把它挪到末尾，装了 Git for Windows 的机器会先命中 vi。
func fallbackEditors() []string {
	return []string{"notepad", "sensible-editor", "editor", "nano", "vi"}
}
