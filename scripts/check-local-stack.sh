#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WEB_PORT="${MULTIGENT_WEB_PORT:-27894}"
API_PORT="${MULTIGENT_API_PORT:-27893}"
WEB_ADDR="${MULTIGENT_WEB_ADDR:-127.0.0.1:${WEB_PORT}}"
API_ADDR="${MULTIGENT_API_ADDR:-0.0.0.0:${API_PORT}}"
DATA_DIR="${MULTIGENT_DATA_DIR:-/root/code/spaceship/multigent_e2e}"
WEB_LOG="${MULTIGENT_WEB_LOG:-/tmp/multigent-web-${WEB_PORT}.log}"
API_LOG="${MULTIGENT_API_LOG:-/tmp/multigent-api-${API_PORT}.log}"

usage() {
  cat <<EOF
Usage:
  scripts/check-local-stack.sh [check|restart]

Default mode is "check".

Environment overrides:
  MULTIGENT_WEB_PORT   default: ${WEB_PORT}
  MULTIGENT_API_PORT   default: ${API_PORT}
  MULTIGENT_DATA_DIR   default: ${DATA_DIR}
  MULTIGENT_WEB_LOG    default: ${WEB_LOG}
  MULTIGENT_API_LOG    default: ${API_LOG}

What it verifies:
  - 27894 is a Vite web dev server
  - 27893 is a multigent api serve process
  - /api/v1/knowledge-base/items returns 401 or 200, not 404
  - /api/v1/knowledge-base/sources returns 401 or 200, not 404

What restart does:
  - kills the current web/api listeners on 27894/27893
  - rebuilds multigent if needed
  - restarts web and api with the expected commands
EOF
}

mode="${1:-check}"
if [[ "$mode" == "-h" || "$mode" == "--help" ]]; then
  usage
  exit 0
fi
if [[ "$mode" != "check" && "$mode" != "restart" ]]; then
  echo "Unknown mode: $mode" >&2
  usage >&2
  exit 1
fi

web_pid() {
  lsof -t -iTCP:"$WEB_PORT" -sTCP:LISTEN 2>/dev/null || true
}

api_pid() {
  lsof -t -iTCP:"$API_PORT" -sTCP:LISTEN 2>/dev/null || true
}

show_listener() {
  local port="$1"
  local pid
  pid="$(lsof -t -iTCP:"$port" -sTCP:LISTEN 2>/dev/null | head -n1 || true)"
  if [[ -z "$pid" ]]; then
    echo "port ${port}: not listening"
    return 0
  fi
  echo "port ${port}:"
  ps -p "$pid" -o pid=,lstart=,cmd=
}

http_code() {
  local url="$1"
  shift || true
  curl -sS -o /dev/null -w '%{http_code}' "$@" "$url" || echo "000"
}

check_endpoint() {
  local label="$1"
  local url="$2"
  local code
  code="$(http_code "$url")"
  printf '%s -> %s\n' "$label" "$code"
  case "$code" in
    200|401)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

restart_stack() {
  local wpid apid
  wpid="$(web_pid)"
  apid="$(api_pid)"

  if [[ -n "$wpid" ]]; then
    echo "[restart] stopping web pid=$wpid"
    kill "$wpid" || true
  fi
  if [[ -n "$apid" ]]; then
    echo "[restart] stopping api pid=$apid"
    kill "$apid" || true
  fi

  sleep 1

  echo "[restart] rebuilding backend binary"
  (
    cd "$ROOT_DIR"
    make build-go
  )

  echo "[restart] starting api on ${API_ADDR}"
  (
    cd "$ROOT_DIR"
    nohup ./dist/multigent --dir "$DATA_DIR" api serve --addr "$API_ADDR" >"$API_LOG" 2>&1 &
  )

  echo "[restart] starting web on ${WEB_ADDR}"
  (
    cd "$ROOT_DIR/web"
    nohup npm run dev -- --host 127.0.0.1 --port "$WEB_PORT" >"$WEB_LOG" 2>&1 &
  )

  sleep 2
}

check_stack() {
  local wpid apid
  wpid="$(web_pid)"
  apid="$(api_pid)"

  echo "[check] listeners"
  show_listener "$WEB_PORT"
  show_listener "$API_PORT"

  echo
  echo "[check] processes"
  if [[ -n "$wpid" ]]; then
    ps -p "$wpid" -o pid=,lstart=,cmd=
  else
    echo "web process missing"
  fi
  if [[ -n "$apid" ]]; then
    ps -p "$apid" -o pid=,lstart=,cmd=
  else
    echo "api process missing"
  fi

  echo
  echo "[check] api routes"
  local fail=0
  check_endpoint "27894 /api/v1/knowledge-base/items" "http://127.0.0.1:${WEB_PORT}/api/v1/knowledge-base/items?limit=1" || fail=1
  check_endpoint "27894 /api/v1/knowledge-base/sources" "http://127.0.0.1:${WEB_PORT}/api/v1/knowledge-base/sources?limit=1" || fail=1
  check_endpoint "27893 /api/v1/knowledge-base/items" "http://127.0.0.1:${API_PORT}/api/v1/knowledge-base/items?limit=1" || fail=1
  check_endpoint "27893 /api/v1/knowledge-base/sources" "http://127.0.0.1:${API_PORT}/api/v1/knowledge-base/sources?limit=1" || fail=1

  echo
  if [[ "$fail" -eq 0 ]]; then
    echo "[check] ok: both routes are wired through the live backend"
  else
    echo "[check] failed: one or more endpoints returned 404/other unexpected status"
    echo "         if the API binary was rebuilt, restart both web and api together."
    return 1
  fi
}

case "$mode" in
  restart)
    restart_stack
    check_stack
    ;;
  check)
    check_stack
    ;;
esac
