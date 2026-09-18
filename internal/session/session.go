// Package session 负责 ~/.ssh/config 的读取与写入。
//
// 设计要点：
//   - ~/.ssh/config 是唯一真相源，本包不自创任何会话格式，
//     因此写入的条目对原生 ssh、scp、git、VSCode Remote 全部可见
//   - 读取时保留原始行切片，写入只做追加与定点删除，
//     绝不重排、重写或"美化"用户手写的内容
//   - 字段解析默认走轻量文本扫描（快）；需要权威值时再调用 `ssh -G`
package session

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Baobug/YunSSH/internal/meta"
)

// DefaultPath 返回 ~/.ssh/config 的路径。
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ssh", "config"), nil
}

// Host 描述一条主机条目。
type Host struct {
	Alias        string
	HostName     string
	User         string
	Port         int
	IdentityFile string
	Meta         meta.Meta
	HasMeta      bool

	// Approximate 为 true 时表示字段来自文本扫描而非 `ssh -G`，
	// 因此不包含 Include、Match、通配符等指令带来的影响。
	Approximate bool
}

// Target 返回 "user@host:port" 形式的展示串，缺项会被省略。
func (h Host) Target() string {
	var b strings.Builder
	if h.User != "" {
		b.WriteString(h.User)
		b.WriteByte('@')
	}
	if h.HostName != "" {
		b.WriteString(h.HostName)
	} else {
		b.WriteString(h.Alias)
	}
	if h.Port != 0 {
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(h.Port))
	}
	return b.String()
}

// Config 是 ~/.ssh/config 的内存表示。
//
// lines 保留原始行（可能含行尾的 \r），写回时按原样拼接，
// 这是"不破坏用户手写内容"这一约束的实现基础。
type Config struct {
	Path string

	lines    []string
	crlf     bool
	notExist bool
	mtime    time.Time
}

// Load 读取配置文件。
//
// 文件不存在时返回一份空配置而非错误——首次使用时这正是预期状态。
func Load(path string) (*Config, error) {
	cfg := &Config{Path: path}

	fi, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg.notExist = true
			return cfg, nil
		}
		return nil, err
	}
	cfg.mtime = fi.ModTime()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return cfg, nil
	}

	content := string(data)
	cfg.crlf = strings.Contains(content, "\r\n")

	// 按 \n 切分并保留行尾的 \r，写回时 Join("\n") 可完整还原原始字节。
	cfg.lines = strings.Split(content, "\n")

	return cfg, nil
}

// Exists 报告配置文件在磁盘上是否存在。
func (c *Config) Exists() bool { return !c.notExist }

// lineAt 返回第 i 行的内容，已剥离 CRLF 的 \r。
func (c *Config) lineAt(i int) string {
	if i < 0 || i >= len(c.lines) {
		return ""
	}
	return strings.TrimSuffix(c.lines[i], "\r")
}

// Aliases 返回配置中全部可连接的别名，按出现顺序排列。
//
// 含通配符（* ? !）的 Host 行会被跳过——它们不是可直接连接的目标。
func (c *Config) Aliases() []string {
	entries := c.entries()

	out := make([]string, 0, len(entries))
	seen := make(map[string]bool, len(entries))
	for _, e := range entries {
		if seen[e.Alias] {
			continue
		}
		seen[e.Alias] = true
		out = append(out, e.Alias)
	}
	return out
}

// List 返回全部主机条目，字段来自轻量文本扫描。
//
// 需要 ssh -G 的权威解析结果时请使用 Resolve。
func (c *Config) List() []Host {
	entries := c.entries()

	out := make([]Host, 0, len(entries))
	seen := make(map[string]bool, len(entries))
	for _, e := range entries {
		if seen[e.Alias] {
			continue
		}
		seen[e.Alias] = true

		h := Host{
			Alias:       e.Alias,
			Meta:        e.Meta,
			HasMeta:     e.HasMeta,
			Approximate: true,
		}
		h.HostName, h.User, h.Port, h.IdentityFile = c.blockFields(e)
		out = append(out, h)
	}
	return out
}

// Find 按别名精确查找。
func (c *Config) Find(alias string) (Host, bool) {
	for _, h := range c.List() {
		if h.Alias == alias {
			return h, true
		}
	}
	return Host{}, false
}

// entry 描述配置文件中一个主机块的边界与元数据。
type entry struct {
	Alias    string
	Start    int // 块起始行（含元数据注释行，若有）
	HostLine int // Host 关键字所在行
	End      int // 块结束行（不含）
	Meta     meta.Meta
	HasMeta  bool
	Aliases  []string // 该 Host 行声明的全部别名
}

// entries 扫描配置文件，返回全部主机块。
//
// 一个 Host 行声明多个别名时会展开为多个 entry，但它们的边界相同，
// 调用方可通过 Aliases 字段识别这种情况。
func (c *Config) entries() []entry {
	type hostLine struct {
		index   int
		aliases []string
	}

	var hosts []hostLine
	for i := range c.lines {
		if aliases, ok := parseHostLine(c.lineAt(i)); ok {
			hosts = append(hosts, hostLine{i, aliases})
		}
	}

	out := make([]entry, 0, len(hosts))
	for i, h := range hosts {
		start := h.index
		var m meta.Meta
		var hasMeta bool

		// 向前吞掉紧邻的元数据注释行，使其成为本块的一部分。
		// 这样删除条目时元数据会一同消失，不留孤儿。
		if start > 0 {
			if mm, ok := meta.Parse(c.lineAt(start - 1)); ok {
				m, hasMeta = mm, true
				start--
			}
		}

		end := len(c.lines)
		if i+1 < len(hosts) {
			end = hosts[i+1].index
			// 下一个块的元数据注释行不属于当前块。
			if end > 0 {
				if _, ok := meta.Parse(c.lineAt(end - 1)); ok {
					end--
				}
			}
		}

		for _, alias := range h.aliases {
			out = append(out, entry{
				Alias:    alias,
				Start:    start,
				HostLine: h.index,
				End:      end,
				Meta:     m,
				HasMeta:  hasMeta,
				Aliases:  h.aliases,
			})
		}
	}
	return out
}

// blockFields 从主机块的文本中提取连接字段。
//
// 遵循 OpenSSH 的语义：同名关键字以**首次出现者**为准。
func (c *Config) blockFields(e entry) (hostname, user string, port int, identity string) {
	seen := make(map[string]bool, 4)

	for i := e.HostLine + 1; i < e.End && i < len(c.lines); i++ {
		trimmed := strings.TrimSpace(c.lineAt(i))
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		key, value, ok := splitKeyword(trimmed)
		if !ok {
			continue
		}

		lower := strings.ToLower(key)
		if seen[lower] {
			continue
		}

		switch lower {
		case "hostname":
			hostname = value
		case "user":
			user = value
		case "port":
			if n, err := strconv.Atoi(value); err == nil {
				port = n
			}
		case "identityfile":
			identity = value
		default:
			continue
		}
		seen[lower] = true
	}
	return hostname, user, port, identity
}

// parseHostLine 解析 "Host alias1 alias2" 行，返回其中的具名别名。
//
// 含通配符的条目会被跳过；若整行只声明通配符则返回 false。
func parseHostLine(line string) ([]string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return nil, false
	}

	// 归一化 "Host=foo" 写法（OpenSSH 允许用 = 代替空格）。
	if idx := strings.Index(trimmed, "="); idx > 0 {
		if strings.EqualFold(strings.TrimSpace(trimmed[:idx]), "host") {
			trimmed = "Host " + strings.TrimSpace(trimmed[idx+1:])
		}
	}

	fields := strings.Fields(trimmed)
	if len(fields) < 2 || !strings.EqualFold(fields[0], "host") {
		return nil, false
	}

	aliases := make([]string, 0, len(fields)-1)
	for _, f := range fields[1:] {
		if strings.ContainsAny(f, "*?!") {
			continue
		}
		aliases = append(aliases, f)
	}
	if len(aliases) == 0 {
		return nil, false
	}
	return aliases, true
}

// splitKeyword 切分 "Key value" 形式的配置行，兼容 "Key=value" 写法。
func splitKeyword(line string) (key, value string, ok bool) {
	if fields := strings.Fields(line); len(fields) >= 2 && !strings.Contains(fields[0], "=") {
		return fields[0], strings.Join(fields[1:], " "), true
	}
	if idx := strings.Index(line, "="); idx > 0 {
		return strings.TrimSpace(line[:idx]), strings.TrimSpace(line[idx+1:]), true
	}
	return "", "", false
}
