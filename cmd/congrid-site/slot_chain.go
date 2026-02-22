package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	registrypb "content-grid-chain/x/registry/typespb"

	query "github.com/cosmos/cosmos-sdk/types/query"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type chainSlotConfig struct {
	GRPCAddr string
}

type chainSlotStore struct {
	cfg         chainSlotConfig
	conn        *grpc.ClientConn
	queryClient registrypb.QueryClient
}

func newChainSlotStore(cfg chainSlotConfig) (*chainSlotStore, error) {
	if strings.TrimSpace(cfg.GRPCAddr) == "" {
		return nil, fmt.Errorf("grpc address required")
	}

	conn, err := grpc.Dial(cfg.GRPCAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	store := &chainSlotStore{
		cfg:         cfg,
		conn:        conn,
		queryClient: registrypb.NewQueryClient(conn),
	}
	return store, nil
}

func (s *chainSlotStore) Close() error {
	if s.conn == nil {
		return nil
	}
	return s.conn.Close()
}

func (s *chainSlotStore) ListMarketplaceSlots(ctx context.Context) ([]Slot, error) {
	slots, err := s.listSlots(ctx, "", registrypb.SlotStatus_SLOT_STATUS_UNSPECIFIED)
	if err != nil {
		return nil, err
	}
	leases, err := s.listLeases(ctx, "", "", true, time.Now().UTC().Unix())
	if err != nil {
		return nil, err
	}
	leaseBySlot := map[string]registrypb.SlotLease{}
	for _, lease := range leases {
		leaseBySlot[lease.GetSlotId()] = lease
	}

	out := make([]Slot, 0, len(slots))
	for _, slot := range slots {
		if slot.GetStatus() == registrypb.SlotStatus_SLOT_STATUS_UNLISTED {
			continue
		}
		ui := slotFromChain(slot)
		if lease, ok := leaseBySlot[slot.GetId()]; ok {
			leaseCopy := lease
			ui.Lease = leaseFromChain(&leaseCopy)
		}
		out = append(out, ui)
	}
	return out, nil
}

func (s *chainSlotStore) ListPublisherSlots(ctx context.Context, publisher string) ([]Slot, error) {
	owner, err := s.publisherOwner(ctx, publisher)
	if err != nil {
		return nil, err
	}
	if owner == "" {
		return nil, fmt.Errorf("publisher not found")
	}

	slots, err := s.listSlots(ctx, owner, registrypb.SlotStatus_SLOT_STATUS_UNSPECIFIED)
	if err != nil {
		return nil, err
	}
	leases, err := s.listLeases(ctx, owner, "", true, time.Now().UTC().Unix())
	if err != nil {
		return nil, err
	}
	leaseBySlot := map[string]registrypb.SlotLease{}
	for _, lease := range leases {
		leaseBySlot[lease.GetSlotId()] = lease
	}

	out := make([]Slot, 0)
	for _, slot := range slots {
		if normalizePublisher(slot.GetDomain()) != normalizePublisher(publisher) {
			continue
		}
		ui := slotFromChain(slot)
		if lease, ok := leaseBySlot[slot.GetId()]; ok {
			leaseCopy := lease
			ui.Lease = leaseFromChain(&leaseCopy)
		}
		out = append(out, ui)
	}
	return out, nil
}

func (s *chainSlotStore) ListPublisherLeases(ctx context.Context, publisher string) ([]SlotLease, error) {
	owner, err := s.publisherOwner(ctx, publisher)
	if err != nil {
		return nil, err
	}
	if owner == "" {
		return nil, fmt.Errorf("publisher not found")
	}

	slots, err := s.listSlots(ctx, owner, registrypb.SlotStatus_SLOT_STATUS_UNSPECIFIED)
	if err != nil {
		return nil, err
	}
	labelBySlot := map[string]string{}
	for _, slot := range slots {
		labelBySlot[slot.GetId()] = slot.GetLabel()
	}

	leases, err := s.listLeases(ctx, owner, "", false, time.Now().UTC().Unix())
	if err != nil {
		return nil, err
	}

	out := make([]SlotLease, 0, len(leases))
	for _, lease := range leases {
		slotID := lease.GetSlotId()
		slotLabel := labelBySlot[slotID]
		if slotLabel == "" {
			continue
		}
		ui := leaseFromChain(&lease)
		ui.SlotLabel = slotLabel
		ui.SlotID = slotID
		out = append(out, *ui)
	}
	return out, nil
}

func (s *chainSlotStore) GetSlot(ctx context.Context, slotID string) (Slot, error) {
	slotID = strings.TrimSpace(slotID)
	if slotID == "" {
		return Slot{}, fmt.Errorf("slot id required")
	}
	slots, err := s.listSlots(ctx, "", registrypb.SlotStatus_SLOT_STATUS_UNSPECIFIED)
	if err != nil {
		return Slot{}, err
	}
	for _, slot := range slots {
		if slot.GetId() == slotID {
			return slotFromChain(slot), nil
		}
	}
	return Slot{}, fmt.Errorf("slot not found")
}

func (s *chainSlotStore) listSlots(ctx context.Context, publisher string, status registrypb.SlotStatus) ([]registrypb.Slot, error) {
	var out []registrypb.Slot
	var pageKey []byte
	for {
		resp, err := s.queryClient.Slots(ctx, &registrypb.QuerySlotsRequest{
			Publisher: publisher,
			Status:    status,
			Pagination: &query.PageRequest{
				Key:   pageKey,
				Limit: 200,
			},
		})
		if err != nil {
			return nil, err
		}
		for _, slot := range resp.GetSlots() {
			if slot == nil {
				continue
			}
			out = append(out, *slot)
		}
		pageKey = resp.GetPagination().GetNextKey()
		if len(pageKey) == 0 {
			break
		}
	}
	return out, nil
}

func (s *chainSlotStore) listLeases(ctx context.Context, publisher, slotID string, activeOnly bool, atUnix int64) ([]registrypb.SlotLease, error) {
	var out []registrypb.SlotLease
	var pageKey []byte
	for {
		resp, err := s.queryClient.Leases(ctx, &registrypb.QueryLeasesRequest{
			Publisher:  publisher,
			SlotId:     slotID,
			ActiveOnly: activeOnly,
			AtUnix:     atUnix,
			Pagination: &query.PageRequest{
				Key:   pageKey,
				Limit: 200,
			},
		})
		if err != nil {
			return nil, err
		}
		for _, lease := range resp.GetLeases() {
			if lease == nil {
				continue
			}
			out = append(out, *lease)
		}
		pageKey = resp.GetPagination().GetNextKey()
		if len(pageKey) == 0 {
			break
		}
	}
	return out, nil
}

func (s *chainSlotStore) publisherOwner(ctx context.Context, domain string) (string, error) {
	resp, err := s.queryClient.Publisher(ctx, &registrypb.QueryPublisherRequest{Domain: normalizePublisher(domain)})
	if err != nil {
		return "", err
	}
	if resp.GetWebsite() == nil {
		return "", nil
	}
	return resp.GetWebsite().GetOwner(), nil
}

func slotFromChain(slot registrypb.Slot) Slot {
	status := SlotStatusListed
	switch slot.GetStatus() {
	case registrypb.SlotStatus_SLOT_STATUS_PAUSED:
		status = SlotStatusPaused
	case registrypb.SlotStatus_SLOT_STATUS_UNLISTED:
		status = SlotStatusUnlisted
	}

	rate := intFromString(slot.GetRateAmount())
	return Slot{
		ID:                 slot.GetId(),
		Publisher:          normalizePublisher(slot.GetDomain()),
		PublisherName:      normalizePublisher(slot.GetDomain()),
		Label:              slot.GetLabel(),
		Summary:            slot.GetSummary(),
		Category:           slot.GetCategory(),
		Placement:          slot.GetPlacement(),
		Size:               slot.GetSize(),
		Rate:               rate,
		UnitSeconds:        slot.GetUnitSeconds(),
		MinDurationSeconds: slot.GetMinDurationSeconds(),
		MaxDurationSeconds: slot.GetMaxDurationSeconds(),
		Status:             status,
		UpdatedAt:          time.Unix(slot.GetUpdatedAtUnix(), 0).UTC(),
		Tags:               append([]string(nil), slot.GetTags()...),
	}
}

func leaseFromChain(lease *registrypb.SlotLease) *SlotLease {
	if lease == nil {
		return nil
	}
	return &SlotLease{
		LeaseID:   lease.GetId(),
		SlotID:    lease.GetSlotId(),
		Lessee:    lease.GetLessee(),
		TargetURL: lease.GetTargetUrl(),
		StartsAt:  time.Unix(lease.GetStartsAtUnix(), 0).UTC(),
		EndsAt:    time.Unix(lease.GetEndsAtUnix(), 0).UTC(),
		Rate:      intFromString(lease.GetRateAmount()),
	}
}

func intFromString(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	if parsed > int64(^uint(0)>>1) {
		return int(^uint(0) >> 1)
	}
	if parsed < 0 {
		return 0
	}
	return int(parsed)
}
