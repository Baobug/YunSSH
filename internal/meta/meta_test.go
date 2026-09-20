// SPDX-FileCopyrightText: 2026 Zhou Tianbo (Baobug)
// SPDX-License-Identifier: Apache-2.0

package meta

import (
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		want   Meta
		isMeta bool
	}{
		{
			name:   "完整字段",
			line:   "# yssh: env=prod tags=web,hk added=2026-09-18 note=主站反代",
			want:   Meta{Env: "prod", Tags: []string{"web", "hk"}, Added: "2026-09-18", Note: "主站反代"},
			isMeta: true,
		},
		{
			name:   "note 含空格时取值到行尾",
			line:   "# yssh: env=lab note=香港跳板机 端口 2222",
			want:   Meta{Env: "lab", Note: "香港跳板机 端口 2222"},
			isMeta: true,
		},
		{
			name:   "仅 added",
			line:   "# yssh: added=2026-09-18",
			want:   Meta{Added: "2026-09-18"},
			isMeta: true,
		},
		{
			name:   "空元数据",
			line:   "# yssh:",
			want:   Meta{},
			isMeta: true,
		},
		{
			name:   "字段顺序无关",
			line:   "# yssh: added=2026-01-01 env=test",
			want:   Meta{Added: "2026-01-01", Env: "test"},
			isMeta: true,
		},
		{
			name:   "允许前导空白",
			line:   "   # yssh: env=dev",
			want:   Meta{Env: "dev"},
			isMeta: true,
		},
		{
			name:   "标签含空白时被剔除",
			line:   "# yssh: tags=a, b ,,c",
			want:   Meta{Tags: []string{"a", "b", "c"}},
			isMeta: true,
		},
		{
			name:   "未知字段被忽略",
			line:   "# yssh: env=dev color=blue",
			want:   Meta{Env: "dev"},
			isMeta: true,
		},
		{
			name:   "普通注释",
			line:   "# 这是我自己的注释",
			isMeta: false,
		},
		{
			name:   "Host 行",
			line:   "Host web",
			isMeta: false,
		},
		{
			name:   "前缀相近但不是元数据",
			line:   "# yssh-other: env=dev",
			isMeta: false,
		},
		{
			name:   "空行",
			line:   "",
			isMeta: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := Parse(tc.line)
			if ok != tc.isMeta {
				t.Fatalf("isMeta = %v，期望 %v", ok, tc.isMeta)
			}
			if !tc.isMeta {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Parse() = %+v，期望 %+v", got, tc.want)
			}
		})
	}
}

// TestRenderParseRoundTrip 验证 Render 与 Parse 互为逆运算。
// 这是元数据能安全往返写入 config 的前提。
func TestRenderParseRoundTrip(t *testing.T) {
	cases := []Meta{
		{Env: "prod", Tags: []string{"web", "hk"}, Added: "2026-09-18", Note: "主站反代 位于香港"},
		{Env: "test"},
		{Added: "2026-01-01"},
		{Tags: []string{"a", "b"}},
		{Note: "只有备注"},
	}

	for _, want := range cases {
		line := want.Render()
		got, ok := Parse(line)
		if !ok {
			t.Fatalf("Render 的结果无法被 Parse 识别: %q", line)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("往返后 = %+v，期望 %+v（原始行 %q）", got, want, line)
		}
	}
}

func TestRenderEmpty(t *testing.T) {
	if got := (Meta{}).Render(); got != "" {
		t.Errorf("空元数据应渲染为空串，得到 %q", got)
	}
}

func TestIsZero(t *testing.T) {
	if !(Meta{}).IsZero() {
		t.Error("零值应被判定为空")
	}
	if (Meta{Env: "dev"}).IsZero() {
		t.Error("含字段的元数据不应被判定为空")
	}
}
