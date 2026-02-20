package main

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const marketplacePageSize = 8

type pager struct {
	Page    int
	Per     int
	Total   int
	Pages   int
	Start   int
	End     int
	HasPrev bool
	HasNext bool
	PrevURL string
	NextURL string
}

type marketplacePageData struct {
	pageData
	Query string
	Sort  string
	Slots []Slot
	Pager pager
}

func (s *server) handleMarketplace(baseURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := strings.TrimSpace(r.URL.Query().Get("q"))
		sortKey := normalizeSort(r.URL.Query().Get("sort"))
		page := parsePositiveInt(r.URL.Query().Get("page"), 1)
		flash := strings.TrimSpace(r.URL.Query().Get("flash"))

		slots, err := s.slotStore.ListMarketplaceSlots(r.Context())
		if err != nil {
			http.Error(w, "failed to load marketplace", http.StatusInternalServerError)
			return
		}

		filtered := filterSlotsByQuery(slots, query)
		sortSlots(filtered, sortKey)
		pageSlots, pager := paginateSlots(filtered, page, marketplacePageSize)
		pager.PrevURL = marketplaceURL("/marketplace", query, sortKey, pager.Page-1)
		pager.NextURL = marketplaceURL("/marketplace", query, sortKey, pager.Page+1)

		data := marketplacePageData{
			pageData: pageData{
				Title:       "Marketplace — Congrid",
				Description: "Browse publisher link slots and request leases across the Congrid network.",
				BaseURL:     baseURL,
				Path:        r.URL.Path,
				NowYear:     time.Now().Year(),
				Flash:       flash,
			},
			Query: query,
			Sort:  sortKey,
			Slots: pageSlots,
			Pager: pager,
		}
		s.render(w, "marketplace.html", data)
	}
}

func (s *server) handleMarketplaceLease(baseURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Redirect(w, r, marketplaceFlash("/marketplace", "Invalid form."), http.StatusSeeOther)
			return
		}
		slotID := strings.TrimSpace(r.Form.Get("slot_id"))
		targetURL := strings.TrimSpace(r.Form.Get("target_url"))
		durationSeconds := parsePositiveInt64(r.Form.Get("duration_seconds"), 0)
		startDate := strings.TrimSpace(r.Form.Get("start_date"))

		if slotID == "" || targetURL == "" {
			http.Redirect(w, r, marketplaceFlash("/marketplace", "Slot and target URL are required."), http.StatusSeeOther)
			return
		}
		if durationSeconds <= 0 {
			http.Redirect(w, r, marketplaceFlash("/marketplace", "Select a lease duration."), http.StatusSeeOther)
			return
		}

		slot, err := s.slotStore.GetSlot(r.Context(), slotID)
		if err != nil {
			http.Redirect(w, r, marketplaceFlash("/marketplace", "Slot not found."), http.StatusSeeOther)
			return
		}
		if !slotIsAvailable(slot) {
			http.Redirect(w, r, marketplaceFlash("/marketplace", "Slot is not available."), http.StatusSeeOther)
			return
		}

		startAt, err := parseStartDate(startDate)
		if err != nil {
			http.Redirect(w, r, marketplaceFlash("/marketplace", err.Error()), http.StatusSeeOther)
			return
		}

		lease, err := s.slotStore.CreateLease(r.Context(), slotID, CreateLeaseInput{
			TargetURL:       targetURL,
			StartsAt:        startAt,
			DurationSeconds: durationSeconds,
		})
		if err != nil {
			http.Redirect(w, r, marketplaceFlash("/marketplace", "Lease request failed: "+err.Error()), http.StatusSeeOther)
			return
		}

		flash := fmt.Sprintf("Lease booked for %s. Publish with slot_id=%s lease_id=%s and target=%s.", slot.Label, lease.SlotID, lease.LeaseID, lease.TargetURL)
		http.Redirect(w, r, marketplaceFlash("/marketplace", flash), http.StatusSeeOther)
	}
}

func filterSlotsByQuery(slots []Slot, query string) []Slot {
	if strings.TrimSpace(query) == "" {
		out := make([]Slot, len(slots))
		copy(out, slots)
		return out
	}
	q := strings.ToLower(strings.TrimSpace(query))
	out := make([]Slot, 0, len(slots))
	for _, slot := range slots {
		if slotMatchesQuery(slot, q) {
			out = append(out, slot)
		}
	}
	return out
}

func slotMatchesQuery(slot Slot, query string) bool {
	fields := []string{
		slot.Publisher,
		slot.PublisherName,
		slot.Label,
		slot.Summary,
		slot.Category,
		slot.Placement,
		slot.Size,
	}
	for _, tag := range slot.Tags {
		fields = append(fields, tag)
	}
	for _, f := range fields {
		if strings.Contains(strings.ToLower(f), query) {
			return true
		}
	}
	return false
}

func normalizeSort(sortKey string) string {
	switch strings.TrimSpace(sortKey) {
	case "latest", "price-low", "price-high", "traffic", "publisher":
		return strings.TrimSpace(sortKey)
	default:
		return "latest"
	}
}

func sortSlots(slots []Slot, sortKey string) {
	switch sortKey {
	case "price-low":
		sort.SliceStable(slots, func(i, j int) bool {
			ri, rj := slots[i].Rate, slots[j].Rate
			if ri == 0 && rj == 0 {
				return slots[i].UpdatedAt.After(slots[j].UpdatedAt)
			}
			if ri == 0 {
				return false
			}
			if rj == 0 {
				return true
			}
			if ri == rj {
				return slots[i].UpdatedAt.After(slots[j].UpdatedAt)
			}
			return ri < rj
		})
	case "price-high":
		sort.SliceStable(slots, func(i, j int) bool {
			ri, rj := slots[i].Rate, slots[j].Rate
			if ri == 0 && rj == 0 {
				return slots[i].UpdatedAt.After(slots[j].UpdatedAt)
			}
			if ri == 0 {
				return false
			}
			if rj == 0 {
				return true
			}
			if ri == rj {
				return slots[i].UpdatedAt.After(slots[j].UpdatedAt)
			}
			return ri > rj
		})
	case "traffic":
		sort.SliceStable(slots, func(i, j int) bool {
			if slots[i].Traffic == slots[j].Traffic {
				return slots[i].UpdatedAt.After(slots[j].UpdatedAt)
			}
			return slots[i].Traffic > slots[j].Traffic
		})
	case "publisher":
		sort.SliceStable(slots, func(i, j int) bool {
			ai := strings.ToLower(slots[i].PublisherName)
			aj := strings.ToLower(slots[j].PublisherName)
			if ai == aj {
				return slots[i].Label < slots[j].Label
			}
			return ai < aj
		})
	default:
		sort.SliceStable(slots, func(i, j int) bool {
			return slots[i].UpdatedAt.After(slots[j].UpdatedAt)
		})
	}
}

func paginateSlots(slots []Slot, page, per int) ([]Slot, pager) {
	if per <= 0 {
		per = marketplacePageSize
	}
	total := len(slots)
	pages := total / per
	if total%per != 0 {
		pages++
	}
	if pages == 0 {
		pages = 1
	}
	if page < 1 {
		page = 1
	}
	if page > pages {
		page = pages
	}
	start := (page - 1) * per
	end := start + per
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}

	view := slots[start:end]
	p := pager{
		Page:    page,
		Per:     per,
		Total:   total,
		Pages:   pages,
		HasPrev: page > 1,
		HasNext: page < pages,
	}
	if total > 0 {
		p.Start = start + 1
		p.End = end
	}
	return view, p
}

func marketplaceURL(basePath, query, sortKey string, page int) string {
	vals := url.Values{}
	if strings.TrimSpace(query) != "" {
		vals.Set("q", strings.TrimSpace(query))
	}
	if strings.TrimSpace(sortKey) != "" && sortKey != "latest" {
		vals.Set("sort", sortKey)
	}
	if page > 1 {
		vals.Set("page", strconv.Itoa(page))
	}
	if len(vals) == 0 {
		return basePath
	}
	return basePath + "?" + vals.Encode()
}

func parsePositiveInt(value string, fallback int) int {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func parsePositiveInt64(value string, fallback int64) int64 {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func parseStartDate(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid start date")
	}
	startAt := time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, time.UTC)
	now := time.Now().UTC()
	if startAt.Before(now.AddDate(0, 0, -1)) {
		return time.Time{}, fmt.Errorf("start date must be today or later")
	}
	if startAt.Before(now) {
		startAt = now.Add(5 * time.Minute)
	}
	return startAt, nil
}

func marketplaceFlash(basePath, message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return basePath
	}
	vals := url.Values{}
	vals.Set("flash", message)
	return basePath + "?" + vals.Encode()
}
