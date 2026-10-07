# Congrid站点

Congrid（内容网格协议）官方网站的小型 Go Web 服务器。

## 语言切换

所有页面均支持英文、中文和法语，点击导航中的地球图标，在下拉菜单中选择 `English / 中文 / Français`。页面由服务器按所选语言渲染，并通过 `congrid_language` Cookie 记住语言，后续页面和表单提交会沿用该选择。也可使用 `?lang=en`、`?lang=zh` 或 `?lang=fr` 指定语言；切换时保留其他查询参数。默认语言为英文。三个语言的首页共用同一张未翻译的架构图片。

页面正文、标题和描述、状态标签及浏览器提示的译文统一维护在 `static/translations.json` 中。钱包连接使用 Keplr。

## 博客与内容管理

公开博客入口为 `/blog`，文章网址为 `/blog/{slug}`，沿用官网的导航、深色配色和
响应式布局。支持 Markdown 正文（链接、图片、表格、代码块）、中英法文章语言、
新闻/指南/更新分类、作者、摘要、封面图片，以及独立 SEO 标题和描述。界面语言
与文章语言独立，系统不会自动翻译文章正文。

后台入口为 `/cms/login`，使用一个管理员账号，无公开注册。账号仅用于 `/cms`，
主网站和博客均不需要登录。草稿与已保存版本的预览仅对管理员可见；发布后文章
才会进入博客、站点地图和 RSS。改回草稿即撤下公开文章，其公开网址返回 404。
首次发布后网址保持固定，以保留已有链接；编辑保留首次发布时间。永久删除需要
勾选确认，同时编辑会检查版本，防止相互覆盖。

### 管理员初始化

默认 SQLite 文件为 `./congrid-cms.db`，保存文章、密码哈希和过期会话。
生产环境建议指定 `/var/lib/congrid-site/cms.db`，由网站服务用户持有，放在公开
downloads/static 目录之外。数据库权限为 `0600`，采用 WAL 模式。

**不提供默认密码**。首次启动前配置：

```bash
export CONGRID_CMS_DB=/var/lib/congrid-site/cms.db
export CONGRID_CMS_ADMIN_USER=admin
export CONGRID_CMS_ADMIN_PASSWORD_FILE=/etc/congrid-site/cms-password
```

先创建密码文件，内容为 12–72 字节的密码，仅允许服务用户读取。文件末尾换行会
移除，其他空格保留。也可通过 `CONGRID_CMS_ADMIN_PASSWORD` 环境变量传入密码；
配置了密码文件时优先使用文件。将变量写入网站服务配置，保留原有链/RPC 启动
参数，启动后访问 `https://congrid.net/cms/login`。

初始化密码仅创建第一个账号，后续重启不会覆盖已保存凭据。未配置账号时公开
博客仍可访问，但后台不能登录；系统没有对外开放的管理员初始化接口。创建账号
后，普通启动无需继续提供初始化密码。

| 参数 | 环境变量/默认值 | 用途 |
| --- | --- | --- |
| `--cms-db` | `CONGRID_CMS_DB`，默认 `./congrid-cms.db` | SQLite 文件路径 |
| `--cms-admin-user` | `CONGRID_CMS_ADMIN_USER`，默认 `admin` | 初始化/重置用户名 |
| `--cms-admin-password-file` | `CONGRID_CMS_ADMIN_PASSWORD_FILE` | 初始化/重置密码文件，未设置则读取 `CONGRID_CMS_ADMIN_PASSWORD` |
| `--cms-reset-admin-password` | 默认 `false` | 设置管理员凭据、注销后台会话后退出 |

忘记密码或需更换凭据时，修改密码文件后以服务用户执行；不需要链配置：

```bash
go build -o /tmp/congrid-site ./cmd/congrid-site
/tmp/congrid-site \
  --cms-db /var/lib/congrid-site/cms.db \
  --cms-admin-user admin \
  --cms-admin-password-file /etc/congrid-site/cms-password \
  --cms-reset-admin-password
```

重置保留全部文章，但会注销所有 CMS 会话。备份应使用 SQLite 备份接口，或先停止
网站再复制数据库；WAL 运行时不能仅复制正在使用的 `.db` 文件。

后台会话有效期为 12 小时，Cookie 为 HttpOnly、SameSite=Strict，路径限于 `/cms`。
HTTPS base URL 在反向代理终止 TLS 时也会启用 Secure Cookie；本地 HTTP 测试需使用
与浏览器地址一致的 HTTP base URL。密码使用 bcrypt，数据库仅保存会话令牌哈希；
写操作检查 CSRF 令牌和来源。按直接连接地址限制为每 10 分钟最多 10 次登录尝试，
并限制密码校验并发。反向代理需额外按真实客户端 IP 限流，应用不会直接信任转发头。

### SEO 与内容发现

页面提供 canonical 网址、社交分享元信息和语言 alternate；文章另外提供
`BlogPosting` JSON-LD、首次发布/更新时间和可选分享图片。`/sitemap.xml` 与
`/feed.xml` 仅收录已发布内容。后台和预览设置 `noindex`、`no-store`，
`/robots.txt` 排除 `/cms`。分页使用独立 canonical，分类/语言筛选结果不索引。
部署并发布首批文章后，可将 `https://congrid.net/sitemap.xml` 提交至搜索引擎站长工具。

## 验证

```bash
go test ./cmd/congrid-site
node --test cmd/congrid-site/testdata/wallet.test.mjs
```

## 本地运行

```bash
go run ./cmd/congrid-site --addr :8080 --base-url http://localhost:8080
```

### 历史市场配置（钱包签名）

> Slot marketplace 已下线（deprecated），仅保留文档备查

插槽和租约直接从链上读取，插槽创建、状态更新和租约下单由浏览器钱包签名（Keplr）。

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
- `/publishers` — 网站注册（连接钱包或填写钱包地址，生成成员标识的嵌入代码和注册命令）
- `/verifiers` — verifier 加入
- `/docs` — 指向存储库文档的指针
- `/airdrop` — 验证网站首页的成员标识并发放启动空投；启用此功能后，每个主域名可领取一次
- `/badge.svg` — Congrid 成员标识图片，使用官方 SVG 标志，并保留用于归因的查询参数
- `/badge.png` — 兼容旧代码片段的别名，返回相同的 SVG 内容
- `/static/*` — CSS + 资源
- `/downloads/{filename}` — 发布归档下载（支持 HEAD 和 Range，不提供目录列表）
