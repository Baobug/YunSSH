// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// TestListPlainOutput 钉住 --plain 的输出契约。
//
// 补全脚本直接消费这份输出，因此两个不变量一旦退化就会静默破坏补全：
// 空配置必须是零输出（否则补全菜单里会冒出中文提示），有主机时每行恰为一个别名。
func TestListPlainOutput(t *testing.T) {
	t.Run("空配置零输出", func(t *testing.T) {
		if got := plainList(t, ""); got != "" {
			t.Errorf("空配置下 stdout 应为空，实际 %q", got)
		}
	})

	t.Run("每行一个别名", func(t *testing.T) {
		got := plainList(t, "Host web\n    HostName 1.2.3.4\n\nHost lab\n    HostName 10.0.0.1\n")
		if got != "web\nlab\n" {
			t.Errorf("stdout = %q，期望 %q", got, "web\nlab\n")
		}
	})

	t.Run("不含控制序列", func(t *testing.T) {
		if got := plainList(t, "Host web\n    HostName 1.2.3.4\n"); bytes.ContainsRune([]byte(got), 0x1b) {
			t.Errorf("stdout 含 ANSI 转义序列: %q", got)
		}
	})
}

// TestListRejectsUnknownOption 覆盖参数收紧：以前 `yssh ls --bogus` 被静默接受。
func TestListRejectsUnknownOption(t *testing.T) {
	withFakeHome(t, "")

	stderr := captureStderr(t, func() {
		if code := cmdList([]string{"--bogus"}); code != exitUsage {
			t.Errorf("cmdList(--bogus) = %d，期望 %d", code, exitUsage)
		}
	})
	if !bytes.Contains([]byte(stderr), []byte("--bogus")) {
		t.Errorf("stderr 未指出非法选项: %q", stderr)
	}
}

// plainList 在隔离的家目录下写入配置并返回 `yssh ls --plain` 的输出。
func plainList(t *testing.T, content string) string {
	t.Helper()
	withFakeHome(t, content)

	return captureStdout(t, func() {
		if code := cmdList([]string{"--plain"}); code != exitOK {
			t.Errorf("cmdList(--plain) = %d，期望 %d", code, exitOK)
		}
	})
}

// withFakeHome 把家目录指向临时目录，并按需写入 ~/.ssh/config。
//
// cmdList 经 os.UserHomeDir 定位配置，Windows 看 USERPROFILE、Unix 看 HOME，
// 两个都要设，否则测试会读到真实的 ~/.ssh/config。
func withFakeHome(t *testing.T, configContent string) string {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if configContent == "" {
		return home
	}
	dir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("创建 .ssh 目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(configContent), 0o600); err != nil {
		t.Fatalf("写入测试配置失败: %v", err)
	}
	return home
}

// captureStdout 临时接住 stdout 的内容并返回，用于断言机器可读输出。
//
// 与 captureStderr 同构：无论回调如何结束都会先恢复 os.Stdout，
// 否则测试框架自己的输出会被吞掉。
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("创建管道失败: %v", err)
	}

	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	func() {
		defer func() { os.Stdout = old }()
		fn()
	}()

	_ = w.Close()
	out := <-done
	_ = r.Close()
	return out
}
