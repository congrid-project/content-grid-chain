# Verifiers (bond, assignment, reward)

Verifiers are normal account addresses (`congrid1...`) that bond into escrow.

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

## Assignment Rule (Current)

Assignments are deterministic and stake-weighted:

- Candidate set: active, non-suspended verifiers that satisfy min bond.
- Selection: deterministic weighted random sampling **without replacement**.
- Weight: verifier bonded stake (`bond.amount`).

This means higher bonded stake increases assignment probability, while still keeping selection auditable from on-chain round seed + verifier set.

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

Assignment-level verifier payout is split into two buckets:

1. **Base bucket** (default `verifier_reward_base_share_bps = 4000`, i.e. 40%)
   - equally split among successful verifiers for that assignment.
2. **Weighted bucket** (remaining 60%)
   - proportional to

`weight = bonded_stake × referral_factor`

- `bonded_stake`: verifier bond in verifier module
- `referral_factor`: active referred-publisher factor (minimum 1)

If no positive weighted verifier exists for an assignment, the weighted bucket is burned from the emission pool.

## Penalties

- Missed submission or voting against majority increases penalty count.
- Repeated penalties can trigger temporary assignment suspension.

## Publisher-side gating that affects burn

Publisher share is evenly split by round assignments, but actual publisher claim is gated by required external links threshold (`required_external_links_for_full_reward`).
Unclaimed publisher amount is burned from pool.
