package main

import (
	"context"
	"strings"

	registrypb "content-grid-chain/x/registry/typespb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type ChainClient struct {
	conn   *grpc.ClientConn
	regqry registrypb.QueryClient
}

type DrandBeaconState struct {
	Round           uint64
	SubmittedAtUnix int64
}

func NewChainClient(addr string) (*ChainClient, error) {
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &ChainClient{conn: conn, regqry: registrypb.NewQueryClient(conn)}, nil
}

func (c *ChainClient) Close() error {
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *ChainClient) LatestDrandBeacon(ctx context.Context) (DrandBeaconState, error) {
	resp, err := c.regqry.LatestDrandBeacon(ctx, &registrypb.QueryLatestDrandBeaconRequest{})
	if err != nil {
		// no beacon yet on-chain
		if strings.Contains(strings.ToLower(err.Error()), "drand beacon") && strings.Contains(strings.ToLower(err.Error()), "not found") {
			return DrandBeaconState{}, nil
		}
		return DrandBeaconState{}, err
	}
	beacon := resp.GetBeacon()
	if beacon == nil {
		return DrandBeaconState{}, nil
	}
	return DrandBeaconState{
		Round:           beacon.GetRound(),
		SubmittedAtUnix: beacon.GetSubmittedAtUnix(),
	}, nil
}
