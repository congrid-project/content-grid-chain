package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"

	registry "content-grid-chain/x/registry"
)

func (s *server) handlePublisherRegister(baseURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
		defer cancel()

		if err := r.ParseForm(); err != nil {
			s.renderPublishersFlash(w, r, baseURL, "Invalid form.")
			return
		}

		domain := registry.NormalizeDomain(r.Form.Get("domain"))
		wallet := strings.TrimSpace(r.Form.Get("wallet"))
		fromKey := strings.TrimSpace(r.Form.Get("from_key"))

		if domain == "" || !registry.IsDomainFormatValid(domain) {
			s.renderPublishersFlash(w, r, baseURL, "Invalid domain format.")
			return
		}
		if _, err := sdk.AccAddressFromBech32(wallet); err != nil || !strings.HasPrefix(wallet, "congrid1") {
			s.renderPublishersFlash(w, r, baseURL, "Invalid wallet address. Use a congrid1... address.")
			return
		}
		if fromKey == "" {
			s.renderPublishersFlash(w, r, baseURL, "Server-side registration requires a signing key name in server keyring.")
			return
		}
		if s.regCfg.ChainID == "" || s.regCfg.NodeRPC == "" {
			s.renderPublishersFlash(w, r, baseURL, "Server is missing chain-id/node config.")
			return
		}

		addr, err := s.lookupKeyAddress(ctx, fromKey)
		if err != nil {
			s.renderPublishersFlash(w, r, baseURL, fmt.Sprintf("Cannot resolve key %q in server keyring.", fromKey))
			return
		}
		if addr != wallet {
			s.renderPublishersFlash(w, r, baseURL, fmt.Sprintf("Key %q address mismatch. key=%s form=%s", fromKey, addr, wallet))
			return
		}

		txHash, err := s.sendPublisherRegister(ctx, domain, fromKey)
		if err != nil {
			s.renderPublishersFlash(w, r, baseURL, "Registration tx failed: "+err.Error())
			return
		}
		s.renderPublishersFlash(w, r, baseURL, fmt.Sprintf("Publisher registration submitted; page verification is pending. tx=%s", txHash))
	}
}

func (s *server) handlePublisherVerify() http.HandlerFunc {
	type verifyRequest struct {
		Domain string `json:"domain"`
		Wallet string `json:"wallet"`
	}
	type verifyResponse struct {
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	}
	writeResponse := func(w http.ResponseWriter, status int, response verifyResponse) {
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(response)
	}

	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
		var request verifyRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeResponse(w, http.StatusBadRequest, verifyResponse{Error: "invalid verification request"})
			return
		}
		domain := registry.NormalizeDomain(request.Domain)
		wallet := strings.TrimSpace(request.Wallet)
		if !registry.IsDomainFormatValid(domain) {
			writeResponse(w, http.StatusBadRequest, verifyResponse{Error: "invalid domain format"})
			return
		}
		if _, err := sdk.AccAddressFromBech32(wallet); err != nil || !strings.HasPrefix(wallet, "congrid1") {
			writeResponse(w, http.StatusBadRequest, verifyResponse{Error: "invalid wallet address"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
		defer cancel()
		verifier := newSecureHomepageVerifier(true, false)
		if err := verifier.Verify(ctx, domain, wallet); err != nil {
			writeResponse(w, http.StatusUnprocessableEntity, verifyResponse{
				Error: "homepage badge does not contain the connected wallet and registered domain",
			})
			return
		}
		writeResponse(w, http.StatusOK, verifyResponse{OK: true})
	}
}

func (s *server) lookupKeyAddress(ctx context.Context, keyName string) (string, error) {
	args := []string{"keys", "show", keyName, "-a", "--keyring-backend", s.regCfg.KeyringBackend}
	if s.regCfg.KeyringDir != "" {
		args = append(args, "--keyring-dir", s.regCfg.KeyringDir)
	}
	cmd := contentGridCommand(ctx, s.regCfg.ContentGridBin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("keys show failed: %w: %s", err, string(out))
	}
	addr := strings.TrimSpace(string(out))
	if addr == "" {
		return "", fmt.Errorf("empty address")
	}
	return addr, nil
}

func (s *server) sendPublisherRegister(ctx context.Context, domain, fromKey string) (string, error) {
	args := []string{
		"publisher", "register", domain,
		"--from", fromKey,
		"--chain-id", s.regCfg.ChainID,
		"--node", s.regCfg.NodeRPC,
		"--keyring-backend", s.regCfg.KeyringBackend,
		"--gas", "auto",
		"--gas-adjustment", "1.3",
		"-y",
		"--output", "json",
	}
	if s.regCfg.KeyringDir != "" {
		args = append(args, "--keyring-dir", s.regCfg.KeyringDir)
	}
	if s.regCfg.Fees != "" {
		args = append(args, "--fees", s.regCfg.Fees)
	}
	if s.regCfg.GasPrices != "" {
		args = append(args, "--gas-prices", s.regCfg.GasPrices)
	}

	cmd := contentGridCommand(ctx, s.regCfg.ContentGridBin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("register failed: %w: %s", err, string(out))
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
		return "", fmt.Errorf("missing txhash")
	}
	return resp.TxHash, nil
}

func (s *server) renderPublishersFlash(w http.ResponseWriter, r *http.Request, baseURL, msg string) {
	s.render(w, "publishers.html", pageData{
		Title:        "Become a Publisher — Congrid",
		Description:  "Register your site, add the Congrid verification badge, and earn rewards while sending high-quality referral traffic across the open web.",
		BaseURL:      baseURL,
		Path:         r.URL.Path,
		NowYear:      time.Now().Year(),
		Flash:        msg,
		WalletConfig: s.walletCfg,
	})
}
