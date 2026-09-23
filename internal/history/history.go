// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

// Package history 记录主机别名的使用情况，用于「最近使用优先」排序。
//
// 数据保存在 ~/.yssh/history.json。这属于非关键数据：
// 任何读写失败都不会影响连接功能，损坏或删除后会自动重建。
package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// currentVersion 是当前的历史记录格式版本。
const currentVersion = 1

// Entry 记录单个别名的使用统计。
type Entry struct {
	LastUsed time.Time `json:"lastUsed"`
	Count    int       `json:"count"`
}

// Store 是历史记录的读写句柄。
type Store struct {
	Version int              `json:"version"`
	Hosts   map[string]Entry `json:"hosts"`

	path string
}

// Dir 返回 YunSSH 的状态目录 ~/.yssh。
func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".yssh"), nil
}

// Load 读取历史记录。
// 文件不存在或内容损坏时返回一份空记录，不向调用方传播错误。
func Load() *Store {
	s := &Store{Version: currentVersion, Hosts: map[string]Entry{}}

	dir, err := Dir()
	if err != nil {
		return s
	}
	s.path = filepath.Join(dir, "history.json")

	data, err := os.ReadFile(s.path)
	if err != nil {
		return s
	}

	var loaded Store
	if err := json.Unmarshal(data, &loaded); err != nil {
		return s
	}
	if loaded.Hosts != nil {
		s.Hosts = loaded.Hosts
	}
	return s
}

// Get 返回别名的使用记录，无记录时返回零值。
func (s *Store) Get(alias string) Entry {
	if s == nil || s.Hosts == nil {
		return Entry{}
	}
	return s.Hosts[alias]
}

// Touch 记录一次使用。
//
// 与 Get/Prune/Save 一样对空句柄免疫：历史只是辅助数据，
// 任何一处拿到 nil 都不该把整个命令带崩。
func (s *Store) Touch(alias string) {
	if s == nil {
		return
	}
	if s.Hosts == nil {
		s.Hosts = map[string]Entry{}
	}
	e := s.Hosts[alias]
	e.Count++
	e.LastUsed = time.Now()
	s.Hosts[alias] = e
}

// Save 原子地持久化历史记录。失败静默忽略——这是辅助数据，不应打断主流程。
//
// 先写临时文件再 rename：CLI 与托盘可能同时在写，直接 WriteFile 会互相
// 踩踏，结果两边都丢统计（文件损坏后虽会自动重建，但记录已经没了）。
// 临时文件名必须唯一（CreateTemp），否则两个进程会写同一个 .tmp，
// 交错的内容被 rename 成正式文件后依然是坏的。
func (s *Store) Save() {
	if s == nil || s.path == "" {
		return
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}

	f, err := os.CreateTemp(dir, ".history-*.tmp")
	if err != nil {
		return
	}
	tmp := f.Name()

	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return
	}
	// CreateTemp 建出来是 0600，这里显式再设一次，避免受 umask 影响。
	_ = os.Chmod(tmp, 0o600)

	// Windows 上 os.Rename 走 MoveFileEx + MOVEFILE_REPLACE_EXISTING，可覆盖已存在文件。
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
	}
}

// Prune 丢弃配置中已不存在的别名记录。
//
// 删除主机后其记录会永久留在 history.json 里，长期积累等于一份已失效的
// 资产清单。valid 为空时不做任何事——那多半意味着调用方读取配置失败，
// 此时清空历史属于误伤。
func (s *Store) Prune(valid []string) {
	if s == nil || s.Hosts == nil || len(valid) == 0 {
		return
	}

	keep := make(map[string]bool, len(valid))
	for _, alias := range valid {
		keep[alias] = true
	}
	for alias := range s.Hosts {
		if !keep[alias] {
			delete(s.Hosts, alias)
		}
	}
}
