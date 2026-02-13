package verifiers

import (
	"context"
	"fmt"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	typespb "content-grid-chain/x/verifiers/typespb"
)

type msgServer struct {
	keeper Keeper
	typespb.UnimplementedMsgServer
}

func NewMsgServerImpl(k Keeper) typespb.MsgServer {
	return msgServer{keeper: k}
}

func (m msgServer) Bond(goCtx context.Context, msg *typespb.MsgBond) (*typespb.MsgBondResponse, error) {
	if msg == nil {
		return nil, fmt.Errorf("message cannot be nil")
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	addr, err := sdk.AccAddressFromBech32(msg.GetVerifier())
	if err != nil {
		return nil, fmt.Errorf("invalid verifier: %w", err)
	}
	amt, ok := sdkmath.NewIntFromString(msg.GetAmount())
	if !ok {
		return nil, fmt.Errorf("invalid amount")
	}
	verifier, err := m.keeper.BondCoins(ctx, addr, sdk.NewCoin(msg.GetDenom(), amt))
	if err != nil {
		return nil, err
	}
	ctx.EventManager().EmitEvent(sdk.NewEvent(
		EventTypeBonded,
		sdk.NewAttribute(AttributeKeyVerifier, verifier.Address),
		sdk.NewAttribute(AttributeKeyAmount, verifier.Bond.String()),
	))
	return &typespb.MsgBondResponse{Verifier: verifier.ToProto()}, nil
}

func (m msgServer) Unbond(goCtx context.Context, msg *typespb.MsgUnbond) (*typespb.MsgUnbondResponse, error) {
	if msg == nil {
		return nil, fmt.Errorf("message cannot be nil")
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	addr, err := sdk.AccAddressFromBech32(msg.GetVerifier())
	if err != nil {
		return nil, fmt.Errorf("invalid verifier: %w", err)
	}
	amt, ok := sdkmath.NewIntFromString(msg.GetAmount())
	if !ok {
		return nil, fmt.Errorf("invalid amount")
	}
	verifier, err := m.keeper.UnbondCoins(ctx, addr, sdk.NewCoin(msg.GetDenom(), amt))
	if err != nil {
		return nil, err
	}
	ctx.EventManager().EmitEvent(sdk.NewEvent(
		EventTypeUnbonded,
		sdk.NewAttribute(AttributeKeyVerifier, verifier.Address),
		sdk.NewAttribute(AttributeKeyAmount, verifier.Bond.String()),
	))
	return &typespb.MsgUnbondResponse{Verifier: verifier.ToProto()}, nil
}
