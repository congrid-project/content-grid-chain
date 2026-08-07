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
  --downloads-dir ./cmd/congrid-site/downloads \
  --slots-store chain \
  --chain-id <chain-id> \
  --node <rpc-url> \
  --slots-grpc <grpc-host:port>
```

可选插槽默认值：`--slot-rate-denom`、`--slot-unit-seconds`、`--slot-min-duration-seconds`、`--slot-max-duration-seconds`。使用 `--gas-prices` 设置钱包 gas price（默认 `0.001ucongrid`）。

服务端注册和空投交易需要调用 `content-grid-d`。默认会从 `PATH` 查找 `content-grid-d`；生产环境建议安装到 `/usr/local/bin/content-grid-d`，或通过 `--content-grid-bin /path/to/content-grid-d` / `CONTENT_GRID_BIN` 显式指定。

打开： <http://localhost:8080>

## 空投服务

空投接口由官网后台直接访问网站首页完成验证，不等待链上 verifier 的
assignment/commit/reveal 流程。验证成功后，服务先在 SQL 中原子占用网站唯一键，
再把一次 bank 转账交给后台单线程 worker，并在后台确认交易是否进块。

网站唯一键刻意沿用 `registry.GetPrimaryDomain` 的“最后两段”规则：
`www.example.com` 与 `api.example.com` 共用 `example.com`；
`example.co.uk` 仍按现有简化规则使用 `co.uk`。钱包地址没有唯一约束，因此同一个
钱包可以代表多个不同的网站唯一键领取。

默认使用 SQLite：

```bash
export CONGRID_FAUCET_KEYRING_PASSPHRASE='<file-keyring-passphrase>'

go run ./cmd/congrid-site \
  --airdrop \
  --airdrop-db ./congrid-airdrop.db \
  --chain-id <chain-id> \
  --node <rpc-url> \
  --slots-grpc <grpc-host:port> \
  --keyring-backend file \
  --keyring-dir <keyring-dir> \
  --keyring-passphrase-env CONGRID_FAUCET_KEYRING_PASSPHRASE \
  --faucet-key faucet \
  --gas-prices 0.001ucongrid
```

如果 SQLite 路径中仍是旧版 JSON claim map，启动时会严格校验并导入，同时把原文件
保留为 `<path>.json.bak`。旧 JSON 损坏时服务会拒绝启动，不会把历史记录静默当成
空库。

共享部署可以改用 PostgreSQL：

```bash
export CONGRID_AIRDROP_DB_DRIVER=postgres
export CONGRID_AIRDROP_DB_DSN='postgres://user:pass@db/airdrop?sslmode=require'
```

同一个 faucet key 只允许一个进程运行转账 worker。多实例部署时，一个实例使用
`--airdrop-worker=true`，其余实例使用 `--airdrop-worker=false`；所有实例都可以通过
共享 PostgreSQL 接收和原子占用 claim。

claim 状态依次为 `verified`、`submitting`、`broadcast`、`confirmed`。明确被拒绝的
交易进入 `failed`；进程中断、CLI 返回结果不确定或确认超时会进入
`needs_reconcile`。这些记录仍保持占用且绝不会自动重发。人工修改状态前，运营人员
必须根据已存交易哈希/备注、收款地址余额和链上历史完成核对。

生产验证默认只允许 HTTPS，只允许跳转到原 host（例外为 `www` 别名），并拒绝解析
到私网、回环、link-local 和特殊用途 IP。`--airdrop-allow-http-verification`、
`--airdrop-allow-private-targets` 和 `--airdrop-allow-insecure-test-keyring` 仅用于开发，
生产环境不得启用。请使用独立、低余额的 faucet key，并在可信反向代理上配置按 IP
限流或 CAPTCHA。

## 发布文件下载

网站通过 `/downloads/{filename}` 提供发布归档。默认文件目录是
`cmd/congrid-site/downloads`，也可以使用 `--downloads-dir` 或环境变量
`CONGRID_SITE_DOWNLOADS_DIR` 指定持久化目录。

例如：

```bash
cp content-grid-d-linux-amd64.tar.gz cmd/congrid-site/downloads/
chmod 0644 cmd/congrid-site/downloads/content-grid-d-linux-amd64.tar.gz

curl -fI http://localhost:8080/downloads/content-grid-d-linux-amd64.tar.gz
```

生产下载地址：

```text
https://congrid.net/downloads/content-grid-d-linux-amd64.tar.gz
```

完整原生 operator 栈还提供 Linux/macOS 交互式一键安装器（不使用容器）：

```bash
curl -fsSL https://congrid.net/downloads/install.sh | bash
```

安装器会从 `/downloads/seeds.txt` 下载网络引导节点列表，并使用
`/downloads/seeds.txt.sha256` 校验内容。

发布安装器及其 `amd64` / `arm64` 原生发布包的流程见
`docs/native-operator-install-zh.md`。

文件在每次请求时从目录读取，复制完成后不需要重新编译或重启网站。归档文件默认被
`.gitignore` 排除；请通过发布流程复制到网站服务器。目录本身不会列出文件，只允许
下载顶层的普通文件；隐藏文件、子目录、符号链接和不安全文件名会返回 404。

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
- `/badge.svg` — 使用 Congrid 标准 SVG logo 的可嵌入验证徽章（保留查询参数用于归因）
- `/badge.png` — 兼容旧代码片段的别名，返回相同的 SVG 内容
- `/static/*` — CSS + 资源
- `/downloads/{filename}` — 发布归档下载（支持 HEAD 和 Range，不提供目录列表）
