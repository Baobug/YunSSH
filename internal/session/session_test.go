package session

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Baobug/YunSSH/internal/meta"
)

// TestMain 把 home 指向临时目录，避免测试写入真实的 ~/.yssh（备份目录就在这里）。
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "yssh-test-home-*")
	if err != nil {
		panic(err)
	}
	os.Setenv("USERPROFILE", dir) // Windows
	os.Setenv("HOME", dir)        // Unix

	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// userConfig 模拟一份用户手写的配置，含自定义注释、多别名行和通配符条目。
// 所有写入测试都以它为基准，验证「不破坏用户内容」这一核心不变量。
const userConfig = `# 我的服务器清单
# 手工维护，请勿重排

Host prod-web
    HostName 1.2.3.4
    User root
    Port 2222
    # 生产环境，动手前想清楚
    IdentityFile ~/.ssh/id_ed25519

Host *.internal
    User admin

# yssh: env=lab tags=kali added=2026-09-18
Host msf
    HostName 192.168.78.129
    User msfadmin
`

// newConfig 在临时目录写入一份配置并载入。
func newConfig(t *testing.T, content string) (*Config, string) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("写入测试配置失败: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("载入测试配置失败: %v", err)
	}
	return cfg, path
}

// readConfig 读取磁盘上的配置内容。
func readConfig(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取配置失败: %v", err)
	}
	return string(data)
}

// findHost 按别名查找。
func findHost(hosts []Host, alias string) (Host, bool) {
	for _, h := range hosts {
		if h.Alias == alias {
			return h, true
		}
	}
	return Host{}, false
}

// aliasesOf 提取别名列表，便于断言失败时输出。
func aliasesOf(hosts []Host) []string {
	out := make([]string, 0, len(hosts))
	for _, h := range hosts {
		out = append(out, h.Alias)
	}
	return out
}

func TestListParsesFields(t *testing.T) {
	cfg, _ := newConfig(t, userConfig)

	hosts := cfg.List()
	if len(hosts) != 2 {
		t.Fatalf("应解析出 2 个可连接别名（通配符条目应被跳过），得到 %v", aliasesOf(hosts))
	}

	prod, ok := findHost(hosts, "prod-web")
	if !ok {
		t.Fatal("未找到 prod-web")
	}
	if prod.HostName != "1.2.3.4" {
		t.Errorf("HostName = %q，期望 1.2.3.4", prod.HostName)
	}
	if prod.User != "root" {
		t.Errorf("User = %q，期望 root", prod.User)
	}
	if prod.Port != 2222 {
		t.Errorf("Port = %d，期望 2222", prod.Port)
	}
	if prod.IdentityFile != "~/.ssh/id_ed25519" {
		t.Errorf("IdentityFile = %q", prod.IdentityFile)
	}
	if !prod.Approximate {
		t.Error("文本扫描的结果应标记为近似值")
	}
	if prod.HasMeta {
		t.Error("prod-web 没有元数据注释，HasMeta 应为 false")
	}

	msf, ok := findHost(hosts, "msf")
	if !ok {
		t.Fatal("未找到 msf")
	}
	if msf.Meta.Env != "lab" {
		t.Errorf("Env = %q，期望 lab", msf.Meta.Env)
	}
	if len(msf.Meta.Tags) != 1 || msf.Meta.Tags[0] != "kali" {
		t.Errorf("Tags = %v，期望 [kali]", msf.Meta.Tags)
	}
	if !msf.HasMeta {
		t.Error("msf 有元数据注释，HasMeta 应为 true")
	}
}

// TestAliasesSkipsWildcards 验证通配符条目不会出现在可连接别名中。
func TestAliasesSkipsWildcards(t *testing.T) {
	cfg, _ := newConfig(t, userConfig)

	for _, a := range cfg.Aliases() {
		if strings.ContainsAny(a, "*?!") {
			t.Errorf("通配符条目 %q 不应作为可连接别名", a)
		}
	}
}

// TestAddPreservesUserContent 是本项目最重要的断言：
// 追加写入后，用户原有内容必须逐字节保持不变。
func TestAddPreservesUserContent(t *testing.T) {
	cfg, path := newConfig(t, userConfig)

	err := cfg.Add(Host{
		Alias:    "new-box",
		HostName: "10.0.0.9",
		User:     "ubuntu",
		Port:     2222,
		Meta:     meta.Meta{Env: "test", Tags: []string{"new"}, Added: "2026-09-18"},
	})
	if err != nil {
		t.Fatalf("添加失败: %v", err)
	}

	got := readConfig(t, path)

	if !strings.HasPrefix(got, userConfig) {
		t.Errorf("原有内容被改动。\n期望前缀:\n%s\n实际:\n%s", userConfig, got)
	}
	for _, want := range []string{
		"Host new-box",
		"HostName 10.0.0.9",
		"User ubuntu",
		"Port 2222",
		"# yssh: env=test tags=new added=2026-09-18",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("新增内容缺少 %q\n实际:\n%s", want, got)
		}
	}

	// 重新载入后应能解析出新条目。
	reloaded, err := Load(path)
	if err != nil {
		t.Fatalf("重新载入失败: %v", err)
	}
	h, ok := findHost(reloaded.List(), "new-box")
	if !ok {
		t.Fatal("重新载入后未找到 new-box")
	}
	if h.Meta.Env != "test" {
		t.Errorf("元数据未正确往返: Env = %q", h.Meta.Env)
	}
}

// TestAddRejectsDuplicate 验证重复别名被拒绝，且文件不被改动。
func TestAddRejectsDuplicate(t *testing.T) {
	cfg, path := newConfig(t, userConfig)
	before := readConfig(t, path)

	err := cfg.Add(Host{Alias: "msf", HostName: "9.9.9.9"})
	if !errors.Is(err, ErrAliasExists) {
		t.Fatalf("应返回 ErrAliasExists，得到 %v", err)
	}
	if got := readConfig(t, path); got != before {
		t.Errorf("失败的添加不应改动文件\n实际:\n%s", got)
	}
}

// TestRemovePreservesOthers 验证删除只影响目标块，且元数据注释一并移除。
func TestRemovePreservesOthers(t *testing.T) {
	cfg, path := newConfig(t, userConfig)

	result, err := cfg.Remove("msf")
	if err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if result.BackupAt == "" {
		t.Error("删除操作应产生备份")
	}
	if _, err := os.Stat(result.BackupAt); err != nil {
		t.Errorf("备份文件不可读: %v", err)
	}

	got := readConfig(t, path)

	if strings.Contains(got, "Host msf") {
		t.Errorf("msf 块未被删除:\n%s", got)
	}
	// 元数据注释必须随块一起删除，不能留下孤儿记录。
	if strings.Contains(got, "tags=kali") {
		t.Errorf("元数据注释残留:\n%s", got)
	}
	for _, want := range []string{
		"# 我的服务器清单",
		"Host prod-web",
		"HostName 1.2.3.4",
		"# 生产环境，动手前想清楚",
		"Host *.internal",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("删除误伤了 %q\n实际:\n%s", want, got)
		}
	}
}

// TestRemoveRejectsSharedHostLine 验证多别名共享一行时拒绝删除，避免误伤兄弟别名。
func TestRemoveRejectsSharedHostLine(t *testing.T) {
	const shared = `Host alpha beta
    HostName 1.2.3.4
    User root
`
	cfg, path := newConfig(t, shared)
	before := readConfig(t, path)

	_, err := cfg.Remove("alpha")
	if !errors.Is(err, ErrSharedHostLine) {
		t.Fatalf("应返回 ErrSharedHostLine，得到 %v", err)
	}
	if got := readConfig(t, path); got != before {
		t.Error("被拒绝的删除不应改动文件")
	}
}

func TestRemoveMissingAlias(t *testing.T) {
	cfg, _ := newConfig(t, userConfig)

	_, err := cfg.Remove("no-such-host")
	if !errors.Is(err, ErrAliasNotFound) {
		t.Fatalf("应返回 ErrAliasNotFound，得到 %v", err)
	}
}

// TestSaveDetectsExternalModification 验证并发修改被拦截，避免覆盖用户或其他编辑器的改动。
func TestSaveDetectsExternalModification(t *testing.T) {
	cfg, path := newConfig(t, userConfig)

	// 显式调整 mtime，避免依赖文件系统的时钟精度。
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatalf("调整 mtime 失败: %v", err)
	}

	err := cfg.Add(Host{Alias: "another", HostName: "10.0.0.1"})
	if !errors.Is(err, ErrModified) {
		t.Fatalf("应检测到外部修改并返回 ErrModified，得到 %v", err)
	}
}

// TestCRLFPreserved 验证 CRLF 换行风格在读写往返后保持不变。
func TestCRLFPreserved(t *testing.T) {
	const crlf = "# 注释\r\nHost a\r\n    HostName 1.2.3.4\r\n"
	cfg, path := newConfig(t, crlf)

	if err := cfg.Add(Host{Alias: "b", HostName: "5.6.7.8"}); err != nil {
		t.Fatalf("添加失败: %v", err)
	}

	got := readConfig(t, path)
	if !strings.HasPrefix(got, crlf) {
		t.Errorf("CRLF 内容被破坏:\n%q", got)
	}
	if !strings.Contains(got, "Host b\r\n") {
		t.Errorf("新增行应沿用 CRLF 风格:\n%q", got)
	}
}

// TestLoadMissingFile 验证首次使用时（配置文件尚不存在）的行为。
func TestLoadMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("缺失文件不应报错: %v", err)
	}
	if cfg.Exists() {
		t.Error("Exists() 应为 false")
	}
	if len(cfg.List()) != 0 {
		t.Error("应返回空主机列表")
	}

	// 应能直接向空配置添加第一条。
	if err := cfg.Add(Host{Alias: "first", HostName: "1.1.1.1", User: "root"}); err != nil {
		t.Fatalf("向空配置添加失败: %v", err)
	}

	got := readConfig(t, path)
	for _, want := range []string{"Host first", "HostName 1.1.1.1", "User root"} {
		if !strings.Contains(got, want) {
			t.Errorf("缺少 %q\n实际:\n%s", want, got)
		}
	}
	// 文件不应以空行开头。
	if strings.HasPrefix(got, "\n") {
		t.Errorf("文件不应以空行开头:\n%q", got)
	}
}

func TestLoadEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("创建空文件失败: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("空文件不应报错: %v", err)
	}
	if len(cfg.List()) != 0 {
		t.Error("空文件应返回空列表")
	}
	if err := cfg.Add(Host{Alias: "a", HostName: "1.1.1.1"}); err != nil {
		t.Fatalf("向空文件添加失败: %v", err)
	}
	if got := readConfig(t, path); strings.HasPrefix(got, "\n") {
		t.Errorf("文件不应以空行开头:\n%q", got)
	}
}

func TestAddRejectsInvalidAlias(t *testing.T) {
	cfg, _ := newConfig(t, "")

	for _, alias := range []string{"has space", "has*wild", "has?mark", "a=b"} {
		if err := cfg.Add(Host{Alias: alias}); err == nil {
			t.Errorf("别名 %q 应被拒绝", alias)
		}
	}
}

func TestTarget(t *testing.T) {
	tests := []struct {
		name string
		host Host
		want string
	}{
		{"完整", Host{Alias: "a", HostName: "1.2.3.4", User: "root", Port: 2222}, "root@1.2.3.4:2222"},
		{"无端口", Host{Alias: "a", HostName: "1.2.3.4", User: "root"}, "root@1.2.3.4"},
		{"无用户", Host{Alias: "a", HostName: "1.2.3.4"}, "1.2.3.4"},
		{"全部缺失时回落到别名", Host{Alias: "a"}, "a"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.host.Target(); got != tc.want {
				t.Errorf("Target() = %q，期望 %q", got, tc.want)
			}
		})
	}
}

func TestParseHostLine(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		want   []string
		wantOK bool
	}{
		{name: "单别名", line: "Host web", want: []string{"web"}, wantOK: true},
		{name: "多别名", line: "Host a b c", want: []string{"a", "b", "c"}, wantOK: true},
		{name: "等号写法", line: "Host=web", want: []string{"web"}, wantOK: true},
		{name: "大小写不敏感", line: "host web", want: []string{"web"}, wantOK: true},
		{name: "缩进", line: "    Host web", want: []string{"web"}, wantOK: true},
		{name: "过滤通配符", line: "Host a *.internal", want: []string{"a"}, wantOK: true},
		{name: "纯通配符", line: "Host *", wantOK: false},
		{name: "注释行", line: "# Host web", wantOK: false},
		{name: "其他关键字", line: "HostName 1.2.3.4", wantOK: false},
		{name: "空行", line: "", wantOK: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseHostLine(tc.line)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v，期望 %v", ok, tc.wantOK)
			}
			if !tc.wantOK {
				return
			}
			if len(got) != len(tc.want) {
				t.Fatalf("别名 = %v，期望 %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("别名 = %v，期望 %v", got, tc.want)
					break
				}
			}
		})
	}
}

// TestSplitKeyword 验证配置行切分，同时兼容空格与等号两种写法。
func TestSplitKeyword(t *testing.T) {
	tests := []struct {
		line      string
		key, want string
		ok        bool
	}{
		{"HostName 1.2.3.4", "HostName", "1.2.3.4", true},
		{"HostName=1.2.3.4", "HostName", "1.2.3.4", true},
		{"IdentityFile ~/.ssh/id_ed25519", "IdentityFile", "~/.ssh/id_ed25519", true},
		{"Bare", "", "", false},
	}

	for _, tc := range tests {
		key, value, ok := splitKeyword(tc.line)
		if ok != tc.ok {
			t.Errorf("splitKeyword(%q) ok = %v，期望 %v", tc.line, ok, tc.ok)
			continue
		}
		if !ok {
			continue
		}
		if key != tc.key || value != tc.want {
			t.Errorf("splitKeyword(%q) = (%q, %q)，期望 (%q, %q)", tc.line, key, value, tc.key, tc.want)
		}
	}
}
