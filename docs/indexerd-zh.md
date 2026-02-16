# indexerd（链下）——发布者主页索引+嵌入+相似性签名

`indexerd` 定期获取发布者主页，提取规范化文本，生成嵌入，派生**紧凑的相似性签名**，并公开 HTTP API 供验证者（和其他组件）查询缓存的结果。

＃＃ 为什么

- 避免每个验证者重复获取相同的发布者主页。
- 避免每个验证者重复运行嵌入推理。
- 提供一个简短、传输成本低的**签名**（例如 128 位），可用于：
  - 多样性/重复数据删除启发法
  - “太相似”门控
  - 轻量级相似度分桶

## 发布者发现

`indexerd` 可以为发布者建立索引：

- **链注册表（推荐）：**通过gRPC（`Query/Publishers`）查询注册表模块。
- **静态列表：** 配置中的 `publishers` （对于本地/开发有用）。

如果两者均已配置，`indexerd` 将索引联合（已删除重复数据）。

活动状态过滤（当前行为）：
- 仅当 `status == VERIFIED` 和 `cooldown_until_unix <= now` 时，链发现的发布者才会被编入索引。
- 如果之前索引的发布者不再处于活动状态，indexerd 会从内存缓存和（启用时）Chroma 中删除它。
- 配置 `chain_grpc_addr` 时，静态列表发布者也会通过链状态进行过滤。

## 什么被索引

对于每个发布者主页，`indexerd` 存储：

- 标准化降价（尽力而为）
- 嵌入向量（内部用于语义查询/调试）
- 紧凑的相似性签名（默认值：`signature_bits=128`）
- 获取的主页字节的 `body_sha256` （用于绑定/调试）
- 找到的 Congrid 链接数量
- 从第一个 Congrid 徽章图像 URL 中提取的钱包地址：
  `https://congrid.net/...?...publisher=<domain>&wallet=<addr>`（尽力而为）

＃＃ 要求

启动嵌入服务：

```bash
python offchain/services/sentence_transformer_server.py --host 0.0.0.0 --port 9000
```

## 配置

```bash
cp offchain/indexerd/config.example.json offchain/indexerd/config.json
```

关键领域：

- `chain_grpc_addr`：链 gRPC 端点（例如 `127.0.0.1:9090`）
- `publishers`：可选的静态域列表（可能包括端口）
- `listen_addr`：例如__代码_1__
- `embedder_base_url`：嵌入服务器基本 URL
- `index_interval_minutes`：重新索引的频率
- `signature_bits`：返回签名的大小（8的倍数；默认128）

＃＃ 跑步

索引一次：

```bash
go run ./offchain/indexerd --config offchain/indexerd/config.json --once
```

守护进程模式：

```bash
go run ./offchain/indexerd --config offchain/indexerd/config.json
```

## API

- __代码_0__
- `GET /v1/publishers` — 列出缓存的发布者文档
- `GET /v1/publishers/{domain}` — 获取发布者的缓存文档
- `POST /v1/index` — 触发后台重新索引
- `POST /v1/query` — 语义搜索（使用存储的嵌入）
- `POST /v1/similar` — 类似的发布者域（`limit` 通过 JSON 正文或查询参数；默认 `15`）

### 编辑/详细模式

默认情况下，`GET /v1/publishers*` **编辑大字段**（`markdown`、`embedding`）。

包含它们（仅调试）：

- __代码_0__
- __代码_0__

### 示例查询

```bash
curl -s http://127.0.0.1:9100/v1/query \
  -H 'content-type: application/json' \
  -d '{"text":"news about AI", "limit": 5}'
```
