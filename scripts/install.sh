#!/usr/bin/env bash
# Multigent installer.
#
# Recommended:
#   curl -fsSL https://raw.githubusercontent.com/multigent/multigent/main/scripts/install.sh | bash
#
# Options:
#   MULTIGENT_BIN_DIR=$HOME/.local/bin bash scripts/install.sh
#   MULTIGENT_VERSION=v0.1.0 bash scripts/install.sh
#
set -euo pipefail

REPO_WEB_URL="${MULTIGENT_REPO_WEB_URL:-https://github.com/multigent/multigent}"
BREW_PACKAGE="${MULTIGENT_BREW_PACKAGE:-multigent/tap/multigent}"
BIN_DIR="${MULTIGENT_BIN_DIR:-/usr/local/bin}"
VERSION="${MULTIGENT_VERSION:-}"

if [ -t 1 ] || [ -t 2 ]; then
  BOLD='\033[1m'; GREEN='\033[0;32m'; YELLOW='\033[0;33m'; RED='\033[0;31m'; CYAN='\033[0;36m'; RESET='\033[0m'
else
  BOLD=''; GREEN=''; YELLOW=''; RED=''; CYAN=''; RESET=''
fi

info() { printf "${BOLD}${CYAN}==> %s${RESET}\n" "$*"; }
ok() { printf "${BOLD}${GREEN}✓ %s${RESET}\n" "$*"; }
warn() { printf "${BOLD}${YELLOW}⚠ %s${RESET}\n" "$*" >&2; }
fail() { printf "${BOLD}${RED}✗ %s${RESET}\n" "$*" >&2; exit 1; }

command_exists() { command -v "$1" >/dev/null 2>&1; }

detect_platform() {
  case "$(uname -s)" in
    Darwin) OS="darwin" ;;
    Linux) OS="linux" ;;
    MINGW*|MSYS*|CYGWIN*)
      fail "Use the PowerShell installer on Windows:
  irm https://raw.githubusercontent.com/multigent/multigent/main/scripts/install.ps1 | iex" ;;
    *) fail "Unsupported operating system: $(uname -s)" ;;
  esac

  case "$(uname -m)" in
    x86_64|amd64) ARCH="amd64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    *) fail "Unsupported architecture: $(uname -m)" ;;
  esac
}

latest_version() {
  if [ -n "$VERSION" ]; then
    printf '%s' "$VERSION"
    return
  fi
  local latest
  latest="$(curl -fsSLI "$REPO_WEB_URL/releases/latest" 2>/dev/null | grep -i '^location:' | sed 's/.*tag\///' | tr -d '\r\n' || true)"
  if [ -z "$latest" ]; then
    latest="$(curl -fsSL "https://api.github.com/repos/multigent/multigent/releases/latest" 2>/dev/null | sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1 || true)"
  fi
  [ -n "$latest" ] || fail "Could not determine latest release. Check your network connection."
  printf '%s' "$latest"
}

add_to_path() {
  local dir="$1"
  local line="export PATH=\"$dir:\$PATH\""
  for rc in "$HOME/.bashrc" "$HOME/.zshrc"; do
    if [ -f "$rc" ] && ! grep -qF "$dir" "$rc"; then
      printf '\n# Added by Multigent installer\n%s\n' "$line" >> "$rc"
    fi
  done
}

install_brew() {
  if ! command_exists brew; then
    return 1
  fi
  info "Installing Multigent via Homebrew..."
  if brew install "$BREW_PACKAGE"; then
    ok "Multigent installed via Homebrew"
    return 0
  fi
  if brew list "$BREW_PACKAGE" >/dev/null 2>&1; then
    ok "Multigent is already installed via Homebrew"
    return 0
  fi
  warn "Homebrew install failed. Falling back to GitHub Releases binary install."
  return 1
}

install_binary() {
  local tag="$1"
  local archive="multigent-${tag}-${OS}-${ARCH}.tar.gz"
  local url="$REPO_WEB_URL/releases/download/${tag}/${archive}"
  local tmp_dir
  tmp_dir="$(mktemp -d)"

  info "Downloading $url ..."
  curl -fsSL "$url" -o "$tmp_dir/$archive" || {
    rm -rf "$tmp_dir"
    fail "Failed to download $archive"
  }
  tar -xzf "$tmp_dir/$archive" -C "$tmp_dir"

  local target="$BIN_DIR"
  if [ ! -d "$target" ]; then
    if mkdir -p "$target" 2>/dev/null; then
      :
    elif command_exists sudo; then
      sudo mkdir -p "$target"
    else
      target="$HOME/.local/bin"
      mkdir -p "$target"
    fi
  fi
  if [ ! -w "$target" ]; then
    if command_exists sudo; then
      sudo install -m 0755 "$tmp_dir/multigent" "$target/multigent"
      sudo install -m 0755 "$tmp_dir/mga" "$target/mga"
    else
      target="$HOME/.local/bin"
      mkdir -p "$target"
      install -m 0755 "$tmp_dir/multigent" "$target/multigent"
      install -m 0755 "$tmp_dir/mga" "$target/mga"
    fi
  else
    install -m 0755 "$tmp_dir/multigent" "$target/multigent"
    install -m 0755 "$tmp_dir/mga" "$target/mga"
  fi

  rm -rf "$tmp_dir"
  if ! echo "$PATH" | tr ':' '\n' | grep -q "^$target$"; then
    add_to_path "$target"
    warn "$target was added to your shell profile. Restart your shell if 'multigent' is not found."
  fi
  ok "Installed multigent and mga to $target"
}

detect_platform
tag="$(latest_version)"
info "Target release: $tag ($OS/$ARCH)"

if [ -z "${MULTIGENT_FORCE_BINARY:-}" ]; then
  install_brew && exit 0
fi
install_binary "$tag"

printf "\n"
ok "Done. Run: multigent start --dir ./multigent-data --open"
