# congrid-site

A small Go web server for the Congrid (Content Grid Protocol) official website.

## Run locally

```bash
go run ./cmd/congrid-site --addr :8080 --base-url http://localhost:8080
```

### Chain-backed slot marketplace

To use on-chain slots/leases instead of the in-memory demo store:

```bash
go run ./cmd/congrid-site \
  --addr :8080 \
  --base-url http://localhost:8080 \
  --slots-store chain \
  --chain-id <chain-id> \
  --node <rpc-url> \
  --slots-grpc <grpc-host:port> \
  --slots-key <publisher-key-name> \
  --keyring-backend <backend>
```

Optional slot tx flags: `--slots-home`, `--slot-fees`, `--slot-gas`, `--slot-gas-prices`, `--slot-gas-adjustment`, `--slot-rate-denom`, `--slot-unit-seconds`, `--slot-min-duration-seconds`, `--slot-max-duration-seconds`, `--lease-key`.

Open: <http://localhost:8080>

## Why Go?

This site is intentionally served by Go so we can add first-party analytics, attribution, and on-chain/off-chain integrations (e.g. publisher badge validation helpers) without rewriting the stack.

## Routes

- `/` — home
- `/marketplace` — publisher slot marketplace
- `/publishers` — publisher onboarding (badge snippet + registration steps)
- `/publisher/dashboard` — manage publisher slots (create, pause, unlist)
- `/verifiers` — verifier onboarding
- `/docs` — pointers to repository docs
- `/airdrop` — verify homepage badge and send a one-time fee airdrop per primary domain (when enabled)
- `/badge.png` — embeddable verification badge (query params preserved for future attribution)
- `/static/*` — CSS + assets
