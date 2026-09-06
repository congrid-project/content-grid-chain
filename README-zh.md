# 内容网格链（Content Grid Chain）

Content Grid Chain 是去中心化内容发现网络的链上协调与结算层。它记录 Publisher 的域名归属，向独立 Verifier 分配任务，就网站证明达成共识，并结算 Publisher/Verifier 奖励与链接市场款项。

[English](README.md) · [协议白皮书](whitepaper.md) · [贡献者指南](CONTRIBUTING-zh.md)

> 项目状态：链运行时、Publisher 注册表、Verifier 核验流程、drand 随机数、奖励与 slot/lease 市场均已实现。本仓库尚未发布正式主网所需的完整发布包（二进制、genesis 和 peer 列表）。如果白皮书与代码存在差异，以当前代码和协议文档为准。

## 协议概览

Content Grid 连接四类参与者：

- **Publisher（发布者）**注册域名，并在首页放置绑定钱包的 Congrid Badge，证明对网站的控制权。
- **Verifier（核验者）**绑定 `CONGRID`，独立检查分配到的 Publisher，并通过 commit–reveal 提交结果。
- **Consensus Validator（共识验证人）**运行 Cosmos 链，对交易排序并最终确认协议状态。
- **消费者与广告主**发现 Publisher，或租用已经核验的链接 slot。

一次常规的 Publisher 核验流程如下：

1. Publisher 注册域名，记录初始状态为 `PENDING`。
2. 到达核验轮次边界后，链获取指定的 drand beacon，并生成可审计的轮次种子。
3. 链从符合条件且未被暂停的 Verifier 中，按质押权重进行确定性无放回抽样。
4. 入选 Verifier 检查网站首页，先提交 commit，再 reveal 核验结果与证据。
5. 达到 quorum 后，链最终确认多数结果，更新 Publisher 状态，记录必要的 Verifier 处罚并结算奖励。
6. 已核验的 Publisher 可以发布链接 slot；lease 款项在租约完成前由协议托管，核验失败时可触发退款。

## 共识模型

Content Grid 包含两个相互关联但职责不同的共识层。

### 链共识

区块链基于 Cosmos SDK v0.53 与 CometBFT v0.38。Cosmos 共识验证人提供拜占庭容错的区块排序和最终性；应用接入了标准的 staking、slashing、governance、bank 与 distribution 模块。

### Publisher 核验共识

网站核验是记录在链上的应用层共识：

- 默认每小时一个轮次；时间窗口和阈值均为链上参数。
- 默认启用严格 drand 模式。链只接受下一核验轮次指定的 beacon，在链上校验其 BLS 签名，不回退到仅依赖区块哈希的随机数。
- Verifier 选择采用确定性的、按质押加权的无放回抽样，任何观察者都可以根据轮次种子和链上 Verifier 集合复算结果。
- Commit–reveal 在 commit 窗口内隐藏投票内容。最终结果必须达到 quorum；只有 pass 票数多于 fail 票数时才通过。
- 漏交结果或投票偏离最终结果会累积处罚；多次处罚可导致 Verifier 暂时无法获得任务。
- Publisher 状态包括 `PENDING`、`VERIFIED` 和 `REVOKED`。已核验 Publisher 修改 owner 或 referrer 时，原记录继续生效，直到新一轮核验接受候选变更。

本文中的 `validator` 始终指 Cosmos 共识验证人；`verifier` 指独立的 Content Grid 网站核验角色。两种角色不要求由同一账户承担。

精确规则请参阅 [Verifier 规则](docs/verifiers-zh.md)和 [drand 规则](docs/drand-zh.md)。

## 协议组成

| 组件 | 职责 |
| --- | --- |
| `x/registry` | Publisher 记录、核验轮次、commit–reveal、相似站点证据、slot、lease 与结算 |
| `x/verifiers` | Verifier 质押托管与资格管理 |
| `x/tokenomics` | 发行池注资、转账与销毁 |
| `offchain/verifierd` | 获取任务、检查 Publisher 页面、提交 commit/reveal，并投递指定的 drand beacon |
| `offchain/indexerd` | 索引 Publisher 首页，生成相似性结果与紧凑签名 |
| `cmd/congrid-site` | Publisher 引导、Dashboard、下载与链接市场的 Web 入口 |

默认经济参数以 10 亿枚 `CONGRID` 为参考总量（`1 CONGRID = 1,000,000 ucongrid`）：40% 为运营储备，10% 用于 Publisher 排放，50% 用于 Verifier 排放，周期为 100 年。Publisher 奖励取决于 Badge 是否有效以及相似站点链接的匹配情况；Verifier 奖励由平均分配的基础部分和按质押、推荐关系加权的部分组成；无人领取的当轮排放会被销毁。以上为默认 genesis 参数，实际网络可能不同。运营储备自动分配、完整消费者支付链路和完整罚没补偿链路目前尚未端到端接通。

完整规则和当前实现范围请参阅 [Tokenomics](docs/tokenomics-zh.md)、[链接市场](docs/marketplace-zh.md)与[治理手册](docs/governance-zh.md)。

IBC Core 与 ICS-20 转账已接入，并提供双链应用测试和 Hermes 本地联调脚本。
已运行网络需要通过 `ibc-transfer-v1` 升级启用；Osmosis 正式通道与流动性池需另外建立。
参阅 [IBC 测试与上线手册](docs/ibc-zh.md)。

## 使用协议

Chain ID、RPC 地址、genesis 和 seed peer 等网络专属信息，应以网络运营方或官方发布包提供的值为准。

### 注册 Publisher

使用浏览器钱包时，可以通过 [congrid.net/publishers](https://congrid.net/publishers) 引导页生成 Badge 代码和注册命令。

在待注册域名的首页加入 Congrid 链接。链接必须包裹一张图片，图片 URL 中包含域名和签名钱包：

```html
<div id="congrid-similar">
  <a href="https://congrid.net">
    <img
      src="https://congrid.net/badge.svg?publisher=example.com&wallet=<congrid-address>"
      alt="Verified by Congrid"
      width="32"
      height="32"
    />
    <span>Congrid — Content Grid Protocol</span>
  </a>
  <!-- 在这里加入网络返回的相似 Publisher 链接。 -->
</div>
```

`a` 必须指向不带 query 或 fragment 的 `https://congrid.net` 或 `https://www.congrid.net/`。图片必须位于上述任一域名下，并在 path 或 query 中编码 `publisher=<domain>` 和 `wallet=<owner>`；其中 wallet 必须与交易签名者一致。

注册域名：

```bash
./content-grid-d publisher register example.com \
  --from <publisher-key> \
  --chain-id <chain-id> \
  --node <rpc-url> \
  --fees 0ucongrid
```

可选参数包括 `--metadata-uri` 和 `--referrer`。注册本身不要求质押。只包含 `MsgRegisterPublisher` 的交易可以使用零手续费；混合交易仍需遵循网络的 minimum gas price 策略。

查询记录：

```bash
./content-grid-d query registry publisher \
  --domain example.com \
  --node <rpc-url>
```

`owner` 同时是控制钱包和奖励接收钱包。修改 owner 或 referrer 时，先更新首页 Badge，再用新签名者重复提交注册命令。`pending_owner` 与 `pending_referrer` 会保持候选状态，直到 Verifier 共识接受；候选核验失败不会影响当前注册。

注册表会占用一个主域名键，防止相互冲突的子域名抢注。当前实现会去掉端口并取主机名最后两段作为该键，因此使用多段公共后缀的站点应在注册前确认实际占用范围。

### 运行 Verifier

使用普通 `congrid1...` 账户绑定代币：

```bash
./content-grid-d verifier bond 1000000 --denom ucongrid --from <verifier-key>
./content-grid-d verifier assignments --from <verifier-key>
```

随后运行 `verifierd` 处理任务并参与 drand 投递。解除绑定：

```bash
./content-grid-d verifier unbond 1000000 --denom ucongrid --from <verifier-key>
```

配置、密钥、手续费和健康检查见 [verifierd 指南](docs/verifierd-zh.md)。容器化的节点与 Verifier 组合部署见 [Docker Operator 指南](docs/docker-operator-zh.md)。

### 运行全节点或共识验证人

接入公开网络时，应使用官方二进制、genesis 和 peer 列表。[生产运行手册](docs/runbook-zh.md)包含节点健康检查和故障处理，[启动清单](docs/launch-checklist-zh.md)说明发布前的验收要求。共识验证人在创世期通过标准 Cosmos `gentx` 流程加入，网络启动后则使用 `tx staking create-validator`。

本地开发网络、构建、测试与 Protobuf 生成统一放在[贡献者指南](CONTRIBUTING-zh.md)中。

### 使用链接市场

已核验的 Publisher 可以发布 slot，广告主可以租用 slot；协议会在 lease 生效期间托管款项。Publisher 必须提供规定的 `data-congrid-slot-id` 和 `data-congrid-lease` 标记，以供 Verifier 检查履约情况。生命周期与 CLI 示例见[链接市场指南](docs/marketplace-zh.md)。

### 查询网络

Daemon 提供 Cosmos RPC/gRPC 服务与 registry REST 路由。常用 CLI 查询包括：

```bash
./content-grid-d query registry publisher --domain example.com --node <rpc-url>
./content-grid-d query registry drand-requirement --node <rpc-url>
./content-grid-d query registry slots --publisher <congrid-address> --node <rpc-url>
./content-grid-d query registry leases --slot-id <slot-id> --node <rpc-url>
```

API 定义位于 [`proto/contentgrid`](proto/contentgrid)。

## 文档导航

- 协议设计：[白皮书](whitepaper.md)（部分历史章节仍描述规划中或已移除的范围）
- 核验与索引：[Verifier](docs/verifiers-zh.md)、[verifierd](docs/verifierd-zh.md)、[drand](docs/drand-zh.md)、[indexerd](docs/indexerd-zh.md)
- 经济与市场：[Tokenomics](docs/tokenomics-zh.md)、[链接市场](docs/marketplace-zh.md)、[治理手册](docs/governance-zh.md)
- 运维：[Docker Operator](docs/docker-operator-zh.md)、[生产运行手册](docs/runbook-zh.md)、[启动清单](docs/launch-checklist-zh.md)
- 开发：[贡献者指南](CONTRIBUTING-zh.md)

## 许可

源代码和文档采用 [MIT License](LICENSE)。

独立的非代码生态知识产权采用 [Chain Inspiring License v1.0](CHAIN-INSPIRING-LICENSE.md)。仅使用 MIT 许可的代码或文档不会触发 CIL-1.0。CIL-1.0 是自定义生态知识产权许可，并非 OSI 批准的开源软件许可证；采用受保护的生态材料前请阅读完整许可文本。
