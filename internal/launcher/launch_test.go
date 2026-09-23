// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

package launcher

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// isolate 清空 PATH 与新终端相关的环境变量，构造一个「什么都找不到」的现场。
func isolate(t *testing.T) {
	t.Helper()

	t.Setenv("PATH", t.TempDir())
	t.Setenv("LOCALAPPDATA", t.TempDir())
}

// stubSSHBin 造一个存在的空文件并让 YSSH_SSH_BIN 指向它。
// 空文件足以通过 SSHBin 的存在性检查，且不会真的被执行。
func stubSSHBin(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "ssh-stub")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("写入桩文件失败: %v", err)
	}
	t.Setenv("YSSH_SSH_BIN", path)
	return path
}

// TestWtArgsUsesResolvedSSHBin 锁定 M1 修复：托盘走 wt 分支时，
// 传给新标签页的必须是 SSHBin() 解析出的绝对路径。
//
// 此前这里是写死的字符串 "ssh"，后果是 YSSH_SSH_BIN 对托盘无效，
// 且 ssh 不在 PATH 时「CLI 能连、托盘点了没反应」。
func TestWtArgsUsesResolvedSSHBin(t *testing.T) {
	sshBin := stubSSHBin(t)

	args, err := wtArgs(Options{Alias: "prod-web", ExtraArgs: []string{"-v"}})
	if err != nil {
		t.Fatalf("wtArgs 报错: %v", err)
	}

	want := []string{"-w", "0", "nt", sshBin, "prod-web", "-v"}
	if !equal(args, want) {
		t.Errorf("wtArgs = %v，期望 %v", args, want)
	}
}

// TestCmdStartArgsUsesResolvedSSHBin 同上，覆盖 default 分支。
func TestCmdStartArgsUsesResolvedSSHBin(t *testing.T) {
	sshBin := stubSSHBin(t)

	args, err := cmdStartArgs(Options{Alias: "prod-web", ExtraArgs: []string{"-p", "2222"}})
	if err != nil {
		t.Fatalf("cmdStartArgs 报错: %v", err)
	}

	want := []string{"/c", "start", "", sshBin, "prod-web", "-p", "2222"}
	if !equal(args, want) {
		t.Errorf("cmdStartArgs = %v，期望 %v", args, want)
	}
}

// TestWtArgsNewWindow 覆盖 -w 窗口参数的映射。
func TestWtArgsNewWindow(t *testing.T) {
	stubSSHBin(t)

	tests := []struct {
		name string
		opt  bool
		want string
	}{
		{name: "复用当前窗口", opt: false, want: "0"},
		{name: "强制新窗口", opt: true, want: "-1"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			args, err := wtArgs(Options{Alias: "a", NewWindow: tc.opt})
			if err != nil {
				t.Fatalf("wtArgs 报错: %v", err)
			}
			if len(args) < 2 || args[0] != "-w" || args[1] != tc.want {
				t.Errorf("窗口参数 = %v，期望 -w %s", args, tc.want)
			}
		})
	}
}

// TestArgsSurfaceSSHBinError 覆盖 ssh 找不到时的传播。
//
// 两个分支都必须把错误抛出去，而不是带着一个空路径去 exec
// —— 后者只会换来一句莫名其妙的「系统找不到指定的文件」。
func TestArgsSurfaceSSHBinError(t *testing.T) {
	t.Setenv("YSSH_SSH_BIN", filepath.Join(t.TempDir(), "missing"))
	isolate(t)

	if _, err := wtArgs(Options{Alias: "a"}); err == nil {
		t.Error("wtArgs 应把 SSHBin 的错误抛出")
	}
	if _, err := cmdStartArgs(Options{Alias: "a"}); err == nil {
		t.Error("cmdStartArgs 应把 SSHBin 的错误抛出")
	}
}

// TestArgsDoNotMutateExtra 确认透传参数是复制而非共享底层数组，
// 否则调用方复用同一个切片时会看到被追加过的内容。
func TestArgsDoNotMutateExtra(t *testing.T) {
	stubSSHBin(t)

	extra := []string{"-v"}
	if _, err := wtArgs(Options{Alias: "a", ExtraArgs: extra}); err != nil {
		t.Fatalf("wtArgs 报错: %v", err)
	}
	if len(extra) != 1 || extra[0] != "-v" {
		t.Errorf("透传切片被改动: %v", extra)
	}
}

// TestWtPathFallsBackToLocalAppData 覆盖 wt.exe 不在 PATH 上的兜底：
// 它通常装在 %LOCALAPPDATA%\Microsoft\WindowsApps，而这个目录
// 未必出现在 PATH 里。
func TestWtPathFallsBackToLocalAppData(t *testing.T) {
	isolate(t)
	if _, err := exec.LookPath("wt.exe"); err == nil {
		t.Skip("PATH 上仍能解析到 wt.exe，无法隔离兜底分支")
	}

	want := filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "WindowsApps", "wt.exe")
	if err := os.MkdirAll(filepath.Dir(want), 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	if err := os.WriteFile(want, []byte("stub"), 0o755); err != nil {
		t.Fatalf("写入桩失败: %v", err)
	}

	got, err := WtPath()
	if err != nil {
		t.Fatalf("WtPath 报错: %v", err)
	}
	if got != want {
		t.Errorf("WtPath = %q，期望 %q", got, want)
	}
}

// TestWtPathMissing 覆盖彻底找不到终端：必须返回哨兵错误，
// 让调用方有机会回退到 startDefault。
func TestWtPathMissing(t *testing.T) {
	isolate(t)
	if _, err := exec.LookPath("wt.exe"); err == nil {
		t.Skip("PATH 上仍能解析到 wt.exe，无法构造缺失场景")
	}

	_, err := WtPath()
	if !errors.Is(err, ErrNoTerminal) {
		t.Errorf("WtPath 应返回 ErrNoTerminal，实际 %v", err)
	}
}

// TestWtPathEmptyLocalAppData 覆盖 LOCALAPPDATA 未设置的退化情形
// （比如非交互环境下启动托盘）。
func TestWtPathEmptyLocalAppData(t *testing.T) {
	isolate(t)
	t.Setenv("LOCALAPPDATA", "")

	if _, err := exec.LookPath("wt.exe"); err == nil {
		t.Skip("PATH 上仍能解析到 wt.exe，无法构造缺失场景")
	}

	if _, err := WtPath(); !errors.Is(err, ErrNoTerminal) {
		t.Errorf("LOCALAPPDATA 为空时应返回 ErrNoTerminal，实际 %v", err)
	}
}

// TestStartFallsBackToDefault 覆盖 Start 的分支选择：
// Wt 不可用时必须回退，而不是直接失败。
//
// 这里只验证「不会因为找不到 wt 就把错误抛给用户」——
// 真正的窗口启动在 CI 上无从断言。
func TestStartFallsBackToDefault(t *testing.T) {
	isolate(t)
	t.Setenv("YSSH_SSH_BIN", filepath.Join(t.TempDir(), "missing"))

	// ssh 也找不到：最终错误应来自 ssh 定位，而不是 ErrNoTerminal，
	// 说明确实走到了 default 分支。
	err := Start(Options{Alias: "a", Terminal: Wt})
	if err == nil {
		t.Skip("环境中存在 cmd，实际启动了窗口，跳过")
	}
	if !strings.Contains(err.Error(), "YSSH_SSH_BIN") && !strings.Contains(err.Error(), "ssh") {
		t.Errorf("回退到 default 后仍失败，但错误与 ssh 无关: %v", err)
	}
}

// equal 逐项比较字符串切片。
func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
