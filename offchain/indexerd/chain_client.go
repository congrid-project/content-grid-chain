package main

import (
	"context"
	"sort"
	"strings"
	"time"

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

	nowUnix := time.Now().UTC().Unix()
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
			if !isActivePublisher(w, nowUnix) {
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

func (c *ChainClient) IsPublisherActive(ctx context.Context, domain string) (bool, error) {
	domain = strings.TrimSpace(strings.ToLower(domain))
	if domain == "" {
		return false, nil
	}
	resp, err := c.regqry.Publisher(ctx, &registrypb.QueryPublisherRequest{Domain: domain})
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return false, nil
		}
		return false, err
	}
	return isActivePublisher(resp.GetWebsite(), time.Now().UTC().Unix()), nil
}

func isActivePublisher(w *registrypb.Website, nowUnix int64) bool {
	if w == nil {
		return false
	}
	if w.GetStatus() != registrypb.WebsiteStatus_WEBSITE_STATUS_VERIFIED {
		return false
	}
	if w.GetCooldownUntilUnix() > nowUnix {
		return false
	}
	d := strings.TrimSpace(strings.ToLower(w.GetDomain()))
	return d != ""
}
