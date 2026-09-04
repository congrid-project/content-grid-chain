# Content Grid Chain

Content Grid Chain is the on-chain coordination and settlement layer for a decentralized publisher-discovery network. It records publisher ownership, assigns independent verifiers, reaches consensus on website proofs, and settles publisher/verifier rewards plus link-marketplace payments.

[中文说明](README-zh.md) · [Protocol whitepaper](whitepaper.md) · [Contributor guide](CONTRIBUTING.md)

> Project status: the chain runtime, publisher registry, verifier workflow, drand randomness, rewards, and slot/lease marketplace are implemented. A production mainnet release bundle (binary, genesis, and peers) is not published by this repository yet. Where the whitepaper and code differ, the current code and protocol documents are authoritative.

## Protocol overview

Content Grid connects four roles:

- **Publishers** register a domain and prove control by placing a wallet-bound Congrid badge on the homepage.
- **Verifiers** bond `CONGRID`, independently inspect assigned publishers, and submit results through commit–reveal.
- **Consensus validators** run the Cosmos chain, order transactions, and finalize protocol state.
- **Consumers and advertisers** discover publishers or lease verified link slots.

A normal publisher verification moves through this flow:

1. The publisher registers a domain. The record starts as `PENDING`.
2. At a verification-round boundary, the chain obtains the required drand beacon and derives an auditable round seed.
3. Eligible, non-suspended verifiers are selected deterministically using stake-weighted sampling without replacement.
4. Selected verifiers inspect the homepage and first commit, then reveal, their result and evidence.
5. Once quorum is reached, the chain finalizes the majority result, updates the publisher status, applies verifier penalties where needed, and settles rewards.
6. Verified publishers may list link slots. Lease payments remain in escrow until the lease completes or verification causes a refund.

## Consensus model

Content Grid uses two related but distinct consensus layers.

### Chain consensus

The blockchain is built with Cosmos SDK v0.53 and CometBFT v0.38. Cosmos consensus validators provide Byzantine-fault-tolerant block ordering and finality; staking, slashing, governance, bank, and distribution use the standard Cosmos modules wired into the application.

### Publisher-verification consensus

Website verification is application-level consensus recorded by the chain:

- Rounds are one hour by default; timing and thresholds are on-chain parameters.
- drand is enabled in strict mode by default. The chain accepts exactly the beacon required for the next round, verifies its BLS signature, and does not fall back to block-hash-only randomness.
- Verifier selection is deterministic, stake-weighted, and without replacement, so any observer can reproduce it from the round seed and on-chain verifier set.
- Commit–reveal keeps votes hidden during the commit window. A result needs quorum, and a pass requires more pass reveals than fail reveals.
- Missed submissions or votes against the finalized result accumulate penalties; repeated penalties can temporarily suspend a verifier from assignments.
- Publisher states are `PENDING`, `VERIFIED`, and `REVOKED`. A verified publisher that changes its owner or referrer keeps its current record until a new verification round accepts the pending change.

`validator` always means a Cosmos consensus validator. `verifier` means the separate Content Grid website-verification role; one account does not need to perform both roles.

See [verifier rules](docs/verifiers.md) and [drand rules](docs/drand.md) for exact behavior.

## Protocol components

| Component | Responsibility |
| --- | --- |
| `x/registry` | Publisher records, verification rounds, commit–reveal, similar-site evidence, slots, leases, and settlement |
| `x/verifiers` | Verifier bond escrow and eligibility |
| `x/tokenomics` | Issuance-pool funding, transfers, and burns |
| `offchain/verifierd` | Fetches assignments, checks publisher pages, submits commits/reveals, and delivers required drand beacons |
| `offchain/indexerd` | Indexes publisher homepages and produces similarity results and compact signatures |
| `cmd/congrid-site` | Publisher onboarding, dashboards, downloads, and marketplace web entry points |

The default economic reference is 1 billion `CONGRID` (`1 CONGRID = 1,000,000 ucongrid`): 40% operator reserve, 10% publisher emissions, and 50% verifier emissions over 100 years. Publisher rewards depend on badge validity and matching similar-site links. Verifier rewards combine an equal base share with a stake-and-referral-weighted share. Unclaimed round emissions are burned. These values are genesis parameters and may differ on a deployed network. Automated operator-reserve distribution, the complete consumer payment rail, and the full slash-compensation rail are not yet wired end to end.

See [tokenomics](docs/tokenomics.md), [marketplace](docs/marketplace.md), and [governance](docs/governance.md) for the complete rules and current scope.

## Use the protocol

Network-specific values such as the chain ID, RPC endpoint, genesis file, and seed peers must come from the network operator or official release bundle.

### Register a publisher

Browser-wallet users can use the onboarding helper at [congrid.net/publishers](https://congrid.net/publishers) to generate the badge snippet and registration command.

Place a Congrid link on the registered domain's homepage. The link must wrap an image whose URL carries the domain and signing wallet:

```html
<div id="congrid-similar">
  <a href="https://congrid.net">
    <img
      src="https://congrid.net/badge.svg?publisher=example.com&wallet=<congrid-address>"
      alt="Verified by Congrid"
      width="32"
      height="32"
    />
    <span>Congrid — Content Grid Protocol</span>
  </a>
  <!-- Add the similar-publisher links returned by the network here. -->
</div>
```

The anchor must target `https://congrid.net` or `https://www.congrid.net/` without a query or fragment. The image must be hosted below one of those origins and encode `publisher=<domain>` and `wallet=<owner>` in its path or query. The wallet must match the signer.

Register the domain:

```bash
./content-grid-d publisher register example.com \
  --from <publisher-key> \
  --chain-id <chain-id> \
  --node <rpc-url> \
  --fees 0ucongrid
```

Optional flags include `--metadata-uri` and `--referrer`. Registration itself requires no bond. A transaction containing only `MsgRegisterPublisher` may use zero fees; mixed transactions still follow the network's minimum-gas-price policy.

Query the record:

```bash
./content-grid-d query registry publisher \
  --domain example.com \
  --node <rpc-url>
```

The `owner` is both the control wallet and reward recipient. To change the owner or referrer, update the homepage badge and submit the same registration command with the new signer. `pending_owner` and `pending_referrer` remain candidates until verifier consensus accepts them; a failed change leaves the current registration intact.

The registry reserves a primary-domain key to prevent competing subdomain claims. The current implementation derives that key from the final two hostname labels after removing a port, so operators of multi-label public suffixes should check the resulting scope before registering.

### Run a verifier

Bond tokens from a normal `congrid1...` account:

```bash
./content-grid-d verifier bond 1000000 --denom ucongrid --from <verifier-key>
./content-grid-d verifier assignments --from <verifier-key>
```

Then run `verifierd` to process assignments and participate in drand delivery. Unbond with:

```bash
./content-grid-d verifier unbond 1000000 --denom ucongrid --from <verifier-key>
```

Follow the [verifierd guide](docs/verifierd.md) for configuration, keys, fees, and health checks. For a containerized node plus verifier stack, see the [Docker operator guide](docs/docker-operator.md).

### Run a full node or validator

Use the official binary, genesis, and peer list for a public network. The [production runbook](docs/runbook.md) covers node health and incident handling; the [launch checklist](docs/launch-checklist.md) describes release readiness. Consensus validators join through the standard Cosmos `gentx` flow at genesis or `tx staking create-validator` after launch.

For local development networks, builds, tests, and protobuf generation, use the [contributor guide](CONTRIBUTING.md).

### Use the link marketplace

A verified publisher can list a slot, an advertiser can lease it, and the protocol escrows payment while the lease is active. The publisher must expose the required `data-congrid-slot-id` and `data-congrid-lease` markup so verifiers can check delivery. See the [marketplace guide](docs/marketplace.md) for lifecycle states and CLI examples.

### Query the network

The daemon exposes Cosmos RPC/gRPC services and registry REST routes. Useful CLI queries include:

```bash
./content-grid-d query registry publisher --domain example.com --node <rpc-url>
./content-grid-d query registry drand-requirement --node <rpc-url>
./content-grid-d query registry slots --publisher <congrid-address> --node <rpc-url>
./content-grid-d query registry leases --slot-id <slot-id> --node <rpc-url>
```

The API definitions live under [`proto/contentgrid`](proto/contentgrid).

## Documentation

- Protocol design: [whitepaper](whitepaper.md) (some legacy sections describe planned or removed scope)
- Verification and indexing: [verifiers](docs/verifiers.md), [verifierd](docs/verifierd.md), [drand](docs/drand.md), [indexerd](docs/indexerd.md)
- Economics and marketplace: [tokenomics](docs/tokenomics.md), [marketplace](docs/marketplace.md), [governance](docs/governance.md)
- Operations: [Docker operator](docs/docker-operator.md), [runbook](docs/runbook.md), [launch checklist](docs/launch-checklist.md)
- Development: [contributor guide](CONTRIBUTING.md)

## License

Source code and documentation are licensed under the [MIT License](LICENSE).

Distinct non-code ecosystem intellectual property is available under the [Chain Inspiring License v1.0](CHAIN-INSPIRING-LICENSE.md). Using MIT-licensed code or documentation alone does not trigger CIL-1.0. CIL-1.0 is a custom ecosystem intellectual-property license and is not an OSI-approved open-source software license; read the license text before adopting protected ecosystem material.
