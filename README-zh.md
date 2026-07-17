# 内容网格链

内容网格项目的核心协议实现——去中心化的内容网络和搜索协议。

＃＃ 关于
该链（基于 Cosmos SDK）协调 publisher 注册与 verifier 核验网络：
- Publisher 域名上链注册
- verifier 参与验证轮次并提交结果
- 确认 Badge 与归属关系
- 通过链上参数分配 Publisher 与 Verifier 奖励

术语说明：`validator` 专指 Cosmos 共识验证人；`verifier` 专指 ConGrid 的发布者核验角色。

快速链接：有关设计，请参阅 `whitepaper.md`（注意：部分历史内容可能与当前范围不同）；有关经济蓝图，请参阅 `docs/tokenomics-zh.md`；有关贡献指南，请参阅 `AGENTS.md`。

＃＃ 要求
- Go 1.22+（使用最新的稳定版本）
- 推荐类 Unix 环境 (macOS/Linux)

## 构建、运行、测试
- 构建：`go build -o content-grid-d ./cmd/content-grid-d`
- 运行：`./content-grid-d version` 或 `./content-grid-d init`（占位符）
- 测试：`go test ./...`
- 原型：`./scripts/proto-gen.sh` 修改 `proto/` 后重新生成 gRPC 代码

### 本地单节点启动

1. 运行`./content-grid-d devnet --home ./devnet-home --chain-id grid-dev-1`，CLI将自动完成`init → keys add → add-genesis-account → gentx → collect-gentxs`，并使用默认共识验证人密钥（名称`validator`，密钥环后端为`test`）生成单节点创世文件。
2. 执行`./content-grid-d start --home ./devnet-home`启动本地节点。如果需要重新初始化，请附加 `--force` 以清除旧的主目录。

### 本地多节点网络（手动）

下面以3个节点为例。流程为：初始化→生成密钥→在同一创世上生成gentx→收集gentx→分发最终创世→配置端口和互连。

1. 初始化和按键
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

2. 生成地址并加入创世账户（对node1的创世进行操作）
   ```bash
   ADDR1=$(./content-grid-d keys show node1 --home $HOME1 --keyring-backend test --address)
   ADDR2=$(./content-grid-d keys show node2 --home $HOME2 --keyring-backend test --address)
   ADDR3=$(./content-grid-d keys show node3 --home $HOME3 --keyring-backend test --address)

   ./content-grid-d genesis add-genesis-account $ADDR1 100000000ucongrid --home $HOME1
   ./content-grid-d genesis add-genesis-account $ADDR2 100000000ucongrid --home $HOME1
   ./content-grid-d genesis add-genesis-account $ADDR3 100000000ucongrid --home $HOME1
   ```

3. 分发相同的 genesis 副本，然后单独生成 gentx。
   ```bash
   cp $HOME1/config/genesis.json $HOME2/config/genesis.json
   cp $HOME1/config/genesis.json $HOME3/config/genesis.json

   ./content-grid-d genesis gentx node1 1000000ucongrid --chain-id $CHAIN_ID --home $HOME1 --keyring-backend test
   ./content-grid-d genesis gentx node2 1000000ucongrid --chain-id $CHAIN_ID --home $HOME2 --keyring-backend test
   ./content-grid-d genesis gentx node3 1000000ucongrid --chain-id $CHAIN_ID --home $HOME3 --keyring-backend test
   ```

4. 收集gentx并分发最终创世币
   ```bash
   cp $HOME2/config/gentx/*.json $HOME1/config/gentx/
   cp $HOME3/config/gentx/*.json $HOME1/config/gentx/
   ./content-grid-d genesis collect-gentxs --home $HOME1

   cp $HOME1/config/genesis.json $HOME2/config/genesis.json
   cp $HOME1/config/genesis.json $HOME3/config/genesis.json
   ```

5. 配置端口和 seed 发现（以避免本地端口冲突）
- 编辑`config/config.toml`：设置`p2p.laddr`和`rpc.laddr`。
- 编辑`config/app.toml`：设置`api.address`和`grpc.address`。
- 在所有节点的 `[p2p]` 段保持 peer exchange 开启：
   ```toml
   pex = true
   ```
- 本地或私有地址（如 `127.0.0.1`）需要关闭严格可路由检查：
   ```toml
   addr_book_strict = false
   ```
- 端口分配示例：
     - 节点1：p2p `26656`，rpc `26657`，api `1317`，grpc `9090`
     - 节点2：p2p `26666`，rpc `26667`，api `1417`，grpc `9190`
     - 节点3：p2p `26676`，rpc `26677`，api `1517`，grpc `9290`

6. 将 node1 配置为新节点的引导 seed
   ```bash
   NODE1_ID=$(./content-grid-d tendermint show-node-id --home $HOME1)
   echo "${NODE1_ID}@127.0.0.1:26656"
   ```
只在 node2 和 node3 的 `config/config.toml` 的 `[p2p]` 段写入上面输出的 seed 值：
   ```
   seeds = "<NODE1_ID>@127.0.0.1:26656"
   persistent_peers = ""
   ```
node1 不需要配置 node2/node3。node2 和 node3 启动后会先连接 seed，再通过 CometBFT PEX 和地址簿自动发现其他 peer。

7. 分别启动节点（不同终端）
   ```bash
   ./content-grid-d start --home $HOME1
   ./content-grid-d start --home $HOME2
   ./content-grid-d start --home $HOME3
   ```

### 线上正式环境部署（主网）

本仓库尚未提供一键主网上线脚本，主网上线请使用官方发布包（binary + genesis + peers）。运维基线见 `docs/runbook-zh.md` 与 `docs/launch-checklist-zh.md`。

若当前阶段尚无官方发布包，请按 `docs/gentx-zh.md` 的“无发布包场景”流程生成并冻结 final genesis。

1. 构建或下载固定版本的发布包，并确认 `./content-grid-d version`。
2. 初始化节点主目录，用官方主网 `genesis.json` 替换 `config/genesis.json`。
3. 配置 `config/config.toml`（seed peers、p2p/rpc 端口）与 `config/app.toml`（api/grpc、`minimum-gas-prices`）。新节点只需要把 `p2p.seeds` 设置为官方 seed 列表，除非运维上明确需要固定连接，否则保持 `p2p.persistent_peers` 为空。
4. 以服务方式启动节点并完成 RPC/gRPC 健康检查。

容器化 operator 栈见 `docs/docker-operator-zh.md`，其中包含加入现有网络并同时运行 `verifierd` 与其支撑组件的 Docker/Compose 示例。

**运营商保留与发行池代币分配**

发行拆分由创世参数 `app_state.registry.params` 控制，默认与白皮书一致：

```json
"registry": {
  "params": {
    "emission_total_supply": "1000000000000000",
    "operator_reserve_bps": 4000,
    "publisher_emission_bps": 1000,
    "verifier_emission_bps": 5000,
    "emission_duration_hours": 876000
  }
}
```

发行池（发布者 + verifier）由 `tokenomics` 模块账户在运行时维护，在每轮结算时按需补足。若希望在创世时预置发行池余额，需要在 `app_state.bank.balances` 为 `tokenomics` 模块账户添加余额，并同步更新 `app_state.bank.supply`。

运营商保留部分目前未在链上自动分发，请在创世时显式分配（例如分配给多签金库或锁仓账户），可通过 `app_state.bank.balances` 或 `content-grid-d genesis add-genesis-account` 完成。

**其他节点 / 发布者 / verifier 部署**

- 其他全节点：重复主网节点步骤，使用独立 `--home` 和端口，将 `p2p.seeds` 设置为主网 seed 列表，并依赖 PEX/地址簿自动发现其他 peer。
- 发布者：部署站点、挂载验证徽章，并按下方“出版商注册”流程在主网 RPC/gRPC 上执行注册。
- verifier（发布者核验代理）：创建并充值 verifier 地址，执行 `content-grid-d verifier bond` 绑定，再按 `docs/verifierd-zh.md` 部署 `verifierd`。
- 共识验证人：创世期走标准 Cosmos `gentx` 流程；主网运行后可用 `content-grid-d tx staking create-validator`。

### 出版商注册

**官网引导（推荐第三方钱包用户）：** 打开 `https://congrid.net/publishers`，连接第三方钱包（Keplr/Leap）读取 bech32 地址，填写域名 + 钱包地址后可自动生成可粘贴的徽章代码片段和注册命令。

1. **添加Congrid官方链接+归因图片（验证所需）**：您必须在要绑定的网站首页（`/`）添加Congrid官方网站的链接，并且该链接必须用归因图片（徽章）包裹。

目前 verifier 判定规则参见`offchain/registry/verifier.go`：
- 官方网站链接必须为**`<a href="https://congrid.net">`**（或`https://www.congrid.net/`）。 **官网地址本身不允许包含查询/片段**。
- `<img src="...">` 必须包含在 `a` 内。
- `img src` 必须是 `https://congrid.net/...` （或 `https://www.congrid.net/...`）并在 **路径或查询** 中携带：
- `publisher=<your-domain>`（允许不带端口），用于统计归因
- `wallet=<bech32-owner-address>`，必须等于注册交易的所有者地址（`publisher register --from`），用于防止抢注

**推荐格式：**
   ```html
   <a href="https://congrid.net">
     <img
       alt="Verified by Congrid"
       src="https://congrid.net/badge.png?publisher=example.com&wallet=<bech32-owner-address>"
     />
   </a>
   ```

2. **执行注册命令**（或使用 `/publishers` 页面生成的命令）：运行`./content-grid-d publisher register <domain> --from <key-or-address> [--metadata-uri <link>] [--referrer <address>]`。
- `--referrer`：可选，referrer地址（用于影响 verifier 的收益权重；publisher 推荐 publisher 不生效）。
- 系统会自动识别并锁定域名的**一级域名**（Primary Domain，如`example.com`）。
- 同一一级域名下只能注册一个站点，防止他人抢注子域名。支持非默认端口（例如 `example.com:8080`）。
3. **验证完成**：命令会访问`https://<domain>/`来验证是否包含 congrid 官方链接；链下 verifier 代理也会定期抓取主页进行确认。
- **无需押金/质押**：发布者注册本身不需要锁定或质押。
- **手续费策略**：当交易只包含 `MsgRegisterPublisher` 时，可使用 0 手续费提交（例如 `--fees 0ucongrid`）。其他交易类型仍遵循共识验证人最小 gas price 策略，除非使用 `feegrant`。
4. **查询状态**：注册成功后，可以通过gRPC查询或CLI `content-grid-d query registry publisher <domain>`查看。

### verifier 债券（普通地址 + 托管）

verifier 以普通账户地址（`congrid1...`）参与验证网络，首先将代币绑定到模块托管账户 **（escrow）** 后才被认为符合资格。

- 债券：`./content-grid-d verifier bond <amount> --denom ucongrid --from <key>`
- 解绑：`./content-grid-d verifier unbond <amount> --denom ucongrid --from <key>`

有关详细信息，请参阅 `docs/verifiers-zh.md`。

### 经济公用事业

- 使用 `go run ./cmd/tokenomics <subcommand>` 模拟供应、生成创世模板或制作空投表。
- 有关参数详细信息，请参阅 `docs/tokenomics-zh.md`；有关提案流程，请参阅 `docs/governance-zh.md`。

笔记：
- 不要提交构建工件（请参阅 `.gitignore`）。
- CLI 是一个最小的框架，等待完整的服务器/运行时连接。

## 链下组件
- `offchain/indexerd`：发布者主页索引 + 嵌入 + 相似性签名（请参阅`docs/indexerd-zh.md`）。
- `offchain/verifierd`：链驱动的发布者核验代理，并负责指定 round 的 drand 信标投递（请参阅`docs/verifierd-zh.md`和`docs/drand-zh.md`）。

## 项目状态
第一阶段骨架。 `app/` 包为 Cosmos SDK v0.53 提供模块基础知识、编码和默认创世帮助程序。
- `x/registry`：发布者注册和验证逻辑。
- `x/tokenomics`：经济参数、通货膨胀逻辑和结算管理员。

## 路线图（高级）
1) [x] 运行时连接：depinject + `runtime.App`，auth/bank/stake 的守护者，ABCI 服务
2) [x] CLI/服务器：`init`、`start`、配置/主目录管理、密钥
3) [x] Publisher 注册 + Verifier 激励参数
4) [x] 经济/治理：奖励、削减、参数、建议
5) 测试网：可复制的起源、文档和 CI
