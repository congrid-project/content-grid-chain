# 验证者（债券、分配、奖励）

验证者是绑定到托管的普通帐户地址 (`grid1...`)。

## 绑定/解除绑定

纽带：
```bash
./content-grid-d verifier bond 1000000 --denom ucongrid --from <key>
```

解除绑定：
```bash
./content-grid-d verifier unbond 500000 --denom ucongrid --from <key>
```

列出作业：
```bash
./content-grid-d verifier assignments --from <key>
```

提交/揭示：
```bash
./content-grid-d verifier commit example.com --passed --nonce <nonce> --from <key>
./content-grid-d verifier reveal example.com --passed --nonce <nonce> --from <key>
```

## 最终确定规则（当前）

- 回合结果基于多数+法定人数。
- 发布者状态转换：
  - 法定人数通过多数：`PENDING/VERIFIED -> VERIFIED`
  - 仲裁多数失败：`VERIFIED -> PENDING`，阈值失败后可能变为 `REVOKED`

## 验证者奖励规则（当前）

只有在最终通过多数轮次中成功的验证者才能参与验证者支付。

每个验证者的权重：

__代码_0__

- `bonded_stake`：验证者模块中的验证者绑定
- `referral_factor`：活跃推荐发布商因素（最小 1）

因此，支出与权益和推荐活动成正比。

## 处罚

- 错过提交或投票反对多数会增加处罚次数。
- 重复处罚可能会导致临时暂停任务。

## 影响刻录的发行商端门控

发布商份额按回合分配平均分配，但实际发布商声明由所需的外部链接阈值 (`required_external_links_for_full_reward`) 限制。
无人认领的发行商金额将从池中销毁。
