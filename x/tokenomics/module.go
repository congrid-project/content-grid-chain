package tokenomics

import (
	stdjson "encoding/json"

	"github.com/grpc-ecosystem/grpc-gateway/runtime"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
)

// StoreKey defines the module store key.
const StoreKey = ModuleName

// ModuleName defines the module name used in app wiring.
const ModuleName = "tokenomics"

// AppModuleBasic implements the AppModuleBasic interface for codec and genesis wiring.
type AppModuleBasic struct{}

// Name returns the module name.
func (AppModuleBasic) Name() string { return ModuleName }

// RegisterLegacyAminoCodec is a no-op for now.
func (AppModuleBasic) RegisterLegacyAminoCodec(*codec.LegacyAmino) {}

// RegisterInterfaces is a no-op until proto types are added.
func (AppModuleBasic) RegisterInterfaces(types.InterfaceRegistry) {}

// DefaultGenesis returns the default genesis state.
func (AppModuleBasic) DefaultGenesis(cdc codec.JSONCodec) stdjson.RawMessage { //nolint:revive // signature set by interface
	gs := DefaultGenesisState()
	bz, err := stdjson.Marshal(gs)
	if err != nil {
		panic(err)
	}
	return stdjson.RawMessage(bz)
}

// ValidateGenesis ensures the supplied genesis is well-formed.
func (AppModuleBasic) ValidateGenesis(cdc codec.JSONCodec, _ client.TxEncodingConfig, bz stdjson.RawMessage) error { //nolint:revive // signature set by interface
	var gs GenesisState
	if len(bz) == 0 {
		gs = DefaultGenesisState()
	} else if err := stdjson.Unmarshal(bz, &gs); err != nil {
		return err
	}
	return gs.Validate()
}

// RegisterGRPCGatewayRoutes exposes REST endpoints (none yet).
func (AppModuleBasic) RegisterGRPCGatewayRoutes(client.Context, *runtime.ServeMux) {}

// AppModule implements the runtime module hooks.
type AppModule struct {
	AppModuleBasic
	keeper Keeper
}

var _ module.AppModule = AppModule{}

// NewAppModule constructs a new AppModule instance.
func NewAppModule(keeper Keeper) AppModule {
	return AppModule{
		AppModuleBasic: AppModuleBasic{},
		keeper:         keeper,
	}
}

// RegisterInvariants registers invariants (none for now).
func (AppModule) RegisterInvariants(sdk.InvariantRegistry) {}

// RegisterServices registers module services (none yet).
func (AppModule) RegisterServices(module.Configurator) {}

// IsAppModule marks the struct as a Cosmos SDK module.
func (AppModule) IsAppModule() {}

// IsOnePerModuleType ensures the module is unique in depinject wiring.
func (AppModule) IsOnePerModuleType() {}

// InitGenesis validates the provided genesis state.
func (a AppModule) InitGenesis(ctx sdk.Context, cdc codec.JSONCodec, data stdjson.RawMessage) {
	var gs GenesisState
	if len(data) == 0 {
		gs = DefaultGenesisState()
	} else if err := stdjson.Unmarshal(data, &gs); err != nil {
		panic(err)
	}
	if err := gs.Validate(); err != nil {
		panic(err)
	}
	// Persist params so the tokenomics KV store has a committed version from genesis.
	if err := a.keeper.SetParams(ctx, gs.Params); err != nil {
		panic(err)
	}
}

// ExportGenesis returns the stateless default genesis.
func (AppModule) ExportGenesis(_ sdk.Context, _ codec.JSONCodec) stdjson.RawMessage {
	gs := DefaultGenesisState()
	bz, err := stdjson.Marshal(gs)
	if err != nil {
		panic(err)
	}
	return stdjson.RawMessage(bz)
}

// ConsensusVersion identifies the module consensus version.
func (AppModule) ConsensusVersion() uint64 { return 1 }
