# Congrid站点

Congrid（内容网格协议）官方网站的小型 Go Web 服务器。

## 本地运行

```bash
go run ./cmd/congrid-site --addr :8080 --base-url http://localhost:8080
```

### 链支持的slot市场（钱包签名）

插槽和租约直接从链上读取，插槽创建、状态更新和租约下单由浏览器钱包签名（Keplr/Leap）。

```bash
go run ./cmd/congrid-site \
  --addr :8080 \
  --base-url http://localhost:8080 \
  --slots-store chain \
  --chain-id <chain-id> \
  --node <rpc-url> \
  --slots-grpc <grpc-host:port>
```

可选插槽默认值：`--slot-rate-denom`、`--slot-unit-seconds`、`--slot-min-duration-seconds`、`--slot-max-duration-seconds`。使用 `--gas-prices` 设置钱包 gas price（默认 `0.001ucongrid`）。

打开： <http://localhost:8080>

## 为什么要去？

该网站有意由 Go 提供服务，因此我们可以添加第一方分析、归因和链上/链下集成（例如发布者徽章验证助手），而无需重写堆栈。

## 路线

- `/` — 家
- `/marketplace` — 发布者slot市场
- `/leases` — 租约发布板（slot/lease ID + 可复制嵌入片段）
- `/publishers` — 发布商入职（第三方钱包连接、域名+钱包填写、自动生成徽章片段和注册命令）
- `/publisher/dashboard` — 管理发布商位置（创建、暂停、取消列出）
- `/verifiers` — verifier 加入
- `/docs` — 指向存储库文档的指针
- `/airdrop` — 验证主页徽章并为每个主域发送一次性可选启动空投（启用时）
- `/badge.png` — 可嵌入验证徽章（保留查询参数以供将来归因）
- `/static/*` — CSS + 资源
