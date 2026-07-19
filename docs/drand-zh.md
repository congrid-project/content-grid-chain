# drand 随机信标

drand 投递已经并入 `verifierd`，不再运行独立的 `drand-relayer`。多个 verifier 可以同时承担投递职责；链上状态决定唯一可接受的 drand round，先成功提交者生效。

## 链上规则

启用 `registry.params.drand_enabled` 后：

1. 链为下一次 Content Grid verification round 计算一个固定的 drand round。
2. `MsgSubmitDrandBeacon` 只接受该 round，拒绝历史、未来或重复 round。
3. 链使用固定的 scheme 和 distributed public key 验证 BLS 签名，并验证 `randomness = SHA256(signature)`。
4. 指定信标尚未上链时不会创建 assignment，也不会回退到 block hash。
5. 信标成功后，`EndBlock` 使用该信标与链上 anchor 生成 seed，再确定性地创建 assignment。

round 映射公式：

```text
latest_allowed_time = content_round_start - drand_round_offset_seconds
required_drand_round = floor((latest_allowed_time - drand_genesis_time_unix) / drand_period_seconds) + 1
```

默认元数据对应 drand quicknet：

- `drand_chain_hash`: `52db9ba70e0cc0f6eaf7803dd07447a1f5477735fd3f661792ba94600c84e971`
- `drand_scheme_id`: `bls-unchained-g1-rfc9380`
- `drand_genesis_time_unix`: `1692803367`
- `drand_period_seconds`: `3`
- `drand_round_offset_seconds`: `60`
- `drand_public_key_hex`: 见 `DefaultDrandPublicKeyHex` 或示例 genesis

quicknet 元数据及链特定 HTTP 路径可对照 [drand quicknet 公告](https://docs.drand.love/blog/2023/10/16/quicknet-is-live/) 和 [drand HTTP API](https://docs.drand.love/developer/http-api/)。

`drand_strict_mode` 仅为旧 genesis JSON 兼容而保留；启用 drand 后现在始终使用严格模式。

查询当前唯一需求：

```bash
content-grid-d query registry drand-requirement
```

## verifierd 配置

`offchain/verifierd/config.json`：

```json
{
  "drand": {
    "disabled": false,
    "api_base_url": "https://api.drand.sh",
    "request_timeout_seconds": 10,
    "fee_granter": ""
  }
}
```

`verifierd` 每次普通轮询时都会查询 `DrandRequirement`。只有当链上有待满足的需求、指定 drand round 已到发布时间且尚未提交时，才请求 `/{chain-hash}/public/{round}` 并发交易；它不会轮询或上传 `latest` 信标。

多个 `verifierd` 可以安全并行运行，但不再默认同时抢跑。每个实例会根据
`required_drand_round`、链上活跃 verifier 地址及其 bond 计算相同的质押加权主
投递者，再确定性排列其余后备：排名 0 立即投递，后备实例按
`relay_stagger_seconds`（默认 60 秒）依次获得投递资格。质押加权使主投递概率与
奖励中的 Sybil 抵抗权重保持一致。
等待时间达到 `relay_max_delay_seconds`（默认 180 秒）后，剩余后备可以竞速，
以便主投递者离线时最终仍能恢复 assignment 创建。链上仍执行严格验签和单次接收。

默认交易配置使用 `gas=250000` 和 `gas_prices=0.001ucongrid`，即每笔按
`250ucongrid` 计算。小时轮次正常情况下全网每天只有约 24 笔 drand 交易，费用约
`6000ucongrid`（0.006 CONGRID），不会随 verifier 数量线性增长。已有配置如果仍是
`fees=5000ucongrid`，需要显式改成：

```json
{
  "drand": {
    "relay_stagger_seconds": 60,
    "relay_max_delay_seconds": 180
  },
  "submit": {
    "gas": "250000",
    "fees": "",
    "gas_prices": "0.001ucongrid"
  }
}
```

`fees` 与 `gas_prices` 只能设置一个。`gas_prices` 不得低于验证人节点实际配置的
`minimum-gas-prices`。

## 让 verifierd 不承担 drand 交易费

应用已经启用 Cosmos SDK `feegrant`。运营方可以向 verifier 账户授予仅允许 `MsgSubmitDrandBeacon` 的额度，并把 grantor 地址写入 `drand.fee_granter`（Docker 为 `CONGRID_DRAND_FEE_GRANTER`）。这样 drand 交易从 grantor 额度扣费，verifier 账户余额不减少。

建议限制：

- allowed message：`/contentgrid.registry.v1.MsgSubmitDrandBeacon`
- 有效期和总额度
- 仅向实际运行 `verifierd` 的账户授权

不要把当前消息直接做成无条件零手续费交易；虽然链只保存指定 round，但无效 BLS 签名仍会消耗验证 CPU。

## 从旧组件迁移

1. 停止并删除旧 `drand-relayer` 服务、密钥和健康端口。
2. 更新 `verifierd` 配置，加入 `drand` 段。
3. 如需代付，建立 fee grant 并设置 `drand.fee_granter`。
4. 已运行的链执行固定名称为 `drand-strict-v2` 的软件升级；完整步骤见 [`upgrade-drand-strict-v2-zh.md`](upgrade-drand-strict-v2-zh.md)。
5. 启动 `verifierd`，检查 `/readyz` 中的 `drand_*` 字段。
