# 注册表模块（x/registry）

内容网格链的最小网站注册和验证模块框架。

状态：仅限模块基础知识（起源/类型）。一旦链与 `runtime.App` 和存储连接起来，就会添加完整的守护者/存储、消息和 gRPC 服务。

## 概念
- 域名注册：记录 `{domain, owner, status}` 其中 `status ∈ {PENDING, VERIFIED, REVOKED}`。
- 规范化：域通过基本格式验证进行规范化（小写、修剪）存储。
- Genesis：支持通过 `genesis.json` 预加载网站。

## 后续步骤
- 使用 `cosmossdk.io/collections` 和 keeper 方法添加存储。
- 添加 `MsgRegisterWebsite` 和 `MsgApprove/VerifyWebsite` 消息。
- 公开 gRPC 查询以按域/所有者获取网站状态。

