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

## 发布安装器与安装包（1.5.0 / ibc-transfer-v1）

官网保留 `install.sh`、`genesis.json`、`seeds.txt` 及 seeds 校验文件。二进制安装包
默认从 GitHub Release `native-ibc-transfer-v1-r1` 下载，不使用 `latest`。
节点版本固定为 `ibc-transfer-v1`，源码提交固定为
`5907b65faa971afd9e0c29e74284baa03675e37c`；安装器同时检查 SHA-256、BUILD-INFO
中的源码/版本/平台和节点实际输出的版本。仅更改 Release 标签不会绕过版本检查。

从上述固定提交的独立源码目录构建（使用更新后的构建脚本）：

```bash
export CONGRID_BUILD_VERSION=ibc-transfer-v1
export CONGRID_INCLUDE_PRE_UPGRADE=false
bash scripts/build-native-release.sh linux amd64 ./dist
bash scripts/build-native-release.sh linux arm64 ./dist
bash scripts/build-native-release.sh darwin amd64 ./dist
bash scripts/build-native-release.sh darwin arm64 ./dist
```

每个平台生成 `congrid-native-<os>-<arch>.tar.gz` 和同名 `.sha256` 文件。
将八个文件全部上传到该 Release，验证完成后正式发布；草稿附件不能被匿名安装器下载。
保留现有 `ibc-transfer-v1` 节点升级 Release 及其附件。

包内结构：

```text
congrid-native/
├── BUILD-INFO
├── bin/
│   ├── content-grid-d
│   ├── verifierd
│   └── indexerd
└── chromad/
    ├── server.py
    └── requirements.txt
```

`1.5.0` 不需要 `content-grid-d-pre-upgrade`，也不会从创世块重放旧升级。
构建脚本默认省略历史辅助二进制；只有显式设置 `CONGRID_INCLUDE_PRE_UPGRADE=true`
才会尝试构建它，且需要仓库中存在对应旧提交。

发布后逐个平台检查 GitHub 上的包与校验文件可匿名下载，并将仓库中的新版
`cmd/congrid-site/downloads/install.sh` 部署到官网的 downloads 目录。
仓库修改不会自动更新官网脚本。

## 新节点状态同步

全新节点从高度 90000 之后的状态快照启动。需要两个不同节点的 RPC 和至少一个
可连接的快照提供者。`CONGRID_STATE_SYNC_RPC_SERVERS` 接收两个逗号分隔的 URL；
交互安装时也会询问。当前没有经过验证的第二个公共 RPC 默认值，必须由部署者提供。

```bash
# 用已验证且可从安装主机访问的两个 RPC 替换这两个值。
export CONGRID_STATE_SYNC_RPC_SERVERS='https://rpc-a.example/rpc,https://rpc-b.example/rpc'
curl -fsSL https://congrid.net/downloads/install.sh | bash
```

脚本核对两台节点的 chain ID、节点 ID、同步状态、同高度区块哈希和可信区块时间，
并检查链上解绑期足够长。可信高度取两台最新高度的较小值减 20，必须大于 90000；
可信块不能早于当前时间一小时。信任期为 24 小时。请保持系统时钟准确。

未指定 persistent peers 时，使用两台 RPC 公布的公网 P2P 地址；也可通过
`CONGRID_PERSISTENT_PEERS` 指定，列表中至少一台必须提供近期快照。
RPC HTTP 可用不代表 P2P 快照可用。没有快照时节点会等待，安装器最终报同步超时，
不会退回到 genesis 启动或自动删除数据库。

仅供维护者在 Mac 上复现已验证的 RPC 隧道方案：

```bash
# 终端一：保持隧道运行，直到首次同步完成。
ssh -N -T -o ExitOnForwardFailure=yes \
  -L 127.0.0.1:28657:127.0.0.1:26657 ociv

# 终端二：使用独立的新目录，避免碰到已有节点。
export CONGRID_STATE_SYNC_RPC_SERVERS='http://127.0.0.1:28657,https://congrid.net/rpc'
export CONGRID_HOME="$HOME/.content-grid-installed"
bash cmd/congrid-site/downloads/install.sh
```

SSH 别名 `ociv` 仅适用于维护者的环境，不作为公共安装依赖。同一公网 IP 下并行运行
多个节点可能被远端 P2P 的重复 IP 限制拒绝；测试时使用不同主机/出口，或明确配置
P2P 隧道，不要因此关闭生产节点的连接保护。

启动器在首次同步前重新获取可信区块，因此 `--no-start` 后延迟启动不会沿用过期锚点。
同步完成标记为 `config/state-sync-complete.json`；后续重启直接使用本地状态，不再依赖
引导 RPC。不要手工伪造标记或仅删除数据库。首次状态同步不提供快照高度之前的区块历史。

启动器持有进程锁；重复启动会拒绝。systemd 使用 journal，launchd 使用日志文件，
无需重复运行 `nohup ... >node.log`。对已经运行的安装器服务，重新安装会要求先停止它们，
以免替换运行中的二进制。已有非本安装器管理的节点目录会被拒绝；安装 verifier 时使用
下面的 `--components-only` 模式。

维护嵌入的启动器与测试：

```bash
python3 scripts/installer/embed.py
python3 -m unittest discover -s scripts/installer/tests -v
bash -n cmd/congrid-site/downloads/install.sh
```

`state_sync.py` 是维护源文件，由 embed.py 嵌入 install.sh；用户只需下载一个安装脚本。

## 用户安装

Linux 使用带有 sudo 权限的普通用户；macOS 直接使用普通登录用户，不要先执行
`sudo`：

```bash
curl -fsSL https://congrid.net/downloads/install.sh | bash
```

### 已有链节点：只安装其他组件

如果这台机器已经有正常运行的 `content-grid-d`，使用隔离的组件模式：

```bash
curl -fsSL https://congrid.net/downloads/install.sh |
  bash -s -- --components-only
```

该模式会询问现有节点的 RPC 和 gRPC 地址，默认分别为
`127.0.0.1:26657`、`127.0.0.1:9090`。安装前会调用 RPC `/status`，确认返回的
chain ID 是 `congrid-main`，并检查 gRPC TCP 端口可连接。RPC 和 gRPC 只需对本机
组件可访问，不需要暴露到公网。

组件模式只安装并管理 `chromad`、`indexerd` 和 `verifierd`。它不会覆盖
`/usr/local/bin/content-grid-d`，不会修改节点 home、genesis、seeds 或 CometBFT
配置，也不会创建、覆盖、停止、启动或重启 `congrid-node.service` /
`net.congrid.node`。安装包中的当前节点二进制会被单独安装成交易客户端：

```text
# Linux
/usr/local/libexec/congrid/content-grid-d-client
/var/lib/congrid-components
/etc/congrid-components

# macOS
~/.local/share/congrid/libexec/content-grid-d-client
~/.content-grid-components
~/.config/congrid-components
```

这些隔离目录用于 verifier keyring、pending state、Chroma 数据和交易客户端配置，
不会递归改动已有节点目录。启动 indexer/verifier 前仍会等待现有节点完成同步；
节点超过 60 分钟仍未同步完成时，已安装的组件会保留，可在节点同步后重新启动服务。

Linux 会通过 apt/dnf/yum/zypper/pacman 补齐依赖。macOS 自带 Python 3 且支持 venv
时直接使用；否则需要先安装 [Homebrew](https://brew.sh)，脚本会调用
`brew install python`。安装器会验证虚拟环境内的 pip；如果检测到已有
`chromad/.venv` 缺少 pip，会先尝试修复，必要时只重建该虚拟环境。
安装器 `1.4.1+` 会主动补齐 Linux 非登录 shell 经常缺少的 `/usr/sbin`、`/sbin`
等 PATH，因此即使 `groupadd` 已安装在 sbin 目录，通过 `curl | bash` 运行也能正确
找到它。

脚本会直接从 `/dev/tty` 读取回答，因此即使脚本内容通过管道送给 Bash，交互仍然
有效。首次安装会询问：

- 节点 moniker；
- 可选 persistent peers 和公开 P2P 地址；
- P2P 地址簿是否拒绝私网地址；
- `indexerd` 和 `verifierd` 的监听地址；
- verifier key 名称和 keyring 口令；
- 创建新 verifier key，或通过 mnemonic 恢复现有 key；
- verifier 交易 gas prices。

全节点安装固定使用 `congrid-main`，并校验主网 genesis 的 SHA-256。
新节点通过升级后的快照进行状态同步；等同步完成后才启动 indexer 和 verifier。
组件模式仍可通过 `CONGRID_CHAIN_ID` 选择要连接的链。

drand delivery 默认启用且不再询问。多个 verifier 可以安全地同时启用，程序会按
确定性顺序选择主投递者和后备投递者。只有已经确认网络中存在其他投递节点的高级
运维场景，才应设置 `CONGRID_DRAND_DELIVERY_DISABLED=true` 关闭本实例的投递职责。

若创建新 key，安装器会把唯一的 mnemonic JSON 备份保存到执行安装的用户 home，
权限为 `0600`，并打印完整路径。必须在给该地址充值前把它转移到安全的离线存储。
随后交互安装会显示新的 verifier 地址并暂停，提醒用户转入足够的 `ucongrid` 以支付
verifier bond 和后续交易费；转账后按 Enter 继续，也可以输入 `skip` 稍后处理。
安装器不会把“按 Enter”当作链上余额校验，也不会自动提交 bond。恢复已有 key 或
重复安装不会出现这个暂停；非交互安装只打印提醒并继续执行。

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
/usr/local/libexec/congrid/congrid-state-sync.py
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

使用 `--components-only` 时，Linux 不安装或覆盖节点使用的
`/usr/local/bin/content-grid-d`，也不安装节点 systemd 单元，只安装后三个单元；
组件状态和配置分别位于 `/var/lib/congrid-components`、
`/etc/congrid-components`。macOS 同样不会创建或加载 `net.congrid.node`，并使用上节
列出的隔离目录。

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

如果 `congrid-chroma` 显示 `status=203/EXEC`，日志同时出现
`.venv/bin/python: Permission denied`，说明 `congrid` 服务用户无法遍历 root 创建的
虚拟环境或其父目录。安装器 `1.4.2+` 会修正 `/opt/congrid` 和虚拟环境的组权限，
并在生成 systemd 单元前以服务用户执行 Python/import 探针；直接重新运行安装器即可
修复已有环境。indexer 在这种情况下只是等待 Chroma 健康检查超时，不是链 RPC 或
gRPC 故障。

新版安装器会给首次链同步和 indexer/verifier 的启动前健康检查预留 60 分钟，
并按 node → Chroma → 等待追块完成 → indexer → verifier 的依赖顺序启动。任一
服务失败时会自动打印四个服务的状态和失败服务日志。

如果节点日志出现 `unexpected ./data/application.db detected`，不要删除
`/var/lib/congrid/data`。旧二进制会在 systemd 工作目录与 `--home` 相同时把正常
数据库误判为相对路径数据库；安装器 `1.2.3+` 会使用兼容工作目录，新构建的
`content-grid-d` 也已经修正这项路径判断。

安装器 `1.5.0` 不再执行旧版高度 1 自动修复，也不会调用 `unsafe-reset-all`。
旧安装遗留的错误状态需要单独备份、诊断；不要用安装 verifier 的流程迁移已有链数据库。

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
genesis。1.5.0 没有自动重置例外。未由此状态同步安装器管理的节点应使用
`--components-only` 安装组件，不能直接使用完整安装模式接管。

## 非交互安装

自动化环境可以使用：

```bash
curl -fsSL https://congrid.net/downloads/install.sh |
  CONGRID_NON_INTERACTIVE=true \
  CONGRID_MONIKER=node-01 \
  CONGRID_STATE_SYNC_RPC_SERVERS='https://rpc-a.example/rpc,https://rpc-b.example/rpc' \
  CONGRID_VERIFIER_KEY_NAME=verifier-key \
  CONGRID_VERIFIER_KEY_ACTION=recover \
  CONGRID_VERIFIER_MNEMONIC='word ...' \
  CONGRID_VERIFIER_KEYRING_PASSPHRASE='replace-with-a-secret' \
  bash
```

Linux 非交互模式需要 root，或已经配置免口令 sudo。macOS 非交互模式使用当前
用户。敏感变量可能被调用方的进程管理或 CI 日志记录；生产环境优先使用交互模式。

已有节点的组件模式可使用：

```bash
curl -fsSL https://congrid.net/downloads/install.sh |
  CONGRID_NON_INTERACTIVE=true \
  CONGRID_COMPONENTS_ONLY=true \
  CONGRID_NODE_RPC_ADDR=127.0.0.1:26657 \
  CONGRID_NODE_GRPC_ADDR=127.0.0.1:9090 \
  CONGRID_VERIFIER_KEY_NAME=verifier-key \
  CONGRID_VERIFIER_KEY_ACTION=recover \
  CONGRID_VERIFIER_MNEMONIC='word ...' \
  CONGRID_VERIFIER_KEYRING_PASSPHRASE='replace-with-a-secret' \
  bash
```

设置 `CONGRID_START_SERVICES=false` 或传入 `--no-start` 可以只安装、配置并 enable
服务，不立即启动。管道方式传参数的示例：

```bash
curl -fsSL https://congrid.net/downloads/install.sh | bash -s -- --no-start
```

需要使用安装包镜像时设置 `CONGRID_RELEASE_BASE_URL`；或使用 `CONGRID_BUNDLE_URL`
和 `CONGRID_BUNDLE_SHA256` 指定并校验单个包。`CONGRID_DOWNLOAD_BASE_URL` 仅控制
genesis/seeds 网络文件地址，不再控制二进制包地址。默认 seed
地址会跟随 `CONGRID_DOWNLOAD_BASE_URL`；也可使用 `CONGRID_SEEDS_URL`、
`CONGRID_SEEDS_SHA256_URL` 或 `CONGRID_SEEDS_SHA256` 单独覆盖。若自动化部署已经
直接提供完整 seed 列表，可以设置 `CONGRID_P2P_SEEDS` 跳过 seed 文件下载。
