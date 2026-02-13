package miners

import (
	"encoding/json"
	"fmt"

	"cosmossdk.io/store/prefix"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
)

var (
	minerStorePrefix  = []byte{0x01}
	paramsStorePrefix = []byte{0x02}
)

// Keeper manages miner registrations and params.
type Keeper struct {
	cdc      codec.BinaryCodec
	storeKey storetypes.StoreKey
}

// NewKeeper creates a keeper instance.
func NewKeeper(cdc codec.BinaryCodec, storeKey storetypes.StoreKey) Keeper {
	return Keeper{cdc: cdc, storeKey: storeKey}
}

// SetParams stores module params.
func (k Keeper) SetParams(ctx sdk.Context, params Params) error {
	if err := params.Validate(); err != nil {
		return err
	}
	store := prefix.NewStore(ctx.KVStore(k.storeKey), paramsStorePrefix)
	bz, err := json.Marshal(params)
	if err != nil {
		return err
	}
	store.Set([]byte{0x00}, bz)
	return nil
}

// GetParams returns module params.
func (k Keeper) GetParams(ctx sdk.Context) Params {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), paramsStorePrefix)
	bz := store.Get([]byte{0x00})
	if len(bz) == 0 {
		return DefaultParams()
	}
	var params Params
	if err := json.Unmarshal(bz, &params); err != nil {
		panic(fmt.Errorf("failed to decode miners params: %w", err))
	}
	return params
}

// SetMiner stores miner state.
func (k Keeper) SetMiner(ctx sdk.Context, miner Miner) error {
	params := k.GetParams(ctx)
	if err := ValidateMiner(miner, params); err != nil {
		return err
	}
	store := prefix.NewStore(ctx.KVStore(k.storeKey), minerStorePrefix)
	bz, err := marshalMiner(miner)
	if err != nil {
		return err
	}
	store.Set([]byte(miner.Operator), bz)
	return nil
}

// GetMiner fetches miner by operator address.
func (k Keeper) GetMiner(ctx sdk.Context, operator string) (Miner, bool) {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), minerStorePrefix)
	bz := store.Get([]byte(operator))
	if len(bz) == 0 {
		return Miner{}, false
	}
	miner, err := unmarshalMiner(bz)
	if err != nil {
		panic(fmt.Errorf("failed to decode miner %s: %w", operator, err))
	}
	return miner, true
}

// IterateMiners iterates all miners.
func (k Keeper) IterateMiners(ctx sdk.Context, cb func(Miner) bool) {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), minerStorePrefix)
	iter := store.Iterator(nil, nil)
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		miner, err := unmarshalMiner(iter.Value())
		if err != nil {
			panic(fmt.Errorf("failed to decode miner: %w", err))
		}
		if stop := cb(miner); stop {
			return
		}
	}
}

// DeleteMiner removes a miner.
func (k Keeper) DeleteMiner(ctx sdk.Context, operator string) {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), minerStorePrefix)
	store.Delete([]byte(operator))
}

// GetMinersPaginated returns miners and pagination.
func (k Keeper) GetMinersPaginated(ctx sdk.Context, req *sdkquery.PageRequest) ([]Miner, *sdkquery.PageResponse, error) {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), minerStorePrefix)
	var miners []Miner
	pageRes, err := sdkquery.Paginate(store, req, func(_, value []byte) error {
		miner, err := unmarshalMiner(value)
		if err != nil {
			return err
		}
		miners = append(miners, miner)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return miners, pageRes, nil
}
