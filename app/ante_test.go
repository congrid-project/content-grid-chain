package app

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"
	protov2 "google.golang.org/protobuf/proto"

	registrytypespb "content-grid-chain/x/registry/typespb"
)

type mockFeeTx struct {
	msgs []sdk.Msg
	fee  sdk.Coins
	gas  uint64
}

func (m mockFeeTx) GetMsgs() []sdk.Msg { return m.msgs }

func (m mockFeeTx) GetMsgsV2() ([]protov2.Message, error) { return nil, nil }

func (m mockFeeTx) GetGas() uint64 { return m.gas }

func (m mockFeeTx) GetFee() sdk.Coins { return m.fee }

func (m mockFeeTx) FeePayer() []byte { return []byte("congrid1payer") }

func (m mockFeeTx) FeeGranter() []byte { return nil }

func TestPublisherScopedTxFeeChecker_RegisterOnlyIsFeeFree(t *testing.T) {
	minGasPrices, err := sdk.ParseDecCoins("0.001ucongrid")
	require.NoError(t, err)

	ctx := sdk.Context{}.
		WithIsCheckTx(true).
		WithMinGasPrices(minGasPrices)

	tx := mockFeeTx{
		msgs: []sdk.Msg{&registrytypespb.MsgRegisterPublisher{Owner: "congrid1owner", Domain: "example.com"}},
		fee:  sdk.NewCoins(sdk.NewInt64Coin("ucongrid", 9999)),
		gas:  200000,
	}

	fee, _, err := publisherScopedTxFeeChecker(ctx, tx)
	require.NoError(t, err)
	require.True(t, fee.IsZero())
}

func TestPublisherScopedTxFeeChecker_NonRegisterStillRequiresFee(t *testing.T) {
	minGasPrices, err := sdk.ParseDecCoins("0.001ucongrid")
	require.NoError(t, err)

	ctx := sdk.Context{}.
		WithIsCheckTx(true).
		WithMinGasPrices(minGasPrices)

	tx := mockFeeTx{
		msgs: []sdk.Msg{&registrytypespb.MsgCreateSlot{Publisher: "congrid1owner", Domain: "example.com", Label: "main", RateDenom: "ucongrid", RateAmount: "1", UnitSeconds: 1, MinDurationSeconds: 1, MaxDurationSeconds: 1}},
		fee:  sdk.NewCoins(),
		gas:  200000,
	}

	_, _, err = publisherScopedTxFeeChecker(ctx, tx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "insufficient fees")
}

func TestPublisherScopedTxFeeChecker_MixedMsgsNotFeeFree(t *testing.T) {
	minGasPrices, err := sdk.ParseDecCoins("0.001ucongrid")
	require.NoError(t, err)

	ctx := sdk.Context{}.
		WithIsCheckTx(true).
		WithMinGasPrices(minGasPrices)

	tx := mockFeeTx{
		msgs: []sdk.Msg{
			&registrytypespb.MsgRegisterPublisher{Owner: "congrid1owner", Domain: "example.com"},
			&registrytypespb.MsgCreateSlot{Publisher: "congrid1owner", Domain: "example.com", Label: "main", RateDenom: "ucongrid", RateAmount: "1", UnitSeconds: 1, MinDurationSeconds: 1, MaxDurationSeconds: 1},
		},
		fee: sdk.NewCoins(),
		gas: 200000,
	}

	_, _, err = publisherScopedTxFeeChecker(ctx, tx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "insufficient fees")
}
