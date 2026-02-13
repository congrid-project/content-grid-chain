# 上线检查清单（Launch Checklist）

> 目标：把当前“可灰度上线”状态推进到“可公开生产上线”。
> 用法：逐项打勾，最后由负责人做 Go/No-Go 决策。

## A. 当前验证快照（2026-02-11）

- ✅ `go test ./...` 全绿
- ✅ e2e smoke 已连续通过（最近多轮，含 publisher verify → slot → lease → lease-anchor reverify）
- ✅ commit–reveal 已进入可稳定运行状态（含重试与串行提交保护）
- ✅ congrid-site tx 输出解析已加固（不再依赖直接 `json.Unmarshal(out)`）
- ✅ 文档已与当前实现对齐（marketplace / verifierd）

> 结论：当前可进入灰度上线；要到公开生产上线，剩余关键项在“密钥策略 + 运维告警落地”。

---

## 0. 发布信息（本次建议填写）

- [ ] 发布版本号：`v0.1.0-rc1`（建议）
- [x] 发布分支：`main`
- [ ] 变更窗口（时区/开始时间/结束时间）已确认（`America/Toronto`）
- [ ] 回滚版本号已确认：`4202f66`（当前稳定基线建议）
- [x] 负责人（链上 / 后端 / 运维 / 值班）已明确（临时同一负责人）

当前负责人（临时）：
- 链上负责人：`eking`
- 应用负责人：`eking`
- 运维负责人：`eking`
- 值班通知：`Telegram @eking (id:6148992071)`

---

## 1. 代码与质量门禁（P0）

- [x] `go test ./...` 全绿
- [x] e2e smoke 连续通过（建议 `>= 5` 次）
  - [x] publisher verify
  - [x] slot create/list
  - [x] lease escrow
  - [x] lease-anchor reverify
- [ ] 关键改动已 code review（至少 1 名 reviewer）
- [ ] 依赖版本已冻结（tag 或 commit pin）
- [x] 产物可重复构建（`content-grid-d` / `verifierd` / `congrid-site`）

---

## 2. 链参数与治理门禁（P0）

### 2.1 生产参数基线（建议 v1）

> 若你有 governance 参数模板，可直接复制覆盖此节。

- [ ] `verifier_bond = 50000000`（50 CONGRID）
- [ ] `min_verifier_count = 3`
- [ ] `verification_ttl = 2000`
- [ ] `commit_window_seconds = 300`
- [ ] `submission_window_seconds = 600`
- [ ] `round_interval_seconds = 3600`
- [ ] `assignment_delay_max_seconds = 300`
- [ ] `cooldown_base_seconds = 604800`
- [ ] `publisher_verification_reward = 1000000`
- [ ] `verifier_verification_reward = 500000`

> 说明：e2e 快速参数（如 12/28/10/2）仅用于本地验证，不建议直接用于生产。

### 2.2 参数约束校验（必须）

- [ ] `commit_window_seconds < submission_window_seconds`
- [ ] `commit_window_seconds < verification_ttl`
- [ ] `assignment_delay_max_seconds <= round_interval_seconds`
- [ ] 以上约束已在目标网络实际配置中验证

### 2.3 治理流程

- [ ] 参数变更流程（提案、审批、执行）已演练
- [ ] 参数快照已存档（发布前 / 发布后）

---

## 3. 密钥与安全门禁（P0，最后硬阻断）

- [ ] 生产密钥策略文档已签字确认
- [ ] `congrid-site` 交易签名路径隔离（最小权限）
- [ ] signer 运行环境（内网/专机）已确认
- [ ] keyring/密钥权限（文件权限、sudo 策略）已锁定
- [ ] 密钥备份与恢复演练完成
- [ ] 密钥轮换流程可执行（计划周期：`每 90 天` 建议）
- [ ] 密钥吊销应急流程可执行（触发条件与责任人明确）

---

## 4. 观测与告警（P1）

### 4.1 verifierd 指标阈值（建议）

- [ ] assignment 拉取成功率（5min）`>= 99%`
- [ ] commit 成功率（15min）`>= 95%`
- [ ] reveal 成功率（15min）`>= 90%`
- [ ] 失败原因分布可见（sequence mismatch / reveal window / commit window）

### 4.2 链上业务告警阈值（建议）

- [ ] 新注册 publisher `PENDING > 3h` 告警
- [ ] cooldown 触发率较 24h 均值上升 `> 2x` 告警
- [ ] lease `VIOLATED/REFUNDED` 比例 `> 10%`（1h 窗口）告警

### 4.3 观测基础设施

- [ ] 日志采集与检索（node/verifierd/site）已可用
- [ ] 告警通知链路（Telegram/电话/值班）已验证

---

## 5. 运维与回滚（P0）

- [x] `docs/runbook.md` 已按当前环境填实
- [ ] 值班同学已演练启动/重启/回滚
- [ ] 回滚脚本/步骤已演练（含配置回退）
- [ ] 故障分级（SEV1/2/3）和响应时限明确
- [ ] 发布后观察窗口（至少 24h）已安排值班

---

## 6. 灰度与放量策略（P1）

- [ ] 灰度范围（初始 publisher/verifier 数量）已定义
- [ ] 灰度成功标准已定义
- [ ] 放量阈值与暂停阈值已定义
- [ ] 一键暂停策略可用（必要时暂停新 slot/lease）

---

## 7. 文档与对外沟通

- [x] `docs/marketplace.md` 与实现一致
- [x] `docs/verifierd.md` 与实现一致
- [ ] 对外使用说明（CLI / API / 限制）已发布
- [ ] 已知限制与风险已公开

---

## 8. Go / No-Go 结论

- [ ] Go
- [ ] No-Go

**决策人：**
- 链上负责人：`eking`
- 应用负责人：`eking`
- 运维负责人：`eking`
- 最终批准：`eking`

**决策时间：** `YYYY-MM-DD HH:mm America/Toronto`
