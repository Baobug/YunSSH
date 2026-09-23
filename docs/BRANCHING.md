# 分支与版本管理

定义 YunSSH 的分支模型、提交规范、发布流程与回滚操作。目标是**任何时候都能安全回到某个已知可用状态**。

---

## 1. 分支模型

| 分支 | 定位 | 规则 |
|---|---|---|
| `main` | **正式交付版** | 只接受来自 `dev` 的合并；每个合并节点打 tag |
| `dev` | **开发分支** | 功能开发、重构、实验都在这里，允许中间状态 |

```
v0.1.0        v0.1.1
  │             │
──●─────────────●──────►  main   每个 ● 是一个交付节点
   ╲           ╱
    ●─●─●─●─●─●         dev    日常提交，粒度细
```

**核心约定**

- `main` 上每个提交都是可交付状态：`go vet` / `go test` / 端到端验证全部通过
- `main` 不直接提交，改动一律经 `dev` 合并进来
- 合并到 `main` 一律用 `--no-ff`，让每个交付节点在历史里显式可见——**这是「一键回滚到某个版本」的前提**

---

## 2. 提交信息规范

采用 [Conventional Commits](https://www.conventionalcommits.org/)：

```
<类型>(<范围>): <简短描述>

<详细说明：为什么这么改，而非改了什么>
```

| 类型 | 用途 |
|---|---|
| `feat` | 新功能 |
| `fix` | 缺陷修复 |
| `refactor` | 重构（不改外部行为） |
| `test` | 测试 |
| `docs` | 文档 |
| `chore` | 构建、依赖、工具链 |
| `release` | 交付合并（仅用于 `dev` → `main`） |

范围用包名：`meta` / `session` / `search` / `backend` / `cli` / `term`。

**为什么坚持**：`git log --oneline --grep '^fix'` 能立刻列出所有修复；`git log v0.1.0..v0.1.1` 能精确看出两个交付版本之间的改动。

---

## 3. 版本与标签

语义化版本 `v<主>.<次>.<修订>`：不兼容变更进主位，新功能进次位，修复与文档进修订位。

标签用**附注标签**（`git tag -a`），带发布说明，`git show v0.1.0` 才能看到当时的交付内容。

| 标签 | 对应阶段 |
|---|---|
| `v0.1.x` | Step 1 —— 会话库 + 模糊搜索 + 密钥认证 |
| `v0.2.x` | Step 2 —— 托盘常驻程序 + 自安装 |
| `v0.3.x` | Step 3 —— 密码层（Credential Manager + plink） |
| `v0.4.x` | Step 4 —— 自研 SSH 客户端 |
| `v0.5.x` | Step 5 —— 批量导入与批量执行 |

> **Linux 支持的定位（2026-09-23 补充）**
>
> 上表的 `v0.2.x`「托盘常驻程序」**仅指 Windows**。Linux 版以 CLI 形态随 `for linux/`
> 的构建与安装脚本分发，**托盘暂缓**（原因见 [`DESIGN.md`](DESIGN.md) §19.8），
> 因此也不提供开机自启——Linux 侧没有常驻进程可启动。
>
> 两个平台共用**同一份 Go 源码与同一份 `~/.ssh/config`**，平台差异全部由 build tag
> 分流（`xxx_windows.go` / `xxx_other.go`），**不设独立分支，也不 fork**。

Step 的完整定义见 [`README.md`](../README.md) 的「状态」一节。

---

## 4. 开发流程

```bash
git checkout dev
git pull --ff-only

# 一个逻辑变更一个提交
git add <相关文件>
git commit -m 'feat(session): 支持 Include 指令下的块定位'

# 推送前本地验证
go vet ./... && go test ./...
docker build -t yssh-dev . && docker run --rm yssh-dev

git push origin dev
```

---

## 5. 发布流程

```bash
git checkout main
git pull --ff-only

git merge --no-ff dev -m 'release: v0.2.1 —— 图标修复与许可规范化

<发布说明：新增/修复了什么，验证状态>'

git tag -a v0.2.1 -m 'Step 2 修订：多尺寸应用图标与许可规范化'

git push origin main
git push origin v0.2.1
```

**发布前检查清单**

- [ ] `go vet ./...` 无输出
- [ ] `go test ./...` 全绿
- [ ] 容器端到端验证通过
- [ ] `README.md` 与 `docs/DESIGN.md` 已同步本次改动
- [ ] 版本号与改动性质相符

---

## 6. 回滚手册

按影响范围从小到大排列，**优先选影响最小的方案**。

### 6.1 只想看旧版本，不改变当前状态

```bash
git checkout v0.1.0                  # 临时切到标签，看完 git checkout main 回来

git worktree add ../YunSSH-v0.1.0 v0.1.0   # 更安全：开独立工作区，不动当前目录
```

### 6.2 撤销本地未推送的提交

```bash
git reset --soft HEAD~1    # 撤销提交，改动留在暂存区（最常用）
git reset HEAD~1           # 撤销提交，改动回到工作区
git reset --hard HEAD~1    # 连改动一起丢弃（危险，不可恢复）
```

### 6.3 回滚已推送的提交（安全，不重写历史）

公开仓库**不要用 `--force` 推送**，会破坏他人的克隆。用 `revert` 生成反向提交：

```bash
git revert <commit-hash>

git revert --no-commit v0.1.0..v0.1.1   # 撤销一个范围
git commit -m 'revert: 回滚 v0.1.1 的改动'
```

### 6.4 把 main 整体回退到某个交付版本

```bash
# 方式一（推荐，保留历史）
git checkout main
git checkout v0.1.0 -- .        # 用 v0.1.0 的文件覆盖工作区
git commit -m 'revert: 将 main 回退到 v0.1.0'

# 方式二（重写历史，仅当确定无人基于 main 工作时）
git reset --hard v0.1.0
git push --force-with-lease origin main
```

### 6.5 找回误删的分支或提交

```bash
git reflog                       # 所有 HEAD 移动记录，含已删分支的最后位置
git branch recovered <hash>      # 从 reflog 恢复
```

### 6.6 恢复单个被覆盖的文件

```bash
git checkout v0.1.0 -- internal/session/session.go
git log --oneline -- internal/session/session.go      # 看该文件的演变
git diff v0.1.0 HEAD -- internal/session/session.go
```

### 6.7 对比两个交付版本

```bash
git log --oneline v0.1.0..v0.1.1
git diff --stat v0.1.0 v0.1.1
```

### 6.8 本地仓库损坏时的恢复

远程仓库是**权威副本**，本地 `.git` 丢失或损坏时直接重建：

```bash
git init -b main
git remote add origin https://github.com/Baobug/YunSSH.git
git fetch origin main
git reset --hard FETCH_HEAD     # 工作区对齐远程
git fetch origin dev
git checkout -b dev FETCH_HEAD  # 恢复 dev 分支
```

注意：工作区中未提交、且不在远程的改动**无法通过此方式找回**——这正是要勤提交、勤推送的原因。

---

## 7. 关于分支可见性

**GitHub 没有分支级可见性控制**——同一仓库的所有分支共用仓库的可见性。

| 仓库可见性 | `main` | `dev` |
|---|---|---|
| Public | 所有人可见 | **同样所有人可见**（`/tree/dev` 可访问） |
| Private | 仅协作者 | 仅协作者 |

若需要「交付版公开、开发分支不公开」，只有两条路：

**方案一：`dev` 不推送，只留本地。**
代价是失去远程备份——换机器或磁盘故障会丢失全部开发历史，与「方便回滚」的目标直接冲突。**不推荐**。

**方案二：双远程。** `main` 推公开仓库，`dev` 推私有仓库：

```bash
git remote add public  https://github.com/Baobug/YunSSH.git
git push public main --tags

git remote add private https://github.com/Baobug/YunSSH-dev.git
git push private dev
```

注意：从 `dev` 合并到 `main` 后，推到 `public` 的**提交历史里会包含 dev 的全部提交**（因为合并保留了分叉）。若要连提交历史也不外露，需改用 squash 合并：

```bash
git checkout main
git merge --squash dev
git commit -m 'release: v0.2.1 —— 图标修复'
```

代价是 `main` 失去细粒度回溯能力（只能回滚到版本节点，不能回滚单个功能提交）。

**当前状态**：仓库为 Public，`main` 与 `dev` 均已推送，两者都对外可见。如需变更请参照上述两条路径。

---

## 8. 本机环境注意事项

Windows 上 `core.autocrlf=true` 会在检出时把文件转成 CRLF，导致 shell 脚本在 Linux 容器中报 `bad interpreter`。仓库已用 `.gitattributes` 锁定为 LF，**不要删除该文件**。

另外，OpenSSH 通过 passwd 数据库定位 `~/.ssh/config`，**不读 `$HOME` / `%USERPROFILE%` 环境变量**（详见 `docs/DESIGN.md` 第 15.3 节），因此涉及 ssh 配置解析的验证必须在容器内进行。
