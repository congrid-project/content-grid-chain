package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	registryoffchain "content-grid-chain/offchain/registry"
	registrypb "content-grid-chain/x/registry/typespb"
)

type Agent struct {
	Cfg      Config
	Chain    *ChainClient
	Verifier registryoffchain.HTTPContentVerifier

	mu       sync.Mutex
	inFlight map[string]struct{}
	wg       sync.WaitGroup
	txMu     sync.Mutex
}

func (a *Agent) PollOnce(ctx context.Context) error {
	assignments, err := a.Chain.VerifierAssignments(ctx, a.Cfg.VerifierAddress)
	if err != nil {
		return err
	}
	now := time.Now().Unix()

	for _, entry := range assignments {
		assignment := entry.GetAssignment()
		if assignment == nil || assignment.GetFinalized() {
			continue
		}
		if entry.GetSubmission() != nil {
			continue
		}
		if now > assignment.GetDeadlineUnix() {
			continue
		}
		if err := a.validateAssignmentDeterministic(ctx, assignment); err != nil {
			log.Printf("assignment %s/%d skipped: deterministic validation failed: %v", assignment.GetDomain(), assignment.GetRoundStartUnix(), err)
			continue
		}
		if a.Cfg.CommitWindowSeconds > 0 {
			commitDeadline := assignment.GetStartAtUnix() + a.Cfg.CommitWindowSeconds
			if now > commitDeadline {
				continue
			}
		}
		key := assignmentKey(assignment)
		if a.isInFlight(key) {
			continue
		}
		a.markInFlight(key)
		a.wg.Add(1)
		go func(assignment *registrypb.PublisherVerificationAssignment, key string) {
			defer a.wg.Done()
			a.runAssignment(ctx, assignment, key)
		}(assignment, key)
	}
	return nil
}

// Wait blocks until all in-flight assignment runs launched by PollOnce complete.
func (a *Agent) Wait() {
	a.wg.Wait()
}

func (a *Agent) runAssignment(ctx context.Context, assignment *registrypb.PublisherVerificationAssignment, key string) {
	defer a.unmarkInFlight(key)

	startAt := time.Unix(assignment.GetStartAtUnix(), 0)
	if delay := time.Until(startAt); delay > 0 {
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
	if time.Now().Unix() > assignment.GetDeadlineUnix() {
		log.Printf("missed assignment %s (deadline passed)", key)
		return
	}

	owner, err := a.Chain.PublisherOwner(ctx, assignment.GetDomain())
	if err != nil {
		log.Printf("assignment %s: failed to fetch owner: %v", key, err)
		return
	}
	if owner == "" {
		log.Printf("assignment %s: owner not found", key)
		return
	}

	timeout := time.Until(time.Unix(assignment.GetDeadlineUnix(), 0))
	if timeout <= 0 {
		log.Printf("assignment %s: deadline passed before verification", key)
		return
	}
	verifyCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	leaseExpectations, leaseErr := a.Chain.ActiveLeaseExpectationsForDomain(verifyCtx, assignment.GetDomain())
	if leaseErr != nil {
		log.Printf("assignment %s: failed to fetch active leases: %v", key, leaseErr)
		err = a.Verifier.Verify(verifyCtx, assignment.GetDomain(), owner)
	} else {
		err = a.Verifier.VerifyWithLeases(verifyCtx, assignment.GetDomain(), owner, leaseExpectations)
	}
	passed := err == nil
	if err != nil {
		log.Printf("assignment %s: verification failed: %v", key, err)
	}

	// Similar-site verification (optional): compute expected top-15 and compare to homepage.
	var (
		observedHash string
	)
	if passed && strings.TrimSpace(a.Cfg.IndexerdBaseURL) != "" {
		// Fetch homepage HTML for parsing similar domains.
		page, ferr := fetchHomepageHTML(verifyCtx, a.Cfg.VerifyScheme, assignment.GetDomain())
		if ferr != nil {
			log.Printf("assignment %s: fetch homepage for similar parse failed: %v", key, ferr)
		} else {
			observed, _ := parseObservedSimilarDomains(page)
			obsHash := sha256HexOfSet(observed)
			expected, _, eerr := fetchExpectedSimilar(ctx, a.Cfg.IndexerdBaseURL, assignment.GetDomain())
			if eerr != nil {
				log.Printf("assignment %s: expected similar fetch failed: %v", key, eerr)
			} else {
				match := overlapCount(observed, expected)
				observedHash = obsHash

				if match < similarOverlapRequired {
					passed = false
					log.Printf("assignment %s: similar overlap insufficient: matched=%d expected=%d", key, match, similarTopN)
				}
			}
		}
	}

	evidenceHash := strings.TrimSpace(observedHash)
	commitDeadlineUnix := assignment.GetStartAtUnix() + a.Cfg.CommitWindowSeconds
	if commitDeadlineUnix >= assignment.GetDeadlineUnix() {
		log.Printf("assignment %s: commit window (%d) overlaps deadline (%d)", key, commitDeadlineUnix, assignment.GetDeadlineUnix())
		return
	}
	if time.Now().Unix() > commitDeadlineUnix {
		log.Printf("assignment %s: commit window closed (deadline %d)", key, commitDeadlineUnix)
		return
	}

	nonce, err := generateNonce(16)
	if err != nil {
		log.Printf("assignment %s: failed to generate nonce: %v", key, err)
		return
	}
	commitHash := registrypb.ComputeVerificationCommitHash(assignment.GetDomain(), assignment.GetRoundStartUnix(), a.Cfg.VerifierAddress, passed, evidenceHash, nonce)
	if err := a.submitCommit(ctx, assignment, commitHash); err != nil {
		log.Printf("assignment %s: commit failed: %v", key, err)
		return
	}
	log.Printf("assignment %s: submitted commit", key)

	revealAt := time.Unix(commitDeadlineUnix+6, 0)
	if delay := time.Until(revealAt); delay > 0 {
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
	if time.Now().Unix() > assignment.GetDeadlineUnix() {
		log.Printf("assignment %s: missed reveal window (deadline %d)", key, assignment.GetDeadlineUnix())
		return
	}
	if err := a.submitReveal(ctx, assignment, passed, evidenceHash, nonce); err != nil {
		log.Printf("assignment %s: reveal failed: %v", key, err)
		return
	}
	log.Printf("assignment %s: revealed result (passed=%t)", key, passed)
}

func (a *Agent) submitCommit(ctx context.Context, assignment *registrypb.PublisherVerificationAssignment, commitHash string) error {
	args := []string{
		"verifier", "commit", assignment.GetDomain(),
		"--round-start", strconv.FormatInt(assignment.GetRoundStartUnix(), 10),
		"--commit-hash", commitHash,
		"--from", a.Cfg.Submit.From,
		"--chain-id", a.Cfg.Submit.ChainID,
		"--node", a.Cfg.Submit.Node,
		"--keyring-backend", a.Cfg.Submit.KeyringBackend,
		"--broadcast-mode", a.Cfg.Submit.BroadcastMode,
		"--output", "json",
	}
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
		args = append(args, "--gas-adjustment", fmt.Sprintf("%.2f", a.Cfg.Submit.GasAdjustment))
	}
	if a.Cfg.Submit.Fees != "" {
		args = append(args, "--fees", a.Cfg.Submit.Fees)
	}
	if a.Cfg.Submit.GasPrices != "" {
		args = append(args, "--gas-prices", a.Cfg.Submit.GasPrices)
	}
	if a.Cfg.Submit.Yes {
		args = append(args, "-y")
	}

	a.txMu.Lock()
	defer a.txMu.Unlock()

	out, txHash, err := a.execTxWithRetry(ctx, args, 3)
	if err != nil {
		return fmt.Errorf("commit tx failed: %w: %s", err, string(out))
	}
	if err := a.waitTxIncluded(ctx, txHash); err != nil {
		return fmt.Errorf("commit wait failed: %w", err)
	}
	return nil
}

func (a *Agent) submitReveal(ctx context.Context, assignment *registrypb.PublisherVerificationAssignment, passed bool, evidenceHash, nonce string) error {
	args := []string{
		"verifier", "reveal", assignment.GetDomain(),
		"--round-start", strconv.FormatInt(assignment.GetRoundStartUnix(), 10),
		"--nonce", nonce,
		"--from", a.Cfg.Submit.From,
		"--chain-id", a.Cfg.Submit.ChainID,
		"--node", a.Cfg.Submit.Node,
		"--keyring-backend", a.Cfg.Submit.KeyringBackend,
		"--broadcast-mode", a.Cfg.Submit.BroadcastMode,
		"--output", "json",
	}
	if passed {
		args = append(args, "--passed")
	} else {
		args = append(args, "--failed")
	}
	if strings.TrimSpace(evidenceHash) != "" {
		args = append(args, "--evidence-hash", evidenceHash)
	}
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
		args = append(args, "--gas-adjustment", fmt.Sprintf("%.2f", a.Cfg.Submit.GasAdjustment))
	}
	if a.Cfg.Submit.Fees != "" {
		args = append(args, "--fees", a.Cfg.Submit.Fees)
	}
	if a.Cfg.Submit.GasPrices != "" {
		args = append(args, "--gas-prices", a.Cfg.Submit.GasPrices)
	}
	if a.Cfg.Submit.Yes {
		args = append(args, "-y")
	}

	a.txMu.Lock()
	defer a.txMu.Unlock()

	out, txHash, err := a.execTxWithRetry(ctx, args, 10)
	if err != nil {
		return fmt.Errorf("reveal tx failed: %w: %s", err, string(out))
	}
	if err := a.waitTxIncluded(ctx, txHash); err != nil {
		return fmt.Errorf("reveal wait failed: %w", err)
	}
	return nil
}

func (a *Agent) execTxWithRetry(ctx context.Context, args []string, maxAttempts int) ([]byte, string, error) {
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	var lastOut []byte
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		cmd := exec.CommandContext(ctx, a.Cfg.Submit.Binary, args...)
		out, err := cmd.CombinedOutput()
		lastOut = out
		raw := string(out)
		if err == nil {
			txHash, perr := ensureTxSuccess(out)
			if perr == nil {
				return out, txHash, nil
			}
			lastErr = perr
			if !isRetriableTxOutput(raw) {
				return out, "", perr
			}
		} else {
			lastErr = fmt.Errorf("tx failed: %w", err)
			if !isRetriableTxOutput(raw) {
				return out, "", fmt.Errorf("%w: %s", lastErr, raw)
			}
		}

		if attempt < maxAttempts-1 {
			time.Sleep(1200 * time.Millisecond)
			continue
		}
	}
	return lastOut, "", lastErr
}

func isRetriableTxOutput(raw string) bool {
	lower := strings.ToLower(raw)
	if strings.Contains(lower, "account sequence mismatch") {
		return true
	}
	if strings.Contains(lower, "reveal window not open") {
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

func (a *Agent) waitTxIncluded(ctx context.Context, txHash string) error {
	if strings.TrimSpace(txHash) == "" {
		return fmt.Errorf("txhash required")
	}
	mode := strings.ToLower(strings.TrimSpace(a.Cfg.Submit.BroadcastMode))
	if mode == "block" {
		return nil
	}

	args := []string{
		"query", "wait-tx", txHash,
		"--node", a.Cfg.Submit.Node,
		"--timeout", "30s",
		"-o", "json",
	}
	if a.Cfg.Submit.Home != "" {
		args = append(args, "--home", a.Cfg.Submit.Home)
	}

	cmd := exec.CommandContext(ctx, a.Cfg.Submit.Binary, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("wait-tx failed: %w: %s", err, string(out))
	}
	return nil
}

func generateNonce(size int) (string, error) {
	if size <= 0 {
		return "", fmt.Errorf("nonce size must be positive")
	}
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func (a *Agent) validateAssignmentDeterministic(ctx context.Context, assignment *registrypb.PublisherVerificationAssignment) error {
	if assignment == nil || a.Cfg.DisableAssignmentCheck {
		return nil
	}
	roundStart := assignment.GetRoundStartUnix()
	if roundStart <= 0 {
		return fmt.Errorf("invalid round_start_unix")
	}

	interval := a.Cfg.RoundIntervalSeconds
	if interval <= 0 {
		interval = 3600
	}
	delayMax := a.Cfg.AssignmentDelayMaxSeconds
	if delayMax <= 0 {
		delayMax = interval
	}

	var roundSeed [32]byte
	seedFromChain := false
	if meta, err := a.Chain.RoundMeta(ctx, roundStart); err == nil && meta != nil {
		if meta.GetRoundIntervalSeconds() > 0 {
			interval = meta.GetRoundIntervalSeconds()
		}
		if meta.GetAssignmentDelayMaxSeconds() > 0 {
			delayMax = meta.GetAssignmentDelayMaxSeconds()
		}
		if meta.GetVerifierSetSize() > 0 && int(meta.GetVerifierSetSize()) < len(assignment.GetVerifiers()) {
			return fmt.Errorf("assignment verifier count exceeds round verifier set size: set=%d assignment=%d", meta.GetVerifierSetSize(), len(assignment.GetVerifiers()))
		}
		seedHex := strings.TrimSpace(meta.GetSeedHex())
		if seedHex != "" {
			seedBytes, decErr := hex.DecodeString(seedHex)
			if decErr == nil && len(seedBytes) == 32 {
				copy(roundSeed[:], seedBytes)
				seedFromChain = true
			}
		}
		anchorHashHex := strings.TrimSpace(meta.GetAnchorHashHex())
		chainID := strings.TrimSpace(a.Cfg.Submit.ChainID)
		if seedFromChain && anchorHashHex != "" && chainID != "" {
			anchorHash, decErr := hex.DecodeString(anchorHashHex)
			if decErr == nil {
				expectedSeed := registrypb.ComputeRoundSeedWithAnchor(chainID, roundStart, meta.GetAnchorHeight(), anchorHash)
				drandRandomnessHex := strings.TrimSpace(meta.GetDrandRandomnessHex())
				if meta.GetDrandRound() > 0 && drandRandomnessHex != "" {
					drandRandomness, drandErr := hex.DecodeString(drandRandomnessHex)
					if drandErr != nil {
						return fmt.Errorf("invalid drand_randomness_hex in round meta")
					}
					expectedSeed = registrypb.ComputeRoundSeedWithDrand(chainID, roundStart, meta.GetAnchorHeight(), anchorHash, meta.GetDrandRound(), drandRandomness)
				}
				if expectedSeed != roundSeed {
					return fmt.Errorf("round seed mismatch against anchor/drand material")
				}
			}
		}
	}
	if delayMax <= 0 || delayMax > interval {
		delayMax = interval
	}

	if !seedFromChain {
		chainID := strings.TrimSpace(a.Cfg.Submit.ChainID)
		if chainID == "" {
			return fmt.Errorf("submit.chain_id required for deterministic assignment validation")
		}
		roundSeed = registrypb.ComputeRoundSeed(chainID, roundStart)
	}

	expectedStart := registrypb.ComputeAssignmentStartAtUnix(roundSeed, assignment.GetDomain(), roundStart, interval, delayMax)
	if expectedStart != assignment.GetStartAtUnix() {
		return fmt.Errorf("start_at mismatch expected=%d got=%d", expectedStart, assignment.GetStartAtUnix())
	}

	if interval >= 3600 {
		offset := assignment.GetStartAtUnix() - roundStart
		if offset%60 != 0 {
			return fmt.Errorf("hourly round requires minute-aligned start_at (offset=%d)", offset)
		}
	}

	active, err := a.Chain.ActiveVerifierAddresses(ctx)
	if err != nil {
		return nil
	}
	if len(active) == 0 || len(assignment.GetVerifiers()) == 0 {
		return nil
	}
	seedMaterial := append(append([]byte{}, roundSeed[:]...), []byte(strings.TrimSpace(strings.ToLower(assignment.GetDomain())))...)
	expected := selectDeterministicAddrs(active, len(assignment.GetVerifiers()), seedMaterial)
	sort.Strings(expected)
	got := append([]string(nil), assignment.GetVerifiers()...)
	sort.Strings(got)
	if !stringSlicesEqual(expected, got) {
		log.Printf("assignment %s/%d: verifier set mismatch (non-fatal, possible suspension delta); expected=%v got=%v", assignment.GetDomain(), roundStart, expected, got)
	}
	return nil
}

func selectDeterministicAddrs(candidates []string, k int, seed []byte) []string {
	if k <= 0 || len(candidates) == 0 {
		return nil
	}
	if len(candidates) <= k {
		out := append([]string(nil), candidates...)
		return out
	}
	type scored struct {
		addr string
		hash [32]byte
	}
	scoredList := make([]scored, 0, len(candidates))
	for _, addr := range candidates {
		buf := make([]byte, 0, len(seed)+len(addr))
		buf = append(buf, seed...)
		buf = append(buf, []byte(addr)...)
		h := sha256.Sum256(buf)
		scoredList = append(scoredList, scored{addr: addr, hash: h})
	}
	sort.Slice(scoredList, func(i, j int) bool {
		cmp := bytes.Compare(scoredList[i].hash[:], scoredList[j].hash[:])
		if cmp == 0 {
			return scoredList[i].addr < scoredList[j].addr
		}
		return cmp < 0
	})
	out := make([]string, 0, k)
	for i := 0; i < k; i++ {
		out = append(out, scoredList[i].addr)
	}
	return out
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (a *Agent) markInFlight(key string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.inFlight == nil {
		a.inFlight = make(map[string]struct{})
	}
	a.inFlight[key] = struct{}{}
}

func (a *Agent) unmarkInFlight(key string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.inFlight, key)
}

func (a *Agent) isInFlight(key string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, ok := a.inFlight[key]
	return ok
}

func assignmentKey(assignment *registrypb.PublisherVerificationAssignment) string {
	return fmt.Sprintf("%d:%s", assignment.GetRoundStartUnix(), assignment.GetDomain())
}
