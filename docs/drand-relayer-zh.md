# drand-relayer（链下）

`drand-relayer` 轮询 drand 公共 API 并通过以下方式提交最新的链上信标：

- `tx registry submit-drand-beacon`

当 `registry` 参数启用 drand 混合以实现分配随机性时，这是必需的。

## 配置

复制并编辑：

```bash
cp offchain/drandrelayer/config.example.json offchain/drandrelayer/config.json
```

关键配置：
- `drand_api_base_url`：默认`https://api.drand.sh`
- `drand_chain_hash`: drand 网络链哈希（示例中默认为quicknet）
- `poll_interval_seconds`：轮询 drand 的频率
- `min_submit_interval_seconds`：两次成功上链提交之间的最小间隔。长期运行时不要按 drand 原生轮次频率逐轮上链。
- `retry_backoff_seconds`：遇到 account sequence mismatch 等可重试交易错误后的退避时间。
- `tx_inclusion_timeout_seconds`：提交交易后等待进块的最长时间。
- `max_submit_retries`：单轮即时重试次数。无人值守长期运行建议设为 `1`，让外层退避处理 sequence/cache 竞争。
- `submit.*`：`content-grid-d` 的链 tx 设置

## 运行

一次性同步：

```bash
go run ./offchain/drandrelayer --config offchain/drandrelayer/config.json --once
```

长时间运行模式：

```bash
go run ./offchain/drandrelayer --config offchain/drandrelayer/config.json
```

## 注释

- 仅当 drand 最新一轮比链上最新一轮更新时，Relayer 才会提交。
- 默认长期运行节奏是每 60 秒轮询一次，最多每 300 秒提交一次。这样对小时级 assignment round 足够新鲜，也不会为每个 drand quicknet round 都支付手续费。
- 链上 `MsgSubmitDrandBeacon` 强制执行信标验证（包括启用/严格 drand 模式时的签名验证）。
