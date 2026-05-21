#!/usr/bin/env sh
# Sova installer for Linux & macOS.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/sova-lang/sova/main/install.sh | sh
#   curl -fsSL https://raw.githubusercontent.com/sova-lang/sova/main/install.sh | SOVA_VERSION=v1.2.3 sh
#
# Re-running this installer upgrades an existing installation in-place.

set -eu

REPO="${SOVA_REPO:-sova-lang/sova}"
INSTALL_DIR="${SOVA_INSTALL_DIR:-$HOME/.sova}"
REQUESTED_VERSION="${SOVA_VERSION:-latest}"

log() { printf '\033[1;36m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m!!\033[0m %s\n' "$*" >&2; }
die() { printf '\033[1;31mxx\033[0m %s\n' "$*" >&2; exit 1; }

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "required command '$1' is not installed"
}

detect_os() {
  uname_s=$(uname -s)
  case "$uname_s" in
    Linux*)   OS_ALIAS=linux ;;
    Darwin*)  OS_ALIAS=osx ;;
    *)        die "unsupported OS: $uname_s (use install.ps1 on Windows)" ;;
  esac
}

detect_arch() {
  uname_m=$(uname -m)
  case "$uname_m" in
    x86_64|amd64)    ARCH_ALIAS=x64 ;;
    aarch64|arm64)   ARCH_ALIAS=arm64 ;;
    *)               die "unsupported architecture: $uname_m" ;;
  esac
}

resolve_version() {
  if [ "$REQUESTED_VERSION" != "latest" ]; then
    VERSION="$REQUESTED_VERSION"
    return
  fi
  log "resolving latest release from github.com/$REPO"
  api_url="https://api.github.com/repos/$REPO/releases/latest"
  if command -v curl >/dev/null 2>&1; then
    payload=$(curl -fsSL -H "Accept: application/vnd.github+json" "$api_url")
  elif command -v wget >/dev/null 2>&1; then
    payload=$(wget -qO- --header="Accept: application/vnd.github+json" "$api_url")
  else
    die "need curl or wget to query the GitHub API"
  fi
  VERSION=$(printf '%s' "$payload" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)
  [ -n "$VERSION" ] || die "could not parse latest release tag from GitHub response"
}

download() {
  url=$1
  out=$2
  if command -v curl >/dev/null 2>&1; then
    curl -fSL --progress-bar -o "$out" "$url"
  else
    wget -q --show-progress -O "$out" "$url"
  fi
}

install_archive() {
  asset="sova-$OS_ALIAS-$ARCH_ALIAS.tar.gz"
  url="https://github.com/$REPO/releases/download/$VERSION/$asset"
  log "downloading $asset ($VERSION)"

  tmp_dir=$(mktemp -d)
  trap 'rm -rf "$tmp_dir"' EXIT
  archive="$tmp_dir/$asset"
  download "$url" "$archive"

  log "extracting into $INSTALL_DIR"
  mkdir -p "$INSTALL_DIR"
  rm -rf "$INSTALL_DIR/std" "$INSTALL_DIR/sova"
  tar -xzf "$archive" -C "$INSTALL_DIR"
  chmod +x "$INSTALL_DIR/sova"
}

update_shell_path() {
  bin_dir="$INSTALL_DIR"
  needs_update=0
  case ":$PATH:" in
    *":$bin_dir:"*) ;;
    *) needs_update=1 ;;
  esac

  managed_line="export PATH=\"$bin_dir:\$PATH\" # added by sova installer"

  for rc in "$HOME/.bashrc" "$HOME/.zshrc" "$HOME/.profile"; do
    [ -f "$rc" ] || continue
    if grep -Fq "added by sova installer" "$rc"; then
      continue
    fi
    {
      printf '\n# Sova\n'
      printf '%s\n' "$managed_line"
    } >> "$rc"
    log "updated PATH in $rc"
  done

  fish_conf_dir="$HOME/.config/fish/conf.d"
  if [ -d "$HOME/.config/fish" ] || command -v fish >/dev/null 2>&1; then
    mkdir -p "$fish_conf_dir"
    fish_file="$fish_conf_dir/sova.fish"
    cat > "$fish_file" <<EOF
# added by sova installer
if not contains $bin_dir \$PATH
    set -gx PATH $bin_dir \$PATH
end
EOF
    log "updated PATH in $fish_file"
  fi

  if [ "$needs_update" -eq 1 ]; then
    warn "open a new shell (or run: export PATH=\"$bin_dir:\$PATH\") to use 'sova' in this session"
  fi
}

verify() {
  if [ -x "$INSTALL_DIR/sova" ]; then
    log "installed: $("$INSTALL_DIR/sova" version --short 2>/dev/null || echo "$VERSION")"
    log "location:  $INSTALL_DIR/sova"
  else
    die "post-install check failed: $INSTALL_DIR/sova is not executable"
  fi
}

main() {
  need_cmd tar
  detect_os
  detect_arch
  resolve_version
  log "target: $OS_ALIAS/$ARCH_ALIAS @ $VERSION"
  install_archive
  update_shell_path
  verify
  printf '\n\033[1;32mSova %s installed.\033[0m Run \033[1msova --help\033[0m to get started.\n' "$VERSION"
}

main "$@"
