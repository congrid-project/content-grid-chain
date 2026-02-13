# Verifiers (bond, assignment, reward)

Verifiers are normal account addresses (`grid1...`) that bond into escrow.

## Bond / Unbond

Bond:
```bash
./content-grid-d verifier bond 1000000 --denom ucongrid --from <key>
```

Unbond:
```bash
./content-grid-d verifier unbond 500000 --denom ucongrid --from <key>
```

List assignments:
```bash
./content-grid-d verifier assignments --from <key>
```

Commit / Reveal:
```bash
./content-grid-d verifier commit example.com --passed --nonce <nonce> --from <key>
./content-grid-d verifier reveal example.com --passed --nonce <nonce> --from <key>
```

## Finalization Rules (Current)

- Round result is majority + quorum based.
- Publisher status transitions:
  - pass-majority with quorum: `PENDING/VERIFIED -> VERIFIED`
  - fail-majority with quorum: `VERIFIED -> PENDING`, and after threshold failures can become `REVOKED`

## Verifier Reward Rule (Current)

Only successful verifiers in finalized pass-majority rounds participate in verifier payout.

Weight per verifier:

`weight = bonded_stake × referral_factor`

- `bonded_stake`: verifier bond in verifier module
- `referral_factor`: active referred-publisher factor (minimum 1)

So payout is proportional to stake and referral activity.

## Penalties

- Missed submission or voting against majority increases penalty count.
- Repeated penalties can trigger temporary assignment suspension.

## Publisher-side gating that affects burn

Publisher share is evenly split by round assignments, but actual publisher claim is gated by required external links threshold (`required_external_links_for_full_reward`).
Unclaimed publisher amount is burned from pool.
