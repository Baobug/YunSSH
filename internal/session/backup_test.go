// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeBackups 造 n 份时间戳备份，返回写入的文件名（按时间升序）。
func writeBackups(t *testing.T, dir string, n int) []string {
	t.Helper()

	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("创建备份目录失败: %v", err)
	}

	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.Local)
	names := make([]string, 0, n)
	for i := 0; i < n; i++ {
		name := "config." + base.Add(time.Duration(i)*time.Minute).Format("20060102-150405")
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatalf("写入备份失败: %v", err)
		}
		names = append(names, name)
	}
	return names
}

// listNames 返回目录下的文件名（已排序）。
func listNames(t *testing.T, dir string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("读取目录失败: %v", err)
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

// TestPruneBackupsKeepsNewest 覆盖 M7 修复的核心语义。
//
// 每份备份都是一份完整主机清单，此前从不清理。这里验证只保留最近
// backupKeep 份，且删掉的是最旧的而不是最新的。
func TestPruneBackupsKeepsNewest(t *testing.T) {
	dir := t.TempDir()
	names := writeBackups(t, dir, backupKeep+5)

	pruneBackups(dir)

	got := listNames(t, dir)
	if len(got) != backupKeep {
		t.Fatalf("保留 %d 份，期望 %d 份：%v", len(got), backupKeep, got)
	}

	// 期望留下的正是时间戳最大的那批。
	want := make(map[string]bool, backupKeep)
	for _, n := range names[len(names)-backupKeep:] {
		want[n] = true
	}
	for _, n := range got {
		if !want[n] {
			t.Errorf("应删除的旧备份 %s 仍在", n)
		}
	}
}

// TestPruneBackupsLeavesForeignFiles 覆盖「不越界」这一安全属性。
//
// 备份目录是用户家目录下的路径，只该动自己命名的那类文件；
// 一旦误删，代价是用户自己放进去的东西。
func TestPruneBackupsLeavesForeignFiles(t *testing.T) {
	dir := t.TempDir()
	writeBackups(t, dir, backupKeep+3)

	foreign := []string{
		"config.bak",            // 有前缀但不是时间戳
		"config.20260901-1000",  // 时间戳位数不对
		"notes.txt",             // 完全无关
		"config",                // 前缀本身
		".config.20260901-1000", // 不是以 config. 开头
	}
	for _, name := range foreign {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("keep me"), 0o600); err != nil {
			t.Fatalf("写入 %s 失败: %v", name, err)
		}
	}
	// 目录也不该被当成文件处理（时间戳刻意避开 writeBackups 生成的那批）。
	if err := os.Mkdir(filepath.Join(dir, "config.20200101-000000"), 0o700); err != nil {
		t.Fatalf("创建子目录失败: %v", err)
	}

	pruneBackups(dir)

	for _, name := range append(foreign, "config.20200101-000000") {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("非本函数管辖的文件被删除了: %s（%v）", name, err)
		}
	}
}

// TestPruneBackupsNoOpBelowLimit 覆盖边界：数量未超上限时不该有动作。
func TestPruneBackupsNoOpBelowLimit(t *testing.T) {
	for _, n := range []int{0, 1, backupKeep - 1, backupKeep} {
		dir := t.TempDir()
		names := writeBackups(t, dir, n)

		pruneBackups(dir)

		if got := listNames(t, dir); len(got) != len(names) {
			t.Errorf("n=%d 时文件数从 %d 变成 %d", n, len(names), len(got))
		}
	}
}

// TestPruneBackupsMissingDir 覆盖目录不存在：必须静默返回，不能 panic。
func TestPruneBackupsMissingDir(t *testing.T) {
	pruneBackups(filepath.Join(t.TempDir(), "nope"))
}

// TestBackupWritesTimestampedCopy 覆盖 Backup 仍按既有命名写盘并返回路径，
// 顺带确认清理钩子没有把刚写的那份一起删掉。
func TestBackupWritesTimestampedCopy(t *testing.T) {
	const content = "Host a\n    HostName 1.2.3.4\n"
	cfg, _ := newConfig(t, content)

	path, err := cfg.Backup()
	if err != nil {
		t.Fatalf("Backup() 报错: %v", err)
	}
	if path == "" {
		t.Fatal("Backup() 未返回备份路径")
	}
	if _, err := time.Parse("20060102-150405", strings.TrimPrefix(filepath.Base(path), "config.")); err != nil {
		t.Errorf("备份文件名不符合 config.<时间戳> 约定: %s", filepath.Base(path))
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("备份文件不可读: %v", err)
	}
	if string(data) != content {
		t.Errorf("备份内容与源文件不一致:\n%s", data)
	}
}

// TestBackupOfMissingConfigIsNoop 覆盖配置文件不存在时返回空路径。
func TestBackupOfMissingConfigIsNoop(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "absent"))
	if err != nil {
		t.Fatalf("Load 报错: %v", err)
	}

	path, err := cfg.Backup()
	if err != nil {
		t.Fatalf("Backup() 报错: %v", err)
	}
	if path != "" {
		t.Errorf("配置不存在时应返回空路径，实际 %q", path)
	}
}

// TestBackupPrunesAccumulated 覆盖端到端效果：反复备份后目录规模有上界。
//
// 同秒内的备份会落到同一个文件名（时间戳精度到秒），因此这里用独立
// 目录预置存量备份，再触发一次真实 Backup 来验证清理被接上了。
func TestBackupPrunesAccumulated(t *testing.T) {
	dir, err := stateDir()
	if err != nil {
		t.Fatalf("stateDir 报错: %v", err)
	}
	backupDir := filepath.Join(dir, "backup")
	if err := os.RemoveAll(backupDir); err != nil {
		t.Fatalf("清理备份目录失败: %v", err)
	}
	writeBackups(t, backupDir, backupKeep+7)

	cfg, _ := newConfig(t, "Host a\n    HostName 1.2.3.4\n")
	if _, err := cfg.Backup(); err != nil {
		t.Fatalf("Backup() 报错: %v", err)
	}

	if got := len(listNames(t, backupDir)); got > backupKeep {
		t.Errorf("备份目录残留 %d 份，上限应为 %d", got, backupKeep)
	}
}

// TestBackupKeepIsSane 防止上限被改成 0 —— 那等于备份功能被静默关闭。
func TestBackupKeepIsSane(t *testing.T) {
	if backupKeep < 1 {
		t.Fatalf("backupKeep = %d，至少应保留 1 份备份", backupKeep)
	}
	if backupKeep > 100 {
		t.Errorf("backupKeep = %d，清理形同虚设", backupKeep)
	}
}
