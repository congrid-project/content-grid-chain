# drand Randomness

drand delivery is embedded in `verifierd`; there is no standalone `drand-relayer`. Multiple verifiers may deliver beacons, while chain state defines the one acceptable drand round and the first successful submission wins.

## On-chain rules

When `registry.params.drand_enabled` is enabled:

1. The chain maps the next Content Grid verification round to one fixed drand round.
2. `MsgSubmitDrandBeacon` rejects historical, future, and duplicate rounds.
3. The chain verifies the BLS signature with the configured scheme/distributed public key and checks `randomness = SHA256(signature)`.
4. Assignment creation waits for the required beacon and never falls back to block-hash-only randomness.
5. Once submitted, `EndBlock` mixes the beacon with the on-chain anchor and creates assignments deterministically.

Mapping:

```text
latest_allowed_time = content_round_start - drand_round_offset_seconds
required_drand_round = floor((latest_allowed_time - drand_genesis_time_unix) / drand_period_seconds) + 1
```

Query the current requirement:

```bash
content-grid-d query registry drand-requirement
```

The default metadata targets drand quicknet (`bls-unchained-g1-rfc9380`, chain hash `52db9ba70e0cc0f6eaf7803dd07447a1f5477735fd3f661792ba94600c84e971`, genesis `1692803367`, period `3s`, offset `60s`). `drand_strict_mode` remains in legacy JSON for compatibility; enabling drand now always implies strict delivery.

See the official [quicknet announcement](https://docs.drand.love/blog/2023/10/16/quicknet-is-live/) for its chain metadata and the [drand HTTP API](https://docs.drand.love/developer/http-api/) for relay endpoints.

## verifierd configuration

```json
{
  "drand": {
    "disabled": false,
    "api_base_url": "https://api.drand.sh",
    "request_timeout_seconds": 10,
    "fee_granter": ""
  }
}
```

On every normal poll, `verifierd` queries `DrandRequirement`. It fetches `/{chain-hash}/public/{round}` only after the required round's publication time and only while that exact requirement is unfilled. It never submits a `latest` beacon.

To keep drand delivery from spending the verifier account's balance, grant a message-restricted Cosmos SDK fee allowance for `/contentgrid.registry.v1.MsgSubmitDrandBeacon` and set the grantor address in `drand.fee_granter` (`CONGRID_DRAND_FEE_GRANTER` in Docker). Limit the allowance by grantee, expiry, and spend cap.

Do not make arbitrary beacon transactions unconditionally fee-free: invalid BLS proofs would still consume verification CPU.

## Migration from the removed component

1. Stop and remove the old `drand-relayer` service, signer, and health port.
2. Add the `drand` section to `verifierd` configuration.
3. Configure a fee grant if delivery should be sponsored.
4. Upgrade an existing chain with the fixed `drand-strict-v2` software plan; follow [`upgrade-drand-strict-v2.md`](upgrade-drand-strict-v2.md).
5. Start `verifierd` and inspect the `drand_*` fields in `/readyz`.
