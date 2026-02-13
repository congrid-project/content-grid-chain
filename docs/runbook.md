# 生产运行手册（Runbook）

> 适用于：`content-grid-d`、`verifierd`、`drand-relayer`、`congrid-site`。  
> 目标：出现问题时，值班同学 5 分钟内定位方向，15 分钟内执行缓解动作。

---

## 0. 环境信息（当前建议基线）

- 代码目录：`/home/eking/workspace/congrid.net`
- 链节点 HOME：`/home/eking/.content-grid-d`（当前默认基线）
- verifierd 配置：`/home/eking/workspace/congrid.net/offchain/verifierd/config.json`
- drand-relayer 配置：`/home/eking/workspace/congrid.net/offchain/drandrelayer/config.json`
- site 配置（建议 env 文件）：`/home/eking/workspace/congrid.net/.env.site`
- 日志目录（建议）：`/home/eking/workspace/congrid.net/logs/`
- 时区：`America/Toronto`

负责人（当前临时）：
- 链：`eking`
- verifierd：`eking`
- site：`eking`
- 值班通知：`Telegram @eking (id:6148992071)`

---

## 1. 服务清单与职责

- 链节点：`content-grid-d`
  - 责任：共识、状态执行、gRPC/RPC 提供
- 验证代理：`verifierd`
  - 责任：拉取 assignment，commit/reveal，站点验证
- 随机信标中继：`drand-relayer`
  - 责任：拉取 drand 最新 beacon 并上链提交
- 官网/市场：`congrid-site`
  - 责任：展示与用户入口，链上 slot/lease 提交

---

## 2. 启停与健康检查

## 2.1 content-grid-d

### 启动（手工模式）
```bash
cd /home/eking/workspace/congrid.net
./content-grid-d start --home /home/eking/.content-grid-d
```

### 健康检查
```bash
# RPC
echo >/dev/tcp/127.0.0.1/26657

# gRPC
echo >/dev/tcp/127.0.0.1/9090

# 最新区块（应持续增长）
./content-grid-d query block --type height --home /home/eking/.content-grid-d --node tcp://127.0.0.1:26657 -o json
```

关键检查：
- 最新区块持续增长
- 无连续 panic / consensus failure

## 2.2 verifierd

### 启动（手工模式）
```bash
cd /home/eking/workspace/congrid.net
./verifierd --config /home/eking/workspace/congrid.net/offchain/verifierd/config.json
```

### 单轮探活
```bash
./verifierd --config /home/eking/workspace/congrid.net/offchain/verifierd/config.json --once
```

关键日志关键词：
- `submitted commit`
- `revealed result (passed=true|false)`
- `commit failed` / `reveal failed`

## 2.3 drand-relayer

### 启动（手工模式）
```bash
cd /home/eking/workspace/congrid.net
./drand-relayer --config /home/eking/workspace/congrid.net/offchain/drandrelayer/config.json
```

### 单轮探活
```bash
./drand-relayer --config /home/eking/workspace/congrid.net/offchain/drandrelayer/config.json --once
```

关键日志关键词：
- `submitted beacon round=`
- `sync error`

## 2.4 congrid-site

### 启动（示例）
```bash
cd /home/eking/workspace/congrid.net
go run ./cmd/congrid-site \
  --addr :8080 \
  --base-url https://congrid.net \
  --slots-store chain \
  --chain-id content-grid-1 \
  --node tcp://127.0.0.1:26657 \
  --slots-grpc 127.0.0.1:9090 \
  --slots-key publisher-key \
  --slots-home /home/eking/.content-grid-d \
  --keyring-backend os
```

健康检查：
- `/`、`/marketplace`、`/publisher/dashboard` 可访问
- 提交 slot/lease 时 tx 返回成功且有 txhash

---

## 3. 日常巡检（每班次）

- [ ] 链高度正常增长
- [ ] verifierd 最近 15 分钟 commit/reveal 成功率达标
- [ ] drand beacon 上链轮次持续增长（无长时间停滞）
- [ ] publisher VERIFIED 比例无异常下降
- [ ] lease 违约（VIOLATED）比例无异常上升
- [ ] site 可用，提交路径可用

建议阈值：
- assignment 拉取成功率（5min）`>= 99%`
- commit 成功率（15min）`>= 95%`
- reveal 成功率（15min）`>= 90%`

---

## 4. 典型告警处置

## 4.1 告警：publisher 长时间 PENDING

排查：
1. 查 assignment 是否生成
2. 查 verifierd 是否有 `submitted commit` / `revealed result`
3. 查是否频繁 `reveal window not open` / `account sequence mismatch`

命令：
```bash
./content-grid-d query registry publisher --domain <domain> --node tcp://127.0.0.1:26657 --grpc-addr 127.0.0.1:9090 --grpc-insecure -o json
./content-grid-d verifier assignments <verifier-addr> --node tcp://127.0.0.1:26657 --grpc-addr 127.0.0.1:9090 --grpc-insecure -o json
```

缓解：
- 确认 verifierd 配置（poll、commit window）
- 必要时重启 verifierd（保留日志）
- 如链参数不合理，走参数调整流程

---

## 4.2 告警：reveal 失败率飙升

排查：
1. 失败是否集中在窗口时序（window not open/closed）
2. 是否账号 sequence 冲突
3. 节点是否出块抖动或时钟偏差

缓解：
- 确认 tx 串行提交是否生效
- 适当增加 reveal 缓冲等待
- 检查系统时钟（NTP）

---

## 4.3 告警：链节点 panic / consensus failure

立即动作：
1. 保护现场日志（node + verifierd + site）
2. 拉起备用节点或重启（按预案）
3. 通知 SEV1 值班群

排查要点：
- panic 栈顶模块
- 最近变更（代码/参数）
- 是否可通过回滚缓解

---

## 5. 应急操作（最小集）

- 暂停新业务入口（site 层）
- 暂停 slot 上架（必要时）
- 回滚到上一个稳定版本
- 恢复后逐步放量

> 建议把上述动作脚本化，避免人工误操作。

---

## 6. 回滚流程（执行版）

1. 宣布进入回滚窗口（记录时间、影响范围）
2. 记录当前版本号与参数快照
3. 切回稳定版本（建议回滚锚点：`4202f66`，按实际发布版本调整）
4. 重启服务并做健康检查
5. 验证核心链路（publisher verify / slot / lease）
6. 宣布回滚完成

回滚后必须执行：
```bash
cd /home/eking/workspace/congrid.net
go test ./...
./scripts/e2e_smoke.sh /tmp/congrid-e2e-home-rollback-check
```

---

## 7. 事后复盘（Postmortem）

- 事件编号：
- 影响范围：
- 发现时间 / 恢复时间：
- 根因：
- 直接修复：
- 长期改进项：
- Owner + Deadline：

---

## 8. 常用排查命令

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
