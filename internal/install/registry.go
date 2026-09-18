//go:build windows

package install

import (
	"errors"
	"fmt"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

// 注册表位置。全部写在 HKCU 下，因此整个安装过程无需管理员权限。
const (
	runKeyPath       = `Software\Microsoft\Windows\CurrentVersion\Run`
	uninstallKeyPath = `Software\Microsoft\Windows\CurrentVersion\Uninstall\YunSSH`
	environmentKey   = `Environment`
	runValueName     = "YunSSH"
)

// ErrNotInstalled 表示注册表中没有本程序的安装记录。
var ErrNotInstalled = errors.New("YunSSH 尚未安装")

// setAutoStart 写入或移除开机自启项。
//
// 命令行加引号是必要的：安装路径通常含空格，不加引号会被拆成多个参数。
func setAutoStart(command string, enable bool) error {
	key, _, err := registry.CreateKey(
		registry.CURRENT_USER, runKeyPath,
		registry.SET_VALUE|registry.QUERY_VALUE,
	)
	if err != nil {
		return fmt.Errorf("打开启动项注册表失败: %w", err)
	}
	defer key.Close()

	if !enable {
		if err := key.DeleteValue(runValueName); err != nil && !errors.Is(err, registry.ErrNotExist) {
			return fmt.Errorf("移除开机自启失败: %w", err)
		}
		return nil
	}

	if err := key.SetStringValue(runValueName, quote(command)); err != nil {
		return fmt.Errorf("写入开机自启失败: %w", err)
	}
	return nil
}

// autoStartEnabled 报告开机自启项是否存在。
func autoStartEnabled() bool {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()

	_, _, err = key.GetStringValue(runValueName)
	return err == nil
}

// addUninstallEntry 在「应用和功能」中登记本程序。
//
// 登记之后控制面板里会出现标准的卸载入口，无需用户手工去删目录。
func addUninstallEntry(displayVersion, installDir, trayExe, selfExe string) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, uninstallKeyPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("创建卸载登记项失败: %w", err)
	}
	defer key.Close()

	values := map[string]string{
		"DisplayName":          "YunSSH",
		"DisplayVersion":       displayVersion,
		"Publisher":            "Baobug",
		"InstallLocation":      installDir,
		"UninstallString":      quote(selfExe) + " uninstall",
		"QuietUninstallString": quote(selfExe) + " uninstall --yes",
		"DisplayIcon":          quote(trayExe),
	}
	for name, value := range values {
		if err := key.SetStringValue(name, value); err != nil {
			return fmt.Errorf("写入卸载登记项 %s 失败: %w", name, err)
		}
	}

	// 本程序没有「修改」和「修复」的概念，隐藏这两个按钮
	if err := key.SetDWordValue("NoModify", 1); err != nil {
		return fmt.Errorf("写入 NoModify 失败: %w", err)
	}
	if err := key.SetDWordValue("NoRepair", 1); err != nil {
		return fmt.Errorf("写入 NoRepair 失败: %w", err)
	}
	return nil
}

// removeUninstallEntry 删除卸载登记项。
func removeUninstallEntry() error {
	if err := registry.DeleteKey(registry.CURRENT_USER, uninstallKeyPath); err != nil &&
		!errors.Is(err, registry.ErrNotExist) {
		return fmt.Errorf("移除卸载登记项失败: %w", err)
	}
	return nil
}

// installLocation 读取登记在册的安装目录。
func installLocation() (string, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, uninstallKeyPath, registry.QUERY_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return "", ErrNotInstalled
		}
		return "", err
	}
	defer key.Close()

	dir, _, err := key.GetStringValue("InstallLocation")
	if err != nil || strings.TrimSpace(dir) == "" {
		return "", ErrNotInstalled
	}
	return dir, nil
}

// installedVersion 读取登记在册的版本号。
func installedVersion() string {
	key, err := registry.OpenKey(registry.CURRENT_USER, uninstallKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer key.Close()

	v, _, err := key.GetStringValue("DisplayVersion")
	if err != nil {
		return ""
	}
	return v
}

// addToUserPath 把目录加入用户级 PATH。
func addToUserPath(dir string) error {
	return editUserPath(func(parts []string) ([]string, bool) {
		for _, p := range parts {
			if samePath(p, dir) {
				return parts, false
			}
		}
		return append(parts, dir), true
	})
}

// removeFromUserPath 从用户级 PATH 中移除目录。
func removeFromUserPath(dir string) error {
	return editUserPath(func(parts []string) ([]string, bool) {
		kept := make([]string, 0, len(parts))
		changed := false
		for _, p := range parts {
			if samePath(p, dir) {
				changed = true
				continue
			}
			kept = append(kept, p)
		}
		return kept, changed
	})
}

// editUserPath 以函数式方式修改用户 PATH。
//
// 注意必须用 SetExpandStringValue：用户 PATH 里常含有 %USERPROFILE% 这类变量，
// 若写成普通字符串会丢失变量展开语义，等于悄悄改坏了别人的环境。
func editUserPath(mutate func([]string) ([]string, bool)) error {
	key, _, err := registry.CreateKey(
		registry.CURRENT_USER, environmentKey,
		registry.SET_VALUE|registry.QUERY_VALUE,
	)
	if err != nil {
		return fmt.Errorf("打开环境变量注册表失败: %w", err)
	}
	defer key.Close()

	current, _, err := key.GetStringValue("Path")
	if err != nil && !errors.Is(err, registry.ErrNotExist) {
		return fmt.Errorf("读取用户 PATH 失败: %w", err)
	}

	updated, changed := mutate(splitPath(current))
	if !changed {
		return nil
	}

	if err := key.SetExpandStringValue("Path", strings.Join(updated, ";")); err != nil {
		return fmt.Errorf("写入用户 PATH 失败: %w", err)
	}
	return nil
}

// splitPath 切分 PATH，丢弃空项。
func splitPath(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ";") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// samePath 比较两个路径是否指向同一位置，忽略大小写与末尾分隔符。
func samePath(a, b string) bool {
	norm := func(s string) string {
		s = strings.TrimSpace(s)
		s = strings.Trim(s, `"`)
		return strings.ToLower(strings.TrimRight(s, `\/`))
	}
	return norm(a) == norm(b)
}

// quote 在必要时为命令行加引号。
func quote(s string) string {
	if strings.ContainsAny(s, " \t") && !strings.HasPrefix(s, `"`) {
		return `"` + s + `"`
	}
	return s
}

// broadcastEnvChange 广播环境变量已变更的消息。
//
// 不广播的话，已经运行的资源管理器不会刷新环境块，
// 用户新开的终端仍读不到更新后的 PATH，必须重启或注销才生效。
func broadcastEnvChange() {
	const (
		hwndBroadcast    = 0xffff
		wmSettingChange  = 0x001A
		smtoAbortIfHung  = 0x0002
		broadcastTimeout = 3000
	)

	user32 := syscall.NewLazyDLL("user32.dll")
	sendMessageTimeout := user32.NewProc("SendMessageTimeoutW")

	param, err := syscall.UTF16PtrFromString("Environment")
	if err != nil {
		return
	}

	_, _, _ = sendMessageTimeout.Call(
		hwndBroadcast,
		wmSettingChange,
		0,
		uintptr(unsafe.Pointer(param)),
		smtoAbortIfHung,
		broadcastTimeout,
		0,
	)
}
