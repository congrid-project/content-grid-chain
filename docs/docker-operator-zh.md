# Docker Operator 栈（节点 + verifier）

仓库现在提供了一套“单一 operator 镜像 + `docker compose` 编排”，用于接入已经存在的 Congrid 网络，并运行节点及 ConGrid verifier 所需的链下组件。

术语说明：

- `validator` 专指 Cosmos 共识验证人。
- `verifier` 专指 ConGrid 的发布者核验角色及其链下进程。

## 包含的服务

- `node`：`content-grid-d` 全节点
- `chromad`：`indexerd` 使用的向量库和嵌入辅助服务
- `indexerd`：发布者索引与相似站点 API
- `verifierd`：链驱动的发布者核验代理，并投递链上指定的 drand 信标

这里的 `verifierd` 指 ConGrid 的发布者核验代理，不是 Cosmos 共识验证人本身；不过同一个 `node` 容器也可以作为共识验证人节点使用，只要你提供正确的质押 / 共识验证人密钥。

## 相关文件

- 编排文件：`docker-compose.operator.yml`
- 环境变量样例：`docker/operator.env.example`
- 创世文件目录：`docker/network/`
- 密钥文件目录：`docker/secrets/`

## 快速开始

1. 复制环境变量模板并编辑：

   ```bash
   cp docker/operator.env.example .env.operator
   ```

2. 准备现网引导信息：
   - 把官方 `genesis.json` 放到 `docker/network/genesis.json`，并保留 `CONGRID_GENESIS_FILE=/network/genesis.json`；或者
   - 直接在 `.env.operator` 中设置 `CONGRID_GENESIS_URL`。
   - 将 `CONGRID_P2P_SEEDS` 设置为官方发布的 seed 列表，节点会通过 CometBFT PEX 和地址簿自动发现其他 peer。
   - 正常运行时保持 `CONGRID_PERSISTENT_PEERS` 为空；只有需要固定连接某个 peer 时才设置。
   - 保持 `CONGRID_P2P_PEX=true`。本地或私有 IP 网络将 `CONGRID_P2P_ADDR_BOOK_STRICT=false`；公开主网/测试网 seed 保持 `true`。

3. 为 `verifierd` 准备签名密钥：
   - `docker/secrets/verifier.mnemonic`
   - `docker/secrets/verifier.passphrase`

4. 启动默认栈：

   ```bash
   docker compose --env-file .env.operator -f docker-compose.operator.yml up -d --build
   ```

drand 投递默认已经随 `verifierd` 启用，不需要额外的 compose profile。

如果你只想在本地运行一个全节点，而不启动 `chromad` / `indexerd` / `verifierd`，可以只构建并启动 `node` 服务：

```bash
docker compose --env-file .env.operator -f docker-compose.operator.yml build node
docker compose --env-file .env.operator -f docker-compose.operator.yml up -d node
```

`node` 服务现在使用轻量 `node-runtime` 构建目标，只包含 `content-grid-d` 和节点入口脚本；其余链下服务仍使用完整的 `operator-runtime` 镜像。

## Keyring 说明

- 样例默认使用 `CONGRID_KEYRING_BACKEND=file`。
- `verifierd` 支持在容器里用 `file` keyring 无人值守签名：口令通过文件注入到环境变量，再由进程读取。
- validator 和 verifier 应使用不同的 keyring 目录。drand 投递复用 verifier signer，不再需要单独的 drand keyring。
- 仅在本地临时测试时，才建议改成 `CONGRID_KEYRING_BACKEND=test` 并把 mnemonic 直接写入 env。

## verifierd 内置 drand 投递

- `CONGRID_DRAND_DELIVERY_DISABLED=false` 默认开启投递。
- `verifierd` 使用普通轮询周期查询链上的唯一 `DrandRequirement`，不会持续上传 latest beacon。
- `CONGRID_DRAND_API_BASE_URL` 默认是 `https://api.drand.sh`。
- 设置 `CONGRID_DRAND_FEE_GRANTER` 后，只有 drand 提交使用该代付账户；详见 `docs/drand-zh.md`。

## verifierd 提交节奏

- 默认 `CONGRID_VERIFIER_COMMIT_START_BUFFER_SECONDS=15`，assignment 开始后延迟 15 秒再提交 commit，避免区块时间略早于本机时间导致 commit 被链上拒绝。
- 默认 `CONGRID_VERIFIER_TX_INCLUSION_TIMEOUT_SECONDS=120`，`verifierd` 会等待交易进块并检查 tx `code`，不会再把“进块但执行失败”的交易误判为成功。
- 默认 `CONGRID_VERIFIER_RETRY_BACKOFF_SECONDS=30`，遇到 sequence mismatch、窗口未打开、tx 等待超时等可重试错误时会退避重试。
- 默认 `CONGRID_VERIFIER_STATE_DIR=/var/lib/congrid/verifierd-state`，commit 的 nonce 会持久化到节点 volume，方便 reveal 在 tx 超时或进程重启后继续使用同一 nonce。

## 共识验证人说明

如果你还希望这个 `node` 同时承担 Cosmos 共识验证人职责：

- 节点需要先同步到目标网络。
- 需要给 operator 账户充值，并用 `content-grid-d tx staking create-validator` 创建共识验证人，或者在创世期按 `gentx` 流程加入。
- 如果你是在迁移已有共识验证人，请在首次启动前把正确的 `priv_validator_key.json` 和 `priv_validator_state.json` 放进节点 home volume。
- 如果你希望把 `create-validator` 用到的 `validator.json` 一并纳入 Docker 管理，可以在 `.env.operator` 里设置：
  - `CONGRID_VALIDATOR_KEY_NAME`
  - `CONGRID_VALIDATOR_KEYRING_DIR`（留空表示沿用 `CONGRID_HOME` / `CONGRID_KEYRING_DIR` 下的默认 keyring）
  - `CONGRID_VALIDATOR_JSON_ENABLE=true`
  - `CONGRID_VALIDATOR_AMOUNT`
  - `CONGRID_VALIDATOR_MONIKER`、`CONGRID_VALIDATOR_IDENTITY`、`CONGRID_VALIDATOR_WEBSITE`、`CONGRID_VALIDATOR_SECURITY`、`CONGRID_VALIDATOR_DETAILS`
  - `CONGRID_VALIDATOR_COMMISSION_RATE`、`CONGRID_VALIDATOR_COMMISSION_MAX_RATE`、`CONGRID_VALIDATOR_COMMISSION_MAX_CHANGE_RATE`
  - `CONGRID_VALIDATOR_MIN_SELF_DELEGATION`
- 启动 `node` 后，入口脚本会用本机的 `content-grid-d comet show-validator` 自动填充 `pubkey`，并将最终 JSON 写到 `CONGRID_VALIDATOR_JSON_PATH`（默认 `/var/lib/congrid/config/validator.json`）。
- 这样生成的文件会跟随节点 volume 持久化，便于后续审计或在容器内执行 `content-grid-d tx staking create-validator /var/lib/congrid/config/validator.json ...`。
- `node` 镜像里还自带了 `congrid-validator-cli`，会自动复用上面的 validator env，便于统一执行常见运维命令。

常用示例：

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

## 健康检查

- Node RPC：宿主机 `${CONGRID_RPC_PORT}` -> 容器 `26657`
- Node gRPC：宿主机 `${CONGRID_GRPC_PORT}` -> 容器 `9090`
- Indexerd HTTP：宿主机 `${CONGRID_INDEXER_PORT}` -> 容器 `9100`
- verifierd readiness HTTP：宿主机 `${CONGRID_VERIFIER_HEALTH_PORT}` -> 容器 `9200`；响应包含 `drand_*` 状态
- `chromad` 默认只在 compose 内网暴露，不映射到宿主机
