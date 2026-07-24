#!/usr/bin/env bash
# Benchmark the first-install path for Multigent sandbox runtimes.
#
# Safe by default: it does not remove existing Docker images or volumes.
# Use --cold --yes to simulate a new machine by removing the selected runtime
# image and the shared agent CLI toolchain volume before measuring.

set -euo pipefail

IMAGE=""
REGION=""
TOOLCHAINS=("codex" "claudecode")
COLD=0
YES=0
SKIP_CLIS=0
VERIFY_CONTAINER=0

usage() {
  cat <<'EOF'
Usage:
  scripts/bench-first-install.sh [options]

Options:
  --region cn          Use the official mainland China runtime mirror.
  --image IMAGE        Runtime image to pull and prepare.
  --toolchain NAME     Toolchain to warm. Repeatable. Defaults: codex, claudecode.
  --skip-clis          Only pull/check the runtime image.
  --verify-container   Also run a tiny container startup check.
  --cold               Remove selected runtime image and multigent-toolchains first.
  --yes                Required with --cold.
  -h, --help           Show help.

Examples:
  scripts/bench-first-install.sh
  scripts/bench-first-install.sh --region cn
  scripts/bench-first-install.sh --region cn --cold --yes
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --region)
      REGION="${2:-}"; shift 2 ;;
    --image)
      IMAGE="${2:-}"; shift 2 ;;
    --toolchain)
      if [ "${#TOOLCHAINS[@]}" -eq 2 ] && [ "${TOOLCHAINS[0]}" = "codex" ] && [ "${TOOLCHAINS[1]}" = "claudecode" ]; then
        TOOLCHAINS=()
      fi
      TOOLCHAINS+=("${2:-}"); shift 2 ;;
    --skip-clis)
      SKIP_CLIS=1; shift ;;
    --verify-container)
      VERIFY_CONTAINER=1; shift ;;
    --cold)
      COLD=1; shift ;;
    --yes)
      YES=1; shift ;;
    -h|--help)
      usage; exit 0 ;;
    *)
      echo "unknown option: $1" >&2
      usage >&2
      exit 2 ;;
  esac
done

if [ "$COLD" = "1" ] && [ "$YES" != "1" ]; then
  echo "--cold removes the selected Docker image and multigent-toolchains volume." >&2
  echo "Re-run with --cold --yes if that is what you want." >&2
  exit 2
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is not installed or not in PATH" >&2
  exit 1
fi

if ! docker info >/dev/null 2>&1; then
  echo "docker daemon is not reachable. Start Docker Desktop or Docker Engine first." >&2
  exit 1
fi

if [ -n "${MULTIGENT_BIN:-}" ]; then
  MULTIGENT="$MULTIGENT_BIN"
elif command -v multigent >/dev/null 2>&1; then
  MULTIGENT="$(command -v multigent)"
elif [ -x "./dist/multigent" ]; then
  MULTIGENT="./dist/multigent"
elif [ -x "./dist/multigent-current" ]; then
  MULTIGENT="./dist/multigent-current"
else
  echo "multigent binary not found. Set MULTIGENT_BIN=/path/to/multigent." >&2
  exit 1
fi

if [ -z "$IMAGE" ]; then
  if [ "$REGION" = "cn" ]; then
    IMAGE="crpi-fu3b7e7lggtmh7za.cn-hangzhou.personal.cr.aliyuncs.com/multigent/runtime-base:latest"
  else
    IMAGE="ghcr.io/multigent/multigent/runtime-base:latest"
  fi
fi

step_start=0
begin_step() {
  echo
  echo "==> $*"
  step_start="$(date +%s)"
}

end_step() {
  local end
  end="$(date +%s)"
  echo "✓ took $((end - step_start))s"
}

echo "Multigent first-install benchmark"
echo "multigent : $MULTIGENT"
echo "docker    : $(command -v docker)"
echo "image     : $IMAGE"
if [ "$SKIP_CLIS" = "1" ]; then
  echo "toolchain : skipped"
else
  echo "toolchain : ${TOOLCHAINS[*]}"
fi
echo "cold      : $COLD"

if [ "$COLD" = "1" ]; then
  begin_step "Reset selected image and toolchain cache"
  docker image rm "$IMAGE" >/dev/null 2>&1 || true
  docker volume rm multigent-toolchains >/dev/null 2>&1 || true
  end_step
fi

begin_step "Inspect remote manifest"
docker manifest inspect "$IMAGE" >/dev/null
end_step

begin_step "Pull runtime image"
docker pull "$IMAGE"
end_step

prepare_args=(sandbox prepare --image "$IMAGE" --skip-pull)
if [ "$SKIP_CLIS" = "1" ]; then
  prepare_args+=(--skip-clis)
else
  for t in "${TOOLCHAINS[@]}"; do
    prepare_args+=(--toolchain "$t")
  done
fi

begin_step "Warm sandbox runtime"
"$MULTIGENT" "${prepare_args[@]}"
end_step

if [ "$VERIFY_CONTAINER" = "1" ]; then
  begin_step "Verify runtime container starts"
  if command -v timeout >/dev/null 2>&1; then
    timeout 30 docker run --rm "$IMAGE" /bin/sh -lc 'node --version && git --version'
  else
    docker run --rm "$IMAGE" /bin/sh -lc 'node --version && git --version'
  fi
  end_step
else
  echo
  echo "Skipping container startup verification. Add --verify-container to test Docker run latency."
fi

echo
echo "Done. Re-run without --cold to measure warm-cache startup."
