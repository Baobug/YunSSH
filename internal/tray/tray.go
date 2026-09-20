//go:build windows

package tray

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"fyne.io/systray"

	"github.com/Baobug/YunSSH/internal/appicon"
	"github.com/Baobug/YunSSH/internal/dialog"
	"github.com/Baobug/YunSSH/internal/launcher"
	"github.com/Baobug/YunSSH/internal/session"
)

// App 是托盘应用。
//
// 所有外部依赖都以函数字段注入，托盘逻辑本身不关心配置从哪里读、
// 自启怎么设置。这样它既能被 cmd/ysshtray 使用，也能在测试中替换。
type App struct {
	// Hosts 返回要展示在菜单里的主机。
	Hosts func() ([]session.Host, error)
	// OpenConfig 打开配置文件。
	OpenConfig func() error
	// OpenSearch 在终端里打开完整的主机列表（等价于直接运行 yssh）。
	OpenSearch func() error
	// AutoStartEnabled 报告当前是否已设置开机自启。
	AutoStartEnabled func() bool
	// SetAutoStart 切换开机自启。
	SetAutoStart func(bool) error
	// Terminal 选择承载连接的终端，留空则用 Windows Terminal。
	Terminal launcher.Terminal

	// ErrLog 记录非致命错误，留空则弹窗提示。
	ErrLog func(error)

	mu    sync.Mutex
	done  chan struct{}
	items []*systray.MenuItem

	mHosts     *systray.MenuItem
	mSearch    *systray.MenuItem
	mEdit      *systray.MenuItem
	mRefresh   *systray.MenuItem
	mAutoStart *systray.MenuItem
	mQuit      *systray.MenuItem
}

// Run 启动托盘消息循环，阻塞直到退出。
func (a *App) Run() {
	systray.Run(a.onReady, func() {})
}

// onReady 在托盘就绪后构建菜单。
//
// 菜单结构：
//
//	主机 ▸            （子菜单，动态；点击父项等价于运行 yssh）
//	  [prod] web
//	  [lab]  msf
//	────────────
//	打开配置文件
//	刷新列表
//	────────────
//	开机自启          （可勾选）
//	────────────
//	关于 / 退出
func (a *App) onReady() {
	systray.SetIcon(appicon.ICO())
	systray.SetTooltip("YunSSH — SSH 会话管理")

	// 主机放在子菜单里：刷新时只需重建子菜单内容，静态项的位置不受影响。
	//
	// 注意父项不能调用 Disable()——Windows 原生菜单里被禁用的项无法展开子菜单，
	// 所以这里给父项一个有意义的动作：点击即在终端打开完整列表。
	a.mHosts = systray.AddMenuItem("主机", "点击在终端里打开完整列表")

	a.rebuildHosts()

	systray.AddSeparator()
	a.mSearch = systray.AddMenuItem("搜索主机…", "在终端里打开主机列表")
	a.mEdit = systray.AddMenuItem("打开配置文件", "编辑 ~/.ssh/config")
	a.mRefresh = systray.AddMenuItem("刷新列表", "重新读取配置文件")

	systray.AddSeparator()
	a.mAutoStart = systray.AddMenuItemCheckbox("开机自启", "登录时自动启动 YunSSH", a.autoStartOn())

	systray.AddSeparator()
	mAbout := systray.AddMenuItem("关于 YunSSH", "")
	a.mQuit = systray.AddMenuItem("退出", "退出 YunSSH")

	go a.loop(mAbout)
}

// loop 分发静态菜单项的点击事件。
func (a *App) loop(mAbout *systray.MenuItem) {
	for {
		select {
		case <-a.mHosts.ClickedCh:
			a.callSafely("打开主机列表", func() error {
				if a.OpenSearch == nil {
					return nil
				}
				return a.OpenSearch()
			})

		case <-a.mSearch.ClickedCh:
			a.callSafely("打开主机列表", func() error {
				if a.OpenSearch == nil {
					return nil
				}
				return a.OpenSearch()
			})

		case <-a.mEdit.ClickedCh:
			a.callSafely("打开配置文件", func() error {
				if a.OpenConfig == nil {
					return nil
				}
				return a.OpenConfig()
			})

		case <-a.mRefresh.ClickedCh:
			a.rebuildHosts()

		case <-a.mAutoStart.ClickedCh:
			a.toggleAutoStart()

		case <-mAbout.ClickedCh:
			dialog.Info("YunSSH",
				"SSH 会话管理工具\n\n"+
					"会话信息保存在 ~/.ssh/config，\n"+
					"与原生 ssh、scp、git、VSCode Remote 共用同一份配置。")

		case <-a.mQuit.ClickedCh:
			systray.Quit()
			return
		}
	}
}

// rebuildHosts 重建「主机」子菜单。
//
// 每次重建会关闭上一代的 done channel，让旧的监听 goroutine 退出，
// 避免刷新多次后累积泄漏。
func (a *App) rebuildHosts() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.mHosts == nil {
		return
	}

	if a.done != nil {
		close(a.done)
	}
	a.done = make(chan struct{})
	done := a.done

	for _, item := range a.items {
		item.Remove()
	}
	a.items = nil

	var hosts []session.Host
	if a.Hosts != nil {
		got, err := a.Hosts()
		if err != nil {
			a.report(fmt.Errorf("读取配置失败: %w", err))
		} else {
			hosts = got
		}
	}

	if len(hosts) == 0 {
		item := a.mHosts.AddSubMenuItem("（暂无主机）", "用 yssh add 添加")
		item.Disable()
		a.items = append(a.items, item)
		return
	}

	sortHosts(hosts)

	for _, h := range hosts {
		item := a.mHosts.AddSubMenuItem(hostLabel(h), h.Target())
		a.items = append(a.items, item)
		a.watchConnect(item, h.Alias, done)
	}
}

// watchConnect 监听某个主机项的点击，并在菜单重建后自动退出。
func (a *App) watchConnect(item *systray.MenuItem, alias string, done <-chan struct{}) {
	go func() {
		for {
			select {
			case <-done:
				return
			case <-item.ClickedCh:
				a.connect(alias)
			}
		}
	}()
}

// connect 用选定的终端发起连接。
func (a *App) connect(alias string) {
	term := a.Terminal
	if term == "" {
		term = launcher.Wt
	}

	err := launcher.Start(launcher.Options{Alias: alias, Terminal: term})
	if err == nil {
		return
	}

	// Windows Terminal 不可用时回退到系统默认方式，尽量让用户仍能连上。
	if term == launcher.Wt {
		if fallbackErr := launcher.Start(launcher.Options{Alias: alias, Terminal: launcher.Default}); fallbackErr == nil {
			return
		}
	}

	a.report(fmt.Errorf("启动连接 %s 失败: %w", alias, err))
}

// toggleAutoStart 反转开机自启状态。
func (a *App) toggleAutoStart() {
	if a.SetAutoStart == nil {
		return
	}

	want := !a.autoStartOn()
	if err := a.SetAutoStart(want); err != nil {
		a.report(fmt.Errorf("设置开机自启失败: %w", err))
		return
	}

	if want {
		a.mAutoStart.Check()
	} else {
		a.mAutoStart.Uncheck()
	}
}

// autoStartOn 报告当前的开机自启状态。
func (a *App) autoStartOn() bool {
	if a.AutoStartEnabled == nil {
		return false
	}
	return a.AutoStartEnabled()
}

// callSafely 执行一个可能失败的动作并统一处理错误。
func (a *App) callSafely(action string, fn func() error) {
	if err := fn(); err != nil {
		a.report(fmt.Errorf("%s 失败: %w", action, err))
	}
}

// report 上报一个错误。
func (a *App) report(err error) {
	if err == nil {
		return
	}
	if a.ErrLog != nil {
		a.ErrLog(err)
		return
	}
	dialog.Error("YunSSH", err.Error())
}

// hostLabel 生成菜单项标题。
//
// 带环境前缀是有意为之：菜单里不展开子菜单也能一眼分辨生产与靶机，
// 降低连错机器的概率。
func hostLabel(h session.Host) string {
	if env := strings.TrimSpace(h.Meta.Env); env != "" {
		return "[" + env + "] " + h.Alias
	}
	return h.Alias
}

// sortHosts 按「环境 → 别名」排序，使同环境的条目相邻。
// 未标注环境的条目排在最后。
func sortHosts(hosts []session.Host) {
	sort.SliceStable(hosts, func(i, j int) bool {
		ei := strings.ToLower(strings.TrimSpace(hosts[i].Meta.Env))
		ej := strings.ToLower(strings.TrimSpace(hosts[j].Meta.Env))

		if ei != ej {
			switch {
			case ei == "":
				return false
			case ej == "":
				return true
			default:
				return ei < ej
			}
		}
		return hosts[i].Alias < hosts[j].Alias
	})
}
