package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	registry "content-grid-chain/x/registry"
)

type airdropConfig struct {
	DBDriver              string
	DBDSN                 string
	DBPath                string
	ChainID               string
	NodeRPC               string
	Denom                 string
	Amount                string
	FaucetKeyName         string
	ContentGridBin        string
	Keyring               string
	KeyringDir            string
	KeyringPassEnv        string
	Home                  string
	Fees                  string
	GasPrices             string
	BaseURL               string
	AllowInsecureKeyring  bool
	AllowHTTPVerification bool
	AllowPrivateTargets   bool
	WorkerEnabled         bool
	ConfirmInterval       time.Duration
	ConfirmTimeout        time.Duration
	VerificationWorkers   int
}

type airdropper struct {
	srv      *server
	cfg      airdropConfig
	verifier homepageVerifier
	store    claimStore
	tx       airdropTxClient

	verificationSlots chan struct{}
	wake              chan struct{}
	workerDone        chan struct{}
	workerCancel      context.CancelFunc
	startOnce         sync.Once
	closeOnce         sync.Once
}

func newAirdropper(srv *server, cfg airdropConfig) (*airdropper, error) {
	if strings.TrimSpace(cfg.Fees) == "" && strings.TrimSpace(cfg.GasPrices) == "" {
		cfg.GasPrices = "0.001" + strings.TrimSpace(cfg.Denom)
	}
	if cfg.ConfirmInterval <= 0 {
		cfg.ConfirmInterval = 5 * time.Second
	}
	if cfg.ConfirmTimeout <= 0 {
		cfg.ConfirmTimeout = 2 * time.Minute
	}
	if cfg.VerificationWorkers <= 0 {
		cfg.VerificationWorkers = 8
	}
	if err := validateAirdropConfig(cfg); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	store, err := openSQLClaimStore(ctx, cfg.DBDriver, cfg.DBDSN, cfg.DBPath)
	if err != nil {
		return nil, err
	}
	if cfg.AllowHTTPVerification {
		log.Printf("WARNING: airdrop HTTP verification is enabled; this is unsafe for production")
	}
	if cfg.AllowPrivateTargets {
		log.Printf("WARNING: airdrop private verification targets are enabled; this is unsafe for production")
	}
	return &airdropper{
		srv:               srv,
		cfg:               cfg,
		verifier:          newSecureHomepageVerifier(cfg.AllowHTTPVerification, cfg.AllowPrivateTargets),
		store:             store,
		tx:                &cliAirdropTxClient{cfg: cfg},
		verificationSlots: make(chan struct{}, cfg.VerificationWorkers),
		wake:              make(chan struct{}, 1),
		workerDone:        make(chan struct{}),
	}, nil
}

func validateAirdropConfig(cfg airdropConfig) error {
	amount, ok := sdkmath.NewIntFromString(strings.TrimSpace(cfg.Amount))
	if !ok || !amount.IsPositive() {
		return fmt.Errorf("airdrop amount must be a positive integer")
	}
	if err := sdk.ValidateDenom(strings.TrimSpace(cfg.Denom)); err != nil {
		return fmt.Errorf("invalid airdrop denom: %w", err)
	}
	if cfg.WorkerEnabled {
		if strings.TrimSpace(cfg.ChainID) == "" || strings.TrimSpace(cfg.NodeRPC) == "" {
			return fmt.Errorf("airdrop chain-id and node rpc required")
		}
		if strings.TrimSpace(cfg.FaucetKeyName) == "" {
			return fmt.Errorf("airdrop faucet key required")
		}
		backend := strings.ToLower(strings.TrimSpace(cfg.Keyring))
		if backend == "" {
			return fmt.Errorf("airdrop keyring backend required")
		}
		if backend == "test" && !cfg.AllowInsecureKeyring {
			return fmt.Errorf("airdrop refuses insecure test keyring; configure os/file or explicitly allow it for development")
		}
		if backend == "file" {
			if strings.TrimSpace(cfg.KeyringPassEnv) == "" {
				return fmt.Errorf("file keyring requires --keyring-passphrase-env for unattended airdrop signing")
			}
			if _, ok := os.LookupEnv(strings.TrimSpace(cfg.KeyringPassEnv)); !ok {
				return fmt.Errorf("airdrop keyring passphrase env %q is not set", cfg.KeyringPassEnv)
			}
		}
		if strings.TrimSpace(cfg.Fees) != "" && strings.TrimSpace(cfg.GasPrices) != "" {
			return fmt.Errorf("airdrop fees and gas-prices are mutually exclusive")
		}
	}
	switch normalizeDBDriver(cfg.DBDriver) {
	case "sqlite", "postgres":
	default:
		return fmt.Errorf("unsupported airdrop db driver %q (use sqlite or postgres)", cfg.DBDriver)
	}
	if cfg.VerificationWorkers > 64 {
		return fmt.Errorf("airdrop verification concurrency must be <= 64")
	}
	if cfg.ConfirmInterval > 0 && cfg.ConfirmTimeout > 0 && cfg.ConfirmTimeout <= cfg.ConfirmInterval {
		return fmt.Errorf("airdrop confirmation timeout must be greater than confirmation interval")
	}
	return nil
}

func (a *airdropper) Start(parent context.Context) error {
	var startErr error
	a.startOnce.Do(func() {
		if !a.cfg.WorkerEnabled {
			close(a.workerDone)
			return
		}
		ctx, cancel := context.WithCancel(parent)
		a.workerCancel = cancel
		recovered, err := a.store.RecoverInterrupted(ctx)
		if err != nil {
			cancel()
			close(a.workerDone)
			startErr = err
			return
		}
		if recovered > 0 {
			log.Printf("airdrop: marked %d interrupted submissions for operator reconciliation", recovered)
		}
		go a.runWorker(ctx)
	})
	return startErr
}

func (a *airdropper) Close(ctx context.Context) error {
	var closeErr error
	a.closeOnce.Do(func() {
		if a.workerCancel != nil {
			a.workerCancel()
			select {
			case <-a.workerDone:
			case <-ctx.Done():
				closeErr = ctx.Err()
			}
		}
		if err := a.store.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
	})
	return closeErr
}

func (a *airdropper) handleAirdropGet() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a.renderFlash(w, r, "")
	}
}

func (a *airdropper) handleAirdropPost() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
		defer cancel()
		r.Body = http.MaxBytesReader(w, r.Body, 8<<10)

		select {
		case a.verificationSlots <- struct{}{}:
			defer func() { <-a.verificationSlots }()
		default:
			a.renderFlash(w, r, "Airdrop verification is busy. Please try again shortly.")
			return
		}

		if err := r.ParseForm(); err != nil {
			a.renderFlash(w, r, "Invalid form.")
			return
		}
		domain := registry.NormalizeDomain(r.Form.Get("domain"))
		wallet := strings.TrimSpace(r.Form.Get("wallet"))

		if !registry.IsDomainFormatValid(domain) {
			a.renderFlash(w, r, "Invalid domain format.")
			return
		}
		if _, err := sdk.AccAddressFromBech32(wallet); err != nil || !strings.HasPrefix(wallet, "congrid1") {
			a.renderFlash(w, r, "Invalid wallet address. Use a congrid1... address.")
			return
		}

		// Intentionally retain the chain's simplified last-two-label rule. There
		// is no wallet uniqueness constraint: one wallet may claim for multiple
		// independently keyed websites.
		primary, err := registry.GetPrimaryDomain(domain)
		if err != nil {
			a.renderFlash(w, r, "Failed to extract primary domain.")
			return
		}

		if existing, ok, err := a.store.Get(ctx, primary); err != nil {
			log.Printf("airdrop: lookup %s: %v", primary, err)
			a.renderFlash(w, r, "Server error.")
			return
		} else if ok {
			a.renderFlash(w, r, claimStatusMessage(existing))
			return
		}

		if err := a.verifier.Verify(ctx, domain, wallet); err != nil {
			log.Printf("airdrop: verification failed domain=%s: %v", domain, err)
			a.renderFlash(w, r, "Verification failed: ensure the HTTPS homepage contains the Congrid badge bound to this domain and wallet.")
			return
		}

		claim, created, err := a.store.Reserve(ctx, claimInfo{
			PrimaryDomain: primary,
			Domain:        domain,
			Wallet:        wallet,
			Amount:        a.cfg.Amount,
			Denom:         a.cfg.Denom,
		})
		if err != nil {
			log.Printf("airdrop: reserve %s: %v", primary, err)
			a.renderFlash(w, r, "Server error.")
			return
		}
		if !created {
			a.renderFlash(w, r, claimStatusMessage(claim))
			return
		}
		a.notifyWorker()
		a.renderFlash(w, r, fmt.Sprintf("Website verified. The %s%s airdrop is queued for %s.", claim.Amount, claim.Denom, claim.Wallet))
	}
}

func (a *airdropper) renderFlash(w http.ResponseWriter, r *http.Request, msg string) {
	a.srv.render(w, "airdrop.html", pageData{
		Title:        "Airdrop — Congrid",
		Description:  "Claim a one-time optional starter airdrop for on-chain actions.",
		BaseURL:      a.cfg.BaseURL,
		Path:         r.URL.Path,
		NowYear:      time.Now().Year(),
		Flash:        msg,
		WalletConfig: a.srv.walletCfg,
	})
}

func claimStatusMessage(claim claimInfo) string {
	switch claim.Status {
	case claimStatusVerified:
		return "This website has already been verified and its airdrop is queued."
	case claimStatusSubmitting:
		return "This website's airdrop transaction is being submitted."
	case claimStatusBroadcast:
		return fmt.Sprintf("This website's airdrop was broadcast and is awaiting confirmation. Tx: %s", claim.TxHash)
	case claimStatusConfirmed:
		return fmt.Sprintf("This website has already claimed its one-time airdrop. Tx: %s", claim.TxHash)
	case claimStatusFailed:
		return "This website's airdrop could not be submitted and requires operator review."
	case claimStatusNeedsReconcile:
		return "This website's airdrop has an uncertain transaction result and requires operator reconciliation; it will not be resent automatically."
	default:
		return "This website already has an airdrop claim."
	}
}

func (a *airdropper) notifyWorker() {
	select {
	case a.wake <- struct{}{}:
	default:
	}
}

func (a *airdropper) runWorker(ctx context.Context) {
	defer close(a.workerDone)
	ticker := time.NewTicker(a.cfg.ConfirmInterval)
	defer ticker.Stop()

	process := func() {
		if err := a.confirmOne(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("airdrop: confirm transaction: %v", err)
		}
		for {
			processed, err := a.submitOne(ctx)
			if err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("airdrop: submit queued claim: %v", err)
			}
			if !processed || ctx.Err() != nil {
				return
			}
		}
	}

	process()
	for {
		select {
		case <-ctx.Done():
			return
		case <-a.wake:
			process()
		case <-ticker.C:
			process()
		}
	}
}

func (a *airdropper) submitOne(ctx context.Context) (bool, error) {
	claim, ok, err := a.store.Next(ctx, claimStatusVerified)
	if err != nil || !ok {
		return false, err
	}
	transitioned, err := a.store.MarkSubmitting(ctx, claim.PrimaryDomain)
	if err != nil || !transitioned {
		return true, err
	}

	txHash, ambiguous, err := a.tx.Broadcast(ctx, claim)
	if err != nil {
		status := claimStatusFailed
		if ambiguous {
			status = claimStatusNeedsReconcile
		}
		if updateErr := a.store.MarkTerminal(ctx, claim.PrimaryDomain, status, err.Error()); updateErr != nil {
			return true, fmt.Errorf("broadcast error: %v; persist status: %w", err, updateErr)
		}
		return true, fmt.Errorf("website %s: %w", claim.PrimaryDomain, err)
	}
	if err := a.store.MarkBroadcast(ctx, claim.PrimaryDomain, txHash, time.Now().UTC()); err != nil {
		// Never automatically resubmit: the chain accepted the transaction but
		// persistence failed, so a retry could pay the website twice.
		reason := fmt.Sprintf("transaction %s was accepted but its broadcast state could not be persisted: %v", txHash, err)
		if updateErr := a.store.MarkTerminal(ctx, claim.PrimaryDomain, claimStatusNeedsReconcile, reason); updateErr != nil {
			return true, fmt.Errorf("%s; reconciliation state also failed: %w", reason, updateErr)
		}
		return true, errors.New(reason)
	}
	log.Printf("airdrop: broadcast website=%s wallet=%s tx=%s", claim.PrimaryDomain, claim.Wallet, txHash)
	return true, nil
}

func (a *airdropper) confirmOne(ctx context.Context) error {
	claim, ok, err := a.store.Next(ctx, claimStatusBroadcast)
	if err != nil || !ok {
		return err
	}
	status, message, err := a.tx.Query(ctx, claim.TxHash)
	if err != nil || status == txConfirmationPending {
		if !claim.BroadcastAt.IsZero() && time.Since(claim.BroadcastAt) >= a.cfg.ConfirmTimeout {
			reason := "transaction was not found before the confirmation timeout; operator reconciliation required"
			if err != nil {
				reason = "transaction confirmation remained unavailable: " + err.Error()
			}
			return a.store.MarkConfirmationUncertain(ctx, claim.PrimaryDomain, reason)
		}
		return err
	}
	if status == txConfirmationFailed {
		return a.store.MarkDeliveryFailed(ctx, claim.PrimaryDomain, message)
	}
	if err := a.store.MarkConfirmed(ctx, claim.PrimaryDomain, time.Now().UTC()); err != nil {
		return err
	}
	log.Printf("airdrop: confirmed website=%s tx=%s", claim.PrimaryDomain, claim.TxHash)
	return nil
}

type txConfirmation int

const (
	txConfirmationPending txConfirmation = iota
	txConfirmationConfirmed
	txConfirmationFailed
)

type airdropTxClient interface {
	// Broadcast returns ambiguous=true when callers cannot prove that the
	// transaction was rejected before broadcast. Ambiguous claims must never be
	// retried automatically.
	Broadcast(context.Context, claimInfo) (txHash string, ambiguous bool, err error)
	Query(context.Context, string) (txConfirmation, string, error)
}

type cliAirdropTxClient struct {
	cfg airdropConfig
}

func (c *cliAirdropTxClient) Broadcast(ctx context.Context, claim claimInfo) (string, bool, error) {
	amount := claim.Amount + claim.Denom
	args := []string{
		"tx", "bank", "send", c.cfg.FaucetKeyName, claim.Wallet, amount,
		"--chain-id", c.cfg.ChainID,
		"--node", c.cfg.NodeRPC,
		"--keyring-backend", c.cfg.Keyring,
		"--gas", "auto",
		"--gas-adjustment", "1.3",
		"--broadcast-mode", "sync",
		"--note", "congrid-airdrop:" + claim.PrimaryDomain,
		"-y",
		"--output", "json",
	}
	args = appendAirdropCLIOptions(args, c.cfg, true)
	cmd := contentGridCommand(ctx, c.cfg.ContentGridBin, args...)
	if err := attachAirdropKeyringPassphrase(cmd, c.cfg); err != nil {
		return "", false, err
	}
	out, runErr := cmd.CombinedOutput()
	resp, decodeErr := decodeTxResponse(out)
	if decodeErr == nil && resp.Code != 0 {
		return "", false, fmt.Errorf("airdrop rejected code=%d: %s", resp.Code, resp.RawLog)
	}
	if runErr != nil {
		return "", true, fmt.Errorf("airdrop command failed: %w: %s", runErr, truncateError(string(out)))
	}
	if decodeErr != nil {
		return "", true, decodeErr
	}
	if strings.TrimSpace(resp.TxHash) == "" {
		return "", true, fmt.Errorf("airdrop response missing txhash")
	}
	return strings.TrimSpace(resp.TxHash), false, nil
}

func (c *cliAirdropTxClient) Query(ctx context.Context, txHash string) (txConfirmation, string, error) {
	queryCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	args := []string{"query", "tx", txHash, "--node", c.cfg.NodeRPC, "--output", "json"}
	args = appendAirdropCLIOptions(args, c.cfg, false)
	cmd := contentGridCommand(queryCtx, c.cfg.ContentGridBin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		lower := strings.ToLower(string(out))
		if strings.Contains(lower, "not found") || strings.Contains(lower, "no transaction") {
			return txConfirmationPending, "", nil
		}
		return txConfirmationPending, "", fmt.Errorf("query tx %s: %w: %s", txHash, err, truncateError(string(out)))
	}
	resp, err := decodeTxResponse(out)
	if err != nil {
		return txConfirmationPending, "", err
	}
	if !strings.EqualFold(strings.TrimSpace(resp.TxHash), strings.TrimSpace(txHash)) {
		return txConfirmationPending, "", fmt.Errorf("query tx response hash mismatch: got %q want %q", resp.TxHash, txHash)
	}
	if resp.Code != 0 {
		return txConfirmationFailed, resp.RawLog, nil
	}
	return txConfirmationConfirmed, "", nil
}

func appendAirdropCLIOptions(args []string, cfg airdropConfig, signing bool) []string {
	if strings.TrimSpace(cfg.Home) != "" {
		args = append(args, "--home", strings.TrimSpace(cfg.Home))
	}
	if signing && strings.TrimSpace(cfg.KeyringDir) != "" {
		args = append(args, "--keyring-dir", strings.TrimSpace(cfg.KeyringDir))
	}
	if signing && strings.TrimSpace(cfg.Fees) != "" {
		args = append(args, "--fees", strings.TrimSpace(cfg.Fees))
	}
	if signing && strings.TrimSpace(cfg.GasPrices) != "" {
		args = append(args, "--gas-prices", strings.TrimSpace(cfg.GasPrices))
	}
	return args
}

func attachAirdropKeyringPassphrase(cmd *exec.Cmd, cfg airdropConfig) error {
	if strings.ToLower(strings.TrimSpace(cfg.Keyring)) != "file" || strings.TrimSpace(cfg.KeyringPassEnv) == "" {
		return nil
	}
	passphrase, ok := os.LookupEnv(strings.TrimSpace(cfg.KeyringPassEnv))
	if !ok {
		return fmt.Errorf("airdrop keyring passphrase env %q is not set", cfg.KeyringPassEnv)
	}
	cmd.Stdin = strings.NewReader(passphrase + "\n")
	return nil
}

type txResponse struct {
	TxHash string `json:"txhash"`
	Code   uint32 `json:"code"`
	RawLog string `json:"raw_log"`
}

func decodeTxResponse(out []byte) (txResponse, error) {
	out = bytes.TrimSpace(out)
	for i := 0; i < len(out); i++ {
		if out[i] != '{' {
			continue
		}
		var response txResponse
		decoder := json.NewDecoder(bytes.NewReader(out[i:]))
		if err := decoder.Decode(&response); err == nil {
			return response, nil
		}
	}
	return txResponse{}, fmt.Errorf("decode tx response: no JSON object found: %s", truncateError(string(out)))
}
