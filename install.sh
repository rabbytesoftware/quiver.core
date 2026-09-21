#!/bin/sh
#
# Quiver installer for macOS and Linux.
#
#   curl -fsSL https://raw.githubusercontent.com/rabbytesoftware/quiver.core/develop/install.sh | bash
#
# Written in POSIX sh, not bash, on purpose. This has to install on any
# reasonable distro, and Alpine and most slim container images ship busybox ash
# with no bash at all -- a bash-only script would fail on exactly the machines a
# single-static-binary install is most attractive on. POSIX sh runs identically
# under bash, dash, ash and ksh, so both the documented `| bash` one-liner and a
# bare `| sh` work.
#
# There is deliberately no package manager anywhere in here. quiver.core builds
# with CGO_ENABLED=0 (see the Makefile's build target), so a release binary has
# zero runtime dependencies: downloading one file and putting it on PATH is the
# whole install. That is the only reason one script can cover every distro.
#
# Every step lives in a function and the last line calls main, so a truncated
# download -- the failure mode `curl | sh` is rightly criticised for -- defines
# some functions and then runs nothing.

set -eu

REPO="rabbytesoftware/quiver.core"
BIN_NAME="quiver"
API_BASE="https://api.github.com/repos/${REPO}/releases"

# Every URL taken out of the API response must start with this. The release body
# is free-form text that lands in the same JSON document being scanned, so
# pinning the prefix is what stops a crafted release note from pointing the
# download somewhere else.
DOWNLOAD_PREFIX="https://github.com/${REPO}/releases/download/"

# Set by resolve_* / require_* before use, so no function has to report an error
# from inside a command substitution (where exit would only end the subshell).
DL_TOOL=""
SHA_TOOL=""
OS=""
ARCH=""
ASSET=""
ASSET_URL=""
CHECKSUMS_URL=""
TAG=""
INSTALL_DIR=""
TMP_DIR=""

main() {
	# Before parse_args: err writes in colour, and parse_args can call it.
	setup_output
	parse_args "$@"
	require_tools
	detect_platform

	say "${C_BOLD}Installing quiver for ${OS}/${ARCH}${C_RESET}"

	TMP_DIR=$(make_temp_dir) ||
		err "cannot create a temporary directory under ${TMPDIR:-/tmp}"
	trap cleanup EXIT INT TERM HUP

	resolve_release
	download_release
	verify_checksum
	resolve_install_dir
	stop_running_daemon
	place_binary
	report_success
}

# ---------------------------------------------------------------------------
# Arguments
# ---------------------------------------------------------------------------

usage() {
	cat <<EOF
Install the quiver CLI and daemon.

Usage:
  install.sh [options]

Options:
  --install-dir <dir>  Where to put the quiver binary.
                       Default: \$HOME/.local/bin, or /usr/local/bin when that
                       is already on PATH and writable without sudo.
  --tag <tag>          Install a specific release (e.g. stable-26.5.1)
                       instead of the latest stable one.
  -h, --help           Show this help.

Environment:
  QUIVER_INSTALL_DIR   Same as --install-dir.
  QUIVER_TAG           Same as --tag.

This installer never uses sudo and never edits your shell startup files. If the
install directory is not on your PATH it prints the exact line to add and the
file to add it to.
EOF
}

parse_args() {
	INSTALL_DIR=${QUIVER_INSTALL_DIR:-}
	TAG=${QUIVER_TAG:-}

	while [ $# -gt 0 ]; do
		case "$1" in
		--install-dir)
			[ $# -ge 2 ] || err "--install-dir needs a directory"
			INSTALL_DIR=$2
			shift 2
			;;
		--install-dir=*)
			INSTALL_DIR=${1#--install-dir=}
			shift
			;;
		--tag)
			[ $# -ge 2 ] || err "--tag needs a release tag"
			TAG=$2
			shift 2
			;;
		--tag=*)
			TAG=${1#--tag=}
			shift
			;;
		-h | --help)
			usage
			exit 0
			;;
		*)
			usage >&2
			err "unknown option: $1"
			;;
		esac
	done
}

# ---------------------------------------------------------------------------
# Output
# ---------------------------------------------------------------------------

# Colour only when stdout is a terminal. Under `curl | sh` only stdin is a pipe,
# so this is still true for the documented one-liner, but it keeps output clean
# when someone redirects the install log to a file.
setup_output() {
	if [ -t 1 ]; then
		C_RESET=$(printf '\033[0m')
		C_BOLD=$(printf '\033[1m')
		C_DIM=$(printf '\033[2m')
		C_GREEN=$(printf '\033[0;32m')
		C_YELLOW=$(printf '\033[0;33m')
		C_RED=$(printf '\033[0;31m')
	else
		C_RESET=""
		C_BOLD=""
		C_DIM=""
		C_GREEN=""
		C_YELLOW=""
		C_RED=""
	fi
}

say() { printf '%s\n' "$*"; }
step() { printf '%s\n' "${C_DIM}$*${C_RESET}"; }
warn() { printf '%s\n' "${C_YELLOW}warning:${C_RESET} $*" >&2; }

err() {
	printf '%s\n' "${C_RED}error:${C_RESET} $*" >&2
	exit 1
}

cleanup() {
	[ -n "$TMP_DIR" ] && [ -d "$TMP_DIR" ] && rm -rf "$TMP_DIR"
	return 0
}

make_temp_dir() {
	if command -v mktemp >/dev/null 2>&1; then
		mktemp -d 2>/dev/null && return 0
	fi
	# No mktemp (very unusual, but it is not in POSIX). $$ plus a private mode
	# is enough here: mkdir fails outright if the name is already taken.
	dir="${TMPDIR:-/tmp}/quiver-install.$$"
	(umask 077 && mkdir "$dir") || return 1
	printf '%s\n' "$dir"
}

# ---------------------------------------------------------------------------
# Environment probing
# ---------------------------------------------------------------------------

require_tools() {
	if command -v curl >/dev/null 2>&1; then
		DL_TOOL=curl
	elif command -v wget >/dev/null 2>&1; then
		DL_TOOL=wget
	else
		err "need curl or wget to download quiver, found neither"
	fi

	# macOS has no sha256sum, most Linux userlands have no shasum, busybox has
	# sha256sum. openssl is the last resort. The install refuses to proceed
	# without one of them rather than skipping verification.
	if command -v sha256sum >/dev/null 2>&1; then
		SHA_TOOL=sha256sum
	elif command -v shasum >/dev/null 2>&1; then
		SHA_TOOL=shasum
	elif command -v openssl >/dev/null 2>&1; then
		SHA_TOOL=openssl
	else
		err "need sha256sum, shasum or openssl to verify the download, found none"
	fi
}

detect_platform() {
	uname_s=$(uname -s 2>/dev/null || echo unknown)
	uname_m=$(uname -m 2>/dev/null || echo unknown)

	case "$uname_s" in
	Darwin) OS=darwin ;;
	Linux) OS=linux ;;
	MINGW* | MSYS* | CYGWIN* | Windows_NT)
		err "this is the macOS and Linux installer. On Windows run:
    irm https://raw.githubusercontent.com/${REPO}/develop/install.ps1 | iex"
		;;
	*)
		err "unsupported operating system: ${uname_s}. quiver.core publishes builds for macOS, Linux and Windows only."
		;;
	esac

	case "$uname_m" in
	x86_64 | amd64) ARCH=amd64 ;;
	aarch64 | arm64) ARCH=arm64 ;;
	*)
		err "unsupported architecture: ${uname_m}. quiver.core publishes amd64 and arm64 builds only; there is no 32-bit release."
		;;
	esac

	# An Apple Silicon Mac running this script through Rosetta reports x86_64
	# from uname -m, which would install the Intel build on an arm64 machine and
	# leave it translated forever. sysctl.proc_translated is the documented way
	# to tell; it is absent on a native process and on Intel hardware.
	if [ "$OS" = darwin ] && [ "$ARCH" = amd64 ]; then
		if [ "$(sysctl -n sysctl.proc_translated 2>/dev/null || echo 0)" = "1" ]; then
			ARCH=arm64
			step "  Rosetta detected, installing the native arm64 build."
		fi
	fi

	ASSET="${BIN_NAME}-${OS}-${ARCH}"
}

# ---------------------------------------------------------------------------
# Release resolution
# ---------------------------------------------------------------------------

download() {
	# $1 url, $2 destination path
	case "$DL_TOOL" in
	curl)
		curl -fsSL --proto '=https' --proto-redir '=https' --tlsv1.2 \
			--retry 3 -o "$2" "$1"
		;;
	wget)
		wget -q -O "$2" "$1"
		;;
	*)
		err "internal: no download tool resolved"
		;;
	esac
}

# resolve_release reads the real release metadata and pulls the asset URLs out
# of it. The URLs are never templated from a version string: a release that
# renamed or dropped an asset has to fail here with a readable message instead
# of 404ing halfway through a download.
resolve_release() {
	if [ -n "$TAG" ]; then
		api_url="${API_BASE}/tags/${TAG}"
		step "  Resolving release ${TAG}..."
	else
		# /releases/latest is the current stable release by definition: betas,
		# hotfixes and nightlies are all published as prereleases, which GitHub
		# excludes from this endpoint.
		api_url="${API_BASE}/latest"
		step "  Resolving the latest release..."
	fi

	meta="${TMP_DIR}/release.json"
	download "$api_url" "$meta" ||
		err "could not reach the GitHub release API at ${api_url}"

	if grep -q 'API rate limit exceeded' "$meta" 2>/dev/null; then
		err "GitHub's unauthenticated API rate limit is exhausted for this IP address.
       Wait for it to reset, or download a binary by hand from
       https://github.com/${REPO}/releases/latest"
	fi

	TAG=$(json_string "$meta" tag_name)
	[ -n "$TAG" ] || err "the release API returned no tag_name; ${api_url} may not name a real release"

	ASSET_URL=$(asset_url "$meta" "$ASSET")
	[ -n "$ASSET_URL" ] || err "release ${TAG} publishes no ${ASSET} asset.
       Assets it does publish:
$(asset_names "$meta" | sed 's/^/         /')"

	CHECKSUMS_URL=$(asset_url "$meta" "checksums.txt")
	[ -n "$CHECKSUMS_URL" ] || err "release ${TAG} publishes no checksums.txt, so the download cannot be verified"
}

# json_string pulls a top-level string field out of the release JSON without a
# JSON parser, since jq is not installed on a stock anything.
json_string() {
	tr -d '\r' <"$1" |
		grep -o "\"$2\"[[:space:]]*:[[:space:]]*\"[^\"]*\"" |
		head -n 1 |
		sed "s/^\"$2\"[[:space:]]*:[[:space:]]*\"//; s/\"$//"
}

# asset_urls lists every browser_download_url that actually belongs to this
# repository's release downloads. The prefix check is the security boundary:
# the release body is arbitrary text inside the same document.
asset_urls() {
	tr -d '\r' <"$1" |
		grep -o '"browser_download_url"[[:space:]]*:[[:space:]]*"[^"]*"' |
		sed 's/.*:[[:space:]]*"//; s/"$//' |
		grep "^${DOWNLOAD_PREFIX}"
}

asset_names() {
	asset_urls "$1" | sed 's|.*/||'
}

# asset_url matches on the asset's own file name rather than building a URL out
# of the tag, so a renamed asset is a clear failure and not a silent 404.
asset_url() {
	asset_urls "$1" | while IFS= read -r url; do
		case "${url##*/}" in
		"$2")
			printf '%s\n' "$url"
			break
			;;
		esac
	done
}

# ---------------------------------------------------------------------------
# Download and verification
# ---------------------------------------------------------------------------

download_release() {
	step "  Downloading ${ASSET} from ${TAG}..."
	download "$ASSET_URL" "${TMP_DIR}/${ASSET}" ||
		err "download failed: ${ASSET_URL}"
	download "$CHECKSUMS_URL" "${TMP_DIR}/checksums.txt" ||
		err "could not download checksums.txt from ${TAG}"
	[ -s "${TMP_DIR}/${ASSET}" ] || err "downloaded ${ASSET} is empty"
}

sha256_of() {
	case "$SHA_TOOL" in
	sha256sum) sha256sum "$1" | cut -d' ' -f1 ;;
	shasum) shasum -a 256 "$1" | cut -d' ' -f1 ;;
	# openssl 1.x prints "SHA256(path)= hash", 3.x prints "SHA2-256(path)= hash".
	openssl) openssl dgst -sha256 "$1" | tr -d '\r' | sed 's/.*[ =]//' ;;
	*) return 1 ;;
	esac
}

# expected_sum reads the hash for one asset out of checksums.txt.
#
# Deliberately no awk: openSUSE's minimal image (and other trimmed base images)
# ship coreutils but no awk at all, and the whole point of this script is not
# assuming what a distro installed. A read loop needs only the shell itself.
#
# checksums.txt is produced by `cd bin && sha256sum ./*` in build-assets.yml, so
# every path in it carries a ./ prefix. Matching on the basename means a change
# to how that workflow invokes sha256sum cannot quietly skip verification.
expected_sum() {
	tr -d '\r' <"$1" | while read -r sum name; do
		[ -n "$name" ] || continue
		if [ "${name##*/}" = "$2" ]; then
			printf '%s\n' "$sum"
			break
		fi
	done
}

verify_checksum() {
	step "  Verifying sha256..."

	expected=$(expected_sum "${TMP_DIR}/checksums.txt" "$ASSET")

	[ -n "$expected" ] ||
		err "checksums.txt in ${TAG} has no entry for ${ASSET}; refusing to install an unverified binary"

	actual=$(sha256_of "${TMP_DIR}/${ASSET}")
	[ -n "$actual" ] || err "could not compute the sha256 of the download using ${SHA_TOOL}"

	if [ "$actual" != "$expected" ]; then
		err "checksum mismatch for ${ASSET}, refusing to install.
       expected ${expected}
       actual   ${actual}
       The download was corrupted or tampered with. Nothing has been installed."
	fi
}

# ---------------------------------------------------------------------------
# Installation
# ---------------------------------------------------------------------------

on_path() {
	case ":${PATH-}:" in
	*":$1:"*) return 0 ;;
	*) return 1 ;;
	esac
}

# resolve_install_dir prefers a directory the user's shell already searches, so
# that in the common case `quiver` works the moment the script finishes. sudo is
# never used: a per-user install needs no privileges, and prompting for a
# password from a script the user just piped into a shell is not a trade worth
# making.
resolve_install_dir() {
	if [ -n "$INSTALL_DIR" ]; then
		return
	fi

	# $HOME/.local/bin first, and only for being on PATH rather than for
	# existing: Debian, Ubuntu and Fedora all add it from ~/.profile whether or
	# not it is there yet. It is also what `make install` uses.
	if [ -n "${HOME:-}" ] && on_path "${HOME}/.local/bin"; then
		INSTALL_DIR="${HOME}/.local/bin"
		return
	fi

	# Writable without sudo means a Homebrew-style macOS box or a root shell in
	# a container. Both want the binary here, and both already have it on PATH.
	if on_path /usr/local/bin && [ -w /usr/local/bin ]; then
		INSTALL_DIR=/usr/local/bin
		return
	fi

	if [ -n "${HOME:-}" ]; then
		INSTALL_DIR="${HOME}/.local/bin"
		return
	fi

	err "cannot pick an install directory: \$HOME is not set.
       Re-run with --install-dir <dir>."
}

# stop_running_daemon mirrors what the Makefile's install target does, for the
# same reason its comment gives: the daemon holds an open handle to its own
# binary, and replacing the binary underneath a daemon that keeps serving leaves
# an old daemon talking to a new CLI. That version skew surfaces as decode
# panics, not as a clean error. Every step is best effort; the install proceeds
# either way.
stop_running_daemon() {
	[ -n "${HOME:-}" ] || return 0
	pid_file="${HOME}/.quiver/quiver.pid"
	[ -f "$pid_file" ] || return 0

	pid=$(cat "$pid_file" 2>/dev/null) || return 0
	case "$pid" in
	'' | *[!0-9]*) return 0 ;;
	esac
	kill -0 "$pid" 2>/dev/null || return 0

	step "  Stopping the running daemon (pid ${pid})..."
	kill -TERM "$pid" 2>/dev/null || return 0

	i=0
	while [ "$i" -lt 10 ]; do
		kill -0 "$pid" 2>/dev/null || return 0
		sleep 1
		i=$((i + 1))
	done
	warn "the daemon at pid ${pid} did not exit; it will keep serving the old version until it is stopped"
}

# place_binary stages inside the install directory and renames into place. The
# rename is atomic and, unlike a copy, cannot fail with ETXTBSY against a binary
# that is still executing -- a copy from a temp directory on another filesystem
# could, since that is a copy-then-unlink underneath.
place_binary() {
	mkdir -p "$INSTALL_DIR" ||
		err "cannot create ${INSTALL_DIR}. Re-run with --install-dir <dir> pointing somewhere writable."

	target="${INSTALL_DIR}/${BIN_NAME}"
	staged="${INSTALL_DIR}/.${BIN_NAME}.install.$$"

	cp "${TMP_DIR}/${ASSET}" "$staged" 2>/dev/null || {
		rm -f "$staged" 2>/dev/null || true
		err "cannot write to ${INSTALL_DIR}.
       Re-run with --install-dir <dir> pointing somewhere you can write."
	}
	chmod 755 "$staged" || {
		rm -f "$staged" 2>/dev/null || true
		err "cannot make ${staged} executable"
	}
	mv -f "$staged" "$target" || {
		rm -f "$staged" 2>/dev/null || true
		err "cannot replace ${target}"
	}
}

report_success() {
	target="${INSTALL_DIR}/${BIN_NAME}"

	installed=""
	if installed=$("$target" --version 2>/dev/null); then
		:
	else
		installed=""
	fi

	say ""
	if [ -n "$installed" ]; then
		say "${C_GREEN}${C_BOLD}Installed ${installed}${C_RESET}"
	else
		# Worth saying out loud: the binary is verified and in place, but it did
		# not run here, which on Linux almost always means a musl/glibc-free
		# sandbox restriction or a wrong-arch kernel.
		say "${C_GREEN}${C_BOLD}Installed quiver ${TAG}${C_RESET}"
		warn "${target} did not respond to --version on this machine"
	fi
	say "  ${C_DIM}${target}${C_RESET}"
	say ""

	if on_path "$INSTALL_DIR"; then
		# Only commands the current stable release actually has. The wider
		# command tree landed after stable-26.5.1, so naming one of those here
		# would print advice that fails on the version just installed.
		say "Next steps:"
		say "  ${C_BOLD}quiver --help${C_RESET}    everything the CLI can do"
		say "  ${C_BOLD}quiver daemon${C_RESET}    run the local daemon in the foreground"
	else
		print_path_instructions
	fi
}

# print_path_instructions tells the user exactly which line to add and where.
# Nothing here edits a startup file: an installer that appends to a shell rc
# stops being idempotent the second time it runs, and it is not this script's
# place to modify a file the user owns.
print_path_instructions() {
	warn "${INSTALL_DIR} is not on your PATH, so \`quiver\` will not be found yet."

	shell_path=${SHELL:-}
	shell_name=${shell_path##*/}
	case "$shell_name" in
	fish)
		rc="${HOME:-~}/.config/fish/config.fish"
		line="fish_add_path ${INSTALL_DIR}"
		;;
	zsh)
		rc="${ZDOTDIR:-${HOME:-~}}/.zshrc"
		line="export PATH=\"${INSTALL_DIR}:\$PATH\""
		;;
	bash)
		# A macOS Terminal tab is a login shell, which reads .bash_profile and
		# never .bashrc; on Linux an interactive terminal reads .bashrc.
		if [ "$OS" = darwin ]; then
			rc="${HOME:-~}/.bash_profile"
		else
			rc="${HOME:-~}/.bashrc"
		fi
		line="export PATH=\"${INSTALL_DIR}:\$PATH\""
		;;
	ksh | ksh93 | mksh)
		rc="${HOME:-~}/.kshrc"
		line="export PATH=\"${INSTALL_DIR}:\$PATH\""
		;;
	*)
		rc="${HOME:-~}/.profile"
		line="export PATH=\"${INSTALL_DIR}:\$PATH\""
		;;
	esac

	say ""
	say "Add this line to ${C_BOLD}${rc}${C_RESET}:"
	say ""
	say "  ${C_BOLD}${line}${C_RESET}"
	say ""
	say "Then open a new terminal, or run it once in this one, and try:"
	say "  ${C_BOLD}quiver --help${C_RESET}"
}

main "$@"
