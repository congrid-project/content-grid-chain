package verifiers

import (
	"encoding/json"
	"fmt"

	"cosmossdk.io/store/prefix"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	sdkmath "cosmossdk.io/math"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
)

var (
	verifierStorePrefix = []byte{0x01}
	paramsStorePrefix   = []byte{0x02}
)

// Keeper stores verifier bonds and params.
type Keeper struct {
	cdc      codec.BinaryCodec
	storeKey storetypes.StoreKey
	bank     bankkeeper.Keeper
}

func NewKeeper(cdc codec.BinaryCodec, storeKey storetypes.StoreKey, bank bankkeeper.Keeper) Keeper {
	return Keeper{cdc: cdc, storeKey: storeKey, bank: bank}
}

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

func (k Keeper) GetParams(ctx sdk.Context) Params {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), paramsStorePrefix)
	bz := store.Get([]byte{0x00})
	if len(bz) == 0 {
		return DefaultParams()
	}
	var p Params
	if err := json.Unmarshal(bz, &p); err != nil {
		panic(fmt.Errorf("failed to decode verifiers params: %w", err))
	}
	return p
}

func (k Keeper) GetVerifier(ctx sdk.Context, addr string) (Verifier, bool) {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), verifierStorePrefix)
	bz := store.Get([]byte(addr))
	if len(bz) == 0 {
		return Verifier{}, false
	}
	v, err := unmarshalVerifier(bz)
	if err != nil {
		panic(fmt.Errorf("failed to decode verifier %s: %w", addr, err))
	}
	return v, true
}

func (k Keeper) SetVerifier(ctx sdk.Context, v Verifier) error {
	params := k.GetParams(ctx)
	if err := ValidateVerifier(v, params); err != nil {
		return err
	}
	store := prefix.NewStore(ctx.KVStore(k.storeKey), verifierStorePrefix)
	bz, err := marshalVerifier(v)
	if err != nil {
		return err
	}
	store.Set([]byte(v.Address), bz)
	return nil
}

func (k Keeper) ListVerifiers(ctx sdk.Context) []Verifier {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), verifierStorePrefix)
	iter := store.Iterator(nil, nil)
	defer iter.Close()
	out := []Verifier{}
	for ; iter.Valid(); iter.Next() {
		v, err := unmarshalVerifier(iter.Value())
		if err != nil {
			panic(fmt.Errorf("failed to decode verifier: %w", err))
		}
		out = append(out, v)
	}
	return out
}

// BondCoins moves coins into module escrow and updates bonded amount.
func (k Keeper) BondCoins(ctx sdk.Context, addr sdk.AccAddress, amount sdk.Coin) (Verifier, error) {
	params := k.GetParams(ctx)
	if amount.Denom == "" {
		amount = sdk.NewCoin(params.BondDenom, amount.Amount)
	}
	if amount.Denom != params.BondDenom {
		return Verifier{}, fmt.Errorf("bond denom must be %s", params.BondDenom)
	}
	if !amount.Amount.IsPositive() {
		return Verifier{}, fmt.Errorf("bond amount must be positive")
	}
	if err := k.bank.SendCoinsFromAccountToModule(ctx, addr, ModuleName, sdk.NewCoins(amount)); err != nil {
		return Verifier{}, err
	}

	existing, found := k.GetVerifier(ctx, addr.String())
	if !found {
		existing = Verifier{Address: addr.String(), Bond: sdk.NewCoin(params.BondDenom, sdkmath.ZeroInt()), Status: StatusActive}
	}
	existing.Bond = existing.Bond.Add(amount)
	existing.Status = StatusActive
	if err := k.SetVerifier(ctx, existing); err != nil {
		return Verifier{}, err
	}
	return existing, nil
}

// UnbondCoins releases escrowed coins back to the verifier.
func (k Keeper) UnbondCoins(ctx sdk.Context, addr sdk.AccAddress, amount sdk.Coin) (Verifier, error) {
	params := k.GetParams(ctx)
	if amount.Denom == "" {
		amount = sdk.NewCoin(params.BondDenom, amount.Amount)
	}
	if amount.Denom != params.BondDenom {
		return Verifier{}, fmt.Errorf("bond denom must be %s", params.BondDenom)
	}
	if !amount.Amount.IsPositive() {
		return Verifier{}, fmt.Errorf("unbond amount must be positive")
	}
	v, found := k.GetVerifier(ctx, addr.String())
	if !found {
		return Verifier{}, fmt.Errorf("verifier not bonded")
	}
	if v.Bond.Amount.LT(amount.Amount) {
		return Verifier{}, fmt.Errorf("insufficient bonded amount")
	}

	// Send coins out first; if this fails we keep the on-chain verifier state unchanged.
	if err := k.bank.SendCoinsFromModuleToAccount(ctx, ModuleName, addr, sdk.NewCoins(amount)); err != nil {
		return Verifier{}, err
	}

	v.Bond = sdk.NewCoin(params.BondDenom, v.Bond.Amount.Sub(amount.Amount))
	if v.Bond.Amount.LT(params.MinBond) {
		// Below minimum bond: remove from active set entirely.
		store := prefix.NewStore(ctx.KVStore(k.storeKey), verifierStorePrefix)
		store.Delete([]byte(v.Address))
		return v, nil
	}

	v.Status = StatusActive
	if err := k.SetVerifier(ctx, v); err != nil {
		return Verifier{}, err
	}
	return v, nil
}
