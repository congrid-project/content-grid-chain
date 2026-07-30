#!/usr/bin/env bash
set -Eeuo pipefail

umask 027
export LC_ALL=C
export LANG=C

INSTALLER_VERSION="1.2.1"

case "$(uname -s)" in
  Linux)
    HOST_OS="linux"
    SERVICE_USER="congrid"
    SERVICE_GROUP="congrid"
    SERVICE_LOGIN_HOME="/var/lib/congrid"
    CONGRID_HOME_DIR="/var/lib/congrid"
    CONFIG_DIR="/etc/congrid"
    CHROMAD_DIR="/opt/congrid/chromad"
    BIN_DIR="/usr/local/bin"
    LIBEXEC_DIR="/usr/local/libexec/congrid"
    SYSTEMD_DIR="/etc/systemd/system"
    LAUNCHD_DIR=""
    ;;
  Darwin)
    HOST_OS="darwin"
    if [ "$(id -u)" -eq 0 ]; then
      printf '[congrid-install] ERROR: on macOS, run the installer as your normal login user without sudo\n' >&2
      exit 1
    fi
    SERVICE_USER="$(id -un)"
    SERVICE_GROUP="$(id -gn)"
    SERVICE_LOGIN_HOME="${HOME:?HOME is required on macOS}"
    CONGRID_HOME_DIR="${CONGRID_HOME:-$HOME/.content-grid}"
    CONFIG_DIR="${CONGRID_CONFIG_DIR:-${XDG_CONFIG_HOME:-$HOME/.config}/congrid}"
    CHROMAD_DIR="${CONGRID_CHROMAD_DIR:-${XDG_DATA_HOME:-$HOME/.local/share}/congrid/chromad}"
    BIN_DIR="${CONGRID_BIN_DIR:-$HOME/.local/bin}"
    LIBEXEC_DIR="${CONGRID_LIBEXEC_DIR:-${XDG_DATA_HOME:-$HOME/.local/share}/congrid/libexec}"
    SYSTEMD_DIR=""
    LAUNCHD_DIR="${CONGRID_LAUNCHD_DIR:-$HOME/Library/LaunchAgents}"
    if [ -x /opt/homebrew/bin/brew ]; then
      PATH="/opt/homebrew/bin:/opt/homebrew/sbin:$PATH"
    elif [ -x /usr/local/bin/brew ]; then
      PATH="/usr/local/bin:$PATH"
    fi
    export PATH
    ;;
  *)
    printf '[congrid-install] ERROR: unsupported operating system: %s\n' "$(uname -s)" >&2
    exit 1
    ;;
esac

DOWNLOAD_BASE_URL="${CONGRID_DOWNLOAD_BASE_URL:-https://congrid.net/downloads}"
BUNDLE_URL="${CONGRID_BUNDLE_URL:-}"
BUNDLE_SHA256="${CONGRID_BUNDLE_SHA256:-}"
SEEDS_URL="${CONGRID_SEEDS_URL:-}"
SEEDS_SHA256="${CONGRID_SEEDS_SHA256:-}"
NON_INTERACTIVE="${CONGRID_NON_INTERACTIVE:-false}"
START_SERVICES="${CONGRID_START_SERVICES:-true}"

ROOT_CMD=()
TMP_WORK=""
TTY_ECHO_DISABLED=false
SENSITIVE_MNEMONIC_PATH=""

log() {
  printf '[congrid-install] %s\n' "$*" >&2
}

warn() {
  printf '[congrid-install] WARNING: %s\n' "$*" >&2
}

die() {
  printf '[congrid-install] ERROR: %s\n' "$*" >&2
  exit 1
}

cleanup() {
  local status=$?
  if [ "$TTY_ECHO_DISABLED" = "true" ] && [ -e /dev/tty ]; then
    stty echo </dev/tty >/dev/null 2>&1 || true
    printf '\n' >/dev/tty 2>/dev/null || true
  fi
  if [ -n "$SENSITIVE_MNEMONIC_PATH" ] && declare -F run_root >/dev/null 2>&1; then
    run_root rm -f -- "$SENSITIVE_MNEMONIC_PATH" >/dev/null 2>&1 || true
  fi
  if [ -n "$TMP_WORK" ] && [ -d "$TMP_WORK" ]; then
    rm -rf -- "$TMP_WORK"
  fi
  unset VERIFIER_PASSPHRASE VERIFIER_MNEMONIC
  unset CONGRID_VERIFIER_KEYRING_PASSPHRASE CONGRID_VERIFIER_MNEMONIC
  exit "$status"
}

on_error() {
  local line="$1"
  local status="$2"
  log "installation stopped at line $line (exit $status)"
}

trap 'on_error "$LINENO" "$?"' ERR
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

usage() {
  cat <<'USAGE'
Install Content Grid Chain's native operator stack on Linux or macOS:
  content-grid-d, verifierd, indexerd, and chromad.

Recommended:
  curl -fsSL https://congrid.net/downloads/install.sh | bash

Options (when piping, pass options after "bash -s --"):
  --download-base-url URL   Release directory (default: https://congrid.net/downloads)
  --non-interactive         Read all settings from CONGRID_* environment variables
  --no-start                Install and configure without starting services
  -h, --help                Show this help
  --version                 Show installer version

Important non-interactive variables:
  CONGRID_MONIKER
  CONGRID_CHAIN_ID                     Override default: congrid-main
  CONGRID_GENESIS_URL                  Override default: downloads/genesis.json
  CONGRID_SEEDS_URL                    Seed-list URL (default: downloads/seeds.txt)
  CONGRID_SEEDS_SHA256                 Expected seed-list SHA-256 override
  CONGRID_P2P_SEEDS                    Full seed-list override; skips download
  CONGRID_PERSISTENT_PEERS             Alternative bootstrap peer list
  CONGRID_VERIFIER_KEY_NAME
  CONGRID_VERIFIER_KEY_ACTION          create or recover
  CONGRID_VERIFIER_MNEMONIC            Required when action=recover
  CONGRID_VERIFIER_KEYRING_PASSPHRASE

Artifact overrides:
  CONGRID_BUNDLE_URL                   Full native bundle URL
  CONGRID_BUNDLE_SHA256                Expected lowercase/uppercase SHA-256
  CONGRID_DOWNLOAD_BASE_URL            Base URL used when BUNDLE_URL is unset

Advanced operational override:
  CONGRID_DRAND_DELIVERY_DISABLED      true disables this verifier's drand relay

The installer supports Linux and macOS on amd64 and arm64. Linux services use
systemd; macOS services use per-user launchd LaunchAgents. It never uses Docker,
Podman, or another container runtime.
USAGE
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --download-base-url)
      [ "$#" -ge 2 ] || die "--download-base-url requires a URL"
      DOWNLOAD_BASE_URL="$2"
      shift 2
      ;;
    --non-interactive)
      NON_INTERACTIVE=true
      shift
      ;;
    --no-start)
      START_SERVICES=false
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    --version)
      printf '%s\n' "$INSTALLER_VERSION"
      exit 0
      ;;
    *)
      die "unknown argument: $1"
      ;;
  esac
done

case "$NON_INTERACTIVE" in
  true|false) ;;
  *) die "CONGRID_NON_INTERACTIVE must be true or false" ;;
esac
case "$START_SERVICES" in
  true|false) ;;
  *) die "CONGRID_START_SERVICES must be true or false" ;;
esac

DOWNLOAD_BASE_URL="${DOWNLOAD_BASE_URL%/}"
if [ -z "$SEEDS_URL" ]; then
  SEEDS_URL="$DOWNLOAD_BASE_URL/seeds.txt"
fi

if [ "$NON_INTERACTIVE" != "true" ] && [ ! -r /dev/tty ]; then
  die "interactive installation needs a terminal; use --non-interactive for automation"
fi

case "$(uname -m)" in
  x86_64|amd64)
    RELEASE_ARCH="amd64"
    ;;
  aarch64|arm64)
    RELEASE_ARCH="arm64"
    ;;
  *)
    die "unsupported CPU architecture: $(uname -m) (supported: amd64, arm64)"
    ;;
esac

TMP_WORK="$(mktemp -d "${TMPDIR:-/tmp}/congrid-install.XXXXXXXX")"
chmod 700 "$TMP_WORK"

command_exists() {
  command -v "$1" >/dev/null 2>&1
}

file_sha256() {
  local path="$1"
  if command_exists sha256sum; then
    sha256sum "$path" | awk '{print $1}'
  elif command_exists shasum; then
    shasum -a 256 "$path" | awk '{print $1}'
  else
    python3 - "$path" <<'PY'
import hashlib
import sys

digest = hashlib.sha256()
with open(sys.argv[1], "rb") as handle:
    for chunk in iter(lambda: handle.read(1024 * 1024), b""):
        digest.update(chunk)
print(digest.hexdigest())
PY
  fi
}

setup_root_command() {
  if [ "$HOST_OS" = "darwin" ]; then
    ROOT_CMD=()
    return
  fi
  if [ "$(id -u)" -eq 0 ]; then
    ROOT_CMD=()
    return
  fi
  command_exists sudo || die "root access is required; install sudo or run this installer as root"
  log "requesting administrator privileges"
  if [ "$NON_INTERACTIVE" = "true" ]; then
    sudo -n -v
    ROOT_CMD=(sudo -n)
  else
    sudo -v </dev/tty
    ROOT_CMD=(sudo)
  fi
}

run_root() {
  if [ "${#ROOT_CMD[@]}" -eq 0 ]; then
    "$@"
    return
  fi
  "${ROOT_CMD[@]}" "$@"
}

run_as_service() {
  if [ "$HOST_OS" = "darwin" ]; then
    env HOME="$SERVICE_LOGIN_HOME" "$@"
    return
  fi
  if [ "$(id -u)" -eq 0 ]; then
    command_exists runuser || die "runuser is required when installing directly as root"
    runuser -u "$SERVICE_USER" -- env HOME="$CONGRID_HOME_DIR" "$@"
    return
  fi
  if [ "$NON_INTERACTIVE" = "true" ]; then
    sudo -n -u "$SERVICE_USER" env HOME="$CONGRID_HOME_DIR" "$@"
  else
    sudo -u "$SERVICE_USER" env HOME="$CONGRID_HOME_DIR" "$@"
  fi
}

setup_root_command

install_system_dependencies() {
  local manager=""
  local need_install=false
  local command_name
  local venv_probe="$TMP_WORK/venv-probe"

  if [ "$HOST_OS" = "darwin" ]; then
    for command_name in curl tar launchctl ps; do
      command_exists "$command_name" || die "required macOS command is missing: $command_name"
    done
    if ! command_exists python3 ||
      ! python3 -m venv "$venv_probe" >/dev/null 2>&1 ||
      ! "$venv_probe/bin/python" -m pip --version >/dev/null 2>&1; then
      rm -rf -- "$venv_probe"
      if command_exists brew; then
        log "installing Python with Homebrew"
        brew install python
      else
        die "Python 3 with venv is required; install Homebrew from https://brew.sh and rerun"
      fi
    fi
    rm -rf -- "$venv_probe"
    return
  fi

  for command_name in curl tar python3 systemctl ps getent groupadd useradd runuser; do
    if ! command_exists "$command_name"; then
      need_install=true
    fi
  done
  if command_exists python3; then
    if ! python3 -m venv "$venv_probe" >/dev/null 2>&1; then
      need_install=true
    elif ! "$venv_probe/bin/python" -m pip --version >/dev/null 2>&1; then
      need_install=true
    fi
    rm -rf -- "$venv_probe"
  fi

  if [ "$need_install" != "true" ]; then
    return
  fi

  for manager in apt-get dnf yum zypper pacman; do
    if command_exists "$manager"; then
      break
    fi
    manager=""
  done
  [ -n "$manager" ] || die "cannot install prerequisites: no supported package manager found"

  log "installing system prerequisites with $manager"
  case "$manager" in
    apt-get)
      run_root apt-get update
      run_root env DEBIAN_FRONTEND=noninteractive apt-get install -y \
        ca-certificates curl tar python3 python3-pip python3-venv procps passwd
      ;;
    dnf)
      run_root dnf install -y ca-certificates curl tar python3 python3-pip util-linux procps-ng shadow-utils
      ;;
    yum)
      run_root yum install -y ca-certificates curl tar python3 python3-pip util-linux procps-ng shadow-utils
      ;;
    zypper)
      run_root zypper --non-interactive install ca-certificates curl tar python3 python3-pip util-linux procps shadow
      ;;
    pacman)
      run_root pacman -Sy --needed --noconfirm ca-certificates curl tar python python-pip util-linux procps-ng shadow
      ;;
  esac
}

install_system_dependencies

if [ "$HOST_OS" = "linux" ]; then
  for required_command in curl tar python3 systemctl ps getent groupadd useradd runuser; do
    command_exists "$required_command" || die "required command is still missing: $required_command"
  done
  if [ "$(ps -p 1 -o comm= | tr -d '[:space:]')" != "systemd" ]; then
    die "systemd is not PID 1; this installer needs a systemd-based Linux host"
  fi
else
  for required_command in curl tar python3 launchctl ps; do
    command_exists "$required_command" || die "required command is still missing: $required_command"
  done
fi

python3 - <<'PY' || die "Python 3.9 or newer is required"
import sys
raise SystemExit(0 if sys.version_info >= (3, 9) else 1)
PY

VENV_VERIFY_PATH="$TMP_WORK/venv-verify"
if ! python3 -m venv "$VENV_VERIFY_PATH" >/dev/null 2>&1 ||
  ! "$VENV_VERIFY_PATH/bin/python" -m pip --version >/dev/null 2>&1; then
  die "Python venv cannot provide pip; install python3-venv and python3-pip (or Homebrew Python on macOS), then rerun"
fi
rm -rf -- "$VENV_VERIFY_PATH"

load_saved_value() {
  local key="$1"
  local fallback="$2"
  local state_file="$CONFIG_DIR/install-state.json"

  if ! run_root test -f "$state_file"; then
    printf '%s' "$fallback"
    return
  fi
  if ! run_root python3 - "$state_file" "$key" <<'PY'
import json
import sys

path, key = sys.argv[1], sys.argv[2]
try:
    with open(path, "r", encoding="utf-8") as handle:
        value = json.load(handle).get(key, "")
except (OSError, ValueError, TypeError):
    raise SystemExit(1)
if isinstance(value, bool):
    print("true" if value else "false", end="")
elif value is None:
    print("", end="")
else:
    print(str(value), end="")
PY
  then
    printf '%s' "$fallback"
  fi
}

env_or_saved() {
  local env_name="$1"
  local state_key="$2"
  local fallback="$3"
  if [ "${!env_name+x}" = "x" ]; then
    printf '%s' "${!env_name}"
    return
  fi
  load_saved_value "$state_key" "$fallback"
}

prompt_value() {
  local output_name="$1"
  local label="$2"
  local default_value="$3"
  local required="$4"
  local value=""

  if [ "$NON_INTERACTIVE" = "true" ]; then
    value="$default_value"
  else
    if [ -n "$default_value" ]; then
      printf '%s [%s]: ' "$label" "$default_value" >/dev/tty
    else
      printf '%s: ' "$label" >/dev/tty
    fi
    IFS= read -r value </dev/tty
    if [ -z "$value" ]; then
      value="$default_value"
    fi
  fi

  if [ "$required" = "true" ] && [ -z "$value" ]; then
    die "$label is required"
  fi
  if [[ "$value" == *$'\n'* || "$value" == *$'\r'* ]]; then
    die "$label must be a single line"
  fi
  printf -v "$output_name" '%s' "$value"
}

prompt_yes_no() {
  local output_name="$1"
  local label="$2"
  local default_value="$3"
  local answer=""

  case "$default_value" in
    true) answer="y" ;;
    false) answer="n" ;;
    *) die "invalid yes/no default for $label" ;;
  esac

  if [ "$NON_INTERACTIVE" != "true" ]; then
    if [ "$default_value" = "true" ]; then
      printf '%s [Y/n]: ' "$label" >/dev/tty
    else
      printf '%s [y/N]: ' "$label" >/dev/tty
    fi
    IFS= read -r answer </dev/tty
    if [ -z "$answer" ]; then
      if [ "$default_value" = "true" ]; then
        answer="y"
      else
        answer="n"
      fi
    fi
  fi

  case "$answer" in
    y|Y|yes|YES|Yes|true)
      printf -v "$output_name" '%s' "true"
      ;;
    n|N|no|NO|No|false)
      printf -v "$output_name" '%s' "false"
      ;;
    *)
      die "please answer yes or no for: $label"
      ;;
  esac
}

read_secret() {
  local output_name="$1"
  local label="$2"
  local value=""

  [ "$NON_INTERACTIVE" != "true" ] || die "$label must be provided through the environment in non-interactive mode"
  printf '%s: ' "$label" >/dev/tty
  TTY_ECHO_DISABLED=true
  stty -echo </dev/tty
  IFS= read -r value </dev/tty
  stty echo </dev/tty
  TTY_ECHO_DISABLED=false
  printf '\n' >/dev/tty
  printf -v "$output_name" '%s' "$value"
}

validate_simple_name() {
  local label="$1"
  local value="$2"
  [[ "$value" =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$ ]] ||
    die "$label contains unsupported characters: $value"
}

validate_bool() {
  local label="$1"
  local value="$2"
  case "$value" in
    true|false) ;;
    *) die "$label must be true or false" ;;
  esac
}

validate_single_line() {
  local label="$1"
  local value="$2"
  [[ "$value" != *$'\n'* && "$value" != *$'\r'* ]] || die "$label must be a single line"
}

validate_listen_address() {
  local label="$1"
  local value="$2"
  python3 - "$label" "$value" <<'PY'
import sys

label, value = sys.argv[1], sys.argv[2]
try:
    host, raw_port = value.rsplit(":", 1)
    if not raw_port.isdigit():
        raise ValueError
    port = int(raw_port)
except (ValueError, TypeError):
    print(f"{label} must use host:port syntax", file=sys.stderr)
    raise SystemExit(1)
if not host or port < 1 or port > 65535:
    print(f"{label} must contain a host and a port from 1 to 65535", file=sys.stderr)
    raise SystemExit(1)
if host not in {"127.0.0.1", "localhost", "0.0.0.0"}:
    print(
        f"{label} host must be 127.0.0.1, localhost, or 0.0.0.0",
        file=sys.stderr,
    )
    raise SystemExit(1)
PY
}

validate_service_ports() {
  local indexer_addr="$1"
  local verifier_addr="$2"
  python3 - "$indexer_addr" "$verifier_addr" <<'PY'
import sys

indexer_port = int(sys.argv[1].rsplit(":", 1)[1])
verifier_port = int(sys.argv[2].rsplit(":", 1)[1])
reserved = {1317, 8000, 9090, 26656, 26657}
if indexer_port in reserved:
    print(f"indexerd port {indexer_port} conflicts with another stack service", file=sys.stderr)
    raise SystemExit(1)
if verifier_port in reserved:
    print(f"verifierd port {verifier_port} conflicts with another stack service", file=sys.stderr)
    raise SystemExit(1)
if indexer_port == verifier_port:
    print("indexerd and verifierd must use different ports", file=sys.stderr)
    raise SystemExit(1)
PY
}

validate_peer_list() {
  local label="$1"
  local value="$2"
  [ -z "$value" ] && return
  python3 - "$label" "$value" <<'PY'
import ipaddress
import re
import sys

label, value = sys.argv[1], sys.argv[2]
peers = value.split(",")
for index, peer in enumerate(peers, 1):
    peer = peer.strip()
    match = re.fullmatch(r"([0-9A-Fa-f]{40})@(.+)", peer)
    if not match:
        print(
            f"{label} entry {index} must use 40-character-node-id@host:port syntax",
            file=sys.stderr,
        )
        raise SystemExit(1)

    endpoint = match.group(2)
    host, separator, raw_port = endpoint.rpartition(":")
    if not separator or not raw_port.isdigit():
        print(f"{label} entry {index} has an invalid endpoint", file=sys.stderr)
        raise SystemExit(1)
    port = int(raw_port)
    if port < 1 or port > 65535:
        print(f"{label} entry {index} has an invalid port", file=sys.stderr)
        raise SystemExit(1)

    candidate = host
    if candidate.startswith("[") and candidate.endswith("]"):
        candidate = candidate[1:-1]
    try:
        ipaddress.ip_address(candidate)
    except ValueError:
        if not re.fullmatch(
            r"(?=.{1,253}\Z)(?:[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?\.)*"
            r"[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?",
            candidate,
        ):
            print(f"{label} entry {index} has an invalid host", file=sys.stderr)
            raise SystemExit(1)
PY
}

download_seed_list() {
  local seeds_path="$TMP_WORK/seeds.txt"
  local checksum_path="$TMP_WORK/seeds.txt.sha256"
  local checksum_url=""
  local expected_sha256="$SEEDS_SHA256"
  local actual_sha256=""
  local normalized=""

  log "downloading network seed list"
  curl --fail --location --silent --show-error --retry 3 --retry-delay 2 \
    "$SEEDS_URL" --output "$seeds_path"

  if [ -z "$expected_sha256" ]; then
    if [ -n "${CONGRID_SEEDS_SHA256_URL:-}" ]; then
      checksum_url="$CONGRID_SEEDS_SHA256_URL"
    elif [[ "$SEEDS_URL" == *\?* ]]; then
      checksum_url="${SEEDS_URL%%\?*}.sha256?${SEEDS_URL#*\?}"
    else
      checksum_url="$SEEDS_URL.sha256"
    fi
    log "downloading seed-list checksum"
    curl --fail --location --silent --show-error --retry 3 --retry-delay 2 \
      "$checksum_url" --output "$checksum_path"
    expected_sha256="$(awk 'NR == 1 { print $1 }' "$checksum_path")"
  fi

  [[ "$expected_sha256" =~ ^[0-9A-Fa-f]{64}$ ]] ||
    die "seed-list checksum is not a valid SHA-256"
  expected_sha256="$(printf '%s' "$expected_sha256" | tr '[:upper:]' '[:lower:]')"
  actual_sha256="$(file_sha256 "$seeds_path")"
  [ "$actual_sha256" = "$expected_sha256" ] ||
    die "seed-list checksum mismatch (expected $expected_sha256, got $actual_sha256)"

  normalized="$(python3 - "$seeds_path" <<'PY'
import sys

entries = []
with open(sys.argv[1], "r", encoding="utf-8") as handle:
    for raw_line in handle:
        line = raw_line.split("#", 1)[0].strip()
        if not line:
            continue
        entries.extend(part.strip() for part in line.split(",") if part.strip())
print(",".join(entries), end="")
PY
)"
  [ -n "$normalized" ] || die "downloaded seed list is empty"
  printf '%s' "$normalized"
}

NODE_ALREADY_INITIALIZED=false
if run_root test -s "$CONGRID_HOME_DIR/config/config.toml"; then
  NODE_ALREADY_INITIALIZED=true
  log "existing node home detected at $CONGRID_HOME_DIR; chain data and genesis will be preserved"
fi

printf '\nContent Grid native operator setup\n' >&2
if [ "$HOST_OS" = "linux" ]; then
  printf 'Services run under the dedicated %s system user and systemd.\n\n' "$SERVICE_USER" >&2
else
  printf 'Services run under the current %s user and launchd.\n\n' "$SERVICE_USER" >&2
fi

default_moniker="$(env_or_saved CONGRID_MONIKER moniker "$(hostname -s 2>/dev/null || printf 'congrid-node')")"
default_peers="$(env_or_saved CONGRID_PERSISTENT_PEERS persistent_peers "")"
default_external_address="$(env_or_saved CONGRID_P2P_EXTERNAL_ADDRESS p2p_external_address "")"
default_addr_book_strict="$(env_or_saved CONGRID_P2P_ADDR_BOOK_STRICT p2p_addr_book_strict "true")"
default_indexer_listen="$(env_or_saved CONGRID_INDEXER_LISTEN_ADDR indexer_listen_addr "127.0.0.1:9100")"
default_verifier_listen="$(env_or_saved CONGRID_VERIFIER_LISTEN_ADDR verifier_listen_addr "127.0.0.1:9200")"
default_key_name="$(env_or_saved CONGRID_VERIFIER_KEY_NAME verifier_key_name "verifier-key")"
default_gas_prices="$(env_or_saved CONGRID_VERIFIER_GAS_PRICES verifier_gas_prices "0.001ucongrid")"

prompt_value MONIKER "Node name (moniker)" "$default_moniker" true
CHAIN_ID="${CONGRID_CHAIN_ID:-congrid-main}"
GENESIS_URL="${CONGRID_GENESIS_URL:-$DOWNLOAD_BASE_URL/genesis.json}"
log "using chain ID: $CHAIN_ID"
log "using genesis: $GENESIS_URL"
if [ "${CONGRID_P2P_SEEDS+x}" = "x" ]; then
  P2P_SEEDS="$CONGRID_P2P_SEEDS"
  SEEDS_SOURCE_URL=""
else
  P2P_SEEDS="$(download_seed_list)"
  SEEDS_SOURCE_URL="$SEEDS_URL"
fi
prompt_value PERSISTENT_PEERS "Persistent peers (optional)" "$default_peers" false
prompt_value P2P_EXTERNAL_ADDRESS "Public P2P address (optional host:port)" "$default_external_address" false
prompt_yes_no P2P_ADDR_BOOK_STRICT "Reject private/unroutable peer addresses" "$default_addr_book_strict"
prompt_value INDEXER_LISTEN_ADDR "indexerd listen address" "$default_indexer_listen" true
prompt_value VERIFIER_LISTEN_ADDR "verifierd health listen address" "$default_verifier_listen" true
prompt_value VERIFIER_KEY_NAME "Verifier key name" "$default_key_name" true
prompt_value VERIFIER_GAS_PRICES "Verifier transaction gas prices" "$default_gas_prices" true

DRAND_DELIVERY_DISABLED="${CONGRID_DRAND_DELIVERY_DISABLED:-false}"
if [ "$DRAND_DELIVERY_DISABLED" = "true" ]; then
  warn "drand delivery is disabled by CONGRID_DRAND_DELIVERY_DISABLED=true"
else
  log "verifier drand delivery is enabled"
fi

validate_simple_name "node moniker" "$MONIKER"
validate_simple_name "chain ID" "$CHAIN_ID"
validate_simple_name "verifier key name" "$VERIFIER_KEY_NAME"
validate_bool "P2P address-book strict setting" "$P2P_ADDR_BOOK_STRICT"
validate_bool "drand delivery disabled setting" "$DRAND_DELIVERY_DISABLED"
validate_single_line "P2P seeds" "$P2P_SEEDS"
validate_single_line "persistent peers" "$PERSISTENT_PEERS"
validate_single_line "P2P external address" "$P2P_EXTERNAL_ADDRESS"
validate_single_line "indexerd listen address" "$INDEXER_LISTEN_ADDR"
validate_single_line "verifierd listen address" "$VERIFIER_LISTEN_ADDR"
validate_single_line "gas prices" "$VERIFIER_GAS_PRICES"
validate_peer_list "P2P seeds" "$P2P_SEEDS"
validate_peer_list "persistent peers" "$PERSISTENT_PEERS"
validate_listen_address "indexerd listen address" "$INDEXER_LISTEN_ADDR"
validate_listen_address "verifierd listen address" "$VERIFIER_LISTEN_ADDR"
validate_service_ports "$INDEXER_LISTEN_ADDR" "$VERIFIER_LISTEN_ADDR"
INDEXER_LOCAL_PORT="${INDEXER_LISTEN_ADDR##*:}"

if [ -z "$P2P_SEEDS" ] && [ -z "$PERSISTENT_PEERS" ]; then
  if [ "${CONGRID_ALLOW_NO_PEERS:-false}" != "true" ]; then
    die "at least one P2P seed or persistent peer is required (set CONGRID_ALLOW_NO_PEERS=true only for a deliberate isolated node)"
  fi
  warn "installing without a bootstrap peer; the node cannot discover the network by itself"
fi

if [ -n "${CONGRID_VERIFIER_KEYRING_PASSPHRASE:-}" ]; then
  VERIFIER_PASSPHRASE="$CONGRID_VERIFIER_KEYRING_PASSPHRASE"
  unset CONGRID_VERIFIER_KEYRING_PASSPHRASE
else
  read_secret VERIFIER_PASSPHRASE "Verifier keyring passphrase (minimum 8 characters)"
  read_secret passphrase_confirmation "Confirm verifier keyring passphrase"
  [ "$VERIFIER_PASSPHRASE" = "$passphrase_confirmation" ] || die "keyring passphrases do not match"
  unset passphrase_confirmation
fi
[ "${#VERIFIER_PASSPHRASE}" -ge 8 ] || die "verifier keyring passphrase must contain at least 8 characters"
validate_single_line "verifier keyring passphrase" "$VERIFIER_PASSPHRASE"

BUNDLE_NAME="${CONGRID_BUNDLE_NAME:-congrid-native-${HOST_OS}-${RELEASE_ARCH}.tar.gz}"
if [ -z "$BUNDLE_URL" ]; then
  BUNDLE_URL="$DOWNLOAD_BASE_URL/$BUNDLE_NAME"
else
  BUNDLE_NAME="${BUNDLE_URL##*/}"
  BUNDLE_NAME="${BUNDLE_NAME%%\?*}"
fi
[ -n "$BUNDLE_NAME" ] || die "cannot determine native bundle filename"
validate_simple_name "native bundle filename" "$BUNDLE_NAME"

BUNDLE_PATH="$TMP_WORK/$BUNDLE_NAME"
CHECKSUM_PATH="$TMP_WORK/$BUNDLE_NAME.sha256"

log "downloading native release bundle for $HOST_OS/$RELEASE_ARCH"
curl --fail --location --silent --show-error --retry 3 --retry-delay 2 \
  "$BUNDLE_URL" --output "$BUNDLE_PATH"

if [ -z "$BUNDLE_SHA256" ]; then
  if [ -n "${CONGRID_BUNDLE_SHA256_URL:-}" ]; then
    checksum_url="$CONGRID_BUNDLE_SHA256_URL"
  elif [[ "$BUNDLE_URL" == *\?* ]]; then
    checksum_url="${BUNDLE_URL%%\?*}.sha256?${BUNDLE_URL#*\?}"
  else
    checksum_url="$BUNDLE_URL.sha256"
  fi
  log "downloading release checksum"
  curl --fail --location --silent --show-error --retry 3 --retry-delay 2 \
    "$checksum_url" --output "$CHECKSUM_PATH"
  BUNDLE_SHA256="$(awk 'NR == 1 { print $1 }' "$CHECKSUM_PATH")"
fi

[[ "$BUNDLE_SHA256" =~ ^[0-9A-Fa-f]{64}$ ]] || die "release checksum is not a valid SHA-256"
BUNDLE_SHA256="$(printf '%s' "$BUNDLE_SHA256" | tr '[:upper:]' '[:lower:]')"

actual_sha256="$(file_sha256 "$BUNDLE_PATH")"

[ "$actual_sha256" = "$BUNDLE_SHA256" ] ||
  die "native bundle checksum mismatch (expected $BUNDLE_SHA256, got $actual_sha256)"
log "release checksum verified: $actual_sha256"

ARCHIVE_LIST="$TMP_WORK/archive-list.txt"
tar -tzf "$BUNDLE_PATH" >"$ARCHIVE_LIST"
while IFS= read -r archive_entry; do
  case "$archive_entry" in
    ""|/*|../*|*/../*|*/..)
      die "unsafe path in native release bundle: $archive_entry"
      ;;
  esac
done <"$ARCHIVE_LIST"

if tar -tvzf "$BUNDLE_PATH" | awk 'substr($1, 1, 1) != "-" && substr($1, 1, 1) != "d" { found=1 } END { exit found ? 0 : 1 }'; then
  die "native release bundle must contain only regular files and directories"
fi

BUNDLE_EXTRACT_DIR="$TMP_WORK/bundle"
mkdir -p "$BUNDLE_EXTRACT_DIR"
tar -xzf "$BUNDLE_PATH" -C "$BUNDLE_EXTRACT_DIR"

BUNDLE_ROOT="$BUNDLE_EXTRACT_DIR/congrid-native"
for required_file in \
  "$BUNDLE_ROOT/bin/content-grid-d" \
  "$BUNDLE_ROOT/bin/verifierd" \
  "$BUNDLE_ROOT/bin/indexerd" \
  "$BUNDLE_ROOT/chromad/server.py" \
  "$BUNDLE_ROOT/chromad/requirements.txt"; do
  [ -f "$required_file" ] && [ ! -L "$required_file" ] ||
    die "native release bundle is missing $(basename "$required_file")"
done

for binary_file in content-grid-d verifierd indexerd; do
  chmod 0755 "$BUNDLE_ROOT/bin/$binary_file"
done
if ! "$BUNDLE_ROOT/bin/content-grid-d" version >/dev/null 2>&1; then
  die "content-grid-d in the release bundle cannot run on this $HOST_OS/$RELEASE_ARCH host"
fi

log "installing Content Grid binaries"
run_root install -d -m 0755 "$BIN_DIR" "$CHROMAD_DIR" "$LIBEXEC_DIR" "$CONFIG_DIR"
run_root install -m 0755 "$BUNDLE_ROOT/bin/content-grid-d" "$BIN_DIR/content-grid-d"
run_root install -m 0755 "$BUNDLE_ROOT/bin/verifierd" "$BIN_DIR/verifierd"
run_root install -m 0755 "$BUNDLE_ROOT/bin/indexerd" "$BIN_DIR/indexerd"
run_root install -m 0644 "$BUNDLE_ROOT/chromad/server.py" "$CHROMAD_DIR/server.py"
run_root install -m 0644 "$BUNDLE_ROOT/chromad/requirements.txt" "$CHROMAD_DIR/requirements.txt"

if [ "$HOST_OS" = "linux" ]; then
  if ! run_root getent group "$SERVICE_GROUP" >/dev/null 2>&1; then
    log "creating system group $SERVICE_GROUP"
    run_root groupadd --system "$SERVICE_GROUP"
  fi
  if ! run_root getent passwd "$SERVICE_USER" >/dev/null 2>&1; then
    log "creating system user $SERVICE_USER"
    nologin_shell="$(command -v nologin || printf '/usr/sbin/nologin')"
    run_root useradd --system \
      --gid "$SERVICE_GROUP" \
      --home-dir "$CONGRID_HOME_DIR" \
      --shell "$nologin_shell" \
      "$SERVICE_USER"
  fi

  run_root install -d -o "$SERVICE_USER" -g "$SERVICE_GROUP" -m 0750 \
    "$CONGRID_HOME_DIR" \
    "$CONGRID_HOME_DIR/chroma" \
    "$CONGRID_HOME_DIR/keyrings" \
    "$CONGRID_HOME_DIR/keyrings/verifier" \
    "$CONGRID_HOME_DIR/verifierd-state" \
    "$CONGRID_HOME_DIR/logs"
else
  install -d -m 0750 \
    "$CONGRID_HOME_DIR" \
    "$CONGRID_HOME_DIR/chroma" \
    "$CONGRID_HOME_DIR/keyrings" \
    "$CONGRID_HOME_DIR/keyrings/verifier" \
    "$CONGRID_HOME_DIR/verifierd-state" \
    "$CONGRID_HOME_DIR/logs" \
    "$LAUNCHD_DIR"
fi

log "creating/updating chromad Python environment"
if ! run_root test -x "$CHROMAD_DIR/.venv/bin/python"; then
  if ! run_root python3 -m venv "$CHROMAD_DIR/.venv"; then
    die "python3 venv creation failed; install your distribution's python3-venv package and rerun"
  fi
fi

if ! run_root "$CHROMAD_DIR/.venv/bin/python" -m pip --version >/dev/null 2>&1; then
  log "chromad virtual environment has no pip; attempting repair"
  if ! run_root "$CHROMAD_DIR/.venv/bin/python" -m ensurepip --upgrade >/dev/null 2>&1 ||
    ! run_root "$CHROMAD_DIR/.venv/bin/python" -m pip --version >/dev/null 2>&1; then
    log "recreating chromad virtual environment with pip"
    if ! run_root python3 -m venv --clear "$CHROMAD_DIR/.venv"; then
      die "failed to recreate chromad Python environment; install python3-venv and python3-pip, then rerun"
    fi
  fi
fi

run_root "$CHROMAD_DIR/.venv/bin/python" -m pip --version >/dev/null 2>&1 ||
  die "chromad virtual environment still has no pip after repair"
run_root "$CHROMAD_DIR/.venv/bin/python" -m pip install \
  --disable-pip-version-check --quiet --upgrade pip
run_root "$CHROMAD_DIR/.venv/bin/python" -m pip install \
  --disable-pip-version-check --quiet --requirement "$CHROMAD_DIR/requirements.txt"

GENESIS_TMP="$TMP_WORK/genesis.json"
if [ "$NODE_ALREADY_INITIALIZED" != "true" ]; then
  log "downloading network genesis"
  curl --fail --location --silent --show-error --retry 3 --retry-delay 2 \
    "$GENESIS_URL" --output "$GENESIS_TMP"
  python3 - "$GENESIS_TMP" "$CHAIN_ID" <<'PY' || die "genesis.json is invalid or belongs to a different chain"
import json
import sys

path, expected_chain_id = sys.argv[1], sys.argv[2]
with open(path, "r", encoding="utf-8") as handle:
    genesis = json.load(handle)
actual_chain_id = str(genesis.get("chain_id", "")).strip()
if actual_chain_id != expected_chain_id:
    print(
        f"genesis chain_id mismatch: expected {expected_chain_id!r}, got {actual_chain_id!r}",
        file=sys.stderr,
    )
    raise SystemExit(1)
PY

  log "initializing content-grid-d home"
  run_as_service "$BIN_DIR/content-grid-d" init "$MONIKER" \
    --home "$CONGRID_HOME_DIR" \
    --chain-id "$CHAIN_ID" >/dev/null 2>&1
  if [ "$HOST_OS" = "linux" ]; then
    run_root install -o "$SERVICE_USER" -g "$SERVICE_GROUP" -m 0640 \
      "$GENESIS_TMP" "$CONGRID_HOME_DIR/config/genesis.json"
  else
    install -m 0600 "$GENESIS_TMP" "$CONGRID_HOME_DIR/config/genesis.json"
  fi
else
  existing_chain_id="$(run_root python3 - "$CONGRID_HOME_DIR/config/genesis.json" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as handle:
    print(str(json.load(handle).get("chain_id", "")).strip(), end="")
PY
)"
  [ "$existing_chain_id" = "$CHAIN_ID" ] ||
    die "configured chain ID $CHAIN_ID does not match existing genesis chain ID $existing_chain_id"
fi

log "updating node configuration"
run_root env \
  CFG_MONIKER="$MONIKER" \
  CFG_CHAIN_ID="$CHAIN_ID" \
  CFG_P2P_SEEDS="$P2P_SEEDS" \
  CFG_PERSISTENT_PEERS="$PERSISTENT_PEERS" \
  CFG_P2P_EXTERNAL_ADDRESS="$P2P_EXTERNAL_ADDRESS" \
  CFG_P2P_ADDR_BOOK_STRICT="$P2P_ADDR_BOOK_STRICT" \
  python3 - "$CONGRID_HOME_DIR" <<'PY'
import json
import os
import re
import sys

home = sys.argv[1]


def update_toml(path, changes):
    with open(path, "r", encoding="utf-8") as handle:
        lines = handle.readlines()

    section = ""
    found = set()
    output = []
    section_pattern = re.compile(r"^\s*\[([^\]]+)\]\s*$")
    key_pattern = re.compile(r"^(\s*)([A-Za-z0-9_-]+)(\s*=\s*).*$")

    for line in lines:
        section_match = section_pattern.match(line)
        if section_match:
            section = section_match.group(1)
            output.append(line)
            continue
        key_match = key_pattern.match(line)
        lookup = (section, key_match.group(2)) if key_match else None
        if lookup in changes:
            indent, key, separator = key_match.group(1), key_match.group(2), key_match.group(3)
            output.append(f"{indent}{key}{separator}{changes[lookup]}\n")
            found.add(lookup)
        else:
            output.append(line)

    missing = set(changes) - found
    if missing:
        names = ", ".join(f"[{section}] {key}" for section, key in sorted(missing))
        raise RuntimeError(f"missing TOML settings in {path}: {names}")

    temporary = path + ".congrid-install.tmp"
    with open(temporary, "w", encoding="utf-8") as handle:
        handle.writelines(output)
    os.replace(temporary, path)


quoted = lambda value: json.dumps(value, ensure_ascii=False)
update_toml(
    os.path.join(home, "config", "config.toml"),
    {
        ("", "moniker"): quoted(os.environ["CFG_MONIKER"]),
        ("rpc", "laddr"): quoted("tcp://127.0.0.1:26657"),
        ("p2p", "laddr"): quoted("tcp://0.0.0.0:26656"),
        ("p2p", "external_address"): quoted(os.environ["CFG_P2P_EXTERNAL_ADDRESS"]),
        ("p2p", "pex"): "true",
        ("p2p", "addr_book_strict"): os.environ["CFG_P2P_ADDR_BOOK_STRICT"],
        ("p2p", "seeds"): quoted(os.environ["CFG_P2P_SEEDS"]),
        ("p2p", "persistent_peers"): quoted(os.environ["CFG_PERSISTENT_PEERS"]),
    },
)
update_toml(
    os.path.join(home, "config", "app.toml"),
    {
        ("", "minimum-gas-prices"): quoted("0.001ucongrid"),
        ("api", "enable"): "true",
        ("api", "address"): quoted("tcp://127.0.0.1:1317"),
        ("grpc", "enable"): "true",
        ("grpc", "address"): quoted("127.0.0.1:9090"),
    },
)
update_toml(
    os.path.join(home, "config", "client.toml"),
    {
        ("", "chain-id"): quoted(os.environ["CFG_CHAIN_ID"]),
        ("", "keyring-backend"): quoted("file"),
        ("", "node"): quoted("tcp://127.0.0.1:26657"),
    },
)
PY
if [ "$HOST_OS" = "linux" ]; then
  run_root chown -R "$SERVICE_USER:$SERVICE_GROUP" "$CONGRID_HOME_DIR/config"
fi

key_command() {
  run_as_service "$BIN_DIR/content-grid-d" "$@" \
    --home "$CONGRID_HOME_DIR" \
    --keyring-backend file \
    --keyring-dir "$CONGRID_HOME_DIR/keyrings/verifier"
}

key_exists=false
KEY_ADDRESS_TMP="$TMP_WORK/key-address"
if printf '%s\n%s\n' "$VERIFIER_PASSPHRASE" "$VERIFIER_PASSPHRASE" |
  key_command keys show "$VERIFIER_KEY_NAME" --address \
    >"$KEY_ADDRESS_TMP" 2>/dev/null; then
  key_address="$(tr -d '[:space:]' <"$KEY_ADDRESS_TMP")"
  if [ -n "$key_address" ]; then
    key_exists=true
  fi
fi

if [ "$key_exists" != "true" ]; then
  keyring_unlocked=false
  if printf '%s\n%s\n' "$VERIFIER_PASSPHRASE" "$VERIFIER_PASSPHRASE" |
    key_command keys list --output json >/dev/null 2>&1; then
    keyring_unlocked=true
  elif ! run_root find "$CONGRID_HOME_DIR/keyrings/verifier" -type f -print -quit | grep -q .; then
    keyring_unlocked=true
  fi
  [ "$keyring_unlocked" = "true" ] ||
    die "cannot unlock the existing verifier keyring; check the passphrase"

  KEY_ACTION="${CONGRID_VERIFIER_KEY_ACTION:-}"
  if [ -z "$KEY_ACTION" ]; then
    if [ "$NON_INTERACTIVE" = "true" ]; then
      die "CONGRID_VERIFIER_KEY_ACTION must be create or recover when the verifier key is missing"
    fi
    printf 'Verifier key "%s" is missing. Create a new key or recover one? [create/recover]: ' \
      "$VERIFIER_KEY_NAME" >/dev/tty
    IFS= read -r KEY_ACTION </dev/tty
    [ -n "$KEY_ACTION" ] || KEY_ACTION="create"
  fi

  case "$KEY_ACTION" in
    create)
      CREATED_KEY_JSON="$TMP_WORK/created-verifier-key.json"
      log "creating verifier key $VERIFIER_KEY_NAME"
      if ! printf '%s\n%s\n' "$VERIFIER_PASSPHRASE" "$VERIFIER_PASSPHRASE" |
        key_command keys add "$VERIFIER_KEY_NAME" --output json >"$CREATED_KEY_JSON"; then
        die "failed to create verifier key"
      fi
      chmod 600 "$CREATED_KEY_JSON"

      invoking_user="${SUDO_USER:-$(id -un)}"
      if [ "$HOST_OS" = "linux" ]; then
        invoking_home="$(getent passwd "$invoking_user" | awk -F: '{print $6}')"
        [ -n "$invoking_home" ] || invoking_home="/root"
      else
        invoking_home="$HOME"
      fi
      backup_path="$invoking_home/congrid-verifier-key-$(date -u +%Y%m%dT%H%M%SZ).json"
      invoking_group="$(id -gn "$invoking_user")"
      if [ "$HOST_OS" = "linux" ]; then
        run_root install -o "$invoking_user" -g "$invoking_group" -m 0600 \
          "$CREATED_KEY_JSON" "$backup_path"
      else
        install -m 0600 "$CREATED_KEY_JSON" "$backup_path"
      fi
      warn "a new verifier key was created; its only mnemonic backup is: $backup_path"
      warn "copy that file to secure offline storage before funding the address"
      ;;
    recover)
      if [ -n "${CONGRID_VERIFIER_MNEMONIC:-}" ]; then
        VERIFIER_MNEMONIC="$CONGRID_VERIFIER_MNEMONIC"
        unset CONGRID_VERIFIER_MNEMONIC
      else
        read_secret VERIFIER_MNEMONIC "Verifier mnemonic"
      fi
      [ -n "$VERIFIER_MNEMONIC" ] || die "verifier mnemonic is empty"
      validate_single_line "verifier mnemonic" "$VERIFIER_MNEMONIC"
      SENSITIVE_MNEMONIC_PATH="$CONGRID_HOME_DIR/.verifier-mnemonic.$$"
      printf '%s' "$VERIFIER_MNEMONIC" >"$TMP_WORK/verifier.mnemonic"
      chmod 600 "$TMP_WORK/verifier.mnemonic"
      if [ "$HOST_OS" = "linux" ]; then
        run_root install -o "$SERVICE_USER" -g "$SERVICE_GROUP" -m 0600 \
          "$TMP_WORK/verifier.mnemonic" "$SENSITIVE_MNEMONIC_PATH"
      else
        install -m 0600 "$TMP_WORK/verifier.mnemonic" "$SENSITIVE_MNEMONIC_PATH"
      fi
      if ! printf '%s\n%s\n' "$VERIFIER_PASSPHRASE" "$VERIFIER_PASSPHRASE" |
        key_command keys add "$VERIFIER_KEY_NAME" \
          --recover \
          --source "$SENSITIVE_MNEMONIC_PATH" \
          --output json >/dev/null; then
        run_root rm -f -- "$SENSITIVE_MNEMONIC_PATH"
        SENSITIVE_MNEMONIC_PATH=""
        die "failed to recover verifier key"
      fi
      run_root rm -f -- "$SENSITIVE_MNEMONIC_PATH"
      SENSITIVE_MNEMONIC_PATH=""
      unset VERIFIER_MNEMONIC
      ;;
    *)
      die "verifier key action must be create or recover"
      ;;
  esac

  key_address="$(
    printf '%s\n%s\n' "$VERIFIER_PASSPHRASE" "$VERIFIER_PASSPHRASE" |
      key_command keys show "$VERIFIER_KEY_NAME" --address 2>/dev/null |
      tr -d '[:space:]'
  )"
fi

[ -n "$key_address" ] || die "could not resolve the verifier account address"
log "verifier account: $key_address"

INDEXER_CONFIG_TMP="$TMP_WORK/indexerd.json"
VERIFIER_CONFIG_TMP="$TMP_WORK/verifierd.json"
STATE_CONFIG_TMP="$TMP_WORK/install-state.json"

env \
  OUT_INDEXER="$INDEXER_CONFIG_TMP" \
  OUT_VERIFIER="$VERIFIER_CONFIG_TMP" \
  OUT_STATE="$STATE_CONFIG_TMP" \
  CFG_MONIKER="$MONIKER" \
  CFG_CHAIN_ID="$CHAIN_ID" \
  CFG_GENESIS_URL="$GENESIS_URL" \
  CFG_P2P_SEEDS="$P2P_SEEDS" \
  CFG_SEEDS_URL="$SEEDS_SOURCE_URL" \
  CFG_PERSISTENT_PEERS="$PERSISTENT_PEERS" \
  CFG_P2P_EXTERNAL_ADDRESS="$P2P_EXTERNAL_ADDRESS" \
  CFG_P2P_ADDR_BOOK_STRICT="$P2P_ADDR_BOOK_STRICT" \
  CFG_INDEXER_LISTEN="$INDEXER_LISTEN_ADDR" \
  CFG_VERIFIER_LISTEN="$VERIFIER_LISTEN_ADDR" \
  CFG_VERIFIER_KEY_NAME="$VERIFIER_KEY_NAME" \
  CFG_VERIFIER_ADDRESS="$key_address" \
  CFG_VERIFIER_GAS_PRICES="$VERIFIER_GAS_PRICES" \
  CFG_DRAND_DISABLED="$DRAND_DELIVERY_DISABLED" \
  CFG_BUNDLE_URL="$BUNDLE_URL" \
  CFG_BUNDLE_SHA256="$BUNDLE_SHA256" \
  CFG_INSTALLER_VERSION="$INSTALLER_VERSION" \
  CFG_HOME_DIR="$CONGRID_HOME_DIR" \
  CFG_BIN_DIR="$BIN_DIR" \
  python3 - <<'PY'
import json
import os


def write_json(path, value):
    with open(path, "w", encoding="utf-8") as handle:
        json.dump(value, handle, indent=2, ensure_ascii=False)
        handle.write("\n")


write_json(
    os.environ["OUT_INDEXER"],
    {
        "publishers": [],
        "chain_grpc_addr": "127.0.0.1:9090",
        "chain_timeout_seconds": 10,
        "chain_page_limit": 200,
        "listen_addr": os.environ["CFG_INDEXER_LISTEN"],
        "chroma_base_url": "http://127.0.0.1:8000",
        "chroma_collection": "publishers",
        "signature_bits": 128,
        "fetch_timeout_seconds": 10,
        "index_interval_minutes": 60,
        "max_body_bytes": 1048576,
    },
)
write_json(
    os.environ["OUT_VERIFIER"],
    {
        "grpc_addr": "127.0.0.1:9090",
        "listen_addr": os.environ["CFG_VERIFIER_LISTEN"],
        "verifier_address": os.environ["CFG_VERIFIER_ADDRESS"],
        "state_dir": os.path.join(os.environ["CFG_HOME_DIR"], "verifierd-state"),
        "poll_interval_seconds": 15,
        "verify_scheme": "https",
        "commit_start_buffer_seconds": 15,
        "commit_window_seconds": 300,
        "round_interval_seconds": 3600,
        "assignment_delay_max_seconds": 3600,
        "disable_assignment_check": False,
        "retry_backoff_seconds": 30,
        "tx_inclusion_timeout_seconds": 120,
        "indexerd_base_url": "http://127.0.0.1:"
        + os.environ["CFG_INDEXER_LISTEN"].rsplit(":", 1)[1],
        "drand": {
            "disabled": os.environ["CFG_DRAND_DISABLED"] == "true",
            "api_base_url": "https://api.drand.sh",
            "request_timeout_seconds": 10,
            "relay_stagger_seconds": 60,
            "relay_max_delay_seconds": 180,
            "fee_granter": "",
        },
        "submit": {
            "binary": os.path.join(os.environ["CFG_BIN_DIR"], "content-grid-d"),
            "chain_id": os.environ["CFG_CHAIN_ID"],
            "node": "tcp://127.0.0.1:26657",
            "from": os.environ["CFG_VERIFIER_KEY_NAME"],
            "keyring_backend": "file",
            "keyring_dir": os.path.join(
                os.environ["CFG_HOME_DIR"], "keyrings", "verifier"
            ),
            "keyring_passphrase_env": "CONGRID_VERIFIER_KEYRING_PASSPHRASE",
            "home": os.environ["CFG_HOME_DIR"],
            "gas": "250000",
            "gas_adjustment": 1.0,
            "fees": "",
            "gas_prices": os.environ["CFG_VERIFIER_GAS_PRICES"],
            "fee_granter": "",
            "broadcast_mode": "sync",
            "yes": True,
        },
    },
)
write_json(
    os.environ["OUT_STATE"],
    {
        "installer_version": os.environ["CFG_INSTALLER_VERSION"],
        "bundle_url": os.environ["CFG_BUNDLE_URL"],
        "bundle_sha256": os.environ["CFG_BUNDLE_SHA256"],
        "moniker": os.environ["CFG_MONIKER"],
        "chain_id": os.environ["CFG_CHAIN_ID"],
        "genesis_url": os.environ["CFG_GENESIS_URL"],
        "p2p_seeds": os.environ["CFG_P2P_SEEDS"],
        "seeds_url": os.environ["CFG_SEEDS_URL"],
        "persistent_peers": os.environ["CFG_PERSISTENT_PEERS"],
        "p2p_external_address": os.environ["CFG_P2P_EXTERNAL_ADDRESS"],
        "p2p_addr_book_strict": os.environ["CFG_P2P_ADDR_BOOK_STRICT"] == "true",
        "indexer_listen_addr": os.environ["CFG_INDEXER_LISTEN"],
        "verifier_listen_addr": os.environ["CFG_VERIFIER_LISTEN"],
        "verifier_key_name": os.environ["CFG_VERIFIER_KEY_NAME"],
        "verifier_address": os.environ["CFG_VERIFIER_ADDRESS"],
        "verifier_gas_prices": os.environ["CFG_VERIFIER_GAS_PRICES"],
        "drand_delivery_disabled": os.environ["CFG_DRAND_DISABLED"] == "true",
    },
)
PY

printf '%s' "$VERIFIER_PASSPHRASE" >"$TMP_WORK/verifier.passphrase"
chmod 600 "$TMP_WORK/verifier.passphrase"
if [ "$HOST_OS" = "linux" ]; then
  run_root install -o root -g "$SERVICE_GROUP" -m 0640 \
    "$INDEXER_CONFIG_TMP" "$CONFIG_DIR/indexerd.json"
  run_root install -o root -g "$SERVICE_GROUP" -m 0640 \
    "$VERIFIER_CONFIG_TMP" "$CONFIG_DIR/verifierd.json"
  run_root install -o root -g "$SERVICE_GROUP" -m 0640 \
    "$STATE_CONFIG_TMP" "$CONFIG_DIR/install-state.json"
  run_root install -o root -g "$SERVICE_GROUP" -m 0640 \
    "$TMP_WORK/verifier.passphrase" "$CONFIG_DIR/verifier.passphrase"
  run_root chown -R "$SERVICE_USER:$SERVICE_GROUP" "$CONGRID_HOME_DIR"
else
  install -m 0600 "$INDEXER_CONFIG_TMP" "$CONFIG_DIR/indexerd.json"
  install -m 0600 "$VERIFIER_CONFIG_TMP" "$CONFIG_DIR/verifierd.json"
  install -m 0600 "$STATE_CONFIG_TMP" "$CONFIG_DIR/install-state.json"
  install -m 0600 "$TMP_WORK/verifier.passphrase" "$CONFIG_DIR/verifier.passphrase"
fi

cat >"$TMP_WORK/verifierd-launcher" <<'SH'
#!/usr/bin/env bash
set -euo pipefail

passphrase_file="${1:?passphrase file required}"
verifier_binary="${2:?verifierd binary required}"
config_file="${3:?verifierd config required}"
indexer_port="${4:-}"

if [ -n "$indexer_port" ]; then
  helper_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
  "$helper_dir/wait-for-tcp" 127.0.0.1 9090 120
  "$helper_dir/wait-for-http" "http://127.0.0.1:${indexer_port}/healthz" 120
fi

CONGRID_VERIFIER_KEYRING_PASSPHRASE="$(<"$passphrase_file")"
export CONGRID_VERIFIER_KEYRING_PASSPHRASE

exec "$verifier_binary" --config "$config_file"
SH
chmod 0755 "$TMP_WORK/verifierd-launcher"
run_root install -m 0755 "$TMP_WORK/verifierd-launcher" "$LIBEXEC_DIR/verifierd-launcher"

cat >"$TMP_WORK/wait-for-http" <<'SH'
#!/usr/bin/env bash
set -euo pipefail

url="${1:?health URL required}"
timeout_seconds="${2:-120}"
deadline=$(( $(date +%s) + timeout_seconds ))

while [ "$(date +%s)" -le "$deadline" ]; do
  if curl --fail --silent --show-error --max-time 3 "$url" >/dev/null 2>&1; then
    exit 0
  fi
  sleep 2
done

printf 'timed out waiting for %s\n' "$url" >&2
exit 1
SH

cat >"$TMP_WORK/wait-for-tcp" <<'SH'
#!/usr/bin/env bash
set -euo pipefail

host="${1:?host required}"
port="${2:?port required}"
timeout_seconds="${3:-120}"
deadline=$(( $(date +%s) + timeout_seconds ))

while [ "$(date +%s)" -le "$deadline" ]; do
  if (echo >/dev/tcp/"$host"/"$port") >/dev/null 2>&1; then
    exit 0
  fi
  sleep 2
done

printf 'timed out waiting for %s:%s\n' "$host" "$port" >&2
exit 1
SH

chmod 0755 "$TMP_WORK/wait-for-http" "$TMP_WORK/wait-for-tcp"
run_root install -m 0755 "$TMP_WORK/wait-for-http" "$LIBEXEC_DIR/wait-for-http"
run_root install -m 0755 "$TMP_WORK/wait-for-tcp" "$LIBEXEC_DIR/wait-for-tcp"

cat >"$TMP_WORK/indexerd-launcher" <<'SH'
#!/usr/bin/env bash
set -euo pipefail

indexer_binary="${1:?indexerd binary required}"
config_file="${2:?indexerd config required}"
helper_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"

"$helper_dir/wait-for-tcp" 127.0.0.1 9090 120
"$helper_dir/wait-for-http" http://127.0.0.1:8000/healthz 120

exec "$indexer_binary" --config "$config_file"
SH
chmod 0755 "$TMP_WORK/indexerd-launcher"
run_root install -m 0755 "$TMP_WORK/indexerd-launcher" "$LIBEXEC_DIR/indexerd-launcher"

if [ "$HOST_OS" = "linux" ]; then
cat >"$TMP_WORK/congrid-node.service" <<'UNIT'
[Unit]
Description=Content Grid Chain node
Documentation=https://congrid.net/
Wants=network-online.target
After=network-online.target
StartLimitIntervalSec=0

[Service]
Type=simple
User=congrid
Group=congrid
WorkingDirectory=/var/lib/congrid
ExecStart=/usr/local/bin/content-grid-d start --home /var/lib/congrid
Restart=on-failure
RestartSec=5s
LimitNOFILE=65535
UMask=0027
NoNewPrivileges=true
PrivateTmp=true
ProtectHome=true
ProtectSystem=strict
ReadWritePaths=/var/lib/congrid

[Install]
WantedBy=multi-user.target
UNIT

cat >"$TMP_WORK/congrid-chroma.service" <<'UNIT'
[Unit]
Description=Content Grid Chroma embedding service
Documentation=https://congrid.net/
Wants=network-online.target
After=network-online.target
StartLimitIntervalSec=0

[Service]
Type=simple
User=congrid
Group=congrid
WorkingDirectory=/var/lib/congrid/chroma
Environment=HOME=/var/lib/congrid/chroma
Environment=CHROMA_PATH=/var/lib/congrid/chroma/data
Environment=CHROMA_HOST=127.0.0.1
Environment=CHROMA_PORT=8000
ExecStart=/opt/congrid/chromad/.venv/bin/python /opt/congrid/chromad/server.py
Restart=on-failure
RestartSec=5s
UMask=0027
NoNewPrivileges=true
PrivateTmp=true
ProtectHome=true
ProtectSystem=strict
ReadWritePaths=/var/lib/congrid/chroma

[Install]
WantedBy=multi-user.target
UNIT

cat >"$TMP_WORK/congrid-indexer.service" <<'UNIT'
[Unit]
Description=Content Grid publisher indexer
Documentation=https://congrid.net/
Wants=network-online.target congrid-node.service congrid-chroma.service
After=network-online.target congrid-node.service congrid-chroma.service
StartLimitIntervalSec=0

[Service]
Type=simple
User=congrid
Group=congrid
WorkingDirectory=/var/lib/congrid
ExecStartPre=/usr/local/libexec/congrid/wait-for-tcp 127.0.0.1 9090 120
ExecStartPre=/usr/local/libexec/congrid/wait-for-http http://127.0.0.1:8000/healthz 120
ExecStart=/usr/local/bin/indexerd --config /etc/congrid/indexerd.json
Restart=on-failure
RestartSec=5s
UMask=0027
NoNewPrivileges=true
PrivateTmp=true
ProtectHome=true
ProtectSystem=strict
ReadWritePaths=/var/lib/congrid

[Install]
WantedBy=multi-user.target
UNIT

cat >"$TMP_WORK/congrid-verifier.service" <<UNIT
[Unit]
Description=Content Grid verifier agent
Documentation=https://congrid.net/
Wants=network-online.target congrid-node.service congrid-indexer.service
After=network-online.target congrid-node.service congrid-indexer.service
StartLimitIntervalSec=0

[Service]
Type=simple
User=congrid
Group=congrid
WorkingDirectory=/var/lib/congrid
ExecStartPre=/usr/local/libexec/congrid/wait-for-tcp 127.0.0.1 9090 120
ExecStartPre=/usr/local/libexec/congrid/wait-for-http http://127.0.0.1:${INDEXER_LOCAL_PORT}/healthz 120
ExecStart=/usr/local/libexec/congrid/verifierd-launcher /etc/congrid/verifier.passphrase /usr/local/bin/verifierd /etc/congrid/verifierd.json
Restart=on-failure
RestartSec=5s
UMask=0027
NoNewPrivileges=true
PrivateTmp=true
ProtectHome=true
ProtectSystem=strict
ReadWritePaths=/var/lib/congrid

[Install]
WantedBy=multi-user.target
UNIT

for unit_name in congrid-node congrid-chroma congrid-indexer congrid-verifier; do
  run_root install -o root -g root -m 0644 \
    "$TMP_WORK/$unit_name.service" "$SYSTEMD_DIR/$unit_name.service"
done

run_root systemctl daemon-reload
run_root systemctl enable \
  congrid-node.service \
  congrid-chroma.service \
  congrid-indexer.service \
  congrid-verifier.service >/dev/null

if [ "$START_SERVICES" = "true" ]; then
  log "starting Content Grid services"
  run_root systemctl restart congrid-node.service congrid-chroma.service
  run_root systemctl restart congrid-indexer.service congrid-verifier.service
  sleep 3

  failed_services=()
  for service_name in congrid-node congrid-chroma congrid-indexer congrid-verifier; do
    if ! run_root systemctl is-active --quiet "$service_name.service"; then
      failed_services+=("$service_name")
    fi
  done
  if [ "${#failed_services[@]}" -gt 0 ]; then
    for service_name in "${failed_services[@]}"; do
      log "recent logs for failed service $service_name:"
      run_root journalctl -u "$service_name.service" -n 30 --no-pager >&2 || true
    done
    die "one or more services failed to start: ${failed_services[*]}"
  fi
fi
else
  env \
    PLIST_DIR="$LAUNCHD_DIR" \
    CFG_LOGIN_HOME="$SERVICE_LOGIN_HOME" \
    CFG_HOME_DIR="$CONGRID_HOME_DIR" \
    CFG_CONFIG_DIR="$CONFIG_DIR" \
    CFG_CHROMAD_DIR="$CHROMAD_DIR" \
    CFG_BIN_DIR="$BIN_DIR" \
    CFG_LIBEXEC_DIR="$LIBEXEC_DIR" \
    CFG_INDEXER_PORT="$INDEXER_LOCAL_PORT" \
    python3 - <<'PY'
import os
import plistlib

plist_dir = os.environ["PLIST_DIR"]
login_home = os.environ["CFG_LOGIN_HOME"]
home = os.environ["CFG_HOME_DIR"]
config = os.environ["CFG_CONFIG_DIR"]
chromad = os.environ["CFG_CHROMAD_DIR"]
binary = os.environ["CFG_BIN_DIR"]
libexec = os.environ["CFG_LIBEXEC_DIR"]
indexer_port = os.environ["CFG_INDEXER_PORT"]
logs = os.path.join(home, "logs")


def service(label, arguments, working_directory, stdout_name, environment=None):
    value = {
        "Label": label,
        "ProgramArguments": arguments,
        "WorkingDirectory": working_directory,
        "RunAtLoad": True,
        "KeepAlive": True,
        "ThrottleInterval": 5,
        "ProcessType": "Background",
        "StandardOutPath": os.path.join(logs, stdout_name + ".log"),
        "StandardErrorPath": os.path.join(logs, stdout_name + ".log"),
        "EnvironmentVariables": {"HOME": login_home},
    }
    if environment:
        value["EnvironmentVariables"].update(environment)
    return value


services = {
    "net.congrid.node": service(
        "net.congrid.node",
        [os.path.join(binary, "content-grid-d"), "start", "--home", home],
        home,
        "node",
    ),
    "net.congrid.chroma": service(
        "net.congrid.chroma",
        [
            os.path.join(chromad, ".venv", "bin", "python"),
            os.path.join(chromad, "server.py"),
        ],
        os.path.join(home, "chroma"),
        "chroma",
        {
            "HOME": os.path.join(home, "chroma"),
            "CHROMA_PATH": os.path.join(home, "chroma", "data"),
            "CHROMA_HOST": "127.0.0.1",
            "CHROMA_PORT": "8000",
        },
    ),
    "net.congrid.indexer": service(
        "net.congrid.indexer",
        [
            os.path.join(libexec, "indexerd-launcher"),
            os.path.join(binary, "indexerd"),
            os.path.join(config, "indexerd.json"),
        ],
        home,
        "indexer",
    ),
    "net.congrid.verifier": service(
        "net.congrid.verifier",
        [
            os.path.join(libexec, "verifierd-launcher"),
            os.path.join(config, "verifier.passphrase"),
            os.path.join(binary, "verifierd"),
            os.path.join(config, "verifierd.json"),
            indexer_port,
        ],
        home,
        "verifier",
    ),
}

os.makedirs(plist_dir, exist_ok=True)
for label, value in services.items():
    path = os.path.join(plist_dir, label + ".plist")
    temporary = path + ".tmp"
    with open(temporary, "wb") as handle:
        plistlib.dump(value, handle, fmt=plistlib.FMT_XML, sort_keys=True)
    os.chmod(temporary, 0o644)
    os.replace(temporary, path)
PY

  if [ "$START_SERVICES" = "true" ]; then
    launch_uid="$(id -u)"
    if launchctl print "gui/$launch_uid" >/dev/null 2>&1; then
      LAUNCH_DOMAIN="gui/$launch_uid"
    else
      LAUNCH_DOMAIN="user/$launch_uid"
    fi
    log "loading Content Grid launchd services in $LAUNCH_DOMAIN"
    for launch_label in \
      net.congrid.node \
      net.congrid.chroma \
      net.congrid.indexer \
      net.congrid.verifier; do
      launch_plist="$LAUNCHD_DIR/$launch_label.plist"
      launchctl bootout "$LAUNCH_DOMAIN" "$launch_plist" >/dev/null 2>&1 || true
      launchctl bootstrap "$LAUNCH_DOMAIN" "$launch_plist"
      launchctl enable "$LAUNCH_DOMAIN/$launch_label"
    done

    launchctl kickstart -k "$LAUNCH_DOMAIN/net.congrid.node"
    launchctl kickstart -k "$LAUNCH_DOMAIN/net.congrid.chroma"
    launchctl kickstart -k "$LAUNCH_DOMAIN/net.congrid.indexer"
    launchctl kickstart -k "$LAUNCH_DOMAIN/net.congrid.verifier"
    sleep 3

    failed_services=()
    for launch_label in \
      net.congrid.node \
      net.congrid.chroma \
      net.congrid.indexer \
      net.congrid.verifier; do
      if ! launchctl print "$LAUNCH_DOMAIN/$launch_label" 2>/dev/null |
        grep -q 'state = running'; then
        failed_services+=("$launch_label")
      fi
    done
    if [ "${#failed_services[@]}" -gt 0 ]; then
      for launch_label in "${failed_services[@]}"; do
        log "recent log for failed service $launch_label:"
        tail -n 30 "$CONGRID_HOME_DIR/logs/${launch_label##*.}.log" >&2 || true
      done
      die "one or more launchd services failed to start: ${failed_services[*]}"
    fi
  fi
fi

unset VERIFIER_PASSPHRASE CONGRID_VERIFIER_KEYRING_PASSPHRASE

printf '\nContent Grid native operator installation is complete.\n'
printf 'Verifier address: %s\n' "$key_address"
if [ -n "$P2P_SEEDS" ]; then
  printf 'P2P seeds:        %s\n' "$P2P_SEEDS"
fi
if [ -n "$SEEDS_SOURCE_URL" ]; then
  printf 'Seed-list URL:    %s\n' "$SEEDS_SOURCE_URL"
fi
printf 'Node home:       %s\n' "$CONGRID_HOME_DIR"
printf 'Configuration:   %s\n' "$CONFIG_DIR"
if [ "$START_SERVICES" = "true" ]; then
  printf 'Services:        content-grid-d, chromad, indexerd, verifierd are running\n'
else
  printf 'Services:        installed and enabled, but not started (--no-start)\n'
fi
printf '\nUseful commands:\n'
if [ "$HOST_OS" = "linux" ]; then
  printf '  sudo systemctl status congrid-node congrid-chroma congrid-indexer congrid-verifier\n'
  printf '  sudo journalctl -u congrid-verifier -f\n'
else
  printf '  launchctl print %s/net.congrid.node\n' "${LAUNCH_DOMAIN:-gui/$(id -u)}"
  printf '  tail -f %s/logs/verifier.log\n' "$CONGRID_HOME_DIR"
fi
printf '  %s/content-grid-d status --node tcp://127.0.0.1:26657\n' "$BIN_DIR"
printf '\nBefore verifier assignments can run, fund and bond the verifier address.\n'
printf 'Also allow inbound TCP/26656 in the host/cloud firewall for P2P connectivity.\n'
