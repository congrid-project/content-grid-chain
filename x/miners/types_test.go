package miners

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"
)

func TestParamsValidate(t *testing.T) {
	params := DefaultParams()
	require.NoError(t, params.Validate())

	params.StakeDenom = ""
	require.Error(t, params.Validate())
}

func TestValidateMiner(t *testing.T) {
	params := DefaultParams()
	miner := Miner{
		Operator:    sdk.AccAddress([]byte("operator___________")).String(),
		MetadataURI: "https://miners.io/meta/1",
		Services:    ServiceFetch | ServiceEmbed,
		MinBid:      sdk.NewInt64Coin(params.StakeDenom, 2_000_000),
		Stake:       sdkmath.NewInt(5_000_000),
		Status:      StatusActive,
	}
	require.NoError(t, ValidateMiner(miner, params))

	miner.Services = 0
	require.Error(t, ValidateMiner(miner, params))
}
