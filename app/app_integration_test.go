package app_test

import (
	"testing"
	"time"

	dbm "github.com/cosmos/cosmos-db"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/cosmos-sdk/server"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	"github.com/cosmos/cosmos-sdk/testutil/network"

	pruningtypes "cosmossdk.io/store/pruning/types"

	"content-grid-chain/app"
)

func TestContentGridAppNetwork(t *testing.T) {
	cfg, err := network.DefaultConfigWithAppConfig(app.AppConfig)
	require.NoError(t, err)

	cfg.NumValidators = 1
	cfg.TimeoutCommit = time.Second
	cfg.MinGasPrices = app.MinGasPrice()
	cfg.PruningStrategy = pruningtypes.PruningOptionNothing

	cfg.AppConstructor = func(val network.ValidatorI) servertypes.Application {
		val.GetCtx().Viper.Set(server.FlagPruning, pruningtypes.PruningOptionNothing)
		baseAppOptions := server.DefaultBaseappOptions(val.GetCtx().Viper)
		return app.NewApp(
			val.GetCtx().Logger,
			dbm.NewMemDB(),
			nil,
			true,
			val.GetCtx().Viper,
			baseAppOptions...,
		)
	}

	net, err := network.New(network.NewCLILogger(&cobra.Command{Use: "grid-test"}), t.TempDir(), cfg)
	require.NoError(t, err)
	t.Cleanup(net.Cleanup)

	_, err = net.WaitForHeightWithTimeout(2, 10*time.Second)
	require.NoError(t, err)
}
