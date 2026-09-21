# congrid.net 的 Cloudflare CDN

## 配置

域名继续在 Namecheap 注册，DNS 和网站 CDN 使用 Cloudflare **Free** 套餐。
Cloudflare 账号为 `congridcoin@gmail.com`。

| 名称 | 类型 | 目标 | Cloudflare 代理 |
| --- | --- | --- | --- |
| `congrid.net` | A | `34.71.169.8` | Proxied |
| `www.congrid.net` | A | `34.71.169.8` | Proxied |
| `node1.congrid.net` | A | `168.138.73.149` | DNS only |

Namecheap 的 Custom DNS：

```text
burt.ns.cloudflare.com
maria.ns.cloudflare.com
```

切换前核对过 Namecheap 后台：没有域名重定向、邮件转发地址或 catch-all。
自动导入的 5 条默认 MX 和 1 条 SPF TXT 记录保留在 Cloudflare。
这些默认记录不表示已经提供可用邮箱；今后需要域名邮箱时，应配置实际邮件服务。

SSL/TLS 使用 **Full (strict)**，源站保留 Let's Encrypt 证书和 Certbot 续期。
边缘连接最低 TLS 版本为 1.2，TLS 1.3 开启。
WebSockets 保持开启，`/rpc/websocket` 的 Nginx Upgrade 转发保持有效。

## 缓存规则

两条规则仅匹配 `congrid.net` 和 `www.congrid.net`：

1. **Congrid static assets**：`/static/` 下的 GET/HEAD 请求可以缓存，包含 `.mjs`。
   Edge TTL 使用源站 Cache-Control，没有该响应头时不缓存；Browser TTL 使用源站 TTL。
   保留默认缓存键和查询参数。
2. **Congrid live pages and APIs**：路径不以 `/static/` 开头时绕过缓存。
   包括 HTML、空投、publisher、marketplace、RPC、REST、下载和 badge 请求。

源站 `/etc/nginx/sites-available/congrid.net` 为 `/static/` 单独设置：

```nginx
location ^~ /static/ {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    expires 5m;
    gzip on;
    gzip_vary on;
    gzip_proxied any;
    gzip_comp_level 5;
    gzip_types text/css text/javascript application/javascript image/svg+xml;
}
```

静态文件 URL 尚未包含内容指纹，因此缓存时间为 300 秒。部署后，已有浏览器或
CDN 缓存可能继续使用旧静态文件至多约 5 分钟；需要立即更新时，应使用新版本 URL，
并按需清除 Cloudflare 的对应 URL 缓存。Cloudflare purge 不会清除浏览器缓存。
不要为整个网站开启 Cache Everything，以免缓存链上状态和用户页面。

## 验证和维护

```bash
dig @1.1.1.1 congrid.net NS +short
curl -sSI https://congrid.net/
curl -sSI https://congrid.net/static/wallet-deps.bundle.mjs
curl -fsS https://congrid.net/rpc/status
curl -fsS https://congrid.net/rest/cosmos/base/tendermint/v1beta1/node_info
```

实际接入应同时满足：Cloudflare 域名状态 Active、Universal SSL 证书 Active，
HTTP 响应含 `cf-ray`，同一静态资源的重复请求出现 `CF-Cache-Status: HIT`。
RPC 的链 ID 应为 `congrid-main`，区块高度持续增长，WebSocket 状态请求成功。
DNS 缓存未更新的访问者可能暂时继续直连旧源站。

2026-09-05 UTC 的实际验证：Cloudflare Universal SSL 状态 Active；1.1.1.1 和
8.8.8.8 均返回 Cloudflare 解析；通过该解析访问首页、空投、publishers、RPC、REST
均为 HTTP 200 且带 `cf-ray`。CSS、JS、MJS 的第二次 GET 均为 `HIT`，缓存时间为
300 秒，动态路径为 `DYNAMIC`。通过 Cloudflare 的 WSS 握手返回 101，状态请求
返回 `congrid-main`、区块高度 68003、`catching_up=false`。

修改源站配置后，先运行 `sudo nginx -t`，再执行 `sudo systemctl reload nginx`。
2026-09-05 UTC 的源站原始配置备份位于：

```text
/var/backups/congrid-nginx/congrid.net.cdn.20260905T003215Z.bak
```

若仅需临时绕过 CDN，可在 Cloudflare 将主站和 `www` 改为 DNS only；保持 `node1`
为 DNS only。源站有效的公网 HTTPS 证书允许网站继续直接提供服务。
若回退整个 DNS 托管，应先核对 Namecheap 原始记录完整，再切回原来的
`dns1.registrar-servers.com` 和 `dns2.registrar-servers.com`。
