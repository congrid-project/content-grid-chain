#!/usr/bin/env bash
set -euo pipefail

source /usr/local/lib/congrid/common.sh

: "${CONGRID_INDEXER_CONFIG:=/tmp/congrid-indexerd.json}"
: "${CONGRID_NODE_GRPC_ADDR:=node:9090}"
: "${CONGRID_INDEXER_PUBLISHERS:=}"
: "${CONGRID_INDEXER_LISTEN_ADDR:=0.0.0.0:9100}"
: "${CONGRID_CHROMA_BASE_URL:=http://chromad:8000}"
: "${CONGRID_CHROMA_COLLECTION:=publishers}"
: "${CONGRID_SIGNATURE_BITS:=128}"
: "${CONGRID_FETCH_TIMEOUT_SECONDS:=10}"
: "${CONGRID_INDEX_INTERVAL_MINUTES:=60}"
: "${CONGRID_INDEXER_MAX_BODY_BYTES:=1048576}"
: "${CONGRID_CHAIN_TIMEOUT_SECONDS:=10}"
: "${CONGRID_CHAIN_PAGE_LIMIT:=200}"

jq -n \
  --arg chain_grpc_addr "$CONGRID_NODE_GRPC_ADDR" \
  --arg publishers_csv "$CONGRID_INDEXER_PUBLISHERS" \
  --arg listen_addr "$CONGRID_INDEXER_LISTEN_ADDR" \
  --arg chroma_base_url "$CONGRID_CHROMA_BASE_URL" \
  --arg chroma_collection "$CONGRID_CHROMA_COLLECTION" \
  --argjson signature_bits "$CONGRID_SIGNATURE_BITS" \
  --argjson fetch_timeout_seconds "$CONGRID_FETCH_TIMEOUT_SECONDS" \
  --argjson index_interval_minutes "$CONGRID_INDEX_INTERVAL_MINUTES" \
  --argjson max_body_bytes "$CONGRID_INDEXER_MAX_BODY_BYTES" \
  --argjson chain_timeout_seconds "$CONGRID_CHAIN_TIMEOUT_SECONDS" \
  --argjson chain_page_limit "$CONGRID_CHAIN_PAGE_LIMIT" \
  '
  ($publishers_csv
    | split(",")
    | map(gsub("^\\s+|\\s+$"; ""))
    | map(select(length > 0))) as $publishers
  | {
      publishers: $publishers,
      chain_grpc_addr: $chain_grpc_addr,
      chain_timeout_seconds: $chain_timeout_seconds,
      chain_page_limit: $chain_page_limit,
      listen_addr: $listen_addr,
      chroma_base_url: $chroma_base_url,
      chroma_collection: $chroma_collection,
      signature_bits: $signature_bits,
      fetch_timeout_seconds: $fetch_timeout_seconds,
      index_interval_minutes: $index_interval_minutes,
      max_body_bytes: $max_body_bytes
    }
  ' >"$CONGRID_INDEXER_CONFIG"

log "starting indexerd listen=$CONGRID_INDEXER_LISTEN_ADDR chain_grpc=$CONGRID_NODE_GRPC_ADDR chroma=$CONGRID_CHROMA_BASE_URL"
exec /usr/local/bin/indexerd --config "$CONGRID_INDEXER_CONFIG"
