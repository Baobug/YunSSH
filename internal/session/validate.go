// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

package session

import (
	"fmt"
	"regexp"
	"strings"
)

// aliasRe 定义别名允许的字符集。
//
// 首字符必须是字母或数字——这是安全关键：以 `-` 开头的别名会被 ssh 解析成
// 命令行选项（`-oProxyCommand=...`、`-L`、`-v` 等），等于把配置内容当命令执行。
// 后续字符放宽到 `. _ -`，覆盖主机名、IP 段与常见的命名习惯。
//
// 刻意不含 `@`：ssh 会把 `foo@bar` 拆成「用户 foo + 主机 bar」，导致别名永远
// 匹配不到、实际连到别的主机。与其写入一个连不上的别名，不如在入口拒绝。
var aliasRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// ValidAlias 报告一个别名能否安全地写进配置、并作为 ssh 的第一个参数。
func ValidAlias(alias string) error {
	if !aliasRe.MatchString(alias) {
		return fmt.Errorf("别名只能由字母、数字、. _ - 组成，且不能以符号开头: %q", alias)
	}
	return nil
}

// rejectControl 拒绝可能破坏配置行结构的字符。
//
// 换行会把一条 HostName 拆成多条指令（配置注入），NUL 则可能被解析方截断。
// 这些字符出现在任何字段里都必须拒绝，而不能只在别名上拦。
func rejectControl(what, s string) error {
	if strings.ContainsAny(s, "\r\n\x00") {
		return fmt.Errorf("%s 不能包含换行或控制字符", what)
	}
	return nil
}

// validateHost 校验一条即将写入配置的主机条目。
//
// 这是写入前的单一入口：Alias 走字符集校验，其余字段与元数据走控制字符校验，
// 确保任何字段都无法通过换行注入额外的 ssh 指令。
func validateHost(h Host) error {
	if err := ValidAlias(h.Alias); err != nil {
		return err
	}

	fields := []struct{ what, value string }{
		{"主机名", h.HostName},
		{"用户名", h.User},
		{"身份文件", h.IdentityFile},
		{"环境", h.Meta.Env},
		{"创建日期", h.Meta.Added},
		{"备注", h.Meta.Note},
	}
	for _, f := range fields {
		if err := rejectControl(f.what, f.value); err != nil {
			return err
		}
	}
	for _, tag := range h.Meta.Tags {
		if err := rejectControl("标签", tag); err != nil {
			return err
		}
	}
	return nil
}
