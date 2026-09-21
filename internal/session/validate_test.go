// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

package session

import (
	"strings"
	"testing"

	"github.com/Baobug/YunSSH/internal/meta"
)

// TestValidAlias 验证别名字符集与首字符约束。
func TestValidAlias(t *testing.T) {
	valid := []string{"web", "prod-web", "web_01", "a.b.c", "web-01", "a1"}
	for _, a := range valid {
		if err := ValidAlias(a); err != nil {
			t.Errorf("ValidAlias(%q) 应通过，得到 %v", a, err)
		}
	}

	invalid := []string{"", "-v", "-L8080:localhost:80", "-oProxyCommand=evil", "has space", "a=b", "a*b", "a?b", "!a", ".hidden", "@host", "foo@bar", "a\nb"}
	for _, a := range invalid {
		if err := ValidAlias(a); err == nil {
			t.Errorf("ValidAlias(%q) 应被拒绝", a)
		}
	}
}

// TestAddRejectsNewlineInjection 验证换行无法通过任何字段注入额外指令。
// 这是对 DESIGN 铁律二「绝不默认 StrictHostKeyChecking=no」的直接保护。
func TestAddRejectsNewlineInjection(t *testing.T) {
	cfg, path := newConfig(t, "")

	cases := []Host{
		{Alias: "web", HostName: "1.2.3.4\n    StrictHostKeyChecking no", User: "root"},
		{Alias: "web", HostName: "1.2.3.4", User: "root\n    ForwardAgent yes"},
		{Alias: "web", HostName: "1.2.3.4", IdentityFile: "~/.ssh/id\n    ProxyCommand evil"},
		{Alias: "web", HostName: "1.2.3.4", Meta: meta.Meta{Env: "prod", Note: "ok\n    StrictHostKeyChecking no"}},
		{Alias: "web", HostName: "1.2.3.4", Meta: meta.Meta{Env: "prod\n    User evil"}},
		{Alias: "web", HostName: "1.2.3.4", Meta: meta.Meta{Tags: []string{"a", "b\n    User evil"}}},
	}

	for i, h := range cases {
		if err := cfg.Add(h); err == nil {
			t.Errorf("用例 %d：含换行的字段应被拒绝，却写入成功", i)
		}
	}

	// 被拒绝的写入不能留下任何内容。
	if got := readConfig(t, path); strings.TrimSpace(got) != "" {
		t.Errorf("失败的写入不应改动文件:\n%q", got)
	}
}

// TestAddRejectsLeadingDashAlias 验证以 - 开头的别名被拒绝。
func TestAddRejectsLeadingDashAlias(t *testing.T) {
	cfg, _ := newConfig(t, "")

	for _, alias := range []string{"-v", "-L8080:localhost:80", "-oProxyCommand=evil"} {
		if err := cfg.Add(Host{Alias: alias, HostName: "1.2.3.4"}); err == nil {
			t.Errorf("别名 %q 应被拒绝", alias)
		}
	}
}

// TestAliasesSkipLeadingDash 验证手工配置中以 - 开头的 Host 不会被当作可连接目标。
func TestAliasesSkipLeadingDash(t *testing.T) {
	const hostile = `Host -oProxyCommand=evil
    HostName 1.2.3.4

Host good
    HostName 5.6.7.8
`
	cfg, _ := newConfig(t, hostile)

	got := cfg.Aliases()
	if len(got) != 1 || got[0] != "good" {
		t.Fatalf("应只保留合法别名 [good]，得到 %v", got)
	}
}

// TestParseHostLineSkipsLeadingDash 直接验证解析层的行为。
func TestParseHostLineSkipsLeadingDash(t *testing.T) {
	aliases, ok := parseHostLine("Host -oProxyCommand=evil good")
	if !ok {
		t.Fatal("应解析成功（good 是合法别名）")
	}
	if len(aliases) != 1 || aliases[0] != "good" {
		t.Fatalf("应只保留 good，得到 %v", aliases)
	}

	if _, ok := parseHostLine("Host -oProxyCommand=evil"); ok {
		t.Error("整行都是非法别名时应返回 false")
	}
}
