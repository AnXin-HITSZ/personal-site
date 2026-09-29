#!/usr/bin/env bash
# 本地开发一键启动：SSH 隧道 → Go 后端 → Vite 前端。
#
#   bash dev.sh                 三个都起
#   bash dev.sh --only-db       只起隧道并保持前台（要用 mysqlsh 连库时）
#   bash dev.sh --no-tunnel     隧道已在跑时，只起后端与前端
#   bash dev.sh --no-frontend   只起隧道与后端（调接口用）
#   bash dev.sh --only-frontend 只起前端（调界面用，不碰数据库）
#   bash dev.sh --no-install    依赖缺失时不自动安装，只报错退出
#   bash dev.sh --stop          收回三个端口（清理上次没收干净的遗留进程）
#
# 已经在跑的环节不会被重复拉起，也不会被本脚本停掉——退出时只关它自己起的进程。
# 三者都未就绪前的失败会打印对应日志的末尾若干行，日志目录在结束时一并给出。
#
# Ctrl+C 或关掉终端窗口都会触发收尾；只有被强杀（SIGKILL）时才收不到，
# 那种情况留下的进程用 --stop 收回。

set -uo pipefail

# 先把自身解析成绝对路径再去 cd，否则 `cd frontend && bash ../dev.sh` 之后
# 相对路径就再也指不回这个文件了。
SELF=$(cd "$(dirname "$0")" && pwd)/$(basename "$0")
ROOT=$(dirname "$SELF")
cd "$ROOT" || exit 1

SSH_ALIAS=${SSH_ALIAS:-aliyun-ecs}      # ~/.ssh/config 里的别名，见 README
TUNNEL_PORT=${TUNNEL_PORT:-13306}       # 本机隧道端口
DB_PORT=${DB_PORT:-3306}                # ECS 上 MySQL 的端口
HTTP_ADDR=${HTTP_ADDR:-127.0.0.1:8080}
HTTP_PORT=${HTTP_ADDR##*:}
FRONTEND_PORT=${FRONTEND_PORT:-5173}
READY_TIMEOUT=${READY_TIMEOUT:-60}      # 每个环节的等待上限（秒）

WANT_TUNNEL=1 WANT_BACKEND=1 WANT_FRONTEND=1 WANT_INSTALL=1 WANT_STOP=0 ONLY_DB=0 ONLY_FRONTEND=0
for arg in "$@"; do
  case $arg in
    --only-db)       ONLY_DB=1 ;;
    --only-frontend) ONLY_FRONTEND=1 ;;
    --no-tunnel)     WANT_TUNNEL=0 ;;
    --no-backend)    WANT_BACKEND=0 ;;
    --no-frontend)   WANT_FRONTEND=0 ;;
    --no-install)    WANT_INSTALL=0 ;;
    --stop)          WANT_STOP=1 ;;
    # 打印开头那段注释，遇到第一行非注释就停——不写死行号，改注释不会失效。
    -h|--help)       awk 'NR>1 && /^#/ {sub(/^# ?/, ""); print; next} NR>1 {exit}' "$SELF"; exit 0 ;;
    *) echo "未知参数：$arg（用 --help 查看用法）" >&2; exit 2 ;;
  esac
done

# 放在循环之后推导，参数写的先后顺序就不影响结果。
if [ "$ONLY_DB" = 1 ]; then
  WANT_BACKEND=0
  WANT_FRONTEND=0
fi
# 纯前端不碰数据库，隧道也就没有存在的理由：留着它只会让人以为后端也在跑。
if [ "$ONLY_FRONTEND" = 1 ]; then
  WANT_TUNNEL=0
  WANT_BACKEND=0
fi

STARTED=()
STARTED_NAMES=()

step()  { printf '\033[36m▸\033[0m %s\n' "$1"; }
ok()    { printf '  \033[32m✓\033[0m %s\n' "$1"; }
skip()  { printf '  \033[33m·\033[0m %s\n' "$1"; }
fail()  { printf '  \033[31m✗\033[0m %s\n' "$1" >&2; }

# /dev/tcp 是 bash 内建的重定向，不需要 nc 或 telnet。
port_open() { (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null; }

# 只认 127.0.0.1 上的监听：同一个端口号绑在别的地址上是别人的东西，不动。
port_owner() {
  netstat -ano 2>/dev/null | grep LISTENING | grep "127.0.0.1:$1 " | awk '{print $NF}' | head -1
}

proc_name() {
  tasklist //FI "PID eq $1" //NH //FO CSV 2>/dev/null | head -1 | cut -d, -f1 | tr -d '"'
}

# taskkill 的 // 前缀是给 MSYS 的：写成 /PID 会被转换成 Windows 路径。
stop_ports() {
  local port pid name found=0
  for port in "$@"; do
    pid=$(port_owner "$port")
    if [ -z "$pid" ]; then
      skip "端口 $port 无监听"
      continue
    fi
    found=1
    name=$(proc_name "$pid")
    if taskkill //PID "$pid" //F >/dev/null 2>&1; then
      ok "端口 $port ← 已停止 $name（PID $pid）"
    else
      fail "端口 $port ← 停不掉 $name（PID $pid），可能需要管理员权限"
    fi
  done
  [ "$found" = 1 ] || echo "  三个端口都是空的，没有要清理的东西。"
}

wait_for_port() {
  local port=$1 deadline=$((SECONDS + READY_TIMEOUT))
  while ((SECONDS < deadline)); do
    port_open "$port" && return 0
    sleep 0.3
  done
  return 1
}

# 等到某行日志出现；进程先死返回 2，超时返回 1，成功返回 0。
wait_for_log() {
  local file=$1 pattern=$2 pid=$3 deadline=$((SECONDS + READY_TIMEOUT))
  while ((SECONDS < deadline)); do
    grep -q "$pattern" "$file" 2>/dev/null && return 0
    kill -0 "$pid" 2>/dev/null || return 2
    sleep 0.3
  done
  return 1
}

# 失败时把日志末尾摊开，否则用户只能看到一个「失败了」。
dump_log() {
  local file=$1
  [ -s "$file" ] || { fail "（$file 为空）"; return; }
  echo "  ── $(basename "$file") 末尾 ──"
  tail -n 12 "$file" | sed 's/^/  │ /'
  echo "  ─────────────────────────"
}

track() { STARTED+=("$1"); STARTED_NAMES+=("$2"); }

cleanup() {
  trap - EXIT INT TERM HUP
  local i pid
  for i in "${!STARTED[@]}"; do
    pid=${STARTED[$i]}
    kill "$pid" 2>/dev/null && ok "已停止 ${STARTED_NAMES[$i]}（PID $pid）"
  done
  sleep 1
  # Git Bash 的 kill 打到原生 Windows 进程上是强杀，没有优雅退出的余地；
  # 后端在 dev 下只读数据库，不存在丢写的风险。
  for i in "${!STARTED[@]}"; do
    pid=${STARTED[$i]}
    kill -0 "$pid" 2>/dev/null && kill -9 "$pid" 2>/dev/null
  done
  wait 2>/dev/null
  # 前置检查就失败时一个日志都没写过，这时说「日志保留在…」是误导，直接撤掉空目录。
  if [ -s "$TUNNEL_LOG" ] || [ -s "$BACKEND_LOG" ] || [ -s "$FRONTEND_LOG" ]; then
    echo "日志保留在 $LOG_DIR"
  else
    rmdir "$LOG_DIR" 2>/dev/null
  fi
}

# ── 只清理，不启动 ──────────────────────────────────────────────────────
# 放在前置检查与日志目录之前：只想收回端口的人，不该被要求先装好 go/node，
# 也不该因为一个 EXIT trap 就凭空多出一个没人看的日志目录。

if [ "$WANT_STOP" = 1 ]; then
  step "收回端口 $TUNNEL_PORT / $HTTP_PORT / $FRONTEND_PORT"
  stop_ports "$TUNNEL_PORT" "$HTTP_PORT" "$FRONTEND_PORT"
  exit 0
fi

# 一个环节都不起，走完全部前置检查再报一句「无需等待」是白费一轮。--only-db 配上
# --no-tunnel 也落在这里：隧道被关掉之后它就没东西可起了。
if [ "$WANT_TUNNEL" = 0 ] && [ "$WANT_BACKEND" = 0 ] && [ "$WANT_FRONTEND" = 0 ]; then
  fail "隧道、后端、前端都被关掉了，没有可启动的环节"
  # --only-db 与 --only-frontend 互相抵消：前者关掉前端，后者关掉隧道与后端。
  if [ "$ONLY_DB" = 1 ] && [ "$ONLY_FRONTEND" = 1 ]; then
    echo "     --only-db 与 --only-frontend 不能同时用" >&2
  elif [ "$ONLY_DB" = 1 ]; then
    echo "     --only-db 起的就是隧道，不能再加 --no-tunnel" >&2
  elif [ "$ONLY_FRONTEND" = 1 ]; then
    echo "     --only-frontend 起的就是前端，不能再加 --no-frontend" >&2
  fi
  exit 2
fi

LOG_DIR=$(mktemp -d) || exit 1
TUNNEL_LOG=$LOG_DIR/tunnel.log
BACKEND_LOG=$LOG_DIR/backend.log
FRONTEND_LOG=$LOG_DIR/frontend.log

# HUP 也要接：直接关掉终端窗口走的是 SIGHUP，只接 INT 的话进程会留下来占端口。
trap cleanup EXIT INT TERM HUP

# ── 前置检查 ────────────────────────────────────────────────────────────

step "检查前置条件"

# 只查本次真正会用到的命令：--only-db 的人不该被要求先装好 go 和 node。
# （三个都不起的情况上面已经拦掉了，所以这里至少有一项。）
REQUIRED_CMDS=()
[ "$WANT_TUNNEL" = 1 ]   && REQUIRED_CMDS+=(ssh)
[ "$WANT_BACKEND" = 1 ]  && REQUIRED_CMDS+=(go)
[ "$WANT_FRONTEND" = 1 ] && REQUIRED_CMDS+=(node)

for cmd in "${REQUIRED_CMDS[@]}"; do
  command -v "$cmd" >/dev/null 2>&1 || { fail "找不到 $cmd，请先安装并加入 PATH"; exit 1; }
done
JOINED=$(printf '%s / ' "${REQUIRED_CMDS[@]}")
ok "${JOINED% / } 在 PATH 上"

# 这两项说的都是数据库链路，--only-frontend 压根不碰库，就不该被它们拦下。
if [ "$WANT_TUNNEL" = 1 ] || [ "$WANT_BACKEND" = 1 ]; then
  if [ ! -f "$ROOT/backend/.env" ]; then
    fail "缺 backend/.env —— 从 backend/.env.example 复制后填入数据库连接信息"
    exit 1
  fi
  ok "backend/.env 存在"

  # 只读端口，不碰 .env 里的密码；这里也绝不打印密码。
  ENV_DB_PORT=$(grep -E '^MYSQL_PORT=' "$ROOT/backend/.env" | head -1 | cut -d= -f2 | tr -d '[:space:]')
  if [ -n "$ENV_DB_PORT" ] && [ "$ENV_DB_PORT" != "$TUNNEL_PORT" ]; then
    fail "backend/.env 的 MYSQL_PORT=$ENV_DB_PORT，但隧道将监听 $TUNNEL_PORT —— 后端会连不上库"
    echo "     改 .env 的 MYSQL_PORT=$TUNNEL_PORT，或用 MYSQL_PORT=$ENV_DB_PORT TUNNEL_PORT=$ENV_DB_PORT bash dev.sh" >&2
    exit 1
  fi
  ok "backend/.env 的 MYSQL_PORT=$ENV_DB_PORT 与隧道端口一致"
fi

if [ "$WANT_FRONTEND" = 1 ] && [ ! -d "$ROOT/frontend/node_modules" ]; then
  if [ "$WANT_INSTALL" = 1 ]; then
    step "安装前端依赖（缺 frontend/node_modules）"
    (cd "$ROOT/frontend" && npm ci) || { fail "npm ci 失败"; exit 1; }
    ok "依赖安装完成"
  else
    fail "缺 frontend/node_modules，请先在 frontend/ 执行 npm ci"
    exit 1
  fi
fi

# ── SSH 隧道 ────────────────────────────────────────────────────────────

if [ "$WANT_TUNNEL" = 1 ]; then
  step "SSH 隧道 127.0.0.1:$TUNNEL_PORT → $SSH_ALIAS:$DB_PORT"
  if port_open "$TUNNEL_PORT"; then
    skip "端口 $TUNNEL_PORT 已在监听，复用现有隧道（退出时不会关掉它）"
  else
    # ExitOnForwardFailure：转发建不起来就让 ssh 立刻失败，而不是装作成功。
    ssh -N \
      -o ExitOnForwardFailure=yes \
      -o ServerAliveInterval=30 \
      -o ServerAliveCountMax=3 \
      -L "127.0.0.1:$TUNNEL_PORT:127.0.0.1:$DB_PORT" \
      "$SSH_ALIAS" >"$TUNNEL_LOG" 2>&1 &
    TUNNEL_PID=$!
    track "$TUNNEL_PID" "SSH 隧道"
    if wait_for_port "$TUNNEL_PORT"; then
      ok "隧道就绪（PID $TUNNEL_PID）"
    else
      fail "等待隧道超时"
      dump_log "$TUNNEL_LOG"
      exit 1
    fi
  fi
else
  step "SSH 隧道"
  # 后端也不起时，隧道没有服务对象，「假定已有隧道」就成了一句空话。
  if [ "$ONLY_FRONTEND" = 1 ]; then skip "--only-frontend：跳过"
  elif [ "$WANT_BACKEND" = 0 ]; then skip "本次没有后端要用它，跳过"
  else skip "--no-tunnel：假定已有隧道"; fi
fi

# ── 后端 ────────────────────────────────────────────────────────────────

if [ "$WANT_BACKEND" = 1 ]; then
  step "Go 后端 http://$HTTP_ADDR"
  if port_open "$HTTP_PORT"; then
    skip "端口 $HTTP_PORT 已在监听，复用现有服务"
  else
    # 先 go build 再直接跑二进制，而不是 go run：
    # go run 会先编译到临时目录再以子进程运行，kill 掉父进程会留下子进程占着端口。
    step "编译后端"
    if ! (cd "$ROOT/backend" && go build -o dist/server.exe ./cmd/server) ; then
      fail "go build 失败"
      exit 1
    fi
    ok "编译通过（backend/dist/server.exe）"

    # CWD 必须是 backend/：config.Load() 只读当前工作目录的 .env。
    (cd "$ROOT/backend" && exec ./dist/server.exe) >"$BACKEND_LOG" 2>&1 &
    BACKEND_PID=$!
    track "$BACKEND_PID" "Go 后端"

    # 后端启动时会 ping 一次 MySQL，所以「连接检查通过」就是数据库链路的真凭据，
    # 比只看 8080 端口更能说明问题——它已经确认过隧道能通到库了。
    wait_for_log "$BACKEND_LOG" "连接检查通过" "$BACKEND_PID"
    case $? in
      0)
        if wait_for_log "$BACKEND_LOG" "listening on" "$BACKEND_PID"; then
          ok "后端就绪（PID $BACKEND_PID），MySQL 连接检查通过"
        else
          fail "MySQL 通过了，但服务没起来"
          dump_log "$BACKEND_LOG"; exit 1
        fi ;;
      2) fail "后端启动即退出，多半是隧道或库连不上"
         dump_log "$BACKEND_LOG"; exit 1 ;;
      *) fail "等待后端超时"
         dump_log "$BACKEND_LOG"; exit 1 ;;
    esac
  fi
else
  step "Go 后端"
  if [ "$ONLY_DB" = 1 ]; then skip "--only-db：跳过"
  elif [ "$ONLY_FRONTEND" = 1 ]; then skip "--only-frontend：跳过"
  else skip "--no-backend：跳过"; fi
fi

# ── 前端 ────────────────────────────────────────────────────────────────

if [ "$WANT_FRONTEND" = 1 ]; then
  step "Vite 前端 http://localhost:$FRONTEND_PORT"
  if port_open "$FRONTEND_PORT"; then
    skip "端口 $FRONTEND_PORT 已在监听，复用现有服务（--strictPort 下不会再开一个）"
  else
    # 直接跑 vite 的入口，不经 npm/npx：npm 是 .cmd 包一层 node，
    # kill 掉 npm 不会连带杀掉 vite，端口会被占住。
    (cd "$ROOT/frontend" && exec node node_modules/vite/bin/vite.js \
      --host 127.0.0.1 --port "$FRONTEND_PORT" --strictPort) >"$FRONTEND_LOG" 2>&1 &
    FRONTEND_PID=$!
    track "$FRONTEND_PID" "Vite 前端"
    if wait_for_port "$FRONTEND_PORT"; then
      ok "前端就绪（PID $FRONTEND_PID）"
    else
      fail "等待前端超时"
      dump_log "$FRONTEND_LOG"
      exit 1
    fi
  fi
else
  step "Vite 前端"
  if [ "$ONLY_DB" = 1 ]; then skip "--only-db：跳过"; else skip "--no-frontend：跳过"; fi
fi

# ── 就绪 ────────────────────────────────────────────────────────────────

echo
# 没被本脚本启动的环节不印地址：--only-db 下报一个 8080 的接口地址是误导。
[ "$WANT_FRONTEND" = 1 ] && echo "  前端    http://localhost:$FRONTEND_PORT"
[ "$WANT_BACKEND" = 1 ]  && echo "  接口    http://$HTTP_ADDR/api/v1/articles?page=1&pageSize=6"
# 隧道那行标的是数据库在哪，但只有后端在跑时它才指着什么活着的东西。
if [ "$WANT_TUNNEL" = 1 ] || [ "$WANT_BACKEND" = 1 ]; then
  echo "  隧道    127.0.0.1:$TUNNEL_PORT → $SSH_ALIAS:$DB_PORT"
fi
# 全部复用现有进程时一个日志都没写，别报一个空目录出来。
[ ${#STARTED[@]} -gt 0 ] && echo "  日志    $LOG_DIR"
# 前端在 5173 上把 /api 转给 8080。这次没起后端，那些请求会没人应答，
# 页面落到错误态是意料之中——先说清楚，免得当成前端坏了。
if [ "$WANT_TUNNEL" = 0 ] && [ "$WANT_BACKEND" = 0 ]; then
  echo
  echo "  本次没有起后端：接口请求无人应答，要读真实数据的页面会显示各自的失败态。"
  echo "  只想看界面的话，在 frontend/.env.local 写 VITE_DATA_SOURCE=mock 再重启。"
fi
echo
echo "  按 Ctrl+C 停止本脚本启动的进程。"
echo "  端口被上次的遗留进程占着时，用 bash dev.sh --stop 收回。"

# 前台等着，让 Ctrl+C 有东西可中断。
if [ ${#STARTED[@]} -gt 0 ]; then
  wait
else
  echo
  echo "  本次没有启动任何进程，无需等待。"
fi
