# 代币经济学助手脚本

用于准备创世分配、运行经济模拟和具体化空投数据的实用程序。

## 快速入门

```
go run ./cmd/tokenomics simulate --years 5 --bonded 0.6
```

- 传递给 `simulate`、`genesis-template`、`airdrop` 的值位于 CONGRID 中（CLI 转换为 `ucongrid`）。
- 将 `--json` 添加到 `simulate` 以获得机器可读的输出。

### 创世模板

```
go run ./cmd/tokenomics genesis-template \
  --foundation grid1... \
  --team grid1... \
  --verifiers grid1...
```

使用提供的地址替换每个分配桶写入经济默认值。

### 空投生成器

创建一个带有标题的 CSV `recipients.csv`：

```
address,weight
grid1exampleaddressaaa,1
grid1exampleaddressbbb,2
```

然后运行：

```
go run ./cmd/tokenomics airdrop --input recipients.csv --supply 25000000 --pretty=false > airdrop.json
```

最终的条目吸收了舍入的灰尘，因此总分配与请求的供应完全匹配。
