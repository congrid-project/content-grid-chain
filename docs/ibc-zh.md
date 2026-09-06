# IBC 接入、跨链测试与 Osmosis 上线

本版本接入 `ibc-go v10.7.0`，保持 Cosmos SDK `v0.53.4` 和 CometBFT
`v0.38.17`。支持 Tendermint 轻客户端、IBC Core 和 ICS-20 Transfer。
面向 Osmosis 的测试路径为 `transfer` 端口、`UNORDERED` 通道、`ics20-1`。
链原生单位是 `ucongrid`，`1 CONGRID = 1,000,000 ucongrid`。

## 可重复的测试

### 应用级双链测试

```bash
go test ./app -run TestIBC -count=1 -v
```

测试在两个真实 Congrid 应用实例上执行签名交易、提交区块并验证
Tendermint 轻客户端和 Merkle 证明，覆盖：

- 客户端、连接和 ICS-20 通道握手。
- CONGRID 转出锁定、对端凭证铸造、原路返回销毁凭证并解锁。
- 1,000 枚 `uusdc-test` 测试资产的双向转账，验证金额和 denom trace。
- 重复接收同一数据包不会重复铸币，篡改数据包无法通过证明验证。
- 无效收款地址的错误回执退款。
- 未接收数据包超时退款；已销毁的返程凭证超时后重新铸回。
- 余额、托管金额、凭证供应量和数据包承诺清理。
- 拒绝重复执行 IBC 初始化，以及缺少或过旧的历史模块版本记录。

测试固定使用晚于 drand 启动日期的时间；仅在测试 genesis 中关闭 SDK
区块通胀，以单独核对 IBC 供应量守恒。生产经济参数不因此改变。

### 两个节点进程 + Hermes

需要 Go、Python 3.8+ 和从 [Hermes 官方发行页](https://github.com/informalsystems/hermes/releases/tag/v1.13.3)
取得的 Hermes。`v1.13.3` 标签的二进制可能报告 `1.13.2+bab3b80`。

```bash
go build -o content-grid-d ./cmd/content-grid-d
python3 scripts/ibc/localnet.py \
  --binary ./content-grid-d \
  --hermes /path/to/hermes
```

脚本创建新的私有临时目录、测试密钥和两个独立的单验证人网络。
所有 RPC、REST、gRPC 和 P2P 地址都绑定到 `127.0.0.1`。
使用 Hermes 完成握手及实际中继，核对 CONGRID 和 1,000 枚测试 USDC 的
往返余额，再停止中继让数据包超时，重启中继验证退款。
脚本退出时停止它启动的进程，并保留日志和 `report.json`。
手续费由测试账户支付；`uusdc-test` 是本地测试币，不是真实 USDC。
可通过 `--work-parent ./tmp` 指定临时目录的父目录。

要同时验证旧数据库升级，先保留当前生产基线版本的二进制，再运行：

```bash
python3 scripts/ibc/localnet.py \
  --binary ./content-grid-d \
  --hermes /path/to/hermes \
  --upgrade-from /path/to/pre-ibc/content-grid-d
```

这会让 A 链先使用旧二进制，经本地治理通过 `ibc-transfer-v1` 计划，
在升级高度停止出块，然后使用新版打开同一数据库。脚本检查升级已执行、
交易账户余额与 genesis 文件保持一致，再连接 B 链做跨链测试。

**测试范围：本地 Congrid ↔ Congrid。它不代表已经接入 Osmosis 主网，
也不验证 Osmosis 前端收录、实际 USDC 路由、建池或行情价格。**

已完成一次[生产状态副本演练](ibc-production-rehearsal-20260905.md)：真实高度 70445 的状态经 IBC 迁移、模拟区块处理和数据库重开校验通过。详细范围及证据见记录。

## 已运行网络的升级

升级计划名称必须是 `ibc-transfer-v1`。

1. 核对线上正在运行的版本及 `query upgrade module-versions`。
   升级处理器要求所有已有模块与当前 IBC 之前的基线版本一致，包括
   registry consensus version 3。需要先完成已有的历史升级。
   缺失的版本记录不能靠删除状态或重新初始化 genesis 修复。
2. 用生产状态副本演练升级并核对余额、验证人集合和业务状态。
   与验证人协调统一的升级高度，保存数据库、配置和二进制备份。
3. 按现有治理流程提交 `MsgSoftwareUpgrade`，计划名称如上，高度使用
   实际协商结果。无需更换 chain ID 或 genesis。
4. 旧节点到高度时写入 `data/upgrade-info.json` 并停止共识。CometBFT
   的进程和 RPC 可能仍在运行；应正常停止节点后切换到新二进制。
5. 新版在加载数据库前根据该计划新增 `ibc`、`transfer` 两个存储。
   升级处理器初始化客户端/连接参数和 transfer 端口。transfer 模块账户由 SDK 在首次需要时创建。
   不要在升级高度之前直接用新二进制打开旧数据库，不要跳过该升级。
6. 确认节点继续出块，以下查询可用，且余额和业务状态符合升级前记录：

```bash
content-grid-d query ibc client params --node "$CONGRID_RPC"
content-grid-d query ibc-transfer params --node "$CONGRID_RPC"
content-grid-d query ibc channel channels --node "$CONGRID_RPC"
```

本版本不为已部署网站提前宣称 IBC 已启用。应在链升级和实际通道验证完成后，
再更新钱包及公开链注册信息。

## 建立到 Osmosis 的正式通道

依据 [Osmosis 接入指南](https://docs.osmosis.zone/integrate/list-asset/transfer/)：

- 先验证正式 Congrid 网络的 chain ID、genesis、稳定出块及公网 RPC/REST/gRPC。
- 选择受支持的 Osmosis 端点，配置 Hermes 两端的 chain ID、地址前缀、
  节点地址、手续费及资金充足的专用中继账户。Osmosis 侧还需要 OSMO 手续费预算。
- 依据两端实际 unbonding period 设置 trusting period，保持客户端及时更新。
  本地脚本的 `trusted_node=true` 和通配 channel 过滤器只用于隔离测试，
  不应原样作为公网生产中继配置。
- 核对是否已有正式通道；优先复用已确认的 canonical 通道，避免产生不同路径的资产凭证。
- 新建时使用 `transfer` / `transfer`、`UNORDERED`、`ics20-1`，记录两端的
  client ID、connection ID、channel ID。channel ID 不一定相同。
- 使用少量真实 CONGRID 做转出和原路转回，核对 Osmosis 上的 `ibc/<hash>`
  与实际 denom trace，再扩大额度。
- 注册 Cosmos Chain Registry 的链、资产和 `_IBC` 通道资料；按
  [Osmosis 资产注册流程](https://docs.osmosis.zone/integrate/list-asset/registration/)
  完成前端识别及后续 verified 申请。

查询实际通道后，转账命令形式如下。`CONGRID_CHANNEL` 必须是 Congrid 侧通道，
`OSMOSIS_RECEIVER` 必须是对端收款地址。此示例转出 1 CONGRID：

```bash
content-grid-d tx ibc-transfer transfer transfer "$CONGRID_CHANNEL" \
  "$OSMOSIS_RECEIVER" 1000000ucongrid \
  --chain-id "$CONGRID_CHAIN_ID" --node "$CONGRID_RPC" \
  --home "$CONGRID_NODE_HOME" --from "$CONGRID_SENDER_KEY" \
  --gas auto --gas-adjustment 1.3 --gas-prices 0.001ucongrid \
  --packet-timeout-timestamp 600000000000
```

超时参数单位是纳秒，默认是相对本机时间的 10 分钟。
超时不会自动在本链退款：仍需要中继提交对端的未接收证明。

## 初始流动性预算

项目方给出的初始配对资产预算是 **1,000 USDC**，目前仅作为计划记录。
本次开发和测试不会转移这些资金。

建池前还需要决定投入多少 CONGRID、池型、初始价格/价格区间、手续费，
并核对 Osmosis 上采用的正式 USDC denom。建池费、中继 OSMO 手续费和
持续运营成本应另外核算，链上费用以执行时参数为准。

DEX 上可交易后，Keplr 的估值仍需要实际行情收录及正确价格映射。
CoinGecko API 确实返回 CONGRID 价格后，才填写有效的 `coinGeckoId`。
