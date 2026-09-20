// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

//go:build windows

// Command ysshtray 是 YunSSH 的托盘常驻程序。
//
// 启动方式：
//   - 无参数：双击启动。未安装时先询问是否安装，已安装则直接进托盘。
//   - install / uninstall：供安装流程与「应用和功能」的卸载项调用。
//   - autostart on|off：切换开机自启。
//
// 它编译为 GUI 子系统（无控制台），因此所有反馈都走原生对话框。
// 需要文字输出的场景请用 yssh 命令。
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"github.com/Baobug/YunSSH"
	"github.com/Baobug/YunSSH/internal/dialog"
	"github.com/Baobug/YunSSH/internal/install"
	"github.com/Baobug/YunSSH/internal/launcher"
	"github.com/Baobug/YunSSH/internal/session"
	"github.com/Baobug/YunSSH/internal/tray"
)

// 版本号取自根包的 yunssh.Version，由 build.bat 用 -ldflags -X 注入。
// 这里写进「应用和功能」的卸载项与关于对话框，必须与 exe 版本资源一致，
// 因此不再另放一份常量。

var (
	kernel32        = syscall.NewLazyDLL("kernel32.dll")
	procCreateMutex = kernel32.NewProc("CreateMutexW")
	procCloseHandle = kernel32.NewProc("CloseHandle")
)

func main() {
	if len(os.Args) > 1 {
		os.Exit(runCommand(os.Args[1:]))
	}

	release, ok := ensureSingleInstance()
	if !ok {
		// 托盘已经在跑。静默退出会让人以为点了没反应，所以给一句提示。
		dialog.Info("YunSSH 已在运行",
			"程序正在系统托盘中运行。\n\n"+
				"如果看不到图标，请点击任务栏的「显示隐藏的图标」箭头查找。")
		return
	}
	defer release()

	startInteractive()
}

// ensureSingleInstance 用命名互斥量保证同一时刻只有一个托盘实例。
//
// 托盘默认开机自启，用户从开始菜单再点一次就会拉起第二个进程；
// 没有这层保护，任务栏里会出现两个 YunSSH 图标。
func ensureSingleInstance() (release func(), ok bool) {
	return acquireInstanceLock(`Local\YunSSH.Tray`)
}

// acquireInstanceLock 是单实例检查的实现。锁名可注入，便于测试使用独立名字，
// 避免与真实运行中的托盘互相干扰。
func acquireInstanceLock(name string) (release func(), ok bool) {
	ptr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return func() {}, true
	}

	// Local\ 前缀表示只在当前登录会话内有效，多用户同时登录互不干扰。
	handle, _, callErr := procCreateMutex.Call(0, 0, uintptr(unsafe.Pointer(ptr)))
	if handle == 0 {
		// 创建失败时不拦截，避免因权限问题导致程序完全起不来
		return func() {}, true
	}
	if errno, isErrno := callErr.(syscall.Errno); isErrno && errno == syscall.ERROR_ALREADY_EXISTS {
		procCloseHandle.Call(handle)
		return func() {}, false
	}

	return func() { procCloseHandle.Call(handle) }, true
}

// startInteractive 处理双击启动的流程。
func startInteractive() {
	if !install.IsInstalled() {
		if !dialog.Confirm("安装 YunSSH",
			"是否把 YunSSH 安装到当前用户目录？\n\n"+
				"· 安装位置：%LOCALAPPDATA%\\Programs\\YunSSH\n"+
				"· 无需管理员权限\n"+
				"· 可在「设置 → 应用」中随时卸载\n"+
				"· 你的 ~/.ssh/config 不会被改动") {
			return
		}

		autoStart := dialog.Confirm("开机自启",
			"是否让 YunSSH 在登录时自动启动并常驻托盘？\n\n"+
				"稍后也可以在托盘菜单里随时切换。")

		if _, err := install.Install(install.Options{
			AutoStart: autoStart,
			AddToPath: true,
			Version:   yunssh.Version,
		}); err != nil {
			dialog.Error("安装失败", err.Error())
			os.Exit(1)
		}

		dialog.Info("安装完成",
			"YunSSH 已安装到：\n"+mustDir()+"\n\n"+
				"· 托盘区已可以看到它\n"+
				"· 命令行工具 yssh 已加入 PATH（重新打开终端后生效）\n"+
				"· 在「设置 → 应用」中可卸载")

		// 从安装目录重新拉起，避免这份"下载目录里的临时副本"继续常驻
		if relaunchFromInstallDir() == nil {
			return
		}
	}

	runTray()
}

// runTray 启动托盘消息循环。
func runTray() {
	app := &tray.App{
		Hosts:            loadHosts,
		OpenConfig:       openConfig,
		OpenSearch:       openSearch,
		AutoStartEnabled: install.AutoStartEnabled,
		SetAutoStart:     install.SetAutoStart,
		Terminal:         launcher.Wt,
	}
	app.Run()
}

// runCommand 处理带参数的调用。
func runCommand(args []string) int {
	switch strings.ToLower(args[0]) {
	case "install":
		return cmdInstall(args[1:])
	case "uninstall":
		return cmdUninstall(args[1:])
	case "autostart":
		return cmdAutoStart(args[1:])
	case "version", "--version", "-v":
		dialog.Info("YunSSH", "ysshtray "+yunssh.Version)
		return 0
	case "help", "--help", "-h":
		dialog.Info("YunSSH 托盘程序",
			"ysshtray 是图形程序，没有控制台输出。\n\n"+
				"命令行操作请使用 yssh：\n"+
				"  yssh install\n"+
				"  yssh uninstall\n"+
				"  yssh autostart on|off")
		return 0
	default:
		dialog.Error("未知参数", "ysshtray 不支持参数："+args[0])
		return 2
	}
}

// cmdInstall 执行安装。
func cmdInstall(args []string) int {
	quiet := false
	autoStart := true
	askAutoStart := true

	for _, a := range args {
		switch strings.ToLower(a) {
		case "--autostart":
			autoStart, askAutoStart = true, false
		case "--no-autostart":
			autoStart, askAutoStart = false, false
		case "--yes", "-y", "--quiet":
			quiet = true
		}
	}

	if askAutoStart && !quiet {
		autoStart = dialog.Confirm("开机自启", "是否让 YunSSH 在登录时自动启动并常驻托盘？")
	}

	if _, err := install.Install(install.Options{
		AutoStart: autoStart,
		AddToPath: true,
		Version:   yunssh.Version,
	}); err != nil {
		if !quiet {
			dialog.Error("安装失败", err.Error())
		}
		return 1
	}

	if !quiet {
		dialog.Info("安装完成", "YunSSH 已安装到：\n"+mustDir())
	}
	return 0
}

// cmdUninstall 执行卸载。
func cmdUninstall(args []string) int {
	quiet := false
	for _, a := range args {
		switch strings.ToLower(a) {
		case "--yes", "-y", "--quiet":
			quiet = true
		}
	}

	dir, err := install.Dir()
	if err != nil {
		if !quiet {
			dialog.Info("YunSSH", "YunSSH 尚未安装。")
		}
		return 0
	}

	if !quiet && !dialog.Confirm("卸载 YunSSH",
		"将从以下位置移除 YunSSH：\n\n"+dir+"\n\n"+
			"· 你的 ~/.ssh/config 不会被删除\n"+
			"· 开机自启项与 PATH 设置会一并清理") {
		return 0
	}

	// 托盘进程还在运行的话文件会被占用，先结束它（排除自己）
	stopOtherInstances()

	if err := install.Uninstall(); err != nil {
		if !quiet {
			dialog.Error("卸载失败", err.Error())
		}
		return 1
	}

	if !quiet {
		dialog.Info("已卸载",
			"YunSSH 已移除。\n\n程序目录会在本对话框关闭后自动清理。")
	}
	return 0
}

// cmdAutoStart 切换开机自启。
func cmdAutoStart(args []string) int {
	if len(args) == 0 {
		dialog.Info("开机自启", fmt.Sprintf("当前状态：%s", onOff(install.AutoStartEnabled())))
		return 0
	}

	var enable bool
	switch strings.ToLower(args[0]) {
	case "on", "true", "1", "enable":
		enable = true
	case "off", "false", "0", "disable":
		enable = false
	default:
		dialog.Error("参数错误", "用法：ysshtray autostart on|off")
		return 2
	}

	if err := install.SetAutoStart(enable); err != nil {
		dialog.Error("设置失败", err.Error())
		return 1
	}
	return 0
}

// stopOtherInstances 结束其它正在运行的托盘进程。
//
// 用 PID 过滤掉自身，否则 taskkill 会把当前进程也一起结束。
func stopOtherInstances() {
	_ = exec.Command("taskkill",
		"/IM", "ysshtray.exe",
		"/F",
		"/FI", fmt.Sprintf("PID ne %d", os.Getpid()),
	).Run()
}

// relaunchFromInstallDir 从安装目录重新启动托盘程序。
// 已在安装目录中运行时返回错误，避免无限重启。
func relaunchFromInstallDir() error {
	dir, err := install.Dir()
	if err != nil {
		return err
	}

	self, err := os.Executable()
	if err != nil {
		return err
	}
	if strings.EqualFold(filepath.Dir(self), dir) {
		return errors.New("已位于安装目录")
	}

	return exec.Command(filepath.Join(dir, "ysshtray.exe")).Start()
}

// mustDir 返回安装目录，失败时返回占位文本（仅用于提示文案）。
func mustDir() string {
	if dir, err := install.Dir(); err == nil {
		return dir
	}
	return "%LOCALAPPDATA%\\Programs\\YunSSH"
}

// onOff 把布尔值渲染为中文状态。
func onOff(v bool) string {
	if v {
		return "已开启"
	}
	return "已关闭"
}

// loadHosts 读取配置文件中的主机列表。
func loadHosts() ([]session.Host, error) {
	path, err := session.DefaultPath()
	if err != nil {
		return nil, err
	}
	cfg, err := session.Load(path)
	if err != nil {
		return nil, err
	}
	return cfg.List(), nil
}

// openConfig 用系统默认程序打开配置文件，文件不存在时先建一份骨架。
func openConfig() error {
	path, err := session.DefaultPath()
	if err != nil {
		return err
	}

	if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
		seed := "# ~/.ssh/config\n" +
			"# 由 YunSSH 创建。可直接手工编辑，yssh 不会重排或改写已有内容。\n"
		if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
			return err
		}
	}

	// 交给系统关联的编辑器，而不是写死 notepad
	return exec.Command("cmd", "/c", "start", "", path).Start()
}

// openSearch 在终端里打开完整的主机列表，等价于直接运行 yssh。
func openSearch() error {
	exe, err := ysshPath()
	if err != nil {
		return err
	}
	return launcher.StartCommand(exe)
}

// ysshPath 定位 CLI 可执行文件。
func ysshPath() (string, error) {
	if p, err := exec.LookPath("yssh.exe"); err == nil {
		return p, nil
	}

	// 优先找与托盘程序同目录的 yssh.exe——安装后两者总是放在一起
	if self, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(self), "yssh.exe")
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	return "", errors.New("未找到 yssh.exe，请确认它与 ysshtray.exe 位于同一目录")
}
