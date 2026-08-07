# verifier（债券、分配、奖励）

术语说明：本文中的 `verifier` 指 ConGrid 的发布者核验角色，不是 Cosmos 共识验证人（`validator`）。

verifier 使用普通账户地址（`congrid1...`）参与网络，并先将代币 bond 到模块 escrow。

## Bond / Unbond

Bond：
```bash
./content-grid-d verifier bond 1000000 --denom ucongrid --from <key>
```

Unbond：
```bash
./content-grid-d verifier unbond 500000 --denom ucongrid --from <key>
```

查询分配：
```bash
./content-grid-d verifier assignments --from <key>
```

提交 / 揭示：
```bash
./content-grid-d verifier commit example.com --passed --nonce <nonce> --from <key>
./content-grid-d verifier reveal example.com --passed --nonce <nonce> --from <key>
```

## 分配规则（当前）

分配是确定性的，并按 stake 加权：

- 候选集合：活跃、未暂停、且满足最小 bond 要求的 verifiers。
- 选择方式：确定性的加权随机抽样，且**无放回**。
- 权重：verifier 的 bonded stake（`bond.amount`）。

这意味着 bonded stake 越高，被分配到任务的概率越高；同时整个过程仍可由链上 round seed 和 verifier 集合审计。

## 最终确定规则（当前）

- 回合结果基于多数 + 法定人数。
- 发布者状态转换：
  - 通过多数且达到法定人数：`PENDING/VERIFIED -> VERIFIED`
  - 失败多数且达到法定人数：`VERIFIED -> PENDING`，达到阈值后可进一步变为 `REVOKED`

## verifier 奖励规则（当前）

只有在最终通过多数轮次中成功提交结果的 verifiers 才能参与 verifier 奖励分配。

单个 assignment 的 verifier 奖励分为两个桶：

1. **基础桶**（默认 `verifier_reward_base_share_bps = 4000`，即 40%）
   - 在该 assignment 中由成功的 verifiers 平均分配。
2. **加权桶**（剩余 60%）
   - 按下式分配：

`weight = bonded_stake × referral_factor`

- `bonded_stake`：verifier 模块中的 bond 数量。
- `referral_factor`：活跃推荐 publisher 因子（最小为 1）。

如果某个 assignment 中不存在正权重 verifier，则该 assignment 的加权桶部分会从发行池烧毁。

## 处罚

- 错过提交，或投票结果与多数结论相反，会增加处罚计数。
- 重复处罚可能触发临时 assignment suspension。

## 影响销毁的 publisher 侧门槛

publisher 池会在通过徽章验证的 assignment 之间平均拆分。每个活跃 publisher 再按 `max(publisher_min_reward_bps, matched_links / required_external_links_for_full_reward)` 领取，最高不超过其基准份额的 100%。默认保底比例为 10%，15 条链接可领取完整份额；相似链接只影响奖励比例，不影响徽章通过状态。
未被领取的 publisher 奖励会从池中烧毁。
