// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

// Package linux 收纳 YunSSH 在非 Windows 平台上的平台实现。
//
// 与 internal/platform/windows 成对：凡是需要按平台分叉的能力，
// 两边各有一份实现——终端能力、提示框、ssh 兜底路径、兜底编辑器。
//
// 构建约束刻意写作 !windows 而不是 linux：本包的内容（ANSI 终端、
// 无原生消息框、FHS 兜底路径、sensible-editor / nano / vi）在 macOS
// 与 BSD 上同样成立，用 !windows 可以让这些平台直接编译通过，
// 不必再各写一份。目录名取 linux 是因为当前实际交付目标只有它。
package linux
