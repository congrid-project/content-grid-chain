package verifiers

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"content-grid-chain/x/tokenomics"
	typespb "content-grid-chain/x/verifiers/typespb"
)

// Verifier represents an address bonded into the verification system.
type Verifier struct {
	Address string         `json:"address"`
	Bond    sdk.Coin       `json:"bond"`
	Status  VerifierStatus `json:"status"`
}

type VerifierStatus int32

const (
	StatusUnspecified VerifierStatus = 0
	StatusActive      VerifierStatus = 1
)

func (s VerifierStatus) String() string {
	switch s {
	case StatusActive:
		return "ACTIVE"
	default:
		return "UNSPECIFIED"
	}
}

// Params controls denom and minimum bond.
type Params struct {
	BondDenom string      `json:"bond_denom"`
	MinBond   sdkmath.Int `json:"min_bond"`
}

func DefaultParams() Params {
	return Params{BondDenom: tokenomics.DefaultDenom, MinBond: sdkmath.NewInt(1)}
}

func (p Params) Validate() error {
	if strings.TrimSpace(p.BondDenom) == "" {
		return fmt.Errorf("bond denom required")
	}
	if err := sdk.ValidateDenom(p.BondDenom); err != nil {
		return fmt.Errorf("invalid bond denom: %w", err)
	}
	if p.MinBond.IsNegative() {
		return fmt.Errorf("min bond must be >= 0")
	}
	return nil
}

func ValidateVerifier(v Verifier, params Params) error {
	if _, err := sdk.AccAddressFromBech32(v.Address); err != nil {
		return fmt.Errorf("invalid verifier address: %w", err)
	}
	if !v.Bond.IsValid() {
		return errors.New("invalid bond coin")
	}
	if v.Bond.Denom == "" {
		return errors.New("bond denom required")
	}
	if v.Bond.Denom != params.BondDenom {
		return fmt.Errorf("bond denom must be %s", params.BondDenom)
	}
	if v.Bond.Amount.IsNegative() {
		return errors.New("bond amount must be >= 0")
	}
	if v.Bond.Amount.LT(params.MinBond) {
		return fmt.Errorf("bond must be >= %s", params.MinBond.String())
	}
	if v.Status != StatusActive {
		return fmt.Errorf("invalid verifier status: %d", v.Status)
	}
	return nil
}

func (v Verifier) ToProto() *typespb.Verifier {
	return &typespb.Verifier{
		Address:    v.Address,
		BondDenom:  v.Bond.Denom,
		BondAmount: v.Bond.Amount.String(),
		Status:     typespb.VerifierStatus(v.Status),
	}
}

func VerifierFromProto(pb *typespb.Verifier) (Verifier, error) {
	if pb == nil {
		return Verifier{}, fmt.Errorf("nil verifier")
	}
	amt, ok := sdkmath.NewIntFromString(pb.GetBondAmount())
	if !ok {
		return Verifier{}, fmt.Errorf("invalid bond amount")
	}
	return Verifier{
		Address: pb.GetAddress(),
		Bond:    sdk.NewCoin(pb.GetBondDenom(), amt),
		Status:  VerifierStatus(pb.GetStatus()),
	}, nil
}

func marshalVerifier(v Verifier) ([]byte, error) { return json.Marshal(v) }

func unmarshalVerifier(b []byte) (Verifier, error) {
	var v Verifier
	return v, json.Unmarshal(b, &v)
}

// GenesisState defines module genesis.
type GenesisState struct {
	Verifiers []Verifier `json:"verifiers"`
	Params    Params     `json:"params"`
}

func DefaultGenesis() *GenesisState {
	return &GenesisState{Verifiers: []Verifier{}, Params: DefaultParams()}
}

func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return err
	}
	seen := map[string]struct{}{}
	for _, v := range gs.Verifiers {
		if _, ok := seen[v.Address]; ok {
			return fmt.Errorf("duplicate verifier: %s", v.Address)
		}
		seen[v.Address] = struct{}{}
		if err := ValidateVerifier(v, gs.Params); err != nil {
			return err
		}
	}
	return nil
}
