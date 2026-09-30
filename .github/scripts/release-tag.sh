#!/usr/bin/env bash
# release-tag.sh <prerelease|stable> <branch>
#
# Prints the tag quiver.core's release workflows publish for <branch>
# (beta/<series> or hotfix/<name>), given the repository's tags on stdin.
# A series is a calendar version (26.5) or a date (2026-09-27):
#   beta      beta-<series>, then beta-<series>-1, -2, ...
#   stable    stable-<series>, then stable-26.5.1 / stable-2026-09-27.1, ...
#   hotfix    hotfix-<next stable patch>, rebuilt as hotfix-<patch>-1, ...
# The latest stable is picked the way quiver's channel ranking orders it: a
# date ranks as YY.MM.DD.<patch>, among the YY.M calendar versions. A tag that
# would not outrank its channel's latest release is refused: quiver would
# never offer it.
set -euo pipefail

DATE_RE='^([0-9]{4})-([0-9]{2})-([0-9]{2})(\.([0-9]+))?$'
CALENDAR_RE='^[0-9]+\.[0-9]+(\.[0-9]+)?$'

TAGS=()
while IFS= read -r line; do
  [ -n "$line" ] && TAGS+=("$line")
done

fail() {
  echo "release-tag: $*" >&2
  exit 1
}

is_version() {
  [[ $1 =~ $DATE_RE || $1 =~ $CALENDAR_RE ]]
}

# rank_key prints a version as four numbers, the order quiver ranks them in.
# Every component is read in base 10: "09" is nine, never an octal error.
rank_key() {
  if [[ $1 =~ $DATE_RE ]]; then
    printf '%d.%d.%d.%d' "$((10#${BASH_REMATCH[1]} - 2000))" "$((10#${BASH_REMATCH[2]}))" \
      "$((10#${BASH_REMATCH[3]}))" "$((10#${BASH_REMATCH[5]:-0}))"
    return
  fi
  local IFS=.
  # shellcheck disable=SC2206 # splitting on dots is the point
  local parts=($1)
  printf '%d.%d.%d.0' "$((10#${parts[0]}))" "$((10#${parts[1]}))" "$((10#${parts[2]:-0}))"
}

# latest_stable prints the highest-ranked stable tag whose version matches
# the filter pattern (a bash regex over the version), or nothing.
latest_stable() {
  local filter=$1 tag version
  for tag in "${TAGS[@]}"; do
    version=${tag#stable-}
    [ "$version" != "$tag" ] || continue
    is_version "$version" || continue
    [[ $version =~ $filter ]] || continue
    echo "$(rank_key "$version") $tag"
  done | sort -V -k1,1 | tail -1 | cut -d' ' -f2
}

# series_of prints the series a version belongs to: its date, or its
# first two calendar components.
series_of() {
  if [[ $1 =~ $DATE_RE ]]; then
    echo "${BASH_REMATCH[1]}-${BASH_REMATCH[2]}-${BASH_REMATCH[3]}"
    return
  fi
  echo "$1" | cut -d'.' -f1,2
}

# next_patch prints the version after the stable version given: a date
# counts .1, .2, ...; a calendar version bumps its third component.
next_patch() {
  if [[ $1 =~ $DATE_RE ]]; then
    echo "${BASH_REMATCH[1]}-${BASH_REMATCH[2]}-${BASH_REMATCH[3]}.$((10#${BASH_REMATCH[5]:-0} + 1))"
    return
  fi
  local base patch
  base=$(echo "$1" | cut -d'.' -f1,2)
  patch=$(echo "$1" | cut -s -d'.' -f3)
  echo "${base}.$((10#${patch:-0} + 1))"
}

# rebuild prints name, or name-N when name was already published N times.
rebuild() {
  local name=$1 count=0 tag
  for tag in "${TAGS[@]}"; do
    if [ "$tag" = "$name" ] || [[ $tag =~ ^${name//./\\.}-[0-9]+$ ]]; then
      count=$((count + 1))
    fi
  done
  if [ "$count" -eq 0 ]; then
    echo "$name"
  else
    echo "${name}-${count}"
  fi
}

beta_series() {
  local series=${1#beta/}
  [[ $series =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ || $series =~ $CALENDAR_RE ]] ||
    fail "beta branch $1 names no series: use beta/<YY.M> or beta/<YYYY-MM-DD>"
  echo "$series"
}

latest_stable_version() {
  local latest
  latest=$(latest_stable '.')
  [ -n "$latest" ] || fail "no stable baseline found; a hotfix requires an existing stable release"
  echo "${latest#stable-}"
}

# stable_for prints the stable tag a release of the version's series gets.
stable_for() {
  local series latest
  series=$(series_of "$1")
  latest=$(latest_stable "^${series//./\\.}(\\.[0-9]+)?$")
  if [ -z "$latest" ]; then
    echo "stable-${series}"
  else
    echo "stable-$(next_patch "${latest#stable-}")"
  fi
}

# tag_key prints a published tag's rank within its family (stable-, beta-,
# hotfix-): its version's rank key, then its rebuild number.
tag_key() {
  local version=${1#*-} rebuild=0 base
  if ! is_version "$version" && [[ $version =~ ^(.+)-([0-9]+)$ ]]; then
    base=${BASH_REMATCH[1]}
    rebuild=${BASH_REMATCH[2]}
    is_version "$base" || return 1
    version=$base
  fi
  is_version "$version" || return 1
  printf '%s.%d' "$(rank_key "$version")" "$((10#$rebuild))"
}

latest_key() {
  local family=$1 tag key
  for tag in "${TAGS[@]}"; do
    [[ $tag == "$family"-* ]] || continue
    key=$(tag_key "$tag") || continue
    echo "$key"
  done | sort -V | tail -1
}

# require_above refuses a tag that does not outrank every published tag of
# its family: quiver offers a channel's highest-ranked member, so a lower one
# would never be offered, while GitHub would still mark it the latest release.
require_above() {
  local tag=$1 family=$2 key latest
  key=$(tag_key "$tag")
  latest=$(latest_key "$family")
  [ -z "$latest" ] && return 0
  if [ "$key" = "$latest" ] || [ "$(printf '%s\n%s\n' "$latest" "$key" | sort -V | tail -1)" != "$key" ]; then
    fail "$tag does not outrank the latest $family release; quiver would never offer it"
  fi
}

stable_tag() {
  local version tag
  case $1 in
    beta/*) version=$(beta_series "$1") ;;
    hotfix/*) version=$(latest_stable_version) ;;
    *) fail "unrecognized branch $1" ;;
  esac
  tag=$(stable_for "$version")
  require_above "$tag" stable
  echo "$tag"
}

prerelease_tag() {
  local version tag
  case $1 in
    beta/*)
      version=$(beta_series "$1")
      tag=$(rebuild "beta-${version}")
      require_above "$tag" beta
      require_above "$(stable_for "$version")" stable
      ;;
    hotfix/*)
      version=$(latest_stable_version)
      tag=$(rebuild "hotfix-$(next_patch "$version")")
      require_above "$tag" hotfix
      ;;
    *) fail "unrecognized branch $1" ;;
  esac
  echo "$tag"
}

case ${1:-} in
  stable) stable_tag "${2:-}" ;;
  prerelease) prerelease_tag "${2:-}" ;;
  *) fail "usage: release-tag.sh <prerelease|stable> <branch>" ;;
esac
