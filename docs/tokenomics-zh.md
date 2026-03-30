# 内容网格 Tokenomics

本文档描述仓库当前实现的**链上行为**。

## 供应与分配基线

- 单位：`ucongrid`（1 CONGRID = 1,000,000 `ucongrid`）
- 参考总供给参数：**1,000,000,000 CONGRID**（`1_000_000_000_000000 ucongrid`）
- 默认排放拆分（registry 参数）：
  - operator reserve：**40%**（`operator_reserve_bps=4000`）
  - publisher emission：**10%**（`publisher_emission_bps=1000`）
  - verifier emission：**50%**（`verifier_emission_bps=5000`）
- 默认排放时长：**100 年**（`876,000` 小时）

## 每小时排放（当前默认值）

在 `round_interval_seconds=3600` 时，默认值对应：

- 每小时 publisher 池：**114,155,251 ucongrid**（约 114.155251 CONGRID）
- 每小时 verifier 池：**570,776,255 ucongrid**（约 570.776255 CONGRID）

这些值由 `PublisherParams.RoundEmissionPools(...)` 计算。

## 奖励如何发放（当前实现）

注册表核验奖励会在 `x/registry` 的 round finalization 阶段结算。

1. 链先计算每轮的 publisher / verifier 奖励池。
2. `tokenomics` 模块负责确保并持有排放池余额。
3. 奖励通过模块池转账（`SendFromPool`）发出，而不是按接收者逐个铸造。
4. 无人领取的部分会从池中烧毁（`BurnFromPool`）。

## publisher 奖励规则

- publisher 池会在该轮所有活跃 assignment 间平均拆分。
- publisher 只有在满足外链门槛时才能领取完整份额：
  - `required_external_links_for_full_reward`（默认 `15`）
- 若低于门槛，则按比例领取。
- 未被领取的 publisher 奖励会被烧毁。

## verifier 奖励规则

只有在最终通过多数轮次中成功提交结果的 verifiers 才有资格领取奖励。

每个 assignment 的 verifier 奖励采用混合拆分：

1. **基础份额**（`verifier_reward_base_share_bps`，默认 `4000`，即 40%）
   - 由成功的 verifiers 平均分配。
2. **加权份额**（剩余 60%）
   - 按下式分配：

`weight = bonded_stake × referral_factor`

其中 `referral_factor` 基于活跃的被推荐 publisher 数量计算，最小为 1。

- 基础份额有利于小规模 operator / verifier 参与。
- 加权份额保留了基于 stake 的 Sybil 抵抗能力。
- 如果某个 assignment 中没有符合条件的 verifier，则该 assignment 的 verifier 池会被烧毁。
- 如果加权份额不存在正权重 verifier，则该部分剩余奖励也会被烧毁。

## 范围说明

- 固定发行拆分（operator / publisher / verifier）、费用路由和 slash 路由相关的 tokenomics 参数已经存在并经过校验。
- 一些白皮书层面的流程，例如完整的消费者支付路径、完整的 slash 补偿路径，以及自动化的 operator reserve 分发，仍处于部分建模状态，尚未完全接入端到端生产结算。

参见：
- `x/registry/types.go`
- `x/registry/verification_rounds.go`
- `x/tokenomics/keeper.go`
