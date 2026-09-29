# 公共状态同步 RPC（2026-09-28）

安装器 1.5.1 内置以下两个地址，不再要求用户输入：

| RPC | 后端节点 | 节点 ID |
| --- | --- | --- |
| `https://congrid.net/rpc-val2` | val2 / `168.138.73.149` | `9d1390f37736424f2c71b88174a8ac0a7810e4cd` |
| `https://congrid.net/rpc` | val3 / `34.71.169.8` | `72165a18f87c580ca92d1db1bb7b1a907612393d` |

两台节点用于交叉核对信任区块；同一节点的两个域名不能替代两个节点。
可信高度和哈希仍在安装及首次启动时动态核验，不写死。
环境变量 `CONGRID_STATE_SYNC_RPC_SERVERS` 可以覆盖默认值；重装保留已有非空配置。
空的环境变量或保存值使用内置默认值。

## 入口与维护

复用 congrid.net 的 DNS、Cloudflare 和 HTTPS 证书，无需新建子域名。
两个地址共用官网入口，因此并不提供网关故障隔离，也不代表两个独立运营方。
Cloudflare 现有非静态路径绕过缓存规则覆盖 `/rpc-val2`；Nginx 同时返回 `no-store`。

val2 的 RPC 仍只监听 `127.0.0.1:26657`，没有开放公网 RPC 端口。
官网服务器运行 `congrid-rpc-val2.service`，通过 SSH 将本机 `127.0.0.1:28657`
转发到 val2 的 `127.0.0.1:26657`。systemd 开机启动、断线重连。

仓库配置：

- `deploy/state-sync/congrid-rpc-val2.service` 安装到官网服务器 `/etc/systemd/system/`。
- `deploy/state-sync/rpc-val2.nginx.conf` 安装到 `/etc/nginx/snippets/congrid-rpc-val2.conf`，
  在 congrid.net 的 HTTPS server 内 include。
- `/etc/congrid-rpc-val2/id_ed25519` 为官网服务器现场生成的专用私钥，
  所有者 `root:congrid-rpc`、权限 `0640`；目录权限 `0750`。不要提交或复制进仓库。
- 同目录 `known_hosts` 固定 val2 的主机公钥，使用已有可信 SSH 连接获取并核对。
- val2 的 opc `authorized_keys` 对专用公钥使用以下选项：
  `from="34.71.169.8",restrict,port-forwarding,permitopen="127.0.0.1:26657",command="/bin/false"`。
  禁止交互命令，限制来源及本地转发目的地。远端旧版 OpenSSH 不支持 `permitlisten`。

检查服务及代理：

```bash
sudo systemctl status congrid-rpc-val2
sudo journalctl -u congrid-rpc-val2 -n 30
curl -fsS http://127.0.0.1:28657/status
sudo nginx -t
curl -fsS https://congrid.net/rpc-val2/status
curl -fsS -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"status","params":{}}' \
  https://congrid.net/rpc-val2
```

代理支持 URL 路径式 GET 和 CometBFT 使用的 JSON-RPC POST。
已核对两个公共入口的 POST `commit`、`validators` 在上述高度返回一致结果。
变更 Nginx 配置后先 `nginx -t` 再 reload，不需要重启链节点。
本次原配置备份为 `/var/backups/congrid-nginx/congrid.net.before-rpc-val2-20260928`。
回退时先恢复安装器或提供替代 RPC，再恢复 Nginx 配置、reload，最后停用隧道服务；
移除 val2 上标记为 `congrid-rpc-val2` 的专用公钥，不覆盖其它 authorized_keys 项。

## 验证范围

2026-09-28 使用安装器原有 `trust_anchor` 完成公共地址预检：两台节点均为
`congrid-main`、非追块状态，节点 ID 不同；高度 `125695` 的区块哈希一致：

```text
8A32035DB7A9F16327DE015BD4936D47B7D1A2CEFE23F3D603D1779564266797
```

可信块时间、链上解绑期检查通过，两个公网 P2P 端口均可连接。
val2 的快照间隔为 10，保留最近 10 份。此次未重新执行完整节点快照恢复，
P2P 端口可连接不能代替快照恢复验证；此前恢复记录见
[2026-09-21 验证](ibc-installer-validation-20260921.md)。
val1 本次仍停在高度 90000，未选为默认 RPC。

安装器的 22 项 Python 测试、Bash 语法检查通过，覆盖无交互默认选择、保存值和环境变量覆盖。

官网 `/var/lib/congrid-site/downloads/install.sh` 已更新为 1.5.1，并核对公开下载内容
与仓库文件逐字节一致。旧版保存在官网服务器
`/var/backups/congrid-site/install.sh.before-1.5.1-20260928`。
