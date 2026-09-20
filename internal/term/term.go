// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

// Package term 负责终端相关的呈现细节：窗口标题、环境徽章与显示宽度计算。
//
// 所有输出都使用 ANSI 转义序列，不修改远端机器的任何配置。
package term

import (
	"fmt"
	"os"
	"strings"
)

// envBadges 把环境标识映射为标题徽章。
// 未登记的环境会原样大写显示，便于用户自定义。
var envBadges = map[string]string{
	"prod":       "PROD",
	"production": "PROD",
	"test":       "TEST",
	"stage":      "STAGE",
	"lab":        "LAB",
	"dev":        "DEV",
	"tmp":        "TMP",
}

// Badge 返回环境对应的徽章文本，环境为空时返回空字符串。
func Badge(env string) string {
	env = strings.TrimSpace(env)
	if env == "" {
		return ""
	}
	if b, ok := envBadges[strings.ToLower(env)]; ok {
		return b
	}
	return strings.ToUpper(env)
}

// ansiEnabled 表示当前输出目标是否接受 ANSI 转义序列。
// 输出被重定向到文件或管道时必须关闭，否则控制序列会污染内容
// （例如 `yssh ls > hosts.txt` 会把窗口标题序列写进文件）。
var ansiEnabled = true

// SetANSIEnabled 开关 ANSI 转义序列输出。
func SetANSIEnabled(on bool) { ansiEnabled = on }

// SetTitle 设置终端窗口标题（OSC 0）。
//
// 这是「防手滑」的第一道防线：连到生产环境时窗口标题立刻带上前缀，
// 断开后由 ResetTitle 恢复。不依赖远端 shell 的任何配合。
func SetTitle(alias, env string) {
	if !ansiEnabled {
		return
	}
	if b := Badge(env); b != "" {
		fmt.Fprintf(os.Stdout, "\x1b]0;[%s] %s\x07", b, alias)
		return
	}
	fmt.Fprintf(os.Stdout, "\x1b]0;%s\x07", alias)
}

// ResetTitle 恢复终端窗口标题为默认值。
func ResetTitle() {
	if !ansiEnabled {
		return
	}
	fmt.Fprint(os.Stdout, "\x1b]0;\x07")
}

// runeWidth 返回单个字符在等宽终端中占用的列数。
// 东亚宽字符占 2 列，其余占 1 列。表驱动而非引入外部依赖。
func runeWidth(r rune) int {
	switch {
	case r < 0x1100:
		return 1
	case r >= 0x1100 && r <= 0x115F: // 韩文字母
		return 2
	case r >= 0x2E80 && r <= 0xA4CF: // CJK 部首扩展、汉字、日文假名
		return 2
	case r >= 0xAC00 && r <= 0xD7A3: // 韩文音节
		return 2
	case r >= 0xF900 && r <= 0xFAFF: // CJK 兼容汉字
		return 2
	case r >= 0xFE30 && r <= 0xFE6F: // CJK 兼容形式
		return 2
	case r >= 0xFF00 && r <= 0xFF60: // 全角 ASCII
		return 2
	case r >= 0xFFE0 && r <= 0xFFE6: // 全角符号
		return 2
	default:
		return 1
	}
}

// DisplayWidth 估算字符串在终端中占用的列数。
func DisplayWidth(s string) int {
	w := 0
	for _, r := range s {
		w += runeWidth(r)
	}
	return w
}

// Pad 在右侧补空格，使显示宽度达到 width。已超宽时原样返回。
func Pad(s string, width int) string {
	gap := width - DisplayWidth(s)
	if gap <= 0 {
		return s
	}
	return s + strings.Repeat(" ", gap)
}

// Truncate 按显示宽度截断字符串，超长部分以 ".." 结尾。
func Truncate(s string, width int) string {
	if width <= 2 || DisplayWidth(s) <= width {
		return s
	}

	var b strings.Builder
	w := 0
	for _, r := range s {
		rw := runeWidth(r)
		if w+rw > width-2 {
			break
		}
		b.WriteRune(r)
		w += rw
	}
	b.WriteString("..")
	return b.String()
}
