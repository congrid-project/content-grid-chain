#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/.." && pwd)
VENV_DIR="$ROOT_DIR/.venv-chromad"

CHROMA_PATH=${CHROMA_PATH:-"$ROOT_DIR/offchain/chromad/data"}
CHROMA_HOST=${CHROMA_HOST:-"127.0.0.1"}
CHROMA_PORT=${CHROMA_PORT:-"8000"}

mkdir -p "$CHROMA_PATH"

if [ ! -d "$VENV_DIR" ]; then
  python3 -m venv "$VENV_DIR"
  "$VENV_DIR/bin/pip" install -U pip
  "$VENV_DIR/bin/pip" install -r "$ROOT_DIR/offchain/chromad/requirements.txt"
fi

export CHROMA_PATH CHROMA_HOST CHROMA_PORT

exec "$VENV_DIR/bin/python" "$ROOT_DIR/offchain/chromad/server.py"
