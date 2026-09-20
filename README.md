# YunSSH

Windows 终端的 SSH 会话管理工具。

把 `ssh root@1.2.3.4 -p 2222` 变成 `yssh ALIAS`。

```console
$ yssh
ENV   ALIAS      TARGET                   TAGS     LAST
prod  prod-web   root@1.2.3.4:2222        web,hk   2h ago
lab   lab-msf    msfadmin@192.168.78.129           just now

$ yssh prod-web
→ prod-web
```

---

## 它是什么，不是什么

**是**：一个加速录入层。快速检索、增删主机条目，然后把终端完整交给系统 ssh 客户端。

**不是**：

- 不替代 `ssh`。连接时实际执行的是系统自带客户端，终端行为与原生完全一致
- 不保存密码（当前版本）。认证走密钥
- 不自建配置格式。所有数据都存在 `~/.ssh/config` 里

最后一条是刻意的选择：因为写入的是标准配置，所以这些条目对**原生 `ssh`、`scp`、`rsync`、`git`、VSCode Remote-SSH、WinSCP** 全部立即可见。工具只是录入层，不是数据孤岛。

---

## 设计原则

1. **`~/.ssh/config` 是唯一真相源** — 元数据以注释内嵌在同一个文件中，删除条目时元数据同步消失，不留孤儿记录
2. **密码绝不落明文磁盘** — 当前版本不碰密码；后续接入时存 Windows Credential Manager（DPAPI 加密）
3. **安全默认值不妥协** — 绝不默认 `StrictHostKeyChecking=no`，首次连接保留 ssh 原生的主机指纹确认
4. **连接层与前端解耦** — 想换成 TUI 或 Web 面板时，只替换外壳

---

## 快速开始

需要 Go 1.21+。

```bash
go build -trimpath -o yssh.exe ./cmd/yssh
```

Windows 上要一并构建带图标的两个可执行文件时，直接跑仓库根的 `build.bat`：它会先由 `tools/genicon` 生成图标与资源对象，再产出 `yssh.exe` 和 `ysshtray.exe`。

把 `yssh.exe` 放进 `PATH`，然后：

```bash
yssh add web root@1.2.3.4 --port 2222 --env prod --tags web,hk --note "主站反代"
yssh                    # 列出全部主机
yssh web                # 连接
```

---

## 安装

### 方式一：双击安装（推荐）

双击 `ysshtray.exe`，按提示确认即可，**全程无需管理员权限**。

| 动作 | 位置 |
|---|---|
| 复制程序 | `%LOCALAPPDATA%\Programs\YunSSH\` |
| 登记卸载项 | `HKCU\...\Uninstall\YunSSH` → 出现在「设置 → 应用」 |
| **开始菜单快捷方式** | `%APPDATA%\Microsoft\Windows\Start Menu\Programs\YunSSH.lnk` |
| 开机自启（可选） | `HKCU\...\CurrentVersion\Run` |
| 加入 PATH | `HKCU\Environment\Path` |

安装后按 `Win` 键搜索 `YunSSH` 即可找到；托盘程序带单实例保护，重复点击不会出现第二个图标。

安装完成后程序会从安装目录重新拉起，托盘图标随即出现。

### 方式二：命令行安装

```bash
yssh install                    # 交互询问是否开机自启
yssh install --autostart        # 直接开启
yssh install --no-autostart --no-path
yssh uninstall                  # 卸载
yssh autostart on|off           # 随时切换开机自启
```

重复执行 `install` 即为升级（同名文件直接覆盖）。

---

## 托盘程序

安装后 `ysshtray.exe` 常驻系统托盘：

```
主机 ▸
├── [prod] prod-web          ← 点击即在 Windows Terminal 新标签中连接
├── [prod] prod-db
├── [lab]  lab-msf
└── hk-jump
────────────
搜索主机…                     ← 在终端里打开完整列表
打开配置文件
刷新列表
────────────
开机自启                      ← 可勾选
────────────
关于 / 退出
```

菜单按「环境 → 别名」排序，未标注环境的条目排在最后。带 `[env]` 前缀是有意为之：不展开子菜单也能一眼分辨生产与靶机。

**托盘与 CLI 共用同一份 `~/.ssh/config`**——命令行里 `yssh add` 添加的主机会立刻出现在托盘菜单中，反之亦然。两者不是两套数据。

连接默认在 **Windows Terminal 的新标签页**打开（`wt -w 0 nt ssh <别名>`），终端行为与手动敲 `ssh` 完全一致；未安装 Windows Terminal 时自动回退到系统默认方式。

---

## 用法

| 命令 | 说明 |
|---|---|
| `yssh` | 列出全部主机 |
| `yssh <关键词>` | 模糊匹配并连接（唯一命中直接连，多命中列出候选） |
| `yssh run <别名> [参数...]` | 显式连接（别名与子命令重名时使用） |
| `yssh add <别名> <用户@主机>` | 添加条目；不带参数则进入交互模式 |
| `yssh rm <别名>` | 删除条目，删除前自动备份 |
| `yssh show <别名>` | 显示 `ssh -G` 解析出的生效配置 |
| `yssh edit [别名]` | 用编辑器打开配置文件 |

**add 选项**：`--port` `--key` `--env` `--tags` `--note`（`--note` 必须放最后，可含空格）

**透传参数**：第一个参数之后的内容原样传给 ssh，因此以下都成立：

```bash
yssh prod-web -L 3306:127.0.0.1:3306      # 本地端口转发
yssh prod-web -N -L 8080:localhost:80     # 常驻隧道
yssh prod-web -v                          # 详细日志
yssh prod-web -- ls -la /var/log          # 远端执行命令
```

命名约定：`yssh` 自有选项一律使用长选项，短选项（`-p` `-L` `-N` `-v` `-D` `-R`）全部保留给底层 ssh，不存在参数冲突。

**环境变量**：

- `YSSH_SSH_BIN` — 覆盖 ssh 可执行文件路径
- `NO_COLOR` — 关闭颜色输出（遵循 <https://no-color.org>）

**退出码**：`0` 正常，`1` 一般错误，`2` 别名未找到或歧义，`3` 后端不可用，`4` 凭据操作失败；其余原样透传底层 ssh 的退出码。

---

## 元数据格式

由 `yssh add` 写入的条目带一行元数据注释：

```sshconfig
# yssh: env=prod tags=web,hk added=2026-09-18 note=主站反代
Host prod-web
    HostName 1.2.3.4
    User root
    Port 2222
```

`#` 注释会被 OpenSSH 忽略，所以这种写法零副作用。手工添加、不带注释的条目同样能正常识别，只是没有环境标记和标签。

---

## 开发与验证

### 本地单元测试

```bash
go test ./...
```

测试使用 `t.TempDir()` 与隔离的 `USERPROFILE`/`HOME`，不会触碰真实的 `~/.ssh`。

### 容器端到端验证（推荐）

```bash
docker build -t yssh-dev .
docker run --rm yssh-dev
```

**为什么端到端验证必须在 Linux 容器里跑**：核心验收项是「`yssh` 写入的配置，原生 `ssh` 能否正确解析」。验证它需要让 `ssh` 去读一份临时配置，而 **OpenSSH 是通过 passwd 数据库中的 `pw_dir` 定位 `~/.ssh/config` 的，不读 `$HOME` / `%USERPROFILE%` 环境变量**——所以无论 Windows 还是 Linux，都无法用环境变量把 `ssh` 重定向到临时目录（Windows 上实测会静默返回系统默认值，看着合理但完全错误）。

隔离 `ssh` 的配置路径只能靠容器：独立的文件系统加上独立的 passwd 数据库。脚本因此要求 `/.dockerenv` 存在才允许运行，并使用当前用户的家目录、在运行前后清理。

容器内会依次执行 `go vet` → `go test` → 20 项端到端断言（`scripts/e2e.sh`），覆盖删除保护、`Include` 指令不被破坏、用户手写内容逐字节保留、参数透传等。

---

## 项目结构

```
cmd/yssh/              命令行入口与全部子命令
cmd/ysshtray/          托盘常驻程序（GUI 子系统）
internal/session/      ~/.ssh/config 的读取、写入与 ssh -G 解析
internal/meta/         # yssh: 元数据注释的解析与生成
internal/search/       四级模糊匹配与编辑距离
internal/history/      ~/.yssh/history.json（最近使用排序）
internal/backend/      连接后端接口与 ssh 实现
internal/launcher/     把连接交给 Windows Terminal 执行
internal/tray/         托盘菜单、事件分发
internal/appicon/      图标绘制、多尺寸 ICO、Windows 资源对象
internal/install/      自安装：注册表、PATH、卸载登记
internal/dialog/       原生消息框（仅依赖 user32.dll）
internal/term/         窗口标题、颜色、显示宽度计算
docs/DESIGN.md         完整技术设计文档
docs/BRANCHING.md      分支规范与回滚手册
```

除托盘库 `fyne.io/systray` 外不引入其它第三方依赖，且以 `CGO_ENABLED=0` 编译——产物是两个单文件二进制（各约 3.2MB），不需要任何运行时。第三方组件的版权声明见 [`THIRD-PARTY-NOTICES.md`](THIRD-PARTY-NOTICES.md)。

---

## 分支与版本

- **`main`** — 正式交付版。每个 tag 都是一个经过完整验证的交付节点，可随时回滚到任一节点
- **`dev`** — 开发分支。日常开发、重构与实验都在这里，允许中间状态

发布流程、提交信息规范与**回滚操作手册**见 [`docs/BRANCHING.md`](docs/BRANCHING.md)。

---

## 状态

**已交付**

- **Step 1** — 会话库 + 四级模糊搜索 + 密钥认证连接
- **Step 2** — 托盘常驻程序 + 自安装（无需管理员权限，可开机自启）

当前版本 `v0.2.1`。后续路线见 [`docs/DESIGN.md`](docs/DESIGN.md) 第 14 章：

- **Step 3** — 密码层：Windows Credential Manager 存凭据 + `plink` 后端 + 环境色带
- **Step 4** — 自研 SSH 客户端：密码仅存内存，顺带获得 SFTP 与隧道能力
- **Step 5** — 批量导入（nmap / CSV）、批量执行、标签过滤

---

## 许可与版权

**Apache License 2.0**，全文见 [`LICENSE`](LICENSE)；著作权归 **Zhou Tianbo (Baobug)**，2026 年。

每个源文件头部都带机器可读的 SPDX 标识：

```go
// SPDX-FileCopyrightText: 2026 Zhou Tianbo (Baobug)
// SPDX-License-Identifier: Apache-2.0
```

据此可以商用、可以改完闭源、可以再分发，只需保留版权与许可声明。许可含明示的专利授权与专利报复终止条款，不含商标授权，且与 GPL-2.0 不兼容。

第三方组件 `fyne.io/systray`（Apache-2.0）、`golang.org/x/sys`（BSD-3-Clause）、`github.com/godbus/dbus/v5`（BSD-2-Clause）的代码被静态链接进分发的 exe，其版权声明与许可原文见 [`THIRD-PARTY-NOTICES.md`](THIRD-PARTY-NOTICES.md)。
