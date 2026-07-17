package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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
	log.Printf("verifierd: submitted required drand beacon round=%d content_round=%d", requirement.GetRequiredDrandRound(), requirement.GetRoundStartUnix())
	return nil
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
