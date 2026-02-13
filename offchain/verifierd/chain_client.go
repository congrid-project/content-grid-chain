package main

import (
	"context"
	"sort"
	"strings"
	"time"

	registryoffchain "content-grid-chain/offchain/registry"
	registrypb "content-grid-chain/x/registry/typespb"
	verifierspb "content-grid-chain/x/verifiers/typespb"

	query "github.com/cosmos/cosmos-sdk/types/query"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type ChainClient struct {
	conn   *grpc.ClientConn
	regqry registrypb.QueryClient
	verqry verifierspb.QueryClient
}

func NewChainClient(addr string) (*ChainClient, error) {
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &ChainClient{
		conn:   conn,
		regqry: registrypb.NewQueryClient(conn),
		verqry: verifierspb.NewQueryClient(conn),
	}, nil
}

func (c *ChainClient) Close() error {
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *ChainClient) VerifierAssignments(ctx context.Context, verifier string) ([]*registrypb.VerifierAssignment, error) {
	resp, err := c.regqry.VerifierAssignments(ctx, &registrypb.QueryVerifierAssignmentsRequest{
		Verifier:         verifier,
		IncludeFinalized: false,
	})
	if err != nil {
		return nil, err
	}
	return resp.GetAssignments(), nil
}

func (c *ChainClient) RoundMeta(ctx context.Context, roundStartUnix int64) (*registrypb.VerificationRoundMeta, error) {
	if roundStartUnix <= 0 {
		return nil, nil
	}
	resp, err := c.regqry.RoundMeta(ctx, &registrypb.QueryRoundMetaRequest{RoundStartUnix: roundStartUnix})
	if err != nil {
		return nil, err
	}
	return resp.GetMeta(), nil
}

func (c *ChainClient) PublisherOwner(ctx context.Context, domain string) (string, error) {
	resp, err := c.regqry.Publisher(ctx, &registrypb.QueryPublisherRequest{Domain: domain})
	if err != nil {
		return "", err
	}
	if resp.GetWebsite() == nil {
		return "", nil
	}
	return resp.GetWebsite().GetOwner(), nil
}

func (c *ChainClient) ActiveLeaseExpectationsForDomain(ctx context.Context, domain string) ([]registryoffchain.LeaseExpectation, error) {
	owner, err := c.PublisherOwner(ctx, domain)
	if err != nil {
		return nil, err
	}
	if owner == "" {
		return nil, nil
	}

	slots, err := c.listSlots(ctx, owner, registrypb.SlotStatus_SLOT_STATUS_UNSPECIFIED)
	if err != nil {
		return nil, err
	}
	normDomain := normalizeDomain(domain)
	var out []registryoffchain.LeaseExpectation
	for _, slot := range slots {
		if normalizeDomain(slot.GetDomain()) != normDomain {
			continue
		}
		leases, err := c.listLeases(ctx, owner, slot.GetId(), true, time.Now().UTC().Unix())
		if err != nil {
			return nil, err
		}
		for _, lease := range leases {
			out = append(out, registryoffchain.LeaseExpectation{
				SlotID:    lease.GetSlotId(),
				LeaseID:   lease.GetId(),
				TargetURL: lease.GetTargetUrl(),
			})
		}
	}
	return out, nil
}

func (c *ChainClient) listSlots(ctx context.Context, publisher string, status registrypb.SlotStatus) ([]*registrypb.Slot, error) {
	var out []*registrypb.Slot
	var pageKey []byte
	for {
		resp, err := c.regqry.Slots(ctx, &registrypb.QuerySlotsRequest{
			Publisher: publisher,
			Status:    status,
			Pagination: &query.PageRequest{
				Key:   pageKey,
				Limit: 200,
			},
		})
		if err != nil {
			return nil, err
		}
		out = append(out, resp.GetSlots()...)
		if resp.GetPagination() == nil || len(resp.GetPagination().GetNextKey()) == 0 {
			break
		}
		pageKey = resp.GetPagination().GetNextKey()
	}
	return out, nil
}

func (c *ChainClient) listLeases(ctx context.Context, publisher, slotID string, activeOnly bool, atUnix int64) ([]*registrypb.SlotLease, error) {
	var out []*registrypb.SlotLease
	var pageKey []byte
	for {
		resp, err := c.regqry.Leases(ctx, &registrypb.QueryLeasesRequest{
			Publisher:  publisher,
			SlotId:     slotID,
			ActiveOnly: activeOnly,
			AtUnix:     atUnix,
			Pagination: &query.PageRequest{
				Key:   pageKey,
				Limit: 200,
			},
		})
		if err != nil {
			return nil, err
		}
		out = append(out, resp.GetLeases()...)
		if resp.GetPagination() == nil || len(resp.GetPagination().GetNextKey()) == 0 {
			break
		}
		pageKey = resp.GetPagination().GetNextKey()
	}
	return out, nil
}

func (c *ChainClient) ActiveVerifierAddresses(ctx context.Context) ([]string, error) {
	resp, err := c.verqry.Verifiers(ctx, &verifierspb.QueryVerifiersRequest{})
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(resp.GetVerifiers()))
	for _, v := range resp.GetVerifiers() {
		if v.GetStatus() != verifierspb.VerifierStatus_VERIFIER_STATUS_ACTIVE {
			continue
		}
		addr := strings.TrimSpace(v.GetAddress())
		if addr == "" {
			continue
		}
		out = append(out, addr)
	}
	sort.Strings(out)
	return out, nil
}

func normalizeDomain(domain string) string {
	return strings.TrimSpace(strings.ToLower(domain))
}
