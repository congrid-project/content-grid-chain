package verifiers

import (
	"context"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	typespb "content-grid-chain/x/verifiers/typespb"
)

type queryServer struct {
	keeper Keeper
	typespb.UnimplementedQueryServer
}

func NewQueryServer(k Keeper) typespb.QueryServer {
	return queryServer{keeper: k}
}

func (q queryServer) Verifier(goCtx context.Context, req *typespb.QueryVerifierRequest) (*typespb.QueryVerifierResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request cannot be nil")
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	v, found := q.keeper.GetVerifier(ctx, req.GetAddress())
	if !found {
		return nil, fmt.Errorf("verifier not found")
	}
	return &typespb.QueryVerifierResponse{Verifier: v.ToProto()}, nil
}

func (q queryServer) Verifiers(goCtx context.Context, req *typespb.QueryVerifiersRequest) (*typespb.QueryVerifiersResponse, error) {
	_ = req
	ctx := sdk.UnwrapSDKContext(goCtx)
	vs := q.keeper.ListVerifiers(ctx)
	out := make([]*typespb.Verifier, 0, len(vs))
	for _, v := range vs {
		out = append(out, v.ToProto())
	}
	return &typespb.QueryVerifiersResponse{Verifiers: out}, nil
}
