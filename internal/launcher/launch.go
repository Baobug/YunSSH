// Package launcher 把一次连接请求交给终端程序执行。
//
// 与 internal/backend 的分工：
//   - backend 负责「拼出正确的 ssh 命令行」
//   - launcher 负责「这条命令行交给谁跑、在哪个窗口里跑」
package launcher

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Terminal 选择承载连接的终端。
type Terminal string

const (
	// Wt 在 Windows Terminal 中以新标签页打开，复用当前窗口。
	Wt Terminal = "wt"
	// Default 交给系统默认方式，新开一个控制台窗口。
	Default Terminal = "default"
)

// ErrNoTerminal 表示找不到可用的终端程序。
var ErrNoTerminal = errors.New("未找到可用的终端程序")

// Options 描述一次启动请求。
type Options struct {
	Alias     string   // ~/.ssh/config 中的别名
	ExtraArgs []string // 透传给 ssh 的参数
	Terminal  Terminal
	NewWindow bool // 强制开新窗口，而不是复用现有 Windows Terminal 窗口
}

// Start 启动连接。立即返回，不等待终端退出。
//
// 连接信息本来就写在 config 里，所以这里只需要把别名交给 ssh，
// 由它自己去解析 HostName / Port / ProxyJump。
func Start(opts Options) error {
	if opts.Terminal == Wt {
		if err := startWindowsTerminal(opts); err == nil {
			return nil
		}
	}
	return startDefault(opts)
}

// startWindowsTerminal 用 wt.exe 打开一个新标签页。
func startWindowsTerminal(opts Options) error {
	exe, err := WtPath()
	if err != nil {
		return err
	}

	// -w 0：复用当前 Windows Terminal 窗口；-w -1：强制新窗口
	window := "0"
	if opts.NewWindow {
		window = "-1"
	}

	// `nt` 表示新标签页（new-tab）
	args := []string{"-w", window, "nt", "ssh", opts.Alias}
	args = append(args, opts.ExtraArgs...)

	return exec.Command(exe, args...).Start()
}

// startDefault 用 `cmd /c start` 弹出一个独立控制台窗口。
func startDefault(opts Options) error {
	sshBin, err := exec.LookPath("ssh")
	if err != nil {
		return fmt.Errorf("未找到 ssh 可执行文件: %w", err)
	}

	// start 的第一个参数是窗口标题，留空即可
	args := []string{"/c", "start", "", sshBin, opts.Alias}
	args = append(args, opts.ExtraArgs...)

	return exec.Command("cmd", args...).Start()
}

// StartCommand 在 Windows Terminal 的新标签页里运行一条命令。
//
// 用于「搜索主机」这类需要把控制权交回 CLI 的场景：
// 托盘只负责开一个标签页，交互仍由 yssh 自己完成。
func StartCommand(exe string, args ...string) error {
	wt, err := WtPath()
	if err != nil {
		return err
	}

	full := append([]string{"-w", "0", "nt", exe}, args...)
	return exec.Command(wt, full...).Start()
}

// WtPath 定位 Windows Terminal 的可执行文件。
//
// wt.exe 通常不在 PATH 的常规目录里，而是 %LOCALAPPDATA%\Microsoft\WindowsApps
// 下的应用执行别名（一个重解析点），因此需要显式兜底。
func WtPath() (string, error) {
	if p, err := exec.LookPath("wt.exe"); err == nil {
		return p, nil
	}

	local := os.Getenv("LOCALAPPDATA")
	if local != "" {
		p := filepath.Join(local, "Microsoft", "WindowsApps", "wt.exe")
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	return "", ErrNoTerminal
}
