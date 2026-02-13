# Marketplace (slots + leases)

This document describes the link-slot marketplace lifecycle and CLI usage.

## Slot lifecycle

- **Listed**: visible to advertisers for booking.
- **Paused**: hidden from booking but retained.
- **Unlisted**: hidden permanently; existing leases are unaffected.

## Lease lifecycle

- **Active**: lease is in effect (between start/end).
- **Completed**: lease finished and escrow released.
- **Violated**: lease refunded due to verification failure / cooldown.
- **Refunded**: lease refunded before completion.

## CLI (content-grid-d)

> `publisher` / `lessee` are inferred from `--from` signer. Do not pass legacy `--publisher`/`--lessee` flags.

Create a slot:

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

Update slot status:

```bash
./content-grid-d tx registry update-slot-status \
  --slot-id slot-000123 \
  --status SLOT_STATUS_LISTED \
  --from <publisher-key>
```

Lease a slot:

```bash
./content-grid-d tx registry lease-slot \
  --slot-id slot-000123 \
  --target-url https://advertiser.example/landing \
  --starts-at-unix 1735689600 \
  --duration-seconds 1209600 \
  --from <advertiser-key>
```

Useful queries:

```bash
./content-grid-d query registry slots --publisher <publisher-bech32>
./content-grid-d query registry leases --slot-id slot-000123
```

## Verification link requirements

When a lease is active, publishers must include anchors like:

```html
<a href="https://advertiser.example/landing" data-congrid-slot="slot-000123" data-congrid-lease="lease-000456">Link</a>
```

The verifier checks host + path match and the `data-congrid-*` attributes.
