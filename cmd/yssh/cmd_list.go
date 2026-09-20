// SPDX-FileCopyrightText: 2026 Zhou Tianbo (Baobug)
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
	cfg, err := loadConfig()
	if err != nil {
		errf("读取配置失败: %v", err)
		return exitError
	}

	hosts := cfg.List()
	if len(hosts) == 0 {
		printEmptyHint()
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
