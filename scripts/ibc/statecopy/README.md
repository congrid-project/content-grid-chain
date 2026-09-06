# Offline production-state upgrade rehearsal

This harness loads a **disposable copy of the production application database**.
It does not start CometBFT, connect to peers, load signing keys, submit real
transactions, or replace production files. Governance and signed ICS-20 flows
are exercised separately by `../localnet.py`.

Build the same harness twice, once against the exact source commit of the
running release and once against the candidate working tree:

```bash
python3 scripts/ibc/build_statecopy.py --source /path/to/production-source --output /path/to/old-statecopy
python3 scripts/ibc/build_statecopy.py --source . --output /path/to/new-statecopy
```

The builder copies `cmd/content-grid-d/dbfix.go` from each selected release.
This is essential: the production daemon uses that adapter to distinguish
existing empty IAVL roots from absent keys. A plain cosmos-db opener is not an
equivalent reproduction of daemon startup. The harness is excluded from normal
builds by the `ibc_rehearsal` build tag.

## Inputs

Obtain a consistent copy from a gracefully stopped non-voting replica, an
application-consistent backup, or a supported snapshot mechanism. Copy public
configuration and database files only. Verify the replica is not in the active
validator set before temporarily stopping it, and restore it immediately after
copying. Do not copy live LevelDB files while they are being written.

For this harness the copied home needs:

- `config/genesis.json` containing the original chain ID.
- `data/application.db` containing the captured state, plus existing
  `data/upgrade-info.json` if present.
- `source-validators.json`: the RPC `validators` result for the captured height.
- `IBC_REHEARSAL_COPY`: an explicit marker created **only inside the copy**.

The harness refuses copied node/validator private keys, keyrings, and symlinked
store/config directories. Never create the marker in a production node home.
Keep the original archive immutable and use a fresh extracted copy for each run.

Independently verify the committed state hash at height H against the `app_hash`
in production block H+1 (a block's header commits to the previous application
state). Record the block time at H and the source binary hash/source commit.

## Execution

```bash
/path/to/old-statecopy --mode inspect --home /path/to/copy \
  --block-time "$SOURCE_BLOCK_TIME" --expected-app-hash "$SOURCE_APP_HASH"
/path/to/old-statecopy --mode prepare --home /path/to/copy \
  --block-time "$SOURCE_BLOCK_TIME" --expected-app-hash "$SOURCE_APP_HASH"
/path/to/new-statecopy --mode migrate --home /path/to/copy \
  --block-time "$PREPARED_BLOCK_TIME"
/path/to/new-statecopy --mode verify --home /path/to/copy \
  --block-time "$FINAL_BLOCK_TIME" --expected-app-hash "$FINAL_APP_HASH"
```

`PREPARED_BLOCK_TIME` is the source time plus 30 seconds; `FINAL_BLOCK_TIME` is
the source time plus 120 seconds. `FINAL_APP_HASH` comes from `migrate-report.json`.

`inspect` recomputes the IAVL store root hash, compares it with the recorded
commit and the independently verified production hash, and exports application
state and per-store digests.

`prepare` injects an upgrade plan **offline** at H+2, processes one synthetic
empty block with the old application, and verifies that the old application
refuses the next block and writes its normal upgrade-info file. This is a state
migration rehearsal; it does not simulate possession of production voter keys
or claim that a production governance vote occurred. Synthetic commit votes use
public validator information solely as ABCI input, without signatures.

`migrate` runs the candidate's real store loader and registered pre-blocker.
It compares all existing persistent key/value entries before and after the
pre-blocker, allowing changes only in `upgrade`, `ibc`, and `transfer`. Auth,
bank, staking, registry, and all other existing stores must be byte-identical
across the migration. Normal begin/end-block economics run after this comparison.
The application processes and commits three synthetic blocks. `verify` reopens
the database in another process and checks its recomputed state hash, module
versions, and persisted applied-upgrade height.

Reports and exports are written with mode 0600 inside the disposable home.
These tests do not establish a production IBC channel or measure multi-validator
consensus restart time.
