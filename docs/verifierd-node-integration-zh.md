# verifierd 与 content-grid-d 合并评估

## 结论

不建议把 verifierd 作为 goroutine 直接嵌入 `content-grid-d start` 的共识节点进程。
推荐的目标形态是：**同一个发布二进制、两个独立进程**，例如：

```bash
content-grid-d start --home /var/lib/congrid
content-grid-d verifier start --config /etc/congrid/verifierd.json
```

这样可以减少二进制构建、下载和版本匹配成本，同时保留故障、密钥、资源和重启边界。
当前 `drand-strict-v2` 升级继续发布独立 `verifierd`，避免在紧急共识升级中同时引入
大规模进程重构；同二进制子命令可以作为后续兼容性发布实现，不需要新的链上升级。

## 为什么不放进同一个节点进程

- verifierd 访问 publisher、indexerd 和 drand HTTP，属于非确定性链下 I/O；不能进入
  ABCI/共识执行路径。
- verifierd 的 HTTP、keyring 或任务 panic 不应让共识节点退出。
- 共识验证人密钥与 verifier 交易密钥应使用不同 keyring 和最小权限。
- verifier 数量和共识 validator 数量没有一一对应关系；非 validator 也应能独立运行
  verifier。
- verifierd 的升级、重启、限流和资源峰值不应影响节点出块。

## 方案比较

| 方案 | 维护成本 | 共识隔离 | 建议 |
| --- | --- | --- | --- |
| 独立 `content-grid-d` + `verifierd` 二进制 | 当前水平 | 强 | 本次升级继续使用 |
| 同一 `content-grid-d` 二进制，不同子命令/进程 | 较低 | 强 | 后续目标 |
| `content-grid-d start --with-verifier` 同进程 | 表面最低 | 弱 | 不采用 |

## 后续实现边界

1. 把 `offchain/verifierd` 的 agent/config/health 代码移到可导入包。
2. 保留一个很薄的 `verifierd` 兼容入口，避免现有 systemd/Docker 配置立即失效。
3. 为 `content-grid-d verifier start` 注册同一 runner。
4. 两个进程继续使用不同 PID、日志、健康端口、keyring 和资源限制。
5. 完成一个发布周期后再决定是否停止发布兼容 `verifierd` 文件。

合并发布物不能改变链上 `MsgSubmitDrandBeacon`、commit/reveal 或奖励语义，因此不需要
修改 upgrade handler，也不应改变现有 node home、共识数据库或 validator 密钥。
