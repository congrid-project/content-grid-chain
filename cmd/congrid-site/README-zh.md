# Congrid站点

Congrid（内容网格协议）官方网站的小型 Go Web 服务器。

## 本地运行

```bash
go run ./cmd/congrid-site --addr :8080 --base-url http://localhost:8080
```

### 链支持的老虎机市场

要使用链上插槽/租约而不是内存中的演示存储：

```bash
go run ./cmd/congrid-site \
  --addr :8080 \
  --base-url http://localhost:8080 \
  --slots-store chain \
  --chain-id <chain-id> \
  --node <rpc-url> \
  --slots-grpc <grpc-host:port> \
  --slots-key <publisher-key-name> \
  --keyring-backend <backend>
```

可选插槽发送标志：`--slots-home`、`--slot-fees`、`--slot-gas`、`--slot-gas-prices`、`--slot-gas-adjustment`、`--slot-rate-denom`、`--slot-unit-seconds`、`--slot-min-duration-seconds`、`--slot-max-duration-seconds`、`--lease-key`。

打开： <http://localhost:8080>

## 为什么要去？

该网站有意由 Go 提供服务，因此我们可以添加第一方分析、归因和链上/链下集成（例如发布者徽章验证助手），而无需重写堆栈。

## 路线

- `/` — 家
- `/marketplace` — 发行商老虎机市场
- `/publishers` — 发布商入职（徽章片段 + 注册步骤）
- `/publisher/dashboard` — 管理发布商位置（创建、暂停、取消列出）
- `/verifiers` — 验证者加入
- `/docs` — 指向存储库文档的指针
- `/airdrop` — 验证主页徽章并为每个主域发送一次性费用空投（启用时）
- `/badge.png` — 可嵌入验证徽章（保留查询参数以供将来归因）
- `/static/*` — CSS + 资源
