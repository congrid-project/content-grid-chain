package main

import (
	"net/http"
	"time"
)

func (s *server) handleAirdropUnavailable(baseURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.render(w, "airdrop-disabled.html", pageData{
			Title:        "Airdrop — Congrid",
			Description:  "Airdrop claim page (currently disabled on this deployment).",
			BaseURL:      baseURL,
			Path:         r.URL.Path,
			NowYear:      time.Now().Year(),
			WalletConfig: s.walletCfg,
		})
	}
}

func (s *server) handleAirdropUnavailablePost(baseURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/airdrop", http.StatusSeeOther)
	}
}
