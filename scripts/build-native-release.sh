#!/usr/bin/env bash
set -euo pipefail

export LC_ALL=C
export LANG=C

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd -P)"

TARGET_OS=""
TARGET_ARCH=""
OUTPUT_DIR=""
PRE_UPGRADE_REF="${CONGRID_PRE_UPGRADE_REF:-ef331816c0c213a145e26f7719bc4fb395e03c0a}"
PRE_UPGRADE_VERSION="pre-drand-strict-v2-ef331816"

usage() {
  cat <<'USAGE'
Build a native Linux or macOS bundle consumed by
cmd/congrid-site/downloads/install.sh.

Usage:
  scripts/build-native-release.sh [linux|darwin] [amd64|arm64] [output-directory]

When the OS or architecture is omitted, the host value is used. The command
produces congrid-native-<os>-<arch>.tar.gz and its .sha256 file.
USAGE
}

die() {
  printf '[congrid-release] error: %s\n' "$*" >&2
  exit 1
}

if [ "${1:-}" = "-h" ] || [ "${1:-}" = "--help" ]; then
  usage
  exit 0
fi

case "${1:-}" in
  linux|darwin)
    TARGET_OS="$1"
    shift
    ;;
esac

case "${1:-}" in
  amd64|arm64)
    TARGET_ARCH="$1"
    shift
    ;;
esac

OUTPUT_DIR="${1:-$ROOT_DIR/dist}"
[ "$#" -le 1 ] || die "too many arguments"

if [ -z "$TARGET_OS" ]; then
  case "$(uname -s)" in
    Linux) TARGET_OS="linux" ;;
    Darwin) TARGET_OS="darwin" ;;
    *) die "cannot infer release OS from $(uname -s)" ;;
  esac
fi

if [ -z "$TARGET_ARCH" ]; then
  case "$(uname -m)" in
    x86_64|amd64) TARGET_ARCH="amd64" ;;
    aarch64|arm64) TARGET_ARCH="arm64" ;;
    *) die "cannot infer release architecture from $(uname -m)" ;;
  esac
fi

case "$TARGET_ARCH" in
  amd64|arm64) ;;
  *) die "architecture must be amd64 or arm64" ;;
esac
case "$TARGET_OS" in
  linux|darwin) ;;
  *) die "OS must be linux or darwin" ;;
esac

command -v go >/dev/null 2>&1 || die "Go is required"
command -v tar >/dev/null 2>&1 || die "tar is required"
command -v git >/dev/null 2>&1 || die "Git is required"

WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/congrid-native-release.XXXXXXXX")"
trap 'rm -rf -- "$WORK_DIR"' EXIT

STAGE_DIR="$WORK_DIR/congrid-native"
ARCHIVE_NAME="congrid-native-${TARGET_OS}-${TARGET_ARCH}.tar.gz"
ARCHIVE_PATH="$OUTPUT_DIR/$ARCHIVE_NAME"

mkdir -p "$STAGE_DIR/bin" "$STAGE_DIR/chromad" "$OUTPUT_DIR"

build_binary() {
  local output_name="$1"
  local package_path="$2"
  printf '[congrid-release] building %s for %s/%s\n' "$output_name" "$TARGET_OS" "$TARGET_ARCH" >&2
  (
    cd "$ROOT_DIR"
    CGO_ENABLED=0 GOOS="$TARGET_OS" GOARCH="$TARGET_ARCH" \
      go build -trimpath -o "$STAGE_DIR/bin/$output_name" "$package_path"
  )
}

build_binary content-grid-d ./cmd/content-grid-d
build_binary verifierd ./offchain/verifierd
build_binary indexerd ./offchain/indexerd

PRE_UPGRADE_COMMIT="$(
  git -C "$ROOT_DIR" rev-parse --verify "$PRE_UPGRADE_REF^{commit}" 2>/dev/null
)" || die "cannot resolve pre-upgrade source ref: $PRE_UPGRADE_REF"
PRE_UPGRADE_SOURCE="$WORK_DIR/pre-upgrade-source"
mkdir -p "$PRE_UPGRADE_SOURCE"
git -C "$ROOT_DIR" archive "$PRE_UPGRADE_COMMIT" |
  tar -x -C "$PRE_UPGRADE_SOURCE"
printf '[congrid-release] building content-grid-d-pre-upgrade from %s for %s/%s\n' \
  "$PRE_UPGRADE_COMMIT" "$TARGET_OS" "$TARGET_ARCH" >&2
(
  cd "$PRE_UPGRADE_SOURCE"
  CGO_ENABLED=0 GOOS="$TARGET_OS" GOARCH="$TARGET_ARCH" \
    go build -trimpath \
      -ldflags "-X github.com/cosmos/cosmos-sdk/version.Version=$PRE_UPGRADE_VERSION" \
      -o "$STAGE_DIR/bin/content-grid-d-pre-upgrade" \
      ./cmd/content-grid-d
)

install -m 0644 "$ROOT_DIR/offchain/chromad/server.py" "$STAGE_DIR/chromad/server.py"
install -m 0644 "$ROOT_DIR/offchain/chromad/requirements.txt" "$STAGE_DIR/chromad/requirements.txt"

cat >"$STAGE_DIR/BUILD-INFO" <<EOF
source_commit=$(git -C "$ROOT_DIR" rev-parse HEAD 2>/dev/null || printf unknown)
pre_upgrade_source_commit=$PRE_UPGRADE_COMMIT
pre_upgrade_plan=drand-strict-v2
pre_upgrade_height=13000
target=$TARGET_OS/$TARGET_ARCH
built_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
EOF

tar -czf "$ARCHIVE_PATH.tmp" -C "$WORK_DIR" congrid-native
mv "$ARCHIVE_PATH.tmp" "$ARCHIVE_PATH"

if command -v sha256sum >/dev/null 2>&1; then
  checksum="$(sha256sum "$ARCHIVE_PATH" | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
  checksum="$(shasum -a 256 "$ARCHIVE_PATH" | awk '{print $1}')"
else
  die "sha256sum or shasum is required"
fi
printf '%s  %s\n' "$checksum" "$ARCHIVE_NAME" >"$ARCHIVE_PATH.sha256"

printf '[congrid-release] wrote %s\n' "$ARCHIVE_PATH" >&2
printf '[congrid-release] wrote %s.sha256\n' "$ARCHIVE_PATH" >&2
