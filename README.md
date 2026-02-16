# Content Grid Chain

Core protocol implementation for the Content Grid project — a decentralized content network and search protocol.

## About
This chain (Cosmos SDK–based) coordinates and incentivizes a network of workers that:
- Crawl and fetch web content
- Compute embeddings off-chain (pluggable embedder; current dev setup uses a SentenceTransformer HTTP service)
- Derive **compact similarity signatures** (e.g. 128-bit) for diversity / de-duplication heuristics
- Index content and serve similarity search via deterministic assignment

Quick links: see `whitepaper.md` for the design, `docs/tokenomics.md` for the economic blueprint, and `AGENTS.md` for contribution guidelines.

## Requirements
- Go 1.22+ (use the latest stable release)
- Unix-like environment (macOS/Linux) recommended

## Build, Run, Test
- Build: `go build -o content-grid-d ./cmd/content-grid-d`
- Run: `./content-grid-d version` or `./content-grid-d init` (placeholder)
- Test: `go test ./...`
- Proto: `./scripts/proto-gen.sh` regenerates gRPC code after modifying `proto/`

### Local single node startup

1. Run `./content-grid-d devnet --home ./devnet-home --chain-id grid-dev-1`, the CLI will automatically complete `init → keys add → add-genesis-account → gentx → collect-gentxs`, and generate a single-node genesis file with the default validator key (name `validator`, keyring backend is `test`).
2. Execute `./content-grid-d start --home ./devnet-home` to start the local node. If reinitialization is required, append `--force` to clear the old home directory.

### Local multi-node network (manual)

The following example takes 3 nodes as an example. The process is: initialization → generate key → generate gentx on the same genesis → collect gentx → distribute the final genesis → configure ports and interconnections.

1. Initialization and keys
   ```bash
   go build -o content-grid-d ./cmd/content-grid-d

   CHAIN_ID=grid-local-1
   HOME1=./localnet/node1
   HOME2=./localnet/node2
   HOME3=./localnet/node3

   ./content-grid-d init node1 --chain-id $CHAIN_ID --home $HOME1
   ./content-grid-d init node2 --chain-id $CHAIN_ID --home $HOME2
   ./content-grid-d init node3 --chain-id $CHAIN_ID --home $HOME3

   ./content-grid-d keys add node1 --home $HOME1 --keyring-backend test
   ./content-grid-d keys add node2 --home $HOME2 --keyring-backend test
   ./content-grid-d keys add node3 --home $HOME3 --keyring-backend test
   ```

2. Generate the address and join the genesis account (operate on the genesis of node1)
   ```bash
   ADDR1=$(./content-grid-d keys show node1 --home $HOME1 --keyring-backend test --address)
   ADDR2=$(./content-grid-d keys show node2 --home $HOME2 --keyring-backend test --address)
   ADDR3=$(./content-grid-d keys show node3 --home $HOME3 --keyring-backend test --address)

   ./content-grid-d genesis add-genesis-account $ADDR1 100000000ucongrid --home $HOME1
   ./content-grid-d genesis add-genesis-account $ADDR2 100000000ucongrid --home $HOME1
   ./content-grid-d genesis add-genesis-account $ADDR3 100000000ucongrid --home $HOME1
   ```

3. Distribute the same copy of genesis, and then generate gentx separately.
   ```bash
   cp $HOME1/config/genesis.json $HOME2/config/genesis.json
   cp $HOME1/config/genesis.json $HOME3/config/genesis.json

   ./content-grid-d genesis gentx node1 1000000ucongrid --chain-id $CHAIN_ID --home $HOME1 --keyring-backend test
   ./content-grid-d genesis gentx node2 1000000ucongrid --chain-id $CHAIN_ID --home $HOME2 --keyring-backend test
   ./content-grid-d genesis gentx node3 1000000ucongrid --chain-id $CHAIN_ID --home $HOME3 --keyring-backend test
   ```

4. Collect gentx and distribute final genesis
   ```bash
   cp $HOME2/config/gentx/*.json $HOME1/config/gentx/
   cp $HOME3/config/gentx/*.json $HOME1/config/gentx/
   ./content-grid-d genesis collect-gentxs --home $HOME1

   cp $HOME1/config/genesis.json $HOME2/config/genesis.json
   cp $HOME1/config/genesis.json $HOME3/config/genesis.json
   ```

5. Configure ports and interconnections (to avoid local port conflicts)
- Edit `config/config.toml`: set `p2p.laddr` and `rpc.laddr`.
- Edit `config/app.toml`: set `api.address` and `grpc.address`.
- Example port assignment:
     - node1: p2p `26656`, rpc `26657`, api `1317`, grpc `9090`
     - node2: p2p `26666`, rpc `26667`, api `1417`, grpc `9190`
     - node3: p2p `26676`, rpc `26677`, api `1517`, grpc `9290`

6. Set up persistent peers (at least let node2/node3 connect to node1)
   ```bash
   NODE1_ID=$(./content-grid-d tendermint show-node-id --home $HOME1)
   ```
Set in `config/config.toml` of node2 and node3:
   ```
   p2p.persistent_peers = "${NODE1_ID}@127.0.0.1:26656"
   ```

7. Start the nodes separately (different terminals)
   ```bash
   ./content-grid-d start --home $HOME1
   ./content-grid-d start --home $HOME2
   ./content-grid-d start --home $HOME3
   ```

### Publisher Registration

1. **Add Congrid official link + attribution image (required for verification)**: You must add a link to the Congrid official website on the homepage (`/`) of the website you want to bind, and the link must be wrapped with an attribution image (badge).

For the current verifier determination rules, see `offchain/registry/verifier.go`:
- The official website link must be **`<a href="https://congrid.net">`** (or `https://www.congrid.net/`). **The official website address itself is not allowed to contain query/fragment**.
- `<img src="...">` must be included within `a`.
- `img src` must be `https://congrid.net/...` (or `https://www.congrid.net/...`) and carried in **path or query**:
- `publisher=<your-domain>` (allowed without port), for statistical attribution
- `wallet=<bech32-owner-address>`, must be equal to the owner address of the registered transaction (`publisher register --from`), used to prevent squatting

**Recommended format:**
   ```html
   <a href="https://congrid.net">
     <img
       alt="Verified by Congrid"
       src="https://congrid.net/badge.png?publisher=example.com&wallet=<bech32-owner-address>"
     />
   </a>
   ```

2. **Execute registration command**: Run `./content-grid-d publisher register <domain> --from <key-or-address> [--metadata-uri <link>] [--referrer <address>]`.
- `--referrer`: optional, referrer address (used to affect the revenue weight of verifier; publisher recommendation publisher does not take effect).
- The system will automatically identify and lock the **first-level domain name** (Primary Domain, such as `example.com`) of the domain name.
- Only one site can be registered under the same first-level domain name to prevent others from preemptively registering subdomain names. Supports non-default ports (such as `example.com:8080`).
3. **Verification completed**: The command will access `https://<domain>/` to verify whether it contains the congrid official link; the off-chain verification node will also regularly crawl the homepage for confirmation.
- **No deposit/pledge required**: Publisher registration itself does not require locking or staking.
- **On-chain transaction fee (gas fee) is still required**: Broadcasting `publisher register` Such transactions usually require payment of network fees (unless the chain parameters allow 0 fee or use `feegrant`).
4. **Query status**: After successful registration, it can be viewed through gRPC query or CLI `content-grid-d query registry publisher <domain>`.

### Miner registration (on-chain protocol part)

1. Use `./content-grid-d miner register <metadata-uri> <services-bitmask> <min-bid-amount> --stake <amount>` to complete the uploading of miner data to the chain. `services-bitmask` uses a bitmask (for example, `3` means providing both fetching and embedding), and the pledge and the lowest bid temporarily use the same denom.
2. Subsequently, you can update the service statement through `./content-grid-d miner update --metadata-uri ... --services ... --min-bid-amount ...`, or use `./content-grid-d miner stake <amount> [--decrease]` to adjust the recorded pledge amount.
3. The on-chain query interface `query miners`/`query miner <address>` will return the current online miners, service capabilities, quotations and pledge information, and task scheduling will be directly based on these statuses.

### Verifier Bond (normal address + escrow)

Verifier participates in the verification network with a normal account address (`grid1...`), and first bonds the token to the module escrow account** (escrow) before it is considered eligible.

- Bond：`./content-grid-d verifier bond <amount> --denom ucongrid --from <key>`
- Unbond：`./content-grid-d verifier unbond <amount> --denom ucongrid --from <key>`

See `docs/verifiers.md` for more information.

### Economics utilities

- Simulate supply, generate genesis templates, or craft airdrop tables with `go run ./cmd/tokenomics <subcommand>`.
- See `docs/tokenomics.md` for parameter details and `docs/governance.md` for proposal processes.

Notes:
- Do not commit build artifacts (see `.gitignore`).
- The CLI is a minimal skeleton pending full server/runtime wiring.

## Off-chain Components
- `offchain/indexerd`: publisher homepage indexing + embeddings + **compact signatures** (see `docs/indexerd.md`).
- `offchain/verifierd`: chain-driven publisher verification agent (see `docs/verifierd.md`).
- `offchain/executor`: chain worker prototype that fetches, embeds, classifies and publishes content records (see `offchain/executor/README.md`).
- `offchain/services/sentence_transformer_server.py`: python HTTP service wrapping SentenceTransformer embeddings.

## Project Status
Phase 1 skeleton. The `app/` package provides module basics, encoding, and default genesis helpers for Cosmos SDK v0.53.
- `x/registry`: Publisher registration and verification logic.
- `x/miners`: Miner registration and service discovery.
- `x/tasks`: Task assignment (Block Hash) and result verification (On-Chain Consensus) with automated reward distribution.
- `x/tokenomics`: Economic parameters, inflation logic, and settlement keeper.

## Roadmap (High Level)
1) [x] Runtime wiring: depinject + `runtime.App`, keepers for auth/bank/staking, ABCI services
2) [x] CLI/server: `init`, `start`, config/home management, keys
3) [x] First modules: minimal task assignment/commit model under `x/`
4) [ ] P2P/Indexing: Full-node vector indexing & Block Hash-based query routing
5) [x] Economics/Governance: rewards, slashing, parameters, proposals
6) Testnet: reproducible genesis, docs, and CI
