# Contributing to Content Grid Chain

Thank you for improving Content Grid Chain. This guide contains the development, build, test, and local-network material intentionally kept out of the protocol-focused [README](README.md).

[中文贡献指南](CONTRIBUTING-zh.md)

## Prerequisites

- Go 1.25.1 or a compatible newer patch release, as declared in `go.mod`
- Git and a Unix-like environment (macOS or Linux recommended)
- Enough disk space for the Go module and build caches

No separately installed protobuf compiler is required for the repository targets; the Makefile runs the pinned Go-based Buf tooling.

## Repository layout

- `app/`: Cosmos SDK runtime wiring, encoding, keepers, genesis, ante handlers, and upgrades
- `cmd/content-grid-d/`: node daemon, client commands, and development-network helpers
- `cmd/congrid-site/`: publisher onboarding, dashboards, downloads, and marketplace web application
- `cmd/tokenomics/`: supply simulation, genesis-template, and airdrop CLI
- `x/registry/`: publishers, verification rounds, drand, similar-site evidence, slots, and leases
- `x/verifiers/`: verifier bond escrow and eligibility
- `x/tokenomics/`: issuance-pool and settlement helpers
- `x/nodes/`: node genesis model; runtime functionality is currently limited
- `offchain/`: verifier, indexer, DHT, and supporting services
- `proto/`: Content Grid protobuf and gRPC definitions
- `scripts/`: generation, release, test-network, and tokenomics helpers
- `docs/`: protocol, operator, and component documentation
- `third_party/`: repository-pinned compatibility code; avoid editing it unless the change specifically requires it

## Build and run locally

Download dependencies and build the daemon with embedded version and commit metadata:

```bash
go mod download
make build
./content-grid-d version
./content-grid-d version --long
```

`make build` writes `content-grid-d` at the repository root. The binary is ignored by Git and must not be committed.

### Single-node development network

Create a self-contained single-validator network:

```bash
./content-grid-d devnet \
  --home ./devnet-home \
  --chain-id grid-dev-1

./content-grid-d start --home ./devnet-home
```

`devnet` runs `init`, creates the default `validator` key with the `test` keyring, adds its genesis account, generates a `gentx`, and collects genesis transactions. To recreate this network, rerun `devnet` with `--force`; that option clears the selected devnet home, so never point it at a home containing keys or data you need.

### Manual three-node network

Use this setup when a change needs peer discovery, multiple consensus validators, or network-level testing.

1. Build and initialize three homes:

   ```bash
   make build

   CHAIN_ID=grid-local-1
   NODE1_HOME=./localnet/node1
   NODE2_HOME=./localnet/node2
   NODE3_HOME=./localnet/node3

   ./content-grid-d init node1 --chain-id "$CHAIN_ID" --home "$NODE1_HOME"
   ./content-grid-d init node2 --chain-id "$CHAIN_ID" --home "$NODE2_HOME"
   ./content-grid-d init node3 --chain-id "$CHAIN_ID" --home "$NODE3_HOME"

   ./content-grid-d keys add node1 --home "$NODE1_HOME" --keyring-backend test
   ./content-grid-d keys add node2 --home "$NODE2_HOME" --keyring-backend test
   ./content-grid-d keys add node3 --home "$NODE3_HOME" --keyring-backend test
   ```

2. Add every validator account to node 1's genesis:

   ```bash
   NODE1_ADDR=$(./content-grid-d keys show node1 --home "$NODE1_HOME" --keyring-backend test --address)
   NODE2_ADDR=$(./content-grid-d keys show node2 --home "$NODE2_HOME" --keyring-backend test --address)
   NODE3_ADDR=$(./content-grid-d keys show node3 --home "$NODE3_HOME" --keyring-backend test --address)

   ./content-grid-d genesis add-genesis-account "$NODE1_ADDR" 100000000ucongrid --home "$NODE1_HOME"
   ./content-grid-d genesis add-genesis-account "$NODE2_ADDR" 100000000ucongrid --home "$NODE1_HOME"
   ./content-grid-d genesis add-genesis-account "$NODE3_ADDR" 100000000ucongrid --home "$NODE1_HOME"
   ```

3. Distribute the shared genesis, then create one `gentx` per node:

   ```bash
   cp "$NODE1_HOME/config/genesis.json" "$NODE2_HOME/config/genesis.json"
   cp "$NODE1_HOME/config/genesis.json" "$NODE3_HOME/config/genesis.json"

   ./content-grid-d genesis gentx node1 1000000ucongrid --chain-id "$CHAIN_ID" --home "$NODE1_HOME" --keyring-backend test
   ./content-grid-d genesis gentx node2 1000000ucongrid --chain-id "$CHAIN_ID" --home "$NODE2_HOME" --keyring-backend test
   ./content-grid-d genesis gentx node3 1000000ucongrid --chain-id "$CHAIN_ID" --home "$NODE3_HOME" --keyring-backend test
   ```

4. Copy the `gentx` files to node 1, collect them, and redistribute the final genesis:

   ```bash
   cp "$NODE2_HOME"/config/gentx/*.json "$NODE1_HOME/config/gentx/"
   cp "$NODE3_HOME"/config/gentx/*.json "$NODE1_HOME/config/gentx/"
   ./content-grid-d genesis collect-gentxs --home "$NODE1_HOME"

   cp "$NODE1_HOME/config/genesis.json" "$NODE2_HOME/config/genesis.json"
   cp "$NODE1_HOME/config/genesis.json" "$NODE3_HOME/config/genesis.json"
   ```

5. Give each node unique ports:

   | Node | P2P | RPC | API | gRPC |
   | --- | ---: | ---: | ---: | ---: |
   | node1 | 26656 | 26657 | 1317 | 9090 |
   | node2 | 26666 | 26667 | 1417 | 9190 |
   | node3 | 26676 | 26677 | 1517 | 9290 |

   Update `p2p.laddr` and `rpc.laddr` in each `config/config.toml`, and `api.address` and `grpc.address` in each `config/app.toml`. Keep `pex = true`. For loopback/private addresses, set `addr_book_strict = false`.

6. Configure node 1 as the seed for nodes 2 and 3:

   ```bash
   NODE1_ID=$(./content-grid-d tendermint show-node-id --home "$NODE1_HOME")
   echo "${NODE1_ID}@127.0.0.1:26656"
   ```

   Put the printed value in the `[p2p]` section of node 2 and node 3:

   ```toml
   seeds = "<NODE1_ID>@127.0.0.1:26656"
   persistent_peers = ""
   ```

7. Start each node in a separate terminal:

   ```bash
   ./content-grid-d start --home "$NODE1_HOME"
   ./content-grid-d start --home "$NODE2_HOME"
   ./content-grid-d start --home "$NODE3_HOME"
   ```

All nodes must use the byte-identical final `genesis.json`. A CometBFT node ID is a network identity, not a seed address by itself; always use `<node-id>@<host>:<p2p-port>`.

## Tests and quality checks

Run the full suite before submitting a change:

```bash
make test
make lint
go test ./... -cover
```

For focused iteration, run a package or test directly:

```bash
go test ./x/registry
go test ./x/registry -run TestName
```

Use table-driven tests where cases share setup. New consensus or settlement behavior should test success, rejection, boundary timing, and deterministic replay. Changes in `app/`, `x/registry`, `x/verifiers`, or `x/tokenomics` should include regression coverage.

## Formatting and generated code

Format Go and protobuf files with:

```bash
make format
```

After changing files under `proto/`, regenerate checked-in Go and gRPC code:

```bash
make proto
```

Review generated diffs and commit them with the source `.proto` change. Do not edit generated `*.pb.go` files by hand.

Go imports should be grouped standard library, third-party, then local. Keep package names lowercase, use `UpperCamelCase` for exported identifiers, wrap errors with context and `%w`, and prefer sentinel errors when callers need to branch on an error.

## Tokenomics utilities

Developer-facing supply simulation, genesis-template, and airdrop commands live in `cmd/tokenomics`:

```bash
go run ./cmd/tokenomics --help
go run ./cmd/tokenomics simulate --years 5 --bonded 0.6
```

See the [tokenomics tools guide](scripts/tokenomics/README.md) for complete examples.

## Protocol-change checklist

Consensus-sensitive changes require extra care:

- Preserve deterministic iteration and sorting; never let map iteration decide state transitions, selection, payouts, or hashes.
- Keep network and filesystem access out of ABCI execution. Off-chain fetching belongs under `offchain/`.
- Validate genesis and parameter changes, including migration and upgrade paths for existing state.
- Document changes to messages, queries, events, status transitions, timing windows, rewards, or slashing behavior.
- Update both English and Chinese user-facing documents when practical.
- Run the full test suite after protobuf, keeper, EndBlock, ante-handler, or application-wiring changes.

## Commits and pull requests

Use an imperative, scoped commit subject when helpful, for example:

```text
registry: reject duplicate verification reveals
app: export tokenomics genesis state
```

A pull request should explain its purpose, behavioral scope, tests run, compatibility or migration impact, and any follow-up work. Link issues with `Fixes #123` when applicable. Include screenshots only when user-facing CLI or rendered documentation changes benefit from them.

By contributing, you agree that source code and documentation are provided under the repository's [MIT License](LICENSE). Review the separate [Chain Inspiring License v1.0](CHAIN-INSPIRING-LICENSE.md) when a contribution includes protected non-code ecosystem intellectual property.
