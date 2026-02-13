package miners

import (
	"context"
	"fmt"
	"strings"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	typespb "content-grid-chain/x/miners/typespb"
)

type msgServer struct {
	keeper Keeper
	typespb.UnimplementedMsgServer
}

// NewMsgServerImpl creates a msg server.
func NewMsgServerImpl(keeper Keeper) typespb.MsgServer {
	return msgServer{keeper: keeper}
}

func (m msgServer) RegisterMiner(goCtx context.Context, msg *typespb.MsgRegisterMiner) (*typespb.MsgRegisterMinerResponse, error) {
	if msg == nil {
		return nil, fmt.Errorf("message cannot be nil")
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	_, exists := m.keeper.GetMiner(ctx, msg.GetOperator())
	if exists {
		return nil, ErrMinerExists
	}

	miner, err := m.buildMinerFromMsg(ctx, msg.GetOperator(), msg.GetMetadataUri(), msg.GetServices(), msg.GetMinBidDenom(), msg.GetMinBidAmount(), msg.GetStake())
	if err != nil {
		return nil, err
	}
	miner.Status = StatusActive
	miner.RegisteredAtHeight = ctx.BlockHeight()
	miner.LastUpdateHeight = ctx.BlockHeight()

	if err := m.keeper.SetMiner(ctx, miner); err != nil {
		return nil, err
	}

	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			EventTypeMinerRegistered,
			sdk.NewAttribute(AttributeKeyOperator, miner.Operator),
			sdk.NewAttribute(AttributeKeyStake, miner.Stake.String()),
		),
	)

	return &typespb.MsgRegisterMinerResponse{Miner: miner.ToProto()}, nil
}

func (m msgServer) UpdateMiner(goCtx context.Context, msg *typespb.MsgUpdateMiner) (*typespb.MsgUpdateMinerResponse, error) {
	if msg == nil {
		return nil, fmt.Errorf("message cannot be nil")
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	miner, found := m.keeper.GetMiner(ctx, msg.GetOperator())
	if !found {
		return nil, ErrMinerNotFound
	}

	params := m.keeper.GetParams(ctx)
	if strings.TrimSpace(msg.GetMetadataUri()) != "" {
		miner.MetadataURI = msg.GetMetadataUri()
	}
	if msg.GetServices() != 0 {
		miner.Services = msg.GetServices()
	}
	if msg.GetMinBidDenom() != "" || msg.GetMinBidAmount() != "" {
		amount, ok := sdkmath.NewIntFromString(msg.GetMinBidAmount())
		if !ok {
			return nil, fmt.Errorf("invalid min bid amount")
		}
		miner.MinBid = sdk.NewCoin(msg.GetMinBidDenom(), amount)
		if miner.MinBid.Denom == "" {
			miner.MinBid = sdk.NewCoin(params.StakeDenom, amount)
		}
	}
	miner.LastUpdateHeight = ctx.BlockHeight()

	if err := m.keeper.SetMiner(ctx, miner); err != nil {
		return nil, err
	}

	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			EventTypeMinerUpdated,
			sdk.NewAttribute(AttributeKeyOperator, miner.Operator),
		),
	)

	return &typespb.MsgUpdateMinerResponse{Miner: miner.ToProto()}, nil
}

func (m msgServer) UpdateMinerStake(goCtx context.Context, msg *typespb.MsgUpdateMinerStake) (*typespb.MsgUpdateMinerStakeResponse, error) {
	if msg == nil {
		return nil, fmt.Errorf("message cannot be nil")
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	miner, found := m.keeper.GetMiner(ctx, msg.GetOperator())
	if !found {
		return nil, ErrMinerNotFound
	}

	delta, ok := sdkmath.NewIntFromString(msg.GetStakeDelta())
	if !ok {
		return nil, fmt.Errorf("invalid stake delta")
	}
	if !delta.IsPositive() {
		return nil, fmt.Errorf("stake delta must be positive")
	}
	if msg.GetIncrease() {
		miner.Stake = miner.Stake.Add(delta)
	} else {
		if miner.Stake.LTE(delta) {
			return nil, fmt.Errorf("cannot reduce stake below zero")
		}
		miner.Stake = miner.Stake.Sub(delta)
	}
	miner.LastUpdateHeight = ctx.BlockHeight()

	if err := m.keeper.SetMiner(ctx, miner); err != nil {
		return nil, err
	}

	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			EventTypeStakeUpdated,
			sdk.NewAttribute(AttributeKeyOperator, miner.Operator),
			sdk.NewAttribute(AttributeKeyStake, miner.Stake.String()),
		),
	)

	return &typespb.MsgUpdateMinerStakeResponse{Miner: miner.ToProto()}, nil
}

func (m msgServer) buildMinerFromMsg(ctx sdk.Context, operator, metadataURI string, services uint32, minBidDenom, minBidAmount, stakeAmount string) (Miner, error) {
	params := m.keeper.GetParams(ctx)
	minBidInt, ok := sdkmath.NewIntFromString(minBidAmount)
	if !ok {
		return Miner{}, fmt.Errorf("invalid min bid amount")
	}
	if minBidDenom == "" {
		minBidDenom = params.StakeDenom
	}
	stakeInt, ok := sdkmath.NewIntFromString(stakeAmount)
	if !ok {
		return Miner{}, fmt.Errorf("invalid stake amount")
	}

	return Miner{
		Operator:           operator,
		MetadataURI:        metadataURI,
		Services:           services,
		MinBid:             sdk.NewCoin(minBidDenom, minBidInt),
		Stake:              stakeInt,
		Status:             StatusPending,
		RegisteredAtHeight: ctx.BlockHeight(),
		LastUpdateHeight:   ctx.BlockHeight(),
	}, nil
}
