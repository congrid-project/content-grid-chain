# verifierd and content-grid-d integration assessment

## Decision

Do not embed verifierd as a goroutine inside the `content-grid-d start`
consensus-node process. The recommended target is **one release binary, two
isolated processes**, for example:

```bash
content-grid-d start --home /var/lib/congrid
content-grid-d verifier start --config /etc/congrid/verifierd.json
```

This reduces build, download, and version-matching overhead while preserving
failure, key, resource, and restart boundaries. The `drand-strict-v2` release
continues to ship the standalone verifierd executable so an urgent consensus
upgrade does not also carry a large process refactor. A compatible
single-binary subcommand can ship later without another chain upgrade.

## Why not one process

- verifierd performs nondeterministic publisher, indexerd, and drand HTTP I/O;
  none of it belongs in ABCI or consensus execution.
- HTTP, keyring, or worker failures must not terminate the consensus node.
- Consensus-validator and verifier transaction keys need separate least-
  privilege keyrings.
- Verifiers and consensus validators are not one-to-one; a non-validator must
  remain able to run verifierd independently.
- verifier restarts, upgrades, rate limits, and resource spikes must not affect
  block production.

## Options

| Option | Maintenance | Consensus isolation | Recommendation |
| --- | --- | --- | --- |
| Separate `content-grid-d` and `verifierd` binaries | Current | Strong | Keep for this upgrade |
| One `content-grid-d` binary, separate subcommand/process | Lower | Strong | Follow-up target |
| `content-grid-d start --with-verifier` in one process | Superficially lowest | Weak | Reject |

## Follow-up boundary

1. Refactor verifierd agent/config/health code into an importable package.
2. Keep a thin standalone verifierd compatibility entrypoint.
3. Register the same runner as `content-grid-d verifier start`.
4. Preserve separate PIDs, logs, health ports, keyrings, and resource limits.
5. Consider retiring the compatibility executable only after one release
   cycle.

Packaging consolidation must not change `MsgSubmitDrandBeacon`, commit/reveal,
or reward semantics. It therefore needs no upgrade handler and must not modify
the node home, consensus database, or validator keys.
