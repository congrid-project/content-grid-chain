package registry

import (
	"context"
	"fmt"
	"sort"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"

	typespb "content-grid-chain/x/registry/typespb"
)

// queryServer implements the Query service.
type queryServer struct {
	keeper Keeper
	typespb.UnimplementedQueryServer
}

// NewQueryServer creates a new query server instance.
func NewQueryServer(keeper Keeper) typespb.QueryServer {
	return queryServer{keeper: keeper}
}

func (q queryServer) Publisher(ctx context.Context, req *typespb.QueryPublisherRequest) (*typespb.QueryPublisherResponse, error) {
	if req == nil || req.GetDomain() == "" {
		return nil, errorsmod.Wrap(ErrInvalidPublisherRequest, "domain required")
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	website, found := q.keeper.GetWebsite(sdkCtx, req.GetDomain())
	if !found {
		return nil, errorsmod.Wrapf(ErrPublisherNotFound, "publisher %s", req.GetDomain())
	}
	return &typespb.QueryPublisherResponse{Website: website.ToProto()}, nil
}

func (q queryServer) Publishers(ctx context.Context, req *typespb.QueryPublishersRequest) (*typespb.QueryPublishersResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	websites, pagination, err := q.keeper.GetWebsitesPaginated(sdkCtx, req.GetPagination())
	if err != nil {
		return nil, err
	}
	out := make([]*typespb.Website, 0, len(websites))
	for _, w := range websites {
		website := w // copy to avoid pointer aliasing
		out = append(out, website.ToProto())
	}
	return &typespb.QueryPublishersResponse{
		Websites:   out,
		Pagination: pagination,
	}, nil
}

func (q queryServer) VerifierAssignments(ctx context.Context, req *typespb.QueryVerifierAssignmentsRequest) (*typespb.QueryVerifierAssignmentsResponse, error) {
	if req == nil || req.GetVerifier() == "" {
		return nil, errorsmod.Wrap(ErrInvalidPublisherRequest, "verifier required")
	}
	if _, err := sdk.AccAddressFromBech32(req.GetVerifier()); err != nil {
		return nil, fmt.Errorf("invalid verifier address: %w", err)
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	assignments := q.keeper.ListAssignmentsForVerifier(sdkCtx, req.GetVerifier(), req.GetIncludeFinalized())
	sort.Slice(assignments, func(i, j int) bool {
		if assignments[i].StartAtUnix == assignments[j].StartAtUnix {
			return assignments[i].Domain < assignments[j].Domain
		}
		return assignments[i].StartAtUnix < assignments[j].StartAtUnix
	})

	out := make([]*typespb.VerifierAssignment, 0, len(assignments))
	for _, a := range assignments {
		assignment := a
		entry := &typespb.VerifierAssignment{Assignment: assignment.ToProto()}
		if sub, found := q.keeper.GetSubmission(sdkCtx, assignment.RoundStartUnix, assignment.Domain, req.GetVerifier()); found {
			submission := sub
			entry.Submission = submission.ToProto()
		}
		out = append(out, entry)
	}
	return &typespb.QueryVerifierAssignmentsResponse{Assignments: out}, nil
}


func (q queryServer) Slots(ctx context.Context, req *typespb.QuerySlotsRequest) (*typespb.QuerySlotsResponse, error) {
	if req == nil {
		return nil, errorsmod.Wrap(ErrInvalidPublisherRequest, "request required")
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	status := SlotStatus(req.GetStatus())
	slots, pagination, err := q.keeper.ListSlotsPaginated(sdkCtx, req.GetPublisher(), status, req.GetPagination())
	if err != nil {
		return nil, err
	}

	out := make([]*typespb.Slot, 0, len(slots))
	for _, s := range slots {
		slot := s
		out = append(out, slot.ToProto())
	}
	return &typespb.QuerySlotsResponse{Slots: out, Pagination: pagination}, nil
}

func (q queryServer) Leases(ctx context.Context, req *typespb.QueryLeasesRequest) (*typespb.QueryLeasesResponse, error) {
	if req == nil {
		return nil, errorsmod.Wrap(ErrInvalidPublisherRequest, "request required")
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	leases, pagination, err := q.keeper.ListLeasesPaginated(sdkCtx, req.GetPublisher(), req.GetSlotId(), req.GetActiveOnly(), req.GetAtUnix(), req.GetPagination())
	if err != nil {
		return nil, err
	}

	out := make([]*typespb.SlotLease, 0, len(leases))
	for _, l := range leases {
		lease := l
		out = append(out, lease.ToProto())
	}
	return &typespb.QueryLeasesResponse{Leases: out, Pagination: pagination}, nil
}
