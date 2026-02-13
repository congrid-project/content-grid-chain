#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/.." && pwd)

HOST=${EMBEDDER_HOST:-"127.0.0.1"}
PORT=${EMBEDDER_PORT:-"9000"}

exec python3 "$ROOT_DIR/offchain/services/sentence_transformer_server.py" --host "$HOST" --port "$PORT"
