// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

// TestParsePort 覆盖端口解析这个唯一入口的边界。
//
// M6 的修复就是让 `--port` 收敛到这里，因此这里必须把范围判断钉死：
// 一旦放宽，两条入口又会漂移。
func TestParsePort(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    int
		wantErr bool
	}{
		{name: "下限", in: "1", want: 1},
		{name: "常用 ssh 端口", in: "22", want: 22},
		{name: "高位端口", in: "2222", want: 2222},
		{name: "上限", in: "65535", want: 65535},

		{name: "零", in: "0", wantErr: true},
		{name: "负数", in: "-1", wantErr: true},
		{name: "超出上限", in: "65536", wantErr: true},
		{name: "非数字", in: "abc", wantErr: true},
		{name: "十六进制", in: "0x16", wantErr: true},
		{name: "空串", in: "", wantErr: true},
		{name: "带空格", in: "22 ", wantErr: true},
		{name: "浮点", in: "22.0", wantErr: true},
		{name: "超大数值", in: "99999999999999999999", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parsePort(tc.in)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("parsePort(%q) = %d，期望报错", tc.in, got)
				}
				if !strings.Contains(err.Error(), "端口无效") {
					t.Errorf("错误信息应说明端口无效: %v", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("parsePort(%q) 报错: %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("parsePort(%q) = %d，期望 %d", tc.in, got, tc.want)
			}
		})
	}
}

// TestCmdAddRejectsInvalidPort 验证 `--port` 这条入口也用上了同一套校验。
//
// 这些输入都在解析阶段就被拒绝，不会走到写配置文件那一步，
// 因此测试不会碰到真实的 ~/.ssh/config。
func TestCmdAddRejectsInvalidPort(t *testing.T) {
	bad := []string{"0", "65536", "-1", "abc", "", "22 ", "99999999999999999999"}

	for _, in := range bad {
		t.Run("port="+in, func(t *testing.T) {
			stderr := captureStderr(t, func() {
				if got := cmdAdd([]string{"myalias", "1.2.3.4", "--port", in}); got != exitUsage {
					t.Errorf("cmdAdd(--port %q) = %d，期望 %d", in, got, exitUsage)
				}
			})
			if !strings.Contains(stderr, "端口无效") {
				t.Errorf("stderr 未给出端口无效提示: %q", stderr)
			}
		})
	}
}

// TestCmdAddPortMissingValue 覆盖 `--port` 后面没有取值。
func TestCmdAddPortMissingValue(t *testing.T) {
	stderr := captureStderr(t, func() {
		if got := cmdAdd([]string{"myalias", "1.2.3.4", "--port"}); got != exitUsage {
			t.Errorf("cmdAdd(--port) = %d，期望 %d", got, exitUsage)
		}
	})
	if !strings.Contains(stderr, "--port") {
		t.Errorf("stderr 未指出缺少取值: %q", stderr)
	}
}

// captureStderr 临时接住 stderr 的内容并返回，用于断言报错文案。
//
// 无论回调如何结束都会先恢复 os.Stderr —— 否则测试框架自己的输出会被吞掉。
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("创建管道失败: %v", err)
	}

	os.Stderr = w
	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	func() {
		defer func() { os.Stderr = old }()
		fn()
	}()

	_ = w.Close()
	out := <-done
	_ = r.Close()
	return out
}
