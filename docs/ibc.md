# IBC integration and local testing

Congrid integrates `ibc-go v10.7.0` with Cosmos SDK `v0.53.4` and CometBFT
`v0.38.17`. The Osmosis integration path uses Tendermint light clients and
ICS-20 over an unordered `transfer` channel with version `ics20-1`.
The native denomination is `ucongrid` (6 decimals).

## Run the tests

```bash
go test ./app -run TestIBC -count=1 -v
go build -o content-grid-d ./cmd/content-grid-d
python3 scripts/ibc/localnet.py --binary ./content-grid-d --hermes /path/to/hermes
```

The Go tests run signed ABCI transactions against two real Congrid applications,
including Tendermint and Merkle proof verification. They cover native-token and
1,000 test-USDC round trips, escrow/voucher accounting, replay and tampering,
error acknowledgements, native and voucher timeout refunds, and unsafe upgrade
baseline rejection. Only the test genesis disables unrelated SDK inflation to
measure IBC supply conservation. Production economics are unchanged.

The Python script requires Python 3.8+ and the binary from the official
[Hermes v1.13.3 release](https://github.com/informalsystems/hermes/releases/tag/v1.13.3)
(which may report `1.13.2+bab3b80`). It starts two isolated Congrid processes and
Hermes, opens a channel, transfers both assets in both directions, and stops the
relayer to test timeout recovery. All endpoints bind to loopback; all keys and
funds are disposable. Processes are stopped on exit. Logs and `report.json` remain
in the printed private temporary directory. `--work-parent ./tmp` changes the
parent directory. `uusdc-test` is not real USDC.

To rehearse a database upgrade before transferring, retain a pre-IBC baseline
binary and add `--upgrade-from /path/to/pre-ibc/content-grid-d`. Chain A starts on
the old binary, passes an `ibc-transfer-v1` governance proposal, halts, and resumes
using the new binary and the same database. The test verifies the applied upgrade,
unchanged trader balances and genesis before opening the IBC channel.

These are **local Congrid-to-Congrid tests**, not an Osmosis mainnet connection,
an actual USDC bridge test, or a liquidity-pool launch.

## Existing network upgrade

Coordinate the `ibc-transfer-v1` software upgrade with validators. The handler
requires the current pre-IBC module version map, including registry version 3;
complete historical upgrades first. Rehearse against a production state copy,
retain backups, and agree on the height through the existing governance process.

At the halt, stop the old process and install the new binary. CometBFT can halt
consensus without exiting its RPC process. Before database load, the new binary
uses `data/upgrade-info.json` to add the `ibc` and `transfer` stores at exactly the
planned height. The handler initializes the new modules without reinitializing
existing module state. Do not replace genesis, change the chain ID, skip this
upgrade, or start the new binary against the old database before the halt.

Afterward, verify resumed blocks, module versions and business state, then query:

```bash
content-grid-d query ibc client params --node "$CONGRID_RPC"
content-grid-d query ibc-transfer params --node "$CONGRID_RPC"
content-grid-d query ibc channel channels --node "$CONGRID_RPC"
```

## Osmosis and liquidity follow-up

Follow the official [connection guide](https://docs.osmosis.zone/integrate/list-asset/transfer/)
and [asset registration guide](https://docs.osmosis.zone/integrate/list-asset/registration/).
Confirm the actual network and public endpoints, fund dedicated relayer accounts,
configure trusting periods from the actual unbonding periods, and use a single
agreed canonical channel. The test script's trusted-node mode and wildcard channel
filter are for isolated local testing, not a production relayer configuration.

Test a small native-token transfer and return before larger transfers. Verify
the Osmosis-side denom trace using that side's channel ID, then register chain,
asset and IBC metadata in Cosmos Chain Registry. A timeout refund requires a relayer
to submit a nonreceipt proof; time passing alone does not refund the sender.

The initial liquidity plan reserves **1,000 USDC**. No real funds are used here.
Before pool creation, decide the CONGRID amount, pool type, starting price/range,
fees and the canonical Osmosis USDC denomination. Budget OSMO for pool creation,
relaying and operations separately using live chain parameters. Keplr valuation
still requires an active price source and a correct `coinGeckoId` mapping.

See [the Chinese runbook](ibc-zh.md) for a transfer command and more detail.

Production-state migration was also rehearsed against a verified copy at height 70445; see the [rehearsal record](ibc-production-rehearsal-20260905.md) and [offline harness guide](../scripts/ibc/statecopy/README.md). This was offline ABCI replay, with governance and signed IBC flows covered separately by the localnet.
