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

func TestSplitIssuance(t *testing.T) {
	params := tokenomics.DefaultParams()
	issued := sdkmath.LegacyMustNewDecFromStr("1000")
	split := params.SplitIssuance(issued)
	require.InDelta(t, 400.0, split.OperatorReserve.MustFloat64(), 0.001)
	require.InDelta(t, 100.0, split.Publishers.MustFloat64(), 0.001)
	require.InDelta(t, 500.0, split.Verifiers.MustFloat64(), 0.001)
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

func TestSimulateYears(t *testing.T) {
	gs := tokenomics.DefaultGenesisState()
	cfg := tokenomics.DefaultSimulationConfig(gs)
	params := tokenomics.DefaultParams()

	projections, err := tokenomics.SimulateYears(params, cfg)
	require.NoError(t, err)
	require.Len(t, projections, cfg.Years)

	require.True(t, projections[0].OperatorIssued.IsPositive())
	require.True(t, projections[0].PublisherIssued.IsPositive())
	require.True(t, projections[0].VerifierIssued.IsPositive())
	require.True(t, projections[0].CumulativeIssued.IsPositive())
	require.True(t, projections[len(projections)-1].CumulativeIssued.GTE(projections[0].CumulativeIssued))
}
