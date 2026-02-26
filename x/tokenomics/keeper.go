package tokenomics

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	sdkmath "cosmossdk.io/math"
	"cosmossdk.io/store/prefix"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// BankKeeper defines the subset of bank functions needed for rewards.
type BankKeeper interface {
	SendCoinsFromModuleToAccount(ctx context.Context, senderModule string, recipientAddr sdk.AccAddress, amt sdk.Coins) error
	MintCoins(ctx context.Context, moduleName string, amt sdk.Coins) error
	BurnCoins(ctx context.Context, moduleName string, amt sdk.Coins) error
}

// Keeper manages the monetary policy and distribution logic.
type Keeper struct {
	cdc      codec.BinaryCodec
	storeKey storetypes.StoreKey
	bank     BankKeeper
}

// NewKeeper creates a new tokenomics keeper.
func NewKeeper(cdc codec.BinaryCodec, storeKey storetypes.StoreKey, bank BankKeeper) Keeper {
	return Keeper{
		cdc:      cdc,
		storeKey: storeKey,
		bank:     bank,
	}
}

// MintAndSend mints new coins to the tokenomics module account and sends them to the recipient.
func (k Keeper) MintAndSend(ctx sdk.Context, recipient sdk.AccAddress, coins sdk.Coins) error {
	if coins.IsZero() {
		return nil
	}
	if err := k.bank.MintCoins(ctx, ModuleName, coins); err != nil {
		return fmt.Errorf("failed to mint coins: %w", err)
	}
	if err := k.bank.SendCoinsFromModuleToAccount(ctx, ModuleName, recipient, coins); err != nil {
		return fmt.Errorf("failed to send coins: %w", err)
	}
	return nil
}

// MintAndBurn mints new coins and burns them immediately (used for unclaimed emissions).
func (k Keeper) MintAndBurn(ctx sdk.Context, coins sdk.Coins) error {
	if coins.IsZero() {
		return nil
	}
	if err := k.bank.MintCoins(ctx, ModuleName, coins); err != nil {
		return fmt.Errorf("failed to mint coins for burn: %w", err)
	}
	if err := k.bank.BurnCoins(ctx, ModuleName, coins); err != nil {
		return fmt.Errorf("failed to burn coins: %w", err)
	}
	return nil
}

func (k Keeper) getEmissionFundedAmount(ctx sdk.Context) sdkmath.Int {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), emissionMetaPrefix)
	bz := store.Get(emissionFundedAmountKey)
	if len(bz) == 0 {
		return sdkmath.ZeroInt()
	}
	amt, ok := sdkmath.NewIntFromString(string(bz))
	if !ok {
		return sdkmath.ZeroInt()
	}
	return amt
}

func (k Keeper) setEmissionFundedAmount(ctx sdk.Context, amount sdkmath.Int) {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), emissionMetaPrefix)
	if !amount.IsPositive() {
		store.Delete(emissionFundedAmountKey)
		return
	}
	store.Set(emissionFundedAmountKey, []byte(amount.String()))
}

// EnsureEmissionPool mints once (or top-ups) into the module account so payouts can be transfer-based.
func (k Keeper) EnsureEmissionPool(ctx sdk.Context, denom string, targetAmount sdkmath.Int) error {
	if strings.TrimSpace(denom) == "" {
		denom = DefaultDenom
	}
	if !targetAmount.IsPositive() {
		return nil
	}
	funded := k.getEmissionFundedAmount(ctx)
	if funded.GTE(targetAmount) {
		return nil
	}
	delta := targetAmount.Sub(funded)
	coins := sdk.NewCoins(sdk.NewCoin(denom, delta))
	if err := k.bank.MintCoins(ctx, ModuleName, coins); err != nil {
		return fmt.Errorf("failed to fund emission pool: %w", err)
	}
	k.setEmissionFundedAmount(ctx, targetAmount)
	return nil
}

// SendFromPool sends coins from tokenomics module balance without minting.
func (k Keeper) SendFromPool(ctx sdk.Context, recipient sdk.AccAddress, coins sdk.Coins) error {
	if coins.IsZero() {
		return nil
	}
	if err := k.bank.SendCoinsFromModuleToAccount(ctx, ModuleName, recipient, coins); err != nil {
		return fmt.Errorf("failed to send from pool: %w", err)
	}
	return nil
}

// BurnFromPool burns coins directly from tokenomics module balance.
func (k Keeper) BurnFromPool(ctx sdk.Context, coins sdk.Coins) error {
	if coins.IsZero() {
		return nil
	}
	if err := k.bank.BurnCoins(ctx, ModuleName, coins); err != nil {
		return fmt.Errorf("failed to burn from pool: %w", err)
	}
	return nil
}

var (
	paramsStorePrefix       = []byte{0x01}
	paramsKey               = []byte{0x00}
	emissionMetaPrefix      = []byte{0x02}
	emissionFundedAmountKey = []byte{0x00}
)

// SetParams persists tokenomics params.
func (k Keeper) SetParams(ctx sdk.Context, params Params) error {
	if err := params.Validate(); err != nil {
		return err
	}
	store := prefix.NewStore(ctx.KVStore(k.storeKey), paramsStorePrefix)
	bz, err := json.Marshal(params)
	if err != nil {
		return err
	}
	store.Set(paramsKey, bz)
	return nil
}

// GetParams fetches the stored params, defaulting if unset.
func (k Keeper) GetParams(ctx sdk.Context) Params {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), paramsStorePrefix)
	bz := store.Get(paramsKey)
	if len(bz) == 0 {
		return DefaultParams()
	}
	var params Params
	if err := json.Unmarshal(bz, &params); err != nil {
		return DefaultParams()
	}
	return params
}
