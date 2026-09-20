// SPDX-FileCopyrightText: 2026 Zhou Tianbo (Baobug)
// SPDX-License-Identifier: Apache-2.0

//go:build windows

// Package install 实现 YunSSH 的自安装与卸载。
//
// 为什么不依赖 Inno Setup 之类的打包器：程序需要的注册表操作
// （开机自启、PATH 注入、卸载登记）用 Go 就能完成，不引入外部工具链，
// 分发时也就只有一个 exe。
//
// 安装逻辑全部集中在本包。将来若改用标准安装器，只需替换这一层，
// 程序自身代码不受影响。
package install

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	trayExeName = "ysshtray.exe"
	cliExeName  = "yssh.exe"
)

// Options 描述一次安装。
type Options struct {
	// Dir 是安装目录，留空则使用 DefaultDir。
	Dir string
	// AutoStart 表示是否设置开机自启。
	AutoStart bool
	// AddToPath 表示是否把安装目录加入用户 PATH。
	AddToPath bool
	// Version 会显示在「应用和功能」里。
	Version string
}

// Result 描述安装结果，供调用方展示。
type Result struct {
	Dir            string
	Files          []string
	AutoStart      bool
	PathUpdated    bool
	StartMenu      bool
	UninstallEntry bool
}

// DefaultDir 返回默认安装目录 %LOCALAPPDATA%\Programs\YunSSH。
//
// 选在用户目录而不是 Program Files，是为了让整个安装过程
// 无需管理员权限、不弹 UAC。
func DefaultDir() (string, error) {
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		return "", errors.New("环境变量 LOCALAPPDATA 未设置")
	}
	return filepath.Join(local, "Programs", "YunSSH"), nil
}

// Dir 返回当前实际安装目录；未安装时返回 ErrNotInstalled。
func Dir() (string, error) { return installLocation() }

// IsInstalled 报告本程序是否已经安装。
func IsInstalled() bool {
	_, err := installLocation()
	return err == nil
}

// InstalledVersion 返回已登记的版本号，未安装时返回空串。
func InstalledVersion() string { return installedVersion() }

// AutoStartEnabled 报告是否已设置开机自启。
func AutoStartEnabled() bool { return autoStartEnabled() }

// SetAutoStart 切换开机自启。要求程序已安装。
func SetAutoStart(enable bool) error {
	dir, err := installLocation()
	if err != nil {
		return errors.New("尚未安装，无法设置开机自启")
	}
	return setAutoStart(filepath.Join(dir, trayExeName), enable)
}

// Install 把程序安装到目标目录，并完成全部注册表登记。
func Install(opts Options) (*Result, error) {
	dir, err := resolveDir(opts.Dir)
	if err != nil {
		return nil, err
	}

	self, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("无法定位当前程序: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(self); err == nil {
		self = resolved
	}
	srcDir := filepath.Dir(self)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("创建安装目录失败: %w", err)
	}

	result := &Result{Dir: dir}

	// 托盘程序是必需的，CLI 是可选的（缺失不致命）。
	// 同名文件直接覆盖，因此重复执行即为升级。
	for _, name := range []string{trayExeName, cliExeName} {
		src := filepath.Join(srcDir, name)
		if _, statErr := os.Stat(src); statErr != nil {
			continue
		}

		dst := filepath.Join(dir, name)
		// 已经在目标位置运行时不复制自身，否则会因文件占用而失败
		if !samePath(src, dst) {
			if err := copyFile(src, dst); err != nil {
				return nil, fmt.Errorf("复制 %s 失败: %w", name, err)
			}
		}
		result.Files = append(result.Files, dst)
	}

	if len(result.Files) == 0 {
		return nil, errors.New("安装目录中未找到可安装的程序文件")
	}

	trayPath := filepath.Join(dir, trayExeName)

	if err := addUninstallEntry(opts.Version, dir, trayPath, trayPath); err != nil {
		return nil, err
	}
	result.UninstallEntry = true

	// 没有这一步，开始菜单里就找不到它——Windows 只索引 .lnk
	if err := createStartMenuShortcut(trayPath); err != nil {
		return nil, err
	}
	result.StartMenu = true

	if opts.AutoStart {
		if err := setAutoStart(trayPath, true); err != nil {
			return nil, err
		}
		result.AutoStart = true
	}

	if opts.AddToPath {
		if err := addToUserPath(dir); err != nil {
			return nil, err
		}
		result.PathUpdated = true
		broadcastEnvChange()
	}

	return result, nil
}

// Uninstall 移除注册表登记并安排删除安装目录。
//
// 正在运行的可执行文件无法删除自己，所以这里生成一个临时批处理，
// 由它在当前进程退出之后再清理目录。
func Uninstall() error {
	dir, err := installLocation()
	if err != nil {
		return err
	}

	// 先关自启：否则下次登录又被拉起来，而文件已经不在了
	if err := setAutoStart("", false); err != nil {
		return err
	}

	if err := removeFromUserPath(dir); err != nil {
		return err
	}
	broadcastEnvChange()

	if err := removeUninstallEntry(); err != nil {
		return err
	}

	if err := removeStartMenuShortcut(); err != nil {
		return err
	}

	return scheduleDirRemoval(dir)
}

// scheduleDirRemoval 生成一个延迟删除安装目录的批处理并启动它。
//
// 用 ping 而不是 timeout 做延时：timeout 在没有控制台的环境下会直接报错。
func scheduleDirRemoval(dir string) error {
	script := "@echo off\r\n" +
		"ping -n 3 127.0.0.1 >nul\r\n" +
		"rmdir /s /q \"" + dir + "\"\r\n" +
		"del \"%~f0\"\r\n"

	f, err := os.CreateTemp("", "yssh-uninstall-*.bat")
	if err != nil {
		return fmt.Errorf("创建卸载脚本失败: %w", err)
	}
	path := f.Name()

	if _, err := f.WriteString(script); err != nil {
		f.Close()
		_ = os.Remove(path)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return err
	}

	// 最小化启动，脚本在当前进程退出后继续执行
	return exec.Command("cmd", "/c", "start", "", "/min", path).Start()
}

// resolveDir 归一化安装目录。
func resolveDir(dir string) (string, error) {
	if strings.TrimSpace(dir) == "" {
		return DefaultDir()
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("安装路径无效: %w", err)
	}
	return abs, nil
}

// copyFile 复制文件内容。
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
