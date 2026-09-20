// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

package backend

import "github.com/Baobug/YunSSH/internal/session"

// SSH 使用系统自带的 OpenSSH 客户端。
//
// 这是整套设计的最大红利：连接信息本来就写在 ~/.ssh/config 里，
// 因此这里只需要把别名交给 ssh，其余（HostName、User、Port、IdentityFile、
// ProxyJump、Include 展开……）全部由它自己解析，无需手工拼装任何参数。
type SSH struct{}

// Name 返回后端名称。
func (SSH) Name() string { return "ssh" }

// Build 生成 ssh 命令行。
func (SSH) Build(req Request) (string, []string, error) {
	exe, err := session.SSHBin()
	if err != nil {
		return "", nil, err
	}

	// 第一个参数之后的内容原样透传，因此 -L / -N / -v / -D / -- 等
	// 全部保持 ssh 原生语义，不存在参数冲突。
	args := make([]string, 0, 1+len(req.ExtraArgs))
	args = append(args, req.Alias)
	args = append(args, req.ExtraArgs...)

	return exe, args, nil
}
