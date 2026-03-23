#!/usr/bin/env bash
set -euo pipefail

: "${CHROMA_PATH:=/var/lib/congrid/chroma}"
: "${CHROMA_HOST:=0.0.0.0}"
: "${CHROMA_PORT:=8000}"

mkdir -p "$CHROMA_PATH"
export HOME="$CHROMA_PATH"

exec python3 /opt/congrid/offchain/chromad/server.py
