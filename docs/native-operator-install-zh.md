# 原生一键安装（无容器）

`cmd/congrid-site/downloads/install.sh` 用于在 Linux 或 macOS 主机上直接安装并运行完整
operator 栈：

- `content-grid-d` 全节点；
- `chromad` 向量与嵌入服务；
- `indexerd` 发布者索引服务；
- `verifierd` 发布者核验和 drand 投递服务。

安装器不使用 Docker、Podman 或其他容器运行时，支持 `amd64` 和 `arm64`：

- Linux：使用 systemd，在专用 `congrid` 系统用户下运行；
- macOS：使用当前登录用户的 launchd LaunchAgent，在用户登录期间持续运行。

## 发布网站需要准备的文件

安装器使用一个原生发布包。正式 `content-grid-d`、`verifierd`、`indexerd` 和
chromad 来自当前 Git 提交；包内还包含一个只用于重放历史区块的
`content-grid-d-pre-upgrade`。构建 Linux 和 macOS 的四种组合：

```bash
scripts/build-native-release.sh linux amd64 ./dist
scripts/build-native-release.sh linux arm64 ./dist
scripts/build-native-release.sh darwin amd64 ./dist
scripts/build-native-release.sh darwin arm64 ./dist
```

构建脚本会自动执行 Go 交叉编译、从提交
`ef331816c0c213a145e26f7719bc4fb395e03c0a` 构建升级前节点二进制、整理目录、
调用 `tar` 并生成 SHA-256。只有在明确发布另一个兼容历史版本时，才使用
`CONGRID_PRE_UPGRADE_REF=<commit>` 覆盖该提交。

如果二进制已经由其他发布流程构建完成，手工打包时目录必须是：

```text
congrid-native/
├── bin/
│   ├── content-grid-d
│   ├── content-grid-d-pre-upgrade
│   ├── verifierd
│   └── indexerd
└── chromad/
    ├── server.py
    └── requirements.txt
```

在包含 `congrid-native` 目录的上一级执行 tar 命令：

```bash
tar -czf congrid-native-linux-amd64.tar.gz congrid-native
sha256sum congrid-native-linux-amd64.tar.gz \
  > congrid-native-linux-amd64.tar.gz.sha256
```

macOS 没有 `sha256sum` 时使用：

```bash
tar -czf congrid-native-darwin-arm64.tar.gz congrid-native
shasum -a 256 congrid-native-darwin-arm64.tar.gz \
  > congrid-native-darwin-arm64.tar.gz.sha256
```

其他平台/架构只需替换归档名；归档内部顶层目录始终保持为 `congrid-native`。
`content-grid-d-pre-upgrade` 必须为同一目标 OS/架构，并由上述固定提交构建。
构建脚本会把它的版本标记为 `pre-drand-strict-v2-ef331816`，安装器会校验该标记，
防止误把当前节点二进制放到兼容文件名下。

将以下文件复制到官网实际使用的 downloads 目录：

```text
install.sh
congrid-native-linux-amd64.tar.gz
congrid-native-linux-amd64.tar.gz.sha256
congrid-native-linux-arm64.tar.gz
congrid-native-linux-arm64.tar.gz.sha256
congrid-native-darwin-amd64.tar.gz
congrid-native-darwin-amd64.tar.gz.sha256
congrid-native-darwin-arm64.tar.gz
congrid-native-darwin-arm64.tar.gz.sha256
genesis.json
seeds.txt
seeds.txt.sha256
```

`install.sh` 的仓库版本位于 `cmd/congrid-site/downloads/install.sh`。发布包默认被
downloads 目录的 `.gitignore` 排除，应由正式发布流程复制。安装器要求发布包的
`.sha256` 文件存在且匹配，否则会拒绝安装。`seeds.txt` 和
`seeds.txt.sha256` 已纳入仓库，可随网站一起发布。

发布后先检查：

```bash
curl -fI https://congrid.net/downloads/install.sh
for artifact in \
  congrid-native-linux-amd64.tar.gz \
  congrid-native-linux-arm64.tar.gz \
  congrid-native-darwin-amd64.tar.gz \
  congrid-native-darwin-arm64.tar.gz; do
  curl -fI "https://congrid.net/downloads/$artifact"
  curl -fI "https://congrid.net/downloads/$artifact.sha256"
done
curl -fI https://congrid.net/downloads/genesis.json
curl -fI https://congrid.net/downloads/seeds.txt
curl -fI https://congrid.net/downloads/seeds.txt.sha256
```

## 用户安装

Linux 使用带有 sudo 权限的普通用户；macOS 直接使用普通登录用户，不要先执行
`sudo`：

```bash
curl -fsSL https://congrid.net/downloads/install.sh | bash
```

Linux 会通过 apt/dnf/yum/zypper/pacman 补齐依赖。macOS 自带 Python 3 且支持 venv
时直接使用；否则需要先安装 [Homebrew](https://brew.sh)，脚本会调用
`brew install python`。安装器会验证虚拟环境内的 pip；如果检测到已有
`chromad/.venv` 缺少 pip，会先尝试修复，必要时只重建该虚拟环境。

脚本会直接从 `/dev/tty` 读取回答，因此即使脚本内容通过管道送给 Bash，交互仍然
有效。首次安装会询问：

- 节点 moniker；
- 可选 persistent peers 和公开 P2P 地址；
- P2P 地址簿是否拒绝私网地址；
- `indexerd` 和 `verifierd` 的监听地址；
- verifier key 名称和 keyring 口令；
- 创建新 verifier key，或通过 mnemonic 恢复现有 key；
- verifier 交易 gas prices。

Chain ID 固定使用 `congrid-main`，genesis 默认使用当前下载目录下的
`genesis.json`，交互安装不会再询问这两个值。测试网络等高级场景仍可分别通过
`CONGRID_CHAIN_ID` 和 `CONGRID_GENESIS_URL` 覆盖。

`congrid-main` 曾在高度 13000 执行 `drand-strict-v2` 软件升级。全新节点会由
安装器自动使用升级前二进制重放 1–12999，在 13000 自动切换当前二进制并执行
迁移；用户不需要手工替换文件。安装器会等节点完成追块后再启动 indexer 和
verifier，避免它们读取不完整的历史状态。

drand delivery 默认启用且不再询问。多个 verifier 可以安全地同时启用，程序会按
确定性顺序选择主投递者和后备投递者。只有已经确认网络中存在其他投递节点的高级
运维场景，才应设置 `CONGRID_DRAND_DELIVERY_DISABLED=true` 关闭本实例的投递职责。

若创建新 key，安装器会把唯一的 mnemonic JSON 备份保存到执行安装的用户 home，
权限为 `0600`，并打印完整路径。必须在给该地址充值前把它转移到安全的离线存储。

安装器默认下载并校验：

```text
https://congrid.net/downloads/seeds.txt
https://congrid.net/downloads/seeds.txt.sha256
```

`seeds.txt` 使用每行一个 seed 的格式，安装器会忽略空行和 `#` 注释，并将所有
条目合并成 CometBFT `config.toml` 需要的逗号列表。当前配置为：

```text
a224505b2c9cacca1263c2fb0a1488f54ff031c3@146.235.195.208:26656
72165a18f87c580ca92d1db1bb7b1a907612393d@34.71.169.8:26656
2c743d0fe8a4aba989c072ef97f7fd56e727e51e@34.29.229.254:26656
9d1390f37736424f2c71b88174a8ac0a7810e4cd@168.138.73.149:26656
```

修改列表后必须重新生成校验文件，并将两个文件作为同一次发布上线：

```bash
cd cmd/congrid-site/downloads
shasum -a 256 seeds.txt > seeds.txt.sha256
```

Linux 也可将 `shasum -a 256` 替换为 `sha256sum`。网站要求客户端每次重新验证这
两个动态文件的缓存，安装器仍会在内容与 checksum 不一致时安全地终止。

每个 CometBFT seed 仍采用 `<node-id>@<host>:<p2p-port>` 语法，但 host 可以是
IP 地址或域名，并不要求是 `congrid.net`。节点必须至少配置一个 seed 或
persistent peer。只有明确部署隔离节点时，才可设置
`CONGRID_ALLOW_NO_PEERS=true` 绕过该检查。

## 安装结果

Linux 主要路径：

```text
/usr/local/bin/content-grid-d
/usr/local/bin/verifierd
/usr/local/bin/indexerd
/usr/local/libexec/congrid/content-grid-d-pre-upgrade
/usr/local/libexec/congrid/content-grid-node-bootstrap
/opt/congrid/chromad
/etc/congrid
/var/lib/congrid
```

systemd 单元：

```text
congrid-node.service
congrid-chroma.service
congrid-indexer.service
congrid-verifier.service
```

macOS 默认路径：

```text
~/.local/bin/{content-grid-d,verifierd,indexerd}
~/.local/share/congrid/chromad
~/.local/share/congrid/libexec
~/.config/congrid
~/.content-grid
~/Library/LaunchAgents/net.congrid.*.plist
```

macOS 使用以下 label：

```text
net.congrid.node
net.congrid.chroma
net.congrid.indexer
net.congrid.verifier
```

服务仅将 P2P 监听在所有网卡的 TCP/26656。节点 RPC、API、gRPC 和 chromad 默认仅
监听 loopback；indexerd/verifierd 的监听地址由安装时输入，默认也是 loopback。
这两个监听地址的 host 可选 `127.0.0.1`、`localhost` 或 `0.0.0.0`。

常用检查命令：

```bash
sudo systemctl status congrid-node congrid-chroma congrid-indexer congrid-verifier
sudo journalctl -u congrid-node -f
sudo journalctl -u congrid-verifier -f
curl -fsS http://127.0.0.1:8000/healthz
curl -fsS http://127.0.0.1:9100/healthz
curl -fsS http://127.0.0.1:9200/healthz
```

如果 `congrid-indexer` 启动超时，先检查它等待的两个上游端点：

```bash
sudo systemctl status congrid-node congrid-chroma --no-pager --full
sudo journalctl -u congrid-node -u congrid-chroma -n 100 --no-pager
curl -fsS http://127.0.0.1:8000/healthz
timeout 3 bash -c '</dev/tcp/127.0.0.1/9090'
```

新版安装器会给首次链同步和 indexer/verifier 的启动前健康检查预留 60 分钟，
并按 node → Chroma → 等待追块完成 → indexer → verifier 的依赖顺序启动。任一
服务失败时会自动打印四个服务的状态和失败服务日志。

如果节点日志出现 `unexpected ./data/application.db detected`，不要删除
`/var/lib/congrid/data`。旧二进制会在 systemd 工作目录与 `--home` 相同时把正常
数据库误判为相对路径数据库；安装器 `1.2.3+` 会使用兼容工作目录，新构建的
`content-grid-d` 也已经修正这项路径判断。

安装器 `1.2.x` 曾直接用升级后节点二进制从 genesis 启动，受影响节点会停在高度
1，并在日志中出现 `wrong Block.Header.AppHash`。重新运行 `1.3.0+` 安装器时，
若本地 RPC 仍可访问，它会识别该特定高度-1状态，将旧 `data` 备份到
`/var/backups/congrid-incompatible-height1-<timestamp>`（macOS 位于 node home
的 `backups/`），只重置可重放的链数据库，然后自动走正确的升级路径。verifier
keyring、助记词备份、节点身份和配置不会被删除。

macOS 检查命令：

```bash
launchctl print "gui/$(id -u)/net.congrid.node"
tail -f ~/.content-grid/logs/node.log
tail -f ~/.content-grid/logs/verifier.log
```

安装完成只代表进程已经运行。verifier 地址还需要充值，并执行
`content-grid-d verifier bond` 后才能接收 assignment。主机防火墙和云安全组也要
允许入站 TCP/26656。

## 更新和重复执行

再次运行同一条安装命令会更新发布包、chromad Python 依赖、配置和服务定义，
然后按依赖顺序重启四个服务。已有的以下数据不会被清除：

- `genesis.json` 和链数据库；
- 共识节点密钥；
- verifier keyring；
- verifier pending state；
- chromad 数据。

已有节点的 chain ID 必须与输入一致；安装器不会使用下载的 genesis 覆盖已有
genesis。唯一的自动重置例外是上一节所述、可精确识别的 `1.2.x` 高度-1
不兼容链数据库；重置前一定会先备份，且不会触碰密钥和配置。

## 非交互安装

自动化环境可以使用：

```bash
curl -fsSL https://congrid.net/downloads/install.sh |
  CONGRID_NON_INTERACTIVE=true \
  CONGRID_MONIKER=node-01 \
  CONGRID_VERIFIER_KEY_NAME=verifier-key \
  CONGRID_VERIFIER_KEY_ACTION=recover \
  CONGRID_VERIFIER_MNEMONIC='word ...' \
  CONGRID_VERIFIER_KEYRING_PASSPHRASE='replace-with-a-secret' \
  bash
```

Linux 非交互模式需要 root，或已经配置免口令 sudo。macOS 非交互模式使用当前
用户。敏感变量可能被调用方的进程管理或 CI 日志记录；生产环境优先使用交互模式。

设置 `CONGRID_START_SERVICES=false` 或传入 `--no-start` 可以只安装、配置并 enable
服务，不立即启动。管道方式传参数的示例：

```bash
curl -fsSL https://congrid.net/downloads/install.sh | bash -s -- --no-start
```

需要从测试下载站安装时，可设置 `CONGRID_DOWNLOAD_BASE_URL`，也可分别使用
`CONGRID_BUNDLE_URL` 和 `CONGRID_BUNDLE_SHA256` 指定并校验发布包。默认 seed
地址会跟随 `CONGRID_DOWNLOAD_BASE_URL`；也可使用 `CONGRID_SEEDS_URL`、
`CONGRID_SEEDS_SHA256_URL` 或 `CONGRID_SEEDS_SHA256` 单独覆盖。若自动化部署已经
直接提供完整 seed 列表，可以设置 `CONGRID_P2P_SEEDS` 跳过 seed 文件下载。
