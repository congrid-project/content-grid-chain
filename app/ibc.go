package app

import (
	storetypes "cosmossdk.io/store/types"

	"github.com/cosmos/cosmos-sdk/runtime"
	"github.com/cosmos/cosmos-sdk/types/module"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	transfer "github.com/cosmos/ibc-go/v10/modules/apps/transfer"
	transferkeeper "github.com/cosmos/ibc-go/v10/modules/apps/transfer/keeper"
	transfertypes "github.com/cosmos/ibc-go/v10/modules/apps/transfer/types"
	transferv2 "github.com/cosmos/ibc-go/v10/modules/apps/transfer/v2"
	ibc "github.com/cosmos/ibc-go/v10/modules/core"
	porttypes "github.com/cosmos/ibc-go/v10/modules/core/05-port/types"
	ibcapi "github.com/cosmos/ibc-go/v10/modules/core/api"
	ibcexported "github.com/cosmos/ibc-go/v10/modules/core/exported"
	ibckeeper "github.com/cosmos/ibc-go/v10/modules/core/keeper"
	ibctm "github.com/cosmos/ibc-go/v10/modules/light-clients/07-tendermint"
)

// IBCModuleBasics is shared by the application, genesis helpers and CLI. IBC
// keepers are wired manually because ibc-go does not provide depinject modules.
var IBCModuleBasics = module.NewBasicManager(
	ibc.AppModuleBasic{}, transfer.AppModuleBasic{}, ibctm.AppModuleBasic{},
)

func registerIBCModules(app *App) {
	ibcKey := storetypes.NewKVStoreKey(ibcexported.StoreKey)
	transferKey := storetypes.NewKVStoreKey(transfertypes.StoreKey)
	if err := app.RegisterStores(ibcKey, transferKey); err != nil {
		panic(err)
	}
	authority := authtypes.NewModuleAddress(govtypes.ModuleName).String()
	// Nil legacy subspaces are intentional: these are new stores, so there are
	// no pre-IBC-v8 x/params values to migrate.
	app.IBCKeeper = ibckeeper.NewKeeper(app.appCodec, runtime.NewKVStoreService(ibcKey), nil, app.UpgradeKeeper, authority)
	app.TransferKeeper = transferkeeper.NewKeeper(
		app.appCodec, runtime.NewKVStoreService(transferKey), nil,
		app.IBCKeeper.ChannelKeeper, app.IBCKeeper.ChannelKeeper,
		app.MsgServiceRouter(), app.AccountKeeper, app.BankKeeper, authority,
	)
	app.IBCKeeper.SetRouter(porttypes.NewRouter().AddRoute(transfertypes.PortID, transfer.NewIBCModule(app.TransferKeeper)))
	app.IBCKeeper.SetRouterV2(ibcapi.NewRouter().AddRoute(transfertypes.PortID, transferv2.NewIBCModule(app.TransferKeeper)))

	client := ibctm.NewLightClientModule(app.appCodec, app.IBCKeeper.ClientKeeper.GetStoreProvider())
	app.IBCKeeper.ClientKeeper.AddRoute(ibctm.ModuleName, &client)
	if err := app.RegisterModules(ibc.NewAppModule(app.IBCKeeper), transfer.NewAppModule(app.TransferKeeper), ibctm.NewAppModule(client)); err != nil {
		panic(err)
	}
}
