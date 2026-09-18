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
func (s *Store) Touch(alias string) {
	if s.Hosts == nil {
		s.Hosts = map[string]Entry{}
	}
	e := s.Hosts[alias]
	e.Count++
	e.LastUsed = time.Now()
	s.Hosts[alias] = e
}

// Save 持久化历史记录。失败静默忽略——这是辅助数据，不应打断主流程。
func (s *Store) Save() {
	if s == nil || s.path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(s.path, data, 0o600)
}
