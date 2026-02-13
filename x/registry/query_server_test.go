package registry

import (
	"testing"

	"github.com/stretchr/testify/require"

	sdk "github.com/cosmos/cosmos-sdk/types"

	typespb "content-grid-chain/x/registry/typespb"
)

func TestQueryPublisher(t *testing.T) {
	keeper, ctx := setupKeeper(t)
	addr := sdk.AccAddress([]byte("addr________________"))
	_, err := keeper.RegisterWebsite(ctx, Website{Domain: "example.com", Owner: addr.String(), Status: StatusPending})
	require.NoError(t, err)

	server := NewQueryServer(keeper)
	resp, err := server.Publisher(sdk.WrapSDKContext(ctx), &typespb.QueryPublisherRequest{Domain: "example.com"})
	require.NoError(t, err)
	require.Equal(t, "example.com", resp.GetWebsite().GetDomain())
}
