#!/usr/bin/env bash
# 生产环境启动与状态查看，在 ECS 上以 root 执行。
#
#   bash deploy/start.sh            启动后端与网关，等到接口真的应答
#   bash deploy/start.sh restart    重启后端（换过二进制后用这个）
#   bash deploy/start.sh status     只报告状态，不改动任何东西
#   bash deploy/start.sh --help     显示这段说明
#
# 进程不归这个脚本管：systemd 负责开机自启与崩溃拉起，脚本只把
# 「起服务 → 等就绪 → 报告」串成一条命令，脚本退出后进程继续跑。
# nginx 同时服务 QA-Agent，所以只会在它没跑的时候拉起来，任何情况下都不停它。
#
# 可覆盖的环境变量：SERVICE、GATEWAY、API、READY_TIMEOUT。

set -uo pipefail

SELF=$(cd "$(dirname "$0")" && pwd)/$(basename "$0")

SERVICE=${SERVICE:-personal-site.service}
GATEWAY=${GATEWAY:-nginx}
API=${API:-http://127.0.0.1:8080/api/v1/articles}
READY_TIMEOUT=${READY_TIMEOUT:-30}

step() { printf '\033[36m▸\033[0m %s\n' "$1"; }
ok()   { printf '  \033[32m✓\033[0m %s\n' "$1"; }
info() { printf '  \033[33m·\033[0m %s\n' "$1"; }
fail() { printf '  \033[31m✗\033[0m %s\n' "$1" >&2; }

action=${1:-start}
case $action in
  # 打印开头那段注释，遇到第一行非注释就停——不写死行号，改注释不会失效。
  -h|--help) awk 'NR>1 && /^#/ {sub(/^# ?/, ""); print; next} NR>1 {exit}' "$SELF"; exit 0 ;;
  start|restart|status) ;;
  *) echo "未知参数：$action（用 --help 查看用法）" >&2; exit 2 ;;
esac

if [ "$(id -u)" -ne 0 ]; then
  fail "需要 root：systemctl 与 systemd 单元都归 root"
  exit 1
fi

unit_state() { systemctl is-active "$1" 2>/dev/null || true; }

# systemd 报 active 只说明进程在，不代表它连上了数据库；接口真应答才算启动完成。
api_code() { curl -s -o /dev/null -m 3 -w '%{http_code}' "$API" 2>/dev/null || true; }

wait_for_api() {
  local deadline=$((SECONDS + READY_TIMEOUT))
  while ((SECONDS < deadline)); do
    [ "$(api_code)" = 200 ] && return 0
    sleep 0.5
  done
  return 1
}

# 失败时把服务日志末尾摊开，否则只剩一句「没起来」，看不出是配置还是数据库的问题。
dump_unit_log() {
  echo "  ── journalctl -u $SERVICE 末尾 ──"
  journalctl -u "$SERVICE" -n 12 --no-pager 2>/dev/null | sed 's/^/  │ /'
  echo "  ─────────────────────────────────"
}

report() {
  local code
  code=$(api_code)
  printf '  后端 %s：%s\n' "$SERVICE" "$(unit_state "$SERVICE")"
  printf '  网关 %s：%s\n' "$GATEWAY" "$(unit_state "$GATEWAY")"
  if [ "$code" = 200 ]; then
    info "接口 GET /api/v1/articles → 200"
  else
    info "接口 GET /api/v1/articles → ${code:-无应答}"
  fi
}

if [ "$action" = status ]; then
  step "生产环境状态"
  report
  if [ "$(unit_state "$SERVICE")" = active ] && [ "$(unit_state "$GATEWAY")" = active ] && [ "$(api_code)" = 200 ]; then
    exit 0
  fi
  exit 1
fi

# nginx 与 QA-Agent 共用，只在没跑的时候拉起来。
if [ "$action" = start ]; then
  step "启动网关 $GATEWAY"
  if [ "$(unit_state "$GATEWAY")" = active ]; then
    info "已在运行，不动它"
  elif systemctl start "$GATEWAY"; then
    ok "已启动"
  else
    fail "启动失败，检查 systemctl status $GATEWAY"
    exit 1
  fi
fi

step "启动后端 $SERVICE"
if [ "$action" = restart ]; then
  systemctl restart "$SERVICE" || { fail "重启失败，检查 systemctl status $SERVICE"; exit 1; }
  ok "已重启"
else
  if systemctl start "$SERVICE"; then
    ok "已启动"
  else
    fail "启动失败"
    dump_unit_log
    exit 1
  fi
fi

step "等待接口应答（上限 ${READY_TIMEOUT}s）"
if wait_for_api; then
  ok "接口已就绪"
else
  fail "超过 ${READY_TIMEOUT}s 仍未就绪"
  dump_unit_log
  exit 1
fi

step "当前状态"
report
