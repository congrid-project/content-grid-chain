#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN="$ROOT/content-grid-d"
VERIFIERD_BIN="${VERIFIERD_BIN:-$ROOT/verifierd}"

HOME_DIR="${1:-$ROOT/.e2e-home}"
CHAIN_ID="grid-e2e-$(date +%s)"
KEYRING_BACKEND="test"

WEB_PORT="${WEB_PORT:-8010}"
PUBLISHER_DOMAIN="127.0.0.1.nip.io:${WEB_PORT}"

log() { echo "[e2e] $*"; }

require_bin() {
  log "building content-grid-d binary"
  (cd "$ROOT" && go build -o content-grid-d ./cmd/content-grid-d) >/dev/null 2>&1 || {
    log "failed to build content-grid-d"
    exit 1
  }

  log "building verifierd binary"
  (cd "$ROOT" && go build -o verifierd ./offchain/verifierd) >/dev/null 2>&1 || {
    log "failed to build verifierd"
    exit 1
  }
}

wait_port() {
  local host=$1
  local port=$2
  local tries=${3:-60}
  for ((i=1;i<=tries;i++)); do
    if (echo > /dev/tcp/$host/$port) >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.5
  done
  return 1
}

# Run a tx and wait until it is included in a block (prevents account sequence mismatches).
tx_wait() {
  local out txjson txhash code rawlog
  out="$($BIN "$@" --broadcast-mode sync --output json -y 2>&1)" || {
    echo "$out"
    return 1
  }

  # Some tx commands (notably with --gas auto) print extra lines like "gas estimate: ...".
  # Extract the last JSON object line.
  txjson="$(echo "$out" | awk 'BEGIN{last=""} /^[[:space:]]*\{/ {last=$0} END{print last}')"
  if [ -z "$txjson" ]; then
    echo "missing json tx response"
    echo "$out"
    return 1
  fi

  code="$(echo "$txjson" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d.get("code",0))' 2>/dev/null || echo 0)"
  if [ "$code" != "0" ]; then
    rawlog="$(echo "$txjson" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d.get("raw_log",""))' 2>/dev/null || true)"
    echo "tx failed code=$code: $rawlog"
    echo "$out"
    return 1
  fi

  txhash="$(echo "$txjson" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d.get("txhash",""))' 2>/dev/null || true)"
  txhash="$(echo "$txhash" | xargs)"
  if [ -z "$txhash" ]; then
    echo "missing txhash in response"
    echo "$out"
    return 1
  fi

  $BIN query wait-tx "$txhash" --home "$HOME_DIR" --node tcp://127.0.0.1:26657 --timeout 30s -o json >/dev/null 2>&1 || {
    echo "wait-tx timeout for $txhash"
    echo "$out"
    return 1
  }

  return 0
}

patch_registry_params() {
  local genesis="$HOME_DIR/config/genesis.json"
  log "patching registry params for fast rounds: $genesis"
  python3 - <<PY
import json
p="$genesis"
with open(p) as f:
  g=json.load(f)
app=g.setdefault('app_state',{})
reg=app.setdefault('registry',{'websites':[], 'params':{}})
params=reg.setdefault('params',{})

# Fill a fully-valid PublisherParams (the module validates weights/splits sum-to-1).
params.setdefault('verifier_bond', '50000000')
params.setdefault('verification_ttl', 2000)
params['min_verifier_count']=1
params.setdefault('min_publisher_score', '0.55')
params.setdefault('min_verifier_score', '0.40')

params.setdefault('reward_weights', {
  'availability': '0.50',
  'engagement': '0.30',
  'freshness': '0.20',
})
params.setdefault('reward_split', {
  'publisher_share': '0.70',
  'verifier_share': '0.25',
  'protocol_share': '0.05',
})
params.setdefault('verifier_weights', {
  'accuracy': '0.50',
  'coverage': '0.20',
  'latency': '0.15',
  'freshness': '0.15',
})

params['commit_window_seconds']=12
params['submission_window_seconds']=28
params['round_interval_seconds']=10
params['assignment_delay_max_seconds']=2
params.setdefault('cooldown_base_seconds', 604800)
params.setdefault('publisher_verification_reward', '1000000')
params.setdefault('verifier_verification_reward', '500000')

with open(p,'w') as f:
  json.dump(g,f,indent=2)
PY
}

write_homepage() {
  local dir="$HOME_DIR/www"
  mkdir -p "$dir"
  cat >"$dir/index.html" <<HTML
<!doctype html>
<html><body>
<a href="https://congrid.net"><img src="https://congrid.net/badge.png?publisher=$PUBLISHER_DOMAIN&wallet=$PUBLISHER_ADDR" /></a>
</body></html>
HTML
}

add_lease_anchor() {
  local dir="$HOME_DIR/www"
  cat >"$dir/index.html" <<HTML
<!doctype html>
<html><body>
<a href="https://congrid.net"><img src="https://congrid.net/badge.png?publisher=$PUBLISHER_DOMAIN&wallet=$PUBLISHER_ADDR" /></a>
<a href="$TARGET_URL" data-congrid-slot="$SLOT_ID" data-congrid-lease="$LEASE_ID">Link</a>
</body></html>
HTML
}

KEEP_ON_FAIL="${KEEP_ON_FAIL:-0}"

dump_debug() {
  set +e
  echo "[e2e] ---- debug: tail node.log ----" >&2
  tail -n 120 "$HOME_DIR/node.log" 2>/dev/null >&2 || true
  echo "[e2e] ---- debug: tail web.log ----" >&2
  tail -n 80 "$HOME_DIR/web.log" 2>/dev/null >&2 || true
  echo "[e2e] ---- debug: tail verifierd.log ----" >&2
  tail -n 120 "$HOME_DIR/verifierd.log" 2>/dev/null >&2 || true

  # Try to print end_block events for last few blocks.
  if (echo >/dev/tcp/127.0.0.1/26657) >/dev/null 2>&1; then
    local h
    h="$($BIN query block --type height --home "$HOME_DIR" --node tcp://127.0.0.1:26657 -o json 2>/dev/null | python3 -c 'import sys,json; print(json.load(sys.stdin)["block"]["header"]["height"])' 2>/dev/null || true)"
    if [ -n "$h" ]; then
      echo "[e2e] ---- debug: finalize_block events (last 5 blocks, height=$h) ----" >&2
      for hh in $(seq $((h-5)) $h 2>/dev/null); do
        echo "[e2e] block_results $hh" >&2
        $BIN query block-results "$hh" --home "$HOME_DIR" --node tcp://127.0.0.1:26657 -o json 2>/dev/null | \
          python3 - <<'PY' 2>/dev/null
import sys,json
r=json.load(sys.stdin)
evts=r.get('finalize_block_events') or []
for e in evts:
  t=e.get('type','')
  if 'registry' in t or 'verification' in t or 'publisher' in t or 'slot' in t or 'lease' in t:
    print(t, e.get('attributes'))
PY
      done
    fi
  fi
}

cleanup() {
  rc=$?
  set +e

  if [ "$rc" != "0" ]; then
    dump_debug
  fi

  if [ "$KEEP_ON_FAIL" = "1" ] && [ "$rc" != "0" ]; then
    echo "[e2e] KEEP_ON_FAIL=1; leaving node/web running" >&2
    echo "[e2e] HOME_DIR=$HOME_DIR" >&2
    echo "[e2e] NODE_PID=${NODE_PID:-}" >&2
    echo "[e2e] WEB_PID=${WEB_PID:-}" >&2
    echo "[e2e] VERIFIERD_PID=${VERIFIERD_PID:-}" >&2
    return 0
  fi

  if [ -n "${VERIFIERD_PID:-}" ]; then kill "$VERIFIERD_PID" 2>/dev/null; fi
  if [ -n "${NODE_PID:-}" ]; then kill "$NODE_PID" 2>/dev/null; fi
  if [ -n "${WEB_PID:-}" ]; then kill "$WEB_PID" 2>/dev/null; fi
  wait 2>/dev/null
}

require_bin

log "scaffolding devnet in $HOME_DIR"
rm -rf "$HOME_DIR"
"$BIN" devnet --home "$HOME_DIR" --force --keyring-backend "$KEYRING_BACKEND" --chain-id "$CHAIN_ID" >/dev/null 2>&1
patch_registry_params

trap cleanup EXIT

log "starting local homepage server on :$WEB_PORT"
(
  cd "$HOME_DIR/www" 2>/dev/null || mkdir -p "$HOME_DIR/www" && cd "$HOME_DIR/www"
  python3 -m http.server "$WEB_PORT" --bind 127.0.0.1 >"$HOME_DIR/web.log" 2>&1
) &
WEB_PID=$!

log "starting node"
"$BIN" start --home "$HOME_DIR" >"$HOME_DIR/node.log" 2>&1 &
NODE_PID=$!

GRPC_ADDR="127.0.0.1:9090"

log "waiting for rpc 26657 and grpc 9090"
wait_port 127.0.0.1 26657 120
wait_port 127.0.0.1 9090 120

log "waiting for first block"
for i in {1..120}; do
  if "$BIN" query block --type height 1 --home "$HOME_DIR" --node tcp://127.0.0.1:26657 -o json >/dev/null 2>&1; then
    break
  fi
  sleep 0.5
  if [ "$i" = 120 ]; then
    log "node did not produce first block"
    tail -n 120 "$HOME_DIR/node.log" || true
    exit 1
  fi
done

log "creating keys"
"$BIN" keys add publisher --home "$HOME_DIR" --keyring-backend "$KEYRING_BACKEND" >/dev/null 2>&1
"$BIN" keys add advertiser --home "$HOME_DIR" --keyring-backend "$KEYRING_BACKEND" >/dev/null 2>&1
"$BIN" keys add verifier --home "$HOME_DIR" --keyring-backend "$KEYRING_BACKEND" >/dev/null 2>&1

PUBLISHER_ADDR="$($BIN keys show publisher --home "$HOME_DIR" --keyring-backend "$KEYRING_BACKEND" -a)"
ADVERTISER_ADDR="$($BIN keys show advertiser --home "$HOME_DIR" --keyring-backend "$KEYRING_BACKEND" -a)"
VERIFIER_ADDR="$($BIN keys show verifier --home "$HOME_DIR" --keyring-backend "$KEYRING_BACKEND" -a)"

write_homepage

log "funding accounts"
tx_wait tx bank send validator "$PUBLISHER_ADDR" 1000000ucongrid --home "$HOME_DIR" --keyring-backend "$KEYRING_BACKEND" --chain-id "$CHAIN_ID" --node tcp://127.0.0.1:26657 --gas auto --gas-adjustment 1.3

tx_wait tx bank send validator "$ADVERTISER_ADDR" 1000000ucongrid --home "$HOME_DIR" --keyring-backend "$KEYRING_BACKEND" --chain-id "$CHAIN_ID" --node tcp://127.0.0.1:26657 --gas auto --gas-adjustment 1.3

tx_wait tx bank send validator "$VERIFIER_ADDR" 1000000ucongrid --home "$HOME_DIR" --keyring-backend "$KEYRING_BACKEND" --chain-id "$CHAIN_ID" --node tcp://127.0.0.1:26657 --gas auto --gas-adjustment 1.3

log "bonding verifier (min bond is 1)"
tx_wait verifier bond 1 --denom ucongrid --home "$HOME_DIR" --keyring-backend "$KEYRING_BACKEND" --chain-id "$CHAIN_ID" --node tcp://127.0.0.1:26657 --gas auto --gas-adjustment 1.3 --from verifier

cat >"$HOME_DIR/verifierd.json" <<JSON
{
  "grpc_addr": "127.0.0.1:9090",
  "listen_addr": "127.0.0.1:0",
  "verifier_address": "$VERIFIER_ADDR",
  "poll_interval_seconds": 1,
  "verify_scheme": "http",
  "commit_window_seconds": 12,
  "round_interval_seconds": 10,
  "assignment_delay_max_seconds": 2,
  "disable_assignment_check": false,
  "submit": {
    "binary": "$BIN",
    "chain_id": "$CHAIN_ID",
    "node": "tcp://127.0.0.1:26657",
    "from": "verifier",
    "keyring_backend": "$KEYRING_BACKEND",
    "home": "$HOME_DIR",
    "gas": "auto",
    "gas_adjustment": 1.3,
    "broadcast_mode": "sync",
    "yes": true
  }
}
JSON

log "starting verifierd"
"$VERIFIERD_BIN" --config "$HOME_DIR/verifierd.json" >"$HOME_DIR/verifierd.log" 2>&1 &
VERIFIERD_PID=$!

log "register publisher (tx registry register-publisher)"
tx_wait tx registry register-publisher "$PUBLISHER_DOMAIN" --home "$HOME_DIR" --keyring-backend "$KEYRING_BACKEND" --chain-id "$CHAIN_ID" --node tcp://127.0.0.1:26657 --gas auto --gas-adjustment 1.3 --from publisher

log "waiting for assignment to appear"
for i in {1..120}; do
  out="$($BIN verifier assignments "$VERIFIER_ADDR" --home "$HOME_DIR" --node tcp://127.0.0.1:26657 --grpc-addr "$GRPC_ADDR" --grpc-insecure -o json 2>&1 || true)"
  echo "$out" | grep -q "$PUBLISHER_DOMAIN" && break
  sleep 1
  if [ "$i" = 120 ]; then
    echo "$out"
    log "no assignment found"
    exit 1
  fi
done

log "waiting publisher status to become VERIFIED"
for i in {1..120}; do
  status_json="$($BIN query registry publisher --domain "$PUBLISHER_DOMAIN" --home "$HOME_DIR" --node tcp://127.0.0.1:26657 --grpc-addr "$GRPC_ADDR" --grpc-insecure -o json 2>/dev/null || true)"
  if echo "$status_json" | grep -q 'WEBSITE_STATUS_VERIFIED'; then
    break
  fi
  sleep 1
  if [ "$i" = 120 ]; then
    echo "$status_json"
    log "publisher not verified"
    exit 1
  fi
done

log "create slot + list + lease"
tx_wait tx registry create-slot \
  --domain "$PUBLISHER_DOMAIN" \
  --label "Test Slot" \
  --summary "e2e" \
  --category "Test" \
  --placement "Homepage" \
  --size "1x1" \
  --rate-denom ucongrid \
  --rate-amount 1 \
  --unit-seconds 10 \
  --min-duration-seconds 10 \
  --max-duration-seconds 10 \
  --home "$HOME_DIR" --keyring-backend "$KEYRING_BACKEND" --chain-id "$CHAIN_ID" --node tcp://127.0.0.1:26657 --gas auto --gas-adjustment 1.3 --from publisher >/dev/null

slots_json="$($BIN query registry slots --home "$HOME_DIR" --node tcp://127.0.0.1:26657 --grpc-addr "$GRPC_ADDR" --grpc-insecure -o json)"
SLOT_ID="$(echo "$slots_json" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d["slots"][0]["id"])')"

tx_wait tx registry update-slot-status --slot-id "$SLOT_ID" --status SLOT_STATUS_LISTED \
  --home "$HOME_DIR" --keyring-backend "$KEYRING_BACKEND" --chain-id "$CHAIN_ID" --node tcp://127.0.0.1:26657 --gas auto --gas-adjustment 1.3 --from publisher >/dev/null

TARGET_URL="https://example.org/landing"
STARTS_AT_UNIX="$(( $(date +%s) + 5 ))"

tx_wait tx registry lease-slot --slot-id "$SLOT_ID" --target-url "$TARGET_URL" --starts-at-unix "$STARTS_AT_UNIX" --duration-seconds 10 \
  --home "$HOME_DIR" --keyring-backend "$KEYRING_BACKEND" --chain-id "$CHAIN_ID" --node tcp://127.0.0.1:26657 --gas auto --gas-adjustment 1.3 --from advertiser >/dev/null

leases_json="$($BIN query registry leases --home "$HOME_DIR" --node tcp://127.0.0.1:26657 --grpc-addr "$GRPC_ADDR" --grpc-insecure -o json)"
LEASE_ID="$(echo "$leases_json" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d["leases"][0]["id"])')"

add_lease_anchor

log "waiting another verification cycle with lease anchor"
sleep 25
status_json="$($BIN query registry publisher --domain "$PUBLISHER_DOMAIN" --home "$HOME_DIR" --node tcp://127.0.0.1:26657 --grpc-addr "$GRPC_ADDR" --grpc-insecure -o json 2>/dev/null || true)"
echo "$status_json" | grep -q 'WEBSITE_STATUS_VERIFIED' || {
  echo "$status_json"
  log "publisher lost verified status after lease-anchor cycle"
  exit 1
}

log "e2e smoke OK"
