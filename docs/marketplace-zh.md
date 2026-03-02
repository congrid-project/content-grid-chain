# 市场（slot+租赁）

本文档描述了链接槽市场生命周期和 CLI 用法。

## 插槽生命周期

- **列出**：广告商可以看到进行预订。
- **暂停**：对预订隐藏但保留。
- **不公开**：永久隐藏；现有租约不受影响。

## 租赁生命周期

- **有效**：租约有效（开始/结束之间）。
- **完成**：租赁完成并释放托管。
- **违反**：由于验证失败/冷却而退还租约。
- **退款**：租约在完工前退还。

## CLI（内容网格-d）

> `publisher` / `lessee` 是从 `--from` 签名者推断出来的。不要传递旧版 `--publisher`/`--lessee` 标志。

创建一个槽：

```bash
./content-grid-d tx registry create-slot \
  --domain example.com \
  --label "Homepage Hero" \
  --summary "Top banner" \
  --category "News" \
  --placement "Homepage" \
  --size "728x90" \
  --rate-denom ucongrid \
  --rate-amount 200 \
  --unit-seconds 604800 \
  --min-duration-seconds 604800 \
  --max-duration-seconds 7776000 \
  --tags "Editorial" --tags "Tech" \
  --from <publisher-key>
```

更新插槽状态：

```bash
./content-grid-d tx registry update-slot-status \
  --slot-id slot-000123 \
  --status SLOT_STATUS_LISTED \
  --from <publisher-key>
```

租用一个插槽：

```bash
./content-grid-d tx registry lease-slot \
  --slot-id slot-000123 \
  --target-url https://advertiser.example/landing \
  --starts-at-unix 1735689600 \
  --duration-seconds 1209600 \
  --from <advertiser-key>
```

有用的查询：

```bash
./content-grid-d query registry slots --publisher <publisher-bech32>
./content-grid-d query registry leases --slot-id slot-000123
```

## 验证链接要求

当租约有效时，发布商必须包含以下锚点：

```html
<a href="https://advertiser.example/landing" data-congrid-slot="slot-000123" data-congrid-lease="lease-000456">Link</a>
```

验证程序检查主机 + 路径匹配和 `data-congrid-*` 属性。
