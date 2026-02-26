package tokenomics_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	sdkmath "cosmossdk.io/math"

	"content-grid-chain/x/tokenomics"
)

func TestDefaultParamsValidate(t *testing.T) {
	params := tokenomics.DefaultParams()
	require.NoError(t, params.Validate())
}

func TestInflationRate(t *testing.T) {
	params := tokenomics.DefaultParams()
	ip := params.Inflation

	high := ip.InflationRate(sdkmath.LegacyMustNewDecFromStr("0.45"))
	require.True(t, high.Equal(ip.MaxRate))

	low := ip.InflationRate(sdkmath.LegacyMustNewDecFromStr("0.80"))
	require.True(t, low.Equal(ip.MinRate))

	base := ip.InflationRate(sdkmath.LegacyMustNewDecFromStr("0.60"))
	require.InDelta(t, 0.07, base.MustFloat64(), 0.0001)
}

func TestAllocationBreakdown(t *testing.T) {
	gs := tokenomics.DefaultGenesisState()
	breakdown, err := gs.AllocationBreakdown()
	require.NoError(t, err)

	sum := sdkmath.LegacyZeroDec()
	for _, amount := range breakdown {
		require.True(t, amount.IsPositive())
		sum = sum.Add(sdkmath.LegacyNewDecFromInt(amount))
	}
	require.True(t, sum.Equal(sdkmath.LegacyNewDecFromInt(gs.InitialSupply)))
}

func TestBlockRewardSplit(t *testing.T) {
	params := tokenomics.DefaultParams()
	minted := sdkmath.LegacyMustNewDecFromStr("1000")
	split := params.SplitBlockRewards(minted)
	require.InDelta(t, 250.0, split.Staking.MustFloat64(), 0.001)
	require.InDelta(t, 650.0, split.Publishers.MustFloat64(), 0.001)
	require.InDelta(t, 100.0, split.Community.MustFloat64(), 0.001)
}

func TestSimulateYears(t *testing.T) {
	gs := tokenomics.DefaultGenesisState()
	cfg := tokenomics.DefaultSimulationConfig(gs)
	params := tokenomics.DefaultParams()

	projections, err := tokenomics.SimulateYears(params, cfg)
	require.NoError(t, err)
	require.Len(t, projections, cfg.Years)

	require.True(t, projections[len(projections)-1].EndSupply.GT(projections[0].StartSupply))
}
