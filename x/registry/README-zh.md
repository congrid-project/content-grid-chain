# 注册表模块（`x/registry`）

registry 模块保存发布者网站、验证轮次和 assignment、commit-reveal 提交、
广告位与租约。

## 验证轮次

- 新注册或到期的发布者在确定性的轮次边界进入 assignment。
- 链根据已保存的 round seed，从合格 verifier 集合中确定性选人。
- verifier 在配置的时间窗口内 commit 和 reveal；结算会更新发布者状态及
  verifier 奖励。

## drand 随机数

启用 `drand_enabled` 后，链为下一次 Content Grid 验证轮次指定唯一的
drand round。`MsgSubmitDrandBeacon` 仅接收该 round 一次，强制要求签名，
并使用配置的 BLS scheme 和 distributed public key 验证。指定信标缺失时，
链会等待，不会回退为只使用 block hash 的随机数。

查询当前需求：

```bash
content-grid-d query registry drand-requirement
```

信标投递已并入 `offchain/verifierd`，独立 relayer 已移除。round 映射公式、
链参数、部署和手续费代付说明见 [`docs/drand-zh.md`](../../docs/drand-zh.md)。
