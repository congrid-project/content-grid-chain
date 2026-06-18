#!/usr/bin/env bash
set -euo pipefail

source /usr/local/lib/congrid/common.sh

load_env_or_file CONGRID_DRAND_KEY_MNEMONIC
load_env_or_file CONGRID_DRAND_KEYRING_PASSPHRASE

: "${CONGRID_CHAIN_ID:=congrid-main}"
: "${CONGRID_NODE_RPC_URL:=tcp://node:26657}"
: "${CONGRID_NODE_GRPC_ADDR:=node:9090}"
: "${CONGRID_DRAND_RELAYER_CONFIG:=/tmp/congrid-drand-relayer.json}"
: "${CONGRID_DRAND_LISTEN_ADDR:=0.0.0.0:9201}"
: "${CONGRID_DRAND_KEY_NAME:=drand-relayer}"
: "${CONGRID_DRAND_KEYRING_BACKEND:=${CONGRID_KEYRING_BACKEND:-test}}"
: "${CONGRID_DRAND_KEYRING_DIR:=${CONGRID_KEYRING_DIR:-}}"
: "${CONGRID_DRAND_HOME:=$CONGRID_HOME}"
: "${CONGRID_DRAND_API_BASE_URL:=https://api.drand.sh}"
: "${CONGRID_DRAND_CHAIN_HASH:=52db9ba70e0cc0f6eaf7803dd07447a1f5477735fd3f661792ba94600c84e971}"
: "${CONGRID_DRAND_POLL_INTERVAL_SECONDS:=60}"
: "${CONGRID_DRAND_MIN_SUBMIT_INTERVAL_SECONDS:=300}"
: "${CONGRID_DRAND_REQUEST_TIMEOUT_SECONDS:=10}"
: "${CONGRID_DRAND_RETRY_BACKOFF_SECONDS:=30}"
: "${CONGRID_DRAND_TX_INCLUSION_TIMEOUT_SECONDS:=120}"
: "${CONGRID_DRAND_MAX_SUBMIT_RETRIES:=1}"
: "${CONGRID_DRAND_GAS:=220000}"
: "${CONGRID_DRAND_GAS_ADJUSTMENT:=1}"
: "${CONGRID_DRAND_FEES:=5000ucongrid}"
: "${CONGRID_DRAND_GAS_PRICES:=}"
: "${CONGRID_DRAND_BROADCAST_MODE:=sync}"

ensure_automated_tx_backend "$CONGRID_DRAND_KEYRING_BACKEND" "${CONGRID_DRAND_KEYRING_PASSPHRASE:-}" "drand-relayer"
ensure_key_present \
  "$CONGRID_DRAND_KEY_NAME" \
  "${CONGRID_DRAND_KEY_MNEMONIC:-}" \
  "$CONGRID_DRAND_HOME" \
  "$CONGRID_DRAND_KEYRING_BACKEND" \
  "$CONGRID_DRAND_KEYRING_DIR" \
  "${CONGRID_DRAND_KEYRING_PASSPHRASE:-}"

keyring_passphrase_env=""
if [ "$CONGRID_DRAND_KEYRING_BACKEND" = "file" ] && [ -n "${CONGRID_DRAND_KEYRING_PASSPHRASE:-}" ]; then
  keyring_passphrase_env="CONGRID_DRAND_KEYRING_PASSPHRASE"
fi

jq -n \
  --arg grpc_addr "$CONGRID_NODE_GRPC_ADDR" \
  --arg listen_addr "$CONGRID_DRAND_LISTEN_ADDR" \
  --arg drand_api_base_url "$CONGRID_DRAND_API_BASE_URL" \
  --arg drand_chain_hash "$CONGRID_DRAND_CHAIN_HASH" \
  --arg binary "$CONTENT_GRID_BIN" \
  --arg chain_id "$CONGRID_CHAIN_ID" \
  --arg node "$CONGRID_NODE_RPC_URL" \
  --arg from "$CONGRID_DRAND_KEY_NAME" \
  --arg keyring_backend "$CONGRID_DRAND_KEYRING_BACKEND" \
  --arg keyring_dir "$CONGRID_DRAND_KEYRING_DIR" \
  --arg keyring_passphrase_env "$keyring_passphrase_env" \
  --arg home "$CONGRID_DRAND_HOME" \
  --arg gas "$CONGRID_DRAND_GAS" \
  --arg fees "$CONGRID_DRAND_FEES" \
  --arg gas_prices "$CONGRID_DRAND_GAS_PRICES" \
  --arg broadcast_mode "$CONGRID_DRAND_BROADCAST_MODE" \
  --argjson poll_interval_seconds "$CONGRID_DRAND_POLL_INTERVAL_SECONDS" \
  --argjson min_submit_interval_seconds "$CONGRID_DRAND_MIN_SUBMIT_INTERVAL_SECONDS" \
  --argjson request_timeout_seconds "$CONGRID_DRAND_REQUEST_TIMEOUT_SECONDS" \
  --argjson retry_backoff_seconds "$CONGRID_DRAND_RETRY_BACKOFF_SECONDS" \
  --argjson tx_inclusion_timeout_seconds "$CONGRID_DRAND_TX_INCLUSION_TIMEOUT_SECONDS" \
  --argjson max_submit_retries "$CONGRID_DRAND_MAX_SUBMIT_RETRIES" \
  --argjson gas_adjustment "$CONGRID_DRAND_GAS_ADJUSTMENT" \
  '
  {
    grpc_addr: $grpc_addr,
    listen_addr: $listen_addr,
    drand_api_base_url: $drand_api_base_url,
    drand_chain_hash: $drand_chain_hash,
    poll_interval_seconds: $poll_interval_seconds,
    min_submit_interval_seconds: $min_submit_interval_seconds,
    request_timeout_seconds: $request_timeout_seconds,
    retry_backoff_seconds: $retry_backoff_seconds,
    tx_inclusion_timeout_seconds: $tx_inclusion_timeout_seconds,
    max_submit_retries: $max_submit_retries,
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
      broadcast_mode: $broadcast_mode,
      yes: true
    }
  }
  ' >"$CONGRID_DRAND_RELAYER_CONFIG"

log "starting drand-relayer grpc=$CONGRID_NODE_GRPC_ADDR listen=$CONGRID_DRAND_LISTEN_ADDR drand_api=$CONGRID_DRAND_API_BASE_URL"
exec /usr/local/bin/drand-relayer --config "$CONGRID_DRAND_RELAYER_CONFIG"
