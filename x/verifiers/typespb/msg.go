package typespb

import (
	"fmt"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

var (
	_ sdk.Msg = (*MsgBond)(nil)
	_ sdk.Msg = (*MsgUnbond)(nil)
)

func (m *MsgBond) ValidateBasic() error {
	if m == nil {
		return fmt.Errorf("message cannot be nil")
	}
	if _, err := sdk.AccAddressFromBech32(strings.TrimSpace(m.Verifier)); err != nil {
		return fmt.Errorf("invalid verifier address: %w", err)
	}
	if strings.TrimSpace(m.Denom) == "" {
		return fmt.Errorf("denom required")
	}
	if strings.TrimSpace(m.Amount) == "" {
		return fmt.Errorf("amount required")
	}
	return nil
}

func (m *MsgBond) GetSigners() []sdk.AccAddress {
	addr, err := sdk.AccAddressFromBech32(m.Verifier)
	if err != nil {
		panic(err)
	}
	return []sdk.AccAddress{addr}
}

func (m *MsgUnbond) ValidateBasic() error {
	if m == nil {
		return fmt.Errorf("message cannot be nil")
	}
	if _, err := sdk.AccAddressFromBech32(strings.TrimSpace(m.Verifier)); err != nil {
		return fmt.Errorf("invalid verifier address: %w", err)
	}
	if strings.TrimSpace(m.Denom) == "" {
		return fmt.Errorf("denom required")
	}
	if strings.TrimSpace(m.Amount) == "" {
		return fmt.Errorf("amount required")
	}
	return nil
}

func (m *MsgUnbond) GetSigners() []sdk.AccAddress {
	addr, err := sdk.AccAddressFromBech32(m.Verifier)
	if err != nil {
		panic(err)
	}
	return []sdk.AccAddress{addr}
}
