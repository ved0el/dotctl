#!/bin/sh
# dotctl bootstrap installer — POSIX sh, no bashisms.
#
#   curl -fsSL https://raw.githubusercontent.com/ved0el/dotctl/main/install.sh | sh
#
# Honors: DOTCTL_REPO (default ~/.dotfiles), DOTCTL_VERSION (default latest),
#         DOTCTL_PROFILES (comma-separated, optional).
set -eu

# Single source of truth for the upstream repo. To fork/rename, override REPO_SLUG
# (owner/name) or set REPO_URL directly — nothing else below hardcodes it.
REPO_SLUG="${REPO_SLUG:-ved0el/dotctl}"
REPO_URL="${REPO_URL:-https://github.com/$REPO_SLUG}"
API_URL="https://api.github.com/repos/$REPO_SLUG"
DOTCTL_REPO="${DOTCTL_REPO:-$HOME/.dotfiles}"
DOTCTL_VERSION="${DOTCTL_VERSION:-latest}"
DOTCTL_PROFILES="${DOTCTL_PROFILES:-}"

os=""
arch=""
BIN=""

log() { printf '[dotctl] %s\n' "$1"; }
die() { printf '[dotctl] error: %s\n' "$1" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1; }

sha_check() { # reads "<hash>  <name>" lines on stdin, verifies against cwd files
	if need shasum; then
		shasum -a 256 -c -
	elif need sha256sum; then
		sha256sum -c -
	else
		die "no sha256 tool (shasum/sha256sum) available"
	fi
}

detect_platform() {
	case "$(uname -s)" in
		Darwin) os=darwin ;;
		Linux) os=linux ;;
		*) die "unsupported OS: $(uname -s)" ;;
	esac
	case "$(uname -m)" in
		arm64 | aarch64) arch=arm64 ;;
		x86_64 | amd64) arch=amd64 ;;
		*) die "unsupported architecture: $(uname -m)" ;;
	esac
}

ensure_prereqs() {
	if [ "$os" = darwin ] && ! need brew; then
		log "installing Homebrew"
		/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
	fi
	need git || die "git is required; install it and re-run"
}

clone_repo() {
	if [ -d "$DOTCTL_REPO/.git" ]; then
		log "updating $DOTCTL_REPO"
		git -C "$DOTCTL_REPO" pull --ff-only
	else
		log "cloning $REPO_URL into $DOTCTL_REPO"
		git clone "$REPO_URL" "$DOTCTL_REPO"
	fi
}

resolve_version() {
	[ "$DOTCTL_VERSION" = latest ] || return 0
	DOTCTL_VERSION=$(curl -fsSL "$API_URL/releases/latest" |
		sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n1)
	[ -n "$DOTCTL_VERSION" ] || DOTCTL_VERSION=latest
}

install_binary() {
	bindir="$DOTCTL_REPO/.bin"
	mkdir -p "$bindir"
	BIN="$bindir/dotctl"
	asset="dotctl_${os}_${arch}"
	base="$REPO_URL/releases/download/$DOTCTL_VERSION"
	tmp=$(mktemp -d)

	if [ "$DOTCTL_VERSION" != latest ] &&
		curl -fsSL "$base/$asset" -o "$tmp/dotctl" &&
		curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt"; then
		# Verify only the asset we downloaded (checksums.txt lists every binary).
		grep " ${asset}\$" "$tmp/checksums.txt" | sed "s/${asset}/dotctl/" >"$tmp/sum"
		( cd "$tmp" && sha_check <sum ) || die "checksum verification failed"
		chmod +x "$tmp/dotctl"
		mv "$tmp/dotctl" "$BIN"
		log "installed dotctl $DOTCTL_VERSION"
	else
		log "no release binary available; building from source"
		need go || die "no released binary and Go is not installed"
		( cd "$DOTCTL_REPO" && go build -o "$BIN" ./cmd/dotctl )
	fi
}

main() {
	detect_platform
	ensure_prereqs
	clone_repo
	resolve_version
	install_binary

	export DOTCTL_REPO
	if [ -n "$DOTCTL_PROFILES" ]; then
		"$BIN" init --profiles "$DOTCTL_PROFILES"
	else
		"$BIN" init
	fi
}

main
