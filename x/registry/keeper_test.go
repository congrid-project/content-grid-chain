package registry

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
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"

	"content-grid-chain/x/verifiers"
)

type mockVerifierKeeper struct {
	verifiers []verifiers.Verifier
	params    verifiers.Params
}

func (m mockVerifierKeeper) ListVerifiers(ctx sdk.Context) []verifiers.Verifier {
	return m.verifiers
}

func (m mockVerifierKeeper) GetParams(ctx sdk.Context) verifiers.Params {
	if m.params.BondDenom == "" {
		return verifiers.DefaultParams()
	}
	return m.params
}

type mockTokenomicsKeeper struct{}

func (mockTokenomicsKeeper) MintAndSend(ctx sdk.Context, recipient sdk.AccAddress, coins sdk.Coins) error {
	return nil
}

func (mockTokenomicsKeeper) MintAndBurn(ctx sdk.Context, coins sdk.Coins) error {
	return nil
}

func (mockTokenomicsKeeper) EnsureEmissionPool(ctx sdk.Context, denom string, targetAmount sdkmath.Int) error {
	return nil
}

func (mockTokenomicsKeeper) SendFromPool(ctx sdk.Context, recipient sdk.AccAddress, coins sdk.Coins) error {
	return nil
}

func (mockTokenomicsKeeper) BurnFromPool(ctx sdk.Context, coins sdk.Coins) error {
	return nil
}

func setupKeeper(t *testing.T) (Keeper, sdk.Context) {
	t.Helper()
	storeKey := storetypes.NewMemoryStoreKey(StoreKey)
	db := dbm.NewMemDB()
	stateStore := store.NewCommitMultiStore(db, log.NewNopLogger(), metrics.NewNoOpMetrics())
	stateStore.MountStoreWithDB(storeKey, storetypes.StoreTypeMemory, nil)
	require.NoError(t, stateStore.LoadLatestVersion())

	interfaceRegistry := codectypes.NewInterfaceRegistry()
	cdc := codec.NewProtoCodec(interfaceRegistry)
	verifierKeeper := mockVerifierKeeper{}
	tokenomicsKeeper := mockTokenomicsKeeper{}
	var bankKeeper bankkeeper.Keeper
	keeper := NewKeeper(cdc, storeKey, verifierKeeper, tokenomicsKeeper, bankKeeper)
	ctx := sdk.NewContext(stateStore, tmproto.Header{ChainID: "registry-test", Height: 1}, false, log.NewNopLogger())
	require.NoError(t, keeper.SetParams(ctx, DefaultPublisherParams()))
	return keeper, ctx
}

func TestKeeperRegisterWebsite(t *testing.T) {
	keeper, ctx := setupKeeper(t)
	addr := sdk.AccAddress([]byte("addr________________"))

	website := Website{
		Domain: "example.com",
		Owner:  addr.String(),
		Status: StatusPending,
	}

	stored, err := keeper.RegisterWebsite(ctx, website)
	require.NoError(t, err)
	require.Equal(t, int64(1), stored.RegisteredAtHeight)

	// Test duplicate domain
	_, err = keeper.RegisterWebsite(ctx, website)
	require.ErrorIs(t, err, ErrWebsiteExists)

	// Test primary domain uniqueness (subdomain)
	sub := Website{
		Domain: "sub.example.com",
		Owner:  addr.String(),
		Status: StatusPending,
	}
	_, err = keeper.RegisterWebsite(ctx, sub)
	require.Error(t, err)
	require.Contains(t, err.Error(), "primary domain example.com already registered")

	// Test primary domain uniqueness (different port)
	port := Website{
		Domain: "example.com:8080",
		Owner:  addr.String(),
		Status: StatusPending,
	}
	_, err = keeper.RegisterWebsite(ctx, port)
	require.Error(t, err)
	require.Contains(t, err.Error(), "primary domain example.com already registered")

	// Test different primary domain
	other := Website{
		Domain: "other.com",
		Owner:  addr.String(),
		Status: StatusPending,
	}
	_, err = keeper.RegisterWebsite(ctx, other)
	require.NoError(t, err)
}
