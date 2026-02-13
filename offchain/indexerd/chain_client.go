package main

import (
	"context"
	"sort"
	"strings"

	registrypb "content-grid-chain/x/registry/typespb"

	"github.com/cosmos/cosmos-sdk/types/query"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type ChainClient struct {
	conn   *grpc.ClientConn
	regqry registrypb.QueryClient
}

func NewChainClient(addr string) (*ChainClient, error) {
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &ChainClient{
		conn:   conn,
		regqry: registrypb.NewQueryClient(conn),
	}, nil
}

func (c *ChainClient) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *ChainClient) ListPublishers(ctx context.Context, pageLimit uint64) ([]string, error) {
	if pageLimit == 0 {
		pageLimit = 200
	}

	seen := make(map[string]struct{})
	out := make([]string, 0)
	var nextKey []byte
	for {
		resp, err := c.regqry.Publishers(ctx, &registrypb.QueryPublishersRequest{
			Pagination: &query.PageRequest{Key: nextKey, Limit: pageLimit},
		})
		if err != nil {
			return nil, err
		}
		for _, w := range resp.GetWebsites() {
			if w == nil {
				continue
			}
			d := strings.TrimSpace(strings.ToLower(w.GetDomain()))
			if d == "" {
				continue
			}
			if _, ok := seen[d]; ok {
				continue
			}
			seen[d] = struct{}{}
			out = append(out, d)
		}
		pk := resp.GetPagination().GetNextKey()
		if len(pk) == 0 {
			break
		}
		nextKey = pk
	}
	sort.Strings(out)
	return out, nil
}
