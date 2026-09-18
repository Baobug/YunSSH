#!/usr/bin/env bash
#
# YunSSH 端到端验证
#
# 在隔离的临时 HOME 中运行，绝不触碰真实配置。
# 覆盖 docs/DESIGN.md 第 15 章「验收标准 / Step 1」的全部条目。
#
# 之所以必须在 Linux 容器里跑：Linux 的 ssh 严格遵循 $HOME，
# 因此可以放心地把整个家目录指向临时目录，让真实的 ssh 去解析
# yssh 写入的配置。Windows 的 OpenSSH 不认 USERPROFILE，做不到这一点。
set -euo pipefail

YSSH="${YSSH:-yssh}"

# ── 隔离策略（重要）──────────────────────────────────────────────
# OpenSSH 通过 passwd 数据库中的 pw_dir 定位 ~/.ssh/config，**不读 $HOME 环境变量**。
# 因此无法靠修改 HOME 把 ssh 重定向到临时目录：实测中 `ssh -G web` 会静默
# 返回系统默认值（hostname 回显为别名本身），不加甄别会得出完全错误的结论。
# 真正的隔离由容器本身提供——这里直接使用当前用户的家目录并在运行前后清理。
if [ ! -f /.dockerenv ]; then
    echo "错误：本脚本会清理家目录下的 .ssh 与 .yssh，只允许在容器内运行。" >&2
    echo "      请使用：docker build -t yssh-dev . && docker run --rm yssh-dev" >&2
    exit 1
fi

# 让 $HOME 与 passwd 中的家目录严格一致，确保 yssh 与 ssh 指向同一份配置。
export HOME="$(eval echo "~$(id -un)")"
export NO_COLOR=1

CONFIG="$HOME/.ssh/config"
PASS_COUNT=0

FAKE_DIR=""
cleanup() { rm -rf "$HOME/.ssh" "$HOME/.yssh" "$FAKE_DIR"; }
trap cleanup EXIT
cleanup

pass() { printf '  \033[32m✓\033[0m %s\n' "$1"; PASS_COUNT=$((PASS_COUNT + 1)); }
fail() { printf '  \033[31m✗\033[0m %s\n' "$1" >&2; exit 1; }
section() { printf '\n\033[1m%s\033[0m\n' "$1"; }

section "1. 空配置的首次运行"
code=0
out=$("$YSSH" 2>&1) || code=$?
[ ! -f "$CONFIG" ] || fail "只读操作不应创建配置文件"
echo "$out" | grep -q "还没有主机条目" || fail "应给出手上引导，实际输出: $out"
pass "空配置不写文件，并给出引导"

section "2. 添加条目"
"$YSSH" add web root@1.2.3.4 --port 2222 --env prod --tags web,hk --note '主站反代' >/dev/null
[ -f "$CONFIG" ] || fail "配置文件未创建"
grep -q "^# yssh: env=prod tags=web,hk" "$CONFIG" || fail "元数据注释未写入"
grep -q "^Host web" "$CONFIG" || fail "Host 块未写入"
pass "写入条目与元数据"

"$YSSH" add lab-msf msfadmin@192.168.78.129 --env lab >/dev/null
pass "写入第二条（不同环境）"

section "3. 核心验收：原生 ssh 能正确解析 yssh 写入的配置"
host=$(ssh -G web 2>/dev/null | awk '$1=="hostname"{print $2; exit}')
port=$(ssh -G web 2>/dev/null | awk '$1=="port"{print $2; exit}')
user=$(ssh -G web 2>/dev/null | awk '$1=="user"{print $2; exit}')

[ "$host" = "1.2.3.4" ] || fail "ssh 解析 hostname 失败，得到 '$host'"
[ "$port" = "2222" ] || fail "ssh 解析 port 失败，得到 '$port'"
[ "$user" = "root" ] || fail "ssh 解析 user 失败，得到 '$user'"
pass "ssh -G 解析出 1.2.3.4 / 2222 / root"

# 这一条是整个设计成立与否的判据：
# 配置文件必须是 yssh 与原生 ssh 共同认账的唯一真相源。
got=$(ssh -G lab-msf 2>/dev/null | awk '$1=="hostname"{print $2; exit}')
[ "$got" = "192.168.78.129" ] || fail "第二条条目未被 ssh 识别，得到 '$got'"
pass "第二条条目同样被 ssh 识别"

section "4. 列表"
out=$("$YSSH")
echo "$out" | grep -q "web" || fail "列表缺少 web"
echo "$out" | grep -q "lab-msf" || fail "列表缺少 lab-msf"
echo "$out" | grep -q "prod" || fail "列表缺少环境标记"
pass "列表输出正确"

section "5. 参数透传（用桩 ssh 验证明细）"
FAKE_DIR=$(mktemp -d)
FAKE="$FAKE_DIR/fake-ssh"
cat > "$FAKE" <<'FAKEEOF'
#!/usr/bin/env bash
printf 'ARGV:'
for a in "$@"; do printf ' [%s]' "$a"; done
printf '\n'
FAKEEOF
chmod +x "$FAKE"

out=$(YSSH_SSH_BIN="$FAKE" "$YSSH" web -L 3306:127.0.0.1:3306 -N 2>&1) || true
echo "$out" | grep -q '\[web\]' || fail "别名未作为首个参数传递: $out"
echo "$out" | grep -q '\[-L\]' || fail "-L 未透传: $out"
echo "$out" | grep -q '\[3306:127.0.0.1:3306\]' || fail "-L 的取值未透传: $out"
echo "$out" | grep -q '\[-N\]' || fail "-N 未透传: $out"
pass "别名与 -L / -N 参数原样透传"

section "6. 模糊匹配"
out=$(YSSH_SSH_BIN="$FAKE" "$YSSH" wb 2>&1) || true
echo "$out" | grep -q "web" || fail "子序列匹配 wb 未命中 web: $out"
pass "子序列匹配 wb -> web"

out=$(YSSH_SSH_BIN="$FAKE" "$YSSH" msf 2>&1) || true
echo "$out" | grep -q "lab-msf" || fail "子串匹配 msf 未命中 lab-msf: $out"
pass "子串匹配 msf -> lab-msf"

section "7. 歧义与未找到"
"$YSSH" add web-01 root@10.0.0.1 --env prod >/dev/null
"$YSSH" add web-02 root@10.0.0.2 --env prod >/dev/null

code=0
out=$("$YSSH" web- 2>&1) || code=$?
[ "$code" -eq 2 ] || fail "歧义应返回退出码 2，得到 $code"
echo "$out" | grep -q "匹配到多个主机" || fail "未提示歧义: $out"
pass "歧义时列出候选并返回 2"

code=0
out=$("$YSSH" zzz-nothing 2>&1) || code=$?
[ "$code" -eq 2 ] || fail "未找到应返回退出码 2，得到 $code"
echo "$out" | grep -q "你是不是想找" || fail "未给出相近候选: $out"
pass "未找到时给出相近候选"

section "8. 删除与备份"
cat >> "$CONFIG" <<'EOF'

# 用户手工添加的条目，删除操作绝不能碰
Host handwritten
    HostName 172.16.0.1
    User someone
EOF

"$YSSH" rm lab-msf >/dev/null
grep -q "Host lab-msf" "$CONFIG" && fail "lab-msf 未被删除"
grep -q "env=lab" "$CONFIG" && fail "lab-msf 的元数据注释残留"
pass "目标条目与其元数据一并删除"

grep -q "^Host web$" "$CONFIG" || fail "误删了 web"
grep -q "Host web-01" "$CONFIG" || fail "误删了 web-01"
grep -q "Host handwritten" "$CONFIG" || fail "误删了用户手工添加的条目"
grep -q "# 用户手工添加的条目" "$CONFIG" || fail "误删了用户的注释"
pass "其他条目与用户注释完整保留"

ls "$HOME/.yssh/backup/" 2>/dev/null | grep -q "^config\." || fail "未生成备份"
pass "删除前已生成备份"

section "9. 共享 Host 行的删除保护"
cat >> "$CONFIG" <<'EOF'

Host shared-a shared-b
    HostName 10.5.5.5
EOF

code=0
out=$("$YSSH" rm shared-a 2>&1) || code=$?
[ "$code" -ne 0 ] || fail "多别名共享行应拒绝删除"
echo "$out" | grep -q "多个别名" || fail "未给出拒绝原因: $out"
grep -q "Host shared-a shared-b" "$CONFIG" || fail "被拒绝的删除改动了文件"
pass "拒绝删除并保持文件不变"

section "10. Include 指令不被破坏"
mkdir -p "$HOME/.ssh/conf.d"
printf 'Host extra\n    HostName 10.9.9.9\n' > "$HOME/.ssh/conf.d/extra.conf"
printf '\nInclude conf.d/*.conf\n' >> "$CONFIG"

"$YSSH" add after-include root@10.0.0.1 >/dev/null
grep -q "Include conf.d/\*.conf" "$CONFIG" || fail "追加写入破坏了 Include 指令"
pass "追加写入不影响 Include 指令"

"$YSSH" rm after-include >/dev/null
grep -q "Include conf.d/\*.conf" "$CONFIG" || fail "删除操作破坏了 Include 指令"
pass "删除操作不影响 Include 指令"

section "11. 重复添加被拒绝"
before=$(cat "$CONFIG")
code=0
out=$("$YSSH" add web root@9.9.9.9 2>&1) || code=$?
[ "$code" -ne 0 ] || fail "重复别名应返回非零退出码"
[ "$(cat "$CONFIG")" = "$before" ] || fail "失败的添加改动了文件"
echo "$out" | grep -q "已存在" || fail "未提示别名已存在: $out"
pass "拒绝重复别名且不改动文件"

section "12. show 输出（走真实的 ssh -G）"
out=$("$YSSH" show web 2>&1) || true
echo "$out" | grep -q "1.2.3.4" || fail "show 未输出解析后的主机地址: $out"
echo "$out" | grep -q "2222" || fail "show 未输出端口: $out"
echo "$out" | grep -q "主站反代" || fail "show 未输出备注: $out"
pass "show 输出 ssh 解析结果与元数据"

section "13. 输出重定向时不含控制序列"
out=$("$YSSH" ls 2>/dev/null)
if printf '%s' "$out" | grep -q $'\x1b'; then
    fail "列表输出中混入了 ANSI 转义序列"
fi
pass "重定向输出干净，无转义序列"

printf '\n\033[32m%s 项端到端验证全部通过\033[0m\n' "$PASS_COUNT"
