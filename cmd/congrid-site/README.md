# congrid-site

A small Go web server for the Congrid (Content Grid Protocol) official website.

## Languages

All pages support English, Chinese, and French. Use the `EN / 中文 / FR` buttons in the navigation. The server renders the selected language and remembers it in the `congrid_language` cookie, including subsequent form submissions. Use `?lang=en`, `?lang=zh`, or `?lang=fr` to link directly to a language; switching preserves other query parameters. English is the default.

Translations for page content, metadata, status labels, and browser notices are maintained in `static/translations.json`. Wallet connections use Keplr.

## Validation

```bash
go test ./cmd/congrid-site
node --test cmd/congrid-site/testdata/wallet.test.mjs
```

## Run locally

```bash
go run ./cmd/congrid-site --addr :8080 --base-url http://localhost:8080
```

### Legacy marketplace configuration (wallet signing)

> Slot marketplace 已下线（deprecated），仅保留文档备查

Slots and leases are read directly from the chain. Slot creation, status updates, and lease booking
are signed by the user wallet in the browser (Keplr).

```bash
go run ./cmd/congrid-site \
  --addr :8080 \
  --base-url http://localhost:8080 \
  --downloads-dir ./cmd/congrid-site/downloads \
  --slots-store chain \
  --chain-id <chain-id> \
  --node <rpc-url> \
  --slots-grpc <grpc-host:port>
```

Optional slot defaults: `--slot-rate-denom`, `--slot-unit-seconds`, `--slot-min-duration-seconds`, `--slot-max-duration-seconds`. Use `--gas-prices` to set the wallet gas price (default `0.001ucongrid`).

Server-side registration and airdrop transactions invoke `content-grid-d`. By default the site looks up `content-grid-d` from `PATH`; for production, install it as `/usr/local/bin/content-grid-d` or set `--content-grid-bin /path/to/content-grid-d` / `CONTENT_GRID_BIN`.

Open: <http://localhost:8080>

## Airdrop service

The airdrop endpoint performs a direct server-side homepage check. It does not
wait for the on-chain verifier assignment/commit/reveal flow. After a successful
check, the site atomically reserves the website in SQL, queues one bank transfer
from the configured faucet key, and confirms the transaction in the background.

Website uniqueness intentionally follows `registry.GetPrimaryDomain`: the last
two DNS labels are the key. For example, `www.example.com` and `api.example.com`
share `example.com`, while `example.co.uk` retains the existing simplified key
`co.uk`. There is no unique-wallet rule, so one wallet may receive airdrops for
multiple distinct website keys.

SQLite is the default:

```bash
export CONGRID_FAUCET_KEYRING_PASSPHRASE='<file-keyring-passphrase>'

go run ./cmd/congrid-site \
  --airdrop \
  --airdrop-db ./congrid-airdrop.db \
  --chain-id <chain-id> \
  --node <rpc-url> \
  --slots-grpc <grpc-host:port> \
  --keyring-backend file \
  --keyring-dir <keyring-dir> \
  --keyring-passphrase-env CONGRID_FAUCET_KEYRING_PASSPHRASE \
  --faucet-key faucet \
  --gas-prices 0.001ucongrid
```

If the SQLite path still contains the former JSON claim map, startup validates
and imports it, retaining the original as `<path>.json.bak`. Invalid legacy JSON
causes startup to fail instead of silently treating the claim set as empty.

PostgreSQL can be selected for a shared deployment:

```bash
export CONGRID_AIRDROP_DB_DRIVER=postgres
export CONGRID_AIRDROP_DB_DSN='postgres://user:pass@db/airdrop?sslmode=require'
```

Only one process per faucet key may run the transfer worker. In a multi-instance
deployment, use `--airdrop-worker=true` on one instance and
`--airdrop-worker=false` on the others. All instances may serve and reserve
claims through the shared PostgreSQL database.

Claims progress through `verified`, `submitting`, `broadcast`, and `confirmed`.
A rejected transaction becomes `failed`. A process interruption, ambiguous CLI
result, or confirmation timeout becomes `needs_reconcile`; these rows remain
reserved and are never resent automatically. Operators should compare the stored
transaction hash/note and recipient balance with chain history before manually
changing such a row.

Production verification is HTTPS-only, restricts redirects to the original host
(with a `www` alias exception), and rejects private, loopback, link-local, and
special-use resolved addresses. The `--airdrop-allow-http-verification`,
`--airdrop-allow-private-targets`, and
`--airdrop-allow-insecure-test-keyring` flags are development escape hatches and
must not be enabled in production. Use a dedicated low-balance faucet key and
enforce per-IP rate limits/CAPTCHA at the trusted reverse proxy.

## Release downloads

The site serves release artifacts at `/downloads/{filename}`. The default
filesystem directory is `cmd/congrid-site/downloads`; override it with
`--downloads-dir` or `CONGRID_SITE_DOWNLOADS_DIR` for a persistent production
directory.

```bash
cp content-grid-d-linux-amd64.tar.gz cmd/congrid-site/downloads/
chmod 0644 cmd/congrid-site/downloads/content-grid-d-linux-amd64.tar.gz
curl -fI http://localhost:8080/downloads/content-grid-d-linux-amd64.tar.gz
```

The public production URL is:

```text
https://congrid.net/downloads/content-grid-d-linux-amd64.tar.gz
```

The complete native operator stack also has an interactive Linux/macOS
installer that does not use containers:

```bash
curl -fsSL https://congrid.net/downloads/install.sh | bash
```

The installer downloads the versioned network bootstrap list from
`/downloads/seeds.txt` and verifies it against `/downloads/seeds.txt.sha256`.

The release procedure for the installer and its amd64/arm64 native bundles is
documented in `docs/native-operator-install-zh.md`.

Files are read on each request, so adding a file does not require rebuilding or
restarting the site. Release archives are gitignored and must be copied by the
deployment process. Directory listings are disabled; only top-level regular
files are served. Hidden files, subdirectories, symlinks, and unsafe filenames
return 404.

## Why Go?

This site is intentionally served by Go so we can add first-party analytics, attribution, and on-chain/off-chain integrations (e.g. helpers for verifying a publisher's member badge) without rewriting the stack.

## Routes

- `/` — home
- `/publishers` — publisher registration with wallet connection or manual address entry, member badge embed code, a CLI command, and an optional server-side registration button
- `/verifiers` — verifier onboarding
- `/docs` — pointers to repository docs
- `/airdrop` — verify the member badge on a website's homepage and send a starter airdrop; each primary domain can claim once when the feature is enabled
- `/badge.svg` — Congrid member badge image, using the official SVG logo and retaining query parameters for attribution
- `/badge.png` — legacy alias that serves the same SVG content for existing snippets
- `/static/*` — CSS + assets
- `/downloads/{filename}` — release artifact download with HEAD/Range support and no directory listing

### Publisher registration from web UI

`/publishers` supports direct wallet signing for registration (no local CLI required when wallet is connected).

- User fills `domain` + `wallet` (wallet from Keplr or manual paste).
- User clicks `Register with connected wallet` and approves tx in wallet.
- Frontend broadcasts `MsgRegisterPublisher` directly to chain.

CLI registration remains available as a fallback path.
