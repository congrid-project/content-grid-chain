# verifierd (off-chain) — chain-driven publisher verification

`verifierd` is an off-chain agent that **polls the chain for assignments**, waits until scheduled start time, verifies content, and submits results on-chain via commit–reveal.

## What it does

- **Polls assignments** using registry query `VerifierAssignments` for its verifier address.
  - New publisher registrations are queued for the **next round** (not verified immediately in the current round).
- **Validates assignment determinism locally** (round seed + domain => expected start time) before submitting tx.
  - Seed source is anchored by chain metadata (`chain_id`, `round_start`, `anchor_height`, `anchor_hash`).
  - `RoundMeta` also exposes `verifier_set_hash` / `verifier_set_size` for auditability of assignment inputs.
- **Waits until startAt** for each assignment (assignment schedule is fully determined on-chain).
- **Verifies homepage** and marks **pass** if:
  - page is reachable (HTTP 2xx/3xx), and
  - homepage contains a **Congrid verification badge**:
    - `<a href="https://congrid.net">` (or `https://www.congrid.net/`) with no query/fragment
    - the `<a>` wraps an `<img>`
    - `<img src>` is served from `https://congrid.net/...` and includes `publisher=<domain>` and `wallet=<owner>`
  - if there are active leases, homepage also contains lease anchors for each active lease:
    - `<a href="https://advertiser.example/landing" data-congrid-slot="slot-000123" data-congrid-lease="lease-000456">`
    - `href` must match lease `target_url` (host + path)
- **Submits commit** via `content-grid-d verifier commit ...`.
- **Waits for reveal window**, then **reveals** via `content-grid-d verifier reveal ...`.

> Assignment cadence is controlled by on-chain params (`round_interval_seconds`, `assignment_delay_max_seconds`, `submission_window_seconds`, `commit_window_seconds`).
> - With hourly rounds (`round_interval_seconds >= 3600`), assignment start times use deterministic **minute slots (0–59)** inside the round.
> - With short/faster rounds (dev/e2e), start times use deterministic second offsets constrained by `assignment_delay_max_seconds`.

## Config

Copy and edit:

```bash
cp offchain/verifierd/config.example.json offchain/verifierd/config.json
```

Fields:
- `grpc_addr`: chain gRPC endpoint (default `127.0.0.1:9090`)
- `verifier_address`: verifier bech32 (`grid1...`)
- `poll_interval_seconds`: assignment poll interval
- `verify_scheme`: `https` (default) or `http` for local dev
- `commit_window_seconds`: local commit window used by verifierd scheduling (must be aligned with on-chain params)
- `round_interval_seconds`: expected round interval for deterministic assignment validation (default `3600`)
- `assignment_delay_max_seconds`: expected assignment delay cap used in deterministic validation (default `round_interval_seconds`)
- `disable_assignment_check`: set `true` to bypass verifierd's local deterministic assignment validation (default `false`)
- `submit`: tx submission settings for `content-grid-d`
  - `binary`
  - `chain_id`, `node`, `from`, `keyring_backend`, `keyring_dir`, `home`
  - `gas`, `gas_adjustment`, `fees`, `gas_prices`, `broadcast_mode`, `yes`

## Run

One immediate poll (and wait for any started assignment workers):

```bash
go run ./offchain/verifierd --config offchain/verifierd/config.json --once
```

Long-running agent:

```bash
go run ./offchain/verifierd --config offchain/verifierd/config.json
```
