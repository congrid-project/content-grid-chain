# ibc-transfer-v1 安装器验证（2026-09-21）

安装器版本：1.5.0。节点源码：`5907b65faa971afd9e0c29e74284baa03675e37c`。
验证使用已构建的 macOS amd64 原生包、独立临时节点目录和新生成的节点密钥。
没有修改生产验证人的配置、二进制或数据库，没有广播交易。

## 实际状态同步与重启

使用 `scripts/installer/state_sync.py`（与 install.sh 中嵌入的代码相同）启动全新节点：

- 从 val2 恢复高度 **105770**、格式 **3** 的快照；日志出现 `Snapshot restored`。
- 追上高度 **105776**，生成首次同步完成标记。
- 在高度 **105775**，本地、val2、val3 三台节点的区块哈希及 AppHash 完全一致。
- 升级查询返回 `ibc-transfer-v1` 执行高度 **90000**；转账收发均为 `true`。
- 第二个启动器被进程锁拒绝。
- 正常停止节点后，将启动器设置文件中的两个引导 RPC 改为不可用的本地端口；
  重启仍可使用本地状态并推进到高度 **105777**。
- 验证后正常停止新增测试节点；原有手工测试节点继续运行。

核对值：

```text
height:     105775
block_hash: 9B8FFC9FE20AFBF883D161C84C08A635016F3FFB73B0E00E6DF01AA8A036FD30
app_hash:   AC005CFA10E4566178080F74BD4A6678477798F244DC7FF603709393C3C97317
```

本地原始报告：`tmp/installer-statesync-check/live-e67erphb/report.json`。
同目录保留节点日志。报告和运行数据不纳入 Git。

## 网络条件与验证范围

RPC 使用 val2 的本地 SSH 转发与 `https://congrid.net/rpc`。由于 Mac 上已有另一个
节点连接主网，第二个并行节点直连遭遇 P2P 拒绝；通过单独的 val2 P2P 隧道完成测试。
这不等于已部署第二个公共 RPC，也不代表普通用户应依赖维护者的 SSH。

发布前仍需公开完整 Release 的八个附件、部署官网安装器，并为新用户提供两个
可访问的 RPC 和快照提供节点。不能将尚未验证可用的 URL 写为公共默认值。

安装包下载、校验及无旧二进制目录布局已通过执行安装器对应 Shell 段落测试；
已有目录保护、哈希不一致、过期信任、短解绑期、重复启动等分支通过自动化测试。
没有在本次验证中执行真实 Linux systemd 全栈安装或 macOS Chroma/launchd 全栈安装。

```bash
python3 -m unittest discover -s scripts/installer/tests -v
bash -n cmd/congrid-site/downloads/install.sh
```
