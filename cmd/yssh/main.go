// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

// Command yssh 是 SSH 会话管理工具。Windows 上另有常驻托盘（ysshtray.exe）。
//
// 它不替代 ssh，而是作为加速录入层：会话信息存放在标准的 ~/.ssh/config 中，
// yssh 只负责快速检索与增删条目，然后把终端完整交给底层 ssh 客户端。
//
// 因此通过 yssh 写入的主机，对原生 ssh、scp、rsync、git、VSCode Remote 全部可见。
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/Baobug/YunSSH"
	"github.com/Baobug/YunSSH/internal/session"
	"github.com/Baobug/YunSSH/internal/term"
)

// 版本号取自根包的 yunssh.Version，由 build.bat 用 -ldflags -X 注入。
//
// 这里不再另放一份常量：版本号同时写进 exe 的版本资源（见 internal/appicon）
// 与本命令的输出，两处必须一致，单一来源才不会漂移。
// 顺带一个副作用是必要的——cmd/yssh 必须引用根包，链接器才会把它链进来，
// 否则 -X 会静默失效，版本号永远停在默认值。

// 退出码约定。参见 docs/DESIGN.md 第 8.5 节。
const (
	exitOK         = 0 // 正常结束
	exitError      = 1 // 一般错误（参数、文件）
	exitUsage      = 1 // 用法错误
	exitNotFound   = 2 // 别名未找到或存在歧义
	exitBackend    = 3 // 连接后端不可用
	exitCredential = 4 // 凭据操作失败（预留给密码层）
)

func main() {
	term.EnableVT()
	initColors()
	os.Exit(run(os.Args[1:]))
}

// run 解析并分发命令，返回进程退出码。
func run(args []string) int {
	if len(args) == 0 {
		return cmdList(nil)
	}

	switch args[0] {
	case "ls", "list":
		return cmdList(args[1:])
	case "add":
		return cmdAdd(args[1:])
	case "rm", "remove", "delete":
		return cmdRemove(args[1:])
	case "show":
		return cmdShow(args[1:])
	case "edit":
		return cmdEdit(args[1:])
	case "install":
		return cmdInstall(args[1:])
	case "uninstall":
		return cmdUninstall(args[1:])
	case "autostart":
		return cmdAutoStart(args[1:])
	case "license":
		return cmdLicense(args[1:])
	case "run":
		if len(args) < 2 {
			errf("用法: yssh run <别名> [透传参数...]")
			return exitUsage
		}
		return cmdConnect(args[1], args[2:])
	case "help", "-h", "--help":
		printHelp()
		return exitOK
	case "version", "-v", "--version":
		fmt.Printf("yssh %s\n", yunssh.Version)
		return exitOK
	}

	if strings.HasPrefix(args[0], "-") {
		errf("未知选项 %q，用 `yssh --help` 查看用法", args[0])
		return exitUsage
	}

	// 首个参数不是子命令，按连接请求处理。
	return cmdConnect(args[0], args[1:])
}

// loadConfig 载入 ~/.ssh/config。
func loadConfig() (*session.Config, error) {
	path, err := session.DefaultPath()
	if err != nil {
		return nil, err
	}
	return session.Load(path)
}

// printHelp 输出用法说明。
func printHelp() {
	fmt.Print(`yssh - SSH 会话管理工具

用法:
  yssh                          列出全部主机
  yssh <关键词>                 模糊匹配并连接
  yssh run <别名> [参数...]     显式连接（别名与子命令重名时使用）
  yssh ls [--plain]             列出全部主机（--plain 每行一个别名，供脚本消费）
  yssh add <别名> <用户@主机>   添加主机条目（无参数时进入交互模式）
  yssh rm <别名>                删除主机条目（会先备份）
  yssh show <别名>              显示 ssh 解析出的生效配置
  yssh edit [别名]              用编辑器打开配置文件
  yssh install                  安装到当前用户目录并常驻托盘（仅 Windows）
  yssh uninstall                卸载（仅 Windows）
  yssh autostart [on|off]       查询或切换开机自启（仅 Windows）
  yssh license [--full]         查看版权信息与第三方组件许可

add 选项:
  --port <端口>     端口
  --key <路径>      私钥文件
  --env <环境>      环境标识，如 prod / test / lab
  --tags <标签>     逗号分隔的标签
  --note <备注>     备注，可含空格，必须放在最后

install 选项（仅 Windows）:
  --autostart       安装时开启开机自启（不再询问）
  --no-autostart    安装时不开启开机自启（不再询问）
  --no-path         不把安装目录加入 PATH

托盘程序:
  ysshtray.exe      常驻托盘，点击图标即可从菜单直接连接主机（仅 Windows）
  yssh              命令行工具，两者共用同一份 ~/.ssh/config

透传参数:
  第一个参数之后的内容会原样传给 ssh，例如:
    yssh prod-web -L 3306:127.0.0.1:3306
    yssh prod-web -N -L 8080:localhost:80
    yssh prod-web -- ls -la /var/log

说明:
  会话信息存放在 ~/.ssh/config，本工具不自建任何配置格式，
  因此写入的条目对原生 ssh、scp、git、VSCode Remote 全部可见。
  认证方式由你的密钥决定（本版本不保存密码）。
  Linux 的安装与 bash 补全见发布包内的 install.sh。
`)
}
