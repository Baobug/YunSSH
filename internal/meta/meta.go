// Package meta 负责解析与生成内嵌在 ~/.ssh/config 中的 YunSSH 元数据注释。
//
// 元数据以注释形式与主机块写在同一文件中，这样删除主机条目时元数据会一同消失，
// 不会产生孤儿记录，而且用 git 管理 config 时元数据也能一起版本化。
//
// 格式：
//
//	# yssh: env=prod tags=web,hk added=2026-09-18 note=主站反代
//	Host prod-web
//	    HostName 1.2.3.4
//
// 规则：
//   - 前缀固定为 "# yssh:"，OpenSSH 会忽略该行
//   - 字段以空格分隔，形式为 key=value
//   - note 必须位于最后一个字段，取值到行尾，允许包含空格
package meta

import "strings"

// Prefix 是元数据注释的固定前缀，OpenSSH 将其视为普通注释。
const Prefix = "# yssh:"

// Meta 承载一条主机条目的扩展元数据。
// 零值表示"没有元数据"，这是合法状态（手工添加的条目就没有）。
type Meta struct {
	Env   string   // 环境标识，如 prod / test / lab
	Tags  []string // 标签列表
	Added string   // 创建日期，格式 YYYY-MM-DD
	Note  string   // 备注，必须位于最后一个字段
}

// IsZero 报告元数据是否完全为空。
func (m Meta) IsZero() bool {
	return m.Env == "" && len(m.Tags) == 0 && m.Added == "" && m.Note == ""
}

// Parse 解析一行文本。第二个返回值表示该行是否是 YunSSH 元数据行。
func Parse(line string) (Meta, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, Prefix) {
		return Meta{}, false
	}

	body := strings.TrimSpace(trimmed[len(Prefix):])
	var m Meta
	if body == "" {
		return m, true
	}

	// note 取值到行尾，先把它切出来，避免其中的空格干扰后续按空格切分。
	if idx := strings.Index(body, "note="); idx >= 0 {
		m.Note = strings.TrimSpace(body[idx+len("note="):])
		body = strings.TrimSpace(body[:idx])
	}

	// 不含等号的片段视为前一个字段值的延续。
	// 这用于容忍手工书写 "tags=web, hk" 这类带空格的写法，
	// 否则后半截会被静默丢弃，用户很难察觉。
	lastKey := ""
	for _, field := range strings.Fields(body) {
		key, value, ok := strings.Cut(field, "=")
		if !ok {
			if lastKey == "tags" {
				m.Tags = append(m.Tags, splitTags(field)...)
			}
			continue
		}
		if value == "" {
			continue
		}

		lastKey = strings.ToLower(key)
		switch lastKey {
		case "env":
			m.Env = value
		case "tags":
			m.Tags = splitTags(value)
		case "added":
			m.Added = value
		}
	}
	return m, true
}

// Render 生成元数据注释行（不含换行符）。
// 元数据为空时返回空字符串，调用方据此决定是否写入该行。
func (m Meta) Render() string {
	if m.IsZero() {
		return ""
	}

	var b strings.Builder
	b.WriteString(Prefix)
	if m.Env != "" {
		b.WriteString(" env=")
		b.WriteString(m.Env)
	}
	if len(m.Tags) > 0 {
		b.WriteString(" tags=")
		b.WriteString(strings.Join(m.Tags, ","))
	}
	if m.Added != "" {
		b.WriteString(" added=")
		b.WriteString(m.Added)
	}
	// note 必须最后写，因为它取值到行尾。
	if m.Note != "" {
		b.WriteString(" note=")
		b.WriteString(m.Note)
	}
	return b.String()
}

// splitTags 按逗号切分标签并去除空白项。
func splitTags(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
