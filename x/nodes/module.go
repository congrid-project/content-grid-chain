package nodes

import (
	stdjson "encoding/json"

	"github.com/grpc-ecosystem/grpc-gateway/runtime"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
)

// ModuleName defines the module name.
const ModuleName = "nodes"

// AppModuleBasic implements the AppModuleBasic interface for genesis and codec basics.
type AppModuleBasic struct{}

// Name returns the nodes module name.
func (AppModuleBasic) Name() string { return ModuleName }

// RegisterLegacyAminoCodec registers legacy amino (no-op for now).
func (AppModuleBasic) RegisterLegacyAminoCodec(cdc *codec.LegacyAmino) {}

// RegisterInterfaces registers interfaces (no messages defined yet).
func (AppModuleBasic) RegisterInterfaces(registry types.InterfaceRegistry) {}

// DefaultGenesis returns default genesis state as JSON.
func (AppModuleBasic) DefaultGenesis(cdc codec.JSONCodec) stdjson.RawMessage { //nolint:revive // keep signature
	_ = cdc // unused until proto types land
	gs := DefaultGenesis()
	bz, err := stdjson.Marshal(gs)
	if err != nil {
		panic(err)
	}
	return stdjson.RawMessage(bz)
}

// ValidateGenesis validates the genesis state.
func (AppModuleBasic) ValidateGenesis(cdc codec.JSONCodec, _ client.TxEncodingConfig, bz stdjson.RawMessage) error { //nolint:revive // keep signature
	_ = cdc // unused until proto types land
	var gs GenesisState
	if len(bz) == 0 {
		gs = *DefaultGenesis()
	} else if err := stdjson.Unmarshal(bz, &gs); err != nil {
		return err
	}
	return gs.Validate()
}

// RegisterGRPCGatewayRoutes is a no-op for now.
func (AppModuleBasic) RegisterGRPCGatewayRoutes(_ client.Context, _ *runtime.ServeMux) {}

// AppModule provides the runtime module implementation hooks (currently stateless).
type AppModule struct {
	AppModuleBasic
}

var _ module.AppModule = AppModule{}

// NewAppModule returns a new AppModule instance.
func NewAppModule() AppModule { return AppModule{} }

// RegisterInvariants registers invariants (none for now).
func (AppModule) RegisterInvariants(_ sdk.InvariantRegistry) {}

// RegisterServices registers gRPC services (none today).
func (AppModule) RegisterServices(_ module.Configurator) {}

// IsAppModule marks this struct as a Cosmos SDK AppModule.
func (AppModule) IsAppModule() {}

// IsOnePerModuleType indicates this module should only be registered once.
func (AppModule) IsOnePerModuleType() {}

// InitGenesis validates the provided genesis state.
func (AppModule) InitGenesis(_ sdk.Context, cdc codec.JSONCodec, data stdjson.RawMessage) {
	_ = cdc
	var gs GenesisState
	if len(data) == 0 {
		gs = *DefaultGenesis()
	} else if err := stdjson.Unmarshal(data, &gs); err != nil {
		panic(err)
	}
	if err := gs.Validate(); err != nil {
		panic(err)
	}
}

// ExportGenesis exports the current state (stateless default).
func (AppModule) ExportGenesis(_ sdk.Context, cdc codec.JSONCodec) stdjson.RawMessage {
	_ = cdc
	gs := DefaultGenesis()
	bz, err := stdjson.Marshal(gs)
	if err != nil {
		panic(err)
	}
	return stdjson.RawMessage(bz)
}

// ConsensusVersion returns the module consensus version.
func (AppModule) ConsensusVersion() uint64 { return 1 }
