package miners

import (
	"testing"

	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/stretchr/testify/require"

	"cosmossdk.io/log"
	sdkmath "cosmossdk.io/math"
	store "cosmossdk.io/store"
	"cosmossdk.io/store/metrics"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

func setupKeeper(t *testing.T) (Keeper, sdk.Context) {
	t.Helper()
	storeKey := storetypes.NewMemoryStoreKey(StoreKey)
	db := dbm.NewMemDB()
	stateStore := store.NewCommitMultiStore(db, log.NewNopLogger(), metrics.NewNoOpMetrics())
	stateStore.MountStoreWithDB(storeKey, storetypes.StoreTypeMemory, nil)
	require.NoError(t, stateStore.LoadLatestVersion())

	interfaceRegistry := codectypes.NewInterfaceRegistry()
	cdc := codec.NewProtoCodec(interfaceRegistry)
	keeper := NewKeeper(cdc, storeKey)

	ctx := sdk.NewContext(stateStore, tmproto.Header{ChainID: "miner-test", Height: 1}, false, log.NewNopLogger())
	require.NoError(t, keeper.SetParams(ctx, DefaultParams()))
	return keeper, ctx
}

func TestKeeperSetGetMiner(t *testing.T) {
	keeper, ctx := setupKeeper(t)
	params := keeper.GetParams(ctx)
	miner := Miner{
		Operator:    sdk.AccAddress([]byte("miner_operator______")).String(),
		MetadataURI: "https://miner.io/meta/1",
		Services:    ServiceFetch,
		MinBid:      sdk.NewInt64Coin(params.StakeDenom, 2_000_000),
		Stake:       sdkmath.NewInt(5_000_000),
		Status:      StatusActive,
	}
	require.NoError(t, keeper.SetMiner(ctx, miner))

	got, found := keeper.GetMiner(ctx, miner.Operator)
	require.True(t, found)
	require.Equal(t, miner.Operator, got.Operator)
}
