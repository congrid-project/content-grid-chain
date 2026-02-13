package types

import (
	"testing"

	"github.com/stretchr/testify/require"

	sdkmath "cosmossdk.io/math"
)

func TestParamsValidate(t *testing.T) {
	require.NoError(t, DefaultParams().Validate())

	bad := Params{MaxAssignments: 0, QuorumPercent: 50}
	require.Error(t, bad.Validate())
}

func TestRewardWeightsValidate(t *testing.T) {
	require.NoError(t, DefaultRewardWeights().Validate())

	weights := RewardWeights{Success: sdkmath.LegacyMustNewDecFromStr("0.5"), Consensus: sdkmath.LegacyMustNewDecFromStr("0.5"), Latency: sdkmath.LegacyZeroDec(), Availability: sdkmath.LegacyZeroDec()}
	require.NoError(t, weights.Validate())

	bad := RewardWeights{Success: sdkmath.LegacyMustNewDecFromStr("1.1"), Consensus: sdkmath.LegacyZeroDec(), Latency: sdkmath.LegacyZeroDec(), Availability: sdkmath.LegacyZeroDec()}
	require.Error(t, bad.Validate())
}

func TestWorkerPerformanceScore(t *testing.T) {
	weights := DefaultRewardWeights()
	perf := WorkerPerformance{
		Assignments:     100,
		Successful:      98,
		Consensus:       97,
		MedianLatencyMs: 5500,
		TargetLatencyMs: 6000,
		OnlineBlocks:    9500,
		ExpectedOnline:  10000,
	}

	score, err := perf.RewardScore(weights)
	require.NoError(t, err)
	val := score.MustFloat64()
	if !(val > 0.65) {
		t.Fatalf("expected score > 0.65 got %f", val)
	}

	zeroAssignments := WorkerPerformance{}
	zeroScore, err := zeroAssignments.RewardScore(weights)
	require.NoError(t, err)
	require.True(t, zeroScore.IsZero())
}
