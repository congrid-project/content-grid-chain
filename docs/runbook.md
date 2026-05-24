# Production runbook (Runbook)

> Applies to: `content-grid-d`, `verifierd`, `drand-relayer`, `congrid-site`.
> Goal: When a problem occurs, students on duty will locate the direction within 5 minutes and implement mitigating actions within 15 minutes.

---

## 0. Environmental information (current recommended baseline)

- Code directory: `/home/eking/workspace/congrid.net`
- Chain node HOME: `/home/eking/.content-grid-d` (current default baseline)
- verifierd configuration: `/home/eking/workspace/congrid.net/offchain/verifierd/config.json`
- drand-relayer configuration: `/home/eking/workspace/congrid.net/offchain/drandrelayer/config.json`
- site configuration (env file recommended): `/home/eking/workspace/congrid.net/.env.site`
- Log directory (recommended): `/home/eking/workspace/congrid.net/logs/`
- Time zone: `America/Toronto`

Person in charge (currently interim):
- Chain: `eking`
- verifierd：`eking`
- site：`eking`
- Duty notification: `Telegram @eking (id:6148992071)`

---

## 1. Service list and responsibilities

- Chain node: `content-grid-d`
- Responsibilities: consensus, state execution, gRPC/RPC provision
- Verification agent: `verifierd`
- Responsibilities: pull assignment, commit/reveal, site verification
- Random beacon relay: `drand-relayer`
- Responsibility: Pull the latest beacon of drand and upload it to the chain for submission
- Official website/market: `congrid-site`
- Responsibility: display and user entrance, slot/lease submission on the chain

---

## 2. Start, stop and health check

## 2.1 content-grid-d

### Start (manual mode)

Pre-check (required): `--home` must already contain `config/genesis.json`.

```bash
ls -l /home/eking/.content-grid-d/config/genesis.json
```

If the file is missing, initialize first (single-node local baseline):

```bash
cd /home/eking/workspace/congrid.net
./content-grid-d devnet --home /home/eking/.content-grid-d --chain-id content-grid-dev-1
```

Then start:

```bash
cd /home/eking/workspace/congrid.net
./content-grid-d start --home /home/eking/.content-grid-d
```

### Health Check
```bash
# RPC
echo >/dev/tcp/127.0.0.1/26657

# gRPC
echo >/dev/tcp/127.0.0.1/9090

# 最新区块（应持续增长）
./content-grid-d query block --type height --home /home/eking/.content-grid-d --node tcp://127.0.0.1:26657 -o json
```

Key checks:
- The latest blocks continue to grow
- No consecutive panic / consensus failure

## 2.2 verifierd

### Start (manual mode)
```bash
cd /home/eking/workspace/congrid.net
./verifierd --config /home/eking/workspace/congrid.net/offchain/verifierd/config.json
```

### Single round exploration
```bash
./verifierd --config /home/eking/workspace/congrid.net/offchain/verifierd/config.json --once
```

### Health Check
```bash
# Liveness
curl -fsS http://127.0.0.1:9200/healthz

# Readiness and operator state
curl -s http://127.0.0.1:9200/readyz | jq .
```

Key log keywords:
- `submitted commit`
- `revealed result (passed=true|false)`
- `commit failed` / `reveal failed`

Key readiness fields:
- `status` / `reasons`: `ready` or why `/readyz` returned `503`
- `last_poll_error`, `consecutive_errors`: chain polling failures
- `in_flight_assignments`, `pending_reveals`: active and persisted commit/reveal work
- `last_assignment_error`: latest worker-level commit/reveal/state failure

## 2.3 drand-relayer

### Start (manual mode)
```bash
cd /home/eking/workspace/congrid.net
./drand-relayer --config /home/eking/workspace/congrid.net/offchain/drandrelayer/config.json
```

### Single round exploration
```bash
./drand-relayer --config /home/eking/workspace/congrid.net/offchain/drandrelayer/config.json --once
```

### Health Check
```bash
# Liveness
curl -fsS http://127.0.0.1:9201/healthz

# Readiness and operator state
curl -s http://127.0.0.1:9201/readyz | jq .
```

Key log keywords:
- `submitted beacon round=`
- `sync error`

Key readiness fields:
- `status` / `reasons`: `ready` or why `/readyz` returned `503`
- `last_sync_error`, `consecutive_errors`: chain or drand API sync failures
- `on_chain_round`, `latest_drand_round`: whether the relayer is observing fresh drand data
- `last_submitted_round`, `next_submit_try_at`, `throttled_until`: submission/backoff state

## 2.4 congrid-site

### Start (example)
```bash
cd /home/eking/workspace/congrid.net
go run ./cmd/congrid-site \
  --addr :8080 \
  --base-url https://congrid.net \
  --slots-store chain \
  --chain-id content-grid-1 \
  --node tcp://127.0.0.1:26657 \
  --slots-grpc 127.0.0.1:9090 \
  --keyring-backend os
```

Optional: set `--keyring-dir` if the content-grid-d keyring lives outside the default location.

Health check:
- `curl -fsS http://127.0.0.1:8080/healthz`
- `/`, `/marketplace`, `/publisher/dashboard` are accessible
- When submitting slot/lease, tx returns successfully and has txhash

---

## 3. Daily inspection (each shift)

- [ ] Chain height grows normally
- [ ] `verifierd` `/readyz` is `ready`
- [ ] `drand-relayer` `/readyz` is `ready` when enabled
- [ ] verifierd The commit/reveal success rate in the last 15 minutes meets the standard
- [ ] The number of drand beacon winding rounds continues to increase (no long-term stagnation)
- [ ] publisher VERIFIED There is no abnormal decrease in the ratio
- [ ] Lease default (VIOLATED) ratio has not increased abnormally
- [ ] site is available, submission path is available

Recommended threshold:
- assignment pull success rate (5min) `>= 99%`
- Commit success rate (15min) `>= 95%`
- reveal success rate (15min)`>= 90%`

---

## 4. Typical alarm handling

## 4.1 Warning: publisher is PENDING for a long time

Troubleshooting:
1. Check whether assignment is generated
2. Check if verifierd has `submitted commit` / `revealed result`
3. Check whether `reveal window not open` / `account sequence mismatch` occurs frequently
4. If logs show success but the assignment still has no `submission`, query the tx `code`; included txs with `code != 0` still failed

Order:
```bash
./content-grid-d query registry publisher --domain <domain> --node tcp://127.0.0.1:26657 --grpc-addr 127.0.0.1:9090 --grpc-insecure -o json
./content-grid-d verifier assignments <verifier-addr> --node tcp://127.0.0.1:26657 --grpc-addr 127.0.0.1:9090 --grpc-insecure -o json
./content-grid-d query tx <txhash> --node tcp://127.0.0.1:26657 -o json
```

ease:
- Confirm verifierd configuration (poll, commit window, `commit_start_buffer_seconds`, `tx_inclusion_timeout_seconds`)
- Restart verifierd if necessary (keep logs)
- If the chain parameters are unreasonable, follow the parameter adjustment process

---

## 4.2 Warning: Reveal failure rate soars

Troubleshooting:
1. Whether the failure is concentrated in window timing (window not open/closed)
2. Whether the account sequence conflicts?
3. Whether the node produces block jitter or clock deviation

ease:
- Confirm whether tx serial submission is effective
- Increase `CONGRID_VERIFIER_COMMIT_START_BUFFER_SECONDS` and `CONGRID_VERIFIER_TX_INCLUSION_TIMEOUT_SECONDS` if needed
- Confirm `CONGRID_VERIFIER_STATE_DIR` is persisted so nonces are not lost before reveal
- Check system clock (NTP)

---

## 4.3 Warning: Link node panic / consensus failure

Immediate action:
1. Protect on-site logs (node ​​+ verifierd + site)
2. Pull up the standby node or restart (according to plan)
3. Notify SEV1 duty group

Troubleshooting points:
- panic stack top module
- Recent changes (code/parameters)
- Whether it can be mitigated by rolling back

---

## 5. Emergency operations (minimum set)

- Suspended new business entrance (site layer)
- Pause slot listing (if necessary)
- Roll back to the previous stable version
- Gradually increase the volume after recovery

> It is recommended to script the above actions to avoid manual errors.

---

## 6. Rollback process (execution version)

1. Announcement of entering the rollback window (recording time, scope of impact)
2. Record the current version number and parameter snapshot
3. Switch back to the stable version (recommended rollback anchor point: `4202f66`, adjusted according to the actual released version)
4. Restart the service and do a health check
5. Verify core link (publisher verify / slot / lease)
6. Announce rollback completion

Must be executed after rollback:
```bash
cd /home/eking/workspace/congrid.net
go test ./...
./scripts/e2e_smoke.sh /tmp/congrid-e2e-home-rollback-check
```

---

## 7. Postmortem

- Event number:
- Scope of influence:
- Discovery time / recovery time:
- Root cause:
- Direct fix:
- Long-term improvements:
- Owner + Deadline：

---

## 8. Common troubleshooting commands

```bash
cd /home/eking/workspace/congrid.net

# 全量测试
go test ./...

# e2e smoke
./scripts/e2e_smoke.sh /tmp/congrid-e2e-home

# 查询 publisher
./content-grid-d query registry publisher --domain <domain> --node tcp://127.0.0.1:26657 --grpc-addr 127.0.0.1:9090 --grpc-insecure -o json

# 查询 verifier assignments
./content-grid-d verifier assignments <verifier-addr> --node tcp://127.0.0.1:26657 --grpc-addr 127.0.0.1:9090 --grpc-insecure -o json
```
