#!/usr/bin/env bash
set -euo pipefail

source /usr/local/lib/congrid/common.sh

: "${CONGRID_KEYRING_BACKEND:=file}"
: "${CONGRID_KEYRING_DIR:=}"
: "${CONGRID_CLIENT_NODE:=tcp://127.0.0.1:26657}"
: "${CONGRID_CHAIN_ID:=congrid-main}"
: "${CONGRID_VALIDATOR_JSON_PATH:=$CONGRID_HOME/config/validator.json}"
: "${CONGRID_VALIDATOR_KEY_NAME:=}"
: "${CONGRID_VALIDATOR_KEYRING_DIR:=$CONGRID_KEYRING_DIR}"

usage() {
  cat <<'EOF'
Usage:
  congrid-validator-cli show-config
  congrid-validator-cli show-account-address
  congrid-validator-cli show-valoper-address
  congrid-validator-cli create-validator [validator.json] [extra tx flags...]
EOF
}

require_key_name() {
  [ -n "$CONGRID_VALIDATOR_KEY_NAME" ] || die "set CONGRID_VALIDATOR_KEY_NAME in the node environment"
}

append_validator_keyring_args() {
  local -n out_ref="$1"
  append_keyring_args out_ref "$CONGRID_HOME" "$CONGRID_KEYRING_BACKEND" "$CONGRID_VALIDATOR_KEYRING_DIR"
}

show_config() {
  cat <<EOF
CONGRID_HOME=$CONGRID_HOME
CONGRID_CHAIN_ID=$CONGRID_CHAIN_ID
CONGRID_CLIENT_NODE=$CONGRID_CLIENT_NODE
CONGRID_KEYRING_BACKEND=$CONGRID_KEYRING_BACKEND
CONGRID_VALIDATOR_KEY_NAME=$CONGRID_VALIDATOR_KEY_NAME
CONGRID_VALIDATOR_KEYRING_DIR=$CONGRID_VALIDATOR_KEYRING_DIR
CONGRID_VALIDATOR_JSON_PATH=$CONGRID_VALIDATOR_JSON_PATH
EOF
}

main() {
  local cmd="${1:-}"
  case "$cmd" in
    show-config)
      show_config
      ;;
    show-account-address)
      require_key_name
      local -a args=(keys show "$CONGRID_VALIDATOR_KEY_NAME" -a)
      append_validator_keyring_args args
      exec "$CONTENT_GRID_BIN" "${args[@]}"
      ;;
    show-valoper-address)
      require_key_name
      local -a args=(keys show "$CONGRID_VALIDATOR_KEY_NAME" --bech val -a)
      append_validator_keyring_args args
      exec "$CONTENT_GRID_BIN" "${args[@]}"
      ;;
    create-validator)
      require_key_name
      shift || true
      local manifest="$CONGRID_VALIDATOR_JSON_PATH"
      if [ $# -gt 0 ] && [[ "$1" != -* ]]; then
        manifest="$1"
        shift
      fi
      local -a args=(
        tx staking create-validator "$manifest"
        --from "$CONGRID_VALIDATOR_KEY_NAME"
        --chain-id "$CONGRID_CHAIN_ID"
        --node "$CONGRID_CLIENT_NODE"
      )
      append_validator_keyring_args args
      if [ $# -gt 0 ]; then
        args+=("$@")
      fi
      exec "$CONTENT_GRID_BIN" "${args[@]}"
      ;;
    ""|-h|--help|help)
      usage
      ;;
    *)
      usage >&2
      die "unknown command: $cmd"
      ;;
  esac
}

main "$@"
