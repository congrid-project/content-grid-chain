# drand-relayer (off-chain)

`drand-relayer` polls drand public API and submits latest beacons on-chain via:

- `tx registry submit-drand-beacon`

This is required when `registry` params enable drand mixing for assignment randomness.

## Config

Copy and edit:

```bash
cp offchain/drandrelayer/config.example.json offchain/drandrelayer/config.json
```

Key fields:
- `listen_addr`: health/readiness HTTP endpoint (default `127.0.0.1:9201`)
- `drand_api_base_url`: default `https://api.drand.sh`
- `drand_chain_hash`: drand network chain hash (quicknet by default in example)
- `poll_interval_seconds`: how often to poll drand
- `min_submit_interval_seconds`: minimum spacing between successful on-chain beacon submissions. Keep this higher than drand's native round time for long-running operators.
- `retry_backoff_seconds`: wait time before retrying after retriable tx errors such as account sequence mismatch.
- `tx_inclusion_timeout_seconds`: how long to wait for a submitted tx to be included before treating it as failed.
- `max_submit_retries`: per-round immediate submit retries. For unattended long-running relayers, `1` is recommended; let the outer backoff handle sequence/cache races.
- `submit.*`: chain tx settings for `content-grid-d`

For unattended container deployments with `submit.keyring_backend=file`, set `submit.keyring_passphrase_env` to the environment variable name that contains the keyring passphrase.

## Run

One-time sync:

```bash
go run ./offchain/drandrelayer --config offchain/drandrelayer/config.json --once
```

Long-running mode:

```bash
go run ./offchain/drandrelayer --config offchain/drandrelayer/config.json
```

## Health and readiness

Long-running mode exposes:

- `GET /healthz` on `listen_addr`: liveness only, returns `200 ok` while the process can serve HTTP.
- `GET /readyz` on `listen_addr`: readiness JSON, returns `200` after successful chain + drand sync and `503` when sync has never succeeded, the last sync failed, or the last successful sync is stale.

Example:

```bash
curl -fsS http://127.0.0.1:9201/healthz
curl -s http://127.0.0.1:9201/readyz | jq .
```

The readiness body includes `last_sync_error`, `consecutive_errors`, `on_chain_round`, `latest_drand_round`, `last_submitted_round`, `next_submit_try_at`, and throttle state.

## Notes

- Relayer only submits when drand latest round is newer than on-chain latest round.
- The default long-running cadence polls every 60 seconds and submits at most once every 300 seconds. This keeps the latest beacon fresh enough for hourly assignment rounds without paying fees for every drand quicknet round.
- On-chain `MsgSubmitDrandBeacon` enforces beacon validation (including signature verification when drand mode is enabled/strict).
