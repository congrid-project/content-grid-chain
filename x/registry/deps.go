package registry

import (
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"content-grid-chain/x/verifiers"
)

// VerifierKeeper exposes the verifier module queries used by registry.
type VerifierKeeper interface {
	ListVerifiers(ctx sdk.Context) []verifiers.Verifier
	GetParams(ctx sdk.Context) verifiers.Params
}

// TokenomicsKeeper mints, burns, and sends rewards.
type TokenomicsKeeper interface {
	MintAndSend(ctx sdk.Context, recipient sdk.AccAddress, coins sdk.Coins) error
	MintAndBurn(ctx sdk.Context, coins sdk.Coins) error
	EnsureEmissionPool(ctx sdk.Context, denom string, targetAmount sdkmath.Int) error
	SendFromPool(ctx sdk.Context, recipient sdk.AccAddress, coins sdk.Coins) error
	BurnFromPool(ctx sdk.Context, coins sdk.Coins) error
}
