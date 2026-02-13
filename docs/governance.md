# Governance Playbook

This playbook documents how the Content Grid chain manages monetary policy and operational decisions using `x/gov` once runtime wiring lands.

## Proposal Types

- **Parameter change**: adjusts monetary levers (inflation bands, reward splits, fee ratios). Requires 2-week voting period, 40% quorum, 55% super-majority.
- **Budget allocation**: draws from the community treasury. Must include milestone-based disbursement schedule and accountability checkpoints.
- **Module authority grant**: temporarily hands a custom module the ability to mint or escrow funds. Default expiry: 6 months.
- **Emergency response**: halts inflation or routes slash proceeds to remediation pools. Triggered via expedited proposal with 48-hour voting and 2/3 approval.

## Process Overview

1. **Initiate**: Draft proposal content (Markdown/JSON) and gather off-chain feedback.
2. **Parameter audit**: Run `go run ./cmd/tokenomics simulate ...` for the proposed parameters to document expected supply impact.
3. **Submit**: Use the CLI once governance wiring lands (placeholder command `content-grid-d tx gov submit-proposal`).
4. **Voting**: Validators communicate rationale, watchers verify parameter safety, and community channels host debates.
5. **Enactment**: After the voting period and deposit pass, post-mortem summary goes to the governance archive repo.

## Monetary Guardrails

- Inflation bounds (`[0.04, 0.12]`) cannot be modified without a 3/5 quorum and 2/3 yes vote.
- Reward splits must remain within ±5 percentage points of their defaults unless accompanied by a formal economic analysis attachment.
- No proposal may grant `MintCoins` authority longer than 12 months without on-chain renewal.

## Treasury Management

- Treasury disbursements stream manually via a multi-sig until automated streaming (x/authz) lands.
- Each budget proposal must specify KPIs and reporting cadence; subsequent funding tranches depend on KPI proof.
- Emergency reserve (10% of treasury) is sequestered for security incidents; tapping it requires an emergency proposal.

## Parameter References

| Parameter | Default | Module | Notes |
| --- | --- | --- | --- |
| Inflation min/max/base | 4% / 12% / 7% | `x/tokenomics` | Computed vs bonded ratio |
| Block reward split | 50/25/15/10 | `x/tokenomics` | Tasks/Staking/Publishers/Community |
| Fee burn share | 20% | `x/tokenomics` | Rest flows to task pool |
| Worker reward weights | 40/25/20/15 | `x/tasks` | Success/Consensus/Latency/Uptime |
| Publisher reward weights | 50/30/20 | `x/registry` | Availability/Engagement/Freshness |

## Emergency Checklist

- Snapshot metrics (staking ratio, reward queues, treasury balance).
- Run `tokenomics simulate` with proposed emergency parameters to ensure stability.
- Coordinate validators to stage software/config changes if required (e.g., halting inflation via parameter patch).
- Publish post-mortem within 72 hours of incident resolution.

## Documentation

- Store proposal drafts, simulations, and audit notes in the `gov-records/` directory (to be added once runtime wiring is complete).
- Update this playbook alongside any governance module upgrades.
