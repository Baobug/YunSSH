# YunSSH 技术设计文档

> 项目代号：YunSSH　|　命令名：`yssh`　|　目标平台：Windows 10/11（后续可扩展 Linux/macOS）
> 文档版本：v1.1　|　日期：2026-09-18　|　状态：Step 1 已实现并通过验证

---

## 1. 项目概述

YunSSH 是一个面向 Windows 终端的 SSH 会话管理工具，解决「每次连接都要重复输入 IP、端口、用户名、密码」的问题。

它不替代 `ssh`，而是作为**加速录入层**：读取标准 SSH 配置、从系统凭据库取出密码、拼装后把终端完整交给底层 SSH 客户端。

---

## 2. 背景与问题定义

### 2.1 痛点分层

| 层级 | 具体问题 | 现有解法 | 本项目是否解决 |
|---|---|---|---|
| L1 参数记忆 | 记不住 IP / 端口 / 用户名 | `~/.ssh/config`（原生，零成本） | ✅ 提供快速录入与检索 |
| L2 检索切换 | 机器多了找不到、想不起来叫啥 | `sshs` 等 TUI 工具 | ✅ 模糊搜索 |
| L3 密码输入 | 每次都要手输密码 | **Windows 上无可靠方案** | ✅ 核心价值点 |
| L4 批量运维 | 靶机/机器批量录入、批量执行 | 无便捷方案 | ⭕ Step 4 考虑 |

### 2.2 为什么 L3 是难点（已验证结论）

**Windows 原生 OpenSSH 对 `SSH_ASKPASS` 的支持不可靠。**

- 该机制仅在「ssh 进程未关联终端」时才可能生效，与交互式登录场景直接冲突。
- Win32-OpenSSH issue #2115 显示：**新版本行为比旧版本更差**。
- MITRE 的 `train-juniper` 项目实测后放弃此路线，改用 `plink.exe`。

因此，网上流传的 `SSH_ASKPASS_REQUIRE=force` 偏方在真交互场景下不可依赖。**本项目不采用该方案。**

### 2.3 本机环境实测（2026-09-18）

| 项目 | 状态 |
|---|---|
| `C:\Windows\System32\OpenSSH\ssh.exe` | ✅ 存在 |
| `C:\Program Files\PuTTY\plink.exe` | ✅ 存在 |
| `~/.ssh` 目录 | ❌ 不存在（尚未配置任何 config / 密钥） |

---

## 3. 设计原则（铁律，不可违反）

### 铁律一：`~/.ssh/config` 是唯一真相源

**不自创 JSON / YAML / SQLite 会话格式。**

理由：Git、VSCode Remote-SSH、`scp`、`rsync`、WinSCP、Ansible 全都读这个文件。自创格式等于把自己变成数据孤岛，用户每次改配置都要改两处。

**推论**：
- 写入采用**追加式**，绝不重排或重写用户手写的内容
- 元数据（标签、环境、备注）以**注释形式内嵌**在同一文件中，保证删除条目时元数据同步消失，不产生孤儿记录

### 铁律二：密码绝不落明文磁盘

密码只存 Windows Credential Manager（DPAPI 加密，绑定当前用户账户，文件被拷走也解不开）。

**推论**：
- 不写 `.env`、不写 JSON、不写明文临时文件
- 不在日志、shell history、命令行参数中暴露密码

### 铁律三：安全默认值不妥协

- 绝不默认设置 `StrictHostKeyChecking=no`
- 首次连接未知主机时，**保留 ssh 原生的交互确认**，不拦截、不静默接受
- 密码输入提示真实回显关闭，读入后立即写入凭据库

### 铁律四：连接器与前端解耦

连接层做成独立模块，接口稳定。将来从 CLI 换成 TUI 或 Web 面板时，只替换外壳，不改内核。

---

## 4. 非目标（明确不做）

| 不做的事 | 原因 |
|---|---|
| 不实现 SFTP 图形化管理器 | 有 WinSCP / `sftp` 命令，不在核心痛点 |
| 不做云同步 / 多设备同步 | 引入云端凭据风险，收益低 |
| 不做会话共享 / 多人协作 | 属于商业 SSH 网关（Teleport 类）的领域 |
| Step 1-2 不做终端仿真 | 用系统 `ssh.exe` / `plink.exe` 原生终端，规避仿真 bug |
| 不替换用户现有 SSH 客户端 | 只做前置包装，不做替代 |

---

## 5. 架构总览

```
用户输入  yssh prod
              │
              ▼
┌─────────────────────────────────────────────┐
│  yssh.exe（单文件，无运行时依赖）             │
│                                             │
│  ① 会话库        ② 凭据库        ③ 连接后端   │
│  别名扫描         CredRead        Backend    │
│  ssh -G 解析      CredWrite       选择器      │
│  追加式写入       CredDelete      ├ ssh.exe  │
│  元数据注释       密码提示        └ plink.exe│
└──────┬─────────────────┬──────────────┬─────┘
       │                 │              │
       ▼                 ▼              ▼
 ~/.ssh/config    Windows 凭据管理器   ssh.exe / plink.exe
 (唯一真相源)      (DPAPI 加密)        完全接管终端
```

**控制流（连接时）**

1. 解析参数 → 判断是「子命令」还是「连接请求」
2. 连接请求：模糊匹配 config 中的别名 → 唯一命中则进入下一步
3. 读取凭据：按 `user@host:port` 查 Credential Manager
4. 选择后端：无密码 → `SshBackend`；有密码 → `PlinkBackend`
5. 终端预处理：设置窗口标题 / 环境色带
6. 执行子进程，stdin/stdout/stderr 直接继承当前终端
7. 子进程退出 → 恢复终端状态 → 返回退出码

---

## 6. 数据设计

### 6.1 会话数据：`~/.ssh/config` + 内嵌元数据

**元数据注释格式**

```sshconfig
# yssh: env=prod tags=web,hk added=2026-09-18 note=主站反代
Host prod-web
    HostName 1.2.3.4
    User root
    Port 2222
    IdentityFile ~/.ssh/id_ed25519

Host hk-jump
    HostName 5.6.7.8
    User ubuntu
```

**格式规则**

| 规则 | 说明 |
|---|---|
| 前缀 | `# yssh:`（`#` 后一个空格，OpenSSH 会忽略该注释） |
| 字段分隔 | 空格分隔的 `key=value` |
| 字段清单 | `env`（环境）、`tags`（逗号分隔标签）、`added`（创建日期）、`note`（备注） |
| `note` 约束 | **必须放在最后一个字段**，取到行尾，允许含空格 |
| 无元数据 | 允许。手工添加的条目没有该注释，功能正常，只是没有标签和色带 |

**为什么用注释而不是 sidecar 文件**

| 方案 | 删除条目时 | 手工编辑 config 时 | Git 版本化 |
|---|---|---|---|
| 注释内嵌 ✅ | 元数据随块删除，无孤儿 | 不受影响 | 一起提交 |
| sidecar JSON ❌ | 产生孤儿记录，需定期清理 | 可能不一致 | 需额外提交 |

### 6.2 凭据数据：Windows Credential Manager

| 属性 | 值 |
|---|---|
| 类型 | Generic Credential |
| TargetName | `yssh:ssh:<user>@<host>:<port>` |
| UserName | `<user>@<host>:<port>` |
| CredentialBlob | 密码（UTF-8 字节序列） |
| Persist | `CRED_PERSIST_LOCAL_MACHINE` |
| Comment | `YunSSH credential for <alias>` |

**关键决策：为什么按 `user@host:port` 而不是按别名绑定？**

| 绑定键 | 改名后 | 多别名同主机 | 语义正确性 |
|---|---|---|---|
| 按别名 ❌ | 凭据失联 | 重复存储 | 密码属于「服务器上的账号」 |
| 按 `user@host:port` ✅ | 依然命中 | 自动共享 | 符合直觉 |

**实现建议**：使用 `github.com/danieljoos/wincred`（纯 Go，Windows 专用，API 简洁）；如需更细粒度控制，直接用 `golang.org/x/sys/windows` 的 `CredRead` / `CredWrite` / `CredDelete`。

**容量约束**：`CRED_MAX_CREDENTIAL_BLOB_SIZE` = 2560 字节，密码远小于此，无需分片。

### 6.3 本地状态：`~/.yssh/history.json`

用于「最近使用优先」排序，非关键数据，损坏可安全删除。

```json
{
  "version": 1,
  "hosts": {
    "prod-web": { "lastUsed": "2026-09-18T19:58:00+08:00", "count": 42 }
  }
}
```

---

## 7. 模块设计

| 模块 | 路径 | 职责 | 依赖 |
|---|---|---|---|
| `session` | `internal/session` | 扫描 config 提取别名；调 `ssh -G` 取生效配置；追加式写入 / 块删除；备份 | 无 |
| `meta` | `internal/meta` | 解析与生成 `# yssh:` 元数据注释 | 无 |
| `credential` | `internal/credential` | Credential Manager 的读写删；密码交互提示 | `wincred` |
| `backend` | `internal/backend` | 连接后端接口 + 实现（ssh / plink / native） | `session` |
| `search` | `internal/search` | 模糊匹配与排序 | `session` |
| `launcher` | `internal/launcher` | 把 ssh 命令行交给 Windows Terminal 执行 | 无 |
| `appicon` | `internal/appicon` | 图标绘制、多尺寸 ICO 编码、Windows 资源对象 | 无 |
| `tray` | `internal/tray` | 托盘菜单构建、事件分发 | `systray` `appicon` `session` `launcher` |
| `install` | `internal/install` | 自安装：文件复制、注册表、PATH、卸载登记 | `registry` |
| `dialog` | `internal/dialog` | 原生消息框（仅 `user32.dll`） | 无 |
| `term` | `internal/term` | 终端标题、环境色带、密码提示、宽度探测 | `x/term` |
| `cli` | `cmd/yssh` | 参数解析与命令分发 | 全部 |
| `ysshtray` | `cmd/ysshtray` | 托盘程序入口（GUI 子系统） | `tray` `install` |

### 7.1 `session` 包核心接口

```go
type Host struct {
    Alias        string
    HostName     string
    User         string
    Port         int
    IdentityFile string
    Meta         meta.Meta // 从 # yssh: 注释解析
    HasMeta      bool
    Approximate  bool      // true 表示字段来自文本扫描，非 ssh -G 权威值
}

// Load 读取配置文件；文件不存在时返回空配置而非错误（首次使用时正是预期状态）
func Load(path string) (*Config, error)

// Aliases 返回全部可连接别名，跳过含通配符的 Host 行
func (c *Config) Aliases() []string

// List 返回全部条目，字段来自轻量文本扫描（快，不 fork 进程）
func (c *Config) List() []Host

// Find 按别名精确查找
func (c *Config) Find(alias string) (Host, bool)

// Add 追加式写入一个新 Host 块（含元数据注释）
func (c *Config) Add(h Host) error

// Remove 删除指定别名的块，操作前自动备份
func (c *Config) Remove(alias string) (*RemoveResult, error)

// Resolve 通过 `ssh -G <alias>` 获取生效配置（处理 Include / 通配符 / 默认值）
func Resolve(alias string) (Host, error)

// SSHBin 定位 ssh 可执行文件，可用环境变量 YSSH_SSH_BIN 覆盖
func SSHBin() (string, error)
```

**核心技巧：用 `ssh -G` 代替自己写 config 解析器**

```bash
ssh -G prod-web
# 输出（节选）
hostname 1.2.3.4
user root
port 2222
identityfile ~/.ssh/id_ed25519
...
```

优势：
- 零成本支持 `Include`、`Match`、通配符、默认值合并
- 不会因语法演进导致解析错误
- 结论永远与真实 `ssh` 行为一致

**注意事项**
- `ssh -G` 对不存在的别名**不报错**（`hostname` 会回显输入值），因此别名列表必须先由文本扫描得到
- 每次调用是一个进程，100 台机器约 1-3 秒。**列表页只用文本扫描，详情页才调 `ssh -G`**

**删除的安全措施**
- 删除前将原文件备份到 `~/.yssh/backup/config.<timestamp>`
- 遇到 `Include` 指令时，仅操作目标块，不触碰被包含的文件

### 7.2 写入的原子性与并发

**问题**：多个 `yssh` 实例或用户的编辑器可能同时写 config。

**方案**：
1. 读取当前内容到内存
2. 修改后写入同目录临时文件 `config.tmp.<random>`
3. 写入前比对文件 mtime，若与读取时不同则中止并提示「配置已被其他程序修改，请重试」
4. 关闭编辑器句柄后，用 `os.Rename` 原子替换（Windows 上需先确保目标未被占用）

---

## 8. CLI 接口规范

### 8.1 参数分发规则

```
yssh <第一个参数>
    ├── 命中子命令集合 → 按子命令处理
    ├── 含 "@" 或匹配 IP:port 形式 → ad-hoc 直连（不写入 config）
    └── 其他 → 作为关键词模糊匹配 config 别名
```

**冲突逃逸**：`yssh run <alias>` 强制按连接处理。

### 8.2 命令清单

| 命令 | 说明 | 阶段 |
|---|---|---|
| `yssh` | 列出全部主机（标记环境、最近使用、凭据状态） | Step 1 |
| `yssh <关键词>` | 模糊搜索并连接（唯一命中直接连，多命中列出候选） | Step 1 |
| `yssh run <alias> [透传参数...]` | 显式连接，别名冲突时使用 | Step 1 |
| `yssh add <alias> <user>@<host> [-p port] [-i key]` | 添加条目 | Step 1 |
| `yssh rm <alias>` | 删除条目（自动备份） | Step 1 |
| `yssh show <alias>` | 显示生效配置（`ssh -G` 结果）与凭据状态 | Step 1 |
| `yssh edit <alias>` | 用编辑器打开 config 并定位到该条目 | Step 1 |
| `yssh doctor` | 环境自检：ssh/plink 版本、config 语法、凭据可用性、权限 | Step 2 |
| `yssh passwd <alias>` | 设置 / 更新密码 | Step 2 |
| `yssh forget <alias>` | 删除该主机的凭据 | Step 2 |
| `yssh import <file>` | 批量导入（nmap / CSV / 文本） | Step 4 |
| `yssh export [file]` | 导出（脱敏，不含密码） | Step 4 |
| `yssh exec <关键词> -- <命令>` | 批量执行 | Step 4 |

### 8.3 参数透传设计

**规则：第一个非选项参数之后的所有参数，全部透传给底层 SSH 客户端。**

```bash
yssh prod-web -L 3306:db.internal:3306      # 本地端口转发
yssh prod-web -N -L 8080:127.0.0.1:80       # 常驻隧道
yssh prod-web -v                            # 详细日志
yssh prod-web -- ls -la /var/log            # 在远端执行命令
```

**冲突处理**：`yssh` 自有选项一律使用长选项（`--port` 而非 `-p`）。这样 `-p`、`-L`、`-N`、`-v`、`-D`、`-R` 等短选项全部保留给底层 SSH，无歧义。

### 8.4 输出示例

```
$ yssh
  ENV    ALIAS        TARGET                    LAST
  prod   prod-web     root@1.2.3.4:2222         2h ago   [key]
  test   hk-jump      ubuntu@5.6.7.8:22         -        [psw]
         lab-msf      msfadmin@192.168.78.129:22 1d ago  [psw]

$ yssh web
→ 匹配 prod-web，正在连接...
```

`[key]` = 使用密钥认证；`[psw]` = 已保存密码；`[   ]` = 每次输入。**只显示凭据是否存在，不显示任何密码内容。**

### 8.5 退出码

| 码 | 含义 |
|---|---|
| 0 | 连接正常结束 |
| 1 | 一般错误（参数、文件） |
| 2 | 别名未找到 / 歧义 |
| 3 | 后端不可用（plink 缺失等） |
| 4 | 凭据操作失败 |
| 其他 | 原样透传底层 SSH 客户端的退出码 |

---

## 9. 连接后端设计

### 9.1 接口定义

```go
type Request struct {
    Alias        string
    Host         string
    Port         int
    User         string
    IdentityFile string
    Password     string   // 空表示使用密钥/交互
    ExtraArgs    []string // 用户透传参数
    Env          string   // 环境标识，用于色带
}

type Backend interface {
    Name() string
    Available() error                    // 探测可用性（可执行文件是否存在、版本是否满足）
    Build(req Request) (exe string, args []string, cleanup func(), err error)
}
```

`Build` 只负责产出「可执行文件 + 参数 + 清理函数」，**不负责执行**。执行统一由 `runner` 负责，保证终端接管逻辑只有一份。

### 9.2 后端选择逻辑

```
if req.Password == ""        → SshBackend    （密钥认证，走原生 ssh）
else if plink 可用            → PlinkBackend  （密码认证）
else                          → 报错，提示 yssh doctor / 安装 PuTTY
```

**不静默降级**：需要密码但 plink 不可用时，明确报错并给出解决路径，绝不悄悄退回交互式输入让用户困惑。

### 9.3 plink 参数翻译表（重点）

| OpenSSH | plink | 备注 |
|---|---|---|
| `-p <port>` | `-P <port>` | ⚠️ **大小写相反**，最容易踩的坑 |
| `-i <key>` | `-i <key>` | ⚠️ PuTTY 传统上只认 `.ppk` 格式 |
| `-l <user>` | `-l <user>` | 一致 |
| `-L a:b:c` | `-L a:b:c` | 一致 |
| `-R a:b:c` | `-R a:b:c` | 一致 |
| `-D <port>` | `-D <port>` | 一致 |
| `-N` | `-N` | 一致 |
| `-v` | `-v` | 一致 |
| `-o StrictHostKeyChecking=no` | 无等价项，可用 `-hostkey <指纹>` 预置 | 语义不同 |
| 读 `~/.ssh/config` | **不支持** | 必须完整翻译，这是持续维护成本 |
| 首次连接未知主机 | 交互确认；`-batch` 会直接拒绝 | 需妥善处理 |

**待实测风险（R2）**：PuTTY 0.68+ 的 `puttygen` 能读取 OpenSSH 新格式私钥，但 `plink -i` 是否能直接加载 OpenSSH 原生格式私钥、以及加密私钥是否支持，需在真机验证。**若不支持，则 Step 2 的 plink 后端仅适用于纯密码认证场景**——这会成为推进 Step 3（自研客户端）的又一理由。

### 9.4 密码传递方式

| 方式 | 安全性 | 采用 |
|---|---|---|
| `-pw <password>` | ❌ 密码出现在命令行参数，本机任意进程可读 | 不用 |
| `-pwfile <path>` | ⭕ 需落盘临时文件，但不在进程列表暴露 | **采用（Step 2）** |
| stdin 管道 | plink 不支持 | - |
| 自研客户端（内存中传递） | ✅ 无落盘、无参数 | **Step 3 目标** |

**`-pwfile` 的临时文件加固**：
1. 路径：`%TEMP%\yssh-<随机16字节hex>.pw`
2. 用 `os.OpenFile` 创建时即设置仅当前用户可读（`0600`，Windows 上同时设置 ACL）
3. `defer os.Remove(path)` 双保险（`Build` 返回的 `cleanup` 函数）
4. 写入后立即 `Sync`，避免缓冲区残留
5. 进程被强杀时残留风险：启动时清理 `%TEMP%\yssh-*.pw`（`yssh doctor` 也做此检查）

---

## 10. 关键技术实现

### 10.1 终端接管

```go
cmd := exec.Command(exe, args...)
cmd.Stdin  = os.Stdin
cmd.Stdout = os.Stdout
cmd.Stderr = os.Stderr
cmd.Run()
```

**要点**
- Windows 无 `exec` 系统调用族，不能替换进程映像，只能这样继承句柄
- **`yssh` 自身在交出终端前绝不能读取 stdin**，否则会吞掉用户的输入缓冲
- 退出码原样返回：`if ee, ok := err.(*exec.ExitError); ok { os.Exit(ee.ExitCode()) }`

### 10.2 密码交互式输入

```go
fmt.Fprint(os.Stderr, "Password: ")
pw, err := term.ReadPassword(int(os.Stdin.Fd()))
fmt.Fprintln(os.Stderr)
```

- 提示输出到 **stderr**，避免污染 stdout 的管道用途
- 首次连接某主机时提示输入 → 询问「是否保存到凭据管理器（y/N）」→ 默认不保存
- 保存时：先 `CredWrite`，成功后再连接

### 10.3 环境色带（防手滑）

**实现方式**：仅在本地终端使用 ANSI 转义序列，**不修改远程机器任何配置**。

```go
// 连接前
fmt.Printf("\x1b]0;%s %s\x07", envBadge, alias)   // 设置窗口标题（OSC 0）
fmt.Print("\x1b]11;rgb:3a/1a/1a\x07")             // 生产：深红背景（OSC 11）
// 断开后
fmt.Print("\x1b]110\x07")                         // 恢复默认背景
```

| 环境 | 标题标记 | 背景色 |
|---|---|---|
| `prod` | `⚡ PROD` | 深红 |
| `test` | `TEST` | 深绿 |
| `lab` | `LAB` | 深蓝 |
| 未标记 | 无 | 不变 |

**待实测风险（R3）**：Windows Terminal 对 `OSC 11`（动态改背景色）的支持需按实际版本确认。**降级方案**：仅设置窗口标题（`OSC 0`，支持度极高），色带走「在远端 shell 注入 `PS1` 前缀」的可选方案。

### 10.4 模糊匹配算法

**匹配优先级（由高到低）**

| 级别 | 规则 | 示例 |
|---|---|---|
| 1 | 完全相等 | `web` → `web` |
| 2 | 前缀匹配 | `web` → `web-hk` |
| 3 | 子串匹配 | `web` → `my-web-01` |
| 4 | 子序列匹配（fzf 风格） | `wbhk` → `web-hk` |

**同级排序**：最近使用时间（`history.json`）倒序 → 使用次数倒序 → 别名字典序。

**歧义处理**：
- 1 个结果 → 直接连接
- 2-9 个结果 → 列出行表格，提示「请补充关键词」
- 0 个结果 → 用编辑距离列出最相近的 3 个候选

### 10.5 批量导入（Step 4，格式预设计）

支持三种输入：

```
# 1. nmap 输出（grepable）
Host: 192.168.78.129 ()	Ports: 22/open/tcp//ssh///

# 2. 简单文本
192.168.78.129
10.0.0.5:2222 root

# 3. CSV
alias,host,port,user,env
lab-dvwa,192.168.78.129,8088,admin,lab
```

导入时统一询问：默认用户名、默认环境标签、是否覆盖同别名条目。

---

## 11. 安全设计

| 项 | 措施 |
|---|---|
| 密码存储 | 仅 Windows Credential Manager（DPAPI），不落盘 |
| 密码传递 | Step 2 用 `-pwfile` + 即时删除；Step 3 改为纯内存 |
| 命令行暴露 | 禁用 `plink -pw` |
| 主机指纹 | 保留 ssh 原生交互确认，绝不默认 `StrictHostKeyChecking=no` |
| 已知主机文件 | 不自动清理 `known_hosts`；指纹变更时原样透传警告 |
| 日志 | 不记录密码、不记录完整连接串到 history 文件 |
| 配置备份 | 删除操作前备份到 `~/.yssh/backup/`，保留最近 10 份 |
| 权限检查 | `yssh doctor` 检查 `~/.ssh` 及 config 的 ACL 是否过于宽松 |
| 凭据可见性 | `yssh` 列表只显示凭据「有/无」，不显示任何内容片段 |

---

## 12. 错误处理

| 场景 | 行为 |
|---|---|
| 别名未找到 | 列出编辑距离最近的 3 个候选，退出码 2 |
| 别名歧义 | 列出全部匹配项，退出码 2 |
| `ssh.exe` 不存在 | 提示启用 Windows 可选功能「OpenSSH 客户端」，退出码 3 |
| 需要密码但 plink 缺失 | 明确报错 + 三种解决路径（装 PuTTY / 改密钥 / 等待 Step 3），**不静默降级**，退出码 3 |
| 凭据读取失败 | 回退到交互式输入，并提示可能原因，退出码 4（若最终失败） |
| config 被占用 / 写入冲突 | 提示「配置已被其他程序修改」，保留内存副本，退出码 1 |
| 首次连接未知主机 | 不拦截，交给底层客户端原生提示 |
| 连接超时 / 拒绝 | 原样透传底层错误文本，附加建议「加 `-v` 查看详情」 |
| plink 缺少 libcrypto 等依赖 | 透传错误 + 建议 `yssh doctor` |

---

## 13. 目录结构

```
YunSSH/
├── cmd/
│   └── yssh/
│       └── main.go              # 入口 + 命令分发
├── internal/
│   ├── session/                 # config 读写、别名扫描、ssh -G 解析
│   │   ├── session.go
│   │   ├── writer.go            # 追加式写入、块删除、备份
│   │   └── resolve.go           # ssh -G 调用与解析
│   ├── meta/                    # # yssh: 元数据注释的解析与生成
│   │   └── meta.go
│   ├── credential/              # Windows Credential Manager
│   │   ├── credential.go
│   │   └── prompt.go            # 交互式密码输入
│   ├── backend/                 # 连接后端
│   │   ├── backend.go           # 接口定义 + 选择器
│   │   ├── ssh.go               # 原生 ssh.exe（密钥认证）
│   │   ├── plink.go             # plink.exe（密码认证，含参数翻译）
│   │   ├── native.go            # [Step 3] 自研 x/crypto/ssh
│   │   └── runner.go            # 统一的执行与终端接管
│   ├── search/                  # 模糊匹配与排序
│   │   └── search.go
│   ├── history/                 # ~/.yssh/history.json
│   │   └── history.go
│   └── term/                    # 终端标题、色带、宽度探测
│       └── term.go
├── docs/
│   ├── DESIGN.md                # 本文档
│   └── PLAN.md                  # 分阶段任务清单
├── go.mod
└── README.md
```

---

## 14. 实施计划

### Step 1 — 可用版（不碰密码）　✅ 已实现并验证

**范围**：会话库 + 模糊搜索 + 密钥认证连接

| 任务 | 交付物 |
|---|---|
| 项目骨架 + `go.mod` | 可编译的空壳 |
| `session`：别名扫描 | `List()` 可用 |
| `session`：`ssh -G` 解析 | `Resolve()` 可用 |
| `session`：追加式写入 + 块删除 + 备份 | `Add()` / `Remove()` 可用 |
| `meta`：注释解析与生成 | 标签 / 环境读写正常 |
| `search`：模糊匹配 | 四级优先级排序 |
| `history`：最近使用 | 排序权重生效 |
| `backend/ssh.go` + `runner` | 终端完整接管 |
| `term`：标题设置 | 连接时标题变化 |
| CLI：`ls` / `add` / `rm` / `show` / `edit` / `<关键词>` | 全部可用 |
| 构建脚本 | `yssh.exe` 产出 |

**关键验收**：`yssh add` 写入的条目，**必须能被原生 `ssh <alias>` 直接连通**——这是「唯一真相源」设计成立的证明。

**实现结果（2026-09-18）**：全部任务完成，零第三方依赖（仅标准库）。

- 源码：`cmd/yssh`（入口 + 列表/连接/增删改查）、`internal/{meta,session,search,history,backend,term}`
- 单元测试：`meta` / `search` / `session` 三个包全绿，含「追加写入后用户原有内容逐字节不变」的核心不变量断言
- 端到端：13 项验证全部通过（真实 ssh 解析、删除保护、`Include` 不被破坏、参数透传、共享 Host 行保护、输出纯净度）
- 关键验收已确认：用 `ssh -G` 实测，`yssh add web root@1.2.3.4 --port 2222` 写入后，OpenSSH 解析出 `1.2.3.4 / 2222 / root`
- 实现期发现并修复两个缺陷：输出重定向时泄漏 OSC 标题序列；元数据 `tags=` 含空格时静默丢标签

### Step 2 — 密码层（覆盖靶机场景）

| 任务 | 交付物 |
|---|---|
| `credential`：CredWrite / CredRead / CredDelete | 凭据读写删 |
| `credential/prompt`：交互式输入 | 首次连接提示并可选保存 |
| `backend/plink.go`：参数翻译 | 密码认证可用 |
| `backend`：选择器与「不静默降级」 | 后端选择逻辑 |
| `-pwfile` 临时文件加固 | 创建即限权 + 即时删除 + 启动清理 |
| `term`：环境色带 | 生产/靶机视觉区分 |
| `yssh doctor` | 环境自检 |
| `yssh passwd` / `forget` | 凭据管理命令 |

### Step 3 — 自研客户端（密码仅存内存）

| 任务 | 交付物 |
|---|---|
| `backend/native.go`：`x/crypto/ssh` | 自建连接 |
| PTY 请求与 `window-change` | 窗口尺寸联动 |
| 本地 raw mode 开关（含异常恢复） | 终端不花 |
| ANSI / UTF-8 双向透传 | 显示正常 |
| `keyboard-interactive` 认证 | 兼容多数服务器 |
| 老算法支持（`diffie-hellman-group1-sha1` 等） | 兼容靶场环境 |
| 密码 `zeroize` 清零 | 安全收口 |
| 会话录制（可选） | 取证场景 |

### Step 4 — 运维增强（可选）

批量导入 / 导出、环境色带增强、批量执行、标签过滤、TUI 前端。

---

## 15. 验收标准

### Step 1

| # | 场景 | 预期 |
|---|---|---|
| 1 | `yssh add web root@1.2.3.4 --port 2222` | 写入 config，格式正确，含元数据注释 |
| 2 | `ssh web` （原生命令） | 能连通 → 证明格式合规 |
| 3 | `yssh web` | 直接连接，终端行为与原生 ssh 一致（vim / tmux / Ctrl+C 正常） |
| 4 | `yssh wb` | 模糊匹配到 `web` |
| 5 | `yssh rm web` | 条目删除，**config 中手写的其他内容与注释原样保留** |
| 6 | 删除后检查 `~/.yssh/backup/` | 存在备份文件 |
| 7 | config 含 `Include` 指令 | 读取正常，不破坏被包含文件 |
| 8 | `yssh prod -L 8080:127.0.0.1:80` | 端口转发参数正确透传 |
| 9 | 别名不存在 | 输出 3 个相近候选，退出码 2 |

### Step 2

| # | 场景 | 预期 |
|---|---|---|
| 10 | 首次连靶机，输入密码并选择保存 | 凭据入 Credential Manager，`控制面板 → 凭据管理器` 可见 |
| 11 | 再次连接同一主机 | 无需输入密码，直接进入 |
| 12 | 连接期间检查进程列表 | **命令行参数中不含密码** |
| 13 | 连接结束后检查 `%TEMP%` | 无 `yssh-*.pw` 残留 |
| 14 | 卸载/删除 plink 后连密码主机 | 明确报错 + 解决路径，不静默降级，退出码 3 |
| 15 | `yssh` 列表 | 只显示凭据有/无，不泄露任何内容 |
| 16 | 连生产环境 | 标题/色带生效，断开后恢复 |

### 15.3 自动化验证策略

实现阶段确立了两层验证，分工来自一个实测得出的平台限制：

| 层级 | 环境 | 覆盖范围 | 局限 |
|---|---|---|---|
| 单元测试 | 任意平台 | `meta` 解析/渲染往返、`search` 四级匹配与编辑距离、`session` 读写全流程（含「不破坏用户手写内容」这一核心不变量）、CRLF 保留、并发修改拦截 | 不触达真实 ssh |
| 端到端验证 | Linux 容器 | 真实 ssh 解析写入的配置、删除保护、`Include` 指令不被破坏、参数透传、输出纯净度 | 需要 Docker |

**为什么端到端验证必须在 Linux 容器里跑**

核心验收项是「`yssh` 写入的配置，原生 `ssh` 能否正确解析」。验证它必须让 `ssh` 去读一份临时配置，这就要求能重定向 `ssh` 的家目录。

实测结论（踩坑两次后才确认）：**OpenSSH 通过 passwd 数据库中的 `pw_dir` 定位 `~/.ssh/config`，完全不读 `$HOME` / `%USERPROFILE%` 环境变量。**

| 平台 | 实测现象 |
|---|---|
| Windows | 设置 `USERPROFILE` 指向临时目录后，`ssh -G web` 仍返回系统默认值：hostname 回显为别名本身、user 为当前系统用户名、port 为 22 |
| Linux 容器 | 设置 `HOME` 指向临时目录后，同样读到的是 passwd 里的家目录 |

关键在于：`os.UserHomeDir()` 在 Go 中读的正是 `USERPROFILE`（Windows）/ `HOME`（Unix），**所以 yssh 自身可以被环境变量隔离，而 ssh 不行**。测试中两者指向了不同目录，`show` 因此输出了看起来合理、实际完全错误的值——不加甄别就会得出错误结论（这正是本项目实现期最容易误判的一处）。

结论：**重定向 OpenSSH 的配置路径只能靠容器**（独立的文件系统与 passwd 数据库）。端到端脚本因此要求 `/.dockerenv` 存在才允许运行，并直接使用当前用户的家目录、在运行前后清理。

**测试注入点**：`session.SSHBin()` 支持用环境变量 `YSSH_SSH_BIN` 覆盖。端到端脚本借此注入桩程序，从而在不建立真实连接的前提下精确验证命令行参数的拼装与透传（例如确认 `-L 3306:127.0.0.1:3306` 与 `-N` 被原样传递、别名作为首个参数）。

**实现阶段补充的两条约束**

| 约束 | 原因 |
|---|---|
| 输出被重定向时不得写 ANSI 转义序列 | 窗口标题（OSC 0）会污染 `yssh ls > hosts.txt` 的结果。实测中该缺陷表现为输出里混入 `;[PROD] db-01` 这样的残片 |
| 元数据解析需容忍 `tags=web, hk` 这类含空格写法 | 严格按空格切分字段会静默丢弃后半个标签，用户难以察觉 |

---

## 16. 风险与待验证项

| # | 风险 | 影响 | 应对 |
|---|---|---|---|
| R1 | Windows OpenSSH 的 `SSH_ASKPASS` 不可靠 | 已规避 | 不采用，改走 plink / 自研 |
| R2 | **`plink -i` 对 OpenSSH 原生格式私钥的支持未知** | 若不支持，plink 后端仅适用纯密码场景 | **Step 2 开工前先实测**；不支持则说明该限制并加速 Step 3 |
| R3 | Windows Terminal 对 `OSC 11`（动态改背景色）的支持未知 | 色带失效 | 降级为「仅改窗口标题」；或改为远端注入 PS1 前缀 |
| R4 | `ssh -G` 在旧版 Windows OpenSSH 上可能不存在 | 无法解析生效配置 | `yssh doctor` 检查版本；降级为自带简化解析器 |
| R5 | 用户手改 config 后 `yssh` 的块定位失败 | 删除操作删错内容 | 删除前展示将删除的确切行范围，要求确认 |
| R6 | config 被编辑器独占锁定时写入失败 | 操作失败 | 明确报错 + 保留内存副本 + 提示重试 |
| R7 | 环境存在 EDR / 进程审计时，密码经临时文件仍有风险 | 安全合规 | Step 2 文档中明确标注；Step 3 彻底解决 |

---

## 17. 与 ssh-ai-tool 的关系

用户此前规划的 `ssh-ai-tool`（Node `ssh2` + `xterm.js` + 可插拔 AI Provider）与本项目在**会话层是同一件事**。

| 维度 | YunSSH | ssh-ai-tool |
|---|---|---|
| 形态 | Windows 原生 CLI | Web 面板 |
| 终端 | 系统 `ssh.exe` / `plink.exe` | xterm.js 自渲染 |
| 连接层 | Go（`ssh.exe` / plink / `x/crypto/ssh`） | Node（`ssh2`） |

**建议**：**尽早决策是合并还是明确分工**，否则 SSH 连接层会被实现两遍。

- 若合并：YunSSH 做「Windows 原生 CLI 客户端」，ssh-ai-tool 做「Web 远程面板」，两者共用同一份 config 与 Credential Manager 中的凭据（凭据键 `yssh:ssh:<user>@<host>:<port>` 可直接复用）。
- 若分工：明确 YunSSH 只做本地终端加速，ssh-ai-tool 只做浏览器访问，互不重叠。

---

## 18. 技术栈决策记录

| 决策 | 选择 | 理由 | 被否方案及原因 |
|---|---|---|---|
| 语言 | **Go** | 单文件分发（5-8MB）；Windows 原生 API 顺手；`x/crypto/ssh` 成熟 | Node+TS：分发体积 40-90MB、`keytar` 停维护、Windows 无 `SIGWINCH`；Rust：瓶颈在网络 IO，性能收益为零 |
| 形态 | **CLI** | 零侵入，终端行为原生；可脚本化 | TUI：开发量 2 倍，中文宽字符与终端恢复是坑；Web：需常驻进程 + 浏览器，日常连接受累 |
| 会话存储 | **`~/.ssh/config` + 注释元数据** | 与生态互通，无孤儿数据 | sidecar JSON：产生孤儿记录 |
| 凭据存储 | **Windows Credential Manager** | DPAPI 加密，绑定用户账户 | 明文文件：违反铁律二 |
| 凭据键 | **`user@host:port`** | 改名不失效，多别名共享 | 按别名：改名即失联 |
| 配置解析 | **`ssh -G`** | 零成本支持 Include / 通配符 / 默认值 | 自写解析器：维护成本高、易与真实行为不符 |
| 写入方式 | **追加 + 块删除 + 备份** | 绝不破坏用户手写内容 | 全量重写：会丢失注释与格式 |

---

## 19. 托盘与安装设计（v0.2.0 补充）

### 19.1 托盘不自己实现终端

托盘点击主机后执行的是 `wt -w 0 nt ssh <别名>`——终端仍然是系统原生的。

理由与 Step 1 一致：自己实现终端仿真意味着要处理 ANSI 解析、PTY resize、鼠标事件、真彩色一整套问题，而收益仅仅是"看起来是一体的"。把终端交给 Windows Terminal 之后，`vim`、`tmux`、Ctrl+C、滚动、复制粘贴的语义全部天然正确。

未安装 Windows Terminal 时回退到 `cmd /c start "" ssh <别名>`，保证功能不缺失。

### 19.2 菜单结构的两个约束

**约束一：父项不能禁用。** Windows 原生菜单里被禁用的项无法展开子菜单。因此「主机」子菜单的父项保留了一个有意义的动作：点击即在终端打开完整列表（等价于运行 `yssh`）。

**约束二：动态内容必须关在子菜单里。** 静态项（刷新、关于、退出）位于菜单末尾，若每次刷新都重建全部菜单项，新加的主机项会被追加到末尾，顺序就乱了。把主机放进独立子菜单后，刷新只影响子菜单内部，静态项位置固定。

菜单项标题带 `[env]` 前缀，是「不展开子菜单也能分辨生产与靶机」的直接实现，与铁律三（安全默认值不妥协）同源。

刷新时旧的监听 goroutine 通过关闭 `done` channel 退出，避免反复刷新后 goroutine 堆积。

### 19.3 自安装的设计取舍

**为什么不用 Inno Setup 之类的打包器**

程序需要的全部能力——复制文件、写注册表、改 PATH、登记卸载项——用 Go 加 `x/sys/windows/registry` 就能完成，不引入外部工具链。代价是放弃图形安装向导；收益是分发时只有一个 exe，构建流程没有任何额外依赖。

安装逻辑全部收在 `internal/install` 内。**将来若改用标准安装器，只需替换这一层，程序自身代码不受影响**——这是把它独立成包而非塞进 `cmd/` 的主要原因。

**为什么装在 %LOCALAPPDATA% 而不是 Program Files**

用户级安装全程无需管理员权限、不弹 UAC。代价是每个用户需各自安装一次，对本工具的使用场景（个人开发机）可以接受。

**卸载时如何删除自己**

正在运行的可执行文件无法自删。做法是生成一个临时批处理：先 `ping` 延时约 2 秒（用 `ping` 而非 `timeout`，后者在无控制台环境下会直接报错），再 `rmdir /s /q` 删除目录，最后删除脚本自身。

**PATH 修改的两处细节**

1. 必须用 `SetExpandStringValue` 写回。用户 PATH 里常含 `%USERPROFILE%` 这类变量，若写成普通字符串会丢失展开语义，等于悄悄改坏别人的环境。
2. 改完必须广播 `WM_SETTINGCHANGE`。否则已运行的资源管理器不会刷新环境块，用户新开的终端仍读不到更新后的 PATH。

### 19.4 应用图标全部由代码生成

仓库里没有任何图片文件：图标由 `internal/appicon` 按参数逐像素绘制，再编码成多尺寸 ICO。配色、圆角、字形都是包里的常量，改完重新构建即可。

**为什么要多尺寸，而不是只存一张 32x32**

托盘槽位随显示缩放变化（100% / 125% / 150% / 200% 分别是 16 / 20 / 24 / 32）。ICO 里若只有单一尺寸，系统只能自己做**非整数**缩放——32 缩到 24 是 0.75 倍，折线笔画本来就只有几像素宽，重采样后边缘会糊成一片。现在收录 16 到 256 共八档，每档都是原生绘制，且小尺寸单独放大字形、加粗笔画做光学补偿。

**exe 图标怎么进去的**

`tools/genicon` 生成 `.syso`（一个只含 `.rsrc` 段的 COFF 目标文件），放在 `cmd/yssh` 与 `cmd/ysshtray` 目录下，`go build` 会自动把它链接进 exe。这样既不需要 `rsrc` / `go-winres` 之类的工具，也不必往仓库里放二进制产物（`.syso` 与 `app.ico` 都在 `.gitignore` 里）。

生成 `.syso` 有三个约束，不写下来一定会踩：

1. **段特性必须恰好是** `IMAGE_SCN_CNT_INITIALIZED_DATA | IMAGE_SCN_MEM_READ`。Go 链接器用特性位判断段类型，多带一个 `IMAGE_SCN_MEM_DISCARDABLE` 会导致整段被跳过，图标静默消失。
2. **资源目录里每个目录的条目数组必须紧贴它的头部。** `IMAGE_RESOURCE_DIRECTORY` 结构体里没有「条目数组偏移」字段，读取方固定按 `目录偏移 + 16` 定位条目；把头部和条目拆成两片区域分别排布会产出非法结构。
3. **数据条目的 `OffsetToData` 必须是 RVA，而段内偏移转 RVA 需要链接期才知道的段基址。** 因此要为每个数据偏移字段补一条 `IMAGE_REL_AMD64_ADDR32` 重定位：链接器读取该位置上的原值作为加数、再加上段基址回写，正好完成换算。

`.syso` 按 `rsrc_windows_amd64.syso` 命名，交叉编译到其它平台时会被 `go build` 自动忽略，不影响 Docker 里的 Linux 验证流程。

### 19.5 一个容易忽略的编译约束

托盘库在 Windows 下是纯 Go 实现，但**在其它平台需要 CGO（GTK）**。因此 `internal/tray`、`internal/install`、`cmd/ysshtray` 全部带 `//go:build windows`；`cmd/yssh` 的安装命令用 `cmd_install_windows.go` / `cmd_install_other.go` 两个文件承载，保证非 Windows 平台仍可编译。

构建时必须设 `CGO_ENABLED=0`，否则会破坏单文件分发的目标。

### 19.6 验证结果（2026-09-18）

| 项 | 结果 |
|---|---|
| 安装 | 文件复制、卸载登记项、自启项、PATH 注入全部正确写入 |
| 重复安装 | 覆盖成功，自启状态按参数正确切换（升级路径可用） |
| 卸载 | 注册表清理 + 延迟删除目录，`~/.ssh/config` 未受影响 |
| 重复卸载 | 提示「尚未安装」并返回 0，操作幂等 |
| 环境残留 | 无。用户原有 10 条 PATH 条目一条未动 |
| 托盘启动 | 从安装目录启动正常，图标进入托盘区 |

---

*本文档为 Step 1 开工前的定稿依据。R2、R3 两项需在对应阶段开工前实测确认，确认结果应回写本文档。*
