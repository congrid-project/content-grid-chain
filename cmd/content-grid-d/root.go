package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"

	cmtcfg "github.com/cometbft/cometbft/config"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"cosmossdk.io/depinject"
	"cosmossdk.io/log"

	"cosmossdk.io/client/v2/autocli"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/config"
	"github.com/cosmos/cosmos-sdk/client/debug"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/keys"
	"github.com/cosmos/cosmos-sdk/client/pruning"
	"github.com/cosmos/cosmos-sdk/client/rpc"
	"github.com/cosmos/cosmos-sdk/client/snapshot"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/server"
	serverconfig "github.com/cosmos/cosmos-sdk/server/config"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	authcmd "github.com/cosmos/cosmos-sdk/x/auth/client/cli"
	authtx "github.com/cosmos/cosmos-sdk/x/auth/tx"
	authtxconfig "github.com/cosmos/cosmos-sdk/x/auth/tx/config"
	"github.com/cosmos/cosmos-sdk/x/auth/types"
	genutilcli "github.com/cosmos/cosmos-sdk/x/genutil/client/cli"

	"github.com/spf13/cast"

	"content-grid-chain/app"
	"content-grid-chain/x/nodes"
	"content-grid-chain/x/registry"
	"content-grid-chain/x/tokenomics"
	"content-grid-chain/x/verifiers"
)

// NewRootCmd returns the root command for the content-grid-d binary.
func NewRootCmd() *cobra.Command {
	var (
		moduleBasicManager module.BasicManager
		clientCtx          client.Context
		autoCliOpts        autocli.AppOptions
	)

	if err := depinject.Inject(
		depinject.Configs(
			app.AppConfig,
			depinject.Supply(log.NewNopLogger()),
			depinject.Provide(ProvideClientContext),
		),
		&moduleBasicManager,
		&clientCtx,
		&autoCliOpts,
	); err != nil {
		panic(err)
	}

	// The depinject-provided basic manager only includes modules declared in AppConfig.
	// Our chain wires several custom modules manually; they still need to be present here
	// so `init`/`devnet` genesis includes their state (params, etc.).
	customBasics := []module.AppModuleBasic{
		nodes.AppModuleBasic{},
		registry.AppModuleBasic{},
		verifiers.AppModuleBasic{},
		tokenomics.AppModuleBasic{},
	}
	for _, b := range customBasics {
		moduleBasicManager[b.Name()] = b
		b.RegisterInterfaces(clientCtx.InterfaceRegistry)
		b.RegisterLegacyAminoCodec(clientCtx.LegacyAmino)
	}

	autoCliOpts.ClientCtx = clientCtx
	rootCmd := &cobra.Command{
		Use:           "content-grid-d",
		Short:         "Content Grid Chain Daemon",
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SetOut(cmd.OutOrStdout())
			cmd.SetErr(cmd.ErrOrStderr())

			clientCtx = clientCtx.WithCmdContext(cmd.Context()).WithViper("")
			clientCtx, err := client.ReadPersistentCommandFlags(clientCtx, cmd.Flags())
			if err != nil {
				return err
			}

			clientCtx, err = config.ReadFromClientConfig(clientCtx)
			if err != nil {
				return err
			}

			if err := client.SetCmdClientContextHandler(clientCtx, cmd); err != nil {
				return err
			}

			customAppTemplate, customAppConfig := initAppConfig()
			customCMTConfig := initCometBFTConfig()

			return server.InterceptConfigsPreRunHandler(cmd, customAppTemplate, customAppConfig, customCMTConfig)
		},
	}

	initRootCmd(rootCmd, clientCtx.TxConfig, moduleBasicManager)
	if err := autoCliOpts.EnhanceRootCommand(rootCmd); err != nil {
		panic(err)
	}

	return rootCmd
}

func ProvideClientContext(
	appCodec codec.Codec,
	interfaceRegistry codectypes.InterfaceRegistry,
	txConfigOpts authtx.ConfigOptions,
	legacyAmino *codec.LegacyAmino,
) client.Context {
	clientCtx := client.Context{}.
		WithCodec(appCodec).
		WithInterfaceRegistry(interfaceRegistry).
		WithLegacyAmino(legacyAmino).
		WithInput(os.Stdin).
		WithAccountRetriever(types.AccountRetriever{}).
		WithHomeDir(app.DefaultNodeHome).
		WithViper("")

	clientCtx, _ = config.ReadFromClientConfig(clientCtx)

	txConfigOpts.TextualCoinMetadataQueryFn = authtxconfig.NewGRPCCoinMetadataQueryFn(clientCtx)
	txCfg, err := authtx.NewTxConfigWithOptions(clientCtx.Codec, txConfigOpts)
	if err != nil {
		panic(err)
	}

	return clientCtx.WithTxConfig(txCfg)
}

func initRootCmd(rootCmd *cobra.Command, txConfig client.TxConfig, basicManager module.BasicManager) {
	initCmd := genutilcli.InitCmd(basicManager, app.DefaultNodeHome)
	wrapInitCommandWithDenomPatch(initCmd)

	rootCmd.AddCommand(
		initCmd,
		debug.Cmd(),
		pruning.Cmd(newApp, app.DefaultNodeHome),
		snapshot.Cmd(newApp),
	)

	server.AddCommandsWithStartCmdOptions(rootCmd, app.DefaultNodeHome, newApp, appExport, server.StartCmdOptions{
		DBOpener: func(rootDir string, backendType dbm.BackendType) (dbm.DB, error) {
			absRoot, err := filepath.Abs(rootDir)
			if err != nil {
				return nil, err
			}
			dataDir := filepath.Join(absRoot, "data")
			base, err := dbm.NewDB("application", backendType, dataDir)
			if err != nil {
				return nil, err
			}
			// Wrap DB so IAVL empty roots (stored as empty values) are treated as present.
			return presenceDB{DB: base}, nil
		},
	})

	rootCmd.AddCommand(
		server.StatusCommand(),
		devnetCommand(),
		publisherCommand(),
		verifierCommand(),
		genesisCommand(txConfig, basicManager),
		queryCommand(),
		txCommand(),
		keys.Commands(),
	)
}

func wrapInitCommandWithDenomPatch(initCmd *cobra.Command) {
	if initCmd == nil {
		return
	}
	origRunE := initCmd.RunE
	origRun := initCmd.Run

	initCmd.Run = nil
	initCmd.RunE = func(cmd *cobra.Command, args []string) error {
		if origRunE != nil {
			if err := origRunE(cmd, args); err != nil {
				return err
			}
		} else if origRun != nil {
			origRun(cmd, args)
		}

		home, err := cmd.Flags().GetString(flags.FlagHome)
		if err != nil {
			return err
		}
		if home == "" {
			home = app.DefaultNodeHome
		}
		genesisPath := filepath.Join(home, "config", "genesis.json")
		return patchGenesisDenoms(genesisPath, tokenomics.DefaultDenom)
	}
}

func genesisCommand(txConfig client.TxConfig, basicManager module.BasicManager, cmds ...*cobra.Command) *cobra.Command {
	cmd := genutilcli.Commands(txConfig, basicManager, app.DefaultNodeHome)
	for _, subCmd := range cmds {
		cmd.AddCommand(subCmd)
	}
	return cmd
}

func queryCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "query",
		Aliases:                    []string{"q"},
		Short:                      "Querying subcommands",
		DisableFlagParsing:         false,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(
		rpc.WaitTxCmd(),
		server.QueryBlockCmd(),
		authcmd.QueryTxsByEventsCmd(),
		server.QueryBlocksCmd(),
		authcmd.QueryTxCmd(),
		server.QueryBlockResultsCmd(),
		registryQueryCmd(),
	)

	return cmd
}

func txCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "tx",
		Short:                      "Transactions subcommands",
		DisableFlagParsing:         false,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(
		authcmd.GetSignCommand(),
		authcmd.GetSignBatchCommand(),
		authcmd.GetMultiSignCommand(),
		authcmd.GetMultiSignBatchCmd(),
		authcmd.GetValidateSignaturesCommand(),
		authcmd.GetBroadcastCommand(),
		authcmd.GetEncodeCommand(),
		authcmd.GetDecodeCommand(),
		authcmd.GetSimulateCmd(),
		registryTxCmd(),
	)

	return cmd
}

func initCometBFTConfig() *cmtcfg.Config {
	cfg := cmtcfg.DefaultConfig()
	return cfg
}

func initAppConfig() (string, interface{}) {
	srvCfg := serverconfig.DefaultConfig()
	srvCfg.MinGasPrices = app.MinGasPrice()
	srvCfg.API.Enable = true
	return serverconfig.DefaultConfigTemplate, srvCfg
}

func newApp(logger log.Logger, db dbm.DB, traceStore io.Writer, appOpts servertypes.AppOptions) servertypes.Application {
	// Defensive check: if the app DB ends up under a different directory than --home,
	// CometBFT height and app store versions will diverge, and queries will fail with
	// "version does not exist".
	// A common culprit is an accidental relative data dir (e.g. ./data).
	if _, err := os.Stat(filepath.Join("data", "application.db")); err == nil {
		logger.Error("unexpected ./data/application.db detected; this often causes Comet/app DB mismatch; remove ./data and restart with a clean --home")
		panic("unexpected ./data/application.db detected")
	}

	home := cast.ToString(appOpts.Get(flags.FlagHome))
	if home != "" {
		logger.Info("using --home", "home", home, "expected_app_data_dir", filepath.Join(home, "data"))
	}

	baseAppOptions := server.DefaultBaseappOptions(appOpts)
	return app.NewApp(logger, db, traceStore, true, appOpts, baseAppOptions...)
}

func appExport(
	logger log.Logger,
	db dbm.DB,
	traceStore io.Writer,
	height int64,
	forZeroHeight bool,
	jailAllowedAddrs []string,
	appOpts servertypes.AppOptions,
	modulesToExport []string,
) (servertypes.ExportedApp, error) {
	viperOpts, ok := appOpts.(*viper.Viper)
	if !ok {
		return servertypes.ExportedApp{}, errors.New("app options must be viper")
	}

	// ensure invariants run during export
	viperOpts.Set(server.FlagInvCheckPeriod, 1)

	baseAppOptions := server.DefaultBaseappOptions(viperOpts)
	gridApp := app.NewApp(logger, db, traceStore, height == -1, viperOpts, baseAppOptions...)

	if height != -1 {
		if err := gridApp.LoadHeight(height); err != nil {
			return servertypes.ExportedApp{}, err
		}
	}

	return gridApp.ExportAppStateAndValidators(forZeroHeight, jailAllowedAddrs, modulesToExport)
}
