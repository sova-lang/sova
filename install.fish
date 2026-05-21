#!/usr/bin/env fish
# Sova installer for Linux & macOS (fish shell edition).
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/sova-lang/sova/main/install.fish | fish
#   curl -fsSL https://raw.githubusercontent.com/sova-lang/sova/main/install.fish | SOVA_VERSION=v1.2.3 fish
#
# Re-running this installer upgrades an existing installation in-place.

set -l repo (test -n "$SOVA_REPO"; and echo $SOVA_REPO; or echo "sova-lang/sova")
set -l install_dir (test -n "$SOVA_INSTALL_DIR"; and echo $SOVA_INSTALL_DIR; or echo "$HOME/.sova")
set -l requested_version (test -n "$SOVA_VERSION"; and echo $SOVA_VERSION; or echo "latest")

function _log
    set_color cyan
    printf '==> '
    set_color normal
    echo $argv
end

function _warn
    set_color yellow
    printf '!! '
    set_color normal
    echo $argv >&2
end

function _die
    set_color red
    printf 'xx '
    set_color normal
    echo $argv >&2
    exit 1
end

function _need_cmd
    if not type -q $argv[1]
        _die "required command '$argv[1]' is not installed"
    end
end

_need_cmd tar

set -l uname_s (uname -s)
switch $uname_s
    case Linux
        set -g os_alias linux
    case Darwin
        set -g os_alias osx
    case '*'
        _die "unsupported OS: $uname_s (use install.ps1 on Windows)"
end

set -l uname_m (uname -m)
switch $uname_m
    case x86_64 amd64
        set -g arch_alias x64
    case aarch64 arm64
        set -g arch_alias arm64
    case '*'
        _die "unsupported architecture: $uname_m"
end

if test "$requested_version" = "latest"
    _log "resolving latest release from github.com/$repo"
    set -l api_url "https://api.github.com/repos/$repo/releases/latest"
    if type -q curl
        set -l payload (curl -fsSL -H "Accept: application/vnd.github+json" $api_url | string collect)
        set -g sova_version (printf '%s' $payload | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)
    else if type -q wget
        set -l payload (wget -qO- --header="Accept: application/vnd.github+json" $api_url | string collect)
        set -g sova_version (printf '%s' $payload | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)
    else
        _die "need curl or wget to query the GitHub API"
    end
    if test -z "$sova_version"
        _die "could not parse latest release tag from GitHub response"
    end
else
    set -g sova_version $requested_version
end

_log "target: $os_alias/$arch_alias @ $sova_version"

set -l asset "sova-$os_alias-$arch_alias.tar.gz"
set -l url "https://github.com/$repo/releases/download/$sova_version/$asset"
set -l tmp_dir (mktemp -d)
set -l archive "$tmp_dir/$asset"

function _cleanup --on-event fish_exit --inherit-variable tmp_dir
    rm -rf $tmp_dir 2>/dev/null
end

_log "downloading $asset ($sova_version)"
if type -q curl
    curl -fSL --progress-bar -o $archive $url; or _die "download failed"
else
    wget -q --show-progress -O $archive $url; or _die "download failed"
end

_log "extracting into $install_dir"
mkdir -p $install_dir
rm -rf "$install_dir/std" "$install_dir/sova"
tar -xzf $archive -C $install_dir; or _die "extraction failed"
chmod +x "$install_dir/sova"

set -l conf_dir "$HOME/.config/fish/conf.d"
mkdir -p $conf_dir
set -l conf_file "$conf_dir/sova.fish"
echo "# added by sova installer" > $conf_file
echo "if not contains $install_dir \$PATH" >> $conf_file
echo "    set -gx PATH $install_dir \$PATH" >> $conf_file
echo "end" >> $conf_file
_log "updated PATH in $conf_file"

if not contains $install_dir $PATH
    set -gx PATH $install_dir $PATH
    _warn "PATH updated for this session and persisted in $conf_file"
end

for rc in "$HOME/.bashrc" "$HOME/.zshrc" "$HOME/.profile"
    if test -f $rc
        if grep -Fq "added by sova installer" $rc
            continue
        end
        echo "" >> $rc
        echo "# Sova" >> $rc
        echo "export PATH=\"$install_dir:\$PATH\" # added by sova installer" >> $rc
        _log "updated PATH in $rc"
    end
end

if test -x "$install_dir/sova"
    set -l installed_version ("$install_dir/sova" version --short 2>/dev/null; or echo $sova_version)
    _log "installed: $installed_version"
    _log "location:  $install_dir/sova"
else
    _die "post-install check failed: $install_dir/sova is not executable"
end

set_color green
echo ""
echo "Sova $sova_version installed. Run 'sova --help' to get started."
set_color normal
