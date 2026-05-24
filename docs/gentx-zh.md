# gentx 与最终 genesis 生成（无发布包场景）

> 适用：尚未有官方 release bundle（binary + genesis + peers）时，自行组织首发网络。

## 什么是 gentx

`gentx`（genesis transaction）是验证人在链启动前提交的“创世验证人声明”，包括：
- 验证人公钥
- 自抵押金额
- 佣金参数
- 最小自抵押

链在高度 0 启动时，会把收集到的 gentx 写入初始验证人集。

---

## 总流程

1. 协调机生成母 genesis
2. 收集所有验证人地址并 `add-genesis-account`
3. 分发同一份母 genesis 给每个验证人
4. 每个验证人本地生成 gentx
5. 协调机 `collect-gentxs` 生成 final genesis
6. 分发 final genesis（所有节点 hash 必须一致）

---

## 最小示例（2 个验证人）

### A. 协调机

```bash
export CHAIN_ID=congrid-main-1
export COORD_HOME=/tmp/congrid-genesis-coord

rm -rf "$COORD_HOME"
./content-grid-d init coordinator --chain-id "$CHAIN_ID" --home "$COORD_HOME"
```

### B. val1 / val2 各自机器

```bash
# val1 机器
./content-grid-d init val1 --chain-id "$CHAIN_ID" --home ~/.content-grid-d
./content-grid-d keys add val1 --home ~/.content-grid-d --keyring-backend file
./content-grid-d keys show val1 --address --home ~/.content-grid-d --keyring-backend file

# val2 机器
./content-grid-d init val2 --chain-id "$CHAIN_ID" --home ~/.content-grid-d
./content-grid-d keys add val2 --home ~/.content-grid-d --keyring-backend file
./content-grid-d keys show val2 --address --home ~/.content-grid-d --keyring-backend file
```

把两个 `congrid1...` 地址发给协调机。

### C. 协调机加入创世账户

```bash
./content-grid-d genesis add-genesis-account <val1_addr> 100000000ucongrid --home "$COORD_HOME"
./content-grid-d genesis add-genesis-account <val2_addr> 100000000ucongrid --home "$COORD_HOME"
```

### D. 分发母 genesis 并生成 gentx

把 `$COORD_HOME/config/genesis.json` 分发到 val1/val2，覆盖各自：
`~/.content-grid-d/config/genesis.json`

各节点生成 gentx：

```bash
# val1
./content-grid-d genesis gentx val1 1000000ucongrid \
  --chain-id "$CHAIN_ID" \
  --home ~/.content-grid-d \
  --keyring-backend file

# val2
./content-grid-d genesis gentx val2 1000000ucongrid \
  --chain-id "$CHAIN_ID" \
  --home ~/.content-grid-d \
  --keyring-backend file
```

将 `~/.content-grid-d/config/gentx/*.json` 回传给协调机。

### E. 协调机收集并生成 final genesis

```bash
mkdir -p "$COORD_HOME/config/gentx"
# 复制各节点 gentx 到上面目录

./content-grid-d genesis collect-gentxs --home "$COORD_HOME"
./content-grid-d genesis validate-genesis --home "$COORD_HOME"
sha256sum "$COORD_HOME/config/genesis.json"
```

### F. 分发 final genesis 并启动

每个节点：

```bash
cp /path/to/final-genesis.json ~/.content-grid-d/config/genesis.json
sha256sum ~/.content-grid-d/config/genesis.json
./content-grid-d start --home ~/.content-grid-d
```

---

## 双节点最小互联（seed + PEX）

先在作为 seed 的节点上拿 node id：

```bash
./content-grid-d tendermint show-node-id --home ~/.content-grid-d
```

设：
- val1 为稳定可访问的 seed：`<NODE1_ID>@<VAL1_IP>:26656`
- val2 为新加入节点

两台机器的 `~/.content-grid-d/config/config.toml` 的 `[p2p]` 段都保持 PEX 开启：

```toml
pex = true
```

val2 只需要在 `[p2p]` 段配置 seed，不需要让 val1 预先写入 val2：

```toml
seeds = "<NODE1_ID>@<VAL1_IP>:26656"
persistent_peers = ""
```

val1 不需要配置 val2。val2 启动后会先连接 val1，再通过 CometBFT PEX 和地址簿发现可连接 peer。如使用私网 IP 或本地地址，将对应节点的 `[p2p] addr_book_strict` 设为 `false`。如有防火墙，放通 seed 节点 TCP/26656。

---

## 常见坑

- 使用旧 `data/` 配新 genesis（会触发高度/状态冲突）
- 各节点使用不同 genesis（必分叉）
- gentx 不是基于同一份母 genesis 生成
- 忘记分配验证人地址初始余额，导致 gentx 金额不足
