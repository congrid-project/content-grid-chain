package typespb

import (
	"encoding/hex"
	"fmt"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

var (
	_ sdk.Msg = (*MsgRegisterPublisher)(nil)
	_ sdk.Msg = (*MsgSubmitVerificationCommit)(nil)
	_ sdk.Msg = (*MsgRevealVerification)(nil)
	_ sdk.Msg = (*MsgCreateSlot)(nil)
	_ sdk.Msg = (*MsgUpdateSlotStatus)(nil)
	_ sdk.Msg = (*MsgLeaseSlot)(nil)
)

// ValidateBasic performs stateless validation of MsgRegisterPublisher.
func (m *MsgRegisterPublisher) ValidateBasic() error {
	if m == nil {
		return fmt.Errorf("message cannot be nil")
	}
	if _, err := sdk.AccAddressFromBech32(m.Owner); err != nil {
		return fmt.Errorf("invalid owner address: %w", err)
	}
	if strings.TrimSpace(m.Domain) == "" {
		return fmt.Errorf("domain required")
	}
	if len(m.MetadataUri) > 512 {
		return fmt.Errorf("metadata uri too long")
	}
	if len(m.Proof) > 0 && len(m.Proof) > 256 {
		return fmt.Errorf("proof too long")
	}
	if m.Verifier != "" {
		if _, err := sdk.AccAddressFromBech32(m.Verifier); err != nil {
			return fmt.Errorf("invalid verifier address: %w", err)
		}
	}
	if strings.TrimSpace(m.Referrer) != "" {
		if _, err := sdk.AccAddressFromBech32(strings.TrimSpace(m.Referrer)); err != nil {
			return fmt.Errorf("invalid referrer address: %w", err)
		}
	}
	return nil
}

// GetSigners returns the owner as signer.
func (m *MsgRegisterPublisher) GetSigners() []sdk.AccAddress {
	addr, err := sdk.AccAddressFromBech32(m.Owner)
	if err != nil {
		panic(err)
	}
	return []sdk.AccAddress{addr}
}

// ValidateBasic performs stateless validation of MsgSubmitVerificationCommit.
func (m *MsgSubmitVerificationCommit) ValidateBasic() error {
	if m == nil {
		return fmt.Errorf("message cannot be nil")
	}
	if _, err := sdk.AccAddressFromBech32(m.Verifier); err != nil {
		return fmt.Errorf("invalid verifier address: %w", err)
	}
	if strings.TrimSpace(m.Domain) == "" {
		return fmt.Errorf("domain required")
	}
	if m.RoundStartUnix <= 0 {
		return fmt.Errorf("round start unix must be positive")
	}
	if err := validateCommitHash(m.CommitHash); err != nil {
		return err
	}
	return nil
}

// GetSigners returns the verifier as signer.
func (m *MsgSubmitVerificationCommit) GetSigners() []sdk.AccAddress {
	addr, err := sdk.AccAddressFromBech32(m.Verifier)
	if err != nil {
		panic(err)
	}
	return []sdk.AccAddress{addr}
}

// ValidateBasic performs stateless validation of MsgRevealVerification.
func (m *MsgRevealVerification) ValidateBasic() error {
	if m == nil {
		return fmt.Errorf("message cannot be nil")
	}
	if _, err := sdk.AccAddressFromBech32(m.Verifier); err != nil {
		return fmt.Errorf("invalid verifier address: %w", err)
	}
	if strings.TrimSpace(m.Domain) == "" {
		return fmt.Errorf("domain required")
	}
	if m.RoundStartUnix <= 0 {
		return fmt.Errorf("round start unix must be positive")
	}
	if strings.TrimSpace(m.Nonce) == "" {
		return fmt.Errorf("nonce required")
	}
	return nil
}

// GetSigners returns the verifier as signer.
func (m *MsgRevealVerification) GetSigners() []sdk.AccAddress {
	addr, err := sdk.AccAddressFromBech32(m.Verifier)
	if err != nil {
		panic(err)
	}
	return []sdk.AccAddress{addr}
}

func validateCommitHash(commitHash string) error {
	commitHash = strings.TrimSpace(commitHash)
	if commitHash == "" {
		return fmt.Errorf("commit_hash required")
	}
	if len(commitHash) != 64 {
		return fmt.Errorf("commit_hash must be 64 hex chars")
	}
	if _, err := hex.DecodeString(commitHash); err != nil {
		return fmt.Errorf("commit_hash must be hex: %w", err)
	}
	return nil
}

// ValidateBasic performs stateless validation of MsgCreateSlot.
func (m *MsgCreateSlot) ValidateBasic() error {
	if m == nil {
		return fmt.Errorf("message cannot be nil")
	}
	if _, err := sdk.AccAddressFromBech32(strings.TrimSpace(m.Publisher)); err != nil {
		return fmt.Errorf("invalid publisher address: %w", err)
	}
	if strings.TrimSpace(m.Domain) == "" {
		return fmt.Errorf("domain required")
	}
	if strings.TrimSpace(m.Label) == "" {
		return fmt.Errorf("label required")
	}
	if strings.TrimSpace(m.RateDenom) == "" {
		return fmt.Errorf("rate denom required")
	}
	if strings.TrimSpace(m.RateAmount) == "" {
		return fmt.Errorf("rate amount required")
	}
	if m.UnitSeconds <= 0 {
		return fmt.Errorf("unit seconds must be positive")
	}
	if m.MinDurationSeconds <= 0 || m.MaxDurationSeconds <= 0 {
		return fmt.Errorf("duration bounds must be positive")
	}
	if m.MinDurationSeconds > m.MaxDurationSeconds {
		return fmt.Errorf("min duration cannot exceed max duration")
	}
	return nil
}

// GetSigners returns the publisher as signer.
func (m *MsgCreateSlot) GetSigners() []sdk.AccAddress {
	addr, err := sdk.AccAddressFromBech32(strings.TrimSpace(m.Publisher))
	if err != nil {
		panic(err)
	}
	return []sdk.AccAddress{addr}
}

// ValidateBasic performs stateless validation of MsgUpdateSlotStatus.
func (m *MsgUpdateSlotStatus) ValidateBasic() error {
	if m == nil {
		return fmt.Errorf("message cannot be nil")
	}
	if _, err := sdk.AccAddressFromBech32(strings.TrimSpace(m.Publisher)); err != nil {
		return fmt.Errorf("invalid publisher address: %w", err)
	}
	if strings.TrimSpace(m.SlotId) == "" {
		return fmt.Errorf("slot id required")
	}
	if m.Status == SlotStatus_SLOT_STATUS_UNSPECIFIED {
		return fmt.Errorf("slot status required")
	}
	return nil
}

// GetSigners returns the publisher as signer.
func (m *MsgUpdateSlotStatus) GetSigners() []sdk.AccAddress {
	addr, err := sdk.AccAddressFromBech32(strings.TrimSpace(m.Publisher))
	if err != nil {
		panic(err)
	}
	return []sdk.AccAddress{addr}
}

// ValidateBasic performs stateless validation of MsgLeaseSlot.
func (m *MsgLeaseSlot) ValidateBasic() error {
	if m == nil {
		return fmt.Errorf("message cannot be nil")
	}
	if _, err := sdk.AccAddressFromBech32(strings.TrimSpace(m.Lessee)); err != nil {
		return fmt.Errorf("invalid lessee address: %w", err)
	}
	if strings.TrimSpace(m.SlotId) == "" {
		return fmt.Errorf("slot id required")
	}
	if strings.TrimSpace(m.TargetUrl) == "" {
		return fmt.Errorf("target url required")
	}
	if m.DurationSeconds <= 0 {
		return fmt.Errorf("duration seconds must be positive")
	}
	if m.StartsAtUnix < 0 {
		return fmt.Errorf("starts_at_unix cannot be negative")
	}
	return nil
}

// GetSigners returns the lessee as signer.
func (m *MsgLeaseSlot) GetSigners() []sdk.AccAddress {
	addr, err := sdk.AccAddressFromBech32(strings.TrimSpace(m.Lessee))
	if err != nil {
		panic(err)
	}
	return []sdk.AccAddress{addr}
}
