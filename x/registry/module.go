package registry

import (
	"context"
	stdjson "encoding/json"

	"github.com/grpc-ecosystem/grpc-gateway/runtime"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"

	typespb "content-grid-chain/x/registry/typespb"
)

// ModuleName defines the module name.
const ModuleName = "registry"

// AppModuleBasic implements the AppModuleBasic interface for genesis and codec basics.
type AppModuleBasic struct{}

// Name returns the registry module's name.
func (AppModuleBasic) Name() string { return ModuleName }

// RegisterLegacyAminoCodec registers legacy amino (no-op for now).
func (AppModuleBasic) RegisterLegacyAminoCodec(cdc *codec.LegacyAmino) {}

// RegisterInterfaces registers interfaces for msg/query services.
func (AppModuleBasic) RegisterInterfaces(registry types.InterfaceRegistry) {
	registry.RegisterImplementations((*sdk.Msg)(nil),
		&typespb.MsgRegisterPublisher{},
		&typespb.MsgSubmitVerificationCommit{},
		&typespb.MsgRevealVerification{},
		&typespb.MsgCreateSlot{},
		&typespb.MsgUpdateSlotStatus{},
		&typespb.MsgLeaseSlot{},
		&typespb.MsgSubmitDrandBeacon{},
	)
}

// DefaultGenesis returns default genesis state as JSON.
func (AppModuleBasic) DefaultGenesis(cdc codec.JSONCodec) stdjson.RawMessage { //nolint:revive // keep signature
	_ = cdc // unused for now; we use std json as types are not protobuf
	gs := DefaultGenesis()
	bz, _ := stdjson.Marshal(gs)
	return stdjson.RawMessage(bz)
}

// ValidateGenesis validates the genesis state.
func (AppModuleBasic) ValidateGenesis(cdc codec.JSONCodec, _ client.TxEncodingConfig, bz stdjson.RawMessage) error { //nolint:revive // keep signature
	_ = cdc // unused for now; we use std json as types are not protobuf
	var gs GenesisState
	if len(bz) == 0 {
		gs = *DefaultGenesis()
	} else if err := stdjson.Unmarshal(bz, &gs); err != nil {
		return err
	}
	return gs.Validate()
}

// RegisterGRPCGatewayRoutes exposes query handlers via gRPC gateway.
func (AppModuleBasic) RegisterGRPCGatewayRoutes(clientCtx client.Context, mux *runtime.ServeMux) {
	_ = clientCtx
	_ = mux
}

// AppModule provides the runtime AppModule implementation.
type AppModule struct {
	AppModuleBasic
	keeper Keeper
}

var _ module.AppModule = AppModule{}

// NewAppModule returns a new AppModule instance.
func NewAppModule(keeper Keeper) AppModule {
	return AppModule{keeper: keeper}
}

// RegisterInvariants registers invariants (none today).
func (AppModule) RegisterInvariants(_ sdk.InvariantRegistry) {}

// RegisterServices wires the gRPC services.
func (a AppModule) RegisterServices(cfg module.Configurator) {
	typespb.RegisterMsgServer(cfg.MsgServer(), NewMsgServerImpl(a.keeper))
	typespb.RegisterQueryServer(cfg.QueryServer(), NewQueryServer(a.keeper))
}

// EndBlock runs verification round processing after transactions.
func (a AppModule) EndBlock(ctx context.Context) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	return a.keeper.EndBlock(sdkCtx)
}

// IsAppModule marks this struct as a Cosmos SDK AppModule.
func (AppModule) IsAppModule() {}

// IsOnePerModuleType indicates this module should only be registered once.
func (AppModule) IsOnePerModuleType() {}

// InitGenesis validates and persists the initial state.
// NOTE: this uses the legacy HasGenesis signature (no return) so module.Manager
// executes it during chain initialization.
func (a AppModule) InitGenesis(ctx sdk.Context, cdc codec.JSONCodec, data stdjson.RawMessage) {
	var gs GenesisState
	if len(data) == 0 {
		gs = *DefaultGenesis()
	} else if err := stdjson.Unmarshal(data, &gs); err != nil {
		panic(err)
	}
	if err := gs.Validate(); err != nil {
		panic(err)
	}
	if err := a.keeper.SetParams(ctx, gs.Params); err != nil {
		panic(err)
	}
	for _, website := range gs.Websites {
		if err := a.keeper.UpsertWebsite(ctx, website); err != nil {
			panic(err)
		}
	}
}

// ExportGenesis exports module genesis state.
func (a AppModule) ExportGenesis(ctx sdk.Context, cdc codec.JSONCodec) stdjson.RawMessage {
	state := DefaultGenesis()
	state.Params = a.keeper.GetParams(ctx)
	a.keeper.IterateWebsites(ctx, func(website Website) bool {
		state.Websites = append(state.Websites, website)
		return false
	})
	bz, err := stdjson.Marshal(state)
	if err != nil {
		panic(err)
	}
	return stdjson.RawMessage(bz)
}

// ConsensusVersion returns the module consensus version.
func (AppModule) ConsensusVersion() uint64 { return 1 }
