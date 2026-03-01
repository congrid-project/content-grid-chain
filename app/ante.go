package app

import (
	"fmt"
	"math"

	errorsmod "cosmossdk.io/errors"
	sdkmath "cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	authante "github.com/cosmos/cosmos-sdk/x/auth/ante"

	registrytypespb "content-grid-chain/x/registry/typespb"
)

func setCustomAnteHandler(app *App) error {
	if app == nil {
		return fmt.Errorf("nil app")
	}
	if app.AccountKeeper == nil {
		return fmt.Errorf("account keeper unavailable for custom ante handler")
	}

	anteHandler, err := authante.NewAnteHandler(authante.HandlerOptions{
		AccountKeeper:   app.AccountKeeper,
		BankKeeper:      app.BankKeeper,
		SignModeHandler: app.txConfig.SignModeHandler(),
		FeegrantKeeper:  app.FeeGrantKeeper,
		SigGasConsumer:  authante.DefaultSigVerificationGasConsumer,
		TxFeeChecker:    publisherScopedTxFeeChecker,
	})
	if err != nil {
		return fmt.Errorf("create custom ante handler: %w", err)
	}

	app.SetAnteHandler(anteHandler)
	return nil
}

func publisherScopedTxFeeChecker(ctx sdk.Context, tx sdk.Tx) (sdk.Coins, int64, error) {
	feeTx, ok := tx.(sdk.FeeTx)
	if !ok {
		return nil, 0, errorsmod.Wrap(sdkerrors.ErrTxDecode, "Tx must be a FeeTx")
	}

	feeCoins := feeTx.GetFee()
	gas := feeTx.GetGas()

	if isPublisherRegisterOnlyTx(tx) {
		// Registration-only txs are fee-free by policy, regardless of the tx-authored fee.
		return sdk.NewCoins(), 0, nil
	}

	// Default validator min-gas-price check for all other tx types.
	if ctx.IsCheckTx() {
		minGasPrices := ctx.MinGasPrices()
		if !minGasPrices.IsZero() {
			requiredFees := make(sdk.Coins, len(minGasPrices))
			glDec := sdkmath.LegacyNewDec(int64(gas))
			for i, gp := range minGasPrices {
				fee := gp.Amount.Mul(glDec)
				requiredFees[i] = sdk.NewCoin(gp.Denom, fee.Ceil().RoundInt())
			}
			if !feeCoins.IsAnyGTE(requiredFees) {
				return nil, 0, errorsmod.Wrapf(sdkerrors.ErrInsufficientFee, "insufficient fees; got: %s required: %s", feeCoins, requiredFees)
			}
		}
	}

	priority := getTxPriority(feeCoins, int64(gas))
	return feeCoins, priority, nil
}

func isPublisherRegisterOnlyTx(tx sdk.Tx) bool {
	if tx == nil {
		return false
	}
	msgs := tx.GetMsgs()
	if len(msgs) != 1 {
		return false
	}
	_, ok := msgs[0].(*registrytypespb.MsgRegisterPublisher)
	return ok
}

func getTxPriority(fee sdk.Coins, gas int64) int64 {
	if gas <= 0 {
		return 0
	}

	var priority int64
	for _, c := range fee {
		p := int64(math.MaxInt64)
		gasPrice := c.Amount.QuoRaw(gas)
		if gasPrice.IsInt64() {
			p = gasPrice.Int64()
		}
		if priority == 0 || p < priority {
			priority = p
		}
	}

	return priority
}
