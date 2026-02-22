package main

import (
	"net/http"
	"strings"
	"time"
)

type publisherDashboardData struct {
	pageData
	Publisher string
	Slots     []Slot
	Leases    []SlotLease
}

func (s *server) handlePublisherDashboard(baseURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		publisher := normalizePublisher(r.URL.Query().Get("publisher"))
		if publisher == "" {
			publisher = defaultPublisherFromStore(r, s.slotStore)
		}
		flash := strings.TrimSpace(r.URL.Query().Get("flash"))

		slots, err := s.slotStore.ListPublisherSlots(r.Context(), publisher)
		if err != nil {
			http.Error(w, "failed to load publisher slots", http.StatusInternalServerError)
			return
		}
		leases, err := s.slotStore.ListPublisherLeases(r.Context(), publisher)
		if err != nil {
			http.Error(w, "failed to load leases", http.StatusInternalServerError)
			return
		}

		data := publisherDashboardData{
			pageData: pageData{
				Title:        "Publisher Dashboard — Congrid",
				Description:  "Manage your publisher link slots and active leases.",
				BaseURL:      baseURL,
				Path:         r.URL.Path,
				NowYear:      time.Now().Year(),
				Flash:        flash,
				WalletConfig: s.walletCfg,
			},
			Publisher: publisher,
			Slots:     slots,
			Leases:    leases,
		}
		s.render(w, "publisher-dashboard.html", data)
	}
}

func (s *server) handlePublisherDashboardPost(baseURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "wallet signing required", http.StatusMethodNotAllowed)
	}
}

func defaultPublisherFromStore(r *http.Request, store SlotStore) string {
	slots, err := store.ListMarketplaceSlots(r.Context())
	if err == nil && len(slots) > 0 {
		return normalizePublisher(slots[0].Publisher)
	}
	return "example.com"
}
