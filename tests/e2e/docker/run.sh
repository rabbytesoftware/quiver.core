#!/usr/bin/env bash
# End-to-end suite for a real quiver binary on Linux, run inside the image
# tests/e2e/docker/Dockerfile builds.
#
#   Phase A  the real github.com/char2cs/crowbar@nightly: install, inspect,
#            no-op update, uninstall, remove. Needs network access.
#   Phase B  a rolling tag served from a container-local HTTPS git host
#            (githost + git-http-backend on https://localhost), force-moved
#            under an installed row: drift -> available -> update -> advanced,
#            on one identity that never stops answering 200.
#   Phase C  the CLI-booted daemon: an update's commit survives the CLI's
#            idle-stop, and a second update of a row whose last run was an
#            update waits for its own steps.
#   Phase D  a stable channel on the same clone-only host, which serves no
#            commit SHA and no ref named "stable": a refless install follows
#            stable to its newest stable-* tag, an update across a new tag
#            keeps the identity, and adopting an older member offers the
#            newest.
#   Phase E  quiver.core's own nightly-latest row, served by the git host
#            standing in for github.com: a build stamped exactly as the
#            release workflows stamp one registers itself, notices the rolling
#            tag moved, downloads the next build through its own ARROW.md and
#            execs it; the successor adopts the new commit on the same identity.
#   Phase F  the same for the stable channel, across a new stable-* tag.
#
# Environment:
#   E2E_PHASES         phases to run, default "A B C D E F"
#   E2E_WAIT_SECONDS   deadline for any single wait, default 1500
#   E2E_HOST_ARCH      the Docker host's architecture (amd64|arm64), set by
#                      `make test-e2e-docker`; evidence that the container is
#                      emulated when it differs from the container's own
set -Eeuo pipefail

readonly CROWBAR_REPO=https://github.com/char2cs/crowbar
readonly CROWBAR=github.com/char2cs/crowbar@nightly
readonly FIXTURE_SRC=/opt/e2e/fixture
readonly MULTI_FIXTURE=$FIXTURE_SRC/multi-tool/ARROW.md
readonly GIT_ROOT=/srv/git
readonly SOCK="$HOME/.quiver/quiver.sock"
readonly PID_FILE="$HOME/.quiver/quiver.pid"
WAIT_SECONDS="${E2E_WAIT_SECONDS:-1500}"
PHASES="${E2E_PHASES:-A B C D E F}"

SUITE_START=$SECONDS
PHASE_START=$SECONDS
CHECKS=0
SUMMARY=()
DAEMON_PID=""
GITHOST_PID=""
POLLER_PID=""
EMULATED=0
HOSTS_REDIRECTED=0

# ─── reporting ────────────────────────────────────────────────────────────────

banner() {
	printf '\n\033[1m══════ %s ══════\033[0m\n' "$*"
}

step() {
	printf '\n\033[1;34m▶ %s\033[0m  (t+%ss)\n' "$*" "$((SECONDS - SUITE_START))"
}

ok() {
	CHECKS=$((CHECKS + 1))
	printf '  \033[32m✓\033[0m %s\n' "$*"
}

# A failure inside a command substitution exits only that subshell, and the
# caller's ERR trap then fires again: the marker keeps the report to one.
readonly FAILED_MARKER="$HOME/.e2e-failed"
rm -f "$FAILED_MARKER"

report_failure() {
	if [[ -e "$FAILED_MARKER" ]]; then
		return 0
	fi
	: >"$FAILED_MARKER"
	printf '\n\033[1;31m✗ FAIL: %s\033[0m\n' "$*" >&2
	diagnostics >&2
}

fail() {
	report_failure "$*"
	exit 1
}

diagnostics() {
	echo "── diagnostics ──"
	echo "quiver ps:"
	quiver ps --all -o json 2>&1 | head -60 || true
	local log="$HOME/.quiver/logs/Quiver.log"
	if [[ -f "$log" ]]; then
		echo "daemon log (last 40 non-request lines):"
		grep -v '"type":"http_request"' "$log" | tail -40 || true
	fi
	if [[ -f "$HOME/daemon.out" ]]; then
		echo "daemon stderr (last 20 non-route lines):"
		grep -v -e GIN-debug -e '"type":"http_request"' "$HOME/daemon.out" | tail -20 || true
	fi
}

on_error() {
	local code=$1 line=$2 cmd=$3
	report_failure "command exited $code at run.sh:$line: $cmd"
	exit "$code"
}
trap 'on_error $? $LINENO "$BASH_COMMAND"' ERR

cleanup() {
	local pid
	for pid in "$POLLER_PID" "$GITHOST_PID" "$DAEMON_PID"; do
		if [[ -n "$pid" ]]; then
			kill "$pid" 2>/dev/null || true
		fi
	done
	if ((HOSTS_REDIRECTED)); then
		sudo -n /usr/local/sbin/e2e-hosts off || true
	fi
}
trap cleanup EXIT

phase_done() {
	SUMMARY+=("$(printf 'Phase %s  PASS  %4ss  %s' "$1" "$((SECONDS - PHASE_START))" "$2")")
}

# ─── assertions ───────────────────────────────────────────────────────────────

# expect_eq LABEL ACTUAL EXPECTED
expect_eq() {
	if [[ "$2" != "$3" ]]; then
		fail "$1: expected '$3', got '$2'"
	fi
	ok "$1 = $3"
}

expect_file() {
	[[ -f "$1" ]] || fail "expected file $1 to exist"
	ok "exists: $1"
}

expect_no_file() {
	[[ ! -e "$1" ]] || fail "expected $1 to be gone"
	ok "gone: $1"
}

# ─── quiver helpers ───────────────────────────────────────────────────────────

# The CLI renders JSON whenever stdout is not a terminal.
show() {
	quiver arrow show "$1" -o json
}

field() {
	show "$1" | jq -r "$2"
}

# wait_for NS JQ_PREDICATE DESCRIPTION polls `arrow show` until the predicate
# holds, failing at the deadline. It never sleeps longer than a poll interval.
wait_for() {
	local ns=$1 predicate=$2 what=$3 deadline=$((SECONDS + WAIT_SECONDS)) detail
	while ((SECONDS < deadline)); do
		if detail=$(show "$ns" 2>&1) && jq -e "$predicate" <<<"$detail" >/dev/null 2>&1; then
			ok "$what (after $((SECONDS - deadline + WAIT_SECONDS))s)"
			return 0
		fi
		sleep 1
	done
	echo "last detail: $detail" >&2
	fail "timed out after ${WAIT_SECONDS}s waiting for: $what"
}

# lifecycle OP NS [ARGS...] runs a lifecycle command under the wait deadline
# and prints its JSON result.
lifecycle() {
	local op=$1 ns=$2
	shift 2
	timeout "$WAIT_SECONDS" quiver "$op" "$@" "$ns"
}

# listed NS prints the `arrow list` entry filed under NS's identity.
listed() {
	local bare=${1%@*} ref=${1##*@}
	quiver arrow list -o json |
		jq -c --arg ns "$bare" --arg ref "$ref" '.[] | select(.namespace == $ns and .ref == $ref)'
}

# peeled_commit URL TAG prints the commit TAG points at, peeling an annotated tag.
peeled_commit() {
	local refs="" peeled attempt
	# A transient DNS or TLS blip on the real remote is retried, not reported.
	for attempt in 1 2 3; do
		if refs=$(git ls-remote "$1" "refs/tags/$2" "refs/tags/$2^{}"); then
			break
		fi
		((attempt < 3)) || fail "git ls-remote $1 failed 3 times"
		sleep 3
	done
	peeled=$(awk -v r="refs/tags/$2^{}" '$2 == r { print $1 }' <<<"$refs")
	if [[ -z "$peeled" ]]; then
		peeled=$(awk -v r="refs/tags/$2" '$2 == r { print $1 }' <<<"$refs")
	fi
	[[ -n "$peeled" ]] || fail "tag $2 not found on $1"
	echo "$peeled"
}

# start_daemon [BINARY] runs BINARY (default: the quiver on PATH) as the
# daemon and waits until it answers.
start_daemon() {
	"${1:-quiver}" daemon >"$HOME/daemon.out" 2>&1 &
	DAEMON_PID=$!
	local deadline=$((SECONDS + 60))
	until curl -sf --unix-socket "$SOCK" http://quiver/v0/health >/dev/null 2>&1; do
		kill -0 "$DAEMON_PID" 2>/dev/null || fail "daemon exited during startup"
		((SECONDS < deadline)) || fail "daemon did not answer /v0/health within 60s"
		sleep 0.2
	done
	ok "daemon up (pid $DAEMON_PID) on $SOCK"
}

stop_daemon() {
	[[ -n "$DAEMON_PID" ]] || return 0
	kill "$DAEMON_PID" 2>/dev/null || true
	wait "$DAEMON_PID" 2>/dev/null || true
	DAEMON_PID=""
	ok "daemon stopped"
}

# ─── Phase A: the real Crowbar ────────────────────────────────────────────────

phase_a() {
	banner "Phase A — real github.com/char2cs/crowbar@nightly"
	PHASE_START=$SECONDS

	local arch
	case "$(uname -m)" in
	x86_64) arch=amd64 ;;
	aarch64 | arm64) arch=aarch64 ;;
	*) fail "no Crowbar AppImage for $(uname -m)" ;;
	esac
	local asset_url="$CROWBAR_REPO/releases/download/nightly/Crowbar_nightly_${arch}.AppImage"

	step "resolve the rolling tag on the real remote"
	local remote_commit
	remote_commit=$(peeled_commit "$CROWBAR_REPO" nightly)
	ok "refs/tags/nightly -> $remote_commit"

	step "quiver arrow add + install $CROWBAR"
	local out
	out=$(quiver arrow add "$CROWBAR")
	expect_eq "add action" "$(jq -r .action <<<"$out")" add
	if ! appimage_executable; then
		require_emulation
		phase_a_emulated "$remote_commit" "$asset_url"
		return 0
	fi
	local t0=$SECONDS
	out=$(lifecycle install "$CROWBAR")
	expect_eq "install outcome" "$(jq -r .outcome <<<"$out")" success
	ok "install took $((SECONDS - t0))s"
	wait_for "$CROWBAR" '.state == "ready"' "state ready"

	step "arrow show: identity and resolved version"
	local detail
	detail=$(show "$CROWBAR")
	expect_eq "namespace" "$(jq -r .namespace <<<"$detail")" "$CROWBAR"
	expect_eq "selector_kind" "$(jq -r .selector_kind <<<"$detail")" channel
	expect_eq "resolved_ref" "$(jq -r .resolved_ref <<<"$detail")" nightly
	expect_eq "installed_commit (vs git ls-remote)" "$(jq -r .installed_commit <<<"$detail")" "$remote_commit"
	expect_eq "available" "$(jq -c '.available // "absent"' <<<"$detail")" '"absent"'
	expect_eq "outdated" "$(jq -r .outdated <<<"$detail")" false
	expect_eq "state" "$(jq -r .state <<<"$detail")" ready
	expect_eq "\${REF} the install ran with" "$(jq -r .last_return.variables.REF <<<"$detail")" nightly

	local install_path
	install_path=$(jq -r .last_return.variables.INSTALL_PATH <<<"$detail")
	expect_eq "install path" "$install_path" "$HOME/.quiver/namespaces/$CROWBAR"

	step "files crowbar's linux manifest creates"
	local appimage="$install_path/Crowbar.AppImage"
	local launcher="$HOME/.local/bin/crowbar"
	local desktop="$HOME/.local/share/applications/crowbar.desktop"
	local icon="$install_path/crowbar.png"
	expect_file "$appimage"
	expect_file "$icon"
	expect_file "$install_path/squashfs-root/AppRun"
	[[ -x "$install_path/squashfs-root/AppRun" ]] || fail "AppRun is not executable"
	ok "AppRun is executable (AppImage extracted)"
	expect_file "$launcher"
	[[ -x "$launcher" ]] || fail "launcher is not executable"
	grep -qF "exec \"$install_path/squashfs-root/AppRun\"" "$launcher" || fail "launcher does not exec the extracted AppRun"
	ok "launcher execs $install_path/squashfs-root/AppRun"
	expect_file "$desktop"
	grep -qx "Exec=$launcher" "$desktop" || fail "desktop entry Exec is not $launcher"
	ok "desktop entry Exec=$launcher"
	expect_eq "icon is a PNG" "$(head -c 4 "$icon" | tail -c 3)" PNG

	step "\${REF} really was the resolved ref: the AppImage is the /releases/download/nightly/ asset"
	local remote_size local_size
	remote_size=$(curl -sIL "$asset_url" | tr -d '\r' | awk 'tolower($1) == "content-length:" { n = $2 } END { print n }')
	local_size=$(stat -c %s "$appimage")
	[[ "$remote_size" -gt 1000000 ]] || fail "unexpected Content-Length '$remote_size' for $asset_url"
	expect_eq "Crowbar.AppImage size vs $asset_url" "$local_size" "$remote_size"

	step "arrow list files the row under its identity"
	local entry
	entry=$(listed "$CROWBAR")
	[[ -n "$entry" ]] || fail "arrow list has no $CROWBAR"
	expect_eq "list resolved_ref" "$(jq -r .resolved_ref <<<"$entry")" nightly
	expect_eq "list state" "$(jq -r .state <<<"$entry")" ready

	step "quiver update: nothing newer is a no-op"
	out=$(lifecycle update "$CROWBAR")
	expect_eq "update reason" "$(jq -r .reason <<<"$out")" "already up to date, nothing to do"
	expect_eq "state after no-op update" "$(field "$CROWBAR" .state)" ready
	expect_eq "installed_commit unchanged" "$(field "$CROWBAR" .installed_commit)" "$remote_commit"

	step "quiver uninstall -y"
	out=$(lifecycle uninstall "$CROWBAR" -y)
	expect_eq "uninstall outcome" "$(jq -r .outcome <<<"$out")" success
	wait_for "$CROWBAR" '.state == "absent"' "state absent"
	expect_no_file "$appimage"
	expect_no_file "$icon"
	expect_no_file "$install_path/squashfs-root"
	expect_no_file "$launcher"
	expect_no_file "$desktop"

	step "quiver arrow remove -y: the row leaves the library"
	out=$(quiver arrow remove -y "$CROWBAR")
	expect_eq "remove action" "$(jq -r .action <<<"$out")" remove
	[[ -z "$(listed "$CROWBAR")" ]] || fail "$CROWBAR is still listed after remove"
	ok "$CROWBAR no longer listed"
	expect_eq "user_installed after remove" "$(field "$CROWBAR" .user_installed)" false

	phase_done A "crowbar@nightly installed at $remote_commit, no-op update, uninstalled, removed"
}

# appimage_executable reports whether this host can exec an ELF carrying the
# AppImage type-2 magic ("AI\x02" in e_ident's padding). A native kernel
# ignores those bytes; a binfmt_misc emulator (qemu, Rosetta) registered with
# a mask over the full e_ident does not match them and the exec fails with
# ENOEXEC — so an emulated linux/amd64 container cannot extract any AppImage.
appimage_executable() {
	local probe="$HOME/appimage-probe"
	cp /bin/true "$probe"
	printf 'AI\002' | dd of="$probe" bs=1 seek=8 conv=notrunc 2>/dev/null
	"$probe" 2>/dev/null
}

# container_goarch prints this container's architecture as Go names it,
# which is what quiver reports as ${PLATFORM}.
container_goarch() {
	case "$(uname -m)" in
	x86_64) echo amd64 ;;
	aarch64 | arm64) echo arm64 ;;
	*) fail "unsupported architecture $(uname -m)" ;;
	esac
}

# require_emulation runs once an AppImage-style ELF failed to exec, and lets
# Phase A take its emulated branch only when emulation is CONFIRMED:
#   1. a plain copy of /bin/true at the same path in $HOME execs, so the
#      failure is the AppImage magic itself, not a noexec home, seccomp or a
#      missing /bin/true; and
#   2. something shows this container runs under binfmt emulation:
#      E2E_HOST_ARCH (the Docker host's architecture, passed by the Makefile)
#      differs from the container's own, or /proc/cpuinfo reports Rosetta's
#      "VirtualApple" vendor, or a qemu-*/rosetta binfmt_misc entry is visible.
# Anything else is a real exec failure on native hardware and fails loudly.
require_emulation() {
	local plain="$HOME/native-probe" arch
	cp /bin/true "$plain"
	"$plain" || fail "a plain ELF copied to $HOME does not exec: this host cannot run anything from its home"
	arch=$(container_goarch)
	if [[ -n "${E2E_HOST_ARCH:-}" && "$E2E_HOST_ARCH" != "$arch" ]]; then
		ok "emulated: host $E2E_HOST_ARCH runs this linux/$arch container"
		return 0
	fi
	if grep -q '^vendor_id.*VirtualApple' /proc/cpuinfo 2>/dev/null; then
		ok "emulated: /proc/cpuinfo reports Rosetta (VirtualApple)"
		return 0
	fi
	if compgen -G '/proc/sys/fs/binfmt_misc/qemu-*' >/dev/null || [[ -e /proc/sys/fs/binfmt_misc/rosetta ]]; then
		ok "emulated: a qemu/rosetta binfmt_misc entry is registered"
		return 0
	fi
	fail "an AppImage-style ELF does not exec, yet nothing shows linux/$arch is emulated (E2E_HOST_ARCH='${E2E_HOST_ARCH:-}'): a real exec failure"
}

# phase_a_emulated covers what an emulated host can: the real asset for this
# platform downloaded through ${REF}=nightly, then the install failing exactly
# at extraction, and the row cleaned up. It never counts as a full Phase A.
phase_a_emulated() {
	local remote_commit=$1 asset_url=$2
	EMULATED=1
	echo "  ! this host cannot exec AppImages (binfmt emulation): extraction is expected to fail"

	step "install runs up to extraction"
	local out=""
	if out=$(lifecycle install "$CROWBAR" 2>&1); then
		fail "install succeeded on a host that cannot exec AppImages: $out"
	fi
	echo "  $out"
	local detail
	detail=$(show "$CROWBAR")
	expect_eq "state after the failed install" "$(jq -r .state <<<"$detail")" absent
	expect_eq "failed step" "$(jq -r '[.last_return.steps[] | select(.status == "failed") | .title] | join(",")' <<<"$detail")" \
		"Extracting the Crowbar AppImage"
	expect_eq "selector_kind" "$(jq -r .selector_kind <<<"$detail")" channel
	expect_eq "resolved_ref" "$(jq -r .resolved_ref <<<"$detail")" nightly
	expect_eq "resolved commit (vs git ls-remote)" "$(jq -r .installed_commit <<<"$detail")" "$remote_commit"
	expect_eq "\${REF} the install ran with" "$(jq -r .last_return.variables.REF <<<"$detail")" nightly
	expect_eq "platform" "$(jq -r .last_return.variables.PLATFORM <<<"$detail")" "linux/$(container_goarch)"

	local install_path appimage remote_size
	install_path=$(jq -r .last_return.variables.INSTALL_PATH <<<"$detail")
	appimage="$install_path/Crowbar.AppImage"
	expect_file "$appimage"
	remote_size=$(curl -sIL "$asset_url" | tr -d '\r' | awk 'tolower($1) == "content-length:" { n = $2 } END { print n }')
	expect_eq "Crowbar.AppImage size vs $asset_url" "$(stat -c %s "$appimage")" "$remote_size"
	expect_eq "icon is a PNG" "$(head -c 4 "$install_path/crowbar.png" | tail -c 3)" PNG

	step "quiver arrow remove -y"
	out=$(quiver arrow remove -y "$CROWBAR")
	[[ -z "$(listed "$CROWBAR")" ]] || fail "$CROWBAR is still listed after remove"
	ok "$CROWBAR no longer listed"

	phase_done "A*" "EMULATED: $(container_goarch) asset fetched via \${REF}=nightly at $remote_commit; extraction impossible under binfmt emulation"
}

# ─── Phase B: a moved rolling tag on a local git host ─────────────────────────

git_setup() {
	git config --global user.email e2e@quiver.invalid
	git config --global user.name "quiver e2e"
	git config --global init.defaultBranch main
	git config --global advice.detachedHead false
}

start_githost() {
	githost -root "$GIT_ROOT" -cert /etc/githost/cert.pem -key /etc/githost/key.pem \
		>"$HOME/githost.out" 2>&1 &
	GITHOST_PID=$!
}

# publish NAME seeds a work repo from the fixture, tags it nightly and serves
# it bare at https://localhost/tester/NAME.
publish() {
	local name=$1 work="$HOME/work/$1"
	mkdir -p "$HOME/work" "$GIT_ROOT/tester"
	git init -q "$work"
	cp "$FIXTURE_SRC/arrow.yaml" "$work/arrow.yaml"
	git -C "$work" add arrow.yaml
	git -C "$work" commit -qm "build 1"
	git -C "$work" tag nightly
	git clone -q --bare "$work" "$GIT_ROOT/tester/$name"
	wait_served "https://localhost/tester/$name"
}

# wait_served URL waits until githost answers for URL: every phase that
# publishes a repo depends on it, whichever phase runs first.
wait_served() {
	local deadline=$((SECONDS + 30))
	until git ls-remote "$1" >/dev/null 2>&1; do
		((SECONDS < deadline)) || fail "githost did not serve $1 within 30s"
		sleep 0.2
	done
}

# move_tag NAME FROM TO commits the fixture with build FROM rewritten to TO,
# force-moves nightly onto it as an annotated tag and pushes both, printing
# the new commit.
move_tag() {
	local name=$1 work="$HOME/work/$1"
	sed -i "s/build-$2/build-$3/g" "$work/arrow.yaml"
	git -C "$work" commit -qam "build $3"
	git -C "$work" tag -f -a -m "nightly build $3" nightly >/dev/null
	git -C "$work" push -q --force "$GIT_ROOT/tester/$name" main refs/tags/nightly
	git -C "$work" rev-parse HEAD
}

# start_poller NS reads GET /v0/arrow/NS in a loop and records every status.
start_poller() {
	local path
	path="/v0/arrow/$(jq -rn --arg ns "$1" '$ns | @uri')"
	: >"$HOME/poll.codes"
	(
		while true; do
			curl -s -o /dev/null -w '%{http_code}\n' --unix-socket "$SOCK" "http://quiver$path" >>"$HOME/poll.codes" || echo 000 >>"$HOME/poll.codes"
			sleep 0.05
		done
	) &
	POLLER_PID=$!
}

stop_poller() {
	kill "$POLLER_PID" 2>/dev/null || true
	wait "$POLLER_PID" 2>/dev/null || true
	POLLER_PID=""
}

phase_b() {
	banner "Phase B — rolling tag force-moved on a local HTTPS git host"
	PHASE_START=$SECONDS
	local name=rolling-tool
	local ns="localhost/tester/$name@nightly"
	local url="https://localhost/tester/$name"
	local workdir="$HOME/.quiver/namespaces/$ns"

	step "serve the fixture from $url"
	publish "$name"
	local c1
	c1=$(git -C "$HOME/work/$name" rev-parse HEAD)
	expect_eq "nightly on $url" "$(peeled_commit "$url" nightly)" "$c1"

	step "install $ns"
	local out
	out=$(quiver arrow add "$ns")
	expect_eq "add action" "$(jq -r .action <<<"$out")" add
	start_poller "$ns"
	out=$(lifecycle install "$ns")
	expect_eq "install outcome" "$(jq -r .outcome <<<"$out")" success
	wait_for "$ns" '.state == "ready"' "state ready"
	local detail
	detail=$(show "$ns")
	expect_eq "selector_kind" "$(jq -r .selector_kind <<<"$detail")" channel
	expect_eq "resolved_ref" "$(jq -r .resolved_ref <<<"$detail")" nightly
	expect_eq "installed_commit" "$(jq -r .installed_commit <<<"$detail")" "$c1"
	expect_eq "available" "$(jq -c '.available // "absent"' <<<"$detail")" '"absent"'
	expect_eq "install step \${REF}" "$(cat "$workdir/install-ref")" nightly
	expect_eq "installed build" "$(cat "$workdir/build")" build-1

	step "output redirected to /dev/null: a successful command still exits 0"
	quiver arrow show "$ns" >/dev/null || fail "arrow show >/dev/null exited $?"
	ok "arrow show >/dev/null exits 0"

	step "force-move nightly to a new commit (annotated tag)"
	local c2
	c2=$(move_tag "$name" 1 2)
	expect_eq "nightly on $url (peeled)" "$(peeled_commit "$url" nightly)" "$c2"
	[[ "$c2" != "$c1" ]] || fail "the moved tag did not change commit"
	expect_eq "row before the check still at" "$(field "$ns" .installed_commit)" "$c1"

	step "quiver arrow refresh: the check records what is ahead"
	out=$(quiver arrow refresh "$ns")
	expect_eq "refresh action" "$(jq -r .action <<<"$out")" refresh
	detail=$(show "$ns")
	expect_eq "namespace" "$(jq -r .namespace <<<"$detail")" "$ns"
	expect_eq "available.ref" "$(jq -r .available.ref <<<"$detail")" nightly
	expect_eq "available.commit" "$(jq -r .available.commit <<<"$detail")" "$c2"
	expect_eq "outdated" "$(jq -r .outdated <<<"$detail")" true
	expect_eq "installed_commit (not yet advanced)" "$(jq -r .installed_commit <<<"$detail")" "$c1"

	step "quiver update: advance the row in place"
	out=$(lifecycle update "$ns")
	expect_eq "update outcome" "$(jq -r .outcome <<<"$out")" success
	# The bracket's commit runs after the update's steps end, detached from
	# the run the CLI waits on, so the row is polled until it lands.
	wait_for "$ns" ".installed_commit == \"$c2\" and .available == null and .state == \"ready\"" \
		"row advanced to $c2 with nothing ahead"
	detail=$(show "$ns")
	expect_eq "identity unchanged" "$(jq -r .namespace <<<"$detail")" "$ns"
	expect_eq "selector_kind" "$(jq -r .selector_kind <<<"$detail")" channel
	expect_eq "resolved_ref" "$(jq -r .resolved_ref <<<"$detail")" nightly
	expect_eq "outdated" "$(jq -r .outdated <<<"$detail")" false
	expect_eq "update step \${REF} runs" "$(paste -sd, "$workdir/update-refs")" nightly
	expect_eq "build the update ran (the target's manifest)" "$(cat "$workdir/build")" build-2
	expect_eq "workdir data preserved across the update" "$(cat "$workdir/user-data")" kept
	local entry
	entry=$(listed "$ns")
	[[ -n "$entry" ]] || fail "arrow list has no $ns"
	expect_eq "list: one row, same identity" "$(quiver arrow list -o json | jq --arg n "localhost/tester/$name" '[.[] | select(.namespace == $n)] | length')" 1

	step "quiver update again: nothing newer is a no-op"
	out=$(lifecycle update "$ns")
	expect_eq "update reason" "$(jq -r .reason <<<"$out")" "already up to date, nothing to do"
	expect_eq "update steps did not run again" "$(paste -sd, "$workdir/update-refs")" nightly

	step "GET /v0/arrow/$ns answered 200 throughout"
	stop_poller
	local polls bad
	polls=$(wc -l <"$HOME/poll.codes")
	bad=$(grep -cv '^200$' "$HOME/poll.codes" || true)
	((polls > 5)) || fail "poller ran only $polls reads"
	expect_eq "non-200 answers out of $polls reads" "$bad" 0

	step "uninstall + remove"
	out=$(lifecycle uninstall "$ns" -y)
	expect_eq "uninstall outcome" "$(jq -r .outcome <<<"$out")" success
	wait_for "$ns" '.state == "absent"' "state absent"
	expect_no_file "$workdir/build"
	expect_no_file "$workdir/user-data"
	out=$(quiver arrow remove -y "$ns")
	[[ -z "$(listed "$ns")" ]] || fail "$ns is still listed after remove"
	ok "$ns no longer listed"

	phase_done B "nightly moved $c1 -> $c2: available, updated, advanced in place, $polls reads all 200"
}

# ─── Phase C: the CLI-booted daemon keeps an update's commit ─────────────────

# expect_idle_stopped NS asserts the CLI stops the daemon it booted once
# nothing is left to settle. The row reads advanced as soon as the commit's
# Advance lands, a moment before the commit clears its badge and stops
# settling, so each round runs another idle-capable read of NS; the idle-stop
# removes the PID file before that command exits.
expect_idle_stopped() {
	local deadline=$((SECONDS + 60))
	until [[ ! -e "$PID_FILE" ]]; do
		((SECONDS < deadline)) || fail "the CLI left its daemon running (pid $(cat "$PID_FILE"))"
		show "$1" >/dev/null
		sleep 0.2
	done
	ok "the CLI idle-stopped its daemon"
}

phase_c() {
	banner "Phase C — CLI-booted daemon: an update's commit survives the idle-stop"
	PHASE_START=$SECONDS
	local name=autoboot-tool
	local ns="localhost/tester/$name@nightly"
	local workdir="$HOME/.quiver/namespaces/$ns"

	step "hand the socket back to the CLI's auto-boot"
	stop_daemon
	publish "$name"

	step "install, move the tag, refresh"
	local out c1
	c1=$(git -C "$HOME/work/$name" rev-parse HEAD)
	out=$(quiver arrow add "$ns")
	out=$(lifecycle install "$ns")
	expect_eq "install outcome" "$(jq -r .outcome <<<"$out")" success
	local c2
	c2=$(move_tag "$name" 1 2)
	out=$(quiver arrow refresh "$ns")
	expect_eq "available.commit" "$(field "$ns" .available.commit)" "$c2"
	expect_eq "installed_commit" "$(field "$ns" .installed_commit)" "$c1"

	step "update, then read the row: the CLI stops its daemon only once the commit landed"
	out=$(lifecycle update "$ns")
	expect_eq "update outcome" "$(jq -r .outcome <<<"$out")" success
	show "$ns" | jq -c '{state, installed_commit, available}'
	# Each read is followed by the CLI's idle check; a read that finds the
	# update still settling leaves the daemon up, so the commit is never cut
	# off. A healthy commit lands in well under a second.
	WAIT_SECONDS=60 wait_for "$ns" ".installed_commit == \"$c2\" and .available == null" "row advanced to $c2"
	expect_idle_stopped "$ns"
	local detail
	detail=$(show "$ns")
	expect_eq "after a fresh boot: installed_commit" "$(jq -r .installed_commit <<<"$detail")" "$c2"
	expect_eq "after a fresh boot: available" "$(jq -c '.available // "absent"' <<<"$detail")" '"absent"'
	expect_eq "update steps ran once" "$(paste -sd, "$workdir/update-refs")" nightly

	step "update again after a moved tag, unrefreshed: the CLI waits for this run's own steps"
	# The row's last run was an update, and the bracket re-announces that
	# return before this update begins; the CLI must not take it for this
	# run's end. The update step takes seconds, so an early return shows.
	local c3
	c3=$(move_tag "$name" 2 3)
	out=$(lifecycle update "$ns")
	expect_eq "update outcome" "$(jq -r .outcome <<<"$out")" success
	expect_eq "this update's steps had run when it returned" "$(cat "$workdir/build")" build-3
	expect_eq "update steps run" "$(paste -sd, "$workdir/update-refs")" nightly,nightly
	WAIT_SECONDS=60 wait_for "$ns" ".installed_commit == \"$c3\" and .available == null" "row advanced to $c3"
	expect_idle_stopped "$ns"

	step "uninstall + remove"
	out=$(lifecycle uninstall "$ns" -y)
	expect_eq "uninstall outcome" "$(jq -r .outcome <<<"$out")" success
	out=$(quiver arrow remove -y "$ns")
	[[ -z "$(listed "$ns")" ]] || fail "$ns is still listed after remove"
	ok "$ns no longer listed"

	phase_done C "CLI-booted daemon: $c1 -> $c2 -> $c3 committed across idle-stops, no update ran twice"
}

# ─── Phase D: a stable channel on a clone-only host ──────────────────────────

# ensure_daemon hands the socket to a daemon this script runs: Phase C leaves
# it to the CLI's auto-boot, and curl never boots one.
ensure_daemon() {
	[[ -z "$DAEMON_PID" ]] || return 0
	local deadline=$((SECONDS + 60))
	while [[ -e "$PID_FILE" ]]; do
		((SECONDS < deadline)) || fail "the CLI-booted daemon (pid $(cat "$PID_FILE")) never idle-stopped"
		quiver arrow list >/dev/null
		sleep 0.2
	done
	start_daemon
}

# release NAME TAG commits a new revision of NAME's work repo, tags it TAG
# and pushes both, printing the commit. Each release appends its tag to the
# manifest's history, so no two releases share a commit.
release() {
	local name=$1 tag=$2 work="$HOME/work/$1"
	echo "- $tag" >>"$work/ARROW.md"
	git -C "$work" add ARROW.md
	git -C "$work" commit -qm "release $tag"
	git -C "$work" tag "$tag"
	if [[ -d "$GIT_ROOT/tester/$name" ]]; then
		git -C "$work" push -q "$GIT_ROOT/tester/$name" main "refs/tags/$tag"
	fi
	git -C "$work" rev-parse HEAD
}

# publish_releases NAME TAG... serves a repo at https://localhost/tester/NAME
# holding one release per TAG, in order.
publish_releases() {
	local name=$1 work="$HOME/work/$1" tag
	shift
	mkdir -p "$HOME/work" "$GIT_ROOT/tester"
	git init -q "$work"
	cp "$MULTI_FIXTURE" "$work/ARROW.md"
	for tag in "$@"; do
		release "$name" "$tag" >/dev/null
	done
	git clone -q --bare "$work" "$GIT_ROOT/tester/$name"
	wait_served "https://localhost/tester/$name"
}

# api METHOD NS [SUFFIX [BODY]] calls the daemon over its socket and prints
# the HTTP status; the response body lands in $HOME/api.body.
api() {
	local method=$1 path args=()
	path="/v0/arrow/$(jq -rn --arg ns "$2" '$ns | @uri')${3:-}"
	if [[ -n "${4:-}" ]]; then
		args=(-H 'Content-Type: application/json' --data "$4")
	fi
	curl -s -o "$HOME/api.body" -w '%{http_code}' --unix-socket "$SOCK" -X "$method" "${args[@]}" "http://quiver$path"
}

phase_d() {
	banner "Phase D — stable channel on a clone-only git host"
	PHASE_START=$SECONDS
	ensure_daemon
	local name=multi-tool
	local bare="localhost/tester/$name"
	local ns="$bare@stable"
	local url="https://localhost/tester/$name"
	local workdir="$HOME/.quiver/namespaces/$ns"

	step "serve stable-26.5.0, stable-26.5.1, stable-26.6.0 from $url"
	publish_releases "$name" stable-26.5.0 stable-26.5.1 stable-26.6.0
	local c260
	c260=$(peeled_commit "$url" stable-26.6.0)
	if git ls-remote --exit-code "$url" refs/heads/stable refs/tags/stable >/dev/null; then
		fail "$url must hold no ref named stable"
	fi
	ok "$url holds no ref named stable"

	step "quiver arrow add + install $bare (refless)"
	local out detail
	out=$(quiver arrow add "$bare")
	expect_eq "add action" "$(jq -r .action <<<"$out")" add
	out=$(lifecycle install "$bare")
	expect_eq "install outcome" "$(jq -r .outcome <<<"$out")" success
	wait_for "$ns" '.state == "ready"' "state ready"
	detail=$(show "$ns")
	expect_eq "identity" "$(jq -r .namespace <<<"$detail")" "$ns"
	expect_eq "selector_kind" "$(jq -r .selector_kind <<<"$detail")" channel
	expect_eq "resolved_ref" "$(jq -r .resolved_ref <<<"$detail")" stable-26.6.0
	expect_eq "installed_commit" "$(jq -r .installed_commit <<<"$detail")" "$c260"
	expect_eq "install step \${REF}" "$(cat "$workdir/install-ref")" stable-26.6.0
	start_poller "$ns"

	step "publish stable-26.7.0, then quiver arrow refresh"
	local c270
	c270=$(release "$name" stable-26.7.0)
	expect_eq "stable-26.7.0 on $url" "$(peeled_commit "$url" stable-26.7.0)" "$c270"
	out=$(quiver arrow refresh "$ns")
	expect_eq "refresh action" "$(jq -r .action <<<"$out")" refresh
	detail=$(show "$ns")
	expect_eq "available.ref" "$(jq -r .available.ref <<<"$detail")" stable-26.7.0
	expect_eq "available.commit" "$(jq -r .available.commit <<<"$detail")" "$c270"
	expect_eq "resolved_ref (not yet advanced)" "$(jq -r .resolved_ref <<<"$detail")" stable-26.6.0

	step "quiver update: advance the row in place to stable-26.7.0"
	out=$(lifecycle update "$ns")
	expect_eq "update outcome" "$(jq -r .outcome <<<"$out")" success
	wait_for "$ns" ".resolved_ref == \"stable-26.7.0\" and .available == null and .state == \"ready\"" \
		"row advanced to stable-26.7.0 with nothing ahead"
	detail=$(show "$ns")
	expect_eq "identity unchanged" "$(jq -r .namespace <<<"$detail")" "$ns"
	expect_eq "selector_kind" "$(jq -r .selector_kind <<<"$detail")" channel
	expect_eq "installed_commit" "$(jq -r .installed_commit <<<"$detail")" "$c270"
	expect_eq "update step \${REF} runs" "$(paste -sd, "$workdir/update-refs")" stable-26.7.0
	expect_eq "list: one row for $bare" "$(quiver arrow list -o json | jq --arg n "$bare" '[.[] | select(.namespace == $n)] | length')" 1

	step "GET /v0/arrow/$ns answered 200 throughout"
	stop_poller
	local polls bad
	polls=$(wc -l <"$HOME/poll.codes")
	bad=$(grep -cv '^200$' "$HOME/poll.codes" || true)
	((polls > 5)) || fail "poller ran only $polls reads"
	expect_eq "non-200 answers out of $polls reads" "$bad" 0

	step "uninstall + remove"
	out=$(lifecycle uninstall "$ns" -y)
	expect_eq "uninstall outcome" "$(jq -r .outcome <<<"$out")" success
	out=$(quiver arrow remove -y "$ns")
	[[ -z "$(listed "$ns")" ]] || fail "$ns is still listed after remove"
	ok "$ns no longer listed"

	phase_d_adopt
	phase_done D "stable on a clone-only host: install at stable-26.6.0, updated to stable-26.7.0 in place ($polls reads all 200), adopted stable-26.5.0 offered and updated to stable-26.6.0"
}

# phase_d_adopt declares an older stable member installed over the API, the
# way a client that installed itself does: the fixture's preinstalled probe
# finds its marker, so POST lands the runtime ready, and /adopt then records
# the build it actually runs.
phase_d_adopt() {
	local name=multi-adopt
	local ns="localhost/tester/$name@stable"
	local url="https://localhost/tester/$name"
	local workdir="$HOME/.quiver/namespaces/$ns"

	step "adopt stable-26.5.0 on $ns"
	publish_releases "$name" stable-26.5.0 stable-26.5.1 stable-26.6.0
	local c250 c260
	c250=$(peeled_commit "$url" stable-26.5.0)
	c260=$(peeled_commit "$url" stable-26.6.0)
	mkdir -p "$(dirname "/tmp/preinstalled/$ns")"
	: >"/tmp/preinstalled/$ns"
	expect_eq "POST /v0/arrow/$ns" "$(api POST "$ns")" 201
	expect_eq "state (the preinstalled probe detected it)" "$(field "$ns" .state)" ready
	expect_eq "POST /v0/arrow/$ns/adopt" "$(api POST "$ns" /adopt '{"resolved_ref":"stable-26.5.0"}')" 201
	local detail
	detail=$(show "$ns")
	expect_eq "identity" "$(jq -r .namespace <<<"$detail")" "$ns"
	expect_eq "selector_kind" "$(jq -r .selector_kind <<<"$detail")" channel
	expect_eq "resolved_ref" "$(jq -r .resolved_ref <<<"$detail")" stable-26.5.0
	expect_eq "installed_commit" "$(jq -r .installed_commit <<<"$detail")" "$c250"
	expect_eq "user_installed" "$(jq -r .user_installed <<<"$detail")" true

	step "quiver arrow refresh offers the newest member"
	quiver arrow refresh "$ns" >/dev/null
	detail=$(show "$ns")
	expect_eq "available.ref" "$(jq -r .available.ref <<<"$detail")" stable-26.6.0
	expect_eq "available.commit" "$(jq -r .available.commit <<<"$detail")" "$c260"
	expect_eq "resolved_ref (adopted, not advanced)" "$(jq -r .resolved_ref <<<"$detail")" stable-26.5.0
	expect_eq "outdated" "$(jq -r .outdated <<<"$detail")" true

	step "quiver update moves the adopted row to stable-26.6.0 in place"
	local out
	out=$(lifecycle update "$ns")
	expect_eq "update outcome" "$(jq -r .outcome <<<"$out")" success
	wait_for "$ns" ".resolved_ref == \"stable-26.6.0\" and .available == null and .state == \"ready\"" \
		"adopted row advanced to stable-26.6.0"
	expect_eq "identity unchanged" "$(field "$ns" .namespace)" "$ns"
	expect_eq "update step \${REF} runs" "$(paste -sd, "$workdir/update-refs")" stable-26.6.0

	step "uninstall + remove"
	out=$(lifecycle uninstall "$ns" -y)
	expect_eq "uninstall outcome" "$(jq -r .outcome <<<"$out")" success
	quiver arrow remove -y "$ns" >/dev/null
	[[ -z "$(listed "$ns")" ]] || fail "$ns is still listed after remove"
	ok "$ns no longer listed"
}

# ─── Phases E and F: quiver.core updates itself ──────────────────────────────

readonly SELF=github.com/rabbytesoftware/quiver.core
readonly SELF_URL=https://github.com/rabbytesoftware/quiver.core
readonly SELF_DIR=/opt/e2e/self
readonly SELF_BARE=$GIT_ROOT/rabbytesoftware/quiver.core
readonly SELF_RELEASES=/srv/releases/rabbytesoftware/quiver.core

# self_hosts on|off points github.com and raw.githubusercontent.com at the
# githost, or back at the real hosts.
self_hosts() {
	sudo -n /usr/local/sbin/e2e-hosts "$1"
	if [[ "$1" == on ]]; then
		HOSTS_REDIRECTED=1
		ok "github.com and raw.githubusercontent.com resolve to this container"
	else
		HOSTS_REDIRECTED=0
	fi
}

# self_build BUILD prints the path of BUILD's daemon binary.
self_build() {
	echo "$SELF_DIR/$1/quiver-linux-$(container_goarch)"
}

# publish_self_release TAG BUILD serves BUILD's assets as release TAG,
# replacing whatever TAG served before, as the nightly workflow's
# delete-and-recreate does.
publish_self_release() {
	rm -rf "${SELF_RELEASES:?}/$1"
	mkdir -p "$SELF_RELEASES/$1"
	cp "$SELF_DIR/$2"/* "$SELF_RELEASES/$1/"
}

# serve_self publishes the stand-in quiver.core repository at $SELF_URL, as
# the release workflows leave it before either phase moves anything:
# nightly-latest at N1, stable-26.5 (and beta-26.5-1) at S1.
serve_self() {
	# shellcheck source=/dev/null
	source "$SELF_DIR/shas.env"
	if [[ -d "$SELF_BARE" ]]; then
		return 0
	fi
	mkdir -p "$(dirname "$SELF_BARE")"
	git init -q --bare "$SELF_BARE"
	git -C "$SELF_DIR/repo" push -q "$SELF_BARE" \
		"$N1:refs/heads/develop" "$S1:refs/heads/master" \
		"$N1:refs/tags/nightly-latest" "$S1:refs/tags/stable-26.5" "$S1:refs/tags/beta-26.5-1"
	git -C "$SELF_BARE" symbolic-ref HEAD refs/heads/develop
	publish_self_release nightly-latest nightly-1
	publish_self_release stable-26.5 stable-1
	wait_served "$SELF_URL"
}

# fresh_self_home stops whatever daemon serves the socket and starts BUILD on
# an empty home: a first boot of that release.
fresh_self_home() {
	ensure_daemon
	stop_daemon
	rm -rf "$HOME/.quiver"
	start_daemon "$(self_build "$1")"
}

# self_get NS prints GET /v0/arrow/NS's data. The CLI is never used here: a
# CLI that finds no daemon boots its own, which during a handover would take
# the socket from the successor.
self_get() {
	api GET "$1" >/dev/null
	jq -c .data "$HOME/api.body"
}

# wait_self NS JQ_PREDICATE DESCRIPTION polls self_get until the predicate
# holds, tolerating a daemon that is between processes.
wait_self() {
	local ns=$1 predicate=$2 what=$3 deadline=$((SECONDS + WAIT_SECONDS)) detail=""
	while ((SECONDS < deadline)); do
		if [[ "$(api GET "$ns" 2>/dev/null)" == 200 ]] &&
			detail=$(jq -c .data "$HOME/api.body") && jq -e "$predicate" <<<"$detail" >/dev/null 2>&1; then
			ok "$what (after $((SECONDS - deadline + WAIT_SECONDS))s)"
			return 0
		fi
		sleep 1
	done
	echo "last detail: $detail" >&2
	fail "timed out after ${WAIT_SECONDS}s waiting for: $what"
}

# versions prints the daemon's GET /versions data, empty while nothing answers.
versions() {
	curl -sf --unix-socket "$SOCK" http://quiver/versions 2>/dev/null | jq -c .data || true
}

# self_rows prints every quiver.core row the catalog lists, as ref=resolved
# pairs; library_rows the same for GET /v0/arrow?user_installed=true.
self_rows() {
	curl -sf --unix-socket "$SOCK" "http://quiver/v0/arrow${1:-}" |
		jq -r --arg n "$SELF" '[.data[] | select(.namespace == $n) | .versions[] | "\(.ref)=\(.resolved_ref)"] | join(",")'
}

library_rows() {
	self_rows "?user_installed=true"
}

# runtime_post NS METHOD [BODY] calls POST /v0/runtime/NS/METHOD and prints
# the HTTP status; the response body lands in $HOME/api.body.
runtime_post() {
	local path body=${3:-'{}'}
	path="/v0/runtime/$(jq -rn --arg ns "$1" '$ns | @uri')/$2"
	curl -s -o "$HOME/api.body" -w '%{http_code}' --unix-socket "$SOCK" -X POST \
		-H 'Content-Type: application/json' --data "$body" "http://quiver$path"
}

# self_update NS TAG BUILD_ID runs quiver.core's own update the way a client
# does: the asset URL and checksum of release TAG for this platform, from the
# release's own checksums.txt. It then waits for the handover: the same PID
# answering as build BUILD_ID, its image the downloaded binary.
self_update() {
	local ns=$1 tag=$2 build_id=$3 asset url sum
	asset="quiver-linux-$(container_goarch)"
	url="$SELF_URL/releases/download/$tag/$asset"
	sum=$(curl -sf "$SELF_URL/releases/download/$tag/checksums.txt" | awk -v a="./$asset" '$2 == a { print $1 }')
	[[ -n "$sum" ]] || fail "no checksum for $asset in release $tag"
	expect_eq "POST /v0/runtime/$ns/update ($url)" \
		"$(runtime_post "$ns" update "$(jq -nc --arg u "$url" --arg c "$sum" '{variables: {QUIVER_RELEASE_ASSET_URL: $u, QUIVER_RELEASE_CHECKSUM: $c}}')")" 202

	local deadline=$((SECONDS + WAIT_SECONDS))
	until [[ "$(versions | jq -r '.build_id // empty' 2>/dev/null)" == "$build_id" ]]; do
		kill -0 "$DAEMON_PID" 2>/dev/null || fail "the daemon (pid $DAEMON_PID) exited instead of handing over"
		((SECONDS < deadline)) || fail "the daemon never answered as build $build_id: $(versions)"
		sleep 0.5
	done
	ok "pid $DAEMON_PID now answers as build $build_id"
	local exe
	exe=$(readlink "/proc/$DAEMON_PID/exe")
	case "$exe" in
	/run/rosetta/* | */qemu-*)
		ok "emulated: $exe hides the process image; the handover is proven by the new build id on the same pid"
		;;
	*)
		expect_eq "the process image" "$exe" "$HOME/.quiver/namespaces/$ns/quiver-new"
		;;
	esac
}

phase_e() {
	banner "Phase E — quiver.core@nightly-latest updates itself"
	PHASE_START=$SECONDS
	local ns="$SELF@nightly-latest" detail

	step "serve quiver.core from a git host standing in for github.com"
	self_hosts on
	serve_self
	expect_eq "nightly-latest on $SELF_URL" "$(peeled_commit "$SELF_URL" nightly-latest)" "$N1"

	step "first boot of the nightly-latest build at $N1"
	fresh_self_home nightly-1
	expect_eq "GET /versions" "$(versions | jq -c '{version, build_id}')" '{"version":"nightly-latest","build_id":"170"}'
	wait_self "$ns" ".installed_commit == \"$N1\"" "core registered itself as $ns"
	detail=$(self_get "$ns")
	expect_eq "identity" "$(jq -r .namespace <<<"$detail")" "$ns"
	expect_eq "selector_kind" "$(jq -r .selector_kind <<<"$detail")" channel
	expect_eq "resolved_ref" "$(jq -r .resolved_ref <<<"$detail")" nightly-latest
	expect_eq "user_installed" "$(jq -r .user_installed <<<"$detail")" true
	expect_eq "state" "$(jq -r .state <<<"$detail")" ready
	expect_eq "PATCH /v0/arrow/$ns" "$(api PATCH "$ns")" 200
	detail=$(self_get "$ns")
	expect_eq "available after a check" "$(jq -c '.available // "absent"' <<<"$detail")" '"absent"'
	expect_eq "outdated" "$(jq -r .outdated <<<"$detail")" false
	expect_eq "catalog rows for $SELF" "$(self_rows)" nightly-latest=nightly-latest
	expect_eq "library rows for $SELF" "$(library_rows)" nightly-latest=nightly-latest

	step "the nightly workflow publishes $N2: nightly-latest force-moved, assets replaced"
	git -C "$SELF_DIR/repo" push -q --force "$SELF_BARE" "$N2:refs/heads/develop" "$N2:refs/tags/nightly-latest"
	publish_self_release nightly-latest nightly-2
	expect_eq "nightly-latest on $SELF_URL" "$(peeled_commit "$SELF_URL" nightly-latest)" "$N2"
	expect_eq "PATCH /v0/arrow/$ns" "$(api PATCH "$ns")" 200
	detail=$(self_get "$ns")
	expect_eq "available" "$(jq -c .available <<<"$detail")" "{\"ref\":\"nightly-latest\",\"commit\":\"$N2\"}"
	expect_eq "outdated" "$(jq -r .outdated <<<"$detail")" true
	expect_eq "state (the runtime badge)" "$(jq -r .state <<<"$detail")" outdated
	expect_eq "installed_commit (not yet updated)" "$(jq -r .installed_commit <<<"$detail")" "$N1"

	step "self-update: download nightly-latest through ARROW.md, exec it"
	local pid=$DAEMON_PID
	self_update "$ns" nightly-latest 171
	expect_eq "same pid" "$DAEMON_PID" "$pid"
	wait_self "$ns" ".installed_commit == \"$N2\" and .available == null and .state == \"ready\"" \
		"the successor adopted $N2 with nothing ahead"
	detail=$(self_get "$ns")
	expect_eq "identity unchanged" "$(jq -r .namespace <<<"$detail")" "$ns"
	expect_eq "selector_kind" "$(jq -r .selector_kind <<<"$detail")" channel
	expect_eq "resolved_ref" "$(jq -r .resolved_ref <<<"$detail")" nightly-latest
	expect_eq "user_installed preserved" "$(jq -r .user_installed <<<"$detail")" true
	expect_eq "outdated" "$(jq -r .outdated <<<"$detail")" false
	expect_eq "the update's \${REF}" "$(jq -r .last_return.variables.REF <<<"$detail")" nightly-latest
	expect_eq "catalog rows for $SELF" "$(self_rows)" nightly-latest=nightly-latest
	expect_eq "library rows for $SELF" "$(library_rows)" nightly-latest=nightly-latest
	expect_eq "PATCH /v0/arrow/$ns" "$(api PATCH "$ns")" 200
	expect_eq "available after a fresh check" "$(self_get "$ns" | jq -c '.available // "absent"')" '"absent"'
	expect_eq "update with nothing newer (idempotent no-op)" "$(runtime_post "$ns" update)" 200
	cmp -s "$HOME/.quiver/self/quiver" "$(self_build nightly-2)" || fail "the running build was not promoted to ~/.quiver/self/quiver"
	ok "the running build is promoted to ~/.quiver/self/quiver"

	step "a reboot starts the promoted build: nothing moves"
	stop_daemon
	start_daemon "$HOME/.quiver/self/quiver"
	expect_eq "GET /versions" "$(versions | jq -c '{version, build_id}')" '{"version":"nightly-latest","build_id":"171"}'
	wait_self "$ns" ".installed_commit == \"$N2\" and .state == \"ready\"" "row still at $N2 after a reboot"
	expect_eq "available" "$(self_get "$ns" | jq -c '.available // "absent"')" '"absent"'
	expect_eq "catalog rows for $SELF" "$(self_rows)" nightly-latest=nightly-latest

	stop_daemon
	self_hosts off
	phase_done E "quiver.core@nightly-latest: $N1 -> $N2 by its own update, same pid, same identity, one row"
}

phase_f() {
	banner "Phase F — quiver.core@stable updates itself across a new tag"
	PHASE_START=$SECONDS
	local ns="$SELF@stable" detail

	step "serve quiver.core from a git host standing in for github.com"
	self_hosts on
	serve_self
	expect_eq "stable-26.5 on $SELF_URL" "$(peeled_commit "$SELF_URL" stable-26.5)" "$S1"

	step "first boot of the stable-26.5 build"
	fresh_self_home stable-1
	expect_eq "GET /versions" "$(versions | jq -c '{version, build_id}')" '{"version":"stable-26.5","build_id":"170"}'
	wait_self "$ns" ".installed_commit == \"$S1\"" "core registered itself as $ns"
	detail=$(self_get "$ns")
	expect_eq "selector_kind" "$(jq -r .selector_kind <<<"$detail")" channel
	expect_eq "resolved_ref" "$(jq -r .resolved_ref <<<"$detail")" stable-26.5
	expect_eq "user_installed" "$(jq -r .user_installed <<<"$detail")" true
	expect_eq "PATCH /v0/arrow/$ns" "$(api PATCH "$ns")" 200
	expect_eq "available after a check" "$(self_get "$ns" | jq -c '.available // "absent"')" '"absent"'
	expect_eq "catalog rows for $SELF" "$(self_rows)" stable=stable-26.5

	step "the stable workflow tags stable-26.5.1 at $S2"
	git -C "$SELF_DIR/repo" push -q "$SELF_BARE" "$S2:refs/heads/master" "$S2:refs/tags/stable-26.5.1"
	publish_self_release stable-26.5.1 stable-2
	expect_eq "PATCH /v0/arrow/$ns" "$(api PATCH "$ns")" 200
	detail=$(self_get "$ns")
	expect_eq "available" "$(jq -c .available <<<"$detail")" "{\"ref\":\"stable-26.5.1\",\"commit\":\"$S2\"}"
	expect_eq "state (the runtime badge)" "$(jq -r .state <<<"$detail")" outdated

	step "self-update to stable-26.5.1"
	self_update "$ns" stable-26.5.1 172
	wait_self "$ns" ".installed_commit == \"$S2\" and .available == null and .state == \"ready\"" \
		"the successor adopted stable-26.5.1 with nothing ahead"
	detail=$(self_get "$ns")
	expect_eq "identity unchanged" "$(jq -r .namespace <<<"$detail")" "$ns"
	expect_eq "resolved_ref" "$(jq -r .resolved_ref <<<"$detail")" stable-26.5.1
	expect_eq "user_installed preserved" "$(jq -r .user_installed <<<"$detail")" true
	expect_eq "the update's \${REF}" "$(jq -r .last_return.variables.REF <<<"$detail")" stable-26.5.1
	expect_eq "catalog rows for $SELF" "$(self_rows)" stable=stable-26.5.1
	expect_eq "library rows for $SELF" "$(library_rows)" stable=stable-26.5.1
	expect_eq "PATCH /v0/arrow/$ns" "$(api PATCH "$ns")" 200
	expect_eq "available after a fresh check" "$(self_get "$ns" | jq -c '.available // "absent"')" '"absent"'

	stop_daemon
	self_hosts off
	phase_done F "quiver.core@stable: stable-26.5 -> stable-26.5.1 by its own update, same identity, one row"
}

# ─── main ─────────────────────────────────────────────────────────────────────

banner "quiver e2e on $(uname -m)"
echo "phases: $PHASES   wait deadline: ${WAIT_SECONDS}s   HOME=$HOME   QUIVER_HOME=$QUIVER_HOME"

git_setup
start_githost
step "start the daemon"
start_daemon
quiver version

for phase in $PHASES; do
	case "$phase" in
	A) phase_a ;;
	B) phase_b ;;
	C) phase_c ;;
	D) phase_d ;;
	E) phase_e ;;
	F) phase_f ;;
	*) fail "unknown phase $phase" ;;
	esac
done

banner "SUMMARY ($(uname -m))"
printf '%s\n' "${SUMMARY[@]}"
printf 'checks passed: %s   total: %ss\n' "$CHECKS" "$((SECONDS - SUITE_START))"
if ((EMULATED)); then
	echo "E2E PASS (Phase A in EMULATED mode: this host cannot exec AppImages)"
else
	echo "E2E PASS"
fi
