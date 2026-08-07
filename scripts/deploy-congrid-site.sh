#!/usr/bin/env bash

set -Eeuo pipefail

# This script runs on the congrid-site server. It builds the current working
# tree as congridcoin, installs the binary as root, and restarts systemd.
BUILD_USER="congridcoin"
BUILD_HOME="/home/congridcoin"
GO_BIN="/usr/local/go/bin/go"
SERVICE="congrid-site"
TARGET_BIN="/usr/local/bin/congrid-site"
LOCAL_HEALTH_URL="http://127.0.0.1:8080/"
PUBLIC_HEALTH_URL="https://congrid.net/"
EXPECTED_PUBLISHER_TEXT="publisher=congrid.net"
EXPECTED_WALLET_TEXT="wallet=congrid18cepycc5rv3dpe24n0mmdkdqwaruptvkuuurxf"

usage() {
  cat <<'EOF'
Usage: ./scripts/deploy-congrid-site.sh

Build the current server-side working tree, install /usr/local/bin/congrid-site,
restart congrid-site.service, and verify the publisher badge binding.

The script does not connect to GCP or access Git.
Run it from the server login account; it elevates with sudo and performs Go
test/build commands as the congridcoin user.
EOF
}

log() {
  printf '[deploy-congrid-site] %s\n' "$*"
}

show_failure_diagnostics() {
  systemctl status "$SERVICE" --no-pager -n 30 || true
  journalctl -u "$SERVICE" --no-pager -n 80 || true
}

wait_for_local_health() {
  local attempts=30
  local body
  local i

  for ((i = 1; i <= attempts; i++)); do
    if systemctl is-active --quiet "$SERVICE"; then
      if body="$(curl --fail --silent --show-error --max-time 5 "$LOCAL_HEALTH_URL")" &&
        grep --fixed-strings --quiet "$EXPECTED_PUBLISHER_TEXT" <<<"$body" &&
        grep --fixed-strings --quiet "$EXPECTED_WALLET_TEXT" <<<"$body"; then
        return 0
      fi
    fi
    sleep 1
  done

  return 1
}

case "${1:-}" in
  "") ;;
  -h|--help)
    usage
    exit
    ;;
  *)
    printf 'unknown argument: %s\n' "$1" >&2
    usage >&2
    exit 2
    ;;
esac

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
script_path="$script_dir/$(basename -- "${BASH_SOURCE[0]}")"
repo_root="$(cd -- "$script_dir/.." && pwd -P)"

if [[ $EUID -ne 0 ]]; then
  if ! command -v sudo >/dev/null 2>&1; then
    printf 'sudo is required to install the binary and restart %s\n' "$SERVICE" >&2
    exit 1
  fi
  if ! sudo -n true 2>/dev/null; then
    printf 'passwordless sudo is required; run this script from the server login account\n' >&2
    exit 1
  fi
  log "elevating to root; build commands will run as $BUILD_USER"
  exec sudo -n -- "$script_path"
fi

for command_name in sudo systemctl curl grep install sha256sum awk cp mv mktemp id stat date sleep bash; do
  if ! command -v "$command_name" >/dev/null 2>&1; then
    printf 'required command not found: %s\n' "$command_name" >&2
    exit 1
  fi
done
if ! id "$BUILD_USER" >/dev/null 2>&1; then
  printf 'build user not found: %s\n' "$BUILD_USER" >&2
  exit 1
fi
if [[ ! -x "$GO_BIN" ]]; then
  printf 'Go binary not found or not executable: %s\n' "$GO_BIN" >&2
  exit 1
fi
if [[ ! -f "$repo_root/go.mod" || ! -d "$repo_root/cmd/congrid-site" ]]; then
  printf 'congrid-site repository not found at inferred path: %s\n' "$repo_root" >&2
  exit 1
fi
if [[ "$(stat -c %U "$repo_root")" != "$BUILD_USER" ]]; then
  printf 'repository %s must be owned by %s\n' "$repo_root" "$BUILD_USER" >&2
  exit 1
fi
if ! systemctl cat "$SERVICE" >/dev/null 2>&1; then
  printf 'systemd service not found: %s\n' "$SERVICE" >&2
  exit 1
fi
if [[ "$(systemctl show "$SERVICE" --property=ExecStart --value)" != *"$TARGET_BIN"* ]]; then
  printf '%s does not execute expected binary %s\n' "$SERVICE" "$TARGET_BIN" >&2
  exit 1
fi
if ! systemctl is-active --quiet "$SERVICE"; then
  printf 'refusing to deploy while %s is not active\n' "$SERVICE" >&2
  show_failure_diagnostics
  exit 1
fi

run_as_build_user() {
  sudo -u "$BUILD_USER" -- env \
    HOME="$BUILD_HOME" \
    PATH="/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin" \
    GOCACHE="$BUILD_HOME/.cache/go-build" \
    "$@"
}

build_bin="$(run_as_build_user mktemp "$repo_root/.congrid-site.build.XXXXXX")"
staged_bin="${TARGET_BIN}.new"
backup_bin="${TARGET_BIN}.backup.$(date -u +%Y%m%dT%H%M%SZ)"
deployed=false

cleanup() {
  rm -f -- "$build_bin" "$staged_bin"
}
trap cleanup EXIT

log "using current working tree: $repo_root"
log "running congrid-site tests as $BUILD_USER"
run_as_build_user bash -c \
  'cd "$1" && "$2" test ./cmd/congrid-site' \
  bash "$repo_root" "$GO_BIN"

log "building congrid-site as $BUILD_USER with $($GO_BIN version)"
run_as_build_user bash -c \
  'cd "$1" && "$2" build -trimpath -o "$3" ./cmd/congrid-site' \
  bash "$repo_root" "$GO_BIN" "$build_bin"

log "built sha256: $(sha256sum "$build_bin" | awk '{print $1}')"
if [[ -f "$TARGET_BIN" ]]; then
  cp --archive -- "$TARGET_BIN" "$backup_bin"
  log "saved rollback binary: $backup_bin"
fi

install -o root -g root -m 0755 -- "$build_bin" "$staged_bin"
mv --force -- "$staged_bin" "$TARGET_BIN"
deployed=true

log "restarting $SERVICE"
if ! systemctl restart "$SERVICE" || ! wait_for_local_health; then
  printf 'deployment health check failed; restoring previous binary\n' >&2
  show_failure_diagnostics
  if [[ -f "$backup_bin" ]]; then
    cp --archive -- "$backup_bin" "$TARGET_BIN"
    systemctl restart "$SERVICE" || true
    if systemctl is-active --quiet "$SERVICE"; then
      log "rollback completed"
    else
      printf 'rollback restart failed; manual intervention required\n' >&2
      show_failure_diagnostics
    fi
  elif [[ "$deployed" == true ]]; then
    printf 'no previous binary was available for rollback\n' >&2
  fi
  exit 1
fi

log "local health check passed and publisher badge binding is present"
if curl --fail --silent --show-error --max-time 10 \
  --output /dev/null "$PUBLIC_HEALTH_URL"; then
  log "public health check passed: $PUBLIC_HEALTH_URL"
else
  log "warning: public health check failed; local service remains healthy"
fi

systemctl status "$SERVICE" --no-pager -n 8
log "deployment complete"
