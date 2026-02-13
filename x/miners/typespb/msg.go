package typespb

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

var (
	_ sdk.Msg = (*MsgRegisterMiner)(nil)
	_ sdk.Msg = (*MsgUpdateMiner)(nil)
	_ sdk.Msg = (*MsgUpdateMinerStake)(nil)
)

func (m *MsgRegisterMiner) ValidateBasic() error {
	if m == nil {
		return fmt.Errorf("message cannot be nil")
	}
	if _, err := sdk.AccAddressFromBech32(m.Operator); err != nil {
		return fmt.Errorf("invalid operator: %w", err)
	}
	if m.Services == 0 {
		return fmt.Errorf("services required")
	}
	if m.MinBidAmount == "" {
		return fmt.Errorf("min bid amount required")
	}
	if m.Stake == "" {
		return fmt.Errorf("stake amount required")
	}
	return nil
}

func (m *MsgRegisterMiner) GetSigners() []sdk.AccAddress {
	addr, err := sdk.AccAddressFromBech32(m.Operator)
	if err != nil {
		panic(err)
	}
	return []sdk.AccAddress{addr}
}

func (m *MsgUpdateMiner) ValidateBasic() error {
	if m == nil {
		return fmt.Errorf("message cannot be nil")
	}
	if _, err := sdk.AccAddressFromBech32(m.Operator); err != nil {
		return fmt.Errorf("invalid operator: %w", err)
	}
	return nil
}

func (m *MsgUpdateMiner) GetSigners() []sdk.AccAddress {
	addr, err := sdk.AccAddressFromBech32(m.Operator)
	if err != nil {
		panic(err)
	}
	return []sdk.AccAddress{addr}
}

func (m *MsgUpdateMinerStake) ValidateBasic() error {
	if m == nil {
		return fmt.Errorf("message cannot be nil")
	}
	if _, err := sdk.AccAddressFromBech32(m.Operator); err != nil {
		return fmt.Errorf("invalid operator: %w", err)
	}
	if m.StakeDelta == "" {
		return fmt.Errorf("stake delta required")
	}
	return nil
}

func (m *MsgUpdateMinerStake) GetSigners() []sdk.AccAddress {
	addr, err := sdk.AccAddressFromBech32(m.Operator)
	if err != nil {
		panic(err)
	}
	return []sdk.AccAddress{addr}
}
