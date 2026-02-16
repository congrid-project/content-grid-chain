# drand-relayer（链下）

`drand-relayer` 轮询 drand 公共 API 并通过以下方式提交最新的链上信标：

- __代码_0__

当 `registry` 参数启用 drand 混合以实现分配随机性时，这是必需的。

## 配置

复制并编辑：

```bash
cp offchain/drandrelayer/config.example.json offchain/drandrelayer/config.json
```

关键领域：
- `drand_api_base_url`：默认`https://api.drand.sh`
- `drand_chain_hash`: drand 网络链哈希（示例中默认为quicknet）
- `poll_interval_seconds`：轮询 drand 的频率
- `submit.*`：`content-grid-d` 的链 tx 设置

＃＃ 跑步

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
- 链上 `MsgSubmitDrandBeacon` 强制执行信标验证（包括启用/严格 drand 模式时的签名验证）。
