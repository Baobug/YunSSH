// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Baobug/YunSSH"
)

// TestWriteLegalFiles 验证许可文件确实被写到安装目录，且内容与嵌入文本逐字节一致。
//
// 这一步很容易被忽略：许可文本嵌在二进制里，写不出来也不会有别的症状，
// 但分发出去的安装目录就少了两份法律要求的文件——正是本测试要钉住的东西。
func TestWriteLegalFiles(t *testing.T) {
	dir := t.TempDir()

	got, err := writeLegalFiles(dir)
	if err != nil {
		t.Fatalf("writeLegalFiles 失败: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("写出 %d 个文件，期望 2 个：%v", len(got), got)
	}

	for _, want := range []struct{ name, body string }{
		{legalLicense, yunssh.License},
		{legalNotices, yunssh.ThirdPartyNotices},
	} {
		data, err := os.ReadFile(filepath.Join(dir, want.name))
		if err != nil {
			t.Errorf("读取 %s 失败: %v", want.name, err)
			continue
		}
		if string(data) != want.body {
			t.Errorf("%s 的内容与嵌入文本不一致", want.name)
		}
	}
}

// TestEmbeddedLegalContent 校验嵌入的许可文本本身没被改坏。
//
// 这两段文本是二进制分发时唯一的版权凭据，而它们被嵌进 exe 之后，少了哪一行
// 外面完全看不出来。所以这里把关键内容钉死：著作权人署名、Apache 正文、
// 三个第三方组件的归属。
//
// 署名写全名是有意为之——它同时也是拼写的守卫，改错了这里会红。
func TestEmbeddedLegalContent(t *testing.T) {
	if !strings.Contains(yunssh.License, "Apache License") {
		t.Error("LICENSE 中缺少 Apache License 正文")
	}
	if !strings.Contains(yunssh.License, "Copyright 2026 Zhou Tianbao (Baobug)") {
		t.Error("LICENSE 中缺少著作权人署名，或署名拼写已改动")
	}

	for _, dep := range []string{
		"fyne.io/systray",
		"golang.org/x/sys",
		"github.com/godbus/dbus/v5",
	} {
		if !strings.Contains(yunssh.ThirdPartyNotices, dep) {
			t.Errorf("第三方声明中缺少 %s", dep)
		}
	}
}
