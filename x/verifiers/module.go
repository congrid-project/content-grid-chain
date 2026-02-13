package verifiers

import (
	stdjson "encoding/json"

	"github.com/grpc-ecosystem/grpc-gateway/runtime"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"

	typespb "content-grid-chain/x/verifiers/typespb"
)

// ModuleName defines the module name.
const ModuleName = "verifiers"

// StoreKey defines the primary module store key.
const StoreKey = ModuleName

// AppModuleBasic implements the AppModuleBasic interface.
type AppModuleBasic struct{}

func (AppModuleBasic) Name() string { return ModuleName }

func (AppModuleBasic) RegisterLegacyAminoCodec(cdc *codec.LegacyAmino) {}

func (AppModuleBasic) RegisterInterfaces(registry codectypes.InterfaceRegistry) {
	registry.RegisterImplementations((*sdk.Msg)(nil), &typespb.MsgBond{}, &typespb.MsgUnbond{})
}

func (AppModuleBasic) DefaultGenesis(cdc codec.JSONCodec) stdjson.RawMessage { //nolint:revive
	_ = cdc
	gs := DefaultGenesis()
	bz, _ := stdjson.Marshal(gs)
	return stdjson.RawMessage(bz)
}

func (AppModuleBasic) ValidateGenesis(cdc codec.JSONCodec, _ client.TxEncodingConfig, bz stdjson.RawMessage) error { //nolint:revive
	_ = cdc
	var gs GenesisState
	if len(bz) == 0 {
		gs = *DefaultGenesis()
	} else if err := stdjson.Unmarshal(bz, &gs); err != nil {
		return err
	}
	return gs.Validate()
}

func (AppModuleBasic) RegisterGRPCGatewayRoutes(_ client.Context, _ *runtime.ServeMux) {}

// AppModule provides a runtime module implementation.
type AppModule struct {
	AppModuleBasic
	keeper Keeper
}

var _ module.AppModule = AppModule{}

func NewAppModule(k Keeper) AppModule {
	return AppModule{keeper: k}
}

func (AppModule) RegisterInvariants(_ sdk.InvariantRegistry) {}

func (a AppModule) RegisterServices(cfg module.Configurator) {
	typespb.RegisterMsgServer(cfg.MsgServer(), NewMsgServerImpl(a.keeper))
	typespb.RegisterQueryServer(cfg.QueryServer(), NewQueryServer(a.keeper))
}

func (AppModule) IsAppModule() {}

func (AppModule) IsOnePerModuleType() {}

func (a AppModule) InitGenesis(ctx sdk.Context, cdc codec.JSONCodec, data stdjson.RawMessage) {
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
	if err := a.keeper.SetParams(ctx, gs.Params); err != nil {
		panic(err)
	}
	for _, v := range gs.Verifiers {
		if err := a.keeper.SetVerifier(ctx, v); err != nil {
			panic(err)
		}
	}
}

func (a AppModule) ExportGenesis(ctx sdk.Context, cdc codec.JSONCodec) stdjson.RawMessage {
	_ = cdc
	gs := DefaultGenesis()
	gs.Params = a.keeper.GetParams(ctx)
	gs.Verifiers = a.keeper.ListVerifiers(ctx)
	bz, err := stdjson.Marshal(gs)
	if err != nil {
		panic(err)
	}
	return stdjson.RawMessage(bz)
}

func (AppModule) ConsensusVersion() uint64 { return 1 }
