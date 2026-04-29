#!/usr/bin/env bash
set -euo pipefail

: "${CONTENT_GRID_BIN:=/usr/local/bin/content-grid-d}"
: "${CONGRID_HOME:=/var/lib/congrid}"

log() {
  printf '[congrid] %s\n' "$*" >&2
}

die() {
  log "error: $*"
  exit 1
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
  value="$(<"$file_value")"
  value="${value%$'\n'}"
  export "$name=$value"
}

escape_regex() {
  printf '%s' "$1" | sed -e 's/[][(){}.^$*+?|\\/]/\\&/g'
}

escape_sed_replacement() {
  printf '%s' "$1" | sed -e 's/[\\/&]/\\&/g'
}

set_toml_value() {
  local file="$1"
  local section="${2:-}"
  local key="$3"
  local raw_value="$4"
  local key_pattern
  local value

  [ -f "$file" ] || die "missing TOML file: $file"

  key_pattern="$(escape_regex "$key")"
  value="$(escape_sed_replacement "$raw_value")"

  if [ -n "$section" ]; then
    local section_pattern
    section_pattern="$(escape_regex "$section")"
    sed -i -E "/^\[$section_pattern\]/,/^\[/{s|^$key_pattern = .*|$key = $value|;}" "$file"
    return
  fi

  sed -i -E "0,/^$key_pattern = .*/s|^$key_pattern = .*|$key = $value|" "$file"
}

set_toml_string() {
  local file="$1"
  local section="${2:-}"
  local key="$3"
  local value="$4"
  set_toml_value "$file" "$section" "$key" "\"$value\""
}

set_toml_bool() {
  local file="$1"
  local section="${2:-}"
  local key="$3"
  local value="$4"
  set_toml_value "$file" "$section" "$key" "$value"
}

append_keyring_args() {
  local -n out_ref="$1"
  local home="$2"
  local backend="$3"
  local keyring_dir="$4"

  out_ref+=(--home "$home" --keyring-backend "$backend")
  if [ -n "$keyring_dir" ]; then
    out_ref+=(--keyring-dir "$keyring_dir")
  fi
}

run_content_grid_with_passphrase() {
  local backend="$1"
  local passphrase="$2"
  shift 2

  if [ "$backend" = "file" ]; then
    [ -n "$passphrase" ] || die "keyring backend=file requires a passphrase"
    printf '%s\n' "$passphrase" | "$CONTENT_GRID_BIN" "$@"
    return
  fi

  "$CONTENT_GRID_BIN" "$@"
}

ensure_automated_tx_backend() {
  local backend="$1"
  local passphrase="$2"
  local service_name="$3"

  case "$backend" in
    test|memory)
      return
      ;;
    file)
      [ -n "$passphrase" ] || die "$service_name uses keyring backend=file; set the passphrase for unattended signing"
      return
      ;;
    *)
      die "$service_name cannot use keyring backend=$backend in unattended containers; use test or file"
      ;;
  esac
}

key_exists() {
  local name="$1"
  local home="$2"
  local backend="$3"
  local keyring_dir="$4"
  local passphrase="$5"
  local -a args=(keys show "$name" --address)

  append_keyring_args args "$home" "$backend" "$keyring_dir"
  run_content_grid_with_passphrase "$backend" "$passphrase" "${args[@]}" >/dev/null 2>&1
}

show_key_address() {
  local name="$1"
  local home="$2"
  local backend="$3"
  local keyring_dir="$4"
  local passphrase="$5"
  local -a args=(keys show "$name" --address)

  append_keyring_args args "$home" "$backend" "$keyring_dir"
  run_content_grid_with_passphrase "$backend" "$passphrase" "${args[@]}"
}

ensure_key_present() {
  local name="$1"
  local mnemonic="$2"
  local home="$3"
  local backend="$4"
  local keyring_dir="$5"
  local passphrase="$6"
  local -a args=(keys add "$name" --recover --output json)
  local source_file

  if key_exists "$name" "$home" "$backend" "$keyring_dir" "$passphrase"; then
    return
  fi
  [ -n "$mnemonic" ] || die "key $name is missing and no mnemonic was provided"

  append_keyring_args args "$home" "$backend" "$keyring_dir"
  source_file="$(mktemp)"
  printf '%s' "$mnemonic" >"$source_file"
  args+=(--source "$source_file")

  case "$backend" in
    test|memory)
      "$CONTENT_GRID_BIN" "${args[@]}" >/dev/null || {
        rm -f "$source_file"
        return 1
      }
      ;;
    file)
      [ -n "$passphrase" ] || die "keyring backend=file requires a passphrase to import $name"
      if printf '%s\n' "$passphrase" | "$CONTENT_GRID_BIN" "${args[@]}" >/dev/null 2>&1; then
        rm -f "$source_file"
        return
      fi
      printf '%s\n%s\n' "$passphrase" "$passphrase" | "$CONTENT_GRID_BIN" "${args[@]}" >/dev/null || {
        rm -f "$source_file"
        return 1
      }
      ;;
    *)
      rm -f "$source_file"
      die "automated key import is not supported for keyring backend=$backend"
      ;;
  esac
  rm -f "$source_file"
}

ensure_home_initialized() {
  local moniker="$1"
  local chain_id="$2"

  export CONGRID_HOME_WAS_INITIALIZED=0
  if [ -f "$CONGRID_HOME/config/config.toml" ]; then
    return
  fi

  mkdir -p "$CONGRID_HOME"
  log "initializing node home at $CONGRID_HOME"
  "$CONTENT_GRID_BIN" init "$moniker" --home "$CONGRID_HOME" --chain-id "$chain_id" >/dev/null
  export CONGRID_HOME_WAS_INITIALIZED=1
}

install_network_genesis() {
  local genesis_path="$CONGRID_HOME/config/genesis.json"

  mkdir -p "$(dirname "$genesis_path")"

  : "${CONGRID_REFRESH_GENESIS:=false}"
  if [ -s "$genesis_path" ] && [ "${CONGRID_HOME_WAS_INITIALIZED:-0}" != "1" ] && [ "$CONGRID_REFRESH_GENESIS" != "true" ]; then
    return
  fi

  if [ -n "${CONGRID_GENESIS_FILE:-}" ]; then
    [ -f "$CONGRID_GENESIS_FILE" ] || die "CONGRID_GENESIS_FILE points to a missing file: $CONGRID_GENESIS_FILE"
    cp "$CONGRID_GENESIS_FILE" "$genesis_path"
    log "installed genesis from $CONGRID_GENESIS_FILE"
    return
  fi

  if [ -n "${CONGRID_GENESIS_URL:-}" ]; then
    curl -fsSL "$CONGRID_GENESIS_URL" -o "$genesis_path"
    log "downloaded genesis from $CONGRID_GENESIS_URL"
    return
  fi

  if [ "${CONGRID_HOME_WAS_INITIALIZED:-0}" = "1" ]; then
    die "fresh node home requires CONGRID_GENESIS_FILE or CONGRID_GENESIS_URL to join an existing network"
  fi
}

render_validator_json() {
  local out_path="$1"
  local amount="$2"
  local moniker="$3"
  local identity="$4"
  local website="$5"
  local security="$6"
  local details="$7"
  local commission_rate="$8"
  local commission_max_rate="$9"
  local commission_max_change_rate="${10}"
  local min_self_delegation="${11}"
  local pubkey_json

  [ -n "$amount" ] || die "CONGRID_VALIDATOR_AMOUNT is required when CONGRID_VALIDATOR_JSON_ENABLE=true"

  mkdir -p "$(dirname "$out_path")"
  pubkey_json="$("$CONTENT_GRID_BIN" comet show-validator --home "$CONGRID_HOME")"

  jq -n \
    --argjson pubkey "$pubkey_json" \
    --arg amount "$amount" \
    --arg moniker "$moniker" \
    --arg identity "$identity" \
    --arg website "$website" \
    --arg security "$security" \
    --arg details "$details" \
    --arg commission_rate "$commission_rate" \
    --arg commission_max_rate "$commission_max_rate" \
    --arg commission_max_change_rate "$commission_max_change_rate" \
    --arg min_self_delegation "$min_self_delegation" \
    '{
      pubkey: $pubkey,
      amount: $amount,
      moniker: $moniker,
      identity: $identity,
      website: $website,
      security: $security,
      details: $details,
      "commission-rate": $commission_rate,
      "commission-max-rate": $commission_max_rate,
      "commission-max-change-rate": $commission_max_change_rate,
      "min-self-delegation": $min_self_delegation
    }' >"$out_path"

  log "rendered validator.json to $out_path"
}
