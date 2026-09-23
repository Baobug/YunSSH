// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// withHome 把家目录指向临时目录，并返回该目录。
//
// history 通过 os.UserHomeDir 定位 ~/.yssh，Windows 看 USERPROFILE、
// Unix 看 HOME，两个都要设，否则测试会污染真实的 ~/.yssh。
func withHome(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	return dir
}

// readRaw 读取 history.json 的原始内容。
func readRaw(t *testing.T, home string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(home, ".yssh", "history.json"))
	if err != nil {
		t.Fatalf("读取 history.json 失败: %v", err)
	}
	return string(data)
}

// TestLoadMissingFileReturnsEmpty 覆盖首次运行：文件不存在要给出可用的空记录。
func TestLoadMissingFileReturnsEmpty(t *testing.T) {
	withHome(t)

	s := Load()
	if s == nil {
		t.Fatal("Load() 返回 nil")
	}
	if s.Version != currentVersion {
		t.Errorf("Version = %d，期望 %d", s.Version, currentVersion)
	}
	if len(s.Hosts) != 0 {
		t.Errorf("应为空记录，实际 %v", s.Hosts)
	}
	if got := s.Get("anything"); got.Count != 0 || !got.LastUsed.IsZero() {
		t.Errorf("无记录时应返回零值，实际 %+v", got)
	}
}

// TestLoadCorruptFileFallsBackToEmpty 覆盖文件损坏：辅助数据不该让主流程崩掉。
func TestLoadCorruptFileFallsBackToEmpty(t *testing.T) {
	home := withHome(t)

	dir := filepath.Join(home, ".yssh")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("建目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "history.json"), []byte("{ 坏掉的 json"), 0o600); err != nil {
		t.Fatalf("写入损坏文件失败: %v", err)
	}

	s := Load()
	if len(s.Hosts) != 0 {
		t.Errorf("损坏文件应退化为空记录，实际 %v", s.Hosts)
	}
}

// TestSaveThenLoadRoundTrip 覆盖最基本的持久化往返。
func TestSaveThenLoadRoundTrip(t *testing.T) {
	home := withHome(t)

	s := Load()
	s.Touch("prod-web")
	s.Touch("prod-web")
	s.Touch("msf")
	s.Save()

	back := Load()
	if got := back.Get("prod-web").Count; got != 2 {
		t.Errorf("prod-web 计数 = %d，期望 2", got)
	}
	if got := back.Get("msf").Count; got != 1 {
		t.Errorf("msf 计数 = %d，期望 1", got)
	}
	if back.Get("prod-web").LastUsed.IsZero() {
		t.Error("LastUsed 未被记录")
	}

	// 落盘内容必须是合法 JSON —— 这是「CLI 与托盘都能读」的前提。
	var raw map[string]any
	if err := json.Unmarshal([]byte(readRaw(t, home)), &raw); err != nil {
		t.Fatalf("history.json 不是合法 JSON: %v", err)
	}
}

// TestSaveLeavesNoTempFiles 覆盖原子写不残留垃圾。
//
// 临时文件残留在 ~/.yssh 里会一直堆积，且名字带随机后缀，
// 用户根本认不出是什么，只敢一直留着。
func TestSaveLeavesNoTempFiles(t *testing.T) {
	home := withHome(t)

	s := Load()
	s.Touch("a")
	s.Save()
	s.Touch("b")
	s.Save()

	entries, err := os.ReadDir(filepath.Join(home, ".yssh"))
	if err != nil {
		t.Fatalf("读取状态目录失败: %v", err)
	}
	for _, e := range entries {
		if e.Name() != "history.json" {
			t.Errorf("状态目录残留多余文件: %s", e.Name())
		}
	}
}

// TestSaveOverwritesFileInsteadOfAppending 覆盖写文件语义：
// 保存的必须是当前内存状态的完整快照，而不是往文件里追加内容。
func TestSaveOverwritesFileInsteadOfAppending(t *testing.T) {
	home := withHome(t)

	first := Load()
	first.Touch("old-alias")
	first.Touch("old-alias")
	first.Save()

	second := Load()
	second.Hosts = map[string]Entry{} // 模拟记录被整体重建（例如 Prune 后）
	second.Touch("new-alias")
	second.Save()

	raw := readRaw(t, home)
	if strings.Contains(raw, "old-alias") {
		t.Errorf("上一版内容未被覆盖:\n%s", raw)
	}
	if !strings.Contains(raw, "new-alias") {
		t.Errorf("新内容未写入:\n%s", raw)
	}
	// 覆盖而非追加的直接证据：别名只该出现一次。
	if n := strings.Count(raw, `"new-alias"`); n != 1 {
		t.Errorf("new-alias 出现了 %d 次，说明发生了追加:\n%s", n, raw)
	}
}

// TestSaveConcurrentStaysValid 覆盖 M8 的核心动机：CLI 与托盘同时写。
//
// 只要临时文件名不唯一，两个写者就会交错写同一个 .tmp，
// rename 之后留下的是一份坏 JSON。
func TestSaveConcurrentStaysValid(t *testing.T) {
	home := withHome(t)

	// 各自 Load 一份，模拟两个独立进程持有的句柄。
	const writers = 8
	var wg sync.WaitGroup
	start := make(chan struct{})

	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			s := Load()
			alias := "alias-" + strings.Repeat("x", i)
			<-start // 尽量让写操作撞在一起
			for n := 0; n < 20; n++ {
				s.Touch(alias)
				s.Save()
			}
		}(i)
	}

	close(start)
	wg.Wait()

	// 并发结束后文件必须仍是合法 JSON，且没有任何临时文件残留。
	var raw map[string]any
	if err := json.Unmarshal([]byte(readRaw(t, home)), &raw); err != nil {
		t.Fatalf("并发写入后 history.json 已损坏: %v", err)
	}

	entries, err := os.ReadDir(filepath.Join(home, ".yssh"))
	if err != nil {
		t.Fatalf("读取状态目录失败: %v", err)
	}
	for _, e := range entries {
		if e.Name() != "history.json" {
			t.Errorf("并发写入残留临时文件: %s", e.Name())
		}
	}
}

// TestPruneDropsStaleAliases 覆盖 M8 的第二半：删除主机后历史不该永久留存。
func TestPruneDropsStaleAliases(t *testing.T) {
	withHome(t)

	s := Load()
	for _, alias := range []string{"keep-1", "keep-2", "gone-1", "gone-2"} {
		s.Touch(alias)
	}

	s.Prune([]string{"keep-1", "keep-2"})

	if len(s.Hosts) != 2 {
		t.Fatalf("清理后剩 %d 条，期望 2：%v", len(s.Hosts), s.Hosts)
	}
	for _, alias := range []string{"keep-1", "keep-2"} {
		if s.Get(alias).Count == 0 {
			t.Errorf("有效别名 %s 的记录被误删", alias)
		}
	}
	for _, alias := range []string{"gone-1", "gone-2"} {
		if _, ok := s.Hosts[alias]; ok {
			t.Errorf("失效别名 %s 未被清理", alias)
		}
	}
}

// TestPruneNoopWhenValidEmpty 覆盖防误伤：读配置失败时 valid 会是空切片，
// 此时清空历史等于把用户的全部统计抹掉。
func TestPruneNoopWhenValidEmpty(t *testing.T) {
	withHome(t)

	for _, valid := range [][]string{nil, {}} {
		s := Load()
		s.Touch("a")
		s.Touch("b")

		s.Prune(valid)

		if len(s.Hosts) != 2 {
			t.Errorf("valid=%v 时不应清理，实际剩 %v", valid, s.Hosts)
		}
	}
}

// TestPruneNilSafety 覆盖零值句柄：history 是辅助数据，
// 任何一处拿到空指针都不该 panic 掉整个命令。
func TestPruneNilSafety(t *testing.T) {
	var s *Store
	s.Prune([]string{"a"})
	s.Save()
	s.Touch("a")
	if got := s.Get("a"); got.Count != 0 {
		t.Errorf("nil Store 的 Get 应返回零值，实际 %+v", got)
	}

	empty := &Store{}
	empty.Prune([]string{"a"})
	empty.Save()
}

// TestTouchInitialisesMap 覆盖 Touch 对零值 Store 的自愈能力。
func TestTouchInitialisesMap(t *testing.T) {
	s := &Store{}
	s.Touch("a")
	if s.Get("a").Count != 1 {
		t.Errorf("Touch 未初始化 Hosts：%v", s.Hosts)
	}
	s.Touch("a")
	if s.Get("a").Count != 2 {
		t.Errorf("计数未累加：%v", s.Get("a"))
	}
}

// TestSaveWithoutPathIsNoop 覆盖未经过 Load 的句柄（path 为空）不写盘。
func TestSaveWithoutPathIsNoop(t *testing.T) {
	home := withHome(t)

	s := &Store{Version: currentVersion, Hosts: map[string]Entry{}}
	s.Touch("a")
	s.Save()

	if _, err := os.Stat(filepath.Join(home, ".yssh", "history.json")); !os.IsNotExist(err) {
		t.Errorf("path 为空时不应写出文件（err=%v）", err)
	}
}

// TestEntryRoundTrip 确认 Entry 的 JSON 字段名稳定——
// 改了字段名会让所有老用户的记录静默失效。
func TestEntryRoundTrip(t *testing.T) {
	when := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	data, err := json.Marshal(Entry{LastUsed: when, Count: 3})
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	if !strings.Contains(string(data), `"lastUsed"`) || !strings.Contains(string(data), `"count"`) {
		t.Errorf("字段名发生变更，老记录会失效: %s", data)
	}
}
