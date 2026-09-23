// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/Baobug/YunSSH/internal/history"
)

// cmdList 列出全部主机条目。
//
// 字段来自配置文本的轻量扫描，不调用 ssh -G，因此主机数量多时也是瞬时返回。
// 需要权威解析结果时用 `yssh show <别名>`。
func cmdList(args []string) int {
	plain := false
	for _, a := range args {
		switch {
		case a == "--plain":
			plain = true
		case strings.HasPrefix(a, "-"):
			errf("未知选项 %q。用法: yssh ls [--plain]", a)
			return exitUsage
		}
		// 裸位置参数沿用旧行为：忽略，不报错。
	}

	cfg, err := loadConfig()
	if err != nil {
		errf("读取配置失败: %v", err)
		return exitError
	}

	hosts := cfg.List()
	if len(hosts) == 0 {
		// --plain 是给补全脚本消费的，stdout 必须零输出，连上手指引也不能打。
		if !plain {
			printEmptyHint()
		}
		return exitOK
	}

	// --plain：每行一个别名，无表头、无提示、无颜色，供补全脚本稳定解析。
	// 刻意不读 history：补全用不上「最近使用」，省一次 ~/.yssh 的磁盘访问。
	if plain {
		for _, h := range hosts {
			fmt.Println(h.Alias)
		}
		return exitOK
	}

	store := history.Load()

	headers := []cell{
		{text: "ENV"},
		{text: "ALIAS"},
		{text: "TARGET"},
		{text: "TAGS"},
		{text: "LAST"},
	}

	rows := make([][]cell, 0, len(hosts))
	for _, h := range hosts {
		rows = append(rows, []cell{
			{text: h.Meta.Env, color: envColor(h.Meta.Env)},
			{text: h.Alias},
			{text: h.Target()},
			{text: strings.Join(h.Meta.Tags, ",")},
			{text: humanizeSince(store.Get(h.Alias))},
		})
	}

	renderTable(os.Stdout, headers, rows)
	fmt.Println()
	hintf("共 %d 台主机，`yssh <关键词>` 直接连接。", len(hosts))

	return exitOK
}
