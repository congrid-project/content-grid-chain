package miners

import (
	stdjson "encoding/json"
	"fmt"

	"github.com/grpc-ecosystem/grpc-gateway/runtime"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"

	typespb "content-grid-chain/x/miners/typespb"
)

// ModuleName defines the module name.
const ModuleName = "miners"

// AppModuleBasic implements codec and genesis hooks.
type AppModuleBasic struct{}

// Name returns module name.
func (AppModuleBasic) Name() string { return ModuleName }

// RegisterLegacyAminoCodec is a no-op.
func (AppModuleBasic) RegisterLegacyAminoCodec(*codec.LegacyAmino) {}

// RegisterInterfaces registers protobuf types.
func (AppModuleBasic) RegisterInterfaces(registry types.InterfaceRegistry) {
	registry.RegisterImplementations((*sdk.Msg)(nil),
		&typespb.MsgRegisterMiner{},
		&typespb.MsgUpdateMiner{},
		&typespb.MsgUpdateMinerStake{},
	)
}

// DefaultGenesis returns empty state.
func (AppModuleBasic) DefaultGenesis(cdc codec.JSONCodec) stdjson.RawMessage {
	gs := DefaultGenesisState()
	bz, err := stdjson.Marshal(gs)
	if err != nil {
		panic(err)
	}
	return stdjson.RawMessage(bz)
}

// ValidateGenesis validates genesis.
func (AppModuleBasic) ValidateGenesis(cdc codec.JSONCodec, _ client.TxEncodingConfig, bz stdjson.RawMessage) error {
	var gs GenesisState
	if len(bz) == 0 {
		gs = DefaultGenesisState()
	} else if err := stdjson.Unmarshal(bz, &gs); err != nil {
		return err
	}
	return gs.Validate()
}

// RegisterGRPCGatewayRoutes registers gRPC routes.
func (AppModuleBasic) RegisterGRPCGatewayRoutes(client.Context, *runtime.ServeMux) {}

// AppModule implements module.AppModule.
type AppModule struct {
	AppModuleBasic
	keeper Keeper
}

var _ module.AppModule = AppModule{}

// NewAppModule creates AppModule.
func NewAppModule(keeper Keeper) AppModule {
	return AppModule{keeper: keeper}
}

// IsAppModule marks struct as AppModule.
func (AppModule) IsAppModule() {}

// IsOnePerModuleType ensures single registration.
func (AppModule) IsOnePerModuleType() {}

// RegisterInvariants is a no-op.
func (AppModule) RegisterInvariants(sdk.InvariantRegistry) {}

// RegisterServices registers Msg/Query services.
func (a AppModule) RegisterServices(cfg module.Configurator) {
	typespb.RegisterMsgServer(cfg.MsgServer(), NewMsgServerImpl(a.keeper))
	typespb.RegisterQueryServer(cfg.QueryServer(), NewQueryServer(a.keeper))
}

// InitGenesis initializes state.
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
	if err := a.keeper.SetParams(ctx, gs.Params); err != nil {
		panic(err)
	}
	for _, miner := range gs.Miners {
		if err := a.keeper.SetMiner(ctx, miner); err != nil {
			panic(err)
		}
	}
}

// ExportGenesis exports state.
func (a AppModule) ExportGenesis(ctx sdk.Context, cdc codec.JSONCodec) stdjson.RawMessage {
	state := GenesisState{
		Params: a.keeper.GetParams(ctx),
	}
	a.keeper.IterateMiners(ctx, func(miner Miner) bool {
		state.Miners = append(state.Miners, miner)
		return false
	})
	bz, err := stdjson.Marshal(state)
	if err != nil {
		panic(err)
	}
	return stdjson.RawMessage(bz)
}

// GenesisState defines module genesis data.
type GenesisState struct {
	Params Params  `json:"params"`
	Miners []Miner `json:"miners"`
}

// DefaultGenesisState returns default genesis.
func DefaultGenesisState() GenesisState {
	return GenesisState{
		Params: DefaultParams(),
		Miners: []Miner{},
	}
}

// Validate ensures genesis is valid.
func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(gs.Miners))
	for _, miner := range gs.Miners {
		if _, dup := seen[miner.Operator]; dup {
			return fmt.Errorf("duplicate miner %s", miner.Operator)
		}
		seen[miner.Operator] = struct{}{}
		if err := ValidateMiner(miner, gs.Params); err != nil {
			return err
		}
	}
	return nil
}
