# YunSSH 代码审查报告

- **审查日期**：2026-09-21
- **审查范围**：`cmd/**`、`internal/**`、`tools/**`、`project.go`、`build.bat`、`Dockerfile`、`scripts/`
- **代码规模**：Go 源文件 47 个，共 7952 行（非测试 6678 行 + 测试 1274 行，含 7 个测试文件）
- **基线状态**：`go vet ./...` 无告警；`go test ./...` 全绿（5 个包有测试，9 个包无测试）
- **审查方法**：全量通读 + 静态分析 + 针对可疑路径编写临时探针测试实测验证（探针已删除，未改动任何业务代码）

---

## 一、总体评价

**结论：这是一个完成度高、工程纪律明显好于同规模个人项目的代码库，但存在一个会被真实触发的静默数据损坏缺陷和一处配置注入面，建议优先修复后再进入批量导入（DESIGN §10.5）阶段。**

做得好的地方（值得保持）：

1. **分层与依赖注入干净**。`session`（读写配置）/ `backend`（拼命令）/ `launcher`（交给谁跑）/ `tray`（UI）四层职责单一；`tray.App` 把外部依赖全部做成函数字段（`Hosts`、`OpenConfig`、`Uninstall`…），托盘逻辑因此可以在没有 Windows 消息循环的情况下单测——这是在很多同规模项目里看不到的自觉。
2. **注释质量极高**。注释写的不是"这行做了什么"，而是"为什么必须这样写"：`install/shortcut.go` 讲清了为什么必须 `SHParseDisplayName` 拿 IDList、`install/registry.go` 讲清了 PATH 必须用 `REG_EXPAND_SZ`、否则会"悄悄改坏别人的环境"、`internal/appicon/rsrc.go` 讲清了 COFF 重定位如何把段内偏移换算成 RVA。这些是踩过坑才写得出来的内容，是本项目最有价值的资产之一。
3. **核心不变量有测试钉住**。`TestAddPreservesUserContent`（追加后用户原内容逐字节不变）、`TestRemovePreservesOthers`（元数据不留孤儿）、`TestRenderParseRoundTrip`、`TestEmbeddedLegalContent`（许可文本没被改坏）——这四条正是这个项目最容易悄悄退化、且退化后没有外部症状的地方，选得很准。
4. **写入路径考虑周到**：删除即备份 + mtime 冲突检测 + 临时文件 rename 落盘，且对"多别名共享同一 Host 行"主动拒绝删除而不是误伤兄弟别名。
5. **合规处理到位**：许可文本 `go:embed` 进二进制、安装时写出、`yssh license --full` 随时可查，单文件分发也满足 Apache-2.0 第 4 条与 BSD 第 2 条。

主要风险集中在**输入校验**与**模块间行为不一致**两处：核心写入层几乎不校验字段内容，而命令行解析层对 IPv6 的处理存在实测可复现的错误。

---

## 二、严重问题（建议立即修复）

### S1. 配置字段未过滤换行 → 可向 `~/.ssh/config` 注入任意指令

**位置**：`internal/session/write.go:51`（`renderBlock`）、`write.go:33`（`Add` 的校验只覆盖别名）
**连带面**：`cmd/yssh/cmd_host.go:141`（`addHost` 对 `--env/--tags/--note` 无校验）

`Add` 只对 `Alias` 做了 `strings.ContainsAny(h.Alias, " \t*?!=")` 检查，其余字段（HostName、User、IdentityFile，以及 Meta 的 Env/Tags/Note）原样拼进文本行。

**实测结果**（临时探针，`Add` 返回 `nil`）：

```go
cfg.Add(Host{Alias: "web2", HostName: "1.2.3.4\n    StrictHostKeyChecking no", User: "root"})
```

写入磁盘得到：

```
Host web2
    HostName 1.2.3.4
    StrictHostKeyChecking no      ← 一条真实生效的指令
    User root
```

**为什么严重**：这直接击穿了 README 与 `docs/DESIGN.md` 铁律二「安全默认值不妥协——绝不默认 `StrictHostKeyChecking=no`」。攻击者只要能影响 `yssh add` 的入参（脚本批量添加、CI 里拼参数、或 DESIGN §10.5 规划的"从 nmap/CSV 批量导入"——那份数据天生是不可信的外部输入），就能静默关掉主机指纹校验，为中间人攻击铺路。同理可注入 `ProxyCommand`、`ForwardAgent yes` 等。

**建议**：

```go
// write.go — Add 开头，单一入口统一拦截
func rejectControlChars(what, s string) error {
    if strings.ContainsAny(s, "\r\n\x00") {
        return fmt.Errorf("%s 不能包含换行或控制字符", what)
    }
    return nil
}
```

- 在 `Config.Add` 里对 `Alias / HostName / User / IdentityFile` 逐个校验；
- `meta.Render` 前对 `Env / Tags / Note` 同样校验（`--note` 是"取值到行尾"的字段，风险等同）；
- CLI 层 `parseTarget` 与 `addOptions` 再校验一次（纵深防御）；
- 补表驱动测试：断言含 `\n` 的 HostName / Note 一律返回错误且文件不被改动。

---

### S2. 裸 IPv6 地址被静默解析成错误的主机和端口

**位置**：`cmd/yssh/cmd_host.go:348`（`parseTarget`）

现有逻辑是"取最后一个冒号、其后全数字即端口"，外加"含 `]` 则整体跳过端口解析"这两条互相打架的规则。函数注释写着"这样 IPv6 字面量不会被误切"，实测恰恰是被误切了。

**实测结果**：

| 输入 | 解析出的 host | 解析出的 port | 是否报错 |
|---|---|---|---|
| `root@1.2.3.4:2222` | `1.2.3.4` | 2222 | 否 ✅ |
| `2001:db8::1` | **`2001:db8:`** | **`1`** | **否** ❌ |
| `[2001:db8::1]:22` | `[2001:db8::1]:22` | 0 | 否 ❌ |
| `root@1.2.3.4:99999` | `1.2.3.4` | **99999** | 否 ❌ |

第一行是**静默数据损坏**：`yssh add v6 2001:db8::1` 会往配置里写 `HostName 2001:db8:` + `Port 1`，用户拿到的是一个连不通、且看不出哪里错的条目。第二行的 HostName 带了方括号和端口，OpenSSH 不接受这种写法。

**建议**（改写 `parseTarget` 的端口分支）：

```go
host = rest
// 1) 带方括号的 IPv6：[::1]:22 —— 端口只可能出现在 ] 之后
if strings.HasPrefix(rest, "[") {
    if end := strings.Index(rest, "]"); end >= 0 {
        host = rest[:end+1]
        if rest[end+1:] == ":" {
            host = rest[:end+1]
        } else if strings.HasPrefix(rest[end+1:], ":") {
            n, err := strconv.Atoi(rest[end+2:]) // 失败则明确报错，不静默
            ...
        }
    }
} else if strings.Count(rest, ":") == 1 { // 2) 无括号时，只允许恰好一个冒号
    ...
} else if strings.Count(rest, ":") > 1 {
    return "", "", 0, fmt.Errorf("IPv6 地址请加方括号，如 [2001:db8::1]:22")
}
```

配套：端口统一校验 `1..65535`（见 M6），并补单测覆盖上表全部四行。

---

### S3. 别名可以以 `-` 开头 → 向 ssh 注入命令行选项

**位置**：`internal/session/write.go:37`（Add 校验）、`internal/backend/ssh.go:29`（`Build` 把别名直接作为 ssh 第一个参数）

`Add` 只拒绝 `空格 \t * ? ! =`，不拒绝前导 `-`。**实测** `Add` 接受 `-v`、`-L8080:localhost:80` 两个别名。

**后果**：

- 自伤场景：`yssh -L8080:localhost:80` 会被 ssh 当成端口转发选项，而不是主机名。
- 真正危险的场景：`Add` 的校验只作用于"yssh 自己添加的条目"，**手工编辑或他人共享的 config 不受约束**。一份含 `Host -oProxyCommand=<任意命令>` 的配置（例如从同事/仓库拿来的 dotfiles），在 `Config.Find` 后照常被 `Build` 原样交给 `ssh` → **任意命令执行**。对一个主打"管理 SSH 会话"的工具来说，配置来源不可信是默认 Threat Model，不是边缘情况。

**建议**：

1. `Add` 的别名校验增加：不以 `-` 开头，且匹配 `^[A-Za-z0-9][A-Za-z0-9._@-]*$`；
2. 在 `connectAlias`（`cmd/yssh/cmd_connect.go:104`）与 `tray.connect`（`internal/tray/tray.go:230`）**连接前**再校验一次别名，不合格直接报错退出，而不是交给 ssh；
3. 更彻底的做法是让 `SSH.Build` 只接受已校验过的别名（把校验放进 `session.Host` 的构造路径），从类型上消灭这条路径。

---

## 三、中等问题

### M1. `launcher` 与 `session` 的 ssh 定位逻辑各写一套，行为不一致

**位置**：`internal/launcher/launch.go:67`（硬编码 `"ssh"`）、`launch.go:75`（`exec.LookPath("ssh")`）

两者都没有走 `session.SSHBin()`，而后者才有 `YSSH_SSH_BIN` 覆盖 + `C:\Windows\System32\OpenSSH\ssh.exe` 兜底。

**后果**：环境变量 `YSSH_SSH_BIN` 对托盘点击**完全无效**；在 ssh 不在 PATH、只存在于 System32 目录的机器上，CLI 能连上、托盘点了没反应——同一个工具两套行为，排障时会非常困惑。

**建议**：两个函数统一改为调用 `session.SSHBin()`，wt 那侧传解析出的绝对路径而非 `"ssh"` 字面量。

### M2. 经 `cmd.exe` 启动进程，参数未做元字符防护

**位置**：`internal/launcher/launch.go:81`、`cmd/ysshtray/main.go:379`

`cmd /c start "" <exe> <alias> <extra...>` 与 `cmd /c start "" <config路径>` 都要过一遍 cmd 的命令行解析，参数里的 `& | ^ % "` 会被解释。（Go 的 `exec.Command` 在 Windows 上会自动给含空格的参数加引号，所以"路径含空格"这一条是安全的，这一点代码没问题。）

**建议**：改用 `ShellExecuteW`（`shell32`）或 `explorer` 打开文件/启动进程，彻底绕开 cmd 解析；如必须保留 `cmd /c start`，至少对参数做 `^` 转义。

### M3. 卸载会递归删除注册表记录的目录，缺少合法性校验

**位置**：`internal/install/install.go:240`（`rmdir /s /q "<dir>"`）、`cmd/yssh/cmd_install_windows.go:107`

`dir` 取自 HKCU 下的 `InstallLocation`。HKCU 对同用户的任意进程可写；而「设置 → 应用」走的是 `QuietUninstallString ... uninstall --yes`，**不弹确认**。

**后果**：一旦 `InstallLocation` 被改写（恶意软件、误操作、或残留的旧版本记录），`yssh uninstall` 会无确认地递归删除那个目录。

**建议**：生成删除脚本前校验——目录内确实存在 `ysshtray.exe` 或 `yssh.exe`、且不是盘符根/系统目录/用户主目录；不满足就拒绝并给出明确提示。

### M4. `ssh -G` 调用无超时，可能挂死

**位置**：`internal/session/resolve.go:60`、`cmd/yssh/cmd_host.go:261`

`ssh -G` 会执行配置里的 `Match ... exec` 与部分 `ProxyCommand` 求值，异常配置下可长时间阻塞；当前 `exec.Command` 没有 context。

**建议**：改 `exec.CommandContext`，加 3~5 秒超时并在超时时给出可诊断的提示。

### M5. `cmd/yssh` 主包零测试，最关键的解析逻辑裸露

`parseTarget`、`splitList`、`cmdAdd` 选项解析、`renderTable`、`sortCandidates` 全部无测试。S2 的 IPv6 缺陷正是藏在这里——而 `session`、`meta`、`search`、`tray`、`install` 五个包都有不错的测试覆盖，反差明显。`internal/launcher`、`backend`、`history`、`term`、`appicon` 同样零覆盖。

**建议**：优先给 `parseTarget` 与 `cmdAdd` 选项解析补表驱动测试（这两个最易回归），再补 `launcher`（可注入 `YSSH_SSH_BIN` 桩程序）。

### M6. 端口范围校验只在 `--port` 路径上

`cmdAdd` 校验了 `--port` 的 `1..65535`，但 `target` 串里的 `:port` 完全不校验（实测 `root@1.2.3.4:99999` 直接写入配置，ssh 后续必然拒绝连接）。**建议**：把校验上移到 `addHost`，两条路径共用一个检查点。

### M7. 备份目录无上限增长

每次 `yssh rm` 在 `~/.yssh/backup/` 留一份完整 config，永不清理。内容是完整主机清单（属于资产信息），长期会积累成百上千份。**建议**：保留最近 10 份，或按月轮转。

### M8. 历史记录非原子写且不清理已删除别名

`history.Save()`（`internal/history/history.go:100`）直接 `WriteFile`，CLI 与托盘同时写会互相踩踏（损坏后虽会自动重建，但会丢统计）；已删除的别名永久留在 `history.json` 里。**建议**：改为临时文件 + rename；`Load` 后剔除配置中已不存在的别名。

---

## 四、轻微问题

| # | 位置 | 问题 | 建议 |
|---|---|---|---|
| L1 | `cmd/yssh/main.go:32-33` | `exitUsage` 与 `exitError` 同为 1，常量名暗示不同语义 | 加注释说明，或真的区分成不同码 |
| L2 | `session/write.go:227` vs `history/history.go:35` | `stateDir()` 与 `history.Dir()` 重复实现 `~/.yssh` | 抽到共享的 `internal/paths` |
| L3 | `appicon/ico.go:115` vs `install/shortcut.go:329` | `writeU16/writeU32` 各写一份 | 抽公共内部包 |
| L4 | `cmd_host.go:315` vs `cmd/ysshtray/main.go:371` | 配置骨架文本重复，且文案不一致（"由 yssh 创建" / "由 YunSSH 创建"） | 统一到一处 |
| L5 | `cmd_host.go:329` | `exec.Command(editor, path)`，`EDITOR="code --wait"` 会失败 | 按空格拆分或给出提示 |
| L6 | `cmd_host.go:58-79` | `--key` 不校验文件存在；`--env/--tags/--note` 不校验控制字符 | 见 S1 的统一校验 |
| L7 | `cmd_install_windows.go:168` | `askYesNo` 每次新建 `bufio.Reader`，多次询问时缓冲残留被丢弃 | 提为参数传入 |
| L8 | `search/search.go:88` | 注释称"最小跨度"，实为贪心左匹配，不保证最小（`q="ab", s="a x a b"` 得 4，最优 2） | 改注释或改算法 |
| L9 | `term/term.go:72` | `runeWidth` 未处理组合字符（应 0）与 emoji（应 2） | 按需补齐，或注明适用范围 |
| L10 | `cmd_connect.go:142` | ssh 失败（退出码 255）后仍 `Touch` 记入"最近使用" | 仅在成功时记录 |
| L11 | `backend/backend.go:13` | `Request` 的 HostName/User/Port/Env 被填充但 `SSH.Build` 完全不用；单实现接口 + 恒定 `Select` 属过度设计 | 若短期不接第二后端，可先用函数替代接口 |
| L12 | `session/write.go:184` | 临时文件名固定为 `config.yssh-tmp`，并发写互相踩踏，异常退出会残留 | 加 PID/随机后缀，失败时清理 |
| L13 | `session/write.go:190` | Windows 上 rename 后的新文件继承目录 ACL，用户手工加固过的 config 权限会被放宽 | 写后显式收紧 ACL |
| L14 | `session/session.go:105` | `crlf` 是全局标志，混合换行的文件会向"多数派"靠拢 | 改为按最后一行判断 |
| L15 | `install/install.go:276` | `copyFile` 无校验、无备份，中断即留下损坏 exe | 先写 `.new` 再替换 |
| L16 | `tray/tray.go:170` | `rebuildHosts` 持锁期间做文件 I/O；`watchConnect` 未处理 `ClickedCh` 被关闭的情况（若 systray 在 `Remove()` 后关闭通道，select 会空转并反复触发连接） | I/O 移出锁；`case _, ok := <-ch; if !ok { return }` |
| L17 | `appicon/icon.go:63` | 每次托盘启动重新光栅化 8 个尺寸（约 145 万次采样 + 一次 256² PNG 编码），多花约百毫秒 | `sync.OnceValue` 缓存 |
| L18 | 仓库根 | 无 GitHub Actions / CI 配置，只有 `Dockerfile` + `scripts/ci.sh`，需手动执行 | 加一条 workflow 跑 `go vet` + `go test` |
| L19 | `Dockerfile:28` | 注释称"本项目零第三方依赖"，但 `go.mod` 实际依赖 systray / x/sys / dbus（只是 Linux 构建未用到） | 改为"CLI 构建无需第三方依赖" |
| L20 | `install/registry.go:77` | 卸载登记项缺 `EstimatedSize` / `InstallDate` / `HelpLink`；`quote()` 不转义内嵌引号 | 补全 |
| L21 | `cmd_install_windows.go:30` | `cmdInstall` 静默忽略未知参数（如 `--foo`），与 `cmdAdd` 的严格校验不一致 | 统一为报错 |

---

## 五、优先修复建议

按"风险 × 成本"排序，建议按此顺序推进：

| 优先级 | 项目 | 预估工作量 | 理由 |
|---|---|---|---|
| **P0** | S1 字段换行注入 | 小（一个校验函数 + 测试） | 击穿项目自己写下的安全铁律；批量导入功能上线后危害放大 |
| **P0** | S2 IPv6 解析 | 小（重写端口分支 + 测试） | 实测静默写坏配置，用户无从察觉 |
| **P1** | S3 别名前导 `-` | 小（加校验 + 连接前复核） | 配置来源不可信是默认场景，属 RCE 面 |
| **P1** | M5 `cmd/yssh` 补测试 | 中 | S2 正是因为这里没测试才潜伏至今 |
| **P2** | M1 ssh 定位统一 | 小 | 托盘与 CLI 行为不一致，直接可见的体验缺陷 |
| **P2** | M4 `ssh -G` 超时 | 小 | 可靠性，卡死无任何提示 |
| **P2** | M6 端口校验统一 | 极小 | 与 S2 同一处改动，顺手做完 |
| **P3** | M2 / M3 / M7 / M8 | 中 | 健壮性与资源治理 |
| **P4** | L1–L21 | 小 | 清理债，可批量处理 |

**一个建议**：S1、S2、M6 三处都落在"写入前的输入校验"与"目标串解析"上，可以合并成一次改动——在 `addHost` 建立统一的校验入口，并同步补 `cmd/yssh` 的测试。这样一轮就能消掉两个 P0 和一个 P2。

---

## 六、审查方式说明

本次审查未修改任何业务代码。为验证可疑路径，我在 `cmd/yssh` 与 `internal/session` 下各临时创建了探针测试（验证 `parseTarget` 的 IPv6/端口行为、别名校验边界、换行注入是否可行），取得实测结果后已全部删除，仓库当前处于审查前的干净状态（`go vet` 与 `go test` 均通过）。报告中所有"实测"结论均可在上述位置复现。
