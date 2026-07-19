#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd -P)"

ENV_FILE=""
CMD="start"

usage() {
  cat <<'USAGE'
Usage:
  scripts/verifier-oneclick.sh [--env FILE] [command]

Commands:
  start       Build/configure, then start verifierd in the background (default)
  foreground  Build/configure, then run verifierd in the foreground
  once        Build/configure, then run one verifierd poll
  install     Build binaries, ensure key, and render verifierd config
  restart     Stop an existing background verifierd, then start it
  stop        Stop the background verifierd started by this script
  status      Show background verifierd status
  logs        Tail verifierd logs
  address     Print the verifier account address
  bond        Submit a verifier bond transaction
  config      Render verifierd config only

Common environment:
  CONGRID_CHAIN_ID=congrid-main
  CONGRID_NODE_RPC_URL=tcp://127.0.0.1:26657
  CONGRID_NODE_GRPC_ADDR=127.0.0.1:9090
  CONGRID_HOME=$HOME/.content-grid

  CONGRID_VERIFIER_KEY_NAME=verifier-key
  CONGRID_VERIFIER_KEYRING_BACKEND=file
  CONGRID_VERIFIER_KEY_MNEMONIC='word ...'
  CONGRID_VERIFIER_KEY_MNEMONIC_FILE=/path/to/mnemonic
  CONGRID_VERIFIER_KEYRING_PASSPHRASE='...'
  CONGRID_VERIFIER_KEYRING_PASSPHRASE_FILE=/path/to/passphrase

  CONGRID_VERIFIER_BOND_AMOUNT=1000000
  CONGRID_VERIFIER_BOND_DENOM=ucongrid

Example:
  CONGRID_CHAIN_ID=congrid-main \
  CONGRID_NODE_RPC_URL=tcp://127.0.0.1:26657 \
  CONGRID_NODE_GRPC_ADDR=127.0.0.1:9090 \
  CONGRID_VERIFIER_KEY_MNEMONIC_FILE=$HOME/congrid-verifier.mnemonic \
  CONGRID_VERIFIER_KEYRING_PASSPHRASE_FILE=$HOME/congrid-verifier.pass \
  scripts/verifier-oneclick.sh start
USAGE
}

log() {
  printf '[congrid-verifier] %s\n' "$*" >&2
}

die() {
  log "error: $*"
  exit 1
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --env)
      [ "$#" -ge 2 ] || die "--env requires a file path"
      ENV_FILE="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    start|foreground|once|install|restart|stop|status|logs|address|bond|config)
      CMD="$1"
      shift
      ;;
    *)
      die "unknown argument: $1"
      ;;
  esac
done

if [ -n "$ENV_FILE" ]; then
  [ -f "$ENV_FILE" ] || die "env file not found: $ENV_FILE"
  set -a
  # shellcheck disable=SC1090
  . "$ENV_FILE"
  set +a
fi

: "${CONGRID_BIN_DIR:=$ROOT_DIR/bin}"
: "${CONTENT_GRID_BIN:=$CONGRID_BIN_DIR/content-grid-d}"
: "${VERIFIERD_BIN:=$CONGRID_BIN_DIR/verifierd}"
: "${CONGRID_SKIP_BUILD:=false}"
: "${CONGRID_FORCE_BUILD:=false}"

: "${CONGRID_HOME:=$HOME/.content-grid}"
: "${CONGRID_CHAIN_ID:=congrid-main}"
: "${CONGRID_NODE_RPC_URL:=tcp://127.0.0.1:26657}"
: "${CONGRID_NODE_GRPC_ADDR:=127.0.0.1:9090}"
: "${CONGRID_WAIT_FOR_NODE:=true}"
: "${CONGRID_WAIT_TIMEOUT_SECONDS:=60}"

: "${CONGRID_VERIFIER_CONFIG:=$ROOT_DIR/offchain/verifierd/config.json}"
: "${CONGRID_VERIFIER_LISTEN_ADDR:=127.0.0.1:9200}"
: "${CONGRID_VERIFIER_STATE_DIR:=$CONGRID_HOME/verifierd-state}"
: "${CONGRID_VERIFIER_KEY_NAME:=verifier-key}"
: "${CONGRID_VERIFIER_KEYRING_BACKEND:=file}"
: "${CONGRID_VERIFIER_KEYRING_DIR:=}"
: "${CONGRID_VERIFIER_HOME:=$CONGRID_HOME}"
: "${CONGRID_VERIFIER_ADDRESS:=}"
: "${CONGRID_CREATE_VERIFIER_KEY:=true}"
: "${CONGRID_VERIFIER_CREATED_KEY_FILE:=$ROOT_DIR/logs/$CONGRID_VERIFIER_KEY_NAME.created-key.json}"

: "${CONGRID_VERIFIER_VERIFY_SCHEME:=https}"
: "${CONGRID_VERIFIER_POLL_INTERVAL_SECONDS:=15}"
: "${CONGRID_VERIFIER_COMMIT_START_BUFFER_SECONDS:=15}"
: "${CONGRID_VERIFIER_COMMIT_WINDOW_SECONDS:=300}"
: "${CONGRID_VERIFIER_ROUND_INTERVAL_SECONDS:=3600}"
: "${CONGRID_VERIFIER_ASSIGNMENT_DELAY_MAX_SECONDS:=3600}"
: "${CONGRID_VERIFIER_DISABLE_ASSIGNMENT_CHECK:=false}"
: "${CONGRID_VERIFIER_RETRY_BACKOFF_SECONDS:=30}"
: "${CONGRID_VERIFIER_TX_INCLUSION_TIMEOUT_SECONDS:=120}"
: "${CONGRID_INDEXERD_BASE_URL:=}"
: "${CONGRID_DRAND_DELIVERY_DISABLED:=false}"
: "${CONGRID_DRAND_API_BASE_URL:=https://api.drand.sh}"
: "${CONGRID_DRAND_REQUEST_TIMEOUT_SECONDS:=10}"
: "${CONGRID_DRAND_RELAY_STAGGER_SECONDS:=60}"
: "${CONGRID_DRAND_RELAY_MAX_DELAY_SECONDS:=180}"
: "${CONGRID_DRAND_FEE_GRANTER:=}"

: "${CONGRID_VERIFIER_GAS:=250000}"
: "${CONGRID_VERIFIER_GAS_ADJUSTMENT:=1}"
: "${CONGRID_VERIFIER_FEES:=}"
: "${CONGRID_VERIFIER_GAS_PRICES:=0.001ucongrid}"
: "${CONGRID_VERIFIER_FEE_GRANTER:=}"
: "${CONGRID_VERIFIER_BROADCAST_MODE:=sync}"
: "${CONGRID_VERIFIER_BOND_AMOUNT:=}"
: "${CONGRID_VERIFIER_BOND_DENOM:=ucongrid}"

: "${CONGRID_LOG_DIR:=$ROOT_DIR/logs}"
: "${CONGRID_VERIFIER_LOG:=$CONGRID_LOG_DIR/verifierd.out}"
: "${CONGRID_VERIFIER_PID_FILE:=$CONGRID_LOG_DIR/verifierd.pid}"

command_exists() {
  command -v "$1" >/dev/null 2>&1
}

load_env_or_file() {
  local name="$1"
  local file_name="${name}_FILE"
  local value="${!name-}"
  local file_value="${!file_name-}"

  if [ -n "$value" ] && [ -n "$file_value" ]; then
    die "set either $name or $file_name, not both"
  fi
  if [ -z "$file_value" ]; then
    return
  fi
  [ -f "$file_value" ] || die "$file_name points to a missing file: $file_value"
  value="$(cat "$file_value")"
  value="${value%$'\n'}"
  export "$name=$value"
}

json_escape() {
  local s="$1"
  s="${s//\\/\\\\}"
  s="${s//\"/\\\"}"
  s="${s//$'\n'/\\n}"
  s="${s//$'\r'/}"
  s="${s//$'\t'/\\t}"
  printf '%s' "$s"
}

json_bool() {
  local name="$1"
  local value="$2"
  case "$value" in
    true|false)
      printf '%s' "$value"
      ;;
    1)
      printf 'true'
      ;;
    0)
      printf 'false'
      ;;
    *)
      die "$name must be true or false"
      ;;
  esac
}

ensure_supported_os() {
  case "$(uname -s)" in
    Linux|Darwin)
      return
      ;;
    *)
      die "unsupported OS: $(uname -s) (Linux and macOS are supported)"
      ;;
  esac
}

ensure_go() {
  if [ "$CONGRID_SKIP_BUILD" = "true" ]; then
    return
  fi
  command_exists go || die "go is required to build binaries; install Go 1.25+ or set CONGRID_SKIP_BUILD=true with existing binaries"
}

need_build() {
  [ "$CONGRID_FORCE_BUILD" = "true" ] && return 0
  [ ! -x "$CONTENT_GRID_BIN" ] && return 0
  [ ! -x "$VERIFIERD_BIN" ] && return 0
  return 1
}

build_binaries() {
  if [ "$CONGRID_SKIP_BUILD" = "true" ]; then
    [ -x "$CONTENT_GRID_BIN" ] || die "CONTENT_GRID_BIN is not executable: $CONTENT_GRID_BIN"
    [ -x "$VERIFIERD_BIN" ] || die "VERIFIERD_BIN is not executable: $VERIFIERD_BIN"
    return
  fi

  if ! need_build; then
    return
  fi

  mkdir -p "$CONGRID_BIN_DIR" "$ROOT_DIR/.gocache"
  log "building content-grid-d -> $CONTENT_GRID_BIN"
  (cd "$ROOT_DIR" && GOCACHE="$ROOT_DIR/.gocache" go build -o "$CONTENT_GRID_BIN" ./cmd/content-grid-d)
  log "building verifierd -> $VERIFIERD_BIN"
  (cd "$ROOT_DIR" && GOCACHE="$ROOT_DIR/.gocache" go build -o "$VERIFIERD_BIN" ./offchain/verifierd)
}

run_content_grid_with_passphrase() {
  local backend="$1"
  local passphrase="$2"
  shift 2

  if [ "$backend" = "file" ]; then
    [ -n "$passphrase" ] || die "keyring backend=file requires CONGRID_VERIFIER_KEYRING_PASSPHRASE or CONGRID_VERIFIER_KEYRING_PASSPHRASE_FILE"
    printf '%s\n' "$passphrase" | "$CONTENT_GRID_BIN" "$@"
    return
  fi

  "$CONTENT_GRID_BIN" "$@"
}

prompt_passphrase_if_needed() {
  if [ "$CONGRID_VERIFIER_KEYRING_BACKEND" != "file" ]; then
    return
  fi
  if [ -n "${CONGRID_VERIFIER_KEYRING_PASSPHRASE:-}" ]; then
    return
  fi
  if [ ! -t 0 ]; then
    die "file keyring needs CONGRID_VERIFIER_KEYRING_PASSPHRASE or CONGRID_VERIFIER_KEYRING_PASSPHRASE_FILE in non-interactive mode"
  fi
  printf 'Keyring passphrase for %s: ' "$CONGRID_VERIFIER_KEY_NAME" >&2
  stty -echo
  IFS= read -r CONGRID_VERIFIER_KEYRING_PASSPHRASE
  stty echo
  printf '\n' >&2
  export CONGRID_VERIFIER_KEYRING_PASSPHRASE
}

key_exists() {
  local name="$1"
  local -a args
  args=(keys show "$name" --address --home "$CONGRID_VERIFIER_HOME" --keyring-backend "$CONGRID_VERIFIER_KEYRING_BACKEND")
  if [ -n "$CONGRID_VERIFIER_KEYRING_DIR" ]; then
    args+=(--keyring-dir "$CONGRID_VERIFIER_KEYRING_DIR")
  fi
  run_content_grid_with_passphrase "$CONGRID_VERIFIER_KEYRING_BACKEND" "${CONGRID_VERIFIER_KEYRING_PASSPHRASE:-}" "${args[@]}" >/dev/null 2>&1
}

show_key_address() {
  local name="$1"
  local -a args
  args=(keys show "$name" --address --home "$CONGRID_VERIFIER_HOME" --keyring-backend "$CONGRID_VERIFIER_KEYRING_BACKEND")
  if [ -n "$CONGRID_VERIFIER_KEYRING_DIR" ]; then
    args+=(--keyring-dir "$CONGRID_VERIFIER_KEYRING_DIR")
  fi
  run_content_grid_with_passphrase "$CONGRID_VERIFIER_KEYRING_BACKEND" "${CONGRID_VERIFIER_KEYRING_PASSPHRASE:-}" "${args[@]}"
}

recover_key() {
  local name="$1"
  local mnemonic="$2"
  local source_file
  local -a args

  [ -n "$mnemonic" ] || die "mnemonic is empty"
  source_file="$(mktemp)"
  chmod 600 "$source_file"
  printf '%s' "$mnemonic" >"$source_file"

  args=(keys add "$name" --recover --output json --source "$source_file" --home "$CONGRID_VERIFIER_HOME" --keyring-backend "$CONGRID_VERIFIER_KEYRING_BACKEND")
  if [ -n "$CONGRID_VERIFIER_KEYRING_DIR" ]; then
    args+=(--keyring-dir "$CONGRID_VERIFIER_KEYRING_DIR")
  fi

  case "$CONGRID_VERIFIER_KEYRING_BACKEND" in
    file)
      if printf '%s\n' "${CONGRID_VERIFIER_KEYRING_PASSPHRASE:-}" | "$CONTENT_GRID_BIN" "${args[@]}" >/dev/null 2>&1; then
        rm -f "$source_file"
        return
      fi
      if ! printf '%s\n%s\n' "${CONGRID_VERIFIER_KEYRING_PASSPHRASE:-}" "${CONGRID_VERIFIER_KEYRING_PASSPHRASE:-}" | "$CONTENT_GRID_BIN" "${args[@]}" >/dev/null; then
        rm -f "$source_file"
        return 1
      fi
      ;;
    *)
      if ! "$CONTENT_GRID_BIN" "${args[@]}" >/dev/null; then
        rm -f "$source_file"
        return 1
      fi
      ;;
  esac
  rm -f "$source_file"
}

create_key() {
  local name="$1"
  local -a args

  mkdir -p "$(dirname "$CONGRID_VERIFIER_CREATED_KEY_FILE")"
  args=(keys add "$name" --output json --home "$CONGRID_VERIFIER_HOME" --keyring-backend "$CONGRID_VERIFIER_KEYRING_BACKEND")
  if [ -n "$CONGRID_VERIFIER_KEYRING_DIR" ]; then
    args+=(--keyring-dir "$CONGRID_VERIFIER_KEYRING_DIR")
  fi

  log "creating verifier key $name"
  (
    umask 077
    case "$CONGRID_VERIFIER_KEYRING_BACKEND" in
      file)
        if printf '%s\n' "${CONGRID_VERIFIER_KEYRING_PASSPHRASE:-}" | "$CONTENT_GRID_BIN" "${args[@]}" >"$CONGRID_VERIFIER_CREATED_KEY_FILE" 2>"$CONGRID_VERIFIER_CREATED_KEY_FILE.err"; then
          rm -f "$CONGRID_VERIFIER_CREATED_KEY_FILE.err"
          exit 0
        fi
        printf '%s\n%s\n' "${CONGRID_VERIFIER_KEYRING_PASSPHRASE:-}" "${CONGRID_VERIFIER_KEYRING_PASSPHRASE:-}" | "$CONTENT_GRID_BIN" "${args[@]}" >"$CONGRID_VERIFIER_CREATED_KEY_FILE" 2>"$CONGRID_VERIFIER_CREATED_KEY_FILE.err"
        ;;
      *)
        "$CONTENT_GRID_BIN" "${args[@]}" >"$CONGRID_VERIFIER_CREATED_KEY_FILE"
        ;;
    esac
  ) || {
    if [ -s "$CONGRID_VERIFIER_CREATED_KEY_FILE.err" ]; then
      cat "$CONGRID_VERIFIER_CREATED_KEY_FILE.err" >&2
    fi
    rm -f "$CONGRID_VERIFIER_CREATED_KEY_FILE" "$CONGRID_VERIFIER_CREATED_KEY_FILE.err"
    return 1
  }
  rm -f "$CONGRID_VERIFIER_CREATED_KEY_FILE.err"
  chmod 600 "$CONGRID_VERIFIER_CREATED_KEY_FILE"
  log "created key material saved to $CONGRID_VERIFIER_CREATED_KEY_FILE (keep it private)"
}

ensure_key_present() {
  mkdir -p "$CONGRID_VERIFIER_HOME"
  if key_exists "$CONGRID_VERIFIER_KEY_NAME"; then
    return
  fi

  if [ -n "${CONGRID_VERIFIER_KEY_MNEMONIC:-}" ]; then
    log "recovering verifier key $CONGRID_VERIFIER_KEY_NAME"
    recover_key "$CONGRID_VERIFIER_KEY_NAME" "$CONGRID_VERIFIER_KEY_MNEMONIC"
    return
  fi

  if [ "$CONGRID_CREATE_VERIFIER_KEY" = "true" ]; then
    create_key "$CONGRID_VERIFIER_KEY_NAME"
    return
  fi

  die "key $CONGRID_VERIFIER_KEY_NAME not found; set CONGRID_VERIFIER_KEY_MNEMONIC_FILE or CONGRID_CREATE_VERIFIER_KEY=true"
}

resolve_verifier_address() {
  if [ -n "$CONGRID_VERIFIER_ADDRESS" ]; then
    printf '%s' "$CONGRID_VERIFIER_ADDRESS"
    return
  fi
  show_key_address "$CONGRID_VERIFIER_KEY_NAME" | tr -d '[:space:]'
}

render_verifier_config() {
  local verifier_address="$1"
  local keyring_passphrase_env=""
  local disable_assignment_check
  local drand_delivery_disabled
  local tmp

  disable_assignment_check="$(json_bool CONGRID_VERIFIER_DISABLE_ASSIGNMENT_CHECK "$CONGRID_VERIFIER_DISABLE_ASSIGNMENT_CHECK")"
  drand_delivery_disabled="$(json_bool CONGRID_DRAND_DELIVERY_DISABLED "$CONGRID_DRAND_DELIVERY_DISABLED")"
  if [ "$CONGRID_VERIFIER_KEYRING_BACKEND" = "file" ]; then
    keyring_passphrase_env="CONGRID_VERIFIER_KEYRING_PASSPHRASE"
  fi

  mkdir -p "$(dirname "$CONGRID_VERIFIER_CONFIG")" "$CONGRID_VERIFIER_STATE_DIR"
  tmp="$CONGRID_VERIFIER_CONFIG.tmp.$$"
  cat >"$tmp" <<EOF
{
  "grpc_addr": "$(json_escape "$CONGRID_NODE_GRPC_ADDR")",
  "listen_addr": "$(json_escape "$CONGRID_VERIFIER_LISTEN_ADDR")",
  "verifier_address": "$(json_escape "$verifier_address")",
  "state_dir": "$(json_escape "$CONGRID_VERIFIER_STATE_DIR")",
  "poll_interval_seconds": $CONGRID_VERIFIER_POLL_INTERVAL_SECONDS,
  "verify_scheme": "$(json_escape "$CONGRID_VERIFIER_VERIFY_SCHEME")",
  "commit_start_buffer_seconds": $CONGRID_VERIFIER_COMMIT_START_BUFFER_SECONDS,
  "commit_window_seconds": $CONGRID_VERIFIER_COMMIT_WINDOW_SECONDS,
  "round_interval_seconds": $CONGRID_VERIFIER_ROUND_INTERVAL_SECONDS,
  "assignment_delay_max_seconds": $CONGRID_VERIFIER_ASSIGNMENT_DELAY_MAX_SECONDS,
  "disable_assignment_check": $disable_assignment_check,
  "retry_backoff_seconds": $CONGRID_VERIFIER_RETRY_BACKOFF_SECONDS,
  "tx_inclusion_timeout_seconds": $CONGRID_VERIFIER_TX_INCLUSION_TIMEOUT_SECONDS,
  "indexerd_base_url": "$(json_escape "$CONGRID_INDEXERD_BASE_URL")",
  "drand": {
    "disabled": $drand_delivery_disabled,
    "api_base_url": "$(json_escape "$CONGRID_DRAND_API_BASE_URL")",
    "request_timeout_seconds": $CONGRID_DRAND_REQUEST_TIMEOUT_SECONDS,
    "relay_stagger_seconds": $CONGRID_DRAND_RELAY_STAGGER_SECONDS,
    "relay_max_delay_seconds": $CONGRID_DRAND_RELAY_MAX_DELAY_SECONDS,
    "fee_granter": "$(json_escape "$CONGRID_DRAND_FEE_GRANTER")"
  },
  "submit": {
    "binary": "$(json_escape "$CONTENT_GRID_BIN")",
    "chain_id": "$(json_escape "$CONGRID_CHAIN_ID")",
    "node": "$(json_escape "$CONGRID_NODE_RPC_URL")",
    "from": "$(json_escape "$CONGRID_VERIFIER_KEY_NAME")",
    "keyring_backend": "$(json_escape "$CONGRID_VERIFIER_KEYRING_BACKEND")",
    "keyring_dir": "$(json_escape "$CONGRID_VERIFIER_KEYRING_DIR")",
    "keyring_passphrase_env": "$(json_escape "$keyring_passphrase_env")",
    "home": "$(json_escape "$CONGRID_VERIFIER_HOME")",
    "gas": "$(json_escape "$CONGRID_VERIFIER_GAS")",
    "gas_adjustment": $CONGRID_VERIFIER_GAS_ADJUSTMENT,
    "fees": "$(json_escape "$CONGRID_VERIFIER_FEES")",
    "gas_prices": "$(json_escape "$CONGRID_VERIFIER_GAS_PRICES")",
    "fee_granter": "$(json_escape "$CONGRID_VERIFIER_FEE_GRANTER")",
    "broadcast_mode": "$(json_escape "$CONGRID_VERIFIER_BROADCAST_MODE")",
    "yes": true
  }
}
EOF
  mv "$tmp" "$CONGRID_VERIFIER_CONFIG"
  log "rendered verifierd config: $CONGRID_VERIFIER_CONFIG"
}

host_port_from_addr() {
  local raw="$1"
  raw="${raw#tcp://}"
  raw="${raw#http://}"
  raw="${raw#https://}"
  raw="${raw%%/*}"
  printf '%s' "$raw"
}

tcp_ready() {
  local host="$1"
  local port="$2"
  if command_exists nc; then
    nc -z "$host" "$port" >/dev/null 2>&1
    return $?
  fi
  (echo >/dev/tcp/"$host"/"$port") >/dev/null 2>&1
}

diagnose_node_wait_failure() {
  local grpc_host="$1"
  local grpc_port="$2"
  local rpc_hp rpc_host rpc_port app_toml grpc_enable grpc_address

  log "diagnostic: verifier-oneclick does not start content-grid-d; it expects an already running node"

  rpc_hp="$(host_port_from_addr "$CONGRID_NODE_RPC_URL")"
  rpc_host="${rpc_hp%:*}"
  rpc_port="${rpc_hp##*:}"
  if [ -n "$rpc_host" ] && [ -n "$rpc_port" ] && [ "$rpc_host" != "$rpc_port" ]; then
    if tcp_ready "$rpc_host" "$rpc_port"; then
      log "diagnostic: node RPC is reachable at $rpc_host:$rpc_port"
    else
      log "diagnostic: node RPC is also not reachable at $rpc_host:$rpc_port"
    fi
  fi

  app_toml="$CONGRID_HOME/config/app.toml"
  if [ -f "$app_toml" ]; then
    grpc_enable="$(awk '
      /^\[grpc\]/ { in_grpc=1; next }
      /^\[/ { in_grpc=0 }
      in_grpc && /^[[:space:]]*enable[[:space:]]*=/ { print $0 }
    ' "$app_toml" | tail -n 1)"
    grpc_address="$(awk '
      /^\[grpc\]/ { in_grpc=1; next }
      /^\[/ { in_grpc=0 }
      in_grpc && /^[[:space:]]*address[[:space:]]*=/ { print $0 }
    ' "$app_toml" | tail -n 1)"
    [ -z "$grpc_enable" ] || log "diagnostic: $app_toml [grpc] $grpc_enable"
    [ -z "$grpc_address" ] || log "diagnostic: $app_toml [grpc] $grpc_address"
  else
    log "diagnostic: node app config not found at $app_toml; set CONGRID_HOME to the node home if it differs"
  fi

  log "diagnostic: start the node or set CONGRID_NODE_GRPC_ADDR to the reachable endpoint instead of $grpc_host:$grpc_port"
}

wait_for_node() {
  [ "$CONGRID_WAIT_FOR_NODE" = "true" ] || return

  local hp host port deadline
  hp="$(host_port_from_addr "$CONGRID_NODE_GRPC_ADDR")"
  host="${hp%:*}"
  port="${hp##*:}"
  [ -n "$host" ] && [ -n "$port" ] && [ "$host" != "$port" ] || die "cannot parse CONGRID_NODE_GRPC_ADDR: $CONGRID_NODE_GRPC_ADDR"

  log "waiting for chain gRPC $host:$port"
  deadline=$(( $(date +%s) + CONGRID_WAIT_TIMEOUT_SECONDS ))
  while [ "$(date +%s)" -le "$deadline" ]; do
    if tcp_ready "$host" "$port"; then
      return
    fi
    sleep 2
  done
  diagnose_node_wait_failure "$host" "$port"
  die "chain gRPC is not reachable at $host:$port after ${CONGRID_WAIT_TIMEOUT_SECONDS}s"
}

pid_running() {
  local pid="$1"
  [ -n "$pid" ] || return 1
  kill -0 "$pid" >/dev/null 2>&1
}

read_pid() {
  if [ -f "$CONGRID_VERIFIER_PID_FILE" ]; then
    sed -n '1p' "$CONGRID_VERIFIER_PID_FILE"
  fi
}

prepare_runtime() {
  local address
  ensure_supported_os
  load_env_or_file CONGRID_VERIFIER_KEY_MNEMONIC
  load_env_or_file CONGRID_VERIFIER_KEYRING_PASSPHRASE
  ensure_go
  build_binaries
  prompt_passphrase_if_needed
  ensure_key_present
  address="$(resolve_verifier_address)"
  [ -n "$address" ] || die "failed to resolve verifier address"
  render_verifier_config "$address"
  printf '%s' "$address"
}

start_background() {
  local address pid
  address="$(prepare_runtime)"
  wait_for_node
  mkdir -p "$CONGRID_LOG_DIR"

  pid="$(read_pid || true)"
  if pid_running "$pid"; then
    die "verifierd already running with pid $pid"
  fi

  log "starting verifierd in background verifier=$address"
  (
    export CONGRID_VERIFIER_KEYRING_PASSPHRASE="${CONGRID_VERIFIER_KEYRING_PASSPHRASE:-}"
    nohup "$VERIFIERD_BIN" --config "$CONGRID_VERIFIER_CONFIG" >>"$CONGRID_VERIFIER_LOG" 2>&1 &
    printf '%s\n' "$!" >"$CONGRID_VERIFIER_PID_FILE"
  )

  sleep 2
  pid="$(read_pid || true)"
  if ! pid_running "$pid"; then
    log "verifierd exited during startup; last log lines:"
    tail -n 40 "$CONGRID_VERIFIER_LOG" >&2 || true
    exit 1
  fi
  log "verifierd started pid=$pid log=$CONGRID_VERIFIER_LOG"
}

run_foreground() {
  local address
  address="$(prepare_runtime)"
  wait_for_node
  log "starting verifierd in foreground verifier=$address"
  export CONGRID_VERIFIER_KEYRING_PASSPHRASE="${CONGRID_VERIFIER_KEYRING_PASSPHRASE:-}"
  exec "$VERIFIERD_BIN" --config "$CONGRID_VERIFIER_CONFIG"
}

run_once() {
  local address
  address="$(prepare_runtime)"
  wait_for_node
  log "running one verifierd poll verifier=$address"
  export CONGRID_VERIFIER_KEYRING_PASSPHRASE="${CONGRID_VERIFIER_KEYRING_PASSPHRASE:-}"
  "$VERIFIERD_BIN" --config "$CONGRID_VERIFIER_CONFIG" --once
}

stop_background() {
  local pid i
  pid="$(read_pid || true)"
  if ! pid_running "$pid"; then
    rm -f "$CONGRID_VERIFIER_PID_FILE"
    log "verifierd is not running"
    return
  fi

  log "stopping verifierd pid=$pid"
  kill "$pid"
  i=0
  while pid_running "$pid" && [ "$i" -lt 30 ]; do
    sleep 1
    i=$((i + 1))
  done
  if pid_running "$pid"; then
    die "verifierd did not stop within 30s"
  fi
  rm -f "$CONGRID_VERIFIER_PID_FILE"
  log "verifierd stopped"
}

show_status() {
  local pid
  pid="$(read_pid || true)"
  if pid_running "$pid"; then
    printf 'verifierd: running pid=%s\n' "$pid"
    printf 'config: %s\n' "$CONGRID_VERIFIER_CONFIG"
    printf 'log: %s\n' "$CONGRID_VERIFIER_LOG"
    return
  fi
  printf 'verifierd: stopped\n'
  [ ! -f "$CONGRID_VERIFIER_PID_FILE" ] || printf 'stale pid file: %s\n' "$CONGRID_VERIFIER_PID_FILE"
}

tail_logs() {
  mkdir -p "$CONGRID_LOG_DIR"
  touch "$CONGRID_VERIFIER_LOG"
  tail -n 80 -f "$CONGRID_VERIFIER_LOG"
}

bond_verifier() {
  local address
  local -a args
  [ -n "$CONGRID_VERIFIER_BOND_AMOUNT" ] || die "set CONGRID_VERIFIER_BOND_AMOUNT before running bond"

  address="$(prepare_runtime)"
  wait_for_node
  log "bonding verifier=$address amount=${CONGRID_VERIFIER_BOND_AMOUNT}${CONGRID_VERIFIER_BOND_DENOM}"

  args=(verifier bond "$CONGRID_VERIFIER_BOND_AMOUNT"
    --denom "$CONGRID_VERIFIER_BOND_DENOM"
    --from "$CONGRID_VERIFIER_KEY_NAME"
    --chain-id "$CONGRID_CHAIN_ID"
    --node "$CONGRID_NODE_RPC_URL"
    --home "$CONGRID_VERIFIER_HOME"
    --keyring-backend "$CONGRID_VERIFIER_KEYRING_BACKEND"
    --gas "$CONGRID_VERIFIER_GAS"
    --gas-adjustment "$CONGRID_VERIFIER_GAS_ADJUSTMENT"
    --broadcast-mode "$CONGRID_VERIFIER_BROADCAST_MODE"
    --output json
    -y)
  if [ -n "$CONGRID_VERIFIER_KEYRING_DIR" ]; then
    args+=(--keyring-dir "$CONGRID_VERIFIER_KEYRING_DIR")
  fi
  if [ -n "$CONGRID_VERIFIER_FEES" ]; then
    args+=(--fees "$CONGRID_VERIFIER_FEES")
  fi
  if [ -n "$CONGRID_VERIFIER_GAS_PRICES" ]; then
    args+=(--gas-prices "$CONGRID_VERIFIER_GAS_PRICES")
  fi

  run_content_grid_with_passphrase "$CONGRID_VERIFIER_KEYRING_BACKEND" "${CONGRID_VERIFIER_KEYRING_PASSPHRASE:-}" "${args[@]}"
}

case "$CMD" in
  start)
    start_background
    ;;
  foreground)
    run_foreground
    ;;
  once)
    run_once
    ;;
  install)
    address="$(prepare_runtime)"
    log "install complete verifier=$address"
    ;;
  restart)
    stop_background
    start_background
    ;;
  stop)
    stop_background
    ;;
  status)
    show_status
    ;;
  logs)
    tail_logs
    ;;
  address)
    address="$(prepare_runtime)"
    printf '%s\n' "$address"
    ;;
  bond)
    bond_verifier
    ;;
  config)
    address="$(prepare_runtime)"
    log "config ready for verifier=$address"
    ;;
  *)
    die "unknown command: $CMD"
    ;;
esac
