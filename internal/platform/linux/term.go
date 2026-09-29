// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package linux

import "os"

// EnableVT 在非 Windows 平台上无需额外处理，ANSI 转义序列本就受支持。
func EnableVT() {}

// SupportsANSI 报告标准输出是否为接受 ANSI 转义序列的终端。
//
// 与 internal/platform/windows 的同名函数保持对称：输出被重定向到文件或管道时
// 返回 false。这不是锦上添花——`yssh ls > hosts.txt` 若照常输出，OSC 0 窗口标题
// 序列会被写进文件，把纯文本结果污染成夹带控制字符的内容。
//
// 判定有两条依据，都不引入任何第三方依赖（为了一个 isatty 去拉
// golang.org/x/term 会破坏「单文件静态二进制、scp 即用」）：
//
//  1. os.Stdout 是否为字符设备。终端是字符设备，常规文件与管道不是。
//     已知边界：/dev/null、/dev/zero 也是字符设备，重定向到它们会被判为终端。
//     这个误差是可接受的——它们是只写不看的黑洞，多写几个转义序列没有任何影响。
//     要精确区分，得对 fd 1 发 TCGETS 并处理 ENOTTY，代价是把 unsafe 与原始
//     系统调用引进项目，为一个没有可观测后果的边界不值得。
//  2. TERM 是否为 "dumb"。这是 terminfo 的约定，表示该终端没有能力处理转义
//     序列（Emacs 的 M-x shell、部分串口控制台、精简 CI 环境都会这么设）。
//
// NO_COLOR 不在这里判断：它跨平台成立且与平台无关，由调用方 cmd/yssh 处理。
func SupportsANSI() bool { return supportsANSI(os.Stdout, os.Getenv) }

// supportsANSI 是 SupportsANSI 的可测内核：把输出目标与环境查询抽成参数，
// 测试即可用临时文件、管道、/dev/null 覆盖各条分支，无需真的接一个终端。
func supportsANSI(out *os.File, getenv func(string) string) bool {
	if out == nil {
		return false
	}
	if getenv("TERM") == "dumb" {
		return false
	}
	fi, err := out.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
