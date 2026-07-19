package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

type drandHTTPResponse struct {
	Round      uint64 `json:"round"`
	Randomness string `json:"randomness"`
	Signature  string `json:"signature"`
}

// relayRequiredDrand fetches and submits only the exact drand round requested
// by chain state. It is intentionally part of verifierd so the deployment does
// not need a separately funded relayer process.
func (a *Agent) relayRequiredDrand(ctx context.Context) error {
	if a.Cfg.Drand.Disabled {
		return nil
	}
	requirement, err := a.Chain.DrandRequirement(ctx)
	if err != nil {
		return fmt.Errorf("query drand requirement: %w", err)
	}
	if a.Health != nil {
		a.Health.recordDrandRequirement(requirement)
	}
	if !requirement.GetEnabled() || !requirement.GetPending() || requirement.GetSubmitted() || requirement.GetRequiredDrandRound() == 0 {
		return nil
	}
	// The required round can be announced well before drand publishes it. Avoid
	// turning an expected wait into repeated HTTP 404/readiness failures.
	if time.Now().UTC().Unix() < requirement.GetRequiredBeaconUnix() {
		return nil
	}

	active, err := a.Chain.ActiveVerifierRelayCandidates(ctx)
	if err != nil {
		return fmt.Errorf("query active verifiers for drand relay: %w", err)
	}
	rank, total, err := drandRelayRank(requirement.GetRequiredDrandRound(), a.Cfg.VerifierAddress, active)
	if err != nil {
		return err
	}
	delaySeconds := drandRelayDelaySeconds(rank, a.Cfg.Drand.RelayStaggerSec, a.Cfg.Drand.RelayMaxDelaySec)
	notBeforeUnix := requirement.GetRequiredBeaconUnix() + delaySeconds
	if a.Health != nil {
		a.Health.recordDrandRelaySchedule(rank, total, notBeforeUnix)
	}
	if time.Now().UTC().Unix() < notBeforeUnix {
		return nil
	}

	beacon, err := a.fetchDrandRound(ctx, requirement.GetDrandChainHash(), requirement.GetRequiredDrandRound())
	if err != nil {
		if a.Health != nil {
			a.Health.recordDrandSubmission(requirement.GetRequiredDrandRound(), err)
		}
		return fmt.Errorf("fetch required drand round %d: %w", requirement.GetRequiredDrandRound(), err)
	}
	if err := a.submitDrandBeacon(ctx, beacon); err != nil {
		// Multiple verifierd instances intentionally race. If another one filled
		// the requirement, the local failed transaction is no longer an error.
		latest, queryErr := a.Chain.DrandRequirement(ctx)
		if queryErr == nil && (!latest.GetPending() || latest.GetSubmitted() || latest.GetRequiredDrandRound() != requirement.GetRequiredDrandRound()) {
			if a.Health != nil {
				a.Health.recordDrandSubmission(requirement.GetRequiredDrandRound(), nil)
			}
			return nil
		}
		if a.Health != nil {
			a.Health.recordDrandSubmission(requirement.GetRequiredDrandRound(), err)
		}
		return err
	}

	if a.Health != nil {
		a.Health.recordDrandSubmission(requirement.GetRequiredDrandRound(), nil)
	}
	log.Printf("verifierd: submitted required drand beacon round=%d content_round=%d relay_rank=%d relay_total=%d", requirement.GetRequiredDrandRound(), requirement.GetRoundStartUnix(), rank, total)
	return nil
}

func drandRelayDelaySeconds(rank int, staggerSeconds, maxDelaySeconds int64) int64 {
	if rank <= 0 || staggerSeconds <= 0 {
		return 0
	}
	delay := int64(rank) * staggerSeconds
	if maxDelaySeconds > 0 && delay > maxDelaySeconds {
		return maxDelaySeconds
	}
	return delay
}

// drandRelayRank chooses a bond-weighted deterministic primary for each round,
// then deterministically orders the remaining active verifiers as fallbacks.
// Ranks whose delay reaches relay_max_delay_seconds may race, deliberately
// preferring liveness over fee efficiency in a degraded network.
func drandRelayRank(round uint64, verifier string, active []drandRelayCandidate) (int, int, error) {
	verifier = strings.TrimSpace(verifier)
	if round == 0 || verifier == "" {
		return 0, 0, fmt.Errorf("drand relay round and verifier required")
	}

	type candidate struct {
		address string
		weight  *big.Int
	}
	seen := make(map[string]struct{}, len(active))
	candidates := make([]candidate, 0, len(active))
	verifierFound := false
	for _, entry := range active {
		address := strings.TrimSpace(entry.Address)
		if address == "" {
			continue
		}
		if _, found := seen[address]; found {
			continue
		}
		seen[address] = struct{}{}
		if address == verifier {
			verifierFound = true
		}
		weight, ok := new(big.Int).SetString(strings.TrimSpace(entry.BondAmount), 10)
		if !ok || weight.Sign() <= 0 {
			weight = big.NewInt(1)
		}
		candidates = append(candidates, candidate{address: address, weight: weight})
	}
	if len(candidates) == 0 {
		return 0, 0, fmt.Errorf("no active verifiers available for drand relay")
	}
	if !verifierFound {
		return 0, len(candidates), fmt.Errorf("verifier %s is not active and cannot relay drand", verifier)
	}
	totalCandidates := len(candidates)
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].address < candidates[j].address })
	var roundBytes [8]byte
	binary.BigEndian.PutUint64(roundBytes[:], round)
	primarySeed := append([]byte("content-grid/drand-relay/v2/primary"), roundBytes[:]...)
	primaryHash := sha256.Sum256(primarySeed)
	totalWeight := big.NewInt(0)
	for _, entry := range candidates {
		totalWeight.Add(totalWeight, entry.weight)
	}
	pick := new(big.Int).SetBytes(primaryHash[:])
	pick.Mod(pick, totalWeight)
	running := big.NewInt(0)
	primaryIndex := len(candidates) - 1
	for i, entry := range candidates {
		running.Add(running, entry.weight)
		if running.Cmp(pick) == 1 {
			primaryIndex = i
			break
		}
	}
	if candidates[primaryIndex].address == verifier {
		return 0, totalCandidates, nil
	}

	type fallbackCandidate struct {
		address string
		score   [sha256.Size]byte
	}
	fallbacks := make([]fallbackCandidate, 0, len(candidates)-1)
	for i, entry := range candidates {
		if i == primaryIndex {
			continue
		}
		material := append([]byte("content-grid/drand-relay/v2/fallback"), roundBytes[:]...)
		material = append(material, entry.address...)
		fallbacks = append(fallbacks, fallbackCandidate{address: entry.address, score: sha256.Sum256(material)})
	}
	sort.Slice(fallbacks, func(i, j int) bool {
		if cmp := bytes.Compare(fallbacks[i].score[:], fallbacks[j].score[:]); cmp != 0 {
			return cmp < 0
		}
		return fallbacks[i].address < fallbacks[j].address
	})
	for i, entry := range fallbacks {
		if entry.address == verifier {
			return i + 1, totalCandidates, nil
		}
	}
	return 0, totalCandidates, fmt.Errorf("verifier %s relay rank unavailable", verifier)
}

func (a *Agent) fetchDrandRound(ctx context.Context, chainHash string, round uint64) (*drandHTTPResponse, error) {
	base := strings.TrimRight(strings.TrimSpace(a.Cfg.Drand.APIBaseURL), "/")
	chainHash = strings.ToLower(strings.TrimSpace(chainHash))
	if base == "" || chainHash == "" || round == 0 {
		return nil, fmt.Errorf("drand endpoint, chain hash, and round are required")
	}
	u := base + "/" + chainHash + "/public/" + strconv.FormatUint(round, 10)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	client := a.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: time.Duration(a.Cfg.Drand.RequestTimeoutSec) * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("drand api status=%d", resp.StatusCode)
	}

	var out drandHTTPResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	out.Randomness = strings.ToLower(strings.TrimSpace(out.Randomness))
	out.Signature = strings.ToLower(strings.TrimSpace(out.Signature))
	if out.Round != round {
		return nil, fmt.Errorf("drand response round mismatch: got %d want %d", out.Round, round)
	}
	if out.Randomness == "" || out.Signature == "" {
		return nil, fmt.Errorf("drand payload missing randomness or signature")
	}
	return &out, nil
}

func (a *Agent) submitDrandBeacon(ctx context.Context, beacon *drandHTTPResponse) error {
	if beacon == nil {
		return fmt.Errorf("drand beacon required")
	}
	args := []string{
		"tx", "registry", "submit-drand-beacon",
		"--round", strconv.FormatUint(beacon.Round, 10),
		"--randomness-hex", beacon.Randomness,
		"--signature-hex", beacon.Signature,
		"--from", a.Cfg.Submit.From,
		"--chain-id", a.Cfg.Submit.ChainID,
		"--node", a.Cfg.Submit.Node,
		"--keyring-backend", a.Cfg.Submit.KeyringBackend,
		"--broadcast-mode", a.Cfg.Submit.BroadcastMode,
		"--output", "json",
	}
	args = a.appendSubmitFlags(args, a.Cfg.Drand.FeeGranter)

	a.txMu.Lock()
	defer a.txMu.Unlock()
	out, txHash, err := a.execTxWithRetry(ctx, args, 1)
	if err != nil {
		return fmt.Errorf("drand beacon tx failed: %w: %s", err, string(out))
	}
	if err := a.waitTxIncluded(ctx, txHash); err != nil {
		return fmt.Errorf("drand beacon wait failed: %w", err)
	}
	return nil
}

func (a *Agent) appendSubmitFlags(args []string, feeGranterOverride ...string) []string {
	if a.Cfg.Submit.KeyringDir != "" {
		args = append(args, "--keyring-dir", a.Cfg.Submit.KeyringDir)
	}
	if a.Cfg.Submit.Home != "" {
		args = append(args, "--home", a.Cfg.Submit.Home)
	}
	if a.Cfg.Submit.Gas != "" {
		args = append(args, "--gas", a.Cfg.Submit.Gas)
	}
	if a.Cfg.Submit.GasAdjustment > 0 {
		args = append(args, "--gas-adjustment", strconv.FormatFloat(a.Cfg.Submit.GasAdjustment, 'f', -1, 64))
	}
	if a.Cfg.Submit.Fees != "" {
		args = append(args, "--fees", a.Cfg.Submit.Fees)
	}
	if a.Cfg.Submit.GasPrices != "" {
		args = append(args, "--gas-prices", a.Cfg.Submit.GasPrices)
	}
	feeGranter := a.Cfg.Submit.FeeGranter
	if len(feeGranterOverride) > 0 && strings.TrimSpace(feeGranterOverride[0]) != "" {
		feeGranter = strings.TrimSpace(feeGranterOverride[0])
	}
	if feeGranter != "" {
		args = append(args, "--fee-granter", feeGranter)
	}
	if a.Cfg.Submit.Yes {
		args = append(args, "--yes")
	}
	return args
}
