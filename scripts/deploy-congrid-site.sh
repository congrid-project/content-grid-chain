#!/usr/bin/env bash

set -Eeuo pipefail

# This script runs on the congrid-site server. It builds the current working
# tree as congridcoin, installs the binary as root, and restarts systemd.
BUILD_USER="congridcoin"
BUILD_HOME="/home/congridcoin"
GO_BIN="/usr/local/go/bin/go"
SERVICE="congrid-site"
TARGET_BIN="/usr/local/bin/congrid-site"
CMS_PASSWORD_FILE="/etc/congrid-site/cms-password"
CMS_STATE_DIRECTORY="congrid-site/cms"
CMS_DB="/var/lib/${CMS_STATE_DIRECTORY}/cms.db"
CMS_DROP_IN_DIR="/etc/systemd/system/${SERVICE}.service.d"
CMS_DROP_IN="${CMS_DROP_IN_DIR}/50-cms.conf"
CMS_RUNTIME_PASSWORD_FILE="/run/credentials/${SERVICE}.service/cms-password"
LOCAL_HEALTH_URL="http://127.0.0.1:8080/"
LOCAL_BLOG_URL="http://127.0.0.1:8080/blog?lang=en"
LOCAL_CMS_URL="http://127.0.0.1:8080/cms/login?lang=en"
PUBLIC_HEALTH_URL="https://congrid.net/"
PUBLIC_BLOG_URL="https://congrid.net/blog"
PUBLIC_CMS_URL="https://congrid.net/cms/login"
EXPECTED_PUBLISHER_TEXT="publisher=congrid.net"
EXPECTED_WALLET_TEXT="wallet=congrid18cepycc5rv3dpe24n0mmdkdqwaruptvkuuurxf"

usage() {
  cat <<'EOF'
Usage: ./scripts/deploy-congrid-site.sh

Build the current server-side working tree, install /usr/local/bin/congrid-site,
configure the blog/CMS, restart congrid-site.service, and verify the homepage,
blog and CMS sign-in page.

Requires systemd 247+ and an existing /etc/congrid-site/cms-password file.
The initial CMS username is admin. SQLite is persisted at
/var/lib/congrid-site/cms/cms.db. Existing CMS accounts are not reset.
The password is passed through systemd credentials, never printed or placed
in the command line or unit environment. Existing ExecStart arguments are kept.

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
  local blog_body
  local cms_body
  local i

  for ((i = 1; i <= attempts; i++)); do
    if systemctl is-active --quiet "$SERVICE"; then
      if body="$(curl --fail --silent --show-error --max-time 5 "$LOCAL_HEALTH_URL")" &&
        grep --fixed-strings --quiet "$EXPECTED_PUBLISHER_TEXT" <<<"$body" &&
        grep --fixed-strings --quiet "$EXPECTED_WALLET_TEXT" <<<"$body" &&
        blog_body="$(curl --fail --silent --show-error --max-time 5 "$LOCAL_BLOG_URL")" &&
        grep --fixed-strings --quiet 'class="hero blog-hero"' <<<"$blog_body" &&
        cms_body="$(curl --fail --silent --show-error --max-time 5 "$LOCAL_CMS_URL")" &&
        grep --fixed-strings --quiet 'action="/cms/login"' <<<"$cms_body" &&
        ! grep --fixed-strings --quiet 'CMS administrator has not been configured.' <<<"$cms_body"; then
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

for command_name in sudo systemctl journalctl curl grep install sha256sum awk cp mv mktemp id stat date sleep bash chmod rm cat; do
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
service_exec_start="$(systemctl show "$SERVICE" --property=ExecStart --value)"
if [[ "$service_exec_start" != *"$TARGET_BIN"* ]]; then
  printf '%s does not execute expected binary %s\n' "$SERVICE" "$TARGET_BIN" >&2
  exit 1
fi
if [[ "$service_exec_start" == *"--cms-db"* || "$service_exec_start" == *"--cms-admin-password-file"* ]]; then
  printf '%s has CMS path flags in ExecStart; remove them so the deployment CMS settings can take effect\n' "$SERVICE" >&2
  exit 1
fi
systemd_version="$(systemctl --version | awk 'NR == 1 {print $2}')"
if [[ ! "$systemd_version" =~ ^[0-9]+$ ]] || ((systemd_version < 247)); then
  printf 'systemd 247+ is required to pass the CMS password as a service credential\n' >&2
  exit 1
fi
if [[ ! -f "$CMS_PASSWORD_FILE" || -L "$CMS_PASSWORD_FILE" || ! -s "$CMS_PASSWORD_FILE" ]]; then
  printf 'CMS password file must be a non-empty regular file: %s\n' "$CMS_PASSWORD_FILE" >&2
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
backup_drop_in="${build_bin}.cms-backup"
staged_drop_in="${build_bin}.cms-conf"
deployed=false
cms_config_changed=false
had_cms_drop_in=false
deployment_succeeded=false

cleanup() {
  rm -f -- "$build_bin" "$staged_bin" "$staged_drop_in" "$backup_drop_in"
}
trap cleanup EXIT

rollback_on_error() {
  local status=$?
  local rollback_ok=true
  trap - ERR
  if [[ "$deployment_succeeded" == false && ( "$deployed" == true || "$cms_config_changed" == true ) ]]; then
    printf 'deployment failed; restoring previous binary and CMS service configuration\n' >&2
    show_failure_diagnostics
    if [[ "$deployed" == true && -f "$backup_bin" ]]; then
      # An active executable cannot be overwritten on Linux; replace its inode.
      if ! install -o root -g root -m 0755 -- "$backup_bin" "$staged_bin" ||
        ! mv --force -- "$staged_bin" "$TARGET_BIN"; then
        rollback_ok=false
      fi
    elif [[ "$deployed" == true ]]; then
      printf 'no previous binary was available for rollback\n' >&2
      rollback_ok=false
    fi
    if [[ "$cms_config_changed" == true ]]; then
      if [[ "$had_cms_drop_in" == true ]]; then
        cp --archive -- "$backup_drop_in" "$CMS_DROP_IN" || rollback_ok=false
      else
        rm -f -- "$CMS_DROP_IN" || rollback_ok=false
      fi
    fi
    systemctl daemon-reload || rollback_ok=false
    if systemctl restart "$SERVICE" && systemctl is-active --quiet "$SERVICE" && [[ "$rollback_ok" == true ]]; then
      log "rollback completed"
    else
      printf 'rollback failed; manual intervention required\n' >&2
      show_failure_diagnostics
    fi
  fi
  exit "$status"
}
trap rollback_on_error ERR

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

cat >"$staged_drop_in" <<EOF
# Managed by scripts/deploy-congrid-site.sh.
[Service]
StateDirectory=${CMS_STATE_DIRECTORY}
LoadCredential=cms-password:${CMS_PASSWORD_FILE}
Environment="CONGRID_CMS_DB=${CMS_DB}"
Environment="CONGRID_CMS_ADMIN_USER=admin"
Environment="CONGRID_CMS_ADMIN_PASSWORD_FILE=${CMS_RUNTIME_PASSWORD_FILE}"
EOF
if [[ -f "$CMS_DROP_IN" ]]; then
  cp --archive -- "$CMS_DROP_IN" "$backup_drop_in"
  had_cms_drop_in=true
fi
install -d -o root -g root -m 0755 -- "$CMS_DROP_IN_DIR"
chmod 0600 -- "$CMS_PASSWORD_FILE"
cms_config_changed=true
install -o root -g root -m 0644 -- "$staged_drop_in" "$CMS_DROP_IN"
log "configured CMS database: $CMS_DB"
log "configured CMS password credential from $CMS_PASSWORD_FILE"
systemctl daemon-reload

mv --force -- "$staged_bin" "$TARGET_BIN"
deployed=true

log "restarting $SERVICE"
systemctl restart "$SERVICE"
wait_for_local_health
deployment_succeeded=true

log "local homepage, blog and CMS sign-in checks passed"
for health_url in "$PUBLIC_HEALTH_URL" "$PUBLIC_BLOG_URL" "$PUBLIC_CMS_URL"; do
  if curl --fail --silent --show-error --max-time 10 --output /dev/null "$health_url"; then
    log "public health check passed: $health_url"
  else
    log "warning: public health check failed; local service remains healthy: $health_url"
  fi
done

systemctl status "$SERVICE" --no-pager -n 8
log "deployment complete"
log "blog: $PUBLIC_BLOG_URL; CMS sign-in: $PUBLIC_CMS_URL (initial username: admin)"
