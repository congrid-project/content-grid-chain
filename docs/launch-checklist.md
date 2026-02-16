# Launch Checklist

> Goal: Advance the current status of "available for grayscale launch" to "available for public production and launch".
> Usage: Tick off items one by one, with the person in charge making the final Go/No-Go decision.

## A. Current verification snapshot (2026-02-11)

- ✅ `go test ./...` all green
- ✅ e2e smoke has passed continuously (recent multiple rounds, including publisher verify → slot → lease → lease-anchor reverify)
- ✅ commit–reveal has entered a stable operation state (including retry and serial submission protection)
- ✅ congrid-site tx output parsing has been strengthened (no longer relies on direct `json.Unmarshal(out)`)
- ✅ Documentation aligned with current implementation (marketplace/verifierd)

> Conclusion: It is currently possible to enter grayscale launch; to go online in public production, the remaining key items are "key strategy + operation and maintenance alarm implementation".

---

## 0. Release information (recommended to fill in this time)

- [ ] Release version number: `v0.1.0-rc1` (recommended)
- [x] Release branch: `main`
- [ ] Change window (time zone/start time/end time) confirmed (`America/Toronto`)
- [ ] Rollback version number confirmed: `4202f66` (current stable baseline recommendation)
- [x] The person in charge (on-chain/backend/operation and maintenance/duty) has been clarified (temporarily the same person in charge)

Current person in charge (interim):
- Person in charge on the chain: `eking`
- Application manager: `eking`
-Operation and maintenance manager: `eking`
- Duty notification: `Telegram @eking (id:6148992071)`

---

## 1. Code and quality access control (P0)

- [x] `go test ./...` All green
- [x] e2e smoke passes continuously (recommended `>= 5` times)
  - [x] publisher verify
  - [x] slot create/list
  - [x] lease escrow
  - [x] lease-anchor reverify
- [ ] Critical changes have been code reviewed (at least 1 reviewer)
- [ ] The dependency version has been frozen (tag or commit pin)
- [x] Product reproducible build (`content-grid-d` / `verifierd` / `congrid-site`)

---

## 2. Chain parameters and governance access control (P0)

### 2.1 Production parameter baseline (v1 recommended)

> If you have a governance parameter template, you can directly copy and overwrite this section.

- [ ] `verifier_bond = 50000000`（50 CONGRID）
- [ ] `min_verifier_count = 3`
- [ ] `verification_ttl = 2000`
- [ ] `commit_window_seconds = 300`
- [ ] `submission_window_seconds = 600`
- [ ] `round_interval_seconds = 3600`
- [ ] `assignment_delay_max_seconds = 300`
- [ ] `cooldown_base_seconds = 604800`
- [ ] `publisher_verification_reward = 1000000`
- [ ] `verifier_verification_reward = 500000`

> Note: e2e fast parameters (such as 12/28/10/2) are only used for local verification and are not recommended for direct use in production.

### 2.2 Parameter constraint verification (required)

- [ ] `commit_window_seconds < submission_window_seconds`
- [ ] `commit_window_seconds < verification_ttl`
- [ ] `assignment_delay_max_seconds <= round_interval_seconds`
- [ ] The above constraints have been verified in the actual configuration of the target network

### 2.3 Governance Process

- [ ] Parameter change process (proposal, approval, execution) has been drilled
- [ ] Parameter snapshot archived (pre/post release)

---

## 3. Key and security access control (P0, final hard blocking)

- [ ] Production key policy document signed and confirmed
- [ ] `congrid-site` Transaction signature path isolation (minimum privileges)
- [ ] signer operating environment (intranet/private plane) confirmed
- [ ] keyring/key permissions (file permissions, sudo policy) locked
- [ ] Key backup and recovery drill completed
- [ ] Key rotation process is executable (scheduling period: `every 90 days` recommended)
- [ ] Key revocation emergency process is executable (trigger conditions and responsible person are clear)

---

## 4. Observation and Alarm (P1)

### 4.1 verifierd indicator threshold (recommended)

- [ ] assignment pull success rate (5min) `>= 99%`
- [ ] commit success rate (15min) `>= 95%`
- [ ] reveal success rate (15min)`>= 90%`
- [ ] Failure cause distribution is visible (sequence mismatch / reveal window / commit window)

### 4.2 On-chain business alarm threshold (recommendation)

- [ ] Newly registered publisher `PENDING > 3h` alarm
- [ ] Cooldown trigger rate increases `> 2x` compared to the 24h average value. Alarm
- [ ] lease `VIOLATED/REFUNDED` ratio `> 10%` (1h window) alarm

### 4.3 Observation infrastructure

- [ ] Log collection and retrieval (node/verifierd/site) is available
- [ ] Alarm notification link (Telegram/Telephone/Duty) verified

---

## 5. Operation, maintenance and rollback (P0)

- [x] `docs/runbook.md` has been filled in according to the current environment
- [ ] Students on duty have practiced startup/restart/rollback
- [ ] Rollback scripts/steps have been drilled (including configuration rollback)
- [ ] Fault classification (SEV1/2/3) and response time limit are clear
- [ ] Post-release observation window (at least 24h) has been scheduled for duty

---

## 6. Grayscale and large-volume strategies (P1)

- [ ] Grayscale range (initial publisher/verifier number) defined
- [ ] Grayscale success criteria defined
- [ ] Volume threshold and pause threshold have been defined
- [ ] One-click pause strategy is available (pause new slot/lease if necessary)

---

## 7. Documentation and external communication

- [x] `docs/marketplace.md` consistent with implementation
- [x] `docs/verifierd.md` consistent with implementation
- [ ] External usage instructions (CLI / API / restrictions) have been released
- [ ] Known limitations and risks disclosed

---

## 8. Go / No-Go summary

- [ ] Go
- [ ] No-Go

**Decision Maker:**
- Person in charge on the chain: `eking`
- Application manager: `eking`
-Operation and maintenance manager: `eking`
- Final approval: `eking`

**Decision Time:** `YYYY-MM-DD HH:mm America/Toronto`
