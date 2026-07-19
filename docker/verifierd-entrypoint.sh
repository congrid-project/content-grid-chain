#!/usr/bin/env bash
set -euo pipefail

source /usr/local/lib/congrid/common.sh

load_env_or_file CONGRID_VERIFIER_KEY_MNEMONIC
load_env_or_file CONGRID_VERIFIER_KEYRING_PASSPHRASE

: "${CONGRID_CHAIN_ID:=congrid-main}"
: "${CONGRID_NODE_RPC_URL:=tcp://node:26657}"
: "${CONGRID_NODE_GRPC_ADDR:=node:9090}"
: "${CONGRID_INDEXERD_BASE_URL:=http://indexerd:9100}"
: "${CONGRID_VERIFIER_CONFIG:=/tmp/congrid-verifierd.json}"
: "${CONGRID_VERIFIER_LISTEN_ADDR:=0.0.0.0:9200}"
: "${CONGRID_VERIFIER_STATE_DIR:=$CONGRID_HOME/verifierd-state}"
: "${CONGRID_VERIFIER_KEY_NAME:=verifier-key}"
: "${CONGRID_VERIFIER_KEYRING_BACKEND:=${CONGRID_KEYRING_BACKEND:-test}}"
: "${CONGRID_VERIFIER_KEYRING_DIR:=${CONGRID_KEYRING_DIR:-}}"
: "${CONGRID_VERIFIER_HOME:=$CONGRID_HOME}"
: "${CONGRID_VERIFIER_ADDRESS:=}"
: "${CONGRID_VERIFIER_VERIFY_SCHEME:=https}"
: "${CONGRID_VERIFIER_POLL_INTERVAL_SECONDS:=15}"
: "${CONGRID_VERIFIER_COMMIT_START_BUFFER_SECONDS:=15}"
: "${CONGRID_VERIFIER_COMMIT_WINDOW_SECONDS:=300}"
: "${CONGRID_VERIFIER_ROUND_INTERVAL_SECONDS:=3600}"
: "${CONGRID_VERIFIER_ASSIGNMENT_DELAY_MAX_SECONDS:=3600}"
: "${CONGRID_VERIFIER_DISABLE_ASSIGNMENT_CHECK:=false}"
: "${CONGRID_VERIFIER_RETRY_BACKOFF_SECONDS:=30}"
: "${CONGRID_VERIFIER_TX_INCLUSION_TIMEOUT_SECONDS:=120}"
: "${CONGRID_VERIFIER_GAS:=250000}"
: "${CONGRID_VERIFIER_GAS_ADJUSTMENT:=1}"
: "${CONGRID_VERIFIER_FEES:=}"
: "${CONGRID_VERIFIER_GAS_PRICES:=0.001ucongrid}"
: "${CONGRID_VERIFIER_FEE_GRANTER:=}"
: "${CONGRID_VERIFIER_BROADCAST_MODE:=sync}"
: "${CONGRID_DRAND_DELIVERY_DISABLED:=false}"
: "${CONGRID_DRAND_API_BASE_URL:=https://api.drand.sh}"
: "${CONGRID_DRAND_REQUEST_TIMEOUT_SECONDS:=10}"
: "${CONGRID_DRAND_RELAY_STAGGER_SECONDS:=60}"
: "${CONGRID_DRAND_RELAY_MAX_DELAY_SECONDS:=180}"
: "${CONGRID_DRAND_FEE_GRANTER:=}"

ensure_automated_tx_backend "$CONGRID_VERIFIER_KEYRING_BACKEND" "${CONGRID_VERIFIER_KEYRING_PASSPHRASE:-}" "verifierd"
ensure_key_present \
  "$CONGRID_VERIFIER_KEY_NAME" \
  "${CONGRID_VERIFIER_KEY_MNEMONIC:-}" \
  "$CONGRID_VERIFIER_HOME" \
  "$CONGRID_VERIFIER_KEYRING_BACKEND" \
  "$CONGRID_VERIFIER_KEYRING_DIR" \
  "${CONGRID_VERIFIER_KEYRING_PASSPHRASE:-}"

if [ -z "$CONGRID_VERIFIER_ADDRESS" ]; then
  CONGRID_VERIFIER_ADDRESS="$(show_key_address \
    "$CONGRID_VERIFIER_KEY_NAME" \
    "$CONGRID_VERIFIER_HOME" \
    "$CONGRID_VERIFIER_KEYRING_BACKEND" \
    "$CONGRID_VERIFIER_KEYRING_DIR" \
    "${CONGRID_VERIFIER_KEYRING_PASSPHRASE:-}")"
fi

keyring_passphrase_env=""
if [ "$CONGRID_VERIFIER_KEYRING_BACKEND" = "file" ] && [ -n "${CONGRID_VERIFIER_KEYRING_PASSPHRASE:-}" ]; then
  keyring_passphrase_env="CONGRID_VERIFIER_KEYRING_PASSPHRASE"
fi

jq -n \
  --arg grpc_addr "$CONGRID_NODE_GRPC_ADDR" \
  --arg listen_addr "$CONGRID_VERIFIER_LISTEN_ADDR" \
  --arg verifier_address "$CONGRID_VERIFIER_ADDRESS" \
  --arg state_dir "$CONGRID_VERIFIER_STATE_DIR" \
  --arg verify_scheme "$CONGRID_VERIFIER_VERIFY_SCHEME" \
  --arg indexerd_base_url "$CONGRID_INDEXERD_BASE_URL" \
  --arg binary "$CONTENT_GRID_BIN" \
  --arg chain_id "$CONGRID_CHAIN_ID" \
  --arg node "$CONGRID_NODE_RPC_URL" \
  --arg from "$CONGRID_VERIFIER_KEY_NAME" \
  --arg keyring_backend "$CONGRID_VERIFIER_KEYRING_BACKEND" \
  --arg keyring_dir "$CONGRID_VERIFIER_KEYRING_DIR" \
  --arg home "$CONGRID_VERIFIER_HOME" \
  --arg gas "$CONGRID_VERIFIER_GAS" \
  --arg fees "$CONGRID_VERIFIER_FEES" \
  --arg gas_prices "$CONGRID_VERIFIER_GAS_PRICES" \
  --arg fee_granter "$CONGRID_VERIFIER_FEE_GRANTER" \
  --arg broadcast_mode "$CONGRID_VERIFIER_BROADCAST_MODE" \
  --arg drand_api_base_url "$CONGRID_DRAND_API_BASE_URL" \
  --arg drand_fee_granter "$CONGRID_DRAND_FEE_GRANTER" \
  --arg keyring_passphrase_env "$keyring_passphrase_env" \
  --argjson poll_interval_seconds "$CONGRID_VERIFIER_POLL_INTERVAL_SECONDS" \
  --argjson commit_start_buffer_seconds "$CONGRID_VERIFIER_COMMIT_START_BUFFER_SECONDS" \
  --argjson commit_window_seconds "$CONGRID_VERIFIER_COMMIT_WINDOW_SECONDS" \
  --argjson round_interval_seconds "$CONGRID_VERIFIER_ROUND_INTERVAL_SECONDS" \
  --argjson assignment_delay_max_seconds "$CONGRID_VERIFIER_ASSIGNMENT_DELAY_MAX_SECONDS" \
  --argjson disable_assignment_check "$CONGRID_VERIFIER_DISABLE_ASSIGNMENT_CHECK" \
  --argjson retry_backoff_seconds "$CONGRID_VERIFIER_RETRY_BACKOFF_SECONDS" \
  --argjson tx_inclusion_timeout_seconds "$CONGRID_VERIFIER_TX_INCLUSION_TIMEOUT_SECONDS" \
  --argjson drand_delivery_disabled "$CONGRID_DRAND_DELIVERY_DISABLED" \
  --argjson drand_request_timeout_seconds "$CONGRID_DRAND_REQUEST_TIMEOUT_SECONDS" \
  --argjson drand_relay_stagger_seconds "$CONGRID_DRAND_RELAY_STAGGER_SECONDS" \
  --argjson drand_relay_max_delay_seconds "$CONGRID_DRAND_RELAY_MAX_DELAY_SECONDS" \
  --argjson gas_adjustment "$CONGRID_VERIFIER_GAS_ADJUSTMENT" \
  '
  {
    grpc_addr: $grpc_addr,
    listen_addr: $listen_addr,
    verifier_address: $verifier_address,
    state_dir: $state_dir,
    poll_interval_seconds: $poll_interval_seconds,
    verify_scheme: $verify_scheme,
    commit_start_buffer_seconds: $commit_start_buffer_seconds,
    commit_window_seconds: $commit_window_seconds,
    round_interval_seconds: $round_interval_seconds,
    assignment_delay_max_seconds: $assignment_delay_max_seconds,
    disable_assignment_check: $disable_assignment_check,
    retry_backoff_seconds: $retry_backoff_seconds,
    tx_inclusion_timeout_seconds: $tx_inclusion_timeout_seconds,
    indexerd_base_url: $indexerd_base_url,
    drand: {
      disabled: $drand_delivery_disabled,
      api_base_url: $drand_api_base_url,
      request_timeout_seconds: $drand_request_timeout_seconds,
      relay_stagger_seconds: $drand_relay_stagger_seconds,
      relay_max_delay_seconds: $drand_relay_max_delay_seconds,
      fee_granter: $drand_fee_granter
    },
    submit: {
      binary: $binary,
      chain_id: $chain_id,
      node: $node,
      from: $from,
      keyring_backend: $keyring_backend,
      keyring_dir: $keyring_dir,
      keyring_passphrase_env: $keyring_passphrase_env,
      home: $home,
      gas: $gas,
      gas_adjustment: $gas_adjustment,
      fees: $fees,
      gas_prices: $gas_prices,
      fee_granter: $fee_granter,
      broadcast_mode: $broadcast_mode,
      yes: true
    }
  }
  ' >"$CONGRID_VERIFIER_CONFIG"

log "starting verifierd verifier=$CONGRID_VERIFIER_ADDRESS grpc=$CONGRID_NODE_GRPC_ADDR listen=$CONGRID_VERIFIER_LISTEN_ADDR"
exec /usr/local/bin/verifierd --config "$CONGRID_VERIFIER_CONFIG"
