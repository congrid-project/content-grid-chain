# Content Grid Protocol White Paper

Implementation-aligned revision · 2026-09-24 · Code baseline: `905bb09`

[中文](whitepaper-zh.md) · [Usage guide](README.md) · [Contributor guide](CONTRIBUTING.md)

## 1. Purpose and scope

Content Grid (Congrid) is a content-discovery protocol connecting independently operated websites. Publishers register their domains, prove control through homepage badges, and display recommendations derived from content similarity. Advertisers can separately purchase available link placements through a slot-and-lease marketplace.

The blockchain coordinates registration, verifier bonds, verification assignments, results, rewards, and lease escrow. Crawling, embeddings, and similarity search run off-chain. CONGRID is the native token used for verifier bonds, protocol rewards, transaction fees, and supported marketplace payments.

This paper describes the current repository implementation and its limits. Default parameters are reference values, not a statement of a deployed network's configuration. Code support for IBC, governance, or upgrades does not establish that a particular production upgrade, exchange connection, liquidity pool, or audit has completed.

The long-term goal is a more open discovery network with inspectable rules and less dependence on a single discovery platform. Current verification establishes homepage/domain-wallet binding; it does not certify editorial quality, factual accuracy, traffic, or search-engine ranking value.

## 2. Architecture and participants

### 2.1 Blockchain coordination

The application uses Cosmos SDK v0.53.4 and CometBFT v0.38.17. Block consensus uses the Cosmos staking validator set and CometBFT consensus. Crawling and embedding computation are application services, not a proof-of-work mechanism for producing blocks.

| Component | Current responsibility |
| --- | --- |
| `x/registry` | Publisher records, drand beacons, verification assignments, commit–reveal, similarity evidence, reward settlement, slots, and leases |
| `x/verifiers` | Account-based verifier bonds held in module escrow |
| `x/tokenomics` | Emission-pool funding, reward transfers, and burns |
| Cosmos modules | Accounts, balances, consensus staking/slashing, distribution, governance, fee grants, and software upgrades |
| IBC Core and ICS-20 | Cross-chain fungible-token transfer support |
| `offchain/verifierd` | Homepage checks, commit–reveal submissions, and required drand beacon delivery |
| `offchain/indexerd` and Chroma helper | Homepage indexing, embeddings, semantic queries, and similar-site results |
| `cmd/congrid-site` | Web onboarding, publisher dashboards, marketplace, and downloads |

### 2.2 Roles

- **Publishers** register a website and maintain its badge and optional recommendation links. Registration requires no publisher bond.
- **Verifiers** bond from normal account addresses, inspect assigned websites, and submit observations. Their bonds are distinct from Cosmos consensus staking.
- **Consensus validators** produce and finalize blocks and participate in Cosmos staking, distribution, and slashing. Running a verifier does not automatically make an operator a consensus validator.
- **Index operators** serve off-chain content and similarity data. Running an index alone does not currently earn a separate on-chain query-task reward.
- **Consumers and advertisers** use content discovery or rent listed placements. General-purpose crawl bounties and metered paid-search tasks are not implemented as a complete protocol workflow.

Verifier eligibility uses `x/verifiers` bond denomination and minimum-bond parameters, active status, and registry suspension state. The current minimum-bond default is 1 `ucongrid`; the older registry `verifier_bond` reference field is not the assignment eligibility threshold. Unbonding returns escrow through the verifier module; it does not use the Cosmos staking unbonding queue.

## 3. Publisher registration and ownership

A publisher registers a normalized domain and an owner address. The registered homepage must contain a link to `https://congrid.net` or `https://www.congrid.net/`, without query or fragment, wrapping a badge image from either official HTTPS origin. The image URL identifies the publisher domain and owner wallet.

The CLI registration helper checks the page before broadcasting. On-chain registration records the request; subsequent verifier observations determine acceptance. A new record begins as `PENDING`. A transaction containing exactly one `MsgRegisterPublisher` is fee-exempt under the custom ante policy; other transactions follow the applicable fee policy.

The registry reserves a primary-domain key to prevent competing subdomain registrations. The present implementation removes the port and uses the final two hostname labels. It is not a Public Suffix List implementation: multi-label suffixes such as `co.uk` require care and remain a limitation.

The `owner` is both the control address and publisher reward recipient. Re-registration stores `pending_owner` and `pending_referrer` without immediately replacing the incumbent. Assignments bind to the candidate wallet. Acceptance updates ownership and associated slot ownership; rejection with quorum clears the candidate while preserving the incumbent. A round without quorum does not accept the candidate. Omitting the referrer in an accepted re-registration clears the previous referrer.

## 4. Verification rounds and drand assignment

### 4.1 Round scheduling

Assignments target the next round boundary, rather than an immediate same-round task on registration. Eligible publishers include pending and verified records outside cooldown, plus records with a pending re-registration candidate.

Default timing and selection parameters are:

| Parameter | Default |
| --- | ---: |
| Round interval | 3,600 seconds |
| Requested verifiers per publisher | 3 |
| Commit window from assignment start | 300 seconds |
| Total submission window from assignment start | 600 seconds |
| drand offset before round start | 60 seconds |

For hourly or longer rounds, each domain receives a deterministic minute offset from 0 to 59 within the first hour. Short test rounds use bounded second offsets. A late assignment can therefore finalize after the nominal round has ended. When fewer eligible verifiers exist than requested, selection uses the available set; it does not enforce a three-verifier minimum quorum.

### 4.2 External randomness and seed derivation

drand is enabled by default, using configured quicknet metadata and the `bls-unchained-g1-rfc9380` BLS scheme. Each upcoming Content Grid round maps to exactly one drand round:

```text
latest_allowed_time = content_round_start - drand_round_offset_seconds
required_drand_round =
    floor((latest_allowed_time - drand_genesis_time_unix) / drand_period_seconds) + 1
```

The default drand genesis timestamp is `1692803367`, with a 3-second period. The chain checks the exact required round, its BLS signature against the configured public key, and `randomness = SHA256(signature)`. Unexpected rounds and duplicate submissions are rejected.

With drand enabled, missing the required beacon prevents assignment creation. There is no automatic block-hash fallback; the legacy `drand_strict_mode` field does not relax this rule. Explicitly disabling drand selects the separate anchor-only code path.

The seed combines chain ID, Content Grid round start, the assignment-creation block's preceding block height/hash, drand round, and drand randomness through SHA-256. The exact encoding is defined in [assignment_random.go](x/registry/typespb/assignment_random.go). The block anchor remains an input; the obsolete `Hash(BlockHash + TaskID + Counter) % TotalMiners` description does not represent current selection.

From the eligible verifier set, the chain performs deterministic stake-weighted sampling without replacement for each domain. Selected addresses are sorted. Stored round metadata includes seed, anchor, drand data, and the eligible-address-set hash; reproduction also requires the bond weights from the applicable historical state.

### 4.3 Beacon delivery

Beacon delivery runs inside `verifierd`, which queries `DrandRequirement` and fetches the specified round rather than continually submitting the latest beacon. Operators deterministically choose a bond-weighted primary and stagger fallback delivery. These off-chain scheduling rules reduce duplicate transactions; on-chain verification and single acceptance remain authoritative. Beacon submissions consume transaction fees unless sponsored through a fee grant.

See [drand](docs/drand.md) for configuration and delivery details.

### 4.4 Commit–reveal and result finalization

Only assigned verifiers may submit. A commit binds the domain, round, verifier, assignment owner, pass/fail result, evidence hash, and nonce. Reveal is accepted after the commit window and before the assignment deadline, and must match the commit.

After the deadline, the current rule for `N` assigned verifiers is:

```text
quorum = ceil(N / 2), for N > 0
has_quorum = valid_reveals >= quorum
passed = has_quorum AND pass_reveals > fail_reveals
```

This is an application-level reveal quorum, not a 67% threshold or CometBFT block-consensus rule. Votes are counted per verifier, not weighted by stake. Ties do not pass. Missing quorum finalizes the assignment as unverified without treating it as a quorum-backed publisher failure.

A passing regular assignment sets or keeps the publisher as `VERIFIED` and clears its failure streak. A quorum-backed failure can move a verified record to `PENDING`; the `REVOKED` transition currently checks the threshold only in the branch for a record that is still `VERIFIED`. The default threshold is three, but it must not be described as a guaranteed automatic revocation after any three consecutive failures: subsequent failures of an already-pending record do not run that transition.

Missed reveals and votes opposing the finalized pass/fail decision incur verifier penalty counts. The default suspension threshold is three penalties, with a suspension duration of three round intervals. Non-penalized participation clears the count. This path currently imposes assignment suspension, not automatic confiscation of the verifier bond. Cosmos validator slashing is separate.

## 5. Content indexing and recommendations

`indexerd` discovers publishers from the registry and/or a configured static list, crawls homepages, normalizes content, and computes embeddings and compact similarity signatures (128 bits by default). When chain filtering is configured, active publishers are verified and outside cooldown; inactive entries are pruned.

The Chroma helper supports embedding storage and semantic similarity search. An in-memory cosine-search fallback exists for development. Large vectors and page text remain off-chain. The current system does not require every consensus validator to hold a complete global vector index or assign a paid miner committee for each search request. DHT and executor code do not constitute that end-to-end workflow.

The similar-site endpoint returns up to 15 domains by default, rather than the former ten-URL design. Publishers can display those content-based recommendations. Paid slot selection is a separate marketplace action, not an input that purchases a higher similarity score.

Verifier observations include expected and observed set hashes and matched-domain counts. Among passing reveals, an expected-set hash must itself reach the assignment quorum. The matched count is then taken from the sorted middle entry of agreeing observations (the upper middle for an even count), capped at 15. Without that agreement, the rewarded matched count is zero.

Similarity links affect the publisher reward multiplier; they do not change badge pass/fail status. Independent indexes can differ because of crawl timing, available sites, and embedding configuration. The chain aggregates submitted evidence; it does not rerun embeddings or establish that a recommendation is editorially high quality.

## 6. Link slots and leases

A verified publisher can create a slot with a rate denomination, rate amount, time unit, and duration bounds. Slots may be listed, paused, or unlisted. A buyer selects the target URL and a valid duration; overlapping active leases on the same slot are rejected.

```text
escrow_payment = rate_amount × (duration_seconds / unit_seconds)
```

The buyer's payment is escrowed in the registry module. The slot's denomination controls payment; CONGRID uses `ucongrid`, but the underlying slot model supports a denomination field rather than hard-coding every lease to CONGRID. At expiry, remaining escrow is paid to the recorded lease publisher unless the implemented cooldown/refund conditions apply. A qualifying publisher verification failure with active leases can trigger cooldown and refund the remaining escrow.

The repository includes a lease-anchor checking helper using slot/lease attributes and target URLs. However, the current `verifierd` assignment loop checks the homepage badge and similarity evidence, not individual lease anchors. Missing a paid link alone therefore does not currently generate a separate automatic breach decision. Escrow and publisher-failure refunds are implemented; complete per-placement delivery verification remains follow-up work.

## 7. Token economics

### 7.1 Reference allocation and funding

`1 CONGRID = 1,000,000 ucongrid`. Current registry defaults use a reference supply of 1 billion CONGRID:

| Allocation | Share of reference supply |
| --- | ---: |
| Operator reserve | 40% |
| Publisher emission pool | 10% |
| Verifier emission pool | 50% |

The emission-duration parameter is 876,000 hours (100 years of 365 days). The operator reserve is not automatically distributed by registry settlement; it requires an explicit allocation.

`EnsureEmissionPool` mints the difference between the configured emission target and its tracked cumulative funded amount into the tokenomics account. Rewards subsequently transfer from that pool; unclaimed portions burn from it. This is not hourly minting to every recipient, nor is it automatic replenishment after every withdrawal.

The reference supply and duration are inputs to the reward schedule, not a global enforced supply cap or a calendar-based end date. The Cosmos mint module is also wired, so total production issuance must be assessed against actual genesis/mint parameters and upgrades. Empty rounds do not settle a pool, and exhausting the pool is not handled by a dedicated schedule-end mechanism.

### 7.2 Per-round pools

For round duration `T` in seconds, reference supply `S` in `ucongrid`, allocation `bps`, and duration `H` in hours:

```text
round_pool = floor(S × bps × T / (10000 × H × 3600))
```

At the default hourly interval:

- Publisher pool: 114,155,251 ucongrid (114.155251 CONGRID).
- Verifier pool: 570,776,255 ucongrid (570.776255 CONGRID).

These are network-wide round pools, not fixed payments per website. Settlement waits for all assignments in the round to finalize.

### 7.3 Publisher rewards

The publisher pool is divided equally among verified assignments for the round. Each publisher receives its base share multiplied by:

```text
claim_bps = max(publisher_min_reward_bps,
                min(10000, floor(matched_links × 10000 / required_links)))
payout = floor(base_share × claim_bps / 10000)
```

Defaults are `publisher_min_reward_bps = 1000` and `required_links = 15`. A badge-valid publisher with zero matching links receives 10% of its base share; 15 matching links earns 100%. Insufficient similarity evidence does not remove the floor. Unclaimed amounts and integer remainder are burned.

### 7.4 Verifier rewards

The verifier pool is first divided across all assignments, including failed ones. Only verified assignments distribute their share, and only passing submissions within those assignments receive payment. A correct fail vote can avoid a penalty but does not currently earn this reward.

Within a successful assignment:

- 40% is divided equally among successful verifiers.
- 60% is divided in proportion to `bonded_stake × max(1, active_referred_publishers)`.

Bonds and active referrals are read at settlement; referral weight does not affect assignment selection. Unpayable amounts and remaining round funds are burned. Legacy score-weight and fee/slash-routing parameter structures must not be mistaken for fully connected payout or compensation paths.

## 8. Governance, interoperability, and implementation boundaries

The application includes Cosmos governance and software-upgrade support. Custom module parameters exist, but there is no general `MsgUpdateParams` governance interface for all custom modules. Parameter changes must follow the supported genesis or upgrade path.

IBC Core, Tendermint light clients, and ICS-20 transfers are integrated through ibc-go v10.7.0, with an `ibc-transfer-v1` migration for existing chains. This enables cross-chain transfer infrastructure; external channels, relayers, asset registration, and DEX liquidity remain separate deployment steps. Repository tests and rehearsals do not establish an active Osmosis mainnet market. See [IBC documentation](docs/ibc.md).

Current follow-up areas are:

- Complete independent verification and settlement of each paid placement.
- Content-quality and abuse resistance beyond badge ownership and similarity evidence.
- Index consistency and broader distributed-index operation.
- General consumer bounties and paid API settlement.
- Custom parameter governance, verifier bond-slashing/compensation, and automated reserve distribution.
- Supply accounting and emission-end handling across all minting paths.
- Public-suffix-aware domain ownership keys and consistent failure-to-revocation transitions.

This replaces the former phase checklist, which marked existing indexing and rewards as future work while describing removed mining tasks as complete. Operational launch and audit claims require separate deployment evidence.

## 9. Implementation references

- [Registration and verification transactions](x/registry/msg_server.go)
- [Round assignment, penalties, and settlement](x/registry/verification_rounds.go)
- [drand schedule](x/registry/drand_schedule.go) and [signature verification](x/registry/drand_verify.go)
- [Default parameters and emission formula](x/registry/types.go)
- [Similar-site evidence aggregation](x/registry/similar_settlement.go)
- [Emission-pool funding](x/tokenomics/keeper.go)
- [Lease settlement](x/registry/lease_settlement.go) and [verifier agent](offchain/verifierd/agent.go)
- [IBC wiring](app/ibc.go) and [upgrade handlers](app/upgrades.go)

For participation and operations, see the [README](README.md). Build, testing, and contribution procedures are in [CONTRIBUTING.md](CONTRIBUTING.md).
