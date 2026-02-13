package registry

import (
	"context"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"

	typespb "content-grid-chain/x/registry/typespb"
)

func (q queryServer) DrandBeacon(ctx context.Context, req *typespb.QueryDrandBeaconRequest) (*typespb.QueryDrandBeaconResponse, error) {
	if req == nil || req.GetRound() == 0 {
		return nil, errorsmod.Wrap(ErrInvalidPublisherRequest, "round required")
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	beacon, found := q.keeper.GetDrandBeacon(sdkCtx, req.GetRound())
	if !found {
		return nil, errorsmod.Wrapf(ErrDrandBeaconNotFound, "drand beacon round %d", req.GetRound())
	}
	return &typespb.QueryDrandBeaconResponse{Beacon: beacon.ToProto()}, nil
}

func (q queryServer) LatestDrandBeacon(ctx context.Context, req *typespb.QueryLatestDrandBeaconRequest) (*typespb.QueryLatestDrandBeaconResponse, error) {
	_ = req
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	beacon, found := q.keeper.GetLatestDrandBeacon(sdkCtx)
	if !found {
		return nil, errorsmod.Wrap(ErrDrandBeaconNotFound, "latest drand beacon")
	}
	return &typespb.QueryLatestDrandBeaconResponse{Beacon: beacon.ToProto()}, nil
}
