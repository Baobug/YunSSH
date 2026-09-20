// SPDX-FileCopyrightText: 2026 Zhou Tianbo (Baobug)
// SPDX-License-Identifier: Apache-2.0

// Command genicon 生成 YunSSH 的图标文件与 Windows 资源对象。
//
// 在仓库内任意目录执行：
//
//	go run ./tools/genicon
//
// 产物：
//
//	app.ico                              多尺寸图标，双击即可预览
//	cmd/yssh/rsrc_windows_amd64.syso
//	cmd/ysshtray/rsrc_windows_amd64.syso
//
// 两个 .syso 会被 go build 自动并入对应的 exe，让资源管理器、任务栏和
// 「应用和功能」里显示图标本身，而不是 Windows 的默认空白图标。
// 它们属于构建产物，已在 .gitignore 中排除。
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Baobug/YunSSH/internal/appicon"
)

// targetDirs 是需要嵌入图标的命令包目录。
var targetDirs = []string{
	filepath.Join("cmd", "yssh"),
	filepath.Join("cmd", "ysshtray"),
}

// sysoName 带上 GOOS_GOARCH，好让 go build 只在 Windows/amd64 下收录它；
// 交叉编译到别的平台时会被自动忽略，不会破坏构建。
const sysoName = "rsrc_windows_amd64.syso"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "genicon:", err)
		os.Exit(1)
	}
}

func run() error {
	root, err := moduleRoot()
	if err != nil {
		return err
	}

	ico := appicon.ICO()
	if err := writeFile(filepath.Join(root, "app.ico"), ico); err != nil {
		return err
	}
	fmt.Printf("[+] app.ico  %d 字节，含尺寸 %v\n", len(ico), appicon.Sizes)

	// 两个 exe 用同一份资源对象，图标一致
	obj := appicon.ResourceObject()
	for _, dir := range targetDirs {
		path := filepath.Join(root, dir, sysoName)
		if err := writeFile(path, obj); err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		fmt.Printf("[+] %s  %d 字节\n", rel, len(obj))
	}

	return nil
}

// moduleRoot 从当前目录向上找到模块根，也就是含 go.mod 的那一层。
func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("未找到 go.mod，请在 YunSSH 仓库内运行")
		}
		dir = parent
	}
}

func writeFile(path string, data []byte) error {
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("写入 %s 失败: %w", path, err)
	}
	return nil
}
