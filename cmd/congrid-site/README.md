# congrid-site

A small Go web server for the Congrid (Content Grid Protocol) official website.

## Run locally

```bash
go run ./cmd/congrid-site --addr :8080 --base-url http://localhost:8080
```

### Chain-backed slot marketplace (wallet signing)

Slots and leases are read directly from the chain. Slot creation, status updates, and lease booking
are signed by the user wallet in the browser (Keplr/Leap).

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

Files are read on each request, so adding a file does not require rebuilding or
restarting the site. Release archives are gitignored and must be copied by the
deployment process. Directory listings are disabled; only top-level regular
files are served. Hidden files, subdirectories, symlinks, and unsafe filenames
return 404.

## Why Go?

This site is intentionally served by Go so we can add first-party analytics, attribution, and on-chain/off-chain integrations (e.g. publisher badge validation helpers) without rewriting the stack.

## Routes

- `/` — home
- `/marketplace` — publisher slot marketplace
- `/leases` — lease publish board (slot/lease IDs + embed snippets)
- `/publishers` — publisher onboarding (wallet connect OR manual address, generated badge snippet, CLI command, and optional server-side registration button)
- `/publisher/dashboard` — manage publisher slots (create, pause, unlist, publish lease snippets)
- `/verifiers` — verifier onboarding
- `/docs` — pointers to repository docs
- `/airdrop` — verify homepage badge and send an optional one-time starter airdrop per primary domain (when enabled)
- `/badge.png` — embeddable verification badge (query params preserved for future attribution)
- `/static/*` — CSS + assets
- `/downloads/{filename}` — release artifact download with HEAD/Range support and no directory listing

### Publisher registration from web UI

`/publishers` supports direct wallet signing for registration (no local CLI required when wallet is connected).

- User fills `domain` + `wallet` (wallet from Keplr/Leap or manual paste).
- User clicks `Register with connected wallet` and approves tx in wallet.
- Frontend broadcasts `MsgRegisterPublisher` directly to chain.

CLI registration remains available as a fallback path.
