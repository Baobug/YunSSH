// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/Baobug/YunSSH/internal/term"
)

// TestInitColorsHonorsNoColor 钉住 NO_COLOR 约定（https://no-color.org）。
//
// 断言分两层，因为「关掉颜色」既是一个布尔量，也是用户能看见的后果：
// colorEnabled 必须为 false，且 term 包真的不再写出转义序列。
// 若有人把 NO_COLOR 判断挪走或写反，`NO_COLOR=1 yssh ls` 会重新带上
// 颜色与窗口标题控制序列，这条测试会失败。
//
// 这里刻意只覆盖 NO_COLOR，不覆盖「输出被重定向」：那条判断属于
// internal/platform/{windows,linux} 的 SupportsANSI，且 Windows 实现读的是
// 真实的标准输出句柄，测试里换掉 os.Stdout 影响不到它，写在这里会随
// 运行环境时红时绿。重定向行为由 internal/platform/linux 的测试负责钉住。
func TestInitColorsHonorsNoColor(t *testing.T) {
	restore := snapshotColorState()
	defer restore()

	t.Setenv("NO_COLOR", "1")

	out := captureStdout(t, func() {
		initColors()
		term.SetTitle("web", "prod")
	})

	if colorEnabled {
		t.Error("设置 NO_COLOR 后 colorEnabled 应为 false")
	}
	if strings.Contains(out, "\x1b") {
		t.Errorf("NO_COLOR 生效时不应写出任何转义序列，实际得到 %q", out)
	}
}

// snapshotColorState 记录并返回恢复函数，避免本用例污染同包其它用例。
//
// colorEnabled 与 term 包内部的 ansiEnabled 都是进程级全局量，
// 改了不还原会让后续用例的断言失去意义。
func snapshotColorState() func() {
	prevColor := colorEnabled
	prevANSI := term.ANSIEnabled()
	return func() {
		colorEnabled = prevColor
		term.SetANSIEnabled(prevANSI)
	}
}
