package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

type drandHTTPResponse struct {
	Round      uint64 `json:"round"`
	Randomness string `json:"randomness"`
	Signature  string `json:"signature"`
}

type Relayer struct {
	Cfg        Config
	Chain      *ChainClient
	HTTPClient *http.Client
	mu         sync.Mutex
}

func (r *Relayer) RunOnce(ctx context.Context) error {
	onChainRound, err := r.Chain.LatestDrandRound(ctx)
	if err != nil {
		return fmt.Errorf("query latest on-chain drand beacon: %w", err)
	}

	latest, err := r.fetchLatestDrand(ctx)
	if err != nil {
		return fmt.Errorf("fetch latest drand: %w", err)
	}
	if latest.Round == 0 {
		return fmt.Errorf("drand latest response missing round")
	}
	if latest.Round <= onChainRound {
		return nil
	}

	if err := r.submitDrandBeacon(ctx, latest); err != nil {
		return err
	}
	log.Printf("drand-relayer: submitted beacon round=%d", latest.Round)
	return nil
}

func (r *Relayer) fetchLatestDrand(ctx context.Context) (*drandHTTPResponse, error) {
	base := strings.TrimRight(r.Cfg.DrandAPIBaseURL, "/")
	u := base + "/public/latest?chain-hash=" + url.QueryEscape(r.Cfg.DrandChainHash)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := r.HTTPClient.Do(req)
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
	out.Randomness = strings.TrimSpace(strings.ToLower(out.Randomness))
	out.Signature = strings.TrimSpace(strings.ToLower(out.Signature))
	if out.Round == 0 || out.Randomness == "" || out.Signature == "" {
		return nil, fmt.Errorf("invalid drand payload: missing required fields")
	}
	return &out, nil
}

func (r *Relayer) submitDrandBeacon(ctx context.Context, b *drandHTTPResponse) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	args := []string{
		"tx", "registry", "submit-drand-beacon",
		"--round", strconv.FormatUint(b.Round, 10),
		"--randomness-hex", b.Randomness,
		"--signature-hex", b.Signature,
		"--from", r.Cfg.Submit.From,
		"--chain-id", r.Cfg.Submit.ChainID,
		"--node", r.Cfg.Submit.Node,
		"--keyring-backend", r.Cfg.Submit.KeyringBackend,
		"--broadcast-mode", r.Cfg.Submit.BroadcastMode,
		"--output", "json",
	}
	if r.Cfg.Submit.KeyringDir != "" {
		args = append(args, "--keyring-dir", r.Cfg.Submit.KeyringDir)
	}
	if r.Cfg.Submit.Home != "" {
		args = append(args, "--home", r.Cfg.Submit.Home)
	}
	if r.Cfg.Submit.Gas != "" {
		args = append(args, "--gas", r.Cfg.Submit.Gas)
	}
	if r.Cfg.Submit.GasAdjustment > 0 {
		args = append(args, "--gas-adjustment", strconv.FormatFloat(r.Cfg.Submit.GasAdjustment, 'f', -1, 64))
	}
	if r.Cfg.Submit.Fees != "" {
		args = append(args, "--fees", r.Cfg.Submit.Fees)
	}
	if r.Cfg.Submit.GasPrices != "" {
		args = append(args, "--gas-prices", r.Cfg.Submit.GasPrices)
	}
	if r.Cfg.Submit.Yes {
		args = append(args, "--yes")
	}

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		cmd := exec.CommandContext(ctx, r.Cfg.Submit.Binary, args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			lastErr = fmt.Errorf("submit tx failed: %w: %s", err, string(out))
			if isRetriableTxError(out) {
				time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
				continue
			}
			return lastErr
		}
		txHash, err := ensureTxSuccess(out)
		if err != nil {
			lastErr = err
			if isRetriableTxError(out) {
				time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
				continue
			}
			return err
		}
		if err := r.waitTxIncluded(ctx, txHash); err != nil {
			lastErr = err
			if isRetriableTxError([]byte(err.Error())) {
				time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
				continue
			}
			return err
		}
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("submit drand beacon failed after retries")
}

func isRetriableTxError(out []byte) bool {
	lower := strings.ToLower(string(out))
	if strings.Contains(lower, "account sequence mismatch") {
		return true
	}
	if strings.Contains(lower, "incorrect account sequence") {
		return true
	}
	if strings.Contains(lower, "tx already exists in cache") {
		return true
	}
	if strings.Contains(lower, "timed out") {
		return true
	}
	return false
}

func ensureTxSuccess(out []byte) (string, error) {
	raw := string(out)
	lines := strings.Split(raw, "\n")

	var lastJSON string
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "{") && strings.HasSuffix(line, "}") {
			lastJSON = line
			break
		}
	}
	if lastJSON == "" {
		return "", fmt.Errorf("missing json tx response: %s", raw)
	}

	var resp struct {
		Code   int    `json:"code"`
		RawLog string `json:"raw_log"`
		TxHash string `json:"txhash"`
	}
	if err := json.Unmarshal([]byte(lastJSON), &resp); err != nil {
		return "", fmt.Errorf("decode tx response: %w: %s", err, raw)
	}
	if resp.Code != 0 {
		return "", fmt.Errorf("code=%d raw_log=%s", resp.Code, resp.RawLog)
	}
	txHash := strings.TrimSpace(resp.TxHash)
	if txHash == "" {
		return "", fmt.Errorf("missing txhash in response")
	}
	return txHash, nil
}

func (r *Relayer) waitTxIncluded(ctx context.Context, txHash string) error {
	if strings.TrimSpace(txHash) == "" {
		return fmt.Errorf("txhash required")
	}
	mode := strings.ToLower(strings.TrimSpace(r.Cfg.Submit.BroadcastMode))
	if mode == "block" {
		return nil
	}

	args := []string{
		"query", "wait-tx", txHash,
		"--node", r.Cfg.Submit.Node,
		"--timeout", "30s",
		"-o", "json",
	}
	if r.Cfg.Submit.Home != "" {
		args = append(args, "--home", r.Cfg.Submit.Home)
	}

	cmd := exec.CommandContext(ctx, r.Cfg.Submit.Binary, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("wait-tx failed: %w: %s", err, string(out))
	}
	return nil
}
