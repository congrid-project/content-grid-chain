package main

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
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
				Title:       "Publisher Dashboard — Congrid",
				Description: "Manage your publisher link slots and active leases.",
				BaseURL:     baseURL,
				Path:        r.URL.Path,
				NowYear:     time.Now().Year(),
				Flash:       flash,
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
		if err := r.ParseForm(); err != nil {
			http.Redirect(w, r, dashboardURL("", "Invalid form."), http.StatusSeeOther)
			return
		}

		publisher := normalizePublisher(r.Form.Get("publisher"))
		if publisher == "" {
			http.Redirect(w, r, dashboardURL("", "Publisher domain required."), http.StatusSeeOther)
			return
		}

		action := strings.TrimSpace(r.Form.Get("action"))
		var flash string

		switch action {
		case "create":
			label := strings.TrimSpace(r.Form.Get("label"))
			if label == "" {
				flash = "Slot label required."
				break
			}
			input := CreateSlotInput{
				Label:     label,
				Summary:   strings.TrimSpace(r.Form.Get("summary")),
				Category:  strings.TrimSpace(r.Form.Get("category")),
				Placement: strings.TrimSpace(r.Form.Get("placement")),
				Size:      strings.TrimSpace(r.Form.Get("size")),
				Rate:      parseOptionalInt(r.Form.Get("rate")),
				Traffic:   parseOptionalInt(r.Form.Get("traffic")),
			}
			slot, err := s.slotStore.CreateSlot(r.Context(), publisher, input)
			if err != nil {
				flash = "Failed to create slot."
				break
			}
			flash = fmt.Sprintf("Created slot %s (paused).", slot.Label)
		case "pause":
			flash = updateSlotStatus(r, s.slotStore, publisher, SlotStatusPaused)
		case "activate":
			flash = updateSlotStatus(r, s.slotStore, publisher, SlotStatusListed)
		case "unlist":
			flash = updateSlotStatus(r, s.slotStore, publisher, SlotStatusUnlisted)
		default:
			flash = "Unknown action."
		}

		http.Redirect(w, r, dashboardURL(publisher, flash), http.StatusSeeOther)
	}
}

func updateSlotStatus(r *http.Request, store SlotStore, publisher string, status SlotStatus) string {
	slotID := strings.TrimSpace(r.Form.Get("slot_id"))
	if slotID == "" {
		return "Slot id required."
	}
	if err := store.UpdateSlotStatus(r.Context(), publisher, slotID, status); err != nil {
		return "Slot update failed."
	}
	switch status {
	case SlotStatusPaused:
		return "Slot paused."
	case SlotStatusListed:
		return "Slot activated."
	case SlotStatusUnlisted:
		return "Slot unlisted."
	default:
		return "Slot updated."
	}
}

func parseOptionalInt(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return 0
	}
	return parsed
}

func dashboardURL(publisher, flash string) string {
	vals := url.Values{}
	if strings.TrimSpace(publisher) != "" {
		vals.Set("publisher", strings.TrimSpace(publisher))
	}
	if strings.TrimSpace(flash) != "" {
		vals.Set("flash", strings.TrimSpace(flash))
	}
	if len(vals) == 0 {
		return "/publisher/dashboard"
	}
	return "/publisher/dashboard?" + vals.Encode()
}

func defaultPublisherFromStore(r *http.Request, store SlotStore) string {
	slots, err := store.ListMarketplaceSlots(r.Context())
	if err == nil && len(slots) > 0 {
		return normalizePublisher(slots[0].Publisher)
	}
	return "example.com"
}
