// SPDX-FileCopyrightText: 2026 Zhou Tianbo (Baobug)
// SPDX-License-Identifier: Apache-2.0

// Package search 实现主机别名的模糊匹配。
//
// 匹配分四级，优先级由高到低：完全相等、前缀、子串、子序列（fzf 风格）。
// 同级结果由调用方结合使用历史排序。
package search

import (
	"sort"
	"strings"
)

// 匹配级别。数值越小优先级越高。
const (
	LevelExact       = 1
	LevelPrefix      = 2
	LevelSubstring   = 3
	LevelSubsequence = 4
)

// Candidate 是一个匹配结果。
type Candidate struct {
	Alias string
	Level int
	Score int // 同级内比较，越大越优
}

// Match 在 aliases 中查找与 query 匹配的项，按优先级降序返回。
//
// 排序规则为「匹配级别 → 匹配得分 → 别名」。调用方若要结合使用历史
// 调整顺序，应在这份结果之上做稳定排序。
//
// 空查询返回 nil——调用方应自行决定空查询的语义（通常是列出全部）。
func Match(query string, aliases []string) []Candidate {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}

	out := make([]Candidate, 0, len(aliases))
	for _, alias := range aliases {
		if level, score, ok := matchOne(q, strings.ToLower(alias)); ok {
			out = append(out, Candidate{Alias: alias, Level: level, Score: score})
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Level != out[j].Level {
			return out[i].Level < out[j].Level // 级别数值越小优先级越高
		}
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Alias < out[j].Alias
	})

	return out
}

// matchOne 对单个候选做四级匹配。
func matchOne(q, s string) (level, score int, ok bool) {
	if q == "" || s == "" {
		return 0, 0, false
	}

	if s == q {
		return LevelExact, 1000, true
	}
	if strings.HasPrefix(s, q) {
		// 前缀匹配：更短的别名优先，web 比 web-backup-01 更可能是目标。
		return LevelPrefix, 900 - (len(s) - len(q)), true
	}
	if idx := strings.Index(s, q); idx >= 0 {
		// 子串匹配：出现位置越靠前越优。
		return LevelSubstring, 800 - idx, true
	}
	if span, found := subsequence(q, s); found {
		// 子序列匹配：跨度越小越紧凑。
		return LevelSubsequence, 700 - span, true
	}
	return 0, 0, false
}

// subsequence 判断 q 是否为 s 的子序列，并返回其最小跨度。
// 例如 q="wbhk"、s="web-hk" 匹配成功，跨度为整个串的长度。
func subsequence(q, s string) (span int, ok bool) {
	qi := 0
	first, last := -1, -1

	for si := 0; si < len(s) && qi < len(q); si++ {
		if s[si] != q[qi] {
			continue
		}
		if first < 0 {
			first = si
		}
		last = si
		qi++
	}

	if qi < len(q) {
		return 0, false
	}
	return last - first + 1, true
}

// Nearest 返回与 query 编辑距离最近的 n 个别名，用于「没有匹配」时的候选提示。
func Nearest(query string, aliases []string, n int) []string {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" || len(aliases) == 0 || n <= 0 {
		return nil
	}

	type scored struct {
		alias string
		dist  int
	}
	items := make([]scored, 0, len(aliases))
	for _, a := range aliases {
		items = append(items, scored{a, levenshtein(q, strings.ToLower(a))})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].dist != items[j].dist {
			return items[i].dist < items[j].dist
		}
		return items[i].alias < items[j].alias
	})

	if len(items) > n {
		items = items[:n]
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.alias)
	}
	return out
}

// levenshtein 计算两个字符串的编辑距离。
// 按字节比较，对 ASCII 别名足够准确。
func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(prev[j]+1, min(curr[j-1]+1, prev[j-1]+cost))
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}
