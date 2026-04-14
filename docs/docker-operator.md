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
   - Set `CONGRID_P2P_SEEDS` and/or `CONGRID_PERSISTENT_PEERS` to the current network peer list.

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
- For disposable local testing only, you can switch to `CONGRID_KEYRING_BACKEND=test` and set mnemonic values directly in the env file instead of using secret files.

## Consensus Validator Notes

If you also want the node to produce blocks as a Cosmos validator:

- The node must be fully synced to the target network.
- You must fund the operator account and create the validator with `content-grid-d tx staking create-validator`, or join at genesis via the normal `gentx` flow.
- If you are migrating an existing validator, persist the node volume and place the correct `priv_validator_key.json` / `priv_validator_state.json` in the node home before first start.

## Health Checks

- Node RPC: host port `${CONGRID_RPC_PORT}` -> container `26657`
- Node gRPC: host port `${CONGRID_GRPC_PORT}` -> container `9090`
- Indexerd HTTP: host port `${CONGRID_INDEXER_PORT}` -> container `9100`
- Chroma stays internal to the compose network by default
