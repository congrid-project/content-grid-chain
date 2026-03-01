package registry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"

	errorsmod "cosmossdk.io/errors"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	typespb "content-grid-chain/x/registry/typespb"
)

type msgServer struct {
	keeper Keeper
	typespb.UnimplementedMsgServer
}

// NewMsgServerImpl wires the gRPC Msg service.
func NewMsgServerImpl(keeper Keeper) typespb.MsgServer {
	return msgServer{keeper: keeper}
}

// RegisterPublisher handles on-chain publisher registrations.
func (m msgServer) RegisterPublisher(ctx context.Context, msg *typespb.MsgRegisterPublisher) (*typespb.MsgRegisterPublisherResponse, error) {
	if msg == nil {
		return nil, fmt.Errorf("message cannot be nil")
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)

	website := Website{
		Domain:      NormalizeDomain(msg.GetDomain()),
		Owner:       msg.GetOwner(),
		Status:      StatusPending,
		MetadataURI: strings.TrimSpace(msg.GetMetadataUri()),
		Proof:       strings.TrimSpace(msg.GetProof()),
		Verifier:    strings.TrimSpace(msg.GetVerifier()),
		Referrer:    strings.TrimSpace(msg.GetReferrer()),
	}

	var (
		registered Website
		err        error
	)

	if existing, found := m.keeper.GetWebsite(sdkCtx, website.Domain); found {
		// Allow re-registration for anti-squatting recovery only when a publisher is still
		// pending and has already failed at least one finalized verification round.
		if existing.Status != StatusPending {
			return nil, fmt.Errorf("%w: %s", ErrWebsiteExists, website.Domain)
		}
		if m.keeper.GetPublisherFailureStreak(sdkCtx, website.Domain) < 1 {
			return nil, fmt.Errorf("pending publisher %s is not yet eligible for re-registration", website.Domain)
		}
		website.RegisteredAtHeight = sdkCtx.BlockHeight()
		website.CooldownCount = 0
		website.CooldownUntilUnix = 0
		if err := ValidateWebsite(website); err != nil {
			return nil, err
		}
		if err := m.keeper.UpsertWebsite(sdkCtx, website); err != nil {
			return nil, err
		}
		m.keeper.ClearPublisherFailureStreak(sdkCtx, website.Domain)
		registered = website
	} else {
		registered, err = m.keeper.RegisterWebsite(sdkCtx, website)
		if err != nil {
			return nil, err
		}
	}

	sdkCtx.EventManager().EmitEvents(sdk.Events{
		sdk.NewEvent(
			EventTypePublisherRegistered,
			sdk.NewAttribute(AttributeKeyDomain, registered.Domain),
			sdk.NewAttribute(AttributeKeyOwner, registered.Owner),
			sdk.NewAttribute(AttributeKeyStatus, registered.Status.String()),
			sdk.NewAttribute(AttributeKeyMetadataURI, registered.MetadataURI),
			sdk.NewAttribute(AttributeKeyVerifier, registered.Verifier),
			sdk.NewAttribute(AttributeKeyReferrer, registered.Referrer),
		),
	})

	return &typespb.MsgRegisterPublisherResponse{Website: registered.ToProto()}, nil
}

// SubmitVerificationCommit stores a verifier's commit hash for an assignment.
func (m msgServer) SubmitVerificationCommit(ctx context.Context, msg *typespb.MsgSubmitVerificationCommit) (*typespb.MsgSubmitVerificationCommitResponse, error) {
	if msg == nil {
		return nil, fmt.Errorf("message cannot be nil")
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	domain := NormalizeDomain(msg.GetDomain())
	assignment, found := m.keeper.GetAssignment(sdkCtx, msg.GetRoundStartUnix(), domain)
	if !found {
		return nil, errorsmod.Wrapf(ErrAssignmentNotFound, "domain %s round %d", domain, msg.GetRoundStartUnix())
	}
	if assignment.Finalized {
		return nil, errorsmod.Wrap(ErrAssignmentClosed, "assignment already finalized")
	}
	if !assignmentHasVerifier(assignment, msg.GetVerifier()) {
		return nil, errorsmod.Wrapf(ErrVerifierNotAssigned, "verifier %s not assigned", msg.GetVerifier())
	}

	params := m.keeper.GetParams(sdkCtx)
	nowUnix := sdkCtx.BlockTime().UTC().Unix()
	commitDeadline := assignment.StartAtUnix + params.CommitWindowSeconds
	if nowUnix < assignment.StartAtUnix || nowUnix > commitDeadline {
		return nil, errorsmod.Wrapf(ErrCommitWindowClosed, "outside commit window [%d,%d]", assignment.StartAtUnix, commitDeadline)
	}
	if _, exists := m.keeper.GetCommit(sdkCtx, msg.GetRoundStartUnix(), domain, msg.GetVerifier()); exists {
		return nil, errorsmod.Wrap(ErrAlreadyCommitted, "commit already recorded")
	}
	if _, exists := m.keeper.GetSubmission(sdkCtx, msg.GetRoundStartUnix(), domain, msg.GetVerifier()); exists {
		return nil, errorsmod.Wrap(ErrAlreadySubmitted, "submission already recorded")
	}

	commitHash := strings.ToLower(strings.TrimSpace(msg.GetCommitHash()))
	m.keeper.SetCommit(sdkCtx, msg.GetRoundStartUnix(), domain, msg.GetVerifier(), commitHash)

	sdkCtx.EventManager().EmitEvent(
		sdk.NewEvent(
			EventTypeVerificationCommitted,
			sdk.NewAttribute(AttributeKeyDomain, domain),
			sdk.NewAttribute(AttributeKeyVerifier, msg.GetVerifier()),
			sdk.NewAttribute(AttributeKeyRoundStart, fmt.Sprintf("%d", msg.GetRoundStartUnix())),
			sdk.NewAttribute(AttributeKeyCommitHash, commitHash),
		),
	)

	return &typespb.MsgSubmitVerificationCommitResponse{}, nil
}

// RevealVerification stores a verifier's revealed result after commit.
func (m msgServer) RevealVerification(ctx context.Context, msg *typespb.MsgRevealVerification) (*typespb.MsgRevealVerificationResponse, error) {
	if msg == nil {
		return nil, fmt.Errorf("message cannot be nil")
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	domain := NormalizeDomain(msg.GetDomain())
	assignment, found := m.keeper.GetAssignment(sdkCtx, msg.GetRoundStartUnix(), domain)
	if !found {
		return nil, errorsmod.Wrapf(ErrAssignmentNotFound, "domain %s round %d", domain, msg.GetRoundStartUnix())
	}
	if assignment.Finalized {
		return nil, errorsmod.Wrap(ErrAssignmentClosed, "assignment already finalized")
	}
	if !assignmentHasVerifier(assignment, msg.GetVerifier()) {
		return nil, errorsmod.Wrapf(ErrVerifierNotAssigned, "verifier %s not assigned", msg.GetVerifier())
	}

	params := m.keeper.GetParams(sdkCtx)
	nowUnix := sdkCtx.BlockTime().UTC().Unix()
	commitDeadline := assignment.StartAtUnix + params.CommitWindowSeconds
	if nowUnix <= commitDeadline {
		return nil, errorsmod.Wrapf(ErrRevealWindowNotOpen, "reveal window opens after %d", commitDeadline)
	}
	if nowUnix > assignment.DeadlineUnix {
		return nil, errorsmod.Wrapf(ErrAssignmentClosed, "outside reveal window (%d,%d]", commitDeadline, assignment.DeadlineUnix)
	}
	if _, exists := m.keeper.GetSubmission(sdkCtx, msg.GetRoundStartUnix(), domain, msg.GetVerifier()); exists {
		return nil, errorsmod.Wrap(ErrAlreadySubmitted, "submission already recorded")
	}
	commitHash, exists := m.keeper.GetCommit(sdkCtx, msg.GetRoundStartUnix(), domain, msg.GetVerifier())
	if !exists {
		return nil, errorsmod.Wrap(ErrCommitNotFound, "no commit recorded")
	}

	evidenceHash := strings.TrimSpace(msg.GetEvidenceHash())
	nonce := strings.TrimSpace(msg.GetNonce())
	expectedHash := typespb.ComputeVerificationCommitHash(domain, msg.GetRoundStartUnix(), msg.GetVerifier(), msg.GetPassed(), evidenceHash, nonce)
	if strings.ToLower(commitHash) != expectedHash {
		return nil, errorsmod.Wrapf(ErrCommitMismatch, "commit hash mismatch")
	}

	submission := PublisherVerificationSubmission{
		RoundStartUnix:  msg.GetRoundStartUnix(),
		Domain:          domain,
		Verifier:        msg.GetVerifier(),
		Passed:          msg.GetPassed(),
		ObservedAtUnix:  nowUnix,
		LatencyMs:       0,
		SubmittedAtUnix: nowUnix,
	}
	if err := m.keeper.SetSubmission(sdkCtx, submission); err != nil {
		return nil, err
	}
	m.keeper.DeleteCommit(sdkCtx, msg.GetRoundStartUnix(), domain, msg.GetVerifier())

	sdkCtx.EventManager().EmitEvent(
		sdk.NewEvent(
			EventTypeVerificationRevealed,
			sdk.NewAttribute(AttributeKeyDomain, submission.Domain),
			sdk.NewAttribute(AttributeKeyVerifier, submission.Verifier),
			sdk.NewAttribute(AttributeKeyRoundStart, fmt.Sprintf("%d", submission.RoundStartUnix)),
			sdk.NewAttribute(AttributeKeyVerified, fmt.Sprintf("%t", submission.Passed)),
			sdk.NewAttribute(AttributeKeyEvidenceHash, evidenceHash),
		),
	)

	return &typespb.MsgRevealVerificationResponse{Submission: submission.ToProto()}, nil
}

// CreateSlot handles new slot listings from publishers.
func (m msgServer) CreateSlot(ctx context.Context, msg *typespb.MsgCreateSlot) (*typespb.MsgCreateSlotResponse, error) {
	if msg == nil {
		return nil, fmt.Errorf("message cannot be nil")
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	nowUnix := sdkCtx.BlockTime().UTC().Unix()
	publisher := strings.TrimSpace(msg.GetPublisher())
	domain := NormalizeDomain(msg.GetDomain())

	website, found := m.keeper.GetWebsite(sdkCtx, domain)
	if !found {
		return nil, errorsmod.Wrapf(ErrPublisherNotFound, "publisher %s", domain)
	}
	if website.Owner != publisher {
		return nil, errorsmod.Wrap(ErrSlotUnauthorized, "publisher does not own domain")
	}
	if website.Status != StatusVerified {
		return nil, errorsmod.Wrap(ErrInvalidPublisherRequest, "publisher not verified")
	}
	if website.CooldownUntilUnix > nowUnix {
		return nil, errorsmod.Wrapf(ErrPublisherInCooldown, "cooldown until %d", website.CooldownUntilUnix)
	}

	rateAmountStr := strings.TrimSpace(msg.GetRateAmount())
	if rateAmountStr == "" {
		return nil, errorsmod.Wrap(ErrInvalidPublisherRequest, "rate amount required")
	}
	rateAmount, ok := sdkmath.NewIntFromString(rateAmountStr)
	if !ok {
		return nil, errorsmod.Wrap(ErrInvalidPublisherRequest, "invalid rate amount")
	}

	slot := Slot{
		ID:                 m.keeper.nextSlotID(sdkCtx),
		Publisher:          publisher,
		Domain:             domain,
		Label:              strings.TrimSpace(msg.GetLabel()),
		Summary:            strings.TrimSpace(msg.GetSummary()),
		Category:           strings.TrimSpace(msg.GetCategory()),
		Placement:          strings.TrimSpace(msg.GetPlacement()),
		Size:               strings.TrimSpace(msg.GetSize()),
		RateDenom:          strings.TrimSpace(msg.GetRateDenom()),
		RateAmount:         rateAmount,
		UnitSeconds:        msg.GetUnitSeconds(),
		MinDurationSeconds: msg.GetMinDurationSeconds(),
		MaxDurationSeconds: msg.GetMaxDurationSeconds(),
		Status:             SlotStatusPaused,
		CreatedAtUnix:      nowUnix,
		UpdatedAtUnix:      nowUnix,
		Tags:               sanitizeTags(msg.GetTags()),
	}

	if err := m.keeper.SetSlot(sdkCtx, slot); err != nil {
		return nil, err
	}

	sdkCtx.EventManager().EmitEvent(
		sdk.NewEvent(
			EventTypeSlotCreated,
			sdk.NewAttribute(AttributeKeyDomain, slot.Domain),
			sdk.NewAttribute(AttributeKeyOwner, slot.Publisher),
			sdk.NewAttribute(AttributeKeySlotID, slot.ID),
			sdk.NewAttribute(AttributeKeySlotStatus, slot.Status.String()),
		),
	)

	return &typespb.MsgCreateSlotResponse{Slot: slot.ToProto()}, nil
}

// UpdateSlotStatus updates the listing status for a slot.
func (m msgServer) UpdateSlotStatus(ctx context.Context, msg *typespb.MsgUpdateSlotStatus) (*typespb.MsgUpdateSlotStatusResponse, error) {
	if msg == nil {
		return nil, fmt.Errorf("message cannot be nil")
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	nowUnix := sdkCtx.BlockTime().UTC().Unix()
	publisher := strings.TrimSpace(msg.GetPublisher())

	slot, found := m.keeper.GetSlot(sdkCtx, strings.TrimSpace(msg.GetSlotId()))
	if !found {
		return nil, errorsmod.Wrap(ErrSlotNotFound, "slot not found")
	}
	if slot.Publisher != publisher {
		return nil, errorsmod.Wrap(ErrSlotUnauthorized, "publisher does not own slot")
	}

	status := SlotStatus(msg.GetStatus())
	if status == SlotStatusUnspecified {
		return nil, errorsmod.Wrap(ErrInvalidPublisherRequest, "slot status required")
	}

	slot.Status = status
	slot.UpdatedAtUnix = nowUnix

	if err := m.keeper.SetSlot(sdkCtx, slot); err != nil {
		return nil, err
	}

	sdkCtx.EventManager().EmitEvent(
		sdk.NewEvent(
			EventTypeSlotStatusUpdated,
			sdk.NewAttribute(AttributeKeyDomain, slot.Domain),
			sdk.NewAttribute(AttributeKeyOwner, slot.Publisher),
			sdk.NewAttribute(AttributeKeySlotID, slot.ID),
			sdk.NewAttribute(AttributeKeySlotStatus, slot.Status.String()),
		),
	)

	return &typespb.MsgUpdateSlotStatusResponse{Slot: slot.ToProto()}, nil
}

// LeaseSlot creates a new lease and escrows payment.
func (m msgServer) LeaseSlot(ctx context.Context, msg *typespb.MsgLeaseSlot) (*typespb.MsgLeaseSlotResponse, error) {
	if msg == nil {
		return nil, fmt.Errorf("message cannot be nil")
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	nowUnix := sdkCtx.BlockTime().UTC().Unix()

	slot, found := m.keeper.GetSlot(sdkCtx, strings.TrimSpace(msg.GetSlotId()))
	if !found {
		return nil, errorsmod.Wrap(ErrSlotNotFound, "slot not found")
	}
	if slot.Status != SlotStatusListed {
		return nil, errorsmod.Wrap(ErrSlotNotListed, "slot not listed")
	}

	website, found := m.keeper.GetWebsite(sdkCtx, slot.Domain)
	if !found {
		return nil, errorsmod.Wrapf(ErrPublisherNotFound, "publisher %s", slot.Domain)
	}
	if website.Status != StatusVerified {
		return nil, errorsmod.Wrap(ErrInvalidPublisherRequest, "publisher not verified")
	}

	startsAtUnix := msg.GetStartsAtUnix()
	if startsAtUnix <= 0 {
		startsAtUnix = nowUnix
	}
	if startsAtUnix < nowUnix {
		return nil, errorsmod.Wrap(ErrLeaseInvalidDuration, "starts_at_unix must be >= now")
	}

	if website.CooldownUntilUnix > nowUnix && startsAtUnix < website.CooldownUntilUnix {
		return nil, errorsmod.Wrapf(ErrPublisherInCooldown, "cooldown until %d", website.CooldownUntilUnix)
	}

	durationSeconds := msg.GetDurationSeconds()
	if durationSeconds <= 0 {
		return nil, errorsmod.Wrap(ErrLeaseInvalidDuration, "duration must be positive")
	}
	if slot.UnitSeconds <= 0 {
		return nil, errorsmod.Wrap(ErrLeaseInvalidDuration, "slot unit seconds invalid")
	}
	if durationSeconds%slot.UnitSeconds != 0 {
		return nil, errorsmod.Wrap(ErrLeaseInvalidDuration, "duration must be multiple of slot unit")
	}
	if durationSeconds < slot.MinDurationSeconds || durationSeconds > slot.MaxDurationSeconds {
		return nil, errorsmod.Wrap(ErrLeaseInvalidDuration, "duration outside slot bounds")
	}

	endsAtUnix := startsAtUnix + durationSeconds
	if endsAtUnix <= startsAtUnix {
		return nil, errorsmod.Wrap(ErrLeaseInvalidDuration, "invalid end time")
	}

	if _, err := url.ParseRequestURI(strings.TrimSpace(msg.GetTargetUrl())); err != nil {
		return nil, errorsmod.Wrapf(ErrInvalidPublisherRequest, "invalid target url: %v", err)
	}

	leases := m.keeper.listLeasesBySlot(sdkCtx, slot.ID)
	for _, existing := range leases {
		if existing.Status != LeaseStatusActive {
			continue
		}
		if startsAtUnix < existing.EndsAtUnix && endsAtUnix > existing.StartsAtUnix {
			return nil, errorsmod.Wrap(ErrLeaseOverlap, "overlapping lease")
		}
	}

	units := durationSeconds / slot.UnitSeconds
	total := slot.RateAmount.MulRaw(units)
	if total.IsNegative() {
		return nil, errorsmod.Wrap(ErrLeaseInvalidDuration, "negative lease amount")
	}

	lesseeAddr, err := sdk.AccAddressFromBech32(strings.TrimSpace(msg.GetLessee()))
	if err != nil {
		return nil, errorsmod.Wrapf(ErrInvalidPublisherRequest, "invalid lessee address: %v", err)
	}

	if total.IsPositive() {
		coin := sdk.NewCoin(slot.RateDenom, total)
		if err := m.keeper.bank.SendCoinsFromAccountToModule(sdkCtx, lesseeAddr, ModuleName, sdk.NewCoins(coin)); err != nil {
			return nil, errorsmod.Wrapf(ErrLeasePaymentFailed, "escrow transfer failed: %v", err)
		}
	}

	lease := SlotLease{
		ID:              m.keeper.nextLeaseID(sdkCtx),
		SlotID:          slot.ID,
		Publisher:       slot.Publisher,
		Lessee:          strings.TrimSpace(msg.GetLessee()),
		TargetURL:       strings.TrimSpace(msg.GetTargetUrl()),
		StartsAtUnix:    startsAtUnix,
		EndsAtUnix:      endsAtUnix,
		RateDenom:       slot.RateDenom,
		RateAmount:      slot.RateAmount,
		UnitSeconds:     slot.UnitSeconds,
		EscrowTotal:     total,
		EscrowRemaining: total,
		PaidOut:         sdkmath.ZeroInt(),
		Status:          LeaseStatusActive,
		CreatedAtUnix:   nowUnix,
		UpdatedAtUnix:   nowUnix,
		PaidThroughUnix: 0,
	}

	if err := m.keeper.SetLease(sdkCtx, lease); err != nil {
		return nil, err
	}

	sdkCtx.EventManager().EmitEvent(
		sdk.NewEvent(
			EventTypeSlotLeased,
			sdk.NewAttribute(AttributeKeySlotID, slot.ID),
			sdk.NewAttribute(AttributeKeyLeaseID, lease.ID),
			sdk.NewAttribute(AttributeKeyOwner, lease.Publisher),
			sdk.NewAttribute(AttributeKeyPayoutAmount, total.String()),
		),
	)

	return &typespb.MsgLeaseSlotResponse{Lease: lease.ToProto()}, nil
}

// SubmitDrandBeacon stores a drand beacon used for assignment seed generation.
func (m msgServer) SubmitDrandBeacon(ctx context.Context, msg *typespb.MsgSubmitDrandBeacon) (*typespb.MsgSubmitDrandBeaconResponse, error) {
	if msg == nil {
		return nil, fmt.Errorf("message cannot be nil")
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	randomnessHex := strings.ToLower(strings.TrimSpace(msg.GetRandomnessHex()))
	signatureHex := strings.ToLower(strings.TrimSpace(msg.GetSignatureHex()))
	if len(randomnessHex) != 64 {
		return nil, errorsmod.Wrap(ErrInvalidDrandBeacon, "randomness_hex must be 64 hex chars")
	}
	if _, err := hex.DecodeString(randomnessHex); err != nil {
		return nil, errorsmod.Wrapf(ErrInvalidDrandBeacon, "invalid randomness_hex: %v", err)
	}
	if signatureHex != "" {
		if _, err := hex.DecodeString(signatureHex); err != nil {
			return nil, errorsmod.Wrapf(ErrInvalidDrandBeacon, "invalid signature_hex: %v", err)
		}
	}

	params := m.keeper.GetParams(sdkCtx)
	if params.EffectiveDrandEnabled() || params.EffectiveDrandStrictMode() {
		if signatureHex == "" {
			return nil, errorsmod.Wrap(ErrInvalidDrandBeacon, "signature_hex required when drand is enabled")
		}
		pubKeyHex := params.EffectiveDrandPublicKeyHex()
		if pubKeyHex == "" {
			return nil, errorsmod.Wrap(ErrInvalidDrandBeacon, "drand_public_key_hex not configured")
		}
		schemeID := params.EffectiveDrandSchemeID()
		if err := verifyDrandBeaconSignature(msg.GetRound(), signatureHex, randomnessHex, pubKeyHex, schemeID); err != nil {
			return nil, errorsmod.Wrapf(ErrInvalidDrandBeacon, "%v", err)
		}
	} else if signatureHex != "" {
		// For non-enforced mode, at least keep randomness-signature consistency.
		sig, _ := hex.DecodeString(signatureHex)
		computed := sha256.Sum256(sig)
		if hex.EncodeToString(computed[:]) != randomnessHex {
			return nil, errorsmod.Wrap(ErrInvalidDrandBeacon, "randomness must equal sha256(signature)")
		}
	}

	beacon := DrandBeacon{
		Round:           msg.GetRound(),
		RandomnessHex:   randomnessHex,
		SignatureHex:    signatureHex,
		SubmittedAtUnix: sdkCtx.BlockTime().UTC().Unix(),
		Submitter:       strings.TrimSpace(msg.GetSubmitter()),
	}
	if err := beacon.ValidateBasic(); err != nil {
		return nil, errorsmod.Wrap(ErrInvalidDrandBeacon, err.Error())
	}
	if err := m.keeper.SetDrandBeacon(sdkCtx, beacon); err != nil {
		return nil, err
	}

	sdkCtx.EventManager().EmitEvent(
		sdk.NewEvent(
			EventTypeDrandBeaconSubmitted,
			sdk.NewAttribute(AttributeKeyDrandRound, fmt.Sprintf("%d", beacon.Round)),
			sdk.NewAttribute(AttributeKeyDrandRandomness, beacon.RandomnessHex),
			sdk.NewAttribute(AttributeKeyOwner, beacon.Submitter),
		),
	)

	return &typespb.MsgSubmitDrandBeaconResponse{Beacon: beacon.ToProto()}, nil
}

func sanitizeTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		clean := strings.TrimSpace(t)
		if clean == "" {
			continue
		}
		out = append(out, clean)
	}
	return out
}
