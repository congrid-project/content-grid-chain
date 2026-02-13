package registry

import (
	"context"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"

	typespb "content-grid-chain/x/registry/typespb"
)

func (q queryServer) RoundMeta(ctx context.Context, req *typespb.QueryRoundMetaRequest) (*typespb.QueryRoundMetaResponse, error) {
	if req == nil || req.GetRoundStartUnix() <= 0 {
		return nil, errorsmod.Wrap(ErrInvalidPublisherRequest, "round_start_unix required")
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	meta, found := q.keeper.GetRoundMeta(sdkCtx, req.GetRoundStartUnix())
	if !found {
		return nil, errorsmod.Wrapf(ErrPublisherNotFound, "round meta for %d", req.GetRoundStartUnix())
	}
	return &typespb.QueryRoundMetaResponse{Meta: meta.ToProto()}, nil
}
