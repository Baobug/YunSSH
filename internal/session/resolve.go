// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

package session

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// ErrSSHNotFound 表示系统中找不到 ssh 可执行文件。
var ErrSSHNotFound = errors.New("未找到 ssh 可执行文件，请确认已启用 Windows 的 OpenSSH 客户端功能")

// sshBinEnv 允许用环境变量覆盖 ssh 可执行文件路径。
// 用途有二：测试时注入桩程序；让用户指定非默认位置的 ssh。
const sshBinEnv = "YSSH_SSH_BIN"

// SSHBin 定位 ssh 可执行文件。
func SSHBin() (string, error) {
	if p := strings.TrimSpace(os.Getenv(sshBinEnv)); p != "" {
		if _, err := os.Stat(p); err != nil {
			return "", fmt.Errorf("%s 指向的路径不可用: %w", sshBinEnv, err)
		}
		return p, nil
	}

	if p, err := exec.LookPath("ssh"); err == nil {
		return p, nil
	}
	// 兜底：Windows OpenSSH 的默认安装位置。
	for _, p := range []string{
		`C:\Windows\System32\OpenSSH\ssh.exe`,
		`C:\Program Files\OpenSSH\ssh.exe`,
	} {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", ErrSSHNotFound
}

// Resolve 通过 `ssh -G` 获取别名对应的生效配置。
//
// 相较自行解析配置文件，这样能零成本正确处理 Include、Match、
// 通配符与默认值合并，结论始终与真实 ssh 的行为一致。
//
// 注意：`ssh -G` 对不存在的别名不会报错（会把输入回显为 hostname），
// 因此调用前应先用 Config.Find 确认别名存在。
func Resolve(alias string) (Host, error) {
	exe, err := SSHBin()
	if err != nil {
		return Host{}, err
	}

	cmd := exec.Command(exe, "-G", alias)
	cmd.Stderr = io.Discard

	out, err := cmd.Output()
	if err != nil {
		return Host{}, err
	}

	h := Host{Alias: alias}
	for _, line := range strings.Split(string(out), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok || value == "" {
			continue
		}

		// ssh -G 输出中同名关键字可能出现多次，以首次为准。
		switch strings.ToLower(key) {
		case "hostname":
			if h.HostName == "" {
				h.HostName = value
			}
		case "user":
			if h.User == "" {
				h.User = value
			}
		case "port":
			if h.Port == 0 {
				if n, err := strconv.Atoi(value); err == nil {
					h.Port = n
				}
			}
		case "identityfile":
			if h.IdentityFile == "" {
				h.IdentityFile = value
			}
		}
	}
	return h, nil
}
