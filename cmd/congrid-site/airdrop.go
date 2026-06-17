package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"

	registryoffchain "content-grid-chain/offchain/registry"
	registry "content-grid-chain/x/registry"
)

type airdropConfig struct {
	DBPath         string
	ChainID        string
	NodeRPC        string
	Denom          string
	Amount         string
	FaucetKeyName  string
	ContentGridBin string
	Keyring        string
	KeyringDir     string
	Fees           string
	GasPrices      string
	BaseURL        string
}

type airdropper struct {
	srv *server
	cfg airdropConfig

	verifier registryoffchain.HTTPContentVerifier
	store    *claimStore
}

func newAirdropper(srv *server, cfg airdropConfig) (*airdropper, error) {
	if strings.TrimSpace(cfg.DBPath) == "" {
		return nil, fmt.Errorf("airdrop db path required")
	}
	if strings.TrimSpace(cfg.Amount) == "" {
		return nil, fmt.Errorf("airdrop amount required")
	}
	if strings.TrimSpace(cfg.Denom) == "" {
		return nil, fmt.Errorf("denom required")
	}
	st, err := openClaimStore(cfg.DBPath)
	if err != nil {
		return nil, err
	}

	return &airdropper{
		srv: srv,
		cfg: cfg,
		verifier: registryoffchain.HTTPContentVerifier{
			Scheme: "", // try https then http
		},
		store: st,
	}, nil
}

func (a *airdropper) handleAirdropGet() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a.srv.render(w, "airdrop.html", pageData{
			Title:        "Airdrop — Congrid",
			Description:  "Claim a one-time optional starter airdrop for on-chain actions.",
			BaseURL:      a.cfg.BaseURL,
			Path:         r.URL.Path,
			NowYear:      time.Now().Year(),
			WalletConfig: a.srv.walletCfg,
		})
	}
}

func (a *airdropper) handleAirdropPost() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()

		if err := r.ParseForm(); err != nil {
			a.renderFlash(w, r, "Invalid form.")
			return
		}
		domain := strings.TrimSpace(strings.ToLower(r.Form.Get("domain")))
		wallet := strings.TrimSpace(r.Form.Get("wallet"))

		if !registry.IsDomainFormatValid(domain) {
			a.renderFlash(w, r, "Invalid domain format.")
			return
		}
		if _, err := sdk.AccAddressFromBech32(wallet); err != nil {
			a.renderFlash(w, r, "Invalid wallet address.")
			return
		}

		primary, err := registry.GetPrimaryDomain(domain)
		if err != nil {
			a.renderFlash(w, r, "Failed to extract primary domain.")
			return
		}

		claimed, info, err := a.store.IsClaimed(primary)
		if err != nil {
			a.renderFlash(w, r, "Server error.")
			return
		}
		if claimed {
			a.renderFlash(w, r, fmt.Sprintf("This primary domain has already claimed an airdrop (%s).", info.TxHash))
			return
		}

		// Verify homepage contains the official Congrid link + badge bound to wallet.
		if err := a.verifier.Verify(ctx, domain, wallet); err != nil {
			a.renderFlash(w, r, "Verification failed: please ensure your homepage includes the Congrid badge link bound to your wallet address.")
			return
		}

		// Mark claimed (optimistically) to prevent races.
		claim := claimInfo{PrimaryDomain: primary, Domain: domain, Wallet: wallet, ClaimedAt: time.Now().UTC()}
		if err := a.store.MarkClaimed(primary, claim); err != nil {
			a.renderFlash(w, r, "Server error.")
			return
		}

		txHash, err := a.sendAirdrop(ctx, wallet)
		if err != nil {
			_ = a.store.Unclaim(primary)
			a.renderFlash(w, r, "Airdrop transaction failed. Please try again later.")
			return
		}
		claim.TxHash = txHash
		_ = a.store.Update(primary, claim)

		a.renderFlash(w, r, fmt.Sprintf("Success! Sent %s%s to %s. Tx: %s", a.cfg.Amount, a.cfg.Denom, wallet, txHash))
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

func (a *airdropper) sendAirdrop(ctx context.Context, toAddr string) (string, error) {
	// For MVP we shell out to the chain binary. This keeps implementation small and matches
	// existing operational workflows. We can replace this with native gRPC signing/broadcast later.
	if strings.TrimSpace(a.cfg.ChainID) == "" || strings.TrimSpace(a.cfg.NodeRPC) == "" {
		return "", fmt.Errorf("chain-id and node rpc required")
	}

	amount := a.cfg.Amount + a.cfg.Denom

	args := []string{
		"tx", "bank", "send", a.cfg.FaucetKeyName, toAddr, amount,
		"--chain-id", a.cfg.ChainID,
		"--node", a.cfg.NodeRPC,
		"--keyring-backend", a.cfg.Keyring,
		"--gas", "auto",
		"--gas-adjustment", "1.3",
		"-y",
		"--output", "json",
	}
	if strings.TrimSpace(a.cfg.KeyringDir) != "" {
		args = append(args, "--keyring-dir", strings.TrimSpace(a.cfg.KeyringDir))
	}
	if strings.TrimSpace(a.cfg.Fees) != "" {
		args = append(args, "--fees", strings.TrimSpace(a.cfg.Fees))
	}
	if strings.TrimSpace(a.cfg.GasPrices) != "" {
		args = append(args, "--gas-prices", strings.TrimSpace(a.cfg.GasPrices))
	}

	cmd := contentGridCommand(ctx, a.cfg.ContentGridBin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("send failed: %w: %s", err, string(out))
	}

	var resp struct {
		TxHash string `json:"txhash"`
		Code   int    `json:"code"`
		RawLog string `json:"raw_log"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", fmt.Errorf("decode tx response: %w", err)
	}
	if resp.Code != 0 {
		return "", fmt.Errorf("tx failed code=%d: %s", resp.Code, resp.RawLog)
	}
	if strings.TrimSpace(resp.TxHash) == "" {
		return "", errors.New("missing txhash")
	}
	return resp.TxHash, nil
}

// --- simple claim store (json file) ---

type claimInfo struct {
	PrimaryDomain string    `json:"primary_domain"`
	Domain        string    `json:"domain"`
	Wallet        string    `json:"wallet"`
	TxHash        string    `json:"tx_hash"`
	ClaimedAt     time.Time `json:"claimed_at"`
}

type claimStore struct {
	path string
	mu   sync.Mutex
	m    map[string]claimInfo
}

func openClaimStore(p string) (*claimStore, error) {
	st := &claimStore{path: p, m: map[string]claimInfo{}}
	// best-effort load
	b, err := osReadFile(p)
	if err == nil {
		_ = json.Unmarshal(b, &st.m)
	}
	return st, nil
}

func (s *claimStore) IsClaimed(primary string) (bool, claimInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ci, ok := s.m[primary]
	return ok, ci, nil
}

func (s *claimStore) MarkClaimed(primary string, ci claimInfo) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.m[primary]; exists {
		return fmt.Errorf("already claimed")
	}
	s.m[primary] = ci
	return s.flushLocked()
}

func (s *claimStore) Update(primary string, ci claimInfo) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.m[primary]; !exists {
		return fmt.Errorf("not claimed")
	}
	s.m[primary] = ci
	return s.flushLocked()
}

func (s *claimStore) Unclaim(primary string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, primary)
	return s.flushLocked()
}

func (s *claimStore) flushLocked() error {
	b, err := json.MarshalIndent(s.m, "", "  ")
	if err != nil {
		return err
	}
	return osWriteFileAtomic(s.path, b, 0o600)
}

func osReadFile(p string) ([]byte, error) { return osReadFileImpl(p) }
func osWriteFileAtomic(p string, b []byte, perm uint32) error {
	return osWriteFileAtomicImpl(p, b, perm)
}
