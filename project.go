// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

// Package yunssh 汇集项目级的常量与嵌入资源。
//
// 它放在模块根目录是有意为之：go:embed 只能引用包目录内的文件，而 LICENSE
// 与 THIRD-PARTY-NOTICES.md 必须留在仓库根——GitHub 靠前者识别许可，使用者
// 靠后者看清第三方归属。放在这里就能直接嵌入，不必再维护一份需要同步的副本。
package yunssh

import _ "embed"

// Version 是构建时注入的版本号，形如 "0.2.4"。
//
// build.bat 通过
//
//	-ldflags "-X github.com/Baobug/YunSSH.Version=0.2.4"
//
// 注入；直接用 go build 时保留默认值，便于区分正式构建与临时构建。
var Version = "0.0.0-dev"

// License 是 Apache-2.0 许可全文，与仓库根的 LICENSE 逐字节一致。
//
//go:embed LICENSE
var License string

// ThirdPartyNotices 是第三方组件声明，与仓库根的 THIRD-PARTY-NOTICES.md
// 逐字节一致。
//
//go:embed THIRD-PARTY-NOTICES.md
var ThirdPartyNotices string
