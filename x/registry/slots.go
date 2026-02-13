package registry

import (
	"fmt"
	"net/url"
	"strings"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	typespb "content-grid-chain/x/registry/typespb"
)

// SlotStatus represents the listing status of a link slot.
type SlotStatus int32

const (
	SlotStatusUnspecified SlotStatus = 0
	SlotStatusListed      SlotStatus = 1
	SlotStatusPaused      SlotStatus = 2
	SlotStatusUnlisted    SlotStatus = 3
)

func (s SlotStatus) String() string {
	switch s {
	case SlotStatusListed:
		return "LISTED"
	case SlotStatusPaused:
		return "PAUSED"
	case SlotStatusUnlisted:
		return "UNLISTED"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", int(s))
	}
}

// LeaseStatus represents the state of a slot lease.
type LeaseStatus int32

const (
	LeaseStatusUnspecified LeaseStatus = 0
	LeaseStatusActive      LeaseStatus = 1
	LeaseStatusCompleted   LeaseStatus = 2
	LeaseStatusViolated    LeaseStatus = 3
	LeaseStatusRefunded    LeaseStatus = 4
)

func (s LeaseStatus) String() string {
	switch s {
	case LeaseStatusActive:
		return "ACTIVE"
	case LeaseStatusCompleted:
		return "COMPLETED"
	case LeaseStatusViolated:
		return "VIOLATED"
	case LeaseStatusRefunded:
		return "REFUNDED"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", int(s))
	}
}

// Slot represents a publisher link slot listing.
type Slot struct {
	ID                 string      `json:"id"`
	Publisher          string      `json:"publisher"`
	Domain             string      `json:"domain"`
	Label              string      `json:"label"`
	Summary            string      `json:"summary,omitempty"`
	Category           string      `json:"category,omitempty"`
	Placement          string      `json:"placement,omitempty"`
	Size               string      `json:"size,omitempty"`
	RateDenom          string      `json:"rate_denom"`
	RateAmount         sdkmath.Int `json:"rate_amount"`
	UnitSeconds        int64       `json:"unit_seconds"`
	MinDurationSeconds int64       `json:"min_duration_seconds"`
	MaxDurationSeconds int64       `json:"max_duration_seconds"`
	Status             SlotStatus  `json:"status"`
	CreatedAtUnix      int64       `json:"created_at_unix"`
	UpdatedAtUnix      int64       `json:"updated_at_unix"`
	Tags               []string    `json:"tags,omitempty"`
}

func (s Slot) ValidateBasic() error {
	if strings.TrimSpace(s.ID) == "" {
		return fmt.Errorf("slot id required")
	}
	if strings.TrimSpace(s.Publisher) == "" {
		return fmt.Errorf("publisher required")
	}
	if _, err := sdk.AccAddressFromBech32(strings.TrimSpace(s.Publisher)); err != nil {
		return fmt.Errorf("invalid publisher address: %w", err)
	}
	if strings.TrimSpace(s.Domain) == "" {
		return fmt.Errorf("domain required")
	}
	if !IsDomainFormatValid(s.Domain) {
		return fmt.Errorf("invalid domain: %s", s.Domain)
	}
	if strings.TrimSpace(s.Label) == "" {
		return fmt.Errorf("label required")
	}
	if strings.TrimSpace(s.RateDenom) == "" {
		return fmt.Errorf("rate denom required")
	}
	if s.RateAmount.IsNegative() {
		return fmt.Errorf("rate amount must be >= 0")
	}
	if s.UnitSeconds <= 0 {
		return fmt.Errorf("unit seconds must be positive")
	}
	if s.MinDurationSeconds <= 0 {
		return fmt.Errorf("min duration seconds must be positive")
	}
	if s.MaxDurationSeconds <= 0 {
		return fmt.Errorf("max duration seconds must be positive")
	}
	if s.MinDurationSeconds > s.MaxDurationSeconds {
		return fmt.Errorf("min duration cannot exceed max duration")
	}
	if s.CreatedAtUnix < 0 || s.UpdatedAtUnix < 0 {
		return fmt.Errorf("timestamps must be non-negative")
	}
	return nil
}

func (s Slot) ToProto() *typespb.Slot {
	return &typespb.Slot{
		Id:                 s.ID,
		Publisher:          s.Publisher,
		Domain:             s.Domain,
		Label:              s.Label,
		Summary:            s.Summary,
		Category:           s.Category,
		Placement:          s.Placement,
		Size:               s.Size,
		RateDenom:          s.RateDenom,
		RateAmount:         s.RateAmount.String(),
		UnitSeconds:        s.UnitSeconds,
		MinDurationSeconds: s.MinDurationSeconds,
		MaxDurationSeconds: s.MaxDurationSeconds,
		Status:             typespb.SlotStatus(s.Status),
		CreatedAtUnix:      s.CreatedAtUnix,
		UpdatedAtUnix:      s.UpdatedAtUnix,
		Tags:               append([]string(nil), s.Tags...),
	}
}

func SlotFromProto(pb *typespb.Slot) (Slot, error) {
	if pb == nil {
		return Slot{}, fmt.Errorf("nil slot")
	}
	amt, ok := sdkmath.NewIntFromString(strings.TrimSpace(pb.GetRateAmount()))
	if !ok {
		return Slot{}, fmt.Errorf("invalid rate amount")
	}
	out := Slot{
		ID:                 strings.TrimSpace(pb.GetId()),
		Publisher:          strings.TrimSpace(pb.GetPublisher()),
		Domain:             NormalizeDomain(pb.GetDomain()),
		Label:              strings.TrimSpace(pb.GetLabel()),
		Summary:            strings.TrimSpace(pb.GetSummary()),
		Category:           strings.TrimSpace(pb.GetCategory()),
		Placement:          strings.TrimSpace(pb.GetPlacement()),
		Size:               strings.TrimSpace(pb.GetSize()),
		RateDenom:          strings.TrimSpace(pb.GetRateDenom()),
		RateAmount:         amt,
		UnitSeconds:        pb.GetUnitSeconds(),
		MinDurationSeconds: pb.GetMinDurationSeconds(),
		MaxDurationSeconds: pb.GetMaxDurationSeconds(),
		Status:             SlotStatus(pb.GetStatus()),
		CreatedAtUnix:      pb.GetCreatedAtUnix(),
		UpdatedAtUnix:      pb.GetUpdatedAtUnix(),
		Tags:               append([]string(nil), pb.GetTags()...),
	}
	return out, out.ValidateBasic()
}

// SlotLease represents an on-chain lease for a slot.
type SlotLease struct {
	ID              string      `json:"id"`
	SlotID          string      `json:"slot_id"`
	Publisher       string      `json:"publisher"`
	Lessee          string      `json:"lessee"`
	TargetURL       string      `json:"target_url"`
	StartsAtUnix    int64       `json:"starts_at_unix"`
	EndsAtUnix      int64       `json:"ends_at_unix"`
	RateDenom       string      `json:"rate_denom"`
	RateAmount      sdkmath.Int `json:"rate_amount"`
	UnitSeconds     int64       `json:"unit_seconds"`
	EscrowTotal     sdkmath.Int `json:"escrow_total"`
	EscrowRemaining sdkmath.Int `json:"escrow_remaining"`
	PaidOut         sdkmath.Int `json:"paid_out"`
	Status          LeaseStatus `json:"status"`
	CreatedAtUnix   int64       `json:"created_at_unix"`
	UpdatedAtUnix   int64       `json:"updated_at_unix"`
	PaidThroughUnix int64       `json:"paid_through_unix"`
}

func (l SlotLease) ValidateBasic() error {
	if strings.TrimSpace(l.ID) == "" {
		return fmt.Errorf("lease id required")
	}
	if strings.TrimSpace(l.SlotID) == "" {
		return fmt.Errorf("slot id required")
	}
	if strings.TrimSpace(l.Publisher) == "" {
		return fmt.Errorf("publisher required")
	}
	if _, err := sdk.AccAddressFromBech32(strings.TrimSpace(l.Publisher)); err != nil {
		return fmt.Errorf("invalid publisher address: %w", err)
	}
	if strings.TrimSpace(l.Lessee) == "" {
		return fmt.Errorf("lessee required")
	}
	if _, err := sdk.AccAddressFromBech32(strings.TrimSpace(l.Lessee)); err != nil {
		return fmt.Errorf("invalid lessee address: %w", err)
	}
	if strings.TrimSpace(l.TargetURL) == "" {
		return fmt.Errorf("target url required")
	}
	if _, err := url.ParseRequestURI(strings.TrimSpace(l.TargetURL)); err != nil {
		return fmt.Errorf("invalid target url: %w", err)
	}
	if l.StartsAtUnix <= 0 {
		return fmt.Errorf("starts_at_unix must be positive")
	}
	if l.EndsAtUnix <= l.StartsAtUnix {
		return fmt.Errorf("ends_at_unix must be > starts_at_unix")
	}
	if strings.TrimSpace(l.RateDenom) == "" {
		return fmt.Errorf("rate denom required")
	}
	if l.RateAmount.IsNegative() {
		return fmt.Errorf("rate amount must be >= 0")
	}
	if l.UnitSeconds <= 0 {
		return fmt.Errorf("unit seconds must be positive")
	}
	if l.EscrowTotal.IsNegative() || l.EscrowRemaining.IsNegative() || l.PaidOut.IsNegative() {
		return fmt.Errorf("escrow amounts must be >= 0")
	}
	if l.CreatedAtUnix < 0 || l.UpdatedAtUnix < 0 {
		return fmt.Errorf("timestamps must be non-negative")
	}
	return nil
}

func (l SlotLease) ToProto() *typespb.SlotLease {
	return &typespb.SlotLease{
		Id:              l.ID,
		SlotId:          l.SlotID,
		Publisher:       l.Publisher,
		Lessee:          l.Lessee,
		TargetUrl:       l.TargetURL,
		StartsAtUnix:    l.StartsAtUnix,
		EndsAtUnix:      l.EndsAtUnix,
		RateDenom:       l.RateDenom,
		RateAmount:      l.RateAmount.String(),
		UnitSeconds:     l.UnitSeconds,
		EscrowTotal:     l.EscrowTotal.String(),
		EscrowRemaining: l.EscrowRemaining.String(),
		PaidOut:         l.PaidOut.String(),
		Status:          typespb.LeaseStatus(l.Status),
		CreatedAtUnix:   l.CreatedAtUnix,
		UpdatedAtUnix:   l.UpdatedAtUnix,
		PaidThroughUnix: l.PaidThroughUnix,
	}
}

func SlotLeaseFromProto(pb *typespb.SlotLease) (SlotLease, error) {
	if pb == nil {
		return SlotLease{}, fmt.Errorf("nil lease")
	}
	amt, ok := sdkmath.NewIntFromString(strings.TrimSpace(pb.GetRateAmount()))
	if !ok {
		return SlotLease{}, fmt.Errorf("invalid rate amount")
	}
	escrowTotal, ok := sdkmath.NewIntFromString(strings.TrimSpace(pb.GetEscrowTotal()))
	if !ok {
		return SlotLease{}, fmt.Errorf("invalid escrow total")
	}
	escrowRemaining, ok := sdkmath.NewIntFromString(strings.TrimSpace(pb.GetEscrowRemaining()))
	if !ok {
		return SlotLease{}, fmt.Errorf("invalid escrow remaining")
	}
	paidOut, ok := sdkmath.NewIntFromString(strings.TrimSpace(pb.GetPaidOut()))
	if !ok {
		return SlotLease{}, fmt.Errorf("invalid paid out")
	}
	out := SlotLease{
		ID:              strings.TrimSpace(pb.GetId()),
		SlotID:          strings.TrimSpace(pb.GetSlotId()),
		Publisher:       strings.TrimSpace(pb.GetPublisher()),
		Lessee:          strings.TrimSpace(pb.GetLessee()),
		TargetURL:       strings.TrimSpace(pb.GetTargetUrl()),
		StartsAtUnix:    pb.GetStartsAtUnix(),
		EndsAtUnix:      pb.GetEndsAtUnix(),
		RateDenom:       strings.TrimSpace(pb.GetRateDenom()),
		RateAmount:      amt,
		UnitSeconds:     pb.GetUnitSeconds(),
		EscrowTotal:     escrowTotal,
		EscrowRemaining: escrowRemaining,
		PaidOut:         paidOut,
		Status:          LeaseStatus(pb.GetStatus()),
		CreatedAtUnix:   pb.GetCreatedAtUnix(),
		UpdatedAtUnix:   pb.GetUpdatedAtUnix(),
		PaidThroughUnix: pb.GetPaidThroughUnix(),
	}
	return out, out.ValidateBasic()
}
