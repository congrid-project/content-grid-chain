# 生产运行手册（Runbook）

> 适用于：`content-grid-d`、`verifierd`、`drand-relayer`、`congrid-site`。
> 目标：出现问题时，值班学生5分钟内定位方向，15分钟内实施缓解措施。

> 术语说明：`validator` 指 Cosmos 共识验证人；`verifier` 指 ConGrid 的发布者核验角色及其 `verifierd` 代理。

---

## 0.环境信息（当前推荐基线）

- 代码目录：`/home/eking/workspace/congrid.net`
- 链节点HOME：`/home/eking/.content-grid`（当前默认基线）
- verifierd 配置：`/home/eking/workspace/congrid.net/offchain/verifierd/config.json`
- drand 中继器配置：`/home/eking/workspace/congrid.net/offchain/drandrelayer/config.json`
- 站点配置（推荐 env 文件）：`/home/eking/workspace/congrid.net/.env.site`
- 日志目录（推荐）：`/home/eking/workspace/congrid.net/logs/`
- 时区：`America/Toronto`

负责人（现任）：
- 链：`eking`
- verifier 负责人：`eking`
- 站点：`eking`
- 值班通知：`Telegram @eking (id:6148992071)`

---

## 1. 服务清单及职责

- 链节点：`content-grid-d`
- 职责：共识、状态执行、gRPC/RPC 提供
- verifier 代理：`verifierd`
- 职责：拉动分配、提交/揭示、站点验证
- 随机信标中继：`drand-relayer`
- 职责：拉取最新的drand信标并上传到链上提交
- 官方网站/市场：`congrid-site`
- 职责：链上展示和用户入口、槽位/租约提交

---

## 2.启动、停止和健康检查

## 2.1 内容网格-d

### 启动（手动模式）

启动前检查（必做）：`--home` 目录下必须已有 `config/genesis.json`。

```bash
ls -l /home/eking/.content-grid/config/genesis.json
```

如果文件不存在，先初始化（单节点本地基线）：

```bash
cd /home/eking/workspace/congrid.net
./content-grid-d devnet --home /home/eking/.content-grid --chain-id content-grid-dev-1
```

再启动：

```bash
cd /home/eking/workspace/congrid.net
./content-grid-d start --home /home/eking/.content-grid
```

### 健康检查
```bash
# RPC
echo >/dev/tcp/127.0.0.1/26657

# gRPC
echo >/dev/tcp/127.0.0.1/9090

# 最新区块（应持续增长）
./content-grid-d query block --type height --home /home/eking/.content-grid --node tcp://127.0.0.1:26657 -o json
```

关键检查：
- 最新区块持续增长
- 没有连续的恐慌/共识失败

### Docker 节点上的共识验证人运维

如果节点跑在仓库自带的 Docker/Podman `node` 容器里，建议把下面这些值固定到 env 文件：

- `CONGRID_VALIDATOR_KEY_NAME`
- `CONGRID_VALIDATOR_KEYRING_DIR`（留空表示沿用默认 keyring 路径）
- `CONGRID_VALIDATOR_JSON_PATH`

容器内置了 `congrid-validator-cli`，会自动复用这些 env：

```bash
podman exec congridnet_node_1 congrid-validator-cli show-config

read -rsp 'Keyring passphrase: ' KEYRING_PASS
echo

ACC=$(
  printf '%s\n' "$KEYRING_PASS" |
  podman exec -i congridnet_node_1 \
    congrid-validator-cli show-account-address 2>/dev/null
)

VALOPER=$(
  printf '%s\n' "$KEYRING_PASS" |
  podman exec -i congridnet_node_1 \
    congrid-validator-cli show-valoper-address 2>/dev/null
)

./content-grid-d query staking validator "$VALOPER" --node tcp://127.0.0.1:26657

podman exec -it congridnet_node_1 \
  congrid-validator-cli create-validator \
  --gas auto \
  --gas-adjustment 1.5 \
  --gas-prices 0.001ucongrid \
  -y
```

## 2.2 verifierd / verifier 代理

### 启动（手动模式）
```bash
cd /home/eking/workspace/congrid.net
./verifierd --config /home/eking/workspace/congrid.net/offchain/verifierd/config.json
```

### 单轮探索
```bash
./verifierd --config /home/eking/workspace/congrid.net/offchain/verifierd/config.json --once
```

关键日志关键字：
- `submitted commit`
- `revealed result (passed=true|false)`
- `commit failed` / `reveal failed`

## 2.3 drand-relayer

### 启动（手动模式）
```bash
cd /home/eking/workspace/congrid.net
./drand-relayer --config /home/eking/workspace/congrid.net/offchain/drandrelayer/config.json
```

### 单轮探索
```bash
./drand-relayer --config /home/eking/workspace/congrid.net/offchain/drandrelayer/config.json --once
```

关键日志关键字：
- `submitted beacon round=`
- `sync error`

## 2.4 congrid-site

### 开始（示例）
```bash
cd /home/eking/workspace/congrid.net
go run ./cmd/congrid-site \
  --addr :8080 \
  --base-url https://congrid.net \
  --slots-store chain \
  --chain-id congrid-main \
  --node tcp://127.0.0.1:26657 \
  --slots-grpc 127.0.0.1:9090 \
  --keyring-backend os
```

可选：如果 content-grid-d 的 keyring 不在默认路径，请设置 `--keyring-dir`。

健康检查：
- `/`、`/marketplace`、`/publisher/dashboard` 可访问
- 提交slot/lease时，tx成功返回，有txhash

---

## 3.日常检查（每班）

- [ ] 链高正常增长
- [ ] verifierd 最近 15 分钟 commit/reveal 成功率符合标准
- [ ] drand信标缠绕轮数持续增加（无长期停滞）
- [ ] 发布者已验证 比例没有异常下降
- [ ] 租赁违约（VIOLATED）比率未异常上升
- [ ]站点可用，提交路径可用

推荐阈值：
- 分配拉取成功率（5 分钟）`>= 99%`
- 提交成功率（15分钟）`>= 95%`
- 显示成功率（15分钟）`>= 90%`

---

## 4. 典型报警处理

## 4.1 警告：发布者长时间处于 PENDING 状态

故障排除：
1. 检查是否生成赋值
2. 检查 verifierd 是否有 `submitted commit` / `revealed result`
3. 检查`reveal window not open` / `account sequence mismatch`是否频繁出现
4. 如果日志显示成功但 assignment 仍无 `submission`，查询对应 tx 的 `code`；进块但 `code != 0` 仍然是失败

命令：
```bash
./content-grid-d query registry publisher --domain <domain> --node tcp://127.0.0.1:26657 --grpc-addr 127.0.0.1:9090 --grpc-insecure -o json
./content-grid-d verifier assignments <verifier-addr> --node tcp://127.0.0.1:26657 --grpc-addr 127.0.0.1:9090 --grpc-insecure -o json
./content-grid-d query tx <txhash> --node tcp://127.0.0.1:26657 -o json
```

缓解措施：
- 确认 verifierd 配置（轮询、提交窗口、`commit_start_buffer_seconds`、`tx_inclusion_timeout_seconds`）
- 如有必要，重新启动verifierd（保留日志）
- 如果链参数不合理，按照参数调整流程

---

## 4.2 警告：揭示失败率飙升

故障排除：
1. 故障是否集中在窗口定时（窗口未打开/关闭）
2. 帐号顺序是否冲突？
3. 节点是否产生块抖动或时钟偏差

缓解措施：
- 确认tx串行提交是否有效
- 适当增加 `CONGRID_VERIFIER_COMMIT_START_BUFFER_SECONDS`、`CONGRID_VERIFIER_TX_INCLUSION_TIMEOUT_SECONDS`
- 确认 `CONGRID_VERIFIER_STATE_DIR` 持久化，避免 nonce 丢失后无法 reveal
- 检查系统时钟 (NTP)

---

## 4.3 警告：链接节点恐慌/共识失败

立即行动：
1. 保护现场日志（节点+verifierd+站点）
2. 拉起备节点或者重启（按计划）
3. 通知 SEV1 值班组

故障排除要点：
- 恐慌堆栈顶部模块
- 最近的更改（代码/参数）
- 是否可以通过回滚来缓解

---

## 5. 紧急行动（最少一套）

- 暂停新增业务入口（站点层）
- 暂停插槽列表（如有必要）
- 回滚到之前的稳定版本
- 恢复后逐渐增加音量

> 建议对上述操作编写脚本以避免手动错误。

---

## 6.回滚流程（执行版）

1. 进入回滚窗口的公告（记录时间、影响范围）
2. 记录当前版本号和参数快照
3. 切换回稳定版本（推荐回滚锚点：`4202f66`，根据实际发布版本调整）
4. 重启服务并进行健康检查
5. 验证核心链接（发布者验证/槽/租约）
6. 宣告回滚完成

回滚后必须执行：
```bash
cd /home/eking/workspace/congrid.net
go test ./...
./scripts/e2e_smoke.sh /tmp/congrid-e2e-home-rollback-check
```

---

## 7. 事后分析

- 活动编号：
- 影响范围：
- 发现时间/恢复时间：
- 根本原因：
- 直接修复：
- 长期改进：
- 业主+截止日期：

---

## 8. 常见故障排除命令

```bash
cd /home/eking/workspace/congrid.net

# 全量测试
go test ./...

# e2e smoke
./scripts/e2e_smoke.sh /tmp/congrid-e2e-home

# 查询 publisher
./content-grid-d query registry publisher --domain <domain> --node tcp://127.0.0.1:26657 --grpc-addr 127.0.0.1:9090 --grpc-insecure -o json

# 查询 verifier assignments
./content-grid-d verifier assignments <verifier-addr> --node tcp://127.0.0.1:26657 --grpc-addr 127.0.0.1:9090 --grpc-insecure -o json
```
