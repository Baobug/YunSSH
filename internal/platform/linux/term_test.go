// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package linux

import (
	"os"
	"testing"
)

// TestSupportsANSIReportsRedirectedOutput 钉住「输出被重定向时必须返回 false」。
//
// 这是 Linux 侧原先缺失的一条：SupportsANSI 恒返回 true，与 Windows 侧
// 「重定向返回 false」的行为不对称，只能靠调用方 cmd/yssh 补一次同样的判断兜住，
// 同一个知识存在两处。有人把它改回 true 时，`yssh ls > hosts.txt` 就会把
// OSC 0 窗口标题序列写进文件，这条测试会立刻失败。
func TestSupportsANSIReportsRedirectedOutput(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}
	defer file.Close()

	pipeRead, pipeWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("创建管道失败: %v", err)
	}
	defer pipeRead.Close()
	defer pipeWrite.Close()

	cases := []struct {
		name string
		out  *os.File
		want bool
	}{
		{"空输出目标", nil, false},
		{"常规文件（yssh ls > hosts.txt）", file, false},
		{"管道（yssh ls | cat）", pipeWrite, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := supportsANSI(tc.out, os.Getenv); got != tc.want {
				t.Errorf("supportsANSI() = %v, 期望 %v", got, tc.want)
			}
		})
	}
}

// TestSupportsANSIHonorsDumbTerminal 钉住 TERM=dumb 约定。
//
// 对照目标刻意选 /dev/null 而不是临时文件：它是字符设备，若没有 TERM=dumb
// 那一条，supportsANSI 会判为 true。所以这条用例能真正区分「有没有实现 dumb
// 分支」，不会被「常规文件本来就返回 false」蒙混过去。
func TestSupportsANSIHonorsDumbTerminal(t *testing.T) {
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Skipf("无法打开 %s: %v", os.DevNull, err)
	}
	defer devNull.Close()

	dumb := func(string) string { return "dumb" }
	if supportsANSI(devNull, dumb) {
		t.Errorf("TERM=dumb 时应返回 false，实际为 true")
	}

	// 同一条分支的边界：TERM 为空视为未设置，仍按字符设备判定。
	empty := func(string) string { return "" }
	if !supportsANSI(devNull, empty) {
		t.Errorf("TERM 未设置且输出为字符设备时应返回 true，实际为 false")
	}
}

// TestSupportsANSIWrapsStdout 钉住公开入口确实把 os.Stdout 与 os.Getenv
// 交给了判定内核。
//
// 断言两边一致而非写死真假，是为了让这条测试在两种运行环境下都成立：
// `go test` 下测试进程的标准输出是管道（期望 false），而开发者手工执行
// 带终端的测试二进制时期望 true。无论哪种，只要有人把 SupportsANSI 改回
// 硬编码的 return true，管道场景就会不一致并失败。
func TestSupportsANSIWrapsStdout(t *testing.T) {
	want := supportsANSI(os.Stdout, os.Getenv)
	if got := SupportsANSI(); got != want {
		t.Errorf("SupportsANSI() = %v, 与 supportsANSI(os.Stdout, os.Getenv) 的 %v 不一致", got, want)
	}
}
