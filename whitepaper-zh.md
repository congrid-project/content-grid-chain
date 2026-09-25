# 内容网格协议白皮书

当前实现对齐版 · 2026-09-24 · 代码基线：`905bb09`

[English](whitepaper.md) · [使用指南](README-zh.md) · [贡献者指南](CONTRIBUTING-zh.md)

## 1. 目标与范围

Content Grid（Congrid）是连接独立网站的内容发现协议。发布者注册域名，通过首页 Badge 证明网站控制权，并展示基于内容相似度生成的站点推荐。广告主也可以通过 slot/lease 市场单独购买可用的链接展示位置。

区块链负责注册、Verifier 质押、核验任务分配、结果、奖励与租赁托管；网页抓取、Embedding 和相似性搜索在链下运行。CONGRID 是原生代币，用于 Verifier 质押、协议奖励、交易手续费与支持该币种的市场支付。

本文描述当前仓库实现及其边界。默认参数是参考值，不代表已部署网络的实际配置。具备 IBC、治理或升级代码，并不意味着某次生产升级、交易所连接、流动性池或审计已经完成。

长期目标是建立规则可检查、减少对单一发现平台依赖的开放网络。当前核验确认的是首页与域名钱包的绑定关系，不认证文章质量、事实准确性、流量或搜索引擎排名价值。

## 2. 架构与参与者

### 2.1 链上协调

应用采用 Cosmos SDK v0.53.4 和 CometBFT v0.38.17。区块共识由 Cosmos 质押验证人集合和 CometBFT 承担。抓取与 Embedding 是应用服务，不是生成区块的工作量证明机制。

| 组件 | 当前职责 |
| --- | --- |
| `x/registry` | 发布者记录、drand 信标、核验任务、commit–reveal、相似性证据、奖励结算、slot 与 lease |
| `x/verifiers` | 普通账户的 Verifier 质押与模块托管 |
| `x/tokenomics` | 发行池注资、奖励转账与销毁 |
| Cosmos 模块 | 账户、余额、共识质押与罚没、分配、治理、手续费授权和软件升级 |
| IBC Core 与 ICS-20 | 跨链同质化代币转账 |
| `offchain/verifierd` | 首页检查、commit–reveal 提交与指定 drand 信标投递 |
| `offchain/indexerd` 与 Chroma 服务 | 首页索引、Embedding、语义查询和相似站点结果 |
| `cmd/congrid-site` | Web 接入、发布者面板、市场与下载 |

### 2.2 角色

- **Publisher（发布者）**注册网站并维护 Badge 和可选的推荐链接。注册无需发布者质押。
- **Verifier（核验者）**通过普通账户质押，检查分配的网站并提交观察结果；其质押独立于 Cosmos 共识质押。
- **Validator（共识验证人）**生产和最终确认区块，参与 Cosmos 质押、分配与罚没。运行 Verifier 不会自动成为共识验证人。
- **索引运营者**提供链下内容和相似性数据。单独运行索引目前没有独立的链上查询任务奖励。
- **消费者与广告主**使用内容发现或租用已上架位置。通用抓取赏金和按次付费搜索尚未形成完整协议流程。

Verifier 资格由 `x/verifiers` 的质押币种、最低质押、活跃状态以及 registry 暂停状态共同决定。当前最低质押默认值为 1 `ucongrid`；旧的 registry `verifier_bond` 参考字段不是实际抽样资格门槛。解绑通过 Verifier 模块返还托管资产，不经过 Cosmos staking 的解绑队列。

## 3. 发布者注册与归属

发布者提交规范化域名和 owner 地址。注册域名的首页必须包含指向 `https://congrid.net` 或 `https://www.congrid.net/` 的链接，不带 query 或 fragment，并包裹来自上述官方 HTTPS 域名的 Badge 图片。图片 URL 标识发布者域名和 owner 钱包。

CLI 注册辅助命令会在广播前检查网页。链上注册记录请求，后续 Verifier 观察决定是否通过。新记录初始为 `PENDING`。按自定义 ante 规则，恰好只含一条 `MsgRegisterPublisher` 的交易免手续费；其他交易遵循相应手续费策略。

注册表占用主域名键以阻止冲突的子域名注册。当前实现去掉端口后取主机名最后两段，并未使用公共后缀列表（Public Suffix List）；`co.uk` 等多段后缀仍存在归属范围限制。

`owner` 同时是控制地址和发布者奖励接收地址。重新注册写入 `pending_owner` 与 `pending_referrer`，不会立即覆盖原归属。核验任务绑定候选钱包；通过后更新归属及相关 slot 所有者；达到 quorum 但被拒绝时清除候选并保留原注册。未达到 quorum 不会接受候选。重新注册时省略 referrer，会在通过后清空原 referrer。

## 4. 核验轮次与 drand 任务分配

### 4.1 轮次调度

任务面向下一个轮次边界创建，注册不会立即触发同轮任务。参与调度的发布者包括冷却期外的 pending、verified 记录，以及存在重新注册候选的记录。

默认时间与抽样参数如下：

| 参数 | 默认值 |
| --- | ---: |
| 轮次间隔 | 3,600 秒 |
| 每个发布者请求的 Verifier 数量 | 3 |
| 从任务开始计算的 commit 窗口 | 300 秒 |
| 从任务开始计算的总提交窗口 | 600 秒 |
| drand 相对轮次开始的提前量 | 60 秒 |

对于一小时及以上轮次，每个域名在首小时内分配确定性的 0–59 分钟偏移；较短测试轮次使用有界秒级偏移。因此，较晚开始的任务可能在名义轮次结束后才最终确认。符合条件的 Verifier 少于请求数量时使用可用集合，并不强制至少三人才能形成 quorum。

### 4.2 外部随机数与种子

默认启用 drand，使用配置的 quicknet 元数据和 `bls-unchained-g1-rfc9380` BLS 方案。每个即将开始的 Content Grid 轮次只对应一个指定 drand round：

```text
latest_allowed_time = content_round_start - drand_round_offset_seconds
required_drand_round =
    floor((latest_allowed_time - drand_genesis_time_unix) / drand_period_seconds) + 1
```

默认 drand genesis 时间戳为 `1692803367`，周期为 3 秒。链检查指定轮次，使用配置的公钥校验 BLS 签名，并检查 `randomness = SHA256(signature)`。不符合当前需求的轮次和重复提交会被拒绝。

启用 drand 时，缺少指定信标将阻止任务创建，不会自动回退到区块哈希随机数；保留的 `drand_strict_mode` 字段不能放宽这一规则。只有显式关闭 drand，才会使用独立的仅区块锚点路径。

种子通过 SHA-256 混合 chain ID、Content Grid 轮次开始时间、创建任务时前一区块的高度与哈希、drand round 和随机数。精确编码见 [assignment_random.go](x/registry/typespb/assignment_random.go)。区块锚点仍是输入之一，但旧版的 `Hash(BlockHash + TaskID + Counter) % TotalMiners` 不再代表当前抽样算法。

链针对每个域名，从符合资格的 Verifier 集合中进行确定性的质押加权无放回抽样，并排序入选地址。轮次元数据保存种子、区块锚点、drand 信息和候选地址集合哈希；完整复算还需对应历史状态下的质押权重。

### 4.3 信标投递

`verifierd` 内置 drand 投递，查询 `DrandRequirement` 并获取指定轮次，不会持续提交 latest 信标。运营者通过确定性、质押加权的主投递者选择与错峰后备机制减少重复交易；链上验签和单次接受规则仍是最终约束。信标交易需要手续费，也可由 fee grant 赞助。

配置和投递细节见 [drand 文档](docs/drand-zh.md)。

### 4.4 Commit–reveal 与最终确认

只有被分配的 Verifier 可以提交。Commit 绑定域名、轮次、Verifier、任务 owner、pass/fail、证据哈希与 nonce。Reveal 必须在 commit 窗口结束后、任务截止前提交，并与 commit 匹配。

截止后，对于分配的 `N` 个 Verifier，当前规则为：

```text
quorum = ceil(N / 2)，其中 N > 0
has_quorum = 有效 reveal 数量 >= quorum
passed = has_quorum 且 pass 票数 > fail 票数
```

这是应用层 reveal quorum，不是 67% 阈值，也不是 CometBFT 的区块共识规则。投票按 Verifier 人数计算，不按质押加权。平票不通过。未达到 quorum 的任务仍会结束并标记为未通过，但不会视为获得 quorum 支持的发布者失败。

常规任务通过时，发布者进入或保持 `VERIFIED`，并清除失败计数。有 quorum 的失败可使已 verified 记录回到 `PENDING`；当前 `REVOKED` 转换只在记录仍为 `VERIFIED` 的分支内检查阈值。默认阈值虽为三，但不能描述为“任意连续失败三次一定自动撤销”：已经 pending 的记录后续失败不会执行该转换。

漏交 reveal 或与最终 pass/fail 结果相反的投票会增加 Verifier 处罚计数。默认累计三次触发暂停，暂停时长为三个轮次间隔；未受罚的参与会清除计数。当前路径实施任务资格暂停，不会自动没收 Verifier 质押。Cosmos 共识验证人的 slashing 属于另一套机制。

## 5. 内容索引与推荐

`indexerd` 从链上注册表和／或静态配置发现发布者，抓取首页、规范化内容，并生成 Embedding 和紧凑相似性签名（默认 128 位）。配置链状态过滤时，活跃发布者需为 verified 且不在冷却期；不活跃条目会被清理。

Chroma 服务提供向量存储和语义相似性查询，另有开发用内存余弦相似度检索。大向量与网页文本保留在链下。当前系统不要求每个共识验证人维护全网完整向量索引，也不会为每次搜索创建一个付费矿工委员会。DHT 和 executor 代码的存在不代表该端到端流程已经接通。

相似站点接口默认最多返回 15 个域名，已不是旧版的十个 URL。发布者可以展示这些基于内容的推荐；付费 slot 是独立的市场行为，购买位置不会提高相似性分数。

Verifier 提交预期集合哈希、观察集合哈希和匹配域名数量。在通过的 reveal 中，某个预期集合哈希必须单独达到任务 quorum；随后从同意该集合的观察值中取排序后中间项（偶数个值时取较大中间项），匹配数量上限为 15。未形成该共识时，用于奖励的匹配数为零。

相似链接只影响发布者奖励比例，不改变 Badge 的 pass/fail。不同索引可能因抓取时间、可用网站和模型配置而产生差异。链聚合提交的证据，不会重新执行 Embedding，也不证明推荐内容的编辑质量。

## 6. 链接位置与租赁

已核验的发布者可以创建 slot，设置计价币种、金额、时间单位与租期边界。Slot 可以上架、暂停或下架。买家指定目标 URL 和有效租期；同一 slot 上时间重叠的 active lease 会被拒绝。

```text
托管金额 = 单位价格 ×（租赁秒数 / 计价单位秒数）
```

买家款项进入 registry 模块托管。支付币种由 slot 决定；CONGRID 使用 `ucongrid`，但底层模型提供币种字段，并非强制所有 lease 只能使用 CONGRID。到期时，除满足已实现的冷却／退款条件外，剩余款项支付给 lease 记录中的发布者。有 active lease 的发布者发生符合条件的核验失败时，可触发冷却并退回剩余托管款。

仓库包含按 slot/lease 属性与目标 URL 检查租赁链接的辅助函数。但是当前 `verifierd` 任务循环检查的是首页 Badge 与相似性证据，并未逐条检查 lease 链接。因此，仅缺失一条付费链接目前不会独立触发自动违约判定。托管与发布者核验失败退款已经实现，完整的逐位置履约核验仍需后续接入。

## 7. 代币经济

### 7.1 参考分配与发行池注资

`1 CONGRID = 1,000,000 ucongrid`。当前 registry 默认以 10 亿 CONGRID 为参考供应量：

| 用途 | 占参考供应量比例 |
| --- | ---: |
| 运营储备 | 40% |
| 发布者发行池 | 10% |
| Verifier 发行池 | 50% |

排放期限参数为 876,000 小时（按每年 365 天计的 100 年）。运营储备不会由 registry 结算自动分配，需要显式配置分配。

`EnsureEmissionPool` 根据配置的发行池目标与已记录累计注资量的差额，向 tokenomics 账户铸币。之后奖励从池中转出，未领取份额从池中销毁。这并非每小时为每位接收者分别铸币，也不会在每次支出后自动补回池余额。

参考供应量与期限是奖励公式的输入，不是全链强制总量上限或按日历执行的终止日期。应用还接入了 Cosmos mint 模块，生产网络总发行量需要结合实际 genesis、mint 参数和升级配置判断。无任务轮次不结算发行池，池耗尽后也没有专门的排放结束处理机制。

### 7.2 每轮额度

设轮次长度为 `T` 秒，参考供应量为 `S`（单位 `ucongrid`），分配基点为 `bps`，期限为 `H` 小时：

```text
round_pool = floor(S × bps × T / (10000 × H × 3600))
```

默认小时轮次对应：

- 发布者池：114,155,251 ucongrid，即 114.155251 CONGRID。
- Verifier 池：570,776,255 ucongrid，即 570.776255 CONGRID。

这些是全网当轮额度，不是每个网站的固定奖励。必须等同一轮所有任务最终确认后，才统一结算。

### 7.3 发布者奖励

发布者池在当轮核验通过的任务之间均分。每位发布者按以下公式领取其基础份额：

```text
claim_bps = max(publisher_min_reward_bps,
                min(10000, floor(matched_links × 10000 / required_links)))
payout = floor(base_share × claim_bps / 10000)
```

默认 `publisher_min_reward_bps = 1000`，`required_links = 15`。Badge 有效但没有匹配链接时领取基础份额的 10%；匹配 15 条时领取 100%。相似性证据不足不会取消这个最低比例。未领取部分与整数余数被销毁。

### 7.4 Verifier 奖励

Verifier 池先按当轮全部任务均分，包含失败任务。只有通过核验的任务会分配对应额度，且只有其中提交 pass 的 Verifier 获奖。正确的 fail 投票可以避免处罚，但目前不领取这项奖励。

单个通过任务内：

- 40% 在成功 Verifier 之间均分。
- 60% 按 `质押量 × max(1, 活跃推荐发布者数量)` 加权分配。

质押和活跃推荐数在结算时读取；推荐因子不参与任务抽样。无法支付的份额及当轮剩余额度销毁。旧的评分权重与手续费／罚没路由参数结构，不应被解读为已经全面接通的奖励或补偿路径。

## 8. 治理、互操作与实现边界

应用包含 Cosmos 治理和软件升级能力。自定义模块有参数，但没有覆盖全部自定义模块的通用 `MsgUpdateParams` 治理接口；参数修改需使用实际支持的 genesis 或升级路径。

通过 ibc-go v10.7.0，已接入 IBC Core、Tendermint 轻客户端与 ICS-20 转账，并提供存量链 `ibc-transfer-v1` 迁移。它提供跨链转账基础设施；外部通道、Relayer、资产注册与 DEX 流动性属于独立部署步骤。仓库测试和演练不能证明 Osmosis 主网市场已开通。详见 [IBC 文档](docs/ibc-zh.md)。

当前后续工作包括：

- 接通每个付费位置的独立履约核验与结算。
- 在 Badge 归属和相似性证据之外建立内容质量与反滥用机制。
- 改进索引一致性与更广泛的分布式索引运行。
- 通用消费者赏金与付费 API 结算。
- 自定义参数治理、Verifier 质押罚没与补偿、运营储备自动分配。
- 覆盖所有铸币路径的供应量核算与排放结束处理。
- 支持公共后缀的域名归属键，以及一致的失败到撤销状态转换。

本节替代旧阶段清单：旧文把已存在的索引与奖励列为未来工作，却把已移除的矿工任务描述为完成。网络上线与审计状态应由独立部署证据确认。

## 9. 实现依据

- [注册与核验交易](x/registry/msg_server.go)
- [轮次分配、处罚与结算](x/registry/verification_rounds.go)
- [drand 调度](x/registry/drand_schedule.go)与[签名验证](x/registry/drand_verify.go)
- [默认参数与排放公式](x/registry/types.go)
- [相似站点证据聚合](x/registry/similar_settlement.go)
- [发行池注资](x/tokenomics/keeper.go)
- [租赁结算](x/registry/lease_settlement.go)与[核验代理](offchain/verifierd/agent.go)
- [IBC 装配](app/ibc.go)与[升级处理](app/upgrades.go)

参与和运维入口见 [README](README-zh.md)，构建、测试与贡献流程见[贡献者指南](CONTRIBUTING-zh.md)。
