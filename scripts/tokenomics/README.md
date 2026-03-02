# Tokenomics Helper Scripts

Utilities for preparing genesis allocations, running economic simulations, and materialising airdrop data.

## Quick Start

```
go run ./cmd/tokenomics simulate --years 5 --bonded 0.6
```

- Values passed to `simulate`, `genesis-template`, `airdrop` are in CONGRID (the CLI converts to `ucongrid`).
- Add `--json` to `simulate` for machine-readable output.

### Genesis Template

```
go run ./cmd/tokenomics genesis-template \
  --foundation congrid1... \
  --team congrid1... \
  --verifiers congrid1...
```

Writes the economic defaults with provided addresses substituted for each allocation bucket.

### Airdrop Builder

Create a CSV `recipients.csv` with header:

```
address,weight
congrid1exampleaddressaaa,1
congrid1exampleaddressbbb,2
```

Then run:

```
go run ./cmd/tokenomics airdrop --input recipients.csv --supply 25000000 --pretty=false > airdrop.json
```

The final entry absorbs rounding dust so total allocation matches the requested supply exactly.
