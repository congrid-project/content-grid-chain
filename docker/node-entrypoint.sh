#!/usr/bin/env bash
set -euo pipefail

source /usr/local/lib/congrid/common.sh

: "${CONGRID_MONIKER:=node-01}"
: "${CONGRID_CHAIN_ID:=content-grid-1}"
: "${CONGRID_KEYRING_BACKEND:=test}"
: "${CONGRID_KEYRING_DIR:=}"
: "${CONGRID_CLIENT_NODE:=tcp://localhost:26657}"
: "${CONGRID_RPC_LADDR:=tcp://0.0.0.0:26657}"
: "${CONGRID_P2P_LADDR:=tcp://0.0.0.0:26656}"
: "${CONGRID_P2P_EXTERNAL_ADDRESS:=}"
: "${CONGRID_P2P_SEEDS:=}"
: "${CONGRID_PERSISTENT_PEERS:=}"
: "${CONGRID_API_ENABLE:=true}"
: "${CONGRID_API_ADDRESS:=tcp://0.0.0.0:1317}"
: "${CONGRID_GRPC_ENABLE:=true}"
: "${CONGRID_GRPC_ADDRESS:=0.0.0.0:9090}"
: "${CONGRID_MINIMUM_GAS_PRICES:=0.001ucongrid}"
: "${CONGRID_PPROF_LADDR:=localhost:6060}"
: "${CONGRID_VALIDATOR_JSON_ENABLE:=false}"
: "${CONGRID_VALIDATOR_JSON_PATH:=$CONGRID_HOME/config/validator.json}"
: "${CONGRID_VALIDATOR_AMOUNT:=}"
: "${CONGRID_VALIDATOR_MONIKER:=}"
: "${CONGRID_VALIDATOR_IDENTITY:=}"
: "${CONGRID_VALIDATOR_WEBSITE:=}"
: "${CONGRID_VALIDATOR_SECURITY:=}"
: "${CONGRID_VALIDATOR_DETAILS:=}"
: "${CONGRID_VALIDATOR_COMMISSION_RATE:=0.10}"
: "${CONGRID_VALIDATOR_COMMISSION_MAX_RATE:=0.20}"
: "${CONGRID_VALIDATOR_COMMISSION_MAX_CHANGE_RATE:=0.01}"
: "${CONGRID_VALIDATOR_MIN_SELF_DELEGATION:=1}"

ensure_home_initialized "$CONGRID_MONIKER" "$CONGRID_CHAIN_ID"
install_network_genesis

config_toml="$CONGRID_HOME/config/config.toml"
app_toml="$CONGRID_HOME/config/app.toml"
client_toml="$CONGRID_HOME/config/client.toml"

set_toml_string "$config_toml" "" moniker "$CONGRID_MONIKER"
set_toml_string "$config_toml" rpc laddr "$CONGRID_RPC_LADDR"
set_toml_string "$config_toml" rpc pprof_laddr "$CONGRID_PPROF_LADDR"
set_toml_string "$config_toml" p2p laddr "$CONGRID_P2P_LADDR"
set_toml_string "$config_toml" p2p external_address "$CONGRID_P2P_EXTERNAL_ADDRESS"
set_toml_string "$config_toml" p2p seeds "$CONGRID_P2P_SEEDS"
set_toml_string "$config_toml" p2p persistent_peers "$CONGRID_PERSISTENT_PEERS"

set_toml_string "$app_toml" "" minimum-gas-prices "$CONGRID_MINIMUM_GAS_PRICES"
set_toml_bool "$app_toml" api enable "$CONGRID_API_ENABLE"
set_toml_string "$app_toml" api address "$CONGRID_API_ADDRESS"
set_toml_bool "$app_toml" grpc enable "$CONGRID_GRPC_ENABLE"
set_toml_string "$app_toml" grpc address "$CONGRID_GRPC_ADDRESS"

set_toml_string "$client_toml" "" chain-id "$CONGRID_CHAIN_ID"
set_toml_string "$client_toml" "" keyring-backend "$CONGRID_KEYRING_BACKEND"
set_toml_string "$client_toml" "" node "$CONGRID_CLIENT_NODE"

if [ "$CONGRID_VALIDATOR_JSON_ENABLE" = "true" ]; then
  render_validator_json \
    "$CONGRID_VALIDATOR_JSON_PATH" \
    "$CONGRID_VALIDATOR_AMOUNT" \
    "${CONGRID_VALIDATOR_MONIKER:-$CONGRID_MONIKER}" \
    "$CONGRID_VALIDATOR_IDENTITY" \
    "$CONGRID_VALIDATOR_WEBSITE" \
    "$CONGRID_VALIDATOR_SECURITY" \
    "$CONGRID_VALIDATOR_DETAILS" \
    "$CONGRID_VALIDATOR_COMMISSION_RATE" \
    "$CONGRID_VALIDATOR_COMMISSION_MAX_RATE" \
    "$CONGRID_VALIDATOR_COMMISSION_MAX_CHANGE_RATE" \
    "$CONGRID_VALIDATOR_MIN_SELF_DELEGATION"
fi

log "starting node moniker=$CONGRID_MONIKER chain_id=$CONGRID_CHAIN_ID home=$CONGRID_HOME"
exec "$CONTENT_GRID_BIN" start --home "$CONGRID_HOME"
