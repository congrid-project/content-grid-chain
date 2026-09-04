# 为 Content Grid Chain 贡献代码

感谢你参与改进 Content Grid Chain。本文集中说明开发环境、构建、测试和本地网络；面向协议参与者的内容请参阅 [README](README-zh.md)。

[English](CONTRIBUTING.md)

## 环境要求

- Go 1.25.1，或与 `go.mod` 兼容的更新补丁版本
- Git 与类 Unix 环境，推荐 macOS 或 Linux
- 足够存放 Go module 和构建缓存的磁盘空间

仓库目标不要求单独安装 Protobuf 编译器；Makefile 会运行固定版本的 Go Buf 工具。

## 仓库结构

- `app/`：Cosmos SDK 运行时装配、编码、Keeper、genesis、ante handler 与升级逻辑
- `cmd/content-grid-d/`：节点 daemon、客户端命令与开发网络辅助命令
- `cmd/congrid-site/`：Publisher 引导、Dashboard、下载与链接市场 Web 应用
- `cmd/tokenomics/`：供应模拟、genesis 模板与空投 CLI
- `x/registry/`：Publisher、核验轮次、drand、相似站点证据、slot 与 lease
- `x/verifiers/`：Verifier 质押托管与资格管理
- `x/tokenomics/`：发行池和结算辅助逻辑
- `x/nodes/`：节点 genesis 模型；当前运行时功能有限
- `offchain/`：Verifier、Indexer、DHT 与配套服务
- `proto/`：Content Grid Protobuf 与 gRPC 定义
- `scripts/`：代码生成、发布、测试网络与 Tokenomics 工具
- `docs/`：协议、运维和组件文档
- `third_party/`：仓库固定的兼容性代码；除非变更明确需要，否则不要修改

## 本地构建与运行

下载依赖，并在二进制中写入版本和 commit 信息：

```bash
go mod download
make build
./content-grid-d version
./content-grid-d version --long
```

`make build` 会在仓库根目录生成 `content-grid-d`。该文件已被 Git 忽略，不应提交。

### 单节点开发网络

创建一个独立的单验证人网络：

```bash
./content-grid-d devnet \
  --home ./devnet-home \
  --chain-id grid-dev-1

./content-grid-d start --home ./devnet-home
```

`devnet` 会依次执行 `init`，用 `test` keyring 创建默认 `validator` 密钥，添加 genesis 账户，生成 `gentx` 并收集 genesis 交易。需要重建时可加入 `--force`；该参数会清空指定的 devnet home，切勿指向包含重要密钥或数据的目录。

### 手动三节点网络

需要验证 peer 发现、多共识验证人或网络级行为时，可以使用以下配置。

1. 构建并初始化三个节点目录：

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

2. 把三个验证人账户都加入 node1 的 genesis：

   ```bash
   NODE1_ADDR=$(./content-grid-d keys show node1 --home "$NODE1_HOME" --keyring-backend test --address)
   NODE2_ADDR=$(./content-grid-d keys show node2 --home "$NODE2_HOME" --keyring-backend test --address)
   NODE3_ADDR=$(./content-grid-d keys show node3 --home "$NODE3_HOME" --keyring-backend test --address)

   ./content-grid-d genesis add-genesis-account "$NODE1_ADDR" 100000000ucongrid --home "$NODE1_HOME"
   ./content-grid-d genesis add-genesis-account "$NODE2_ADDR" 100000000ucongrid --home "$NODE1_HOME"
   ./content-grid-d genesis add-genesis-account "$NODE3_ADDR" 100000000ucongrid --home "$NODE1_HOME"
   ```

3. 分发相同的 genesis，再分别创建 `gentx`：

   ```bash
   cp "$NODE1_HOME/config/genesis.json" "$NODE2_HOME/config/genesis.json"
   cp "$NODE1_HOME/config/genesis.json" "$NODE3_HOME/config/genesis.json"

   ./content-grid-d genesis gentx node1 1000000ucongrid --chain-id "$CHAIN_ID" --home "$NODE1_HOME" --keyring-backend test
   ./content-grid-d genesis gentx node2 1000000ucongrid --chain-id "$CHAIN_ID" --home "$NODE2_HOME" --keyring-backend test
   ./content-grid-d genesis gentx node3 1000000ucongrid --chain-id "$CHAIN_ID" --home "$NODE3_HOME" --keyring-backend test
   ```

4. 把 `gentx` 汇总到 node1，收集后再次分发最终 genesis：

   ```bash
   cp "$NODE2_HOME"/config/gentx/*.json "$NODE1_HOME/config/gentx/"
   cp "$NODE3_HOME"/config/gentx/*.json "$NODE1_HOME/config/gentx/"
   ./content-grid-d genesis collect-gentxs --home "$NODE1_HOME"

   cp "$NODE1_HOME/config/genesis.json" "$NODE2_HOME/config/genesis.json"
   cp "$NODE1_HOME/config/genesis.json" "$NODE3_HOME/config/genesis.json"
   ```

5. 为每个节点配置不重复的端口：

   | 节点 | P2P | RPC | API | gRPC |
   | --- | ---: | ---: | ---: | ---: |
   | node1 | 26656 | 26657 | 1317 | 9090 |
   | node2 | 26666 | 26667 | 1417 | 9190 |
   | node3 | 26676 | 26677 | 1517 | 9290 |

   在各节点的 `config/config.toml` 中修改 `p2p.laddr` 和 `rpc.laddr`，在 `config/app.toml` 中修改 `api.address` 和 `grpc.address`。保持 `pex = true`；使用回环或私网地址时，设置 `addr_book_strict = false`。

6. 把 node1 配置为 node2 和 node3 的 seed：

   ```bash
   NODE1_ID=$(./content-grid-d tendermint show-node-id --home "$NODE1_HOME")
   echo "${NODE1_ID}@127.0.0.1:26656"
   ```

   将输出值写入 node2 和 node3 的 `[p2p]` 段：

   ```toml
   seeds = "<NODE1_ID>@127.0.0.1:26656"
   persistent_peers = ""
   ```

7. 在不同终端启动三个节点：

   ```bash
   ./content-grid-d start --home "$NODE1_HOME"
   ./content-grid-d start --home "$NODE2_HOME"
   ./content-grid-d start --home "$NODE3_HOME"
   ```

所有节点必须使用字节完全一致的最终 `genesis.json`。CometBFT node ID 只是网络身份，不是完整的 seed 地址；应始终使用 `<node-id>@<host>:<p2p-port>` 格式。

更完整的首发网络注意事项见 [gentx 与最终 genesis 指南](docs/gentx-zh.md)。

## 测试与质量检查

提交变更前运行完整检查：

```bash
make test
make lint
go test ./... -cover
```

开发时可以只运行单个 package 或测试：

```bash
go test ./x/registry
go test ./x/registry -run TestName
```

多个场景共用初始化逻辑时优先使用表驱动测试。新增共识或结算行为时，应覆盖成功、拒绝、时间边界和确定性重放。修改 `app/`、`x/registry`、`x/verifiers` 或 `x/tokenomics` 时应加入回归测试。

## 格式化与生成代码

格式化 Go 和 Protobuf 文件：

```bash
make format
```

修改 `proto/` 后，重新生成并提交 Go 与 gRPC 代码：

```bash
make proto
```

请检查生成结果，并将其与源 `.proto` 变更一同提交。不要手工修改生成的 `*.pb.go` 文件。

Go import 按标准库、第三方、本地代码分组。Package 名保持小写，导出标识符使用 `UpperCamelCase`。错误应带有上下文并使用 `%w` 包装；调用方需要区分错误时优先使用 sentinel error。

## Tokenomics 工具

开发者使用的供应模拟、genesis 模板与空投命令位于 `cmd/tokenomics`：

```bash
go run ./cmd/tokenomics --help
go run ./cmd/tokenomics simulate --years 5 --bonded 0.6
```

完整示例见 [Tokenomics 工具指南](scripts/tokenomics/README-zh.md)。

## 协议变更检查表

影响共识的变更需要额外检查：

- 保证迭代和排序的确定性；不能让 Go map 的遍历顺序决定状态转换、抽样、奖励或哈希。
- ABCI 执行路径中不得访问网络或文件系统；链下抓取应放在 `offchain/`。
- 修改 genesis 或参数时加入校验，并考虑存量状态的 migration 与 upgrade 路径。
- 消息、查询、事件、状态转换、时间窗口、奖励或惩罚发生变化时同步更新文档。
- 条件允许时同步维护中英文用户文档。
- 修改 Protobuf、Keeper、EndBlock、ante handler 或应用装配后运行完整测试。

## Commit 与 Pull Request

建议使用祈使语气和清晰 scope，例如：

```text
registry: reject duplicate verification reveals
app: export tokenomics genesis state
```

Pull Request 应说明目的、行为范围、已运行的测试、兼容性或迁移影响，以及后续工作。适用时使用 `Fixes #123` 关联 issue。只有用户可见的 CLI 或渲染文档变更需要视觉说明时才添加截图。

提交贡献即表示你同意源代码与文档按仓库的 [MIT License](LICENSE) 提供。如果贡献包含受保护的非代码生态知识产权，请同时阅读 [Chain Inspiring License v1.0](CHAIN-INSPIRING-LICENSE.md)。
