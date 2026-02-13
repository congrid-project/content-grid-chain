#!/usr/bin/env bash
set -euo pipefail

# Starts 12 http.server instances for website/publisher1..12 on ports 8001..8012.
# Logs go to tmp/http-ports/*.log and PIDs to tmp/http-ports/*.pid

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

mkdir -p tmp/http-ports

for i in $(seq 1 12); do
  port=$((8000 + i))
  dir="website/publisher${i}"
  log="tmp/http-ports/${port}.log"
  pidfile="tmp/http-ports/${port}.pid"

  if [[ -f "$pidfile" ]] && kill -0 "$(cat "$pidfile")" 2>/dev/null; then
    echo "port ${port} already running (pid $(cat "$pidfile"))"
    continue
  fi

  echo "starting ${dir} on :${port}"
  nohup python3 -m http.server "$port" --directory "$dir" >"$log" 2>&1 &
  echo $! >"$pidfile"
  sleep 0.05
done

echo "done. Example: http://publisher1.com:8001/"
