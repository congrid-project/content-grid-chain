package miners

import (
	"context"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"

	typespb "content-grid-chain/x/miners/typespb"
)

type queryServer struct {
	keeper Keeper
	typespb.UnimplementedQueryServer
}

// NewQueryServer builds a query server.
func NewQueryServer(keeper Keeper) typespb.QueryServer {
	return queryServer{keeper: keeper}
}

func (q queryServer) Miner(ctx context.Context, req *typespb.QueryMinerRequest) (*typespb.QueryMinerResponse, error) {
	if req == nil || req.GetOperator() == "" {
		return nil, errorsmod.Wrap(ErrInvalidMinerUpdate, "operator required")
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	miner, found := q.keeper.GetMiner(sdkCtx, req.GetOperator())
	if !found {
		return nil, ErrMinerNotFound
	}
	return &typespb.QueryMinerResponse{Miner: miner.ToProto()}, nil
}

func (q queryServer) Miners(ctx context.Context, req *typespb.QueryMinersRequest) (*typespb.QueryMinersResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	miners, pageRes, err := q.keeper.GetMinersPaginated(sdkCtx, req.GetPagination())
	if err != nil {
		return nil, err
	}
	out := make([]*typespb.Miner, 0, len(miners))
	for _, miner := range miners {
		minerCopy := miner
		out = append(out, minerCopy.ToProto())
	}
	return &typespb.QueryMinersResponse{
		Miners:     out,
		Pagination: pageRes,
	}, nil
}

func (q queryServer) Params(ctx context.Context, _ *typespb.QueryParamsRequest) (*typespb.QueryParamsResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	params := q.keeper.GetParams(sdkCtx)
	return &typespb.QueryParamsResponse{
		Params: &typespb.Params{
			StakeDenom:        params.StakeDenom,
			MinStake:          params.MinStake.String(),
			MaxMetadataLength: params.MaxMetadataLength,
		},
	}, nil
}
