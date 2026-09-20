// SPDX-FileCopyrightText: 2026 Zhou Tianbo (Baobug)
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Baobug/YunSSH/internal/history"
	"github.com/Baobug/YunSSH/internal/term"
)

// ANSI 颜色码。仅在支持 ANSI 的终端下启用。
const (
	ansiReset  = "\x1b[0m"
	ansiDim    = "\x1b[2m"
	ansiRed    = "\x1b[31m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiBlue   = "\x1b[34m"
)

// colorEnabled 表示是否输出 ANSI 颜色。
var colorEnabled bool

// initColors 决定是否启用颜色与转义序列输出。
//
// 遵循 NO_COLOR 约定（https://no-color.org）；输出被重定向到文件或管道时自动关闭。
func initColors() {
	colorEnabled = term.SupportsANSI() && os.Getenv("NO_COLOR") == ""

	if fi, err := os.Stdout.Stat(); err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		colorEnabled = false
	}

	// 窗口标题（OSC 0）与颜色共用同一个判断，避免重定向时污染内容。
	term.SetANSIEnabled(colorEnabled)
}

// paint 按需为文本着色。
func paint(color, s string) string {
	if !colorEnabled || color == "" || s == "" {
		return s
	}
	return color + s + ansiReset
}

// envColor 返回环境标识对应的颜色。
//
// 生产环境用红色是刻意的：它出现在列表第一列和终端标题上，
// 目的就是在误连生产机器之前让人多看一眼。
func envColor(env string) string {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "prod", "production":
		return ansiRed
	case "test":
		return ansiGreen
	case "stage":
		return ansiYellow
	case "lab", "dev":
		return ansiBlue
	default:
		return ansiDim
	}
}

// errf 输出错误信息到 stderr。
func errf(format string, a ...any) {
	fmt.Fprintln(os.Stderr, paint(ansiRed, "yssh: "+fmt.Sprintf(format, a...)))
}

// hintf 输出次要提示到 stderr。
func hintf(format string, a ...any) {
	fmt.Fprintln(os.Stderr, paint(ansiDim, fmt.Sprintf(format, a...)))
}

// cell 是表格中的一个单元。text 用于计算列宽，color 用于着色。
type cell struct {
	text  string
	color string
}

// renderTable 输出一张对齐的表格。
//
// 列宽按显示宽度计算（东亚宽字符占 2 列），因此中英混排也能对齐。
func renderTable(w io.Writer, headers []cell, rows [][]cell) {
	cols := len(headers)
	widths := make([]int, cols)
	for i, h := range headers {
		widths[i] = term.DisplayWidth(h.text)
	}
	for _, row := range rows {
		for i := 0; i < cols && i < len(row); i++ {
			if n := term.DisplayWidth(row[i].text); n > widths[i] {
				widths[i] = n
			}
		}
	}

	writeRow := func(cells []cell) {
		var b strings.Builder
		for i := 0; i < cols; i++ {
			if i > 0 {
				b.WriteString("  ")
			}
			var c cell
			if i < len(cells) {
				c = cells[i]
			}

			padded := term.Pad(c.text, widths[i])
			if c.color == "" {
				b.WriteString(padded)
				continue
			}

			// 只给实际内容着色，补齐的空格保持无色，避免背景色拖尾。
			trimmed := strings.TrimRight(padded, " ")
			b.WriteString(paint(c.color, trimmed))
			b.WriteString(padded[len(trimmed):])
		}
		fmt.Fprintln(w, strings.TrimRight(b.String(), " "))
	}

	writeRow(headers)
	writeRow(make([]cell, cols))
	for _, row := range rows {
		writeRow(row)
	}
}

// humanizeSince 把使用记录渲染为 "2h ago" 形式的相对时间。
func humanizeSince(e history.Entry) string {
	if e.LastUsed.IsZero() {
		return "-"
	}

	d := time.Since(e.LastUsed)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// printEmptyHint 在配置中没有主机时给出上手引导。
func printEmptyHint() {
	fmt.Println("配置中还没有主机条目。")
	fmt.Println()
	hintf("添加一条：  yssh add web root@1.2.3.4 --port 2222 --env prod")
	hintf("查看帮助：  yssh --help")
}
