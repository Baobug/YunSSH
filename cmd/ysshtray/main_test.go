// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package main

import (
	"fmt"
	"os"
	"testing"
)

// testLockName 返回一个只属于当前测试进程的锁名。
//
// 必须和真实运行中的托盘用不同的名字，否则本地开着托盘时测试会直接失败。
func testLockName(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf(`Local\YunSSH.Tray.Test.%d.%s`, os.Getpid(), t.Name())
}

// TestAcquireInstanceLock 验证第二次获取同一把锁会被拒绝。
// 这正是防止从开始菜单点击后出现第二个托盘图标的关键。
func TestAcquireInstanceLock(t *testing.T) {
	name := testLockName(t)

	release, ok := acquireInstanceLock(name)
	if !ok {
		t.Fatal("首次获取实例锁应当成功")
	}
	defer release()

	release2, ok2 := acquireInstanceLock(name)
	if ok2 {
		release2()
		t.Fatal("第二次获取同一把锁应当被拒绝，但成功了")
	}
}

// TestAcquireInstanceLockAfterRelease 验证释放后可以重新获取。
// 卸载流程会结束托盘进程，之后用户应当能再次正常启动。
func TestAcquireInstanceLockAfterRelease(t *testing.T) {
	name := testLockName(t)

	release, ok := acquireInstanceLock(name)
	if !ok {
		t.Fatal("首次获取实例锁应当成功")
	}
	release()

	release2, ok2 := acquireInstanceLock(name)
	if !ok2 {
		t.Fatal("释放之后应当能够重新获取实例锁")
	}
	release2()
}

// TestAcquireInstanceLockIndependentNames 验证不同名字互不影响。
func TestAcquireInstanceLockIndependentNames(t *testing.T) {
	releaseA, okA := acquireInstanceLock(testLockName(t) + ".A")
	if !okA {
		t.Fatal("锁 A 应当获取成功")
	}
	defer releaseA()

	releaseB, okB := acquireInstanceLock(testLockName(t) + ".B")
	if !okB {
		t.Fatal("锁 B 应当与锁 A 互不影响")
	}
	releaseB()
}
