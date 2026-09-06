package app

import (
	"fmt"
	"math"

	errorsmod "cosmossdk.io/errors"
	sdkmath "cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	authante "github.com/cosmos/cosmos-sdk/x/auth/ante"
	ibcante "github.com/cosmos/ibc-go/v10/modules/core/ante"

	registrytypespb "content-grid-chain/x/registry/typespb"
)

func setCustomAnteHandler(app *App) error {
	if app == nil {
		return fmt.Errorf("nil app")
	}

	if app.IBCKeeper == nil || app.txConfig == nil {
		return fmt.Errorf("IBC keeper and transaction config are required for ante handler")
	}
	// Keep the SDK v0.53 authentication/fee sequence, then apply IBC's
	// redundant-relay check inside the setup decorator's gas recovery scope.
	app.SetAnteHandler(sdk.ChainAnteDecorators(
		authante.NewSetUpContextDecorator(),
		authante.NewExtensionOptionsDecorator(nil),
		authante.NewValidateBasicDecorator(),
		authante.NewTxTimeoutHeightDecorator(),
		authante.NewValidateMemoDecorator(app.AccountKeeper),
		authante.NewConsumeGasForTxSizeDecorator(app.AccountKeeper),
		authante.NewDeductFeeDecorator(app.AccountKeeper, app.BankKeeper, app.FeeGrantKeeper, publisherScopedTxFeeChecker),
		authante.NewSetPubKeyDecorator(app.AccountKeeper),
		authante.NewValidateSigCountDecorator(app.AccountKeeper),
		authante.NewSigGasConsumeDecorator(app.AccountKeeper, authante.DefaultSigVerificationGasConsumer),
		authante.NewSigVerificationDecorator(app.AccountKeeper, app.txConfig.SignModeHandler()),
		authante.NewIncrementSequenceDecorator(app.AccountKeeper),
		ibcante.NewRedundantRelayDecorator(app.IBCKeeper),
	))
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
