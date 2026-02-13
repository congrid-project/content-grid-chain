package registry

import (
	"context"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"

	typespb "content-grid-chain/x/registry/typespb"
)

func (q queryServer) PublisherSimilarStats(ctx context.Context, req *typespb.QueryPublisherSimilarStatsRequest) (*typespb.QueryPublisherSimilarStatsResponse, error) {
	if req == nil || req.GetDomain() == "" {
		return nil, errorsmod.Wrap(ErrInvalidPublisherRequest, "domain required")
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	stats, found := q.keeper.GetPublisherSimilarStats(sdkCtx, req.GetDomain())
	if !found {
		return nil, errorsmod.Wrapf(ErrPublisherNotFound, "similar stats for %s", req.GetDomain())
	}
	return &typespb.QueryPublisherSimilarStatsResponse{Stats: stats.ToProto()}, nil
}
