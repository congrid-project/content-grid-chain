package main

import (
	"net/http"
	"sort"
	"strings"
	"time"
)

type leaseListing struct {
	Publisher string
	SlotLabel string
	SlotID    string
	Lease     SlotLease
}

type leasesPageData struct {
	pageData
	Items []leaseListing
}

func (s *server) handleLeases(baseURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slots, err := s.slotStore.ListMarketplaceSlots(r.Context())
		if err != nil {
			http.Error(w, "failed to load leases", http.StatusInternalServerError)
			return
		}

		items := make([]leaseListing, 0)
		for _, slot := range slots {
			if slot.Lease == nil {
				continue
			}
			lease := *slot.Lease
			if strings.TrimSpace(lease.SlotID) == "" {
				lease.SlotID = slot.ID
			}
			if strings.TrimSpace(lease.SlotLabel) == "" {
				lease.SlotLabel = slot.Label
			}
			items = append(items, leaseListing{
				Publisher: slot.Publisher,
				SlotLabel: slot.Label,
				SlotID:    slot.ID,
				Lease:     lease,
			})
		}
		sort.Slice(items, func(i, j int) bool {
			li := items[i].Lease
			lj := items[j].Lease
			if li.EndsAt.Equal(lj.EndsAt) {
				return items[i].Publisher < items[j].Publisher
			}
			if li.EndsAt.IsZero() {
				return false
			}
			if lj.EndsAt.IsZero() {
				return true
			}
			return li.EndsAt.After(lj.EndsAt)
		})

		s.render(w, "leases.html", leasesPageData{
			pageData: pageData{
				Title:       "Leases — Congrid",
				Description: "Published lease placements and copy-ready embed snippets.",
				BaseURL:     baseURL,
				Path:        r.URL.Path,
				NowYear:     time.Now().Year(),
			},
			Items: items,
		})
	}
}
