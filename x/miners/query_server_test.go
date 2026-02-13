package miners

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	typespb "content-grid-chain/x/miners/typespb"
)

func TestQueryMiner(t *testing.T) {
	keeper, ctx := setupKeeper(t)
	params := keeper.GetParams(ctx)
	miner := Miner{
		Operator:    sdk.AccAddress([]byte("operator_query_test__")).String(),
		MetadataURI: "https://miner.example/meta",
		Services:    ServiceFetch,
		MinBid:      sdk.NewInt64Coin(params.StakeDenom, 2_000_000),
		Stake:       sdkmath.NewInt(5_000_000),
		Status:      StatusActive,
	}
	require.NoError(t, keeper.SetMiner(ctx, miner))

	server := NewQueryServer(keeper)
	resp, err := server.Miner(sdk.WrapSDKContext(ctx), &typespb.QueryMinerRequest{Operator: miner.Operator})
	require.NoError(t, err)
	require.Equal(t, miner.Operator, resp.GetMiner().GetOperator())
}
