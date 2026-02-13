# Registry Module (x/registry)

Minimal website registry and verification module skeleton for Content Grid Chain.

Status: module basics only (genesis/types). Full keeper/storage, messages, and gRPC services will be added once the chain is wired with `runtime.App` and stores.

## Concepts
- Domain registration: records `{domain, owner, status}` where `status ∈ {PENDING, VERIFIED, REVOKED}`.
- Normalization: domains are stored normalized (lowercase, trimmed) with basic format validation.
- Genesis: supports preloading websites via `genesis.json`.

## Next Steps
- Add storage with `cosmossdk.io/collections` and keeper methods.
- Add `MsgRegisterWebsite` and `MsgApprove/VerifyWebsite` messages.
- Expose gRPC queries to fetch website status by domain/owner.

