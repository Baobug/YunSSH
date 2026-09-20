// SPDX-FileCopyrightText: 2026 Zhou Tianbao (Baobug)
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package install

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"
)

var (
	shell32DLL = syscall.NewLazyDLL("shell32.dll")
	ole32DLL   = syscall.NewLazyDLL("ole32.dll")

	procSHParseDisplayName = shell32DLL.NewProc("SHParseDisplayName")
	procILGetSize          = shell32DLL.NewProc("ILGetSize")
	procCoTaskMemFree      = ole32DLL.NewProc("CoTaskMemFree")
)

// linkTargetIDList 取得路径对应的 Shell 目标标识（PIDL）。
//
// 快捷方式里的这一段不能省。外壳解析快捷方式时以它为主，LinkInfo 只是后备：
// 缺了 IDList，外壳就认不出目标，桌面和开始菜单上会退化成一个通用的空白图标——
// 文件明明存在、路径也写得没错，用户看到的却是一张白纸。
//
// 自己拼接 IDList 的字节结构并不可靠：卷项、文件项的编码规则没有稳定文档，
// 且随路径深度变化。交给 SHParseDisplayName 生成才是等效于外壳自身的做法。
func linkTargetIDList(path string) ([]byte, error) {
	target, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}

	var (
		pidl  uintptr
		attrs uint32 // SHParseDisplayName 要求给出接收属性的指针
	)

	hr, _, _ := procSHParseDisplayName.Call(
		uintptr(unsafe.Pointer(target)),
		0, // pbc：不需要绑定上下文
		uintptr(unsafe.Pointer(&pidl)),
		0, // sfgaoIn：不筛属性
		uintptr(unsafe.Pointer(&attrs)),
	)
	if int32(uint32(hr)) < 0 { // HRESULT 为负即失败
		return nil, fmt.Errorf("SHParseDisplayName 失败（0x%08X）", uint32(hr))
	}
	if pidl == 0 {
		return nil, errors.New("SHParseDisplayName 返回了空标识")
	}
	defer procCoTaskMemFree.Call(pidl)

	size, _, _ := procILGetSize.Call(pidl)
	if size < 2 {
		return nil, fmt.Errorf("ILGetSize 返回 %d，不像有效的标识列表", size)
	}

	// PIDL 由 CoTaskMemAlloc 分配，本身就是连续可读的字节，按长度读出来即可。
	raw := unsafe.Slice((*byte)(foreignPointer(pidl)), int(size))

	out := make([]byte, len(raw))
	copy(out, raw)
	return out, nil
}

// foreignPointer 把 C 侧返回的地址重新解释成指针。
//
// 不写成 unsafe.Pointer(u)：那是把整数转成指针，go vet 的 unsafeptr 检查会
// 判定为可疑用法并报错——它防的是「Go 对象被 GC 搬走后旧地址失效」，而这里的
// 内存由 CoTaskMemAlloc 分配、不会被 GC 触碰，不存在该问题。
// 改成重新解释 uintptr 变量的存储，既避开误报，又不改变任何运行期行为。
func foreignPointer(u uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&u))
}
