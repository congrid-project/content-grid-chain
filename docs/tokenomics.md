# Content Grid Tokenomics

This document describes the **current on-chain behavior** in this repository.

## Supply & Allocation Baseline

- Unit: `ucongrid` (1 CONGRID = 1,000,000 `ucongrid`)
- Reference total supply parameter: **1,000,000,000 CONGRID** (`1_000_000_000_000000 ucongrid`)
- Emission split defaults (registry params):
  - Operator reserve: **40%** (`operator_reserve_bps=4000`)
  - Publisher emission: **10%** (`publisher_emission_bps=1000`)
  - Verifier emission: **50%** (`verifier_emission_bps=5000`)
- Emission duration default: **100 years** (`876,000` hours)

## Hourly Emission (Current Default)

For `round_interval_seconds=3600`, defaults map to:

- Publisher pool per hour: **114,155,251 ucongrid** (~114.155251 CONGRID)
- Verifier pool per hour: **570,776,255 ucongrid** (~570.776255 CONGRID)

These values are computed by `PublisherParams.RoundEmissionPools(...)`.

## How Rewards Are Paid (Current Implementation)

Registry verification rewards are paid during round finalization in `x/registry`.

1. Chain computes per-round publisher/verifier pools.
2. `tokenomics` module ensures/holds the emission pool balance.
3. Payouts are **transfer-based from module pool** (`SendFromPool`) rather than mint-per-recipient.
4. Unclaimed portions are burned from pool (`BurnFromPool`).

## Publisher Reward Rule

- Rewards settle only after every publisher assignment in the round is finalized.
- A publisher is active when the registered homepage passes badge verification: the official `congrid.net` anchor wraps a badge image whose publisher domain and wallet match the on-chain registration.
- `owner` is both the registration-control wallet and the publisher-reward recipient. Re-registration may change owner/referrer, but verifier consensus must first confirm `pending_owner` in the homepage badge; the existing owner remains effective until then.
- The publisher pool is split evenly across active publishers only. Inactive publishers do not dilute the split.
- Each active publisher's claim is then adjusted by matching similar-site links:
  - full-reward threshold: `required_external_links_for_full_reward` (default `15`)
  - minimum claim: `publisher_min_reward_bps` (default `1000`, or 10%)
  - formula: `max(10%, matched_links / required_links)`, capped at 100%
- Similar-site links affect payout only, not active status. A publisher with zero matching links still receives 10% of its equal base share.
- Unclaimed publisher amount is burned.

## Verifier Reward Rule

Only verifiers with successful submissions in finalized pass-majority rounds are eligible.

Verifier payout per assignment uses a hybrid split:

1. **Base share** (`verifier_reward_base_share_bps`, default `4000` = 40%)
   - equal split among successful verifiers.
2. **Weighted share** (remaining 60%)
   - proportional to

`weight = bonded_stake × referral_factor`

Where referral factor uses active referred publishers (minimum factor 1).

- Base share improves small-operator participation.
- Weighted share preserves stake-based Sybil resistance.
- If no eligible verifier exists, verifier pool for that assignment is burned.
- If weighted share has no positive-weight verifier, that weighted remainder is burned.

## Important Scope Notes

### Verifier operating fees

Drand delivery now uses a bond-weighted deterministic per-round primary with staggered
fallbacks, so a healthy network normally produces one drand transaction per
round. At hourly cadence with `gas=250000` and
`gas_prices=0.001ucongrid`, network-wide drand cost is approximately
`6000ucongrid` (0.006 CONGRID) per day. This removes the O(N) duplicate-delivery
cost as verifier count grows without changing total issuance or the
publisher/verifier emission split.
At the same gas settings, commit plus reveal costs approximately
`500ucongrid` (0.0005 CONGRID) per successful assignment, excluding abnormal
retries.

- Tokenomics params for fixed issuance split (operator/publisher/verifier), fee routing, and slash routing exist and are validated.
- Some whitepaper-level flows (full consumer payment rail, full slash compensation rail, and automated operator reserve distribution) are still partially modeled and not yet fully wired into end-to-end production settlement paths.

See:
- `x/registry/types.go`
- `x/registry/verification_rounds.go`
- `x/tokenomics/keeper.go`
