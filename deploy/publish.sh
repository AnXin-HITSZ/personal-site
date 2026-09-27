#!/usr/bin/env bash
# 生产发布，在 ECS 上以 root 执行：拉代码 → 构建后端与前端 → 换产物 → 重启 → 验证。
#
#   bash deploy/publish.sh
#   bash deploy/publish.sh --help   显示这段说明
#
# 任何一步失败都当场停下并打印原因，不会留下半成品：后端先编译到 dist/server.new，
# 编译通过才替换（同目录内 mv 是原子的，旧二进制在编译失败时原样留着）；前端在构建
# 成功之后才 rsync，并且用 --delete 保证产物目录与本次构建完全一致。
#
# 数据库迁移不在这里做——迁移由人手动执行，见 backend/migrations/README.md；
# 本次拉取若带进新的迁移文件，脚本只把文件名打出来提醒你，不代执行、也不拦发布。
#
# 可覆盖的环境变量：WEB_ROOT、SERVICE。

set -uo pipefail

SELF=$(cd "$(dirname "$0")" && pwd)/$(basename "$0")
ROOT=$(dirname "$(dirname "$SELF")")
BACKEND=$ROOT/backend
FRONTEND=$ROOT/frontend

WEB_ROOT=${WEB_ROOT:-/var/www/anxin-site/dist}
SERVICE=${SERVICE:-personal-site.service}
MIGRATIONS=$BACKEND/migrations

# 传给 deploy/start.sh，让「重启哪个单元」和「验证哪个单元」始终是同一个。
export SERVICE

step() { printf '\033[36m▸\033[0m %s\n' "$1"; }
ok()   { printf '  \033[32m✓\033[0m %s\n' "$1"; }
info() { printf '  \033[33m·\033[0m %s\n' "$1"; }
warn() { printf '  \033[33m!\033[0m %s\n' "$1"; }
fail() { printf '  \033[31m✗\033[0m %s\n' "$1" >&2; }

if [ "${1:-}" = -h ] || [ "${1:-}" = --help ]; then
  awk 'NR>1 && /^#/ {sub(/^# ?/, ""); print; next} NR>1 {exit}' "$SELF"
  exit 0
fi
if [ $# -gt 0 ]; then
  echo "未知参数：$1（用 --help 查看用法）" >&2
  exit 2
fi

elapsed() {
  local s=$SECONDS
  if ((s >= 60)); then printf '%dm%ds' $((s / 60)) $((s % 60)); else printf '%ds' "$s"; fi
}

# ECS 上 go 不在默认 PATH 里，装在哪就补哪，别让用户为了发布去改 profile。
for candidate in /usr/local/go/bin /usr/lib/go/bin; do
  [ -x "$candidate/go" ] && case ":$PATH:" in *":$candidate:"*) ;; *) PATH=$PATH:$candidate ;; esac
done

step "检查前置条件"

if [ "$(id -u)" -ne 0 ]; then
  fail "需要 root：产物目录与 systemd 都归 root"
  exit 1
fi

for cmd in git go node npm rsync systemctl curl; do
  command -v "$cmd" >/dev/null 2>&1 || { fail "找不到 $cmd，请先安装并加入 PATH"; exit 1; }
done
ok "git / go / node / npm / rsync / systemctl / curl 均在 PATH 上"

if ! git -C "$ROOT" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  fail "$ROOT 不是 git 检出，无法拉取代码"
  exit 1
fi

# .env 不入库，也不会被 pull 覆盖；缺了它后端起不来，在这里就拦下。
if [ ! -f "$BACKEND/.env" ]; then
  fail "缺 backend/.env —— 后端只读工作目录下的 .env，部署机上必须有"
  exit 1
fi
ok "backend/.env 就位"

# 只查已跟踪的文件：未跟踪的 .env、dist/ 之类是正常的，不该拦住发布。
if ! git -C "$ROOT" diff --quiet || ! git -C "$ROOT" diff --cached --quiet; then
  fail "工作区有未提交的改动，先处理再发布（git -C $ROOT status）"
  exit 1
fi
ok "工作区干净"

step "拉取代码"
before=$(git -C "$ROOT" rev-parse --short HEAD)
if ! pull=$(git -C "$ROOT" pull --ff-only 2>&1); then
  fail "git pull 失败，先处理下面这条报错再发布"
  printf '%s\n' "$pull" | sed 's/^/  │ /'
  exit 1
fi
after=$(git -C "$ROOT" rev-parse --short HEAD)

if [ "$before" = "$after" ]; then
  info "已是最新（$after），仍会重建产物"
else
  ok "$before → $after"
  git -C "$ROOT" log --oneline "$before..$after" | sed 's/^/  │ /'
fi

# 只提示、不代执行、也不拦：迁移由人手动跑，这里只保证你不会忘了它。
new_migrations=$(git -C "$ROOT" diff --name-only --diff-filter=A "$before..$after" -- "${MIGRATIONS#$ROOT/}" | grep '\.up\.sql$' || true)
if [ -n "$new_migrations" ]; then
  warn "本次拉取带进了新的迁移文件，确认已在目标库手工执行："
  printf '%s\n' "$new_migrations" | sed 's/^/  │ /'
fi

step "构建后端"
mkdir -p "$BACKEND/dist"
if ! (cd "$BACKEND" && go build -o dist/server.new ./cmd/server); then
  fail "go build 失败，dist/server 未改动"
  rm -f "$BACKEND/dist/server.new"
  exit 1
fi
mv "$BACKEND/dist/server.new" "$BACKEND/dist/server"
ok "dist/server 已更新（$(du -h "$BACKEND/dist/server" | cut -f1)）"

step "构建前端"
if ! (cd "$FRONTEND" && npm ci && npm run build); then
  fail "前端构建失败，线上产物未改动"
  exit 1
fi
if [ ! -f "$FRONTEND/dist/index.html" ]; then
  fail "构建结束但没有 $FRONTEND/dist/index.html，产物不完整"
  exit 1
fi
ok "dist/ 已生成（$(find "$FRONTEND/dist" -type f | wc -l) 个文件）"

step "同步到 $WEB_ROOT"
mkdir -p "$WEB_ROOT"
if ! rsync -a --delete "$FRONTEND/dist/" "$WEB_ROOT/"; then
  fail "rsync 失败"
  exit 1
fi
ok "线上产物已与本次构建一致"

step "重启并验证"
if ! bash "$ROOT/deploy/start.sh" restart; then
  fail "服务没起来，发布未完成；二进制已换，日志见上"
  exit 1
fi

step "发布完成"
printf '  版本 %s → %s，耗时 %s\n' "$before" "$after" "$(elapsed)"
