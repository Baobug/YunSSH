// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

package session

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// stubSSHBin 造一个「存在但不是可执行程序」的空文件，并让 YSSH_SSH_BIN 指向它。
//
// SSHBin 只做 os.Stat，所以空文件就能通过定位检查；随后 exec 启动它必然失败，
// 于是能稳定地走进错误分支，而不依赖机器上是否真的装了 ssh。
func stubSSHBin(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "ssh-stub")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("写入桩文件失败: %v", err)
	}
	t.Setenv(sshBinEnv, path)
	return path
}

// TestSSHBinPrefersEnvOverride 覆盖环境变量覆盖路径这一约定：
// 托盘与 CLI 都依赖它，用户也要靠它指定非默认位置的 ssh。
func TestSSHBinPrefersEnvOverride(t *testing.T) {
	want := stubSSHBin(t)

	got, err := SSHBin()
	if err != nil {
		t.Fatalf("SSHBin() 报错: %v", err)
	}
	if got != want {
		t.Errorf("SSHBin() = %q，期望 %q", got, want)
	}
}

// TestSSHBinRejectsBadOverride 覆盖覆盖值不可用时的报错。
//
// 静默退回 PATH 上的 ssh 是错误设计：那会让用户以为覆盖生效了，
// 实际连的却是另一个程序。
func TestSSHBinRejectsBadOverride(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	t.Setenv(sshBinEnv, missing)

	_, err := SSHBin()
	if err == nil {
		t.Fatal("SSHBin() 应报错，却成功了")
	}
	if !strings.Contains(err.Error(), sshBinEnv) {
		t.Errorf("错误信息应点明是哪个环境变量: %v", err)
	}
}

// realExe 返回一个当前平台确定可执行的程序路径。
//
// 超时路径需要它：exec 在检查 context 是否过期之前会先做可执行性校验，
// Windows 上这一步先于超时判断，指向空文件会得到「不是可执行文件」
// 而不是超时错误。程序本身永远不会被真正启动（context 早已过期）。
func realExe(t *testing.T) string {
	t.Helper()

	candidates := []string{os.Getenv("COMSPEC"), "/bin/sh", "/usr/bin/true"}
	if p, err := exec.LookPath("sh"); err == nil {
		candidates = append(candidates, p)
	}
	for _, p := range candidates {
		if p == "" {
			continue
		}
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}
	t.Skip("找不到可用于超时测试的可执行文件")
	return ""
}

// TestResolveTimeoutReportsFriendlyError 覆盖 M4 修复：ssh -G 卡住时必须
// 主动中止并给出可诊断的提示，而不是让用户对着黑屏等。
//
// 做法是把超时设成 1ns —— context 在进程启动之前就已过期，
// 于是无需真的拉起一个慢进程也能稳定命中超时分支。
func TestResolveTimeoutReportsFriendlyError(t *testing.T) {
	t.Setenv(sshBinEnv, realExe(t))

	_, err := resolveWithTimeout("prod-web", time.Nanosecond)
	if err == nil {
		t.Fatal("resolveWithTimeout 应返回超时错误，却成功了")
	}
	msg := err.Error()
	if !strings.Contains(msg, "超过") || !strings.Contains(msg, "未返回") {
		t.Errorf("超时错误缺少可读提示: %v", err)
	}
	if !strings.Contains(msg, "prod-web") {
		t.Errorf("超时错误应带上别名，便于定位: %v", err)
	}
}

// TestResolveNonTimeoutErrorNotMislabelled 覆盖反面：普通的启动失败
// 不能被包装成「超时」，否则用户会去查一个根本不存在的 Match exec。
func TestResolveNonTimeoutErrorNotMislabelled(t *testing.T) {
	stubSSHBin(t)

	// 桩文件启动必然失败，但此时 context 远未到期。
	_, err := resolveWithTimeout("prod-web", 30*time.Second)
	if err == nil {
		t.Fatal("resolveWithTimeout 应返回错误，却成功了")
	}
	if strings.Contains(err.Error(), "超过") {
		t.Errorf("非超时错误被误标为超时: %v", err)
	}
}

// TestResolveTimeoutIsBounded 锁定默认超时值。
//
// 这不是为了复述常量，而是防止有人为了「让它更稳」把它调成 30s 甚至 0
// ——0 会被 context 当成「立即过期」，Resolve 将彻底失效。
func TestResolveTimeoutIsBounded(t *testing.T) {
	if resolveTimeout <= 0 {
		t.Fatalf("resolveTimeout = %v，必须为正数（0 会被当作立即过期）", resolveTimeout)
	}
	if resolveTimeout > 30*time.Second {
		t.Errorf("resolveTimeout = %v，对一次配置解析而言过长", resolveTimeout)
	}
}
