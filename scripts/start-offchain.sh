#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/.." && pwd)

mkdir -p "$ROOT_DIR/logs"

# Start chromad
nohup "$ROOT_DIR/scripts/start-chromad.sh" >"$ROOT_DIR/logs/chromad.out" 2>&1 &
echo "chromad: started (logs/chromad.out)"

# Start embedder
nohup "$ROOT_DIR/scripts/start-embedder.sh" >"$ROOT_DIR/logs/embedder.out" 2>&1 &
echo "embedder: started (logs/embedder.out)"

# Start indexerd
nohup "$ROOT_DIR/scripts/start-indexerd.sh" >"$ROOT_DIR/logs/indexerd.out" 2>&1 &
echo "indexerd: started (logs/indexerd.out)"

echo "done"
