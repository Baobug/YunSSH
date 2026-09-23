// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

package session

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

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
	// 兜底：各平台 ssh 的常见安装位置。
	// 具体路径与 ErrSSHNotFound 的文案由平台文件提供
	// （resolve_windows.go / resolve_other.go），这里不出现平台判断。
	for _, p := range sshFallbackPaths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", ErrSSHNotFound
}

// resolveTimeout 限制 `ssh -G` 的执行时间。
//
// ssh -G 不只是读配置：它会求值 Match exec 与部分 ProxyCommand 等指令，
// 异常配置下可能长时间阻塞甚至挂死。此前没有超时，用户只能看到卡住，
// 既没有提示也无从诊断。
const resolveTimeout = 5 * time.Second

// Resolve 通过 `ssh -G` 获取别名对应的生效配置。
//
// 相较自行解析配置文件，这样能零成本正确处理 Include、Match、
// 通配符与默认值合并，结论始终与真实 ssh 的行为一致。
//
// 注意：`ssh -G` 对不存在的别名不会报错（会把输入回显为 hostname），
// 因此调用前应先用 Config.Find 确认别名存在。
func Resolve(alias string) (Host, error) {
	return resolveWithTimeout(alias, resolveTimeout)
}

// resolveWithTimeout 是 Resolve 的实现，超时时间可注入以便测试。
func resolveWithTimeout(alias string, timeout time.Duration) (Host, error) {
	exe, err := SSHBin()
	if err != nil {
		return Host{}, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, exe, "-G", alias)
	cmd.Stderr = io.Discard

	out, err := cmd.Output()
	if err != nil {
		// 超时单独报错：默认的 "signal: killed" 对用户毫无信息量。
		if ctx.Err() == context.DeadlineExceeded {
			return Host{}, fmt.Errorf(
				"ssh -G %s 超过 %s 未返回，已中止（配置中可能有异常的 Match exec 或 ProxyCommand）",
				alias, timeout)
		}
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
