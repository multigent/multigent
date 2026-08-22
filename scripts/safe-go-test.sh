#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
API_PORT="${MULTIGENT_SAFE_API_PORT:-27893}"
TIMEOUT="${MULTIGENT_SAFE_TEST_TIMEOUT:-90s}"
GOMAX="${GOMAXPROCS:-2}"
PARALLEL="${MULTIGENT_SAFE_TEST_P:-1}"
NICE="${MULTIGENT_SAFE_NICE:-10}"

usage() {
  cat <<EOF
Usage: scripts/safe-go-test.sh [go test args...]

Runs Go tests with conservative local defaults to avoid saturating small dev boxes.

Defaults:
  GOMAXPROCS=${GOMAX}
  go test -p ${PARALLEL} -timeout ${TIMEOUT}
  nice -n ${NICE}

Examples:
  scripts/safe-go-test.sh ./internal/api -run TestRuntimeNotify -count=1
  MULTIGENT_SAFE_TEST_TIMEOUT=2m scripts/safe-go-test.sh ./internal/api
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

if [[ $# -eq 0 ]]; then
  set -- ./...
fi

echo "[safe-go-test] before: top CPU processes"
ps -eo pid,ppid,stat,pcpu,pmem,comm,args --sort=-pcpu | head -12 || true
echo

if command -v ss >/dev/null 2>&1; then
  echo "[safe-go-test] api listener on :${API_PORT}"
  ss -ltnp | grep -E ":${API_PORT}\\b" || true
  echo
fi

cd "$ROOT_DIR"
echo "[safe-go-test] running: GOMAXPROCS=${GOMAX} nice -n ${NICE} go test -p ${PARALLEL} -timeout ${TIMEOUT} $*"
GOMAXPROCS="$GOMAX" nice -n "$NICE" go test -p "$PARALLEL" -timeout "$TIMEOUT" "$@"

echo
echo "[safe-go-test] after: top CPU processes"
ps -eo pid,ppid,stat,pcpu,pmem,comm,args --sort=-pcpu | head -12 || true
