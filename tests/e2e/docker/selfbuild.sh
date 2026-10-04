#!/bin/sh
# Builds the quiver.core releases the self-update phases (E, F) publish, run
# inside the Dockerfile's builder stage from the repository root.
#
# A release is a commit of a stand-in quiver.core repository carrying the real
# ARROW.md, and a daemon stamped the way .github/workflows/build-assets.yml
# stamps one: the LDFLAGS line below is that workflow's, verbatim. Only `-a`
# is left out of the go build, which forces a rebuild of every package and
# changes nothing the binary reports.
#
#   /out/self/repo              the stand-in repository: develop N1-N2-N3, master S1-S2
#   /out/self/shas.env          N1 N2 N3 S1 S2 as shell assignments
#   /out/self/<build>/          quiver-linux-<arch> + checksums.txt, per build
set -eu

: "${TARGETOS:?}" "${TARGETARCH:?}"

OUT=/out/self
REPO=$OUT/repo
mkdir -p "$OUT"

export GIT_AUTHOR_NAME="quiver e2e" GIT_AUTHOR_EMAIL=e2e@quiver.invalid
export GIT_COMMITTER_NAME="quiver e2e" GIT_COMMITTER_EMAIL=e2e@quiver.invalid

git init -q -b develop "$REPO"
cp ARROW.md "$REPO/ARROW.md"

# commit MESSAGE commits the next revision of BUILD, printing its commit.
commit() {
	echo "$1" >"$REPO/BUILD"
	git -C "$REPO" add ARROW.md BUILD
	git -C "$REPO" commit -qm "$1"
	git -C "$REPO" rev-parse HEAD
}

N1=$(commit "nightly 1")
git -C "$REPO" branch master
N2=$(commit "nightly 2")
N3=$(commit "nightly 3")
git -C "$REPO" checkout -q master
S1=$(commit "stable 26.5")
S2=$(commit "stable 26.5.1")
git -C "$REPO" checkout -q develop

printf 'N1=%s\nN2=%s\nN3=%s\nS1=%s\nS2=%s\n' "$N1" "$N2" "$N3" "$S1" "$S2" >"$OUT/shas.env"

# build NAME VERSION COMMIT CHANNEL BUILD_ID
build() {
	VERSION=$2 COMMIT=$3 CHANNEL=$4 BUILD_ID=$5
	LDFLAGS="-X main.version=${VERSION} -X main.commit=${COMMIT} -X main.channel=${CHANNEL} -X main.builtAt=$(date -u +%Y-%m-%dT%H:%M:%SZ) -X main.buildID=${BUILD_ID}"
	mkdir -p "$OUT/$1"
	CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -installsuffix cgo -ldflags "$LDFLAGS" \
		-o "$OUT/$1/quiver-$TARGETOS-$TARGETARCH" ./cmd/quiver
	(cd "$OUT/$1" && sha256sum ./* >checksums.txt)
}

build nightly-1 nightly-latest "$N1" nightly-latest 170
build nightly-2 nightly-latest "$N2" nightly-latest 171
build nightly-3 nightly-latest "$N3" nightly-latest 172
build stable-1 stable-26.5 "$S1" stable 170
build stable-2 stable-26.5.1 "$S2" stable 172
