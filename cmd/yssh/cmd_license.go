// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"

	"github.com/Baobug/YunSSH"
)

// cmdLicense 打印版权信息与第三方组件清单。
//
// 许可全文是以嵌入方式随二进制分发的，因此单独拷走一个 exe 也随时能调出来，
// 不依赖仓库或安装目录里是否还留着许可文件。
func cmdLicense(args []string) int {
	full := false
	for _, arg := range args {
		switch arg {
		case "--full", "-f":
			full = true
		case "help", "-h", "--help":
			fmt.Print(licenseHelp)
			return exitOK
		default:
			errf("未知选项 %q，用 yssh license --help 查看用法", arg)
			return exitUsage
		}
	}

	if full {
		fmt.Print(yunssh.License)
		fmt.Println()
		fmt.Print(yunssh.ThirdPartyNotices)
		return exitOK
	}

	fmt.Printf(`YunSSH %s

Copyright 2026 Zhou Tianbao (Baobug)
Licensed under the Apache License, Version 2.0
https://www.apache.org/licenses/LICENSE-2.0

本程序静态链接了以下第三方组件，其代码已被编入本可执行文件：

  fyne.io/systray            Apache-2.0    Copyright 2014 Brave New Software Project, Inc.
  golang.org/x/sys           BSD-3-Clause  Copyright 2009 The Go Authors.
  github.com/godbus/dbus/v5  BSD-2-Clause  Copyright (c) 2013, Georg Reinke, Google

许可全文与免责声明随程序分发，用 yssh license --full 打印。
`, yunssh.Version)

	return exitOK
}

const licenseHelp = `用法: yssh license [选项]

  yssh license          打印版权信息与第三方组件清单
  yssh license --full   打印 LICENSE 与 THIRD-PARTY-NOTICES.md 全文

说明:
  两份许可文本以嵌入方式随二进制分发，因此即使只拿到一个 exe，
  也能随时查看完整的版权声明与免责条款。
`
