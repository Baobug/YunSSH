// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestVerifyUninstallDir 验证卸载前的目录合法性校验。
//
// 这是 M3 的修复：注册表 InstallLocation 可被同用户进程改写，卸载若不加校验，
// 会把任意目录递归删除。目录内必须存在 YunSSH 的程序文件才允许删。
func TestVerifyUninstallDir(t *testing.T) {
	t.Run("含托盘程序通过", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, trayExeName), nil, 0o644); err != nil {
			t.Fatalf("写入桩文件失败: %v", err)
		}
		if err := verifyUninstallDir(dir); err != nil {
			t.Errorf("含 %s 的目录应通过校验，得到 %v", trayExeName, err)
		}
	})

	t.Run("含 CLI 程序通过", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, cliExeName), nil, 0o644); err != nil {
			t.Fatalf("写入桩文件失败: %v", err)
		}
		if err := verifyUninstallDir(dir); err != nil {
			t.Errorf("含 %s 的目录应通过校验，得到 %v", cliExeName, err)
		}
	})

	t.Run("空目录拒绝", func(t *testing.T) {
		if err := verifyUninstallDir(t.TempDir()); err == nil {
			t.Error("空目录应被拒绝")
		}
	})

	t.Run("只有无关文件拒绝", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "random.txt"), nil, 0o644); err != nil {
			t.Fatalf("写入无关文件失败: %v", err)
		}
		err := verifyUninstallDir(dir)
		if err == nil {
			t.Fatal("不含 YunSSH 程序的目录应被拒绝")
		}
		if !strings.Contains(err.Error(), "拒绝删除") {
			t.Errorf("错误信息应提示拒绝删除，得到 %v", err)
		}
	})
}
