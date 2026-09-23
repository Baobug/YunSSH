// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Baobug/YunSSH/internal/meta"
	"github.com/Baobug/YunSSH/internal/session"
	"github.com/Baobug/YunSSH/internal/term"
)

// addOptions 收集 `yssh add` 的输入。
type addOptions struct {
	alias    string
	target   string
	port     int
	identity string
	env      string
	tags     string
	note     string
}

// cmdAdd 添加一条主机条目。
func cmdAdd(args []string) int {
	var opts addOptions
	positional := make([]string, 0, 2)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--port":
			i++
			if i >= len(args) {
				errf("--port 缺少取值")
				return exitUsage
			}
			// 与目标串里的 :port 共用同一个校验入口，避免两处范围判断漂移。
			n, err := parsePort(args[i])
			if err != nil {
				errf("%v", err)
				return exitUsage
			}
			opts.port = n
		case "--key":
			i++
			if i >= len(args) {
				errf("--key 缺少取值")
				return exitUsage
			}
			opts.identity = args[i]
		case "--env":
			i++
			if i >= len(args) {
				errf("--env 缺少取值")
				return exitUsage
			}
			opts.env = args[i]
		case "--tags":
			i++
			if i >= len(args) {
				errf("--tags 缺少取值")
				return exitUsage
			}
			opts.tags = args[i]
		case "--note":
			i++
			if i >= len(args) {
				errf("--note 缺少取值")
				return exitUsage
			}
			opts.note = args[i]
		case "-h", "--help":
			fmt.Println("用法: yssh add <别名> <用户@主机[:端口]> [--port N] [--key 路径] [--env 环境] [--tags a,b] [--note 备注]")
			return exitOK
		default:
			if strings.HasPrefix(args[i], "-") {
				errf("未知选项 %q", args[i])
				return exitUsage
			}
			positional = append(positional, args[i])
		}
	}

	if len(positional) == 0 {
		return cmdAddInteractive(opts)
	}
	if len(positional) > 2 {
		errf("参数过多，用法: yssh add <别名> <用户@主机[:端口]>")
		return exitUsage
	}

	opts.alias = positional[0]
	if len(positional) == 2 {
		opts.target = positional[1]
	}
	return addHost(opts)
}

// cmdAddInteractive 通过问答方式收集条目信息，用于不带参数直接敲 `yssh add` 的场景。
func cmdAddInteractive(opts addOptions) int {
	r := bufio.NewReader(os.Stdin)

	if opts.alias == "" {
		opts.alias = ask(r, "别名")
	}
	if opts.target == "" {
		opts.target = ask(r, "目标 (用户@主机[:端口])")
	}
	if opts.env == "" {
		opts.env = ask(r, "环境标签 (可空)")
	}
	if opts.tags == "" {
		opts.tags = ask(r, "其他标签，逗号分隔 (可空)")
	}
	if opts.note == "" {
		opts.note = ask(r, "备注 (可空)")
	}
	return addHost(opts)
}

// ask 输出提示并读取一行输入，输入为空时返回空串。
func ask(r *bufio.Reader, prompt string) string {
	fmt.Printf("%s: ", prompt)

	line, err := r.ReadString('\n')
	if err != nil && line == "" {
		return ""
	}
	return strings.TrimSpace(line)
}

// addHost 校验并写入一条主机条目。
func addHost(opts addOptions) int {
	if opts.alias == "" {
		errf("别名不能为空")
		return exitUsage
	}
	if opts.target == "" {
		errf("目标不能为空")
		return exitUsage
	}

	user, host, port, err := parseTarget(opts.target)
	if err != nil {
		errf("%v", err)
		return exitUsage
	}
	// 显式的 --port 优先于目标串里写的端口。
	if opts.port != 0 {
		port = opts.port
	}

	cfg, err := loadConfig()
	if err != nil {
		errf("读取配置失败: %v", err)
		return exitError
	}

	h := session.Host{
		Alias:        opts.alias,
		HostName:     host,
		User:         user,
		Port:         port,
		IdentityFile: opts.identity,
		Meta: meta.Meta{
			Env:   opts.env,
			Tags:  splitList(opts.tags),
			Added: time.Now().Format("2006-01-02"),
			Note:  opts.note,
		},
	}

	if err := cfg.Add(h); err != nil {
		if errors.Is(err, session.ErrAliasExists) {
			errf("%v（如需修改请用 `yssh edit %s`）", err, opts.alias)
			return exitError
		}
		errf("写入配置失败: %v", err)
		return exitError
	}

	fmt.Printf("已添加 %s → %s\n", paint(ansiBlue, h.Alias), h.Target())
	hintf("配置路径: %s", cfg.Path)
	hintf("提示: 现在用原生 `ssh %s` 也能连通。", h.Alias)

	return exitOK
}

// cmdRemove 删除一条主机条目。删除前会自动备份原配置。
func cmdRemove(args []string) int {
	if len(args) != 1 {
		errf("用法: yssh rm <别名>")
		return exitUsage
	}
	alias := args[0]

	cfg, err := loadConfig()
	if err != nil {
		errf("读取配置失败: %v", err)
		return exitError
	}

	if _, found := cfg.Find(alias); !found {
		errf("配置中找不到别名 %q", alias)
		return exitNotFound
	}

	result, err := cfg.Remove(alias)
	if err != nil {
		switch {
		case errors.Is(err, session.ErrAliasNotFound):
			errf("%v", err)
			return exitNotFound
		case errors.Is(err, session.ErrSharedHostLine), errors.Is(err, session.ErrModified):
			errf("%v", err)
			return exitError
		default:
			errf("删除失败: %v", err)
			return exitError
		}
	}

	fmt.Printf("已删除 %s\n", paint(ansiBlue, result.Alias))
	if result.BackupAt != "" {
		hintf("备份: %s", result.BackupAt)
	}
	return exitOK
}

// cmdShow 显示某个别名的生效配置。
//
// 这里刻意使用 `ssh -G` 的结果而非文本扫描的近似值，
// 因此 Include、Match、ProxyJump 等指令的影响都会体现在输出中。
func cmdShow(args []string) int {
	if len(args) != 1 {
		errf("用法: yssh show <别名>")
		return exitUsage
	}
	alias := args[0]

	cfg, err := loadConfig()
	if err != nil {
		errf("读取配置失败: %v", err)
		return exitError
	}

	local, found := cfg.Find(alias)
	if !found {
		errf("配置中找不到别名 %q", alias)
		return exitNotFound
	}

	resolved, err := session.Resolve(alias)
	if err != nil {
		errf("解析生效配置失败: %v", err)
		return exitBackend
	}

	// `ssh -G` 对未知别名不会报错，而是把输入原样回显为 hostname。
	// 出现这种情况说明 ssh 没读到这份配置，此时输出的是它的默认值，
	// 必须明确告知用户，否则会拿着错误的地址去排查问题。
	if resolved.HostName == alias && local.HostName != "" && local.HostName != alias {
		hintf("注意: ssh 未能解析别名 %q，下方主机/用户/端口是 ssh 的默认值。", alias)
		hintf("      请确认 ssh 读取的配置文件与 %s 是同一份。", cfg.Path)
	}

	fields := []struct{ label, value string }{
		{"别名", local.Alias},
		{"主机", resolved.HostName},
		{"用户", resolved.User},
		{"端口", strconv.Itoa(resolved.Port)},
		{"私钥", resolved.IdentityFile},
		{"环境", local.Meta.Env},
		{"标签", strings.Join(local.Meta.Tags, ", ")},
		{"添加于", local.Meta.Added},
		{"备注", local.Meta.Note},
	}

	for _, f := range fields {
		if f.value == "" {
			continue
		}
		fmt.Printf("%s  %s\n", paint(ansiDim, term.Pad(f.label, 8)), f.value)
	}
	return exitOK
}

// cmdEdit 用系统编辑器打开配置文件。
func cmdEdit(args []string) int {
	if len(args) > 1 {
		errf("用法: yssh edit [别名]")
		return exitUsage
	}

	cfg, err := loadConfig()
	if err != nil {
		errf("读取配置失败: %v", err)
		return exitError
	}

	// 文件不存在时先创建，避免编辑器打开一个不存在的路径。
	if !cfg.Exists() {
		if err := os.MkdirAll(filepath.Dir(cfg.Path), 0o700); err != nil {
			errf("创建配置目录失败: %v", err)
			return exitError
		}
		seed := "# ~/.ssh/config\n" +
			"# 由 yssh 创建。可直接手工编辑，yssh 不会重排或改写本文件已有内容。\n"
		if err := os.WriteFile(cfg.Path, []byte(seed), 0o600); err != nil {
			errf("创建配置文件失败: %v", err)
			return exitError
		}
		fmt.Printf("已创建 %s\n", cfg.Path)
	}

	editor, extra, err := pickEditor()
	if err != nil {
		errf("未找到可用的编辑器，请设置 $EDITOR（例如 export EDITOR=nano）。")
		hintf("配置文件：%s", cfg.Path)
		return exitError
	}

	cmd := exec.Command(editor, append(append([]string{}, extra...), cfg.Path)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		errf("启动编辑器 %q 失败: %v", editor, err)
		return exitError
	}

	if len(args) == 1 {
		hintf("提示: 别名 %q 位于 %s", args[0], cfg.Path)
	}
	return exitOK
}

// pickEditor 挑出一个可用的编辑器，返回可执行文件路径与其前置参数。
//
// 用同一份候选表覆盖两个平台，不做 build tag：Windows 上 notepad 必然命中，
// Linux 上 notepad 探测失败后自然落到 nano / vi。notepad 放最前而非最后，
// 是为了让 Windows 的行为与改动前逐字一致——若放到末尾，装了 Git for Windows
// 的机器会先命中 vi，反而改变了现有行为。
//
// $VISUAL 面向全屏编辑器，按惯例优先于 $EDITOR；两者都可能带参数
// （如 EDITOR="code --wait"），因此按空格拆分，首个 token 才是可执行文件。
//
// 探测失败的候选直接跳过下一个，保证 yssh edit 总能打开。
func pickEditor() (string, []string, error) {
	var candidates [][]string
	for _, env := range []string{os.Getenv("VISUAL"), os.Getenv("EDITOR")} {
		if fields := strings.Fields(env); len(fields) > 0 {
			candidates = append(candidates, fields)
		}
	}
	for _, fallback := range []string{"notepad", "sensible-editor", "editor", "nano", "vi"} {
		candidates = append(candidates, []string{fallback})
	}

	for _, c := range candidates {
		if path, err := exec.LookPath(c[0]); err == nil {
			return path, c[1:], nil
		}
	}
	return "", nil, errors.New("未找到可用的编辑器")
}

// parseTarget 解析 "用户@主机[:端口]" 形式的目标描述。
//
// IPv6 字面量必须带方括号，如 user@[2001:db8::1]:22；裸 IPv6（多个冒号且无
// 括号）无法可靠地区分地址与端口，会明确报错而不是静默切错。
func parseTarget(s string) (user, host string, port int, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", "", 0, errors.New("目标不能为空")
	}

	rest := s
	if u, r, found := strings.Cut(s, "@"); found {
		user, rest = u, r
	}

	host, port, err = splitHostPort(rest)
	if err != nil {
		return "", "", 0, err
	}
	if host == "" {
		return "", "", 0, fmt.Errorf("无法从 %q 解析出主机地址", s)
	}
	return user, host, port, nil
}

// splitHostPort 从主机部分拆出端口，正确处理 IPv6 字面量。
func splitHostPort(rest string) (host string, port int, err error) {
	// 带方括号的 IPv6：[::1] 或 [::1]:22
	if strings.HasPrefix(rest, "[") {
		end := strings.Index(rest, "]")
		if end < 0 {
			return "", 0, fmt.Errorf("IPv6 地址缺少右括号 ]: %q", rest)
		}
		// config 的 HostName 用裸 IPv6，方括号只是命令行/URI 的表示法
		host = rest[1:end]
		switch suffix := rest[end+1:]; {
		case suffix == "":
			return host, 0, nil
		case strings.HasPrefix(suffix, ":"):
			n, err := parsePort(suffix[1:])
			if err != nil {
				return "", 0, err
			}
			return host, n, nil
		default:
			return "", 0, fmt.Errorf("方括号后只能是端口，形如 [::1]:22: %q", rest)
		}
	}

	// 无括号却出现多个冒号，只能是裸 IPv6：无法可靠切分端口，要求加方括号。
	if strings.Count(rest, ":") > 1 {
		return "", 0, fmt.Errorf("IPv6 地址请加方括号，如 [2001:db8::1]:22: %q", rest)
	}

	// 恰好一个冒号：host:port
	if idx := strings.Index(rest, ":"); idx >= 0 {
		n, err := parsePort(rest[idx+1:])
		if err != nil {
			return "", 0, err
		}
		return rest[:idx], n, nil
	}

	// 无冒号：纯主机
	return rest, 0, nil
}

// parsePort 解析并校验端口范围。
func parsePort(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > 65535 {
		return 0, fmt.Errorf("端口无效: %q（应在 1..65535）", s)
	}
	return n, nil
}

// splitList 把逗号分隔的字符串切分为列表，忽略空白项。
func splitList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	out := make([]string, 0, 4)
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
