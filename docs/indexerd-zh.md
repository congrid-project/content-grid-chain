# indexerd（链下）——发布者主页索引、嵌入与相似性签名

`indexerd` 会定期抓取 publisher 首页，提取规范化文本，生成嵌入，派生**紧凑的相似性签名**，并提供 HTTP API，供 verifier 及其他链下组件查询缓存结果。

## 为什么需要它

- 避免每个 verifier 都重复抓取相同的 publisher 首页。
- 避免每个 verifier 都重复执行嵌入推理。
- 提供一个短小、便于传输的**签名**（例如 128-bit），用于：
  - 多样性 / 去重启发式
  - “过于相似”门槛判断
  - 轻量级相似度分桶

## publisher 发现方式

`indexerd` 可以从以下来源建立 publisher 索引：

- **链上注册表（推荐）**：通过 gRPC 查询注册表模块的 `Query/Publishers`。
- **静态列表**：配置文件里的 `publishers` 字段（适合本地 / 开发环境）。

如果两者都配置了，`indexerd` 会取并集并去重。

当前的活跃状态过滤规则：

- 只有当 `status == VERIFIED` 且 `cooldown_until_unix <= now` 时，链上发现的 publisher 才会被索引。
- 如果某个已索引 publisher 不再活跃，indexerd 会把它从内存缓存以及（启用时）Chroma 中剔除。
- 当配置了 `chain_grpc_addr` 时，静态列表中的 publisher 也会经过链状态过滤。

## 索引内容

对每个 publisher 首页，`indexerd` 会存储：

- 规范化后的 markdown（best-effort）
- 嵌入向量（仅供内部语义查询 / 调试）
- 紧凑的相似性签名（默认 `signature_bits=128`）
- 抓取页面字节流的 `body_sha256`（用于绑定 / 调试）
- 找到的 Congrid 链接数量
- 从第一个 Congrid 徽章图片 URL 中提取的钱包地址：
  `https://congrid.net/...?...publisher=<domain>&wallet=<addr>`（best-effort）

## 依赖

先启动基于 Chroma 的辅助服务。`indexerd` 会通过它完成嵌入、持久化和相似站点查询：

```bash
python offchain/chromad/server.py
```

## 配置

```bash
cp offchain/indexerd/config.example.json offchain/indexerd/config.json
```

关键字段：

- `chain_grpc_addr`：链 gRPC 端点，例如 `127.0.0.1:9090`
- `publishers`：可选的静态域名列表（可包含端口）
- `listen_addr`：例如 `127.0.0.1:9100`
- `chroma_base_url`：Chroma 辅助服务地址
- `chroma_collection`：publisher 文档使用的 collection 名称
- `index_interval_minutes`：重新索引间隔
- `signature_bits`：返回签名的位数（必须是 8 的倍数；默认 128）

## 运行

索引一次：

```bash
go run ./offchain/indexerd --config offchain/indexerd/config.json --once
```

守护进程模式：

```bash
go run ./offchain/indexerd --config offchain/indexerd/config.json
```

## API

- `GET /healthz`
- `GET /v1/publishers`：列出缓存的 publisher 文档
- `GET /v1/publishers/{domain}`：读取指定 publisher 的缓存文档
- `POST /v1/index`：触发后台重新索引
- `POST /v1/query`：语义搜索（使用已存储的嵌入）
- `POST /v1/similar`：查询相似 publisher 域名（`limit` 可通过 JSON body 或 query 参数传入，默认 `15`）

### Redaction / verbose 模式

默认情况下，`GET /v1/publishers*` 会**隐藏大字段**（`markdown`、`embedding`）。

仅在调试时包含这些字段：

- `GET /v1/publishers?verbose=true`
- `GET /v1/publishers/{domain}?verbose=true`

### 示例查询

```bash
curl -s http://127.0.0.1:9100/v1/query \
  -H 'content-type: application/json' \
  -d '{"text":"news about AI", "limit": 5}'
```
