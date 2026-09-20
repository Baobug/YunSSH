# YunSSH

Windows 终端的 SSH 会话管理工具。

把 `ssh root@1.2.3.4 -p 2222` 变成 `yssh prod-web`。

```console
$ yssh
ENV   ALIAS      TARGET                   TAGS     LAST
prod  prod-web   root@1.2.3.4:2222        web,hk   2h ago
lab   lab-msf    msfadmin@192.168.78.129           just now

$ yssh prod-web
→ prod-web
```

---

## 它是什么

一个**加速录入层**：快速检索、增删主机条目，然后把终端完整交给系统 ssh 客户端。

- 不替代 `ssh`——连接时执行的就是系统自带客户端，终端行为与原生完全一致
- 不保存密码（当前版本）——认证走密钥
- 不自建配置格式——所有数据都存在 `~/.ssh/config` 里

编译产物是两个单文件二进制（各约 3.2MB），不需要任何运行时。

---

## 设计原则

1. **`~/.ssh/config` 是唯一真相源** — 元数据以注释内嵌其中，删条目时元数据同步消失，不留孤儿
2. **安全默认值不妥协** — 绝不默认 `StrictHostKeyChecking=no`，保留 ssh 原生的主机指纹确认
3. **不做终端仿真** — 连接交给系统 ssh 与 Windows Terminal，`vim`、`tmux`、`Ctrl+C` 语义天然正确

完整铁律与取舍见 [`docs/DESIGN.md`](docs/DESIGN.md) 第 3 章。

---

## 安装

### 双击安装（推荐）

双击 `ysshtray.exe` 确认即可，**全程无需管理员权限**。

| 动作 | 位置 |
|---|---|
| 复制程序 | `%LOCALAPPDATA%\Programs\YunSSH\` |
| 登记卸载项 | 「设置 → 应用」中可见 |
| 开始菜单快捷方式 | `%APPDATA%\Microsoft\Windows\Start Menu\Programs\YunSSH.lnk` |
| 开机自启（可选） | `HKCU\...\CurrentVersion\Run` |
| 加入 PATH | `HKCU\Environment\Path` |

安装后按 `Win` 键搜 `YunSSH` 即可找到，托盘图标随即出现。

### 命令行安装

```bash
yssh install                    # 交互询问是否开机自启
yssh install --autostart        # 直接开启
yssh install --no-autostart --no-path
yssh uninstall                  # 卸载
yssh autostart on|off           # 随时切换开机自启
```

重复执行 `install` 即为升级（同名文件直接覆盖）。

### 从源码构建

需要 Go 1.21+。仓库根的 `build.bat` 会生成图标与版本资源、从最新 tag 取版本号注入，再产出两个 exe：

```bash
build.bat
```

直接 `go build ./cmd/yssh` 也能编译，但不会有图标和版本资源，版本号也停在 `0.0.0-dev`——这是刻意的，好让临时构建一眼可辨。

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
搜索主机… / 打开配置文件 / 刷新列表
开机自启（可勾选）
关于 / 退出
```

菜单按「环境 → 别名」排序，未标注环境的条目排在最后。带 `[env]` 前缀是有意为之：不展开子菜单也能一眼分辨生产与靶机。

**托盘与 CLI 共用同一份 `~/.ssh/config`**——命令行里 `yssh add` 添加的主机会立刻出现在托盘菜单中，反之亦然。也因为写入的是标准配置，这些条目对原生 `ssh`、`scp`、`rsync`、`git`、VSCode Remote-SSH 同样立即可见。

连接默认在 **Windows Terminal 的新标签页**打开，未安装时自动回退到系统默认方式。

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
| `yssh license [--full]` | 查看版权与第三方组件；`--full` 打印许可全文 |

**add 选项**：`--port` `--key` `--env` `--tags` `--note`（`--note` 必须放最后，可含空格）

**透传参数**：第一个参数之后的内容原样传给 ssh：

```bash
yssh prod-web -L 3306:127.0.0.1:3306      # 本地端口转发
yssh prod-web -N -L 8080:localhost:80     # 常驻隧道
yssh prod-web -- ls -la /var/log          # 远端执行命令
```

自有选项一律用长选项，短选项（`-p` `-L` `-N` `-v` `-D` `-R`）全部保留给底层 ssh。

**环境变量**：`YSSH_SSH_BIN` 覆盖 ssh 路径；`NO_COLOR` 关闭颜色输出（遵循 [no-color.org](https://no-color.org)）。

退出码约定见 [`docs/DESIGN.md`](docs/DESIGN.md) 第 8.5 节。

---

## 元数据格式

`yssh add` 写入的条目带一行元数据注释：

```sshconfig
# yssh: env=prod tags=web,hk added=2026-09-18 note=主站反代
Host prod-web
    HostName 1.2.3.4
    User root
    Port 2222
```

`#` 注释会被 OpenSSH 忽略，零副作用。手工添加的条目无需注释，同样能正常识别。

---

## 状态

**已交付**：Step 1（会话库 + 四级模糊搜索 + 密钥认证）、Step 2（托盘常驻 + 自安装）

当前版本 `v0.2.4`。后续路线见 [`docs/DESIGN.md`](docs/DESIGN.md) 第 14 章：密码层 → 自研 SSH 客户端 → 批量运维。

开发分支模型与回滚方式见 [`docs/BRANCHING.md`](docs/BRANCHING.md)。

---

## 许可与版权

**Apache License 2.0**，全文见 [`LICENSE`](LICENSE)；著作权归 **Zhou Tianbao (Baobug)**，2026 年。

每个源文件头部带 SPDX 标识，据此可商用、可修改后闭源、可再分发，只需保留版权与许可声明。

**版权信息跟着二进制走**，不依赖使用者手上有没有仓库：

- 两个 exe 内嵌 Windows 版本资源，「属性 → 详细信息」里能看到版权、产品名与版本号
- `LICENSE` 与 `THIRD-PARTY-NOTICES.md` 以 `go:embed` 编入二进制，安装时一并写到安装目录
- 随时可用 `yssh license` 看摘要、`yssh license --full` 打印全文——**即使只拿到一个 exe**

第三方组件 `fyne.io/systray`（Apache-2.0）、`golang.org/x/sys`（BSD-3-Clause）、`github.com/godbus/dbus/v5`（BSD-2-Clause）被静态链接进分发的 exe，其版权声明与许可原文见 [`THIRD-PARTY-NOTICES.md`](THIRD-PARTY-NOTICES.md)。
