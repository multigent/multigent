#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
DATA_DIR="${MULTIGENT_DATA_DIR:-/root/code/spaceship/multigent_e2e}"
API_ADDR="${MULTIGENT_API_ADDR:-127.0.0.1:27893}"
WEB_HOST="${MULTIGENT_WEB_HOST:-127.0.0.1}"
WEB_PORT="${MULTIGENT_WEB_PORT:-27894}"
GO_BIN="${GO_BIN:-/root/.goenv/shims/go}"
NODE_BIN_DIR="${NODE_BIN_DIR:-/root/.nvm/versions/node/v24.16.0/bin}"
SUPERVISOR_API_NAME="${SUPERVISOR_API_NAME:-multigent-api}"
SUPERVISOR_WEB_NAME="${SUPERVISOR_WEB_NAME:-multigent-web}"
USE_SUPERVISOR="${USE_SUPERVISOR:-auto}"
RESTART_WEB=0
BUILD_WEB=0

usage() {
  cat <<EOF
Usage: internal/dev/restart-local.sh [options]

Build and restart the local Multigent API service.

Options:
  --web                       Also restart the local Vite web server.
  --build-web                 Run npm --prefix web run build before restart.
  --supervisor                Force restart through supervisorctl.
  --no-supervisor             Force direct process restart without supervisorctl.
  --supervisor-api-name <n>   Supervisor API program. Default: ${SUPERVISOR_API_NAME}
  --supervisor-web-name <n>   Supervisor Web program. Default: ${SUPERVISOR_WEB_NAME}
  --data-dir <path>           Data dir. Default: ${DATA_DIR}
  --api-addr <addr>           API bind addr. Default: ${API_ADDR}
  --web-host <host>           Web host. Default: ${WEB_HOST}
  --web-port <port>           Web port. Default: ${WEB_PORT}
  -h, --help                  Show this help.

Environment:
  MULTIGENT_DATA_DIR, MULTIGENT_API_ADDR, MULTIGENT_WEB_HOST, MULTIGENT_WEB_PORT,
  GO_BIN, NODE_BIN_DIR, USE_SUPERVISOR, SUPERVISOR_API_NAME, SUPERVISOR_WEB_NAME
EOF
}

kill_matching_processes() {
  local pattern="$1"
  local self="$$"
  local parent="${PPID:-}"
  local pids pid
  pids="$(pgrep -f "$pattern" 2>/dev/null || true)"
  if [[ -z "$pids" ]]; then
    return 0
  fi
  while read -r pid; do
    [[ -z "$pid" ]] && continue
    [[ "$pid" == "$self" || "$pid" == "$parent" ]] && continue
    kill "$pid" 2>/dev/null || true
  done <<< "$pids"
}

supervisor_program_exists() {
  local name="$1"
  command -v supervisorctl >/dev/null 2>&1 || return 1
  supervisorctl status "$name" >/dev/null 2>&1
}

should_use_supervisor() {
  local name="$1"
  case "$USE_SUPERVISOR" in
    1|true|yes|on) return 0 ;;
    0|false|no|off) return 1 ;;
    auto) supervisor_program_exists "$name" ;;
    *)
      echo "invalid USE_SUPERVISOR=${USE_SUPERVISOR}; expected auto/true/false" >&2
      exit 2
      ;;
  esac
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --web)
      RESTART_WEB=1
      shift
      ;;
    --build-web)
      BUILD_WEB=1
      shift
      ;;
    --supervisor)
      USE_SUPERVISOR=true
      shift
      ;;
    --no-supervisor)
      USE_SUPERVISOR=false
      shift
      ;;
    --supervisor-api-name)
      SUPERVISOR_API_NAME="${2:-}"
      shift 2
      ;;
    --supervisor-web-name)
      SUPERVISOR_WEB_NAME="${2:-}"
      shift 2
      ;;
    --data-dir)
      DATA_DIR="${2:-}"
      shift 2
      ;;
    --api-addr)
      API_ADDR="${2:-}"
      shift 2
      ;;
    --web-host)
      WEB_HOST="${2:-}"
      shift 2
      ;;
    --web-port)
      WEB_PORT="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

cd "$ROOT_DIR"
mkdir -p dist .multigent/dev

echo "==> building dist/multigent"
"$GO_BIN" build -o dist/multigent ./cmd/multigent

if [[ "$BUILD_WEB" == "1" ]]; then
  echo "==> building web"
  npm --prefix web run build
fi

if should_use_supervisor "$SUPERVISOR_API_NAME"; then
  echo "==> restarting supervisor program: ${SUPERVISOR_API_NAME}"
  supervisorctl restart "$SUPERVISOR_API_NAME"
else
  echo "==> stopping existing local API processes"
  kill_matching_processes "dist/multigent .*api serve --addr ${API_ADDR}"
  kill_matching_processes "dist/multigent --dir ${DATA_DIR} api serve"
  sleep 0.3
  echo "==> starting API on http://${API_ADDR}"
  setsid -f ./dist/multigent --dir "$DATA_DIR" api serve --addr "$API_ADDR" \
    > .multigent/dev/api.log 2>&1
fi

if [[ "$RESTART_WEB" == "1" ]]; then
  if should_use_supervisor "$SUPERVISOR_WEB_NAME"; then
    echo "==> restarting supervisor program: ${SUPERVISOR_WEB_NAME}"
    supervisorctl restart "$SUPERVISOR_WEB_NAME"
  else
    echo "==> stopping existing Vite web server on ${WEB_HOST}:${WEB_PORT}"
    kill_matching_processes "vite --host ${WEB_HOST} --port ${WEB_PORT}"
    kill_matching_processes "npm --prefix web run dev -- --host ${WEB_HOST} --port ${WEB_PORT}"
    sleep 0.3
    echo "==> starting web on http://${WEB_HOST}:${WEB_PORT}"
    PATH="${NODE_BIN_DIR}:${PATH}" setsid -f npm --prefix web run dev -- --host "$WEB_HOST" --port "$WEB_PORT" \
      > .multigent/dev/web.log 2>&1
  fi
fi

echo "==> status"
if command -v supervisorctl >/dev/null 2>&1; then
  supervisorctl status "$SUPERVISOR_API_NAME" "$SUPERVISOR_WEB_NAME" 2>/dev/null || true
fi
pgrep -af "dist/multigent .*api serve|vite --host ${WEB_HOST} --port ${WEB_PORT}" || true
echo "API log: ${ROOT_DIR}/.multigent/dev/api.log"
if [[ "$RESTART_WEB" == "1" ]]; then
  echo "Web log: ${ROOT_DIR}/.multigent/dev/web.log"
  echo "Supervisor API log: ${ROOT_DIR}/.multigent/dev/supervisor-api.stderr.log"
  echo "Supervisor Web log: ${ROOT_DIR}/.multigent/dev/supervisor-web.stderr.log"
fi
