package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

type SlotStatus string

const (
	SlotStatusListed   SlotStatus = "listed"
	SlotStatusPaused   SlotStatus = "paused"
	SlotStatusUnlisted SlotStatus = "unlisted"
)

type Slot struct {
	ID                 string
	Publisher          string
	PublisherName      string
	Label              string
	Summary            string
	Category           string
	Placement          string
	Size               string
	Rate               int
	Traffic            int
	UnitSeconds        int64
	MinDurationSeconds int64
	MaxDurationSeconds int64
	Status             SlotStatus
	UpdatedAt          time.Time
	Tags               []string
	Lease              *SlotLease
}

type SlotLease struct {
	LeaseID   string
	SlotID    string
	SlotLabel string
	Lessee    string
	TargetURL string
	StartsAt  time.Time
	EndsAt    time.Time
	Rate      int
}

type CreateSlotInput struct {
	Label     string
	Summary   string
	Category  string
	Placement string
	Size      string
	Rate      int
	Traffic   int
	Tags      []string
}

type SlotStore interface {
	ListMarketplaceSlots(ctx context.Context) ([]Slot, error)
	ListPublisherSlots(ctx context.Context, publisher string) ([]Slot, error)
	ListPublisherLeases(ctx context.Context, publisher string) ([]SlotLease, error)
	GetSlot(ctx context.Context, slotID string) (Slot, error)
	CreateSlot(ctx context.Context, publisher string, input CreateSlotInput) (Slot, error)
	UpdateSlotStatus(ctx context.Context, publisher, slotID string, status SlotStatus) error
	CreateLease(ctx context.Context, slotID string, input CreateLeaseInput) (SlotLease, error)
}

// CreateLeaseInput captures advertiser booking details.
type CreateLeaseInput struct {
	TargetURL       string
	StartsAt        time.Time
	DurationSeconds int64
}

type DurationOption struct {
	Label   string
	Seconds int64
}

type memorySlotStore struct {
	mu          sync.Mutex
	slots       []Slot
	nextID      int
	nextLeaseID int
}

func newMemorySlotStore() *memorySlotStore {
	now := time.Now().UTC()
	slots := []Slot{
		{
			ID:            "slot-001",
			Publisher:     "atlasjournal.io",
			PublisherName: "Atlas Journal",
			Label:         "Homepage Hero",
			Summary:       "Rotating banner above the fold with editorial mention.",
			Category:      "News",
			Placement:     "Homepage hero",
			Size:          "728x90",
			Rate:          220,
			Traffic:       120000,
			Status:        SlotStatusListed,
			UpdatedAt:     now.Add(-48 * time.Hour),
			Tags:          []string{"Editorial", "Tech"},
		},
		{
			ID:            "slot-002",
			Publisher:     "atlasjournal.io",
			PublisherName: "Atlas Journal",
			Label:         "Article Sidebar",
			Summary:       "Persistent sidebar slot on long-form features.",
			Category:      "News",
			Placement:     "Article sidebar",
			Size:          "300x600",
			Rate:          140,
			Traffic:       90000,
			Status:        SlotStatusListed,
			UpdatedAt:     now.Add(-72 * time.Hour),
			Tags:          []string{"Longform", "B2B"},
		},
		{
			ID:            "slot-003",
			Publisher:     "atlasjournal.io",
			PublisherName: "Atlas Journal",
			Label:         "Newsletter Inline",
			Summary:       "Inline placement inside the weekly newsletter.",
			Category:      "Email",
			Placement:     "Newsletter body",
			Size:          "600x200",
			Rate:          180,
			Traffic:       45000,
			Status:        SlotStatusListed,
			UpdatedAt:     now.Add(-18 * time.Hour),
			Tags:          []string{"Newsletter", "Premium"},
			Lease: &SlotLease{
				LeaseID:   "lease-001",
				SlotID:    "slot-003",
				SlotLabel: "Newsletter Inline",
				Lessee:    "voyager.ai",
				TargetURL: "https://voyager.ai",
				StartsAt:  now.Add(-7 * 24 * time.Hour),
				EndsAt:    now.Add(21 * 24 * time.Hour),
				Rate:      180,
			},
		},
		{
			ID:            "slot-004",
			Publisher:     "atlasjournal.io",
			PublisherName: "Atlas Journal",
			Label:         "Footer Grid",
			Summary:       "Small grid slot at the end of editorial pages.",
			Category:      "News",
			Placement:     "Article footer",
			Size:          "320x180",
			Rate:          90,
			Traffic:       78000,
			Status:        SlotStatusPaused,
			UpdatedAt:     now.Add(-6 * 24 * time.Hour),
			Tags:          []string{"Editorial"},
		},
		{
			ID:            "slot-005",
			Publisher:     "northwind.dev",
			PublisherName: "Northwind Dev",
			Label:         "Docs Inline",
			Summary:       "Inline slot on API documentation pages.",
			Category:      "Developer",
			Placement:     "Docs inline",
			Size:          "640x120",
			Rate:          160,
			Traffic:       60000,
			Status:        SlotStatusListed,
			UpdatedAt:     now.Add(-30 * time.Hour),
			Tags:          []string{"Developer", "SaaS"},
		},
		{
			ID:            "slot-006",
			Publisher:     "northwind.dev",
			PublisherName: "Northwind Dev",
			Label:         "Changelog Header",
			Summary:       "Header placement on weekly changelog posts.",
			Category:      "Developer",
			Placement:     "Changelog header",
			Size:          "728x90",
			Rate:          110,
			Traffic:       40000,
			Status:        SlotStatusListed,
			UpdatedAt:     now.Add(-12 * time.Hour),
			Tags:          []string{"Product"},
		},
		{
			ID:            "slot-007",
			Publisher:     "northwind.dev",
			PublisherName: "Northwind Dev",
			Label:         "Community Sidebar",
			Summary:       "Slot on community guides and tutorials.",
			Category:      "Developer",
			Placement:     "Guide sidebar",
			Size:          "300x250",
			Rate:          80,
			Traffic:       35000,
			Status:        SlotStatusPaused,
			UpdatedAt:     now.Add(-5 * 24 * time.Hour),
			Tags:          []string{"Community"},
		},
		{
			ID:            "slot-008",
			Publisher:     "paperplane.studio",
			PublisherName: "Paperplane Studio",
			Label:         "Portfolio Hero",
			Summary:       "Hero slot on agency portfolio pages.",
			Category:      "Design",
			Placement:     "Portfolio hero",
			Size:          "900x120",
			Rate:          200,
			Traffic:       52000,
			Status:        SlotStatusListed,
			UpdatedAt:     now.Add(-10 * 24 * time.Hour),
			Tags:          []string{"Design", "Agency"},
		},
		{
			ID:            "slot-009",
			Publisher:     "paperplane.studio",
			PublisherName: "Paperplane Studio",
			Label:         "Case Study Inline",
			Summary:       "Inline slot on case study pages.",
			Category:      "Design",
			Placement:     "Case study inline",
			Size:          "600x200",
			Rate:          130,
			Traffic:       46000,
			Status:        SlotStatusListed,
			UpdatedAt:     now.Add(-4 * 24 * time.Hour),
			Tags:          []string{"Creative"},
			Lease: &SlotLease{
				SlotID:    "slot-009",
				SlotLabel: "Case Study Inline",
				Lessee:    "archway.design",
				StartsAt:  now.Add(-3 * 24 * time.Hour),
				EndsAt:    now.Add(10 * 24 * time.Hour),
				Rate:      130,
			},
		},
		{
			ID:            "slot-010",
			Publisher:     "paperplane.studio",
			PublisherName: "Paperplane Studio",
			Label:         "Footer Partners",
			Summary:       "Small partner logo slot in footer.",
			Category:      "Design",
			Placement:     "Site footer",
			Size:          "240x120",
			Rate:          60,
			Traffic:       38000,
			Status:        SlotStatusUnlisted,
			UpdatedAt:     now.Add(-14 * 24 * time.Hour),
			Tags:          []string{"Agency"},
		},
		{
			ID:            "slot-011",
			Publisher:     "harborbyte.net",
			PublisherName: "Harborbyte",
			Label:         "Data Brief Banner",
			Summary:       "Banner slot on weekly data brief posts.",
			Category:      "Analytics",
			Placement:     "Brief header",
			Size:          "728x90",
			Rate:          150,
			Traffic:       68000,
			Status:        SlotStatusListed,
			UpdatedAt:     now.Add(-36 * time.Hour),
			Tags:          []string{"Analytics", "B2B"},
		},
		{
			ID:            "slot-012",
			Publisher:     "harborbyte.net",
			PublisherName: "Harborbyte",
			Label:         "Research Sidebar",
			Summary:       "Sidebar slot on research reports.",
			Category:      "Analytics",
			Placement:     "Report sidebar",
			Size:          "300x600",
			Rate:          170,
			Traffic:       54000,
			Status:        SlotStatusListed,
			UpdatedAt:     now.Add(-9 * 24 * time.Hour),
			Tags:          []string{"Research"},
		},
		{
			ID:            "slot-013",
			Publisher:     "harborbyte.net",
			PublisherName: "Harborbyte",
			Label:         "Podcast Notes",
			Summary:       "Placement inside weekly podcast notes.",
			Category:      "Analytics",
			Placement:     "Podcast notes",
			Size:          "640x160",
			Rate:          95,
			Traffic:       30000,
			Status:        SlotStatusPaused,
			UpdatedAt:     now.Add(-2 * 24 * time.Hour),
			Tags:          []string{"Audio"},
		},
		{
			ID:            "slot-014",
			Publisher:     "openclimb.org",
			PublisherName: "Openclimb",
			Label:         "Trail Guide Inline",
			Summary:       "Inline slot on guide pages for outdoor gear.",
			Category:      "Outdoors",
			Placement:     "Guide inline",
			Size:          "600x200",
			Rate:          70,
			Traffic:       26000,
			Status:        SlotStatusListed,
			UpdatedAt:     now.Add(-15 * 24 * time.Hour),
			Tags:          []string{"Outdoors"},
		},
		{
			ID:            "slot-015",
			Publisher:     "openclimb.org",
			PublisherName: "Openclimb",
			Label:         "Trip Planner Sidebar",
			Summary:       "Sidebar slot inside trip planner tool.",
			Category:      "Outdoors",
			Placement:     "Planner sidebar",
			Size:          "300x250",
			Rate:          85,
			Traffic:       31000,
			Status:        SlotStatusListed,
			UpdatedAt:     now.Add(-20 * 24 * time.Hour),
			Tags:          []string{"Travel"},
		},
		{
			ID:            "slot-016",
			Publisher:     "civiclens.ai",
			PublisherName: "Civic Lens",
			Label:         "Investigations Hero",
			Summary:       "Hero slot on investigations landing page.",
			Category:      "Civic",
			Placement:     "Investigations hero",
			Size:          "900x120",
			Rate:          210,
			Traffic:       88000,
			Status:        SlotStatusListed,
			UpdatedAt:     now.Add(-11 * time.Hour),
			Tags:          []string{"Civic", "News"},
		},
		{
			ID:            "slot-017",
			Publisher:     "civiclens.ai",
			PublisherName: "Civic Lens",
			Label:         "Opinion Inline",
			Summary:       "Inline slot on opinion essays.",
			Category:      "Civic",
			Placement:     "Opinion inline",
			Size:          "600x200",
			Rate:          120,
			Traffic:       73000,
			Status:        SlotStatusListed,
			UpdatedAt:     now.Add(-26 * time.Hour),
			Tags:          []string{"Opinion"},
		},
		{
			ID:            "slot-018",
			Publisher:     "civiclens.ai",
			PublisherName: "Civic Lens",
			Label:         "Newsletter Sponsor",
			Summary:       "Primary sponsor slot in the daily brief.",
			Category:      "Email",
			Placement:     "Newsletter header",
			Size:          "600x200",
			Rate:          190,
			Traffic:       56000,
			Status:        SlotStatusPaused,
			UpdatedAt:     now.Add(-8 * 24 * time.Hour),
			Tags:          []string{"Newsletter"},
		},
	}

	for i := range slots {
		if slots[i].UnitSeconds == 0 {
			slots[i].UnitSeconds = 7 * 24 * 60 * 60
		}
		if slots[i].MinDurationSeconds == 0 {
			slots[i].MinDurationSeconds = slots[i].UnitSeconds
		}
		if slots[i].MaxDurationSeconds == 0 {
			slots[i].MaxDurationSeconds = 90 * 24 * 60 * 60
		}
	}

	return &memorySlotStore{slots: slots, nextID: len(slots) + 1, nextLeaseID: 1}
}

func (s *memorySlotStore) ListMarketplaceSlots(ctx context.Context) ([]Slot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Slot, 0, len(s.slots))
	for _, slot := range s.slots {
		if slot.Status == SlotStatusUnlisted {
			continue
		}
		out = append(out, slot)
	}
	return out, nil
}

func (s *memorySlotStore) ListPublisherSlots(ctx context.Context, publisher string) ([]Slot, error) {
	publisher = normalizePublisher(publisher)
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Slot, 0)
	for _, slot := range s.slots {
		if normalizePublisher(slot.Publisher) != publisher {
			continue
		}
		out = append(out, slot)
	}
	return out, nil
}

func (s *memorySlotStore) ListPublisherLeases(ctx context.Context, publisher string) ([]SlotLease, error) {
	publisher = normalizePublisher(publisher)
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]SlotLease, 0)
	for _, slot := range s.slots {
		if normalizePublisher(slot.Publisher) != publisher {
			continue
		}
		if slot.Lease == nil {
			continue
		}
		lease := *slot.Lease
		lease.SlotLabel = slot.Label
		lease.SlotID = slot.ID
		out = append(out, lease)
	}
	return out, nil
}

func (s *memorySlotStore) GetSlot(ctx context.Context, slotID string) (Slot, error) {
	slotID = strings.TrimSpace(slotID)
	if slotID == "" {
		return Slot{}, fmt.Errorf("slot id required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, slot := range s.slots {
		if slot.ID == slotID {
			return slot, nil
		}
	}
	return Slot{}, fmt.Errorf("slot not found")
}

func (s *memorySlotStore) CreateSlot(ctx context.Context, publisher string, input CreateSlotInput) (Slot, error) {
	publisher = normalizePublisher(publisher)
	if publisher == "" {
		return Slot{}, fmt.Errorf("publisher required")
	}
	if strings.TrimSpace(input.Label) == "" {
		return Slot{}, fmt.Errorf("label required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id := fmt.Sprintf("slot-%03d", s.nextID)
	s.nextID++

	slot := Slot{
		ID:            id,
		Publisher:     publisher,
		PublisherName: publisher,
		Label:         strings.TrimSpace(input.Label),
		Summary:       strings.TrimSpace(input.Summary),
		Category:      strings.TrimSpace(input.Category),
		Placement:     strings.TrimSpace(input.Placement),
		Size:          strings.TrimSpace(input.Size),
		Rate:          input.Rate,
		Traffic:       input.Traffic,
		Status:        SlotStatusPaused,
		UpdatedAt:     time.Now().UTC(),
		Tags:          input.Tags,
	}
	if slot.Summary == "" {
		slot.Summary = "Slot created by publisher."
	}
	if slot.Category == "" {
		slot.Category = "General"
	}
	s.slots = append(s.slots, slot)
	return slot, nil
}

func (s *memorySlotStore) UpdateSlotStatus(ctx context.Context, publisher, slotID string, status SlotStatus) error {
	publisher = normalizePublisher(publisher)
	if publisher == "" {
		return fmt.Errorf("publisher required")
	}
	if strings.TrimSpace(slotID) == "" {
		return fmt.Errorf("slot id required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.slots {
		slot := &s.slots[i]
		if normalizePublisher(slot.Publisher) != publisher {
			continue
		}
		if slot.ID != slotID {
			continue
		}
		slot.Status = status
		slot.UpdatedAt = time.Now().UTC()
		return nil
	}
	return fmt.Errorf("slot not found")
}

func (s *memorySlotStore) CreateLease(ctx context.Context, slotID string, input CreateLeaseInput) (SlotLease, error) {
	slotID = strings.TrimSpace(slotID)
	if slotID == "" {
		return SlotLease{}, fmt.Errorf("slot id required")
	}
	if strings.TrimSpace(input.TargetURL) == "" {
		return SlotLease{}, fmt.Errorf("target url required")
	}
	if input.DurationSeconds <= 0 {
		return SlotLease{}, fmt.Errorf("duration required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.slots {
		slot := &s.slots[i]
		if slot.ID != slotID {
			continue
		}
		if slot.Status != SlotStatusListed {
			return SlotLease{}, fmt.Errorf("slot not available")
		}
		startsAt := input.StartsAt
		if startsAt.IsZero() {
			startsAt = time.Now().UTC()
		}
		endsAt := startsAt.Add(time.Duration(input.DurationSeconds) * time.Second)
		if slot.Lease != nil && leaseActive(*slot.Lease) {
			if startsAt.Before(slot.Lease.EndsAt) && endsAt.After(slot.Lease.StartsAt) {
				return SlotLease{}, fmt.Errorf("slot already leased")
			}
		}

		lease := SlotLease{
			LeaseID:   fmt.Sprintf("lease-%03d", s.nextLeaseID),
			SlotID:    slot.ID,
			SlotLabel: slot.Label,
			Lessee:    "advertiser",
			TargetURL: strings.TrimSpace(input.TargetURL),
			StartsAt:  startsAt,
			EndsAt:    endsAt,
			Rate:      slot.Rate,
		}
		s.nextLeaseID++
		slot.Lease = &lease
		return lease, nil
	}
	return SlotLease{}, fmt.Errorf("slot not found")
}

func normalizePublisher(publisher string) string {
	return strings.TrimSpace(strings.ToLower(publisher))
}

func formatRate(rate int) string {
	if rate <= 0 {
		return "Negotiable"
	}
	return fmt.Sprintf("%d ucongrid / week", rate)
}

func formatTraffic(traffic int) string {
	if traffic <= 0 {
		return "No data"
	}
	if traffic >= 1000000 {
		v := float64(traffic) / 1000000
		return fmt.Sprintf("%sM / mo", formatDecimal(v))
	}
	if traffic >= 1000 {
		v := float64(traffic) / 1000
		return fmt.Sprintf("%sk / mo", formatDecimal(v))
	}
	return fmt.Sprintf("%d / mo", traffic)
}

func formatDecimal(v float64) string {
	s := fmt.Sprintf("%.1f", v)
	s = strings.TrimSuffix(s, ".0")
	return s
}

func formatDate(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format("Jan 2, 2006")
}

func slotStatusLabel(status SlotStatus) string {
	switch status {
	case SlotStatusListed:
		return "Listed"
	case SlotStatusPaused:
		return "Paused"
	case SlotStatusUnlisted:
		return "Unlisted"
	default:
		return "Unknown"
	}
}

func slotStatusClass(status SlotStatus) string {
	switch status {
	case SlotStatusListed:
		return "badge-ok"
	case SlotStatusPaused:
		return "badge-warn"
	case SlotStatusUnlisted:
		return "badge-muted"
	default:
		return "badge-muted"
	}
}

func slotAvailabilityLabel(slot Slot) string {
	if slot.Status == SlotStatusUnlisted {
		return "Unlisted"
	}
	if slot.Lease != nil && leaseActive(*slot.Lease) {
		return fmt.Sprintf("Leased until %s", formatDate(slot.Lease.EndsAt))
	}
	if slot.Status == SlotStatusPaused {
		return "Paused"
	}
	return "Available"
}

func slotAvailabilityClass(slot Slot) string {
	if slot.Status == SlotStatusUnlisted {
		return "badge-muted"
	}
	if slot.Lease != nil && leaseActive(*slot.Lease) {
		return "badge-warn"
	}
	if slot.Status == SlotStatusPaused {
		return "badge-muted"
	}
	return "badge-ok"
}

func slotIsAvailable(slot Slot) bool {
	if slot.Status != SlotStatusListed {
		return false
	}
	if slot.Lease != nil && leaseActive(*slot.Lease) {
		return false
	}
	return true
}

func leaseStatusLabel(lease SlotLease) string {
	now := time.Now().UTC()
	switch {
	case lease.StartsAt.IsZero() || lease.EndsAt.IsZero():
		return "Unknown"
	case now.Before(lease.StartsAt):
		return "Pending"
	case now.After(lease.EndsAt):
		return "Ended"
	default:
		return "Active"
	}
}

func leaseStatusClass(lease SlotLease) string {
	switch leaseStatusLabel(lease) {
	case "Active":
		return "badge-ok"
	case "Pending":
		return "badge-info"
	case "Ended":
		return "badge-muted"
	default:
		return "badge-muted"
	}
}

func formatLeaseTerm(lease SlotLease) string {
	if lease.StartsAt.IsZero() || lease.EndsAt.IsZero() {
		return "-"
	}
	return fmt.Sprintf("%s - %s", formatDate(lease.StartsAt), formatDate(lease.EndsAt))
}

func leaseActive(lease SlotLease) bool {
	now := time.Now().UTC()
	return !lease.StartsAt.IsZero() && !lease.EndsAt.IsZero() && now.After(lease.StartsAt) && now.Before(lease.EndsAt)
}

func slotDurationOptions(slot Slot) []DurationOption {
	unit := slot.UnitSeconds
	if unit <= 0 {
		unit = 7 * 24 * 60 * 60
	}
	minSeconds := slot.MinDurationSeconds
	if minSeconds <= 0 {
		minSeconds = unit
	}
	maxSeconds := slot.MaxDurationSeconds
	if maxSeconds <= 0 {
		maxSeconds = 90 * 24 * 60 * 60
	}
	minUnits := minSeconds / unit
	if minUnits < 1 {
		minUnits = 1
	}
	maxUnits := maxSeconds / unit
	if maxUnits < minUnits {
		maxUnits = minUnits
	}

	options := make([]DurationOption, 0, maxUnits-minUnits+1)
	for units := minUnits; units <= maxUnits; units++ {
		seconds := units * unit
		options = append(options, DurationOption{
			Label:   formatDurationUnits(units, unit),
			Seconds: seconds,
		})
	}
	return options
}

func formatDurationUnits(units int64, unitSeconds int64) string {
	if unitSeconds == 7*24*60*60 {
		if units == 1 {
			return "1 week"
		}
		return fmt.Sprintf("%d weeks", units)
	}
	if unitSeconds == 24*60*60 {
		if units == 1 {
			return "1 day"
		}
		return fmt.Sprintf("%d days", units)
	}
	if units == 1 {
		return "1 unit"
	}
	return fmt.Sprintf("%d units", units)
}
