package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	registrypb "content-grid-chain/x/registry/typespb"

	sdk "github.com/cosmos/cosmos-sdk/types"
	query "github.com/cosmos/cosmos-sdk/types/query"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type chainSlotConfig struct {
	ChainID        string
	NodeRPC        string
	GRPCAddr       string
	KeyName        string
	KeyringBackend string
	KeyringDir     string
	Home           string
	Fees           string
	GasPrices      string
	Gas            string
	GasAdjustment  string
	Binary         string
	LeaseKeyName   string

	RateDenom          string
	UnitSeconds        int64
	MinDurationSeconds int64
	MaxDurationSeconds int64
}

type chainSlotStore struct {
	cfg             chainSlotConfig
	conn            *grpc.ClientConn
	queryClient     registrypb.QueryClient
	keyAddress      string
	leaseKeyAddress string
}

func newChainSlotStore(cfg chainSlotConfig) (*chainSlotStore, error) {
	if strings.TrimSpace(cfg.GRPCAddr) == "" {
		return nil, fmt.Errorf("grpc address required")
	}
	if strings.TrimSpace(cfg.KeyName) == "" {
		return nil, fmt.Errorf("slot key name required")
	}
	if strings.TrimSpace(cfg.ChainID) == "" {
		return nil, fmt.Errorf("chain id required")
	}
	if strings.TrimSpace(cfg.NodeRPC) == "" {
		return nil, fmt.Errorf("node rpc required")
	}
	if strings.TrimSpace(cfg.Binary) == "" {
		cfg.Binary = "./content-grid-d"
	}
	if strings.TrimSpace(cfg.RateDenom) == "" {
		cfg.RateDenom = "ucongrid"
	}
	if cfg.UnitSeconds <= 0 {
		cfg.UnitSeconds = 7 * 24 * 60 * 60
	}
	if cfg.MinDurationSeconds <= 0 {
		cfg.MinDurationSeconds = cfg.UnitSeconds
	}
	if cfg.MaxDurationSeconds <= 0 {
		cfg.MaxDurationSeconds = 90 * 24 * 60 * 60
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
	addr, err := store.resolveKeyAddress(context.Background(), cfg.KeyName)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	store.keyAddress = addr
	leaseKey := cfg.LeaseKeyName
	if strings.TrimSpace(leaseKey) == "" {
		leaseKey = cfg.KeyName
	}
	leaseAddr, err := store.resolveKeyAddress(context.Background(), leaseKey)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	store.leaseKeyAddress = leaseAddr
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

func (s *chainSlotStore) CreateSlot(ctx context.Context, publisher string, input CreateSlotInput) (Slot, error) {
	owner, err := s.publisherOwner(ctx, publisher)
	if err != nil {
		return Slot{}, err
	}
	if owner == "" {
		return Slot{}, fmt.Errorf("publisher not found")
	}
	if err := s.ensureSigner(owner); err != nil {
		return Slot{}, err
	}

	label := strings.TrimSpace(input.Label)
	if label == "" {
		return Slot{}, fmt.Errorf("label required")
	}

	rateAmount := input.Rate
	rateStr := strconv.Itoa(rateAmount)

	args := []string{
		"tx", "registry", "create-slot",
		"--domain", normalizePublisher(publisher),
		"--label", label,
		"--summary", strings.TrimSpace(input.Summary),
		"--category", strings.TrimSpace(input.Category),
		"--placement", strings.TrimSpace(input.Placement),
		"--size", strings.TrimSpace(input.Size),
		"--rate-denom", s.cfg.RateDenom,
		"--rate-amount", rateStr,
		"--unit-seconds", strconv.FormatInt(s.cfg.UnitSeconds, 10),
		"--min-duration-seconds", strconv.FormatInt(s.cfg.MinDurationSeconds, 10),
		"--max-duration-seconds", strconv.FormatInt(s.cfg.MaxDurationSeconds, 10),
		"--from", s.cfg.KeyName,
		"--chain-id", s.cfg.ChainID,
		"--node", s.cfg.NodeRPC,
		"--keyring-backend", s.cfg.KeyringBackend,
		"--output", "json",
		"-y",
	}
	if strings.TrimSpace(s.cfg.KeyringDir) != "" {
		args = append(args, "--keyring-dir", s.cfg.KeyringDir)
	}
	if strings.TrimSpace(s.cfg.Gas) != "" {
		args = append(args, "--gas", s.cfg.Gas)
	}
	if strings.TrimSpace(s.cfg.GasAdjustment) != "" {
		args = append(args, "--gas-adjustment", s.cfg.GasAdjustment)
	}
	if strings.TrimSpace(s.cfg.Fees) != "" {
		args = append(args, "--fees", s.cfg.Fees)
	}
	if strings.TrimSpace(s.cfg.GasPrices) != "" {
		args = append(args, "--gas-prices", s.cfg.GasPrices)
	}
	for _, tag := range input.Tags {
		clean := strings.TrimSpace(tag)
		if clean == "" {
			continue
		}
		args = append(args, "--tags", clean)
	}

	if err := s.execTx(ctx, args); err != nil {
		return Slot{}, err
	}

	// Return best-effort slot info; list reload will fetch canonical data.
	return Slot{
		Publisher:     normalizePublisher(publisher),
		PublisherName: normalizePublisher(publisher),
		Label:         label,
		Summary:       strings.TrimSpace(input.Summary),
		Category:      strings.TrimSpace(input.Category),
		Placement:     strings.TrimSpace(input.Placement),
		Size:          strings.TrimSpace(input.Size),
		Rate:          rateAmount,
		Status:        SlotStatusPaused,
		UpdatedAt:     time.Now().UTC(),
		Tags:          input.Tags,
	}, nil
}

func (s *chainSlotStore) UpdateSlotStatus(ctx context.Context, publisher, slotID string, status SlotStatus) error {
	owner, err := s.publisherOwner(ctx, publisher)
	if err != nil {
		return err
	}
	if owner == "" {
		return fmt.Errorf("publisher not found")
	}
	if err := s.ensureSigner(owner); err != nil {
		return err
	}

	var chainStatus registrypb.SlotStatus
	switch status {
	case SlotStatusListed:
		chainStatus = registrypb.SlotStatus_SLOT_STATUS_LISTED
	case SlotStatusPaused:
		chainStatus = registrypb.SlotStatus_SLOT_STATUS_PAUSED
	case SlotStatusUnlisted:
		chainStatus = registrypb.SlotStatus_SLOT_STATUS_UNLISTED
	default:
		return fmt.Errorf("invalid slot status")
	}

	args := []string{
		"tx", "registry", "update-slot-status",
		"--slot-id", strings.TrimSpace(slotID),
		"--status", chainStatus.String(),
		"--from", s.cfg.KeyName,
		"--chain-id", s.cfg.ChainID,
		"--node", s.cfg.NodeRPC,
		"--keyring-backend", s.cfg.KeyringBackend,
		"--output", "json",
		"-y",
	}
	if strings.TrimSpace(s.cfg.KeyringDir) != "" {
		args = append(args, "--keyring-dir", s.cfg.KeyringDir)
	}
	if strings.TrimSpace(s.cfg.Gas) != "" {
		args = append(args, "--gas", s.cfg.Gas)
	}
	if strings.TrimSpace(s.cfg.GasAdjustment) != "" {
		args = append(args, "--gas-adjustment", s.cfg.GasAdjustment)
	}
	if strings.TrimSpace(s.cfg.Fees) != "" {
		args = append(args, "--fees", s.cfg.Fees)
	}
	if strings.TrimSpace(s.cfg.GasPrices) != "" {
		args = append(args, "--gas-prices", s.cfg.GasPrices)
	}

	return s.execTx(ctx, args)
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

func (s *chainSlotStore) CreateLease(ctx context.Context, slotID string, input CreateLeaseInput) (SlotLease, error) {
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
	if s.leaseKeyAddress == "" {
		return SlotLease{}, fmt.Errorf("lease signer not configured")
	}

	now := time.Now().UTC()
	startsAt := input.StartsAt
	if startsAt.IsZero() {
		// Give the chain a small buffer so starts_at_unix is not interpreted as "in the past".
		startsAt = now.Add(5 * time.Second)
	}
	if startsAt.Before(now) {
		return SlotLease{}, fmt.Errorf("start date must be in the future")
	}

	leaseKeyName := strings.TrimSpace(s.cfg.LeaseKeyName)
	if leaseKeyName == "" {
		leaseKeyName = s.cfg.KeyName
	}
	args := []string{
		"tx", "registry", "lease-slot",
		"--slot-id", slotID,
		"--target-url", strings.TrimSpace(input.TargetURL),
		"--starts-at-unix", strconv.FormatInt(startsAt.Unix(), 10),
		"--duration-seconds", strconv.FormatInt(input.DurationSeconds, 10),
		"--from", leaseKeyName,
		"--chain-id", s.cfg.ChainID,
		"--node", s.cfg.NodeRPC,
		"--keyring-backend", s.cfg.KeyringBackend,
		"--output", "json",
		"-y",
	}
	if strings.TrimSpace(s.cfg.KeyringDir) != "" {
		args = append(args, "--keyring-dir", s.cfg.KeyringDir)
	}
	if strings.TrimSpace(s.cfg.Gas) != "" {
		args = append(args, "--gas", s.cfg.Gas)
	}
	if strings.TrimSpace(s.cfg.GasAdjustment) != "" {
		args = append(args, "--gas-adjustment", s.cfg.GasAdjustment)
	}
	if strings.TrimSpace(s.cfg.Fees) != "" {
		args = append(args, "--fees", s.cfg.Fees)
	}
	if strings.TrimSpace(s.cfg.GasPrices) != "" {
		args = append(args, "--gas-prices", s.cfg.GasPrices)
	}

	txHash, err := s.execTxWithHash(ctx, args)
	if err != nil {
		return SlotLease{}, err
	}

	lease := SlotLease{
		LeaseID:   txHash,
		SlotID:    slotID,
		Lessee:    s.leaseKeyAddress,
		TargetURL: strings.TrimSpace(input.TargetURL),
		StartsAt:  startsAt,
		EndsAt:    startsAt.Add(time.Duration(input.DurationSeconds) * time.Second),
	}
	if lease.LeaseID == "" {
		lease.LeaseID = fmt.Sprintf("lease-%d", time.Now().Unix())
	}
	if slot, err := s.GetSlot(ctx, slotID); err == nil {
		lease.SlotLabel = slot.Label
		lease.Rate = slot.Rate
	}
	return lease, nil
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

func (s *chainSlotStore) resolveKeyAddress(ctx context.Context, keyName string) (string, error) {
	args := []string{"keys", "show", keyName, "-a", "--keyring-backend", s.cfg.KeyringBackend}
	if strings.TrimSpace(s.cfg.KeyringDir) != "" {
		args = append(args, "--keyring-dir", s.cfg.KeyringDir)
	}
	fullArgs := args
	if strings.TrimSpace(s.cfg.Home) != "" {
		fullArgs = append([]string{"--home", strings.TrimSpace(s.cfg.Home)}, fullArgs...)
	}
	cmd := exec.CommandContext(ctx, s.cfg.Binary, fullArgs...)
	cmd.Dir = "/home/eking/workspace/congrid.net"
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("key lookup failed: %w: %s", err, string(out))
	}
	addr := strings.TrimSpace(string(out))
	if addr == "" {
		return "", errors.New("key lookup returned empty address")
	}
	if _, err := sdk.AccAddressFromBech32(addr); err != nil {
		return "", fmt.Errorf("invalid key address: %w", err)
	}
	return addr, nil
}

func (s *chainSlotStore) ensureSigner(owner string) error {
	if s.keyAddress == "" {
		return errors.New("slot key address not configured")
	}
	if s.keyAddress != owner {
		return fmt.Errorf("publisher owner %s does not match configured key %s", owner, s.keyAddress)
	}
	return nil
}

func (s *chainSlotStore) execTx(ctx context.Context, args []string) error {
	_, err := s.execTxWithHash(ctx, args)
	return err
}

func (s *chainSlotStore) execTxWithHash(ctx context.Context, args []string) (string, error) {
	fullArgs := args
	if strings.TrimSpace(s.cfg.Home) != "" {
		fullArgs = append([]string{"--home", strings.TrimSpace(s.cfg.Home)}, fullArgs...)
	}
	cmd := exec.CommandContext(ctx, s.cfg.Binary, fullArgs...)
	cmd.Dir = "/home/eking/workspace/congrid.net"
	out, err := cmd.CombinedOutput()
	txHash, parseErr := parseTxJSONResponse(out)
	if err != nil {
		if parseErr != nil {
			return "", fmt.Errorf("tx failed: %w: %w", err, parseErr)
		}
		return "", fmt.Errorf("tx failed: %w", err)
	}
	if parseErr != nil {
		return "", parseErr
	}
	return txHash, nil
}

func parseTxJSONResponse(out []byte) (string, error) {
	raw := string(out)
	lines := strings.Split(raw, "\n")

	var txLine string
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "{") && strings.HasSuffix(line, "}") {
			txLine = line
			break
		}
	}
	if txLine == "" {
		return "", fmt.Errorf("missing json tx response: %s", raw)
	}

	var resp struct {
		Code   int    `json:"code"`
		RawLog string `json:"raw_log"`
		TxHash string `json:"txhash"`
	}
	if err := json.Unmarshal([]byte(txLine), &resp); err != nil {
		return "", fmt.Errorf("decode tx response: %w: %s", err, raw)
	}
	if resp.Code != 0 {
		return "", fmt.Errorf("tx failed code=%d: %s", resp.Code, strings.TrimSpace(resp.RawLog))
	}
	txHash := strings.TrimSpace(resp.TxHash)
	if txHash == "" {
		return "", fmt.Errorf("missing txhash in tx response")
	}
	return txHash, nil
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
