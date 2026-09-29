// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

//go:build windows

// Package windows 收纳 YunSSH 在 Windows 上的平台实现。
//
// 凡「同一件事、两个平台做法不同」的能力都集中在这里，与
// internal/platform/linux 成对出现：终端能力、原生提示框、
// ssh 兜底路径、兜底编辑器。
//
// 上层包（internal/term、internal/session、internal/dialog、cmd/yssh）
// 只保留与平台无关的部分，平台差异通过一层薄委托进入本包。
// 因此想知道 Windows 上到底怎么做的，看这一个目录即可。
//
// 注意边界：整包天生只服务单一平台的模块（internal/tray、
// internal/install、internal/launcher、internal/appicon）不搬进来，
// 它们的包名加上 build tag 已经自证身份。
package windows
