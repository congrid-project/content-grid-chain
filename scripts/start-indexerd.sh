#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/.." && pwd)
CONFIG=${INDEXERD_CONFIG:-"$ROOT_DIR/offchain/indexerd/config.json"}

exec go run "$ROOT_DIR/offchain/indexerd" --config "$CONFIG"
