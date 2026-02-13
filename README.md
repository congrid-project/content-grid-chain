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
- Proto: `./scripts/proto-gen.sh` 在修改 `proto/` 后重新生成 gRPC 代码

### 本地单节点启动

1. 运行 `./content-grid-d devnet --home ./devnet-home --chain-id grid-dev-1`，CLI 会自动完成 `init → keys add → add-genesis-account → gentx → collect-gentxs`，生成带有默认验证人密钥（名称 `validator`，keyring backend 为 `test`）的单节点创世文件。
2. 执行 `./content-grid-d start --home ./devnet-home` 即可启动本地节点。若需重新初始化，附加 `--force` 以清空旧的 home 目录。

### 本地多节点网络（手动）

下面示例以 3 个节点为例，流程是：初始化 → 生成密钥 → 在同一份 genesis 上生成 gentx → 收集 gentx → 分发最终 genesis → 配置端口与互连。

1. 初始化与密钥
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

2. 生成地址并加入创世账户（在 node1 的 genesis 上操作）
   ```bash
   ADDR1=$(./content-grid-d keys show node1 --home $HOME1 --keyring-backend test --address)
   ADDR2=$(./content-grid-d keys show node2 --home $HOME2 --keyring-backend test --address)
   ADDR3=$(./content-grid-d keys show node3 --home $HOME3 --keyring-backend test --address)

   ./content-grid-d genesis add-genesis-account $ADDR1 100000000ucongrid --home $HOME1
   ./content-grid-d genesis add-genesis-account $ADDR2 100000000ucongrid --home $HOME1
   ./content-grid-d genesis add-genesis-account $ADDR3 100000000ucongrid --home $HOME1
   ```

3. 分发同一份 genesis，再各自生成 gentx
   ```bash
   cp $HOME1/config/genesis.json $HOME2/config/genesis.json
   cp $HOME1/config/genesis.json $HOME3/config/genesis.json

   ./content-grid-d genesis gentx node1 1000000ucongrid --chain-id $CHAIN_ID --home $HOME1 --keyring-backend test
   ./content-grid-d genesis gentx node2 1000000ucongrid --chain-id $CHAIN_ID --home $HOME2 --keyring-backend test
   ./content-grid-d genesis gentx node3 1000000ucongrid --chain-id $CHAIN_ID --home $HOME3 --keyring-backend test
   ```

4. 收集 gentx 并分发最终 genesis
   ```bash
   cp $HOME2/config/gentx/*.json $HOME1/config/gentx/
   cp $HOME3/config/gentx/*.json $HOME1/config/gentx/
   ./content-grid-d genesis collect-gentxs --home $HOME1

   cp $HOME1/config/genesis.json $HOME2/config/genesis.json
   cp $HOME1/config/genesis.json $HOME3/config/genesis.json
   ```

5. 配置端口与互连（避免本机端口冲突）
   - 编辑 `config/config.toml`：设置 `p2p.laddr` 与 `rpc.laddr`。
   - 编辑 `config/app.toml`：设置 `api.address` 与 `grpc.address`。
   - 示例端口分配：
     - node1: p2p `26656`, rpc `26657`, api `1317`, grpc `9090`
     - node2: p2p `26666`, rpc `26667`, api `1417`, grpc `9190`
     - node3: p2p `26676`, rpc `26677`, api `1517`, grpc `9290`

6. 设置 persistent peers（至少让 node2/node3 连接 node1）
   ```bash
   NODE1_ID=$(./content-grid-d tendermint show-node-id --home $HOME1)
   ```
   在 node2 与 node3 的 `config/config.toml` 中设置：
   ```
   p2p.persistent_peers = "${NODE1_ID}@127.0.0.1:26656"
   ```

7. 分别启动节点（不同终端）
   ```bash
   ./content-grid-d start --home $HOME1
   ./content-grid-d start --home $HOME2
   ./content-grid-d start --home $HOME3
   ```

### Publisher 注册

1. **添加 Congrid 官方链接 + 归因图片（验证必需）**：在您要绑定的网站主页（`/`）必须添加一个指向 Congrid 官网的链接，并且该链接必须包裹一张归因图片（badge）。

   当前 verifier 的判定规则见 `offchain/registry/verifier.go`：
   - 官网链接必须是 **`<a href="https://congrid.net">`**（或 `https://www.congrid.net/`），**官网地址本身不允许带 query/fragment**。
   - `a` 内必须包含 `<img src="...">`。
   - `img src` 必须是 `https://congrid.net/...`（或 `https://www.congrid.net/...`），并且在 **path 或 query** 中携带：
     - `publisher=<your-domain>`（允许不带端口），用于统计归因
     - `wallet=<bech32-owner-address>`，必须等于注册交易的 owner 地址（`publisher register --from`），用于防止抢注

   **推荐格式：**
   ```html
   <a href="https://congrid.net">
     <img
       alt="Verified by Congrid"
       src="https://congrid.net/badge.png?publisher=example.com&wallet=<bech32-owner-address>"
     />
   </a>
   ```

2. **执行注册命令**：运行 `./content-grid-d publisher register <domain> --from <key-or-address> [--metadata-uri <link>] [--referrer <address>]`。
   - `--referrer`：可选，引荐人地址（用于影响 verifier 的收益权重；publisher 引荐 publisher 不生效）。
   - 系统会自动识别并锁定该域名的**一级域名**（Primary Domain，如 `example.com`）。
   - 同一个一级域名下只能注册一个站点，防止他人抢注子域名。支持非默认端口（如 `example.com:8080`）。
3. **完成验证**：命令会访问 `https://<domain>/` 校验是否包含 congrid 官方链接；链下验证节点也会定期抓取主页确认。
   - **无需押金/质押**：Publisher 注册本身不要求锁仓或抵押。
   - **仍需链上交易手续费（gas fee）**：广播 `publisher register` 这类交易通常需要支付网络手续费（除非链参数允许 0 fee 或使用 `feegrant`）。
4. **查询状态**：注册成功后，可通过 gRPC 查询或 CLI `content-grid-d query registry publisher <domain>` 查看。

### Miner 注册（链上协议部分）

1. 使用 `./content-grid-d miner register <metadata-uri> <services-bitmask> <min-bid-amount> --stake <amount>` 完成矿工资料上链。`services-bitmask` 采用位掩码（例如 `3` 表示同时提供抓取与 embedding），质押与最低报价暂使用同一 denom。
2. 后续可通过 `./content-grid-d miner update --metadata-uri ... --services ... --min-bid-amount ...` 更新服务声明，或使用 `./content-grid-d miner stake <amount> [--decrease]` 调整记录的质押数量。
3. 链上查询接口 `query miners`/`query miner <address>` 会返回当前在线矿工、服务能力、报价以及质押信息，任务调度将直接基于这些状态。

### Verifier Bond（普通地址 + escrow）

Verifier 以普通账户地址（`grid1...`）参与验证网络，先把代币 **bond 到模块托管账户**（escrow）才会被视为 eligible。

- Bond：`./content-grid-d verifier bond <amount> --denom ucongrid --from <key>`
- Unbond：`./content-grid-d verifier unbond <amount> --denom ucongrid --from <key>`

更多见 `docs/verifiers.md`。

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
