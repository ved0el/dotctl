#!/bin/sh
# dotctl bootstrap installer — POSIX sh, no bashisms.
#
#   curl -fsSL https://tinyurl.com/get-dotctl | sh
#
# Honors: DOTCTL_REPO (default ~/.dotfiles), DOTCTL_VERSION (default latest),
#         DOTCTL_PROFILES (comma-separated, optional).
set -eu

# Single source of truth for the upstream repo. To fork/rename, override REPO_SLUG
# (owner/name) or set REPO_URL directly — nothing else below hardcodes it.
REPO_SLUG="${REPO_SLUG:-ved0el/dotctl}"
REPO_URL="${REPO_URL:-https://github.com/$REPO_SLUG}"
DOTCTL_REPO="${DOTCTL_REPO:-$HOME/.dotfiles}"
DOTCTL_VERSION="${DOTCTL_VERSION:-latest}"
DOTCTL_PROFILES="${DOTCTL_PROFILES:-}"

os=""
arch=""
BIN=""
TMP=""

log() { printf '[dotctl] %s\n' "$1"; }
die() { printf '[dotctl] error: %s\n' "$1" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1; }

# fetch enforces HTTPS + TLS 1.2 and follows redirects.
fetch() { curl --proto '=https' --tlsv1.2 -fsSL "$@"; }

cleanup() { [ -n "$TMP" ] && rm -rf "$TMP"; }
trap cleanup EXIT INT TERM

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
		/bin/bash -c "$(fetch https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
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

# resolve_version turns "latest" into a concrete tag via the releases/latest
# redirect (no JSON scraping). Hard-fails rather than silently proceeding
# unverified — a known version is what makes checksum verification mandatory.
resolve_version() {
	[ "$DOTCTL_VERSION" = latest ] || return 0
	url=$(fetch -o /dev/null -w '%{url_effective}' "$REPO_URL/releases/latest") || true
	DOTCTL_VERSION="${url##*/}"
	case "$DOTCTL_VERSION" in
		v[0-9]*) ;;
		*) die "could not resolve the latest version (set DOTCTL_VERSION=vX.Y.Z)" ;;
	esac
}

# cosign_verify verifies the checksums signature when cosign is present. Most
# bare machines won't have it — the sha256 check below is the guaranteed gate;
# this is a stronger check for those who can run it. When cosign IS present we
# fail closed: a missing signature artifact is treated as tampering, not a reason
# to silently downgrade to checksum-only. The identity is pinned to the exact
# release workflow + tag (not a loose regexp), so only that workflow can sign.
cosign_verify() {
	need cosign || { log "cosign not installed — skipping signature check (checksums still verified)"; return 0; }
	fetch "$1/checksums.txt.pem" -o "$TMP/checksums.txt.pem" \
		|| die "cosign present but checksums.txt.pem could not be fetched — refusing to downgrade to checksum-only"
	fetch "$1/checksums.txt.sig" -o "$TMP/checksums.txt.sig" \
		|| die "cosign present but checksums.txt.sig could not be fetched — refusing to downgrade to checksum-only"
	( cd "$TMP" && cosign verify-blob \
		--certificate checksums.txt.pem --signature checksums.txt.sig \
		--certificate-identity "https://github.com/$REPO_SLUG/.github/workflows/release.yml@refs/tags/$DOTCTL_VERSION" \
		--certificate-oidc-issuer https://token.actions.githubusercontent.com \
		checksums.txt ) || die "cosign signature verification failed"
	log "cosign signature verified"
}

install_binary() {
	# Install to ~/.local/bin so `dotctl` is on PATH (the base profile's zshrc
	# adds it). Anything else leaves `dotctl: command not found` after install.
	bindir="$HOME/.local/bin"
	mkdir -p "$bindir"
	BIN="$bindir/dotctl"
	asset="dotctl_${os}_${arch}"
	base="$REPO_URL/releases/download/$DOTCTL_VERSION"
	TMP=$(mktemp -d)

	if fetch "$base/$asset" -o "$TMP/dotctl" && fetch "$base/checksums.txt" -o "$TMP/checksums.txt"; then
		cosign_verify "$base"
		# Verify only the asset we downloaded (checksums.txt lists every binary).
		# Assert the entry exists so an empty list can't fail open through a
		# sha tool that accepts empty input (POSIX sh has no pipefail here).
		grep " ${asset}\$" "$TMP/checksums.txt" | sed "s/${asset}/dotctl/" >"$TMP/sum"
		[ -s "$TMP/sum" ] || die "checksums.txt has no entry for $asset"
		( cd "$TMP" && sha_check <sum ) || die "checksum verification failed"
		chmod +x "$TMP/dotctl"
		mv "$TMP/dotctl" "$BIN"
		log "installed dotctl $DOTCTL_VERSION"
	else
		# No release binary for this version — build from the cloned repo (trusted
		# source), never an unverified download.
		log "no release binary for $DOTCTL_VERSION; building from source"
		need go || die "no release binary and Go is not installed"
		( cd "$DOTCTL_REPO" && go build -ldflags "-X main.version=$DOTCTL_VERSION" -o "$BIN" ./cmd/dotctl )
	fi
}

# reload_shell starts a fresh login shell so the just-linked config takes effect
# (and ~/.local/bin — where dotctl now lives — lands on PATH). Falls back to a
# hint when there's no terminal to attach to (piped/CI runs).
reload_shell() {
	shell="${SHELL:-/bin/sh}"
	log "done. reloading your shell to apply the new config..."
	if [ -t 1 ] && [ -r /dev/tty ]; then
		exec "$shell" -l </dev/tty
	fi
	log "no interactive terminal — open a new shell (or run: exec \"$shell\" -l) to finish."
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

	reload_shell
}

main
