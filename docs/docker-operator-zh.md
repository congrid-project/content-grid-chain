# Docker Operator 栈（节点 + verifier）

仓库现在提供了一套“单一 operator 镜像 + `docker compose` 编排”，用于接入已经存在的 Congrid 网络，并运行节点及 ConGrid verifier 所需的链下组件。

术语说明：

- `validator` 专指 Cosmos 共识验证人。
- `verifier` 专指 ConGrid 的发布者核验角色及其链下进程。

## 包含的服务

- `node`：`content-grid-d` 全节点
- `chromad`：`indexerd` 使用的向量库和嵌入辅助服务
- `indexerd`：发布者索引与相似站点 API
- `verifierd`：链驱动的发布者核验代理
- `drand-relayer`：可选 profile，用于上链 drand 信标

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
   - 同时填入当前网络的 `CONGRID_P2P_SEEDS` 和/或 `CONGRID_PERSISTENT_PEERS`。

3. 为 `verifierd` 准备签名密钥：
   - `docker/secrets/verifier.mnemonic`
   - `docker/secrets/verifier.passphrase`

4. 启动默认栈：

   ```bash
   docker compose --env-file .env.operator -f docker-compose.operator.yml up -d --build
   ```

5. 如果该节点还要运行 `drand-relayer`，带上 profile：

   ```bash
   docker compose --env-file .env.operator -f docker-compose.operator.yml --profile drand up -d --build
   ```

## Keyring 说明

- 样例默认使用 `CONGRID_KEYRING_BACKEND=file`。
- `verifierd` 和 `drand-relayer` 已支持在容器里用 `file` keyring 无人值守签名：口令通过文件注入到环境变量，再由进程读取。
- 仅在本地临时测试时，才建议改成 `CONGRID_KEYRING_BACKEND=test` 并把 mnemonic 直接写入 env。

## 共识验证人说明

如果你还希望这个 `node` 同时承担 Cosmos 共识验证人职责：

- 节点需要先同步到目标网络。
- 需要给 operator 账户充值，并用 `content-grid-d tx staking create-validator` 创建共识验证人，或者在创世期按 `gentx` 流程加入。
- 如果你是在迁移已有共识验证人，请在首次启动前把正确的 `priv_validator_key.json` 和 `priv_validator_state.json` 放进节点 home volume。

## 健康检查

- Node RPC：宿主机 `${CONGRID_RPC_PORT}` -> 容器 `26657`
- Node gRPC：宿主机 `${CONGRID_GRPC_PORT}` -> 容器 `9090`
- Indexerd HTTP：宿主机 `${CONGRID_INDEXER_PORT}` -> 容器 `9100`
- `chromad` 默认只在 compose 内网暴露，不映射到宿主机
