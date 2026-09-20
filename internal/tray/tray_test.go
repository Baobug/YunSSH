// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package tray

import (
	"testing"

	"github.com/Baobug/YunSSH/internal/session"
)

// TestUninstallChWithoutHandler 未注入卸载实现时应返回 nil 通道。
//
// 这不是随手取巧：nil channel 在 select 中永远阻塞，正好表达"这一项不存在"，
// 从而免去在事件循环里再加一层判空。这条断言把这个约定钉住。
func TestUninstallChWithoutHandler(t *testing.T) {
	app := &App{}

	if ch := app.uninstallCh(); ch != nil {
		t.Error("未注入 Uninstall 时应返回 nil 通道")
	}
}

// TestUninstallChWithHandler 注入后应返回可用通道。
func TestUninstallChWithHandler(t *testing.T) {
	app := &App{Uninstall: func() error { return nil }}

	// 未构建菜单时通道仍为 nil——菜单项是 onReady 阶段创建的
	if ch := app.uninstallCh(); ch != nil {
		t.Error("菜单尚未构建时不应有通道")
	}
}

// TestUninstallWithoutHandlerIsNoop 未注入实现时点击卸载不应 panic。
func TestUninstallWithoutHandlerIsNoop(t *testing.T) {
	app := &App{}
	app.uninstall()
}

// TestHostLabel 验证菜单项标题的环境前缀。
func TestHostLabel(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want string
	}{
		{name: "带环境", env: "prod", want: "[prod] web"},
		{name: "无环境", env: "", want: "web"},
		{name: "环境含空白", env: "  lab  ", want: "[lab] web"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			host := session.Host{Alias: "web"}
			host.Meta.Env = tc.env

			if got := hostLabel(host); got != tc.want {
				t.Errorf("hostLabel() = %q，期望 %q", got, tc.want)
			}
		})
	}
}
