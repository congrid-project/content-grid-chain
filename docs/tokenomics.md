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

- Publisher pool is split evenly across active assignments in that round.
- A publisher can claim full share only if required external links threshold is met:
  - `required_external_links_for_full_reward` (default `10`)
- If below threshold, claim is proportional.
- Unclaimed publisher amount is burned.

## Verifier Reward Rule

Only verifiers with successful submissions in finalized pass-majority rounds are eligible.

Verifier weight:

`weight = bonded_stake × referral_factor`

Where referral factor uses active referred publishers (minimum factor 1).

- Higher stake => higher share.
- More active referred publishers => higher share.
- If no eligible verifier exists, verifier pool for that assignment is burned.

## Important Scope Notes

- Tokenomics params for inflation/block splits/fee routing exist and are validated.
- Some whitepaper-level flows (full consumer payment rail, full slash compensation rail, full block-level inflation router) are still partially modeled and not yet fully wired into end-to-end production settlement paths.

See:
- `x/registry/types.go`
- `x/registry/verification_rounds.go`
- `x/tokenomics/keeper.go`
