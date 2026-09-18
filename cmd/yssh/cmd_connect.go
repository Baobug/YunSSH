package main

import (
	"os"
	"sort"

	"github.com/Baobug/YunSSH/internal/backend"
	"github.com/Baobug/YunSSH/internal/history"
	"github.com/Baobug/YunSSH/internal/search"
	"github.com/Baobug/YunSSH/internal/session"
	"github.com/Baobug/YunSSH/internal/term"
)

// cmdConnect 处理连接请求：模糊匹配别名，选定后用底层客户端接管终端。
func cmdConnect(keyword string, extra []string) int {
	cfg, err := loadConfig()
	if err != nil {
		errf("读取配置失败: %v", err)
		return exitError
	}

	aliases := cfg.Aliases()
	if len(aliases) == 0 {
		printEmptyHint()
		return exitNotFound
	}

	candidates := search.Match(keyword, aliases)
	if len(candidates) == 0 {
		errf("没有匹配 %q 的主机", keyword)
		if near := search.Nearest(keyword, aliases, 3); len(near) > 0 {
			hintf("你是不是想找：")
			for _, n := range near {
				hintf("  %s", n)
			}
		}
		return exitNotFound
	}

	store := history.Load()
	sortCandidates(candidates, store)

	best := candidates[0]
	if len(candidates) > 1 {
		next := candidates[1]
		// 级别与得分完全相同，说明无法判定用户意图，交由用户补充关键词。
		if next.Level == best.Level && next.Score == best.Score {
			errf("%q 匹配到多个主机，请补充关键词", keyword)
			printCandidates(candidates, store)
			return exitNotFound
		}
	}

	if best.Alias != keyword {
		hintf("→ %s", best.Alias)
	}
	return connectAlias(cfg, best.Alias, extra, store)
}

// sortCandidates 依次按「匹配级别 → 匹配得分 → 最近使用 → 使用次数 → 名称」排序。
//
// 前两级来自 search 的匹配质量，后三级利用使用历史打破平局：
// 常用的、刚用过的排前面，符合直觉。
func sortCandidates(cands []search.Candidate, store *history.Store) {
	sort.SliceStable(cands, func(i, j int) bool {
		a, b := cands[i], cands[j]

		if a.Level != b.Level {
			return a.Level < b.Level // 级别数值越小优先级越高
		}
		if a.Score != b.Score {
			return a.Score > b.Score
		}

		ea, eb := store.Get(a.Alias), store.Get(b.Alias)
		if !ea.LastUsed.Equal(eb.LastUsed) {
			return ea.LastUsed.After(eb.LastUsed)
		}
		if ea.Count != eb.Count {
			return ea.Count > eb.Count
		}
		return a.Alias < b.Alias
	})
}

// printCandidates 在匹配有歧义时列出全部候选。
func printCandidates(cands []search.Candidate, store *history.Store) {
	headers := []cell{{text: "ALIAS"}, {text: "LAST"}}

	rows := make([][]cell, 0, len(cands))
	for _, c := range cands {
		rows = append(rows, []cell{
			{text: c.Alias},
			{text: humanizeSince(store.Get(c.Alias))},
		})
	}
	renderTable(os.Stdout, headers, rows)
}

// connectAlias 用选定后端接管当前终端，返回子进程退出码。
func connectAlias(cfg *session.Config, alias string, extra []string, store *history.Store) int {
	host, found := cfg.Find(alias)
	if !found {
		errf("配置中找不到别名 %q", alias)
		return exitNotFound
	}

	req := backend.Request{
		Alias:     host.Alias,
		HostName:  host.HostName,
		User:      host.User,
		Port:      host.Port,
		Env:       host.Meta.Env,
		ExtraArgs: extra,
	}

	be, err := backend.Select(req)
	if err != nil {
		errf("%v", err)
		return exitBackend
	}

	exe, argv, err := be.Build(req)
	if err != nil {
		errf("%v", err)
		return exitBackend
	}

	// 连接前改窗口标题，断开后恢复——这是「防手滑」的第一道防线。
	term.SetTitle(host.Alias, host.Meta.Env)
	code, err := backend.Run(exe, argv)
	term.ResetTitle()

	if err != nil {
		errf("连接失败: %v", err)
		return exitError
	}

	store.Touch(alias)
	store.Save()

	return code
}
