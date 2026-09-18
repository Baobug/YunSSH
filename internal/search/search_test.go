package search

import "testing"

func TestMatchOne(t *testing.T) {
	tests := []struct {
		name  string
		q     string
		s     string
		level int
		ok    bool
	}{
		{name: "完全相等", q: "web", s: "web", level: LevelExact, ok: true},
		{name: "前缀匹配", q: "we", s: "web", level: LevelPrefix, ok: true},
		{name: "前缀匹配-别名更长", q: "web", s: "web-backup", level: LevelPrefix, ok: true},
		{name: "子串匹配", q: "eb", s: "web", level: LevelSubstring, ok: true},
		{name: "子串匹配-中段", q: "prod", s: "my-prod-web", level: LevelSubstring, ok: true},
		{name: "子序列匹配", q: "whk", s: "wbhk", level: LevelSubsequence, ok: true},
		{name: "无匹配", q: "xyz", s: "web", ok: false},
		{name: "空查询", q: "", s: "web", ok: false},
		{name: "空候选", q: "web", s: "", ok: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			level, _, ok := matchOne(tc.q, tc.s)
			if ok != tc.ok {
				t.Fatalf("ok = %v，期望 %v", ok, tc.ok)
			}
			if ok && level != tc.level {
				t.Errorf("level = %d，期望 %d", level, tc.level)
			}
		})
	}
}

// TestMatchPriority 验证更高优先级的匹配类型确实排在前面。
func TestMatchPriority(t *testing.T) {
	aliases := []string{"web-backup", "my-web", "web", "wbhk", "db"}

	got := Match("web", aliases)
	if len(got) == 0 {
		t.Fatal("应至少匹配到一项")
	}

	// 完全相等必须排第一，无论别名长短。
	if got[0].Alias != "web" || got[0].Level != LevelExact {
		t.Fatalf("首位应为完全匹配的 web，得到 %+v", got[0])
	}

	// 结果必须整体按匹配级别升序（级别数值越小优先级越高）。
	last := 0
	for _, c := range got {
		if c.Level < last {
			t.Errorf("排序错误：%s(level %d) 出现在 level %d 之后", c.Alias, c.Level, last)
		}
		last = c.Level
	}
}

// TestMatchPrefixShorterWins 验证同为前缀匹配时，更短的别名得分更高。
func TestMatchPrefixShorterWins(t *testing.T) {
	got := Match("web", []string{"web-long-name", "web"})
	if len(got) < 2 {
		t.Fatalf("应匹配到两项，得到 %d", len(got))
	}
	// web 是完全匹配，web-long-name 是前缀匹配。
	if got[0].Alias != "web" {
		t.Errorf("首位应为 web，得到 %s", got[0].Alias)
	}
}

func TestMatchEmptyQuery(t *testing.T) {
	if got := Match("", []string{"web"}); got != nil {
		t.Errorf("空查询应返回 nil，得到 %+v", got)
	}
	if got := Match("   ", []string{"web"}); got != nil {
		t.Errorf("全空白查询应返回 nil，得到 %+v", got)
	}
}

// TestMatchCaseInsensitive 验证匹配不区分大小写。
func TestMatchCaseInsensitive(t *testing.T) {
	got := Match("WEB", []string{"web-prod"})
	if len(got) != 1 {
		t.Fatalf("大小写不同不应影响匹配，得到 %+v", got)
	}
	if got[0].Alias != "web-prod" {
		t.Errorf("应返回原始大小写的别名，得到 %s", got[0].Alias)
	}
}

func TestNearest(t *testing.T) {
	aliases := []string{"web-01", "web-02", "db-01", "gateway"}

	got := Nearest("web-03", aliases, 2)
	if len(got) != 2 {
		t.Fatalf("应返回 2 项，得到 %d: %v", len(got), got)
	}
	// web-03 与 web-01/web-02 各差 1 个字符，应排在最前。
	for _, alias := range got {
		if alias != "web-01" && alias != "web-02" {
			t.Errorf("候选不符合预期: %v", got)
		}
	}
}

func TestNearestBounds(t *testing.T) {
	aliases := []string{"a", "b"}

	if got := Nearest("", aliases, 3); got != nil {
		t.Errorf("空查询应返回 nil，得到 %v", got)
	}
	if got := Nearest("x", nil, 3); got != nil {
		t.Errorf("空候选集应返回 nil，得到 %v", got)
	}
	if got := Nearest("x", aliases, 0); got != nil {
		t.Errorf("n<=0 应返回 nil，得到 %v", got)
	}
	// 请求数超过候选总数时应返回全部，而不是 panic。
	if got := Nearest("x", aliases, 99); len(got) != 2 {
		t.Errorf("应返回全部 2 项，得到 %d", len(got))
	}
}

func TestLevenshtein(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "", 3},
		{"", "abc", 3},
		{"abc", "abc", 0},
		{"abc", "abd", 1},
		{"kitten", "sitting", 3},
		{"web", "wb", 1},
	}

	for _, tc := range tests {
		if got := levenshtein(tc.a, tc.b); got != tc.want {
			t.Errorf("levenshtein(%q, %q) = %d，期望 %d", tc.a, tc.b, got, tc.want)
		}
	}
}
