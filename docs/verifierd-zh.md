# verifierd (off-chain) — 链驱动的发布者验证

`verifierd` 是一个链下代理，它**轮询链上的分配**，等待预定的开始时间，验证内容，并通过提交-显示在链上提交结果。

## 它的作用

- **使用注册表查询 `VerifierAssignments` 对其验证者地址进行轮询分配**。
  - 新的发布商注册将排队等待**下一轮**（在本轮中不会立即验证）。
- **在提交交易之前在本地验证分配确定性**（轮种子 + 域 => 预期开始时间）。
  - 种子源由链元数据（`chain_id`、`round_start`、`anchor_height`、`anchor_hash`）锚定。
  - `RoundMeta` 还公开 `verifier_set_hash` / `verifier_set_size` 以实现分配输入的可审核性。
- **等待每个分配的 startAt**（分配时间表完全在链上确定）。
- **验证主页**并标记**通过**，如果：
  - 页面可访问（HTTP 2xx/3xx），并且
  - 主页包含 **Congrid 验证徽章**：
    - 没有查询/片段的 `<a href="https://congrid.net">` （或 `https://www.congrid.net/`）
    - `<a>` 包装了 `<img>`
    - `<img src>` 由 `https://congrid.net/...` 提供，包括 `publisher=<domain>` 和 `wallet=<owner>`
  - 如果有有效租约，主页还包含每个有效租约的租约锚：
    - __代码_0__
    - `href` 必须匹配租约 `target_url` （主机 + 路径）
- **通过 `content-grid-d verifier commit ...` 提交提交**。
- **等待显示窗口**，然后通过 `content-grid-d verifier reveal ...` **显示**。

> 分配节奏由链上参数（`round_interval_seconds`、`assignment_delay_max_seconds`、`submission_window_seconds`、`commit_window_seconds`）控制。
> - 对于每小时轮次 (`round_interval_seconds >= 3600`)，分配开始时间在轮次内使用确定性**分钟时段 (0–59)**。
> - 对于短/更快的回合 (dev/e2e)，开始时间使用受 `assignment_delay_max_seconds` 约束的确定性秒偏移量。

## 配置

复制并编辑：

```bash
cp offchain/verifierd/config.example.json offchain/verifierd/config.json
```

领域：
- `grpc_addr`：链 gRPC 端点（默认 `127.0.0.1:9090`）
- `verifier_address`：验证器 bech32 (`congrid1...`)
- `poll_interval_seconds`：分配轮询间隔
- `verify_scheme`：`https`（默认）或 `http` 对于本地开发
- `commit_window_seconds`：verifierd调度使用的本地提交窗口（必须与链上参数对齐）
- `round_interval_seconds`：确定性分配验证的预期轮次间隔（默认 `3600`）
- `assignment_delay_max_seconds`：确定性验证中使用的预期分配延迟上限（默认 `round_interval_seconds`）
- `disable_assignment_check`：设置 `true` 以绕过 verifierd 的本地确定性分配验证（默认 `false`）
- `submit`：`content-grid-d` 的 tx 提交设置
  - __代码_0__
  - __代码_0__、__代码_1__、__代码_2__、__代码_3__、__代码_4__、__代码_5__
  - __代码_0__、__代码_1__、__代码_2__、__代码_3__、__代码_4__、__代码_5__

＃＃ 跑步

立即进行一项民意调查（并等待任何开始的分配工作人员）：

```bash
go run ./offchain/verifierd --config offchain/verifierd/config.json --once
```

长期代理：

```bash
go run ./offchain/verifierd --config offchain/verifierd/config.json
```
