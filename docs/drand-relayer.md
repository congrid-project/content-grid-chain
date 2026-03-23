# drand-relayer (off-chain)

`drand-relayer` polls drand public API and submits latest beacons on-chain via:

- `tx registry submit-drand-beacon`

This is required when `registry` params enable drand mixing for assignment randomness.

## Config

Copy and edit:

```bash
cp offchain/drandrelayer/config.example.json offchain/drandrelayer/config.json
```

Key fields:
- `drand_api_base_url`: default `https://api.drand.sh`
- `drand_chain_hash`: drand network chain hash (quicknet by default in example)
- `poll_interval_seconds`: how often to poll drand
- `submit.*`: chain tx settings for `content-grid-d`

For unattended container deployments with `submit.keyring_backend=file`, set `submit.keyring_passphrase_env` to the environment variable name that contains the keyring passphrase.

## Run

One-time sync:

```bash
go run ./offchain/drandrelayer --config offchain/drandrelayer/config.json --once
```

Long-running mode:

```bash
go run ./offchain/drandrelayer --config offchain/drandrelayer/config.json
```

## Notes

- Relayer only submits when drand latest round is newer than on-chain latest round.
- On-chain `MsgSubmitDrandBeacon` enforces beacon validation (including signature verification when drand mode is enabled/strict).
