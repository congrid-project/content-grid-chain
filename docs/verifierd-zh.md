# verifierd（off-chain）——链驱动的发布者核验

术语说明：这里的 `verifierd` 服务于 ConGrid 的 `verifier` 角色，不是 Cosmos 共识验证人（`validator`）进程。

`verifierd` 是一个链下代理。它会**轮询链上的 assignment**，等待计划开始时间，核验内容，然后通过 commit-reveal 将结果提交到链上。

## 它做什么

- 通过注册表查询 `VerifierAssignments`，为自己的 `verifier_address` 拉取 assignment。
  - 新注册的 publisher 会排到**下一轮**，不会在当前轮立即核验。
- 在发交易前，本地验证 assignment 的确定性（round seed + domain => 期望开始时间）。
  - seed 来源锚定于链元数据：`chain_id`、`round_start`、`anchor_height`、`anchor_hash`。
  - `RoundMeta` 还会公开 `verifier_set_hash` / `verifier_set_size`，便于审计 assignment 输入。
- 等待每个 assignment 的 `startAt` 到来。
- 核验 publisher 首页；若满足以下条件，则判定 **pass**：
  - 页面可访问（HTTP 2xx/3xx）。
  - 首页包含 **Congrid 验证徽章**：
    - `<a href="https://congrid.net">`（或 `https://www.congrid.net/`），且不带 query / fragment。
    - `<a>` 内包裹 `<img>`。
    - `<img src>` 来自 `https://congrid.net/...`，并带有 `publisher=<domain>` 和 `wallet=<owner>`。
  - 若存在活跃租约，首页还必须包含每个活跃租约对应的标记：
    - 包裹元素示例：`<div data-congrid-slot-id="slot-000123" data-congrid-lease="lease-000456"> ... </div>`
    - 被包裹的锚点示例：`<a href="https://advertiser.example/landing">...`
    - `href` 必须与租约 `target_url` 的 host + path 一致。
- 通过 `content-grid-d verifier commit ...` 提交 commit。
- 等待 reveal window 打开后，通过 `content-grid-d verifier reveal ...` 提交 reveal。

> assignment 节奏由链上参数控制：`round_interval_seconds`、`assignment_delay_max_seconds`、`submission_window_seconds`、`commit_window_seconds`。
> - 当 `round_interval_seconds >= 3600` 时，assignment 的开始时间会落在该轮内部的确定性**分钟槽位（0–59）**。
> - 在更短的 dev/e2e 轮次中，开始时间使用受 `assignment_delay_max_seconds` 约束的确定性秒偏移。

## 配置

复制并编辑：

```bash
cp offchain/verifierd/config.example.json offchain/verifierd/config.json
```

关键字段：

- `grpc_addr`：链 gRPC 端点（默认 `127.0.0.1:9090`）
- `verifier_address`：verifier 的 bech32 地址（`congrid1...`）
- `state_dir`：pending commit/reveal 状态目录，用来持久化 nonce，避免进程重启或 tx 等待超时后无法 reveal
- `poll_interval_seconds`：轮询 assignment 的间隔
- `verify_scheme`：`https`（默认）；本地开发可用 `http`
- `commit_start_buffer_seconds`：assignment 开始后等待多久再提交 commit，用来避开区块时间略早于本机时间的边界问题
- `commit_window_seconds`：verifierd 调度使用的本地 commit window，必须与链上参数对齐
- `round_interval_seconds`：本地确定性校验使用的预期轮次间隔（默认 `3600`）
- `assignment_delay_max_seconds`：本地确定性校验使用的最大 assignment 延迟（默认等于 `round_interval_seconds`）
- `disable_assignment_check`：设为 `true` 可跳过 verifierd 的本地确定性校验（默认 `false`）
- `retry_backoff_seconds`：commit/reveal 遇到 sequence mismatch、窗口未打开、tx 等待超时等可重试错误后的退避时间
- `tx_inclusion_timeout_seconds`：提交交易后等待进块并检查 tx `code` 的最长时间
- `submit`：`content-grid-d` 的交易提交配置
  - `binary`
  - `chain_id`、`node`、`from`、`keyring_backend`、`keyring_dir`、`keyring_passphrase_env`、`home`
  - `gas`、`gas_adjustment`、`fees`、`gas_prices`、`broadcast_mode`、`yes`

如果是容器里的无人值守部署，并且使用 `keyring_backend=file`，请把 `submit.keyring_passphrase_env` 设置为一个环境变量名；该环境变量的值就是 keyring 口令。

## 运行

### 一键脚本（Linux/macOS，无 Docker）

脚本会在本机完成构建、key 检查/导入、配置生成和启动：

```bash
cp /path/to/verifier.mnemonic ~/.congrid-verifier.mnemonic
printf '%s\n' '<keyring-passphrase>' > ~/.congrid-verifier.pass
chmod 600 ~/.congrid-verifier.mnemonic ~/.congrid-verifier.pass

cat > .env.verifier <<'EOF'
CONGRID_CHAIN_ID=congrid-main
CONGRID_NODE_RPC_URL=tcp://127.0.0.1:26657
CONGRID_NODE_GRPC_ADDR=127.0.0.1:9090
CONGRID_VERIFIER_KEY_NAME=verifier-key
CONGRID_VERIFIER_KEYRING_BACKEND=file
CONGRID_VERIFIER_KEY_MNEMONIC_FILE=$HOME/.congrid-verifier.mnemonic
CONGRID_VERIFIER_KEYRING_PASSPHRASE_FILE=$HOME/.congrid-verifier.pass
EOF

./scripts/verifier-oneclick.sh --env .env.verifier start
./scripts/verifier-oneclick.sh status
./scripts/verifier-oneclick.sh logs
```

默认行为：

- 构建 `bin/content-grid-d` 和 `bin/verifierd`
- 生成 `offchain/verifierd/config.json`
- 后台启动 `verifierd`，日志写入 `logs/verifierd.out`，PID 写入 `logs/verifierd.pid`
- 如果 key 不存在且未提供 mnemonic，会创建新 key，并把创建输出保存到 `logs/verifier-key.created-key.json`

停止或重启：

```bash
./scripts/verifier-oneclick.sh stop
./scripts/verifier-oneclick.sh restart
```

可选：提交 verifier bond（账户需要已有可用余额）：

```bash
CONGRID_VERIFIER_BOND_AMOUNT=1000000 \
./scripts/verifier-oneclick.sh --env .env.verifier bond
```

立即轮询一次（并等待已经开始的 assignment window）：

```bash
go run ./offchain/verifierd --config offchain/verifierd/config.json --once
```

常驻模式：

```bash
go run ./offchain/verifierd --config offchain/verifierd/config.json
```
