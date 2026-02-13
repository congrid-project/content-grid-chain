package tasks

import (
	stdjson "encoding/json"

	"github.com/grpc-ecosystem/grpc-gateway/runtime"

	"content-grid-chain/x/tasks/keeper"
	"content-grid-chain/x/tasks/types"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
)

// ModuleName defines the module name.
const ModuleName = "tasks"

// StoreKey defines the primary module store key.
const StoreKey = ModuleName

// AppModuleBasic implements the AppModuleBasic interface for genesis and codec basics.
type AppModuleBasic struct{}

// Name returns the tasks module name.
func (AppModuleBasic) Name() string { return ModuleName }

// RegisterLegacyAminoCodec registers legacy amino (no-op for now).
func (AppModuleBasic) RegisterLegacyAminoCodec(cdc *codec.LegacyAmino) {}

// RegisterInterfaces registers interfaces (no protobuf messages yet).
func (AppModuleBasic) RegisterInterfaces(registry codectypes.InterfaceRegistry) {}

// DefaultGenesis returns default genesis state as JSON.
func (AppModuleBasic) DefaultGenesis(cdc codec.JSONCodec) stdjson.RawMessage { //nolint:revive // keep signature
	_ = cdc // unused until proto types land
	gs := types.DefaultGenesis()
	bz, err := stdjson.Marshal(gs)
	if err != nil {
		panic(err)
	}
	return stdjson.RawMessage(bz)
}

// ValidateGenesis validates the genesis state.
func (AppModuleBasic) ValidateGenesis(cdc codec.JSONCodec, _ client.TxEncodingConfig, bz stdjson.RawMessage) error { //nolint:revive // keep signature
	_ = cdc // unused until proto types land
	var gs types.GenesisState
	if len(bz) == 0 {
		gs = *types.DefaultGenesis()
	} else if err := stdjson.Unmarshal(bz, &gs); err != nil {
		return err
	}
	return gs.Validate()
}

// RegisterGRPCGatewayRoutes is a no-op for now.
func (AppModuleBasic) RegisterGRPCGatewayRoutes(_ client.Context, _ *runtime.ServeMux) {}

// AppModule provides a runtime module implementation for tasks.
type AppModule struct {
	AppModuleBasic
	keeper keeper.Keeper
}

var _ module.AppModule = AppModule{}

// NewAppModule returns a new AppModule instance.
func NewAppModule(k keeper.Keeper) AppModule {
	return AppModule{
		AppModuleBasic: AppModuleBasic{},
		keeper:         k,
	}
}

// RegisterInvariants registers invariants (none yet).
func (AppModule) RegisterInvariants(_ sdk.InvariantRegistry) {}

// RegisterServices registers services (none yet).
func (AppModule) RegisterServices(_ module.Configurator) {}

// IsAppModule marks this struct as a Cosmos SDK AppModule.
func (AppModule) IsAppModule() {}

// IsOnePerModuleType indicates this module should only be registered once.
func (AppModule) IsOnePerModuleType() {}

// InitGenesis validates the genesis data.
func (a AppModule) InitGenesis(ctx sdk.Context, cdc codec.JSONCodec, data stdjson.RawMessage) {
	_ = cdc
	var gs types.GenesisState
	if len(data) == 0 {
		gs = *types.DefaultGenesis()
	} else if err := stdjson.Unmarshal(data, &gs); err != nil {
		panic(err)
	}
	if err := gs.Validate(); err != nil {
		panic(err)
	}
	// Persist params so the tasks KV store has a committed version from genesis.
	if err := a.keeper.SetParams(ctx, gs.Params); err != nil {
		panic(err)
	}
}

// ExportGenesis exports the default state as genesis data.
func (AppModule) ExportGenesis(_ sdk.Context, cdc codec.JSONCodec) stdjson.RawMessage {
	_ = cdc
	gs := types.DefaultGenesis()
	bz, err := stdjson.Marshal(gs)
	if err != nil {
		panic(err)
	}
	return stdjson.RawMessage(bz)
}

// ConsensusVersion returns the module consensus version.
func (AppModule) ConsensusVersion() uint64 { return 1 }
