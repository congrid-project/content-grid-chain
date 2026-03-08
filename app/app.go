package app

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"cosmossdk.io/depinject"
	"cosmossdk.io/log"
	storetypes "cosmossdk.io/store/types"

	tmtypes "github.com/cometbft/cometbft/types"
	dbm "github.com/cosmos/cosmos-db"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	"github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/cosmos-sdk/server/api"
	serverconfig "github.com/cosmos/cosmos-sdk/server/config"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	authante "github.com/cosmos/cosmos-sdk/x/auth/ante"

	"content-grid-chain/x/nodes"
	"content-grid-chain/x/registry"
	"content-grid-chain/x/tokenomics"
	"content-grid-chain/x/verifiers"

	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
)

const (
	// AppName is the official name of the binary and running application.
	AppName = "content-grid-chain"
)

// DefaultNodeHome defines the default home directory for the daemon.
var DefaultNodeHome string

func init() {
	home, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	DefaultNodeHome = filepath.Join(home, ".content-grid-d")

	cfg := sdk.GetConfig()
	cfg.SetBech32PrefixForAccount(DefaultChainIDPrefix, DefaultChainIDPrefix+"pub")
	cfg.SetBech32PrefixForValidator(DefaultChainIDPrefix+"valoper", DefaultChainIDPrefix+"valoperpub")
	cfg.SetBech32PrefixForConsensusNode(DefaultChainIDPrefix+"valcons", DefaultChainIDPrefix+"valconspub")
	cfg.Seal()
}

// App implements a Cosmos SDK application using the runtime wiring introduced in v0.53.
type App struct {
	*runtime.App

	appCodec          codec.Codec
	legacyAmino       *codec.LegacyAmino
	txConfig          client.TxConfig
	interfaceRegistry codectypes.InterfaceRegistry
	basicManager      module.BasicManager
	BankKeeper        bankkeeper.Keeper
	AccountKeeper     authante.AccountKeeper
	FeeGrantKeeper    authante.FeegrantKeeper

	RegistryKeeper   registry.Keeper
	TokenomicsKeeper tokenomics.Keeper
	VerifiersKeeper  verifiers.Keeper
}

// ModuleBasics contains the Content Grid-specific module basics used for genesis helpers.
var ModuleBasics = module.NewBasicManager(
	nodes.AppModuleBasic{},
	registry.AppModuleBasic{},
	verifiers.AppModuleBasic{},
	tokenomics.AppModuleBasic{},
)

// NewApp creates a fully configured application using the depinject wiring defined in AppConfig.
func NewApp(
	logger log.Logger,
	db dbm.DB,
	traceStore io.Writer,
	loadLatest bool,
	appOpts servertypes.AppOptions,
	baseAppOptions ...func(*baseapp.BaseApp),
) *App {
	var (
		application = &App{}
		appBuilder  *runtime.AppBuilder
		basicMgr    module.BasicManager
	)

	if err := depinject.Inject(
		depinject.Configs(
			AppConfig,
			depinject.Supply(appOpts, logger),
		),
		&appBuilder,
		&application.appCodec,
		&application.legacyAmino,
		&application.txConfig,
		&application.interfaceRegistry,
		&basicMgr,
		&application.BankKeeper,
		&application.AccountKeeper,
		&application.FeeGrantKeeper,
	); err != nil {
		panic(err)
	}
	application.basicManager = basicMgr

	application.App = appBuilder.Build(db, traceStore, baseAppOptions...)

	if err := setCustomAnteHandler(application); err != nil {
		panic(err)
	}

	registerCustomModules(application)
	registerCustomModuleOrders(application)

	if err := application.Load(loadLatest); err != nil {
		panic(err)
	}

	return application
}

// registerCustomModules wires chain-specific modules that are not part of the SDK default wiring.
func registerCustomModules(app *App) {
	nodesModule := nodes.NewAppModule()

	verifiersStoreKey := storetypes.NewKVStoreKey(verifiers.StoreKey)
	if err := app.RegisterStores(verifiersStoreKey); err != nil {
		panic(err)
	}
	verifiersKeeper := verifiers.NewKeeper(app.appCodec, verifiersStoreKey, app.BankKeeper)
	verifiersModule := verifiers.NewAppModule(verifiersKeeper)

	tokenomicsStoreKey := storetypes.NewKVStoreKey(tokenomics.StoreKey)
	if err := app.RegisterStores(tokenomicsStoreKey); err != nil {
		panic(err)
	}
	// We use app.BankKeeper which is available from runtime.App (injected via depinject)
	// Actually, in Cosmos SDK v0.53, BankKeeper is usually injected into the App struct or we can get it from appBuilder.
	// For now, let's assume it's available or we can get it from the module balance.
	tokenomicsKeeper := tokenomics.NewKeeper(app.appCodec, tokenomicsStoreKey, app.BankKeeper)
	tokenomicsModule := tokenomics.NewAppModule(tokenomicsKeeper)

	registryStoreKey := storetypes.NewKVStoreKey(registry.StoreKey)
	if err := app.RegisterStores(registryStoreKey); err != nil {
		panic(err)
	}
	registryKeeper := registry.NewKeeper(app.appCodec, registryStoreKey, verifiersKeeper, tokenomicsKeeper, app.BankKeeper)
	registryModule := registry.NewAppModule(registryKeeper)

	if err := app.RegisterModules(&nodesModule, &registryModule, &verifiersModule, &tokenomicsModule); err != nil {
		panic(err)
	}
	app.RegistryKeeper = registryKeeper
	app.VerifiersKeeper = verifiersKeeper
	app.TokenomicsKeeper = tokenomicsKeeper
}

// registerCustomModuleOrders appends project modules to the init/export order so genesis includes them.
func registerCustomModuleOrders(app *App) {
	// Ensure custom modules participate in init/export and block processing.
	orderInit := appendUnique(runtimeInitGenesisOrder, nodes.ModuleName, registry.ModuleName, verifiers.ModuleName, tokenomics.ModuleName)
	orderExport := appendUnique(runtimeExportGenesisOrder, nodes.ModuleName, registry.ModuleName, verifiers.ModuleName, tokenomics.ModuleName)

	// The runtime module manager is built from AppConfig before we register custom modules.
	// Re-apply begin/end blocker ordering here so newly-registered modules (notably registry.EndBlock)
	// are actually invoked.
	orderBegin := appendUnique(runtimeBeginBlockersOrder, registry.ModuleName)
	orderEnd := appendUnique(runtimeEndBlockersOrder, registry.ModuleName)

	app.ModuleManager.SetOrderInitGenesis(orderInit...)
	app.ModuleManager.SetOrderExportGenesis(orderExport...)
	app.ModuleManager.SetOrderBeginBlockers(orderBegin...)
	app.ModuleManager.SetOrderEndBlockers(orderEnd...)
}

func appendUnique(list []string, values ...string) []string {
	seen := make(map[string]struct{}, len(list))
	for _, item := range list {
		seen[item] = struct{}{}
	}

	for _, val := range values {
		if _, ok := seen[val]; ok {
			continue
		}
		list = append(list, val)
		seen[val] = struct{}{}
	}
	return list
}

// RegisterAPIRoutes registers all API routes including swagger.
func (app *App) RegisterAPIRoutes(apiSvr *api.Server, apiConfig serverconfig.APIConfig) {
	app.App.RegisterAPIRoutes(apiSvr, apiConfig)
	if err := server.RegisterSwaggerAPI(apiSvr.ClientCtx, apiSvr.Router, apiConfig.Swagger); err != nil {
		panic(err)
	}
}

// AppCodec returns the binary codec used by the app.
func (app *App) AppCodec() codec.Codec {
	return app.appCodec
}

// LegacyAmino returns the legacy Amino codec for backwards compatibility.
func (app *App) LegacyAmino() *codec.LegacyAmino {
	return app.legacyAmino
}

// TxConfig exposes the configured transaction encoding.
func (app *App) TxConfig() client.TxConfig {
	return app.txConfig
}

// InterfaceRegistry returns the app's interface registry.
func (app *App) InterfaceRegistry() codectypes.InterfaceRegistry {
	return app.interfaceRegistry
}

// BasicModuleManager exposes the module basic manager for CLI scaffolding.
func (app *App) BasicModuleManager() module.BasicManager {
	return app.basicManager
}

// ExportAppStateAndValidators exports application state for genesis export helpers.
func (app *App) ExportAppStateAndValidators(forZeroHeight bool, jailAllowedAddrs, modulesToExport []string) (servertypes.ExportedApp, error) {
	ctx := app.BaseApp.NewContext(true)

	genesis, err := app.ModuleManager.ExportGenesisForModules(ctx, app.appCodec, modulesToExport)
	if err != nil {
		return servertypes.ExportedApp{}, err
	}

	appState, err := json.Marshal(genesis)
	if err != nil {
		return servertypes.ExportedApp{}, err
	}

	consensus := app.BaseApp.GetConsensusParams(ctx)

	return servertypes.ExportedApp{
		AppState:        appState,
		Validators:      []tmtypes.GenesisValidator{},
		Height:          app.LastBlockHeight(),
		ConsensusParams: consensus,
	}, nil
}

// NewEncodingConfig returns a basic encoding configuration for common tooling.
func NewEncodingConfig() (codec.Codec, *codec.LegacyAmino, codectypes.InterfaceRegistry) {
	var (
		cdc      codec.Codec
		amino    *codec.LegacyAmino
		registry codectypes.InterfaceRegistry
	)

	if err := depinject.Inject(
		depinject.Configs(
			AppConfig,
			depinject.Supply(log.NewNopLogger()),
		),
		&cdc,
		&amino,
		&registry,
	); err != nil {
		panic(err)
	}

	return cdc, amino, registry
}

// DefaultGenesis returns the default module genesis map used by `content-grid-d init`.
func DefaultGenesis() map[string]json.RawMessage {
	var appBuilder *runtime.AppBuilder

	if err := depinject.Inject(
		depinject.Configs(
			AppConfig,
			depinject.Supply(log.NewNopLogger()),
		),
		&appBuilder,
	); err != nil {
		panic(err)
	}

	genesis := appBuilder.DefaultGenesis()
	patchDefaultDenoms(genesis, tokenomics.DefaultDenom)
	return genesis
}

// patchDefaultDenoms normalizes SDK default staking/mint/gov denoms so `init`
// emits a genesis aligned with chain tokenomics defaults.
func patchDefaultDenoms(genesis map[string]json.RawMessage, denom string) {
	if len(genesis) == 0 || denom == "" {
		return
	}

	patchModule := func(moduleName string, patch func(map[string]any)) {
		raw, ok := genesis[moduleName]
		if !ok || len(raw) == 0 {
			return
		}
		var state map[string]any
		if err := json.Unmarshal(raw, &state); err != nil {
			return
		}
		patch(state)
		updated, err := json.Marshal(state)
		if err != nil {
			return
		}
		genesis[moduleName] = updated
	}

	patchCoins := func(v any) {
		coins, ok := v.([]any)
		if !ok {
			return
		}
		for _, item := range coins {
			coin, ok := item.(map[string]any)
			if !ok {
				continue
			}
			coin["denom"] = denom
		}
	}

	patchModule("staking", func(state map[string]any) {
		params, _ := state["params"].(map[string]any)
		if params == nil {
			return
		}
		params["bond_denom"] = denom
	})

	patchModule("mint", func(state map[string]any) {
		params, _ := state["params"].(map[string]any)
		if params == nil {
			return
		}
		params["mint_denom"] = denom
	})

	patchModule("gov", func(state map[string]any) {
		params, _ := state["params"].(map[string]any)
		if params == nil {
			return
		}
		patchCoins(params["min_deposit"])
		patchCoins(params["expedited_min_deposit"])
	})
}
