# Registry Module (`x/registry`)

The registry module stores publisher websites, verification rounds and
assignments, commit-reveal submissions, ad slots and leases.

## Verification rounds

- New or due publishers are assigned at deterministic round boundaries.
- Eligible verifiers are selected from chain state using the stored round seed.
- Verifiers commit and reveal results inside the configured windows; settlement
  updates publisher status and verifier rewards.

## drand randomness

When `drand_enabled` is true, the chain derives exactly one drand round for the
next Content Grid verification round. `MsgSubmitDrandBeacon` accepts that round
once, requires a signature, and verifies it with the configured BLS scheme and
distributed public key. Assignment creation waits for the required beacon and
does not fall back to block-hash-only randomness.

Use the query below to inspect the current requirement:

```bash
content-grid-d query registry drand-requirement
```

Beacon delivery is part of `offchain/verifierd`; the standalone relayer was
removed. See [`docs/drand.md`](../../docs/drand.md) for the mapping formula,
parameters, deployment and fee-grant guidance.
