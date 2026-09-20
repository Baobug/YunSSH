// SPDX-FileCopyrightText: 2026 Zhou Tianbo (Baobug)
// SPDX-License-Identifier: Apache-2.0

package session

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var (
	// ErrModified 表示配置文件在本次读取之后被其他程序改动过。
	ErrModified = errors.New("配置文件已被其他程序修改，请重试")

	// ErrAliasExists 表示要添加的别名已存在。
	ErrAliasExists = errors.New("别名已存在")

	// ErrAliasNotFound 表示要操作的别名不存在。
	ErrAliasNotFound = errors.New("别名不存在")

	// ErrSharedHostLine 表示目标 Host 行声明了多个别名，删除会波及兄弟别名。
	ErrSharedHostLine = errors.New("该 Host 行声明了多个别名，为避免误删请手工编辑配置文件")
)

// Add 在配置文件末尾追加一条主机条目。
//
// 已存在的别名会返回 ErrAliasExists，不做静默覆盖。
func (c *Config) Add(h Host) error {
	if _, found := c.Find(h.Alias); found {
		return fmt.Errorf("%w: %s", ErrAliasExists, h.Alias)
	}
	if strings.ContainsAny(h.Alias, " \t*?!=") {
		return fmt.Errorf("别名不能包含空格、等号或通配符: %q", h.Alias)
	}

	// 确保已有内容与新增块之间留一个空行。
	if n := len(c.lines); n > 0 && strings.TrimSpace(c.lineAt(n-1)) != "" {
		c.lines = append(c.lines, "")
	}
	c.lines = append(c.lines, c.renderBlock(h)...)

	return c.Save()
}

// renderBlock 生成一条主机条目的文本行（含元数据注释）。
func (c *Config) renderBlock(h Host) []string {
	lines := make([]string, 0, 7)

	if m := h.Meta.Render(); m != "" {
		lines = append(lines, m)
	}
	lines = append(lines, "Host "+h.Alias)

	if h.HostName != "" {
		lines = append(lines, "    HostName "+h.HostName)
	}
	if h.User != "" {
		lines = append(lines, "    User "+h.User)
	}
	if h.Port != 0 {
		lines = append(lines, "    Port "+strconv.Itoa(h.Port))
	}
	if h.IdentityFile != "" {
		lines = append(lines, "    IdentityFile "+h.IdentityFile)
	}

	// 跟随原文件的行尾风格，避免 CRLF 与 LF 混用。
	if c.crlf {
		for i := range lines {
			lines[i] += "\r"
		}
	}
	return lines
}

// RemoveResult 描述一次删除操作的结果。
type RemoveResult struct {
	Alias    string
	Lines    []string // 被删除的原始行
	BackupAt string   // 备份文件路径，可能为空
}

// Remove 删除指定别名的整块配置。
//
// 操作前会备份原文件；若目标 Host 行声明了多个别名则拒绝执行，
// 以免在用户未察觉的情况下删掉兄弟别名。
func (c *Config) Remove(alias string) (*RemoveResult, error) {
	var target *entry
	for _, e := range c.entries() {
		if e.Alias == alias {
			cp := e
			target = &cp
			break
		}
	}
	if target == nil {
		return nil, fmt.Errorf("%w: %s", ErrAliasNotFound, alias)
	}
	if len(target.Aliases) > 1 {
		return nil, fmt.Errorf("%w（%s 与 %s 共享同一行）",
			ErrSharedHostLine, alias, strings.Join(target.Aliases, "、"))
	}

	backup, err := c.Backup()
	if err != nil {
		return nil, fmt.Errorf("备份配置文件失败: %w", err)
	}

	removed := make([]string, target.End-target.Start)
	copy(removed, c.lines[target.Start:target.End])

	c.lines = append(c.lines[:target.Start], c.lines[target.End:]...)
	c.trimBlankLines(target.Start)

	if err := c.Save(); err != nil {
		return nil, err
	}
	return &RemoveResult{Alias: alias, Lines: removed, BackupAt: backup}, nil
}

// trimBlankLines 在删除位置附近压缩多余空行，避免条目移除后留下空洞。
func (c *Config) trimBlankLines(pos int) {
	for pos > 0 && pos < len(c.lines) {
		if strings.TrimSpace(c.lineAt(pos)) != "" || strings.TrimSpace(c.lineAt(pos-1)) != "" {
			return
		}
		c.lines = append(c.lines[:pos], c.lines[pos+1:]...)
	}
}

// Backup 把当前磁盘上的配置文件复制到 ~/.yssh/backup/。
//
// 返回备份文件路径；配置文件不存在时返回空字符串。
func (c *Config) Backup() (string, error) {
	if c.notExist {
		return "", nil
	}

	data, err := os.ReadFile(c.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	dir, err := stateDir()
	if err != nil {
		return "", err
	}
	backupDir := filepath.Join(dir, "backup")
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		return "", err
	}

	path := filepath.Join(backupDir, "config."+time.Now().Format("20060102-150405"))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// Save 原子地写回配置文件。
//
// 写入前比对文件修改时间：若期间被其他程序改动过则中止并返回 ErrModified，
// 避免覆盖用户或其他编辑器的改动。
func (c *Config) Save() error {
	if err := c.checkUnchanged(); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(c.Path), 0o700); err != nil {
		return err
	}

	content := strings.Join(c.lines, "\n")

	// 先写临时文件再重命名，避免写入中断导致配置文件损坏。
	tmp := c.Path + ".yssh-tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o600); err != nil {
		return err
	}

	// Windows 上 os.Rename 使用 MoveFileEx + MOVEFILE_REPLACE_EXISTING，可覆盖已存在文件。
	if err := os.Rename(tmp, c.Path); err != nil {
		_ = os.Remove(tmp)
		return err
	}

	if fi, err := os.Stat(c.Path); err == nil {
		c.mtime = fi.ModTime()
	}
	c.notExist = false
	return nil
}

// checkUnchanged 校验磁盘上的文件与 Load 时相比未被改动。
func (c *Config) checkUnchanged() error {
	if c.notExist {
		// 加载时不存在，如今却出现了，说明有其他程序抢先创建，不覆盖。
		if _, err := os.Stat(c.Path); err == nil {
			return ErrModified
		}
		return nil
	}

	fi, err := os.Stat(c.Path)
	switch {
	case err == nil:
		if !c.mtime.IsZero() && !fi.ModTime().Equal(c.mtime) {
			return ErrModified
		}
	case os.IsNotExist(err):
		// 文件被删除，视为无冲突，交由后续写入重建。
	default:
		return err
	}
	return nil
}

// stateDir 返回 ~/.yssh 目录路径。
func stateDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".yssh"), nil
}
