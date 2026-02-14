# indexerd (off-chain) — publisher homepage indexing + embeddings + similarity signatures

`indexerd` periodically fetches publisher homepages, extracts normalized text, generates embeddings, derives a **compact similarity signature**, and exposes an HTTP API for verifiers (and other components) to query cached results.

## Why

- Avoids every verifier repeatedly fetching the same publisher homepage.
- Avoids every verifier repeatedly running embedding inference.
- Provides a short, cheap-to-transfer **signature** (e.g. 128-bit) that can be used for:
  - diversity / de-duplication heuristics
  - “too-similar” gating
  - lightweight similarity bucketing

## Publisher discovery

`indexerd` can index publishers from:

- **Chain registry (recommended):** query the registry module over gRPC (`Query/Publishers`).
- **Static list:** `publishers` in config (useful for local/dev).

If both are configured, `indexerd` will index the union (deduped).

Active-state filtering (current behavior):
- Chain-discovered publishers are indexed only when `status == VERIFIED` and `cooldown_until_unix <= now`.
- If a previously indexed publisher is no longer active, indexerd prunes it from in-memory cache and (when enabled) Chroma.
- Static-list publishers are also filtered through chain status when `chain_grpc_addr` is configured.

## What is indexed

For each publisher homepage, `indexerd` stores:

- normalized markdown (best-effort)
- embedding vector (used internally for semantic query/debug)
- compact similarity signature (default: `signature_bits=128`)
- `body_sha256` of the fetched homepage bytes (for binding/debug)
- number of Congrid links found
- wallet address extracted from the first Congrid badge image URL:
  `https://congrid.net/...?...publisher=<domain>&wallet=<addr>` (best-effort)

## Requirements

Start the embedding service:

```bash
python offchain/services/sentence_transformer_server.py --host 0.0.0.0 --port 9000
```

## Config

```bash
cp offchain/indexerd/config.example.json offchain/indexerd/config.json
```

Key fields:

- `chain_grpc_addr`: chain gRPC endpoint (e.g. `127.0.0.1:9090`)
- `publishers`: optional static list of domains (may include ports)
- `listen_addr`: e.g. `127.0.0.1:9100`
- `embedder_base_url`: embedding server base URL
- `index_interval_minutes`: how often to re-index
- `signature_bits`: size of the returned signature (multiple of 8; default 128)

## Run

Index once:

```bash
go run ./offchain/indexerd --config offchain/indexerd/config.json --once
```

Daemon mode:

```bash
go run ./offchain/indexerd --config offchain/indexerd/config.json
```

## API

- `GET /healthz`
- `GET /v1/publishers` — list cached publisher docs
- `GET /v1/publishers/{domain}` — get cached doc for a publisher
- `POST /v1/index` — trigger background re-index
- `POST /v1/query` — semantic search (uses stored embeddings)
- `POST /v1/similar` — similar publisher domains (`limit` via JSON body or query param; default `15`)

### Redaction / verbose mode

By default, `GET /v1/publishers*` **redacts large fields** (`markdown`, `embedding`).

To include them (debug only):

- `GET /v1/publishers?verbose=true`
- `GET /v1/publishers/{domain}?verbose=true`

### Example query

```bash
curl -s http://127.0.0.1:9100/v1/query \
  -H 'content-type: application/json' \
  -d '{"text":"news about AI", "limit": 5}'
```
