# `drand-strict-v2` Mainnet Upgrade

This runbook upgrades an existing ConGrid chain. The governance software plan
name must be exactly `drand-strict-v2`.

The handler runs all module migrations, moves registry from consensus version 1
to 2, fills missing quicknet metadata, and forces `drand_enabled=true` and
`drand_strict_mode=true` without replacing existing chain state.

## Procedure

1. Choose an upgrade height after the full deposit/voting period and leave time
   for every validator to test, download, verify, and back up its node.
2. Build `content-grid-d` and `verifierd` from the same reviewed Git commit:

   ```bash
   go test ./...
   mkdir -p build
   go build -trimpath -o build/content-grid-d ./cmd/content-grid-d
   go build -trimpath -o build/verifierd ./offchain/verifierd
   sha256sum build/content-grid-d build/verifierd
   ```

3. Publish per-platform archives. The node archive must contain an executable
   named `content-grid-d`. Put `?checksum=sha256:<hex>` on each download URL.
   The official site can serve the amd64 archive from
   `cmd/congrid-site/downloads` at
   `https://congrid.net/downloads/content-grid-d-linux-amd64.tar.gz`.
4. Prepare the new verifierd with `drand.disabled=false` and sufficient fees or
   a message-restricted fee grant.
5. Submit and pass the proposal:

   ```bash
   export UPGRADE_NAME=drand-strict-v2
   export UPGRADE_HEIGHT=<future-height>
   export NODE_ARCHIVE_URL='https://congrid.net/downloads/content-grid-d-linux-amd64.tar.gz?checksum=sha256:<sha256>'
   export UPGRADE_INFO="$(jq -nc --arg url "$NODE_ARCHIVE_URL" '{binaries:{"linux/amd64":$url}}')"

   content-grid-d tx upgrade software-upgrade "$UPGRADE_NAME" \
     --title "Enable strict drand delivery" \
     --summary "Run registry v1-to-v2 migration and enable exact-round strict drand" \
     --upgrade-height "$UPGRADE_HEIGHT" \
     --upgrade-info "$UPGRADE_INFO" \
     --deposit <governance-deposit> \
     --from <proposer-key> \
     --chain-id <chain-id> \
     --node <rpc-url> \
     --fees <fee> \
     -y
   ```

6. Confirm `content-grid-d query upgrade plan`. At least 2/3 voting power must
   have the same binary checksum ready. Never use `--unsafe-skip-upgrades`.
7. Let the old binary run until the planned height. After it halts, replace the
   binary (or let Cosmovisor select
   `$DAEMON_HOME/cosmovisor/upgrades/drand-strict-v2/bin/content-grid-d`) and
   restart with the existing home, database, and consensus keys. Do not re-init
   the node or overwrite state with a genesis file.
8. For Docker Compose, the new images may be built in advance, but recreate the
   node container only after the old node halts. Preserve the existing
   `congrid-home` volume; never delete it during the upgrade.
9. Start the new verifierd and verify:

   ```bash
   content-grid-d query upgrade applied drand-strict-v2
   content-grid-d query upgrade module-versions registry
   content-grid-d query registry drand-requirement
   curl -fsS http://127.0.0.1:9200/readyz | jq
   ```

Registry must report version 2 and the drand requirement must report
`enabled=true`. Once the handler has executed, individual validators must not
roll back to the old binary. See the
[Chinese production runbook](upgrade-drand-strict-v2-zh.md) for detailed
preflight, voting, cancellation, and recovery steps.
