#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

shopt -s nullglob
for pidfile in tmp/http-ports/*.pid; do
  pid="$(cat "$pidfile")"
  port="$(basename "$pidfile" .pid)"
  if kill -0 "$pid" 2>/dev/null; then
    echo "stopping :${port} (pid ${pid})"
    kill "$pid" || true
  fi
  rm -f "$pidfile"
done

echo "done"
