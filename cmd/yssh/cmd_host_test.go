// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

// TestParseTarget 覆盖目标串解析的常规形态、IPv6 字面量与端口边界。
//
// 这份表驱动测试是补课：IPv6 的静默数据损坏正是一直没有它才潜伏至今。
func TestParseTarget(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		user    string
		host    string
		port    int
		wantErr bool
	}{
		{name: "纯主机", in: "1.2.3.4", host: "1.2.3.4", port: 0},
		{name: "用户@主机", in: "root@1.2.3.4", user: "root", host: "1.2.3.4", port: 0},
		{name: "用户@主机:端口", in: "root@1.2.3.4:2222", user: "root", host: "1.2.3.4", port: 2222},
		{name: "主机:端口", in: "1.2.3.4:2222", host: "1.2.3.4", port: 2222},

		// IPv6：带方括号的正确写法
		{name: "裸 IPv6 地址", in: "[2001:db8::1]", host: "2001:db8::1", port: 0},
		{name: "IPv6 带端口", in: "[2001:db8::1]:22", host: "2001:db8::1", port: 22},
		{name: "IPv6 带用户与端口", in: "root@[::1]:2222", user: "root", host: "::1", port: 2222},

		// 错误输入：必须明确报错，不能静默切错
		{name: "裸 IPv6 无括号", in: "2001:db8::1", wantErr: true},
		{name: "端口越界", in: "root@1.2.3.4:99999", wantErr: true},
		{name: "端口为零", in: "root@1.2.3.4:0", wantErr: true},
		{name: "端口非数字", in: "root@1.2.3.4:abc", wantErr: true},
		{name: "空端口", in: "root@1.2.3.4:", wantErr: true},
		{name: "缺右括号", in: "[2001:db8::1", wantErr: true},
		{name: "方括号后有非法后缀", in: "[::1]extra", wantErr: true},
		{name: "空目标", in: "", wantErr: true},
		{name: "纯空白", in: "   ", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			user, host, port, err := parseTarget(tc.in)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseTarget(%q) 应报错，却返回 (%q, %q, %d)", tc.in, user, host, port)
				}
				return
			}

			if err != nil {
				t.Fatalf("parseTarget(%q) 报错: %v", tc.in, err)
			}
			if user != tc.user || host != tc.host || port != tc.port {
				t.Errorf("parseTarget(%q) = (%q, %q, %d)，期望 (%q, %q, %d)",
					tc.in, user, host, port, tc.user, tc.host, tc.port)
			}
		})
	}
}
