# `drand-strict-v2` 主网升级手册

本升级用于已经运行的 ConGrid 链。升级 handler 的固定名称是
`drand-strict-v2`；治理提案必须使用完全相同的名称。

升级会：

- 运行全部模块 migration，并把 registry consensus version 从 1 升到 2；
- 保留已有 publisher、verifier、余额、质押和租约状态；
- 补齐 quicknet chain hash、公钥、genesis time、period 和 round offset；
- 强制设置 `drand_enabled=true`、`drand_strict_mode=true`；
- 从升级后的下一次 verification round 开始执行“指定 round、严格验签、单次接收”。

## 1. 选择升级高度

升级高度必须晚于提案的 deposit/voting 周期，并为所有验证人预留下载、验签、
备份和演练时间。先记录当前高度和治理参数：

```bash
content-grid-d status
content-grid-d query gov params voting
content-grid-d query gov params deposit
```

确定一个尚未产生的高度：

```bash
export UPGRADE_NAME=drand-strict-v2
export UPGRADE_HEIGHT=<future-height>
```

不要把升级高度设置在当前或即将到达的高度。

## 2. 构建、测试并发布相同产物

从审核通过的同一个 Git commit 构建：

```bash
go test ./...
mkdir -p build
go build -trimpath -o build/content-grid-d ./cmd/content-grid-d
go build -trimpath -o build/verifierd ./offchain/verifierd
sha256sum build/content-grid-d build/verifierd
```

为每个验证人平台发布归档文件。节点归档中必须包含名为
`content-grid-d` 的可执行文件。所有运营方应独立核对 Git commit 和 SHA-256。

官网可以直接托管 Linux amd64 归档：

```bash
cp content-grid-d-linux-amd64.tar.gz cmd/congrid-site/downloads/
curl -fI https://congrid.net/downloads/content-grid-d-linux-amd64.tar.gz
```

下载目录和生产持久化配置见 [`../cmd/congrid-site/README-zh.md`](../cmd/congrid-site/README-zh.md)。

如果使用 Cosmovisor，把节点二进制预置到：

```text
$DAEMON_HOME/cosmovisor/upgrades/drand-strict-v2/bin/content-grid-d
```

如果手动升级，可以提前下载并校验，但不要在升级高度前用新二进制启动共识节点。

## 3. 准备 verifierd

升级前准备新版 `verifierd` 配置：

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

确认 verifier signer 有足够 `ucongrid` 支付交易费，或已经获得只允许
`/contentgrid.registry.v1.MsgSubmitDrandBeacon` 的 fee grant。新版 verifierd
可以在链完成升级后启动；不必继续运行旧 `drand-relayer`。

## 4. 提交软件升级提案

`--upgrade-info` 应提供可下载归档及 SHA-256。URL 的 checksum 格式为
`?checksum=sha256:<hex>`：

```bash
export NODE_ARCHIVE_URL='https://congrid.net/downloads/content-grid-d-linux-amd64.tar.gz?checksum=sha256:<sha256>'
export UPGRADE_INFO="$(jq -nc --arg url "$NODE_ARCHIVE_URL" '{binaries:{"linux/amd64":$url}}')"

content-grid-d tx upgrade software-upgrade "$UPGRADE_NAME" \
  --title "Enable strict drand delivery" \
  --summary "Run registry v1-to-v2 migration and enable exact-round strict drand" \
  --upgrade-height "$UPGRADE_HEIGHT" \
  --upgrade-info "$UPGRADE_INFO" \
  --deposit <governance-deposit> \
  --from <proposer-key> \
  --chain-id <chain-id> \
  --node <rpc-url> \
  --gas auto \
  --gas-adjustment 1.4 \
  --fees <fee> \
  -y
```

不要在生产提案中使用 `--no-validate` 或 `--no-checksum-required`。

从交易响应取得 proposal ID，完成投票：

```bash
content-grid-d query gov proposal <proposal-id>
content-grid-d tx gov vote <proposal-id> yes \
  --from <voter-key> \
  --chain-id <chain-id> \
  --node <rpc-url> \
  --fees <fee> \
  -y
```

提案通过后确认链上计划：

```bash
content-grid-d query upgrade plan
```

输出中的 name 和 height 必须分别等于 `drand-strict-v2` 和预定高度。

## 5. 升级高度前的验证人检查

- 对应用数据库做可恢复的快照，并记录快照高度；不要修改 `priv_validator_state.json`。
- 确认至少 2/3 投票权的验证人已准备相同 checksum 的二进制。
- 确认监控能识别升级高度的预期停机。
- 确认新版 verifierd、keyring、手续费/feegrant 和 drand HTTP 出口已经准备好。
- 不要配置 `--unsafe-skip-upgrades`；跳过 handler 会导致状态版本不一致或共识分叉。

## 6. 在升级高度切换二进制

旧二进制运行到升级高度并按计划停止。之后每个验证人：

1. 停止节点服务。
2. 再次核对下载文件的 SHA-256。
3. 原子替换 `content-grid-d`，或让 Cosmovisor 切换到升级目录。
4. 使用原来的 home、数据库、共识密钥和启动参数启动节点。
5. 观察节点追上并开始产生升级高度之后的区块。

不要执行 `unsafe-reset-all`，不要重新 `init`，也不要用新的 genesis 覆盖现有链状态。

### Docker Compose 运营节点

可以在升级高度前构建新镜像；只要不重建正在运行的 node 容器，旧进程仍使用旧镜像：

```bash
docker compose --env-file .env.operator -f docker-compose.operator.yml build node verifierd
```

旧 node 在升级高度停止后再切换容器，并保留原有 `congrid-home` volume：

```bash
docker compose --env-file .env.operator -f docker-compose.operator.yml stop node verifierd
docker compose --env-file .env.operator -f docker-compose.operator.yml up -d --no-deps node
docker compose --env-file .env.operator -f docker-compose.operator.yml up -d verifierd
```

禁止删除 volume，也不要使用新的 genesis 重建该 volume。

## 7. 升级后验收

```bash
content-grid-d status
content-grid-d query upgrade applied drand-strict-v2
content-grid-d query upgrade module-versions registry
content-grid-d query registry drand-requirement
```

预期结果：

- registry module version 为 `2`；
- `drand-requirement.enabled` 为 `true`；
- 返回唯一的 `required_drand_round` 和 quicknet chain hash；
- 新版 verifierd 启动后，`submitted` 变为 `true`，或该 Content Grid round 已创建；
- `/readyz` 没有持续的 `last_drand_error`；
- 后续 round meta 包含非零 `drand_round` 和 `drand_randomness_hex`。

检查日志：

```bash
curl -fsS http://127.0.0.1:9200/readyz | jq
journalctl -u <verifierd-service> -f
```

## 8. 取消和故障处理

升级执行前可以通过治理取消：

```bash
content-grid-d tx upgrade cancel-software-upgrade \
  --title "Cancel drand-strict-v2" \
  --summary "Explain the reason for cancellation" \
  --deposit <governance-deposit> \
  --from <proposer-key> \
  --chain-id <chain-id> \
  --node <rpc-url> \
  --fees <fee> \
  -y
```

升级 handler 已成功执行后，不要单个节点回退到旧二进制。严重故障只能由验证人
共同决定修复版本，或从同一升级前快照在一致高度协调恢复。

drand 暂时不可用不会停止出块；链会暂停创建新的 assignment，直到指定信标通过
严格验签并被单次接收。
