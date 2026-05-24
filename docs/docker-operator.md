# Docker Operator Stack

This repository now includes a single operator image plus a `docker compose` stack for joining an existing Congrid network and running a node together with the off-chain verifier support services.

Terminology in this document:

- `validator` means a Cosmos consensus validator.
- `verifier` means the Congrid publisher-verification role and its off-chain agents.

## What It Runs

- `node`: `content-grid-d` full node
- `chromad`: vector DB + embedding helper used by `indexerd`
- `indexerd`: publisher indexing and similarity API
- `verifierd`: chain-driven verifier agent
- `drand-relayer`: optional profile for drand beacon submission

`verifierd` here is the Congrid publisher-verification agent. It is separate from a Cosmos consensus validator, though the same node container can also be used as a consensus validator if you provide the right staking/validator keys.

## Files

- Compose stack: `docker-compose.operator.yml`
- Environment example: `docker/operator.env.example`
- Genesis drop-in directory: `docker/network/`
- Secret file directory: `docker/secrets/`

## Quick Start

1. Copy the environment template and edit it:

   ```bash
   cp docker/operator.env.example .env.operator
   ```

2. Provide the existing network bootstrap data:
   - Put the official `genesis.json` at `docker/network/genesis.json` and keep `CONGRID_GENESIS_FILE=/network/genesis.json`, or
   - set `CONGRID_GENESIS_URL` to a reachable genesis URL.
   - Set `CONGRID_P2P_SEEDS` to the published seed list. The node will use CometBFT PEX and its address book to discover the rest of the peers.
   - Leave `CONGRID_PERSISTENT_PEERS` empty for normal operation; use it only when a node must pin a specific peer connection.
   - Keep `CONGRID_P2P_PEX=true`. For local or private IP networks, set `CONGRID_P2P_ADDR_BOOK_STRICT=false`; for public mainnet/testnet seeds, keep it `true`.

3. Provide signer secrets for `verifierd`:
   - `docker/secrets/verifier.mnemonic`
   - `docker/secrets/verifier.passphrase`

4. Start the stack:

   ```bash
   docker compose --env-file .env.operator -f docker-compose.operator.yml up -d --build
   ```

5. If this operator should also run `drand-relayer`, start with the profile enabled:

   ```bash
   docker compose --env-file .env.operator -f docker-compose.operator.yml --profile drand up -d --build
   ```

If you only want a local full node and do not need `chromad`, `indexerd`, `verifierd`, or `drand-relayer`, build and start only the `node` service:

```bash
docker compose --env-file .env.operator -f docker-compose.operator.yml build node
docker compose --env-file .env.operator -f docker-compose.operator.yml up -d node
```

The `node` service now uses a lightweight `node-runtime` build target that contains only `content-grid-d` plus the node entrypoint. The other off-chain services continue to use the full `operator-runtime` image.

## Keyring Notes

- The compose example defaults to `CONGRID_KEYRING_BACKEND=file`.
- `verifierd` and `drand-relayer` support unattended `file` keyring signing by reading the passphrase from a file-backed environment variable.
- Use separate keyring directories for the validator, verifier, and drand-relayer accounts, such as `CONGRID_VALIDATOR_KEYRING_DIR`, `CONGRID_VERIFIER_KEYRING_DIR`, and `CONGRID_DRAND_KEYRING_DIR`. This avoids sharing one `file` keyring passphrase and import flow across unrelated accounts.
- For disposable local testing only, you can switch to `CONGRID_KEYRING_BACKEND=test` and set mnemonic values directly in the env file instead of using secret files.

## drand-relayer Long-Running Cadence

- Defaults are `CONGRID_DRAND_POLL_INTERVAL_SECONDS=60` and `CONGRID_DRAND_MIN_SUBMIT_INTERVAL_SECONDS=300`, so the relayer checks once per minute and submits at most once every 5 minutes.
- For account sequence mismatch or tx cache races, defaults are `CONGRID_DRAND_RETRY_BACKOFF_SECONDS=30` and `CONGRID_DRAND_MAX_SUBMIT_RETRIES=1` to avoid repeatedly signing with a stale account sequence.
- If block inclusion is slow, increase `CONGRID_DRAND_TX_INCLUSION_TIMEOUT_SECONDS`; the default is `120` seconds.

## verifierd Submission Cadence

- `CONGRID_VERIFIER_COMMIT_START_BUFFER_SECONDS=15` waits 15 seconds after assignment start before committing, avoiding block-time edge cases where the next block is still timestamped before the assignment start.
- `CONGRID_VERIFIER_TX_INCLUSION_TIMEOUT_SECONDS=120` makes `verifierd` wait for tx inclusion and validate the returned tx `code`, so an included-but-failed tx is not treated as successful.
- `CONGRID_VERIFIER_RETRY_BACKOFF_SECONDS=30` backs off after retriable errors such as sequence mismatch, reveal window not open, or tx wait timeout.
- `CONGRID_VERIFIER_STATE_DIR=/var/lib/congrid/verifierd-state` persists commit nonces in the node volume so reveal can continue with the same nonce after tx timeouts or process restarts.

## Consensus Validator Notes

If you also want the node to produce blocks as a Cosmos validator:

- The node must be fully synced to the target network.
- You must fund the operator account and create the validator with `content-grid-d tx staking create-validator`, or join at genesis via the normal `gentx` flow.
- If you are migrating an existing validator, persist the node volume and place the correct `priv_validator_key.json` / `priv_validator_state.json` in the node home before first start.
- If you want the `validator.json` manifest used by `create-validator` to live under Docker-managed config as well, set these in `.env.operator`:
  - `CONGRID_VALIDATOR_KEY_NAME`
  - `CONGRID_VALIDATOR_KEYRING_DIR` (leave empty to reuse the default keyring under `CONGRID_HOME` / `CONGRID_KEYRING_DIR`)
  - `CONGRID_VALIDATOR_JSON_ENABLE=true`
  - `CONGRID_VALIDATOR_AMOUNT`
  - `CONGRID_VALIDATOR_MONIKER`, `CONGRID_VALIDATOR_IDENTITY`, `CONGRID_VALIDATOR_WEBSITE`, `CONGRID_VALIDATOR_SECURITY`, `CONGRID_VALIDATOR_DETAILS`
  - `CONGRID_VALIDATOR_COMMISSION_RATE`, `CONGRID_VALIDATOR_COMMISSION_MAX_RATE`, `CONGRID_VALIDATOR_COMMISSION_MAX_CHANGE_RATE`
  - `CONGRID_VALIDATOR_MIN_SELF_DELEGATION`
- On node startup, the entrypoint will fill `pubkey` from the local `content-grid-d comet show-validator` output and write the final JSON to `CONGRID_VALIDATOR_JSON_PATH` (default `/var/lib/congrid/config/validator.json`).
- The generated file persists with the node volume, which makes it easier to audit later or run `content-grid-d tx staking create-validator /var/lib/congrid/config/validator.json ...` inside the container.
- The node image also ships with `congrid-validator-cli`, which reuses those validator env vars so common validator operations do not need repeated `--home` / `--keyring-dir` flags.

Common examples:

```bash
podman exec congridnet_node_1 congrid-validator-cli show-config

read -rsp 'Keyring passphrase: ' KEYRING_PASS
echo

ACC=$(
  printf '%s\n' "$KEYRING_PASS" |
  podman exec -i congridnet_node_1 \
    congrid-validator-cli show-account-address 2>/dev/null
)

VALOPER=$(
  printf '%s\n' "$KEYRING_PASS" |
  podman exec -i congridnet_node_1 \
    congrid-validator-cli show-valoper-address 2>/dev/null
)

printf 'ACC=%s\nVALOPER=%s\n' "$ACC" "$VALOPER"

podman exec -it congridnet_node_1 \
  congrid-validator-cli create-validator \
  --gas auto \
  --gas-adjustment 1.5 \
  --gas-prices 0.001ucongrid \
  -y
```

## Health Checks

- Node RPC: host port `${CONGRID_RPC_PORT}` -> container `26657`
- Node gRPC: host port `${CONGRID_GRPC_PORT}` -> container `9090`
- Indexerd HTTP: host port `${CONGRID_INDEXER_PORT}` -> container `9100`
- verifierd readiness HTTP: host port `${CONGRID_VERIFIER_HEALTH_PORT}` -> container `9200`
- drand-relayer readiness HTTP: host port `${CONGRID_DRAND_HEALTH_PORT}` -> container `9201` when the `drand` profile is enabled
- Chroma stays internal to the compose network by default

Endpoint conventions:

```bash
curl -fsS http://127.0.0.1:${CONGRID_INDEXER_PORT:-9100}/healthz
curl -fsS http://127.0.0.1:${CONGRID_VERIFIER_HEALTH_PORT:-9200}/healthz
curl -s http://127.0.0.1:${CONGRID_VERIFIER_HEALTH_PORT:-9200}/readyz | jq .
curl -fsS http://127.0.0.1:${CONGRID_DRAND_HEALTH_PORT:-9201}/healthz
curl -s http://127.0.0.1:${CONGRID_DRAND_HEALTH_PORT:-9201}/readyz | jq .
```

`/healthz` is process liveness. `/readyz` is dependency/work readiness for signer agents and returns `503` with a JSON `reasons` list when the last poll/sync failed, no successful poll/sync has happened yet, or the last success is stale. The compose healthchecks use `/readyz` for `verifierd` and `drand-relayer`, and `/healthz` for `indexerd` and `chromad`.
